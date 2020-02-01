package main

import (
	"database/sql"
	"github.com/jinzhu/gorm"
	"github.com/petrjahoda/zapsi_database"
	"sort"
	"strconv"
	"time"
)

func AddData(workplace zapsi_database.Workplace) []IntermediateData {
	connectionString, dialect := zapsi_database.CheckDatabaseType(DatabaseType, DatabaseIpAddress, DatabasePort, DatabaseLogin, DatabaseName, DatabasePassword)
	db, err := gorm.Open(dialect, connectionString)
	if err != nil {
		LogError(workplace.Name, "Problem opening "+DatabaseName+" database: "+err.Error())
		return nil
	}
	defer db.Close()
	var workplaceState zapsi_database.WorkplaceState
	db.Where("workplace_id=?", workplace.ID).Last(&workplaceState)
	poweroffRecords := DownloadPoweroffRecords(workplace.ID, db, workplaceState)
	productionRecords := DownloadProductionRecords(workplace.ID, db, workplaceState)
	intermediateData := CreateIntermediateData(poweroffRecords, productionRecords)
	return intermediateData
}

func CreateIntermediateData(poweroffRecords []zapsi_database.DeviceAnalogRecord, productionRecords []zapsi_database.DeviceDigitalRecord) []IntermediateData {
	var intermediateData []IntermediateData
	for _, poweroffRecord := range poweroffRecords {
		rawData := strconv.FormatFloat(float64(poweroffRecord.Data), 'g', 15, 64)
		data := IntermediateData{DateTime: poweroffRecord.DateTime, RawData: rawData, Type: poweroff}
		intermediateData = append(intermediateData, data)
	}
	for _, productionRecord := range productionRecords {
		rawData := strconv.FormatFloat(float64(productionRecord.Data), 'g', 15, 64)
		data := IntermediateData{DateTime: productionRecord.DateTime, RawData: rawData, Type: production}
		intermediateData = append(intermediateData, data)
	}
	sort.Slice(intermediateData, func(i, j int) bool {
		return intermediateData[i].DateTime.Before(intermediateData[j].DateTime)
	})
	return intermediateData
}

func DownloadProductionRecords(workplaceId uint, db *gorm.DB, workplaceState zapsi_database.WorkplaceState) []zapsi_database.DeviceDigitalRecord {
	var production zapsi_database.State
	db.Where("name=?", "Production").Find(&production)
	var productionPort zapsi_database.WorkplacePort
	db.Where("workplace_id=?", workplaceId).Where("state_id=?", production.ID).First(&productionPort)
	var productionRecords []zapsi_database.DeviceDigitalRecord
	db.Where("device_port_id=?", productionPort.DevicePortId).Where("date_time > ?", workplaceState.DateTimeStart).Find(&productionRecords)
	return productionRecords
}

func DownloadPoweroffRecords(workplaceId uint, db *gorm.DB, workplaceState zapsi_database.WorkplaceState) []zapsi_database.DeviceAnalogRecord {
	var poweroff zapsi_database.State
	db.Where("name=?", "Poweroff").Find(&poweroff)
	var poweroffPort zapsi_database.WorkplacePort
	db.Where("workplace_id=?", workplaceId).Where("state_id=?", poweroff.ID).First(&poweroffPort)
	var poweroffRecords []zapsi_database.DeviceAnalogRecord
	db.Where("device_port_id=?", poweroffPort.DevicePortId).Where("date_time > ?", workplaceState.DateTimeStart).Find(&poweroffRecords)
	return poweroffRecords
}

//
func GetLatestWorkplaceStateId(workplaceId uint, db *gorm.DB) int {
	var workplaceState zapsi_database.WorkplaceState
	db.Where("workplace_id=?", workplaceId).Last(&workplaceState)
	return int(workplaceState.StateId)
}

func GetActualState(latestworkplaceStateId int, db *gorm.DB) zapsi_database.State {
	var actualState zapsi_database.State
	db.Where("id=?", latestworkplaceStateId).Find(&actualState)
	return actualState
}

func ProcessData(workplace *zapsi_database.Workplace, data []IntermediateData) {
	connectionString, dialect := zapsi_database.CheckDatabaseType(DatabaseType, DatabaseIpAddress, DatabasePort, DatabaseLogin, DatabaseName, DatabasePassword)
	db, err := gorm.Open(dialect, connectionString)
	if err != nil {
		LogError(workplace.Name, "Problem opening "+DatabaseName+" database: "+err.Error())
		return
	}
	defer db.Close()
	var actualWorkplaceMode zapsi_database.WorkplaceMode
	db.Where("id=?", workplace.ActualWorkplaceModeId).Find(&actualWorkplaceMode)
	poweroffInterval := actualWorkplaceMode.PoweroffInterval
	downtimeInterval := actualWorkplaceMode.DowntimeInterval
	var actualState zapsi_database.State
	latestworkplaceStateId := GetLatestWorkplaceStateId(workplace.ID, db)
	actualState = GetActualState(latestworkplaceStateId, db)
	for _, actualData := range data {
		if actualData.Type == poweroff {
			workplace.PoweroffPortDateTime = sql.NullTime{Time: actualData.DateTime}
		} else if actualData.Type == production {
			workplace.PoweroffPortDateTime = sql.NullTime{Time: actualData.DateTime}
			workplace.ProductionPortDateTime = sql.NullTime{Time: actualData.DateTime}
		}
		switch actualState.Name {
		case "Poweroff":
			{
				if actualData.Type == production && actualData.RawData == "1" {
					UpdateState(db, &workplace, actualData.DateTime, "Production")
					actualState.Name = "Production"
					break
				}
				if actualData.Type == poweroff {
					UpdateState(db, &workplace, actualData.DateTime, "Downtime")
					actualState.Name = "Downtime"

					break
				}
			}
		case "Production":
			{

				workplacePoweroffDifference := uint(actualData.DateTime.Sub(workplace.PoweroffPortDateTime.Time).Seconds())
				if workplacePoweroffDifference > poweroffInterval {
					UpdateState(db, &workplace, workplace.PoweroffPortDateTime.Time, "Poweroff")
					actualState.Name = "Poweroff"

					if actualData.Type == production && actualData.RawData == "1" {
						UpdateState(db, &workplace, actualData.DateTime, "Production")
						actualState.Name = "Production"

						break
					}
					UpdateState(db, &workplace, actualData.DateTime, "Downtime")
					actualState.Name = "Downtime"

				} else {
					workplaceDowntimeDifference := uint(actualData.DateTime.Sub(workplace.ProductionPortDateTime.Time).Seconds())
					if workplace.ProductionPortValue.Int32 == 0 && workplaceDowntimeDifference > downtimeInterval {
						UpdateState(db, &workplace, workplace.ProductionPortDateTime.Time, "Downtime")
						actualState.Name = "Downtime"
						break
					}
				}
			}
		case "Downtime":
			{
				workplacePoweroffDifference := uint(actualData.DateTime.Sub(workplace.PoweroffPortDateTime.Time).Seconds())
				if workplacePoweroffDifference > poweroffInterval {
					UpdateState(db, &workplace, workplace.PoweroffPortDateTime.Time, "Poweroff")
					actualState.Name = "Poweroff"

					if actualData.Type == production && actualData.RawData == "1" {
						UpdateState(db, &workplace, actualData.DateTime, "Production")
						actualState.Name = "Production"
						break
					}
					UpdateState(db, &workplace, actualData.DateTime, "Downtime")
					actualState.Name = "Downtime"

					break
				} else {
					if actualData.Type == production && actualData.RawData == "1" {
						UpdateState(db, &workplace, actualData.DateTime, "Production")
						actualState.Name = "Production"

						break
					}
				}
			}
		default:
			{
				if actualData.Type == production && actualData.RawData == "1" {
					UpdateState(db, &workplace, actualData.DateTime, "Production")
					actualState.Name = "Production"

					break
				}
				if actualData.Type == poweroff {
					UpdateState(db, &workplace, actualData.DateTime, "Downtime")
					actualState.Name = "Downtime"

					break
				}
			}
		}
	}
	workplacePoweroffDifference := uint(time.Now().UTC().Sub(workplace.PoweroffPortDateTime.Time).Seconds())
	if workplacePoweroffDifference > poweroffInterval && actualState.Name != "Poweroff" {
		UpdateState(db, &workplace, workplace.PoweroffPortDateTime.Time, "Poweroff")
		actualState.Name = "Poweroff"

	}
}

func UpdateState(db *gorm.DB, workplace **zapsi_database.Workplace, stateChangeTime time.Time, stateName string) {
	LogInfo((*workplace).Name, "Changing state ==> "+stateName+" at "+stateChangeTime.String())
	var workplaceMode zapsi_database.WorkplaceMode
	db.Where("Name = ?", "Production").Find(&workplaceMode)
	var state zapsi_database.State
	db.Where("name=?", stateName).Last(&state)
	(*workplace).ActualStateDateTime = stateChangeTime
	(*workplace).ActualStateId = state.ID
	(*workplace).ActualWorkplaceModeId = workplaceMode.ID
	db.Save(&workplace)
	var lastWorkplaceState zapsi_database.WorkplaceState
	db.Where("workplace_id=?", (*workplace).ID).Last(&lastWorkplaceState)
	if lastWorkplaceState.Id != 0 {
		interval := stateChangeTime.Sub(lastWorkplaceState.DateTimeStart)
		lastWorkplaceState.DateTimeEnd = sql.NullTime{Time: stateChangeTime}
		lastWorkplaceState.Interval = uint(interval.Seconds())
		db.Save(&lastWorkplaceState)
	}
	newWorkplaceState := zapsi_database.WorkplaceState{WorkplaceId: (*workplace).ID, StateId: state.ID, DateTimeStart: stateChangeTime}
	db.Save(&newWorkplaceState)
}
