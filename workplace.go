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
	LogInfo(workplace.Name, "Adding data")
	timer := time.Now()
	connectionString, dialect := zapsi_database.CheckDatabaseType(DatabaseType, DatabaseIpAddress, DatabasePort, DatabaseLogin, DatabaseName, DatabasePassword)
	db, err := gorm.Open(dialect, connectionString)
	if err != nil {
		LogError(workplace.Name, "Problem opening "+DatabaseName+" database: "+err.Error())
		return nil
	}
	defer db.Close()
	var workplaceState zapsi_database.StateRecord
	db.Where("workplace_id=?", workplace.ID).Last(&workplaceState)
	poweroffRecords := DownloadPoweroffRecords(workplace, db, workplaceState)
	productionRecords := DownloadProductionRecords(workplace, db, workplaceState)
	intermediateData := CreateIntermediateData(workplace, poweroffRecords, productionRecords)
	LogInfo(workplace.Name, "Data added, elapsed: "+time.Since(timer).String())
	return intermediateData
}

func CreateIntermediateData(workplace zapsi_database.Workplace, poweroffRecords []zapsi_database.DevicePortAnalogRecord, productionRecords []zapsi_database.DevicePortDigitalRecord) []IntermediateData {
	LogInfo(workplace.Name, "Creating intermediate data")
	timer := time.Now()
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
	LogInfo(workplace.Name, "Intermediate data created, elapsed: "+time.Since(timer).String())
	return intermediateData
}

func DownloadProductionRecords(workplace zapsi_database.Workplace, db *gorm.DB, workplaceState zapsi_database.StateRecord) []zapsi_database.DevicePortDigitalRecord {
	LogInfo(workplace.Name, "Downloading production records")
	timer := time.Now()
	var production zapsi_database.State
	db.Where("name=?", "Production").Find(&production)
	var productionPort zapsi_database.WorkplacePort
	db.Where("workplace_id=?", workplace.ID).Where("state_id=?", production.ID).First(&productionPort)
	var productionRecords []zapsi_database.DevicePortDigitalRecord
	db.Where("device_port_id=?", productionPort.DevicePortId).Where("date_time > ?", workplaceState.DateTimeStart).Find(&productionRecords)
	LogInfo(workplace.Name, "Production records downloaded, elapsed: "+time.Since(timer).String())
	return productionRecords
}

func DownloadPoweroffRecords(workplace zapsi_database.Workplace, db *gorm.DB, workplaceState zapsi_database.StateRecord) []zapsi_database.DevicePortAnalogRecord {
	LogInfo(workplace.Name, "Downloading poweroff records")
	timer := time.Now()
	var poweroff zapsi_database.State
	db.Where("name=?", "Poweroff").Find(&poweroff)
	var poweroffPort zapsi_database.WorkplacePort
	db.Where("workplace_id=?", workplace.ID).Where("state_id=?", poweroff.ID).First(&poweroffPort)
	var poweroffRecords []zapsi_database.DevicePortAnalogRecord
	db.Where("device_port_id=?", poweroffPort.DevicePortId).Where("date_time > ?", workplaceState.DateTimeStart).Find(&poweroffRecords)
	LogInfo(workplace.Name, "Poweroff records downloaded, elapsed: "+time.Since(timer).String())
	return poweroffRecords
}

//
func GetLatestWorkplaceStateId(db *gorm.DB, workplace *zapsi_database.Workplace) int {
	var workplaceState zapsi_database.StateRecord
	db.Where("workplace_id=?", workplace.ID).Last(&workplaceState)
	return workplaceState.StateId
}

func GetActualState(db *gorm.DB, latestworkplaceStateId int) zapsi_database.State {
	var actualState zapsi_database.State
	db.Where("id=?", latestworkplaceStateId).Find(&actualState)
	return actualState
}

func ProcessData(workplace *zapsi_database.Workplace, data []IntermediateData) {
	LogInfo(workplace.Name, "Processing data")
	timer := time.Now()
	connectionString, dialect := zapsi_database.CheckDatabaseType(DatabaseType, DatabaseIpAddress, DatabasePort, DatabaseLogin, DatabaseName, DatabasePassword)
	db, err := gorm.Open(dialect, connectionString)
	if err != nil {
		LogError(workplace.Name, "Problem opening "+DatabaseName+" database: "+err.Error())
		return
	}
	defer db.Close()
	actualWorkplaceMode := GetActualWorkplaceMode(db, workplace)
	latestworkplaceStateId := GetLatestWorkplaceStateId(db, workplace)
	actualWorkplaceState := GetActualState(db, latestworkplaceStateId)
	poweroffInterval := actualWorkplaceMode.PoweroffDuration
	downtimeInterval := actualWorkplaceMode.DowntimeDuration
	for _, actualData := range data {
		if actualData.Type == poweroff {
			workplace.PoweroffPortDateTime = sql.NullTime{Time: actualData.DateTime, Valid: true}
		} else if actualData.Type == production {
			workplace.PoweroffPortDateTime = sql.NullTime{Time: actualData.DateTime, Valid: true}
			workplace.ProductionPortDateTime = sql.NullTime{Time: actualData.DateTime, Valid: true}
		}
		switch actualWorkplaceState.Name {
		case "Poweroff":
			{
				if actualData.Type == production && actualData.RawData == "1" {
					UpdateState(db, &workplace, actualData.DateTime, "Production")
					actualWorkplaceState.Name = "Production"
					break
				}
				if actualData.Type == poweroff {
					UpdateState(db, &workplace, actualData.DateTime, "Downtime")
					actualWorkplaceState.Name = "Downtime"
					break
				}
			}
		case "Production":
			{
				workplacePoweroffDifference := actualData.DateTime.Sub(workplace.PoweroffPortDateTime.Time)
				if workplacePoweroffDifference > poweroffInterval {
					UpdateState(db, &workplace, workplace.PoweroffPortDateTime.Time, "Poweroff")
					actualWorkplaceState.Name = "Poweroff"
					if actualData.Type == production && actualData.RawData == "1" {
						UpdateState(db, &workplace, actualData.DateTime, "Production")
						actualWorkplaceState.Name = "Production"
						break
					}
					UpdateState(db, &workplace, actualData.DateTime, "Downtime")
					actualWorkplaceState.Name = "Downtime"

				} else {
					workplaceDowntimeDifference := actualData.DateTime.Sub(workplace.ProductionPortDateTime.Time)
					if workplace.ProductionPortValue.Int32 == 0 && workplaceDowntimeDifference > downtimeInterval {
						UpdateState(db, &workplace, workplace.ProductionPortDateTime.Time, "Downtime")
						actualWorkplaceState.Name = "Downtime"
						break
					}
				}
			}
		case "Downtime":
			{
				workplacePoweroffDifference := actualData.DateTime.Sub(workplace.PoweroffPortDateTime.Time)
				if workplacePoweroffDifference > poweroffInterval {
					UpdateState(db, &workplace, workplace.PoweroffPortDateTime.Time, "Poweroff")
					actualWorkplaceState.Name = "Poweroff"
					if actualData.Type == production && actualData.RawData == "1" {
						UpdateState(db, &workplace, actualData.DateTime, "Production")
						actualWorkplaceState.Name = "Production"
						break
					}
					UpdateState(db, &workplace, actualData.DateTime, "Downtime")
					actualWorkplaceState.Name = "Downtime"

					break
				} else {
					if actualData.Type == production && actualData.RawData == "1" {
						UpdateState(db, &workplace, actualData.DateTime, "Production")
						actualWorkplaceState.Name = "Production"
						break
					}
				}
			}
		default:
			{
				if actualData.Type == production && actualData.RawData == "1" {
					UpdateState(db, &workplace, actualData.DateTime, "Production")
					actualWorkplaceState.Name = "Production"
					break
				}
				if actualData.Type == poweroff {
					UpdateState(db, &workplace, actualData.DateTime, "Downtime")
					actualWorkplaceState.Name = "Downtime"
					break
				}
			}
		}
	}
	workplacePoweroffDifference := time.Now().UTC().Sub(workplace.PoweroffPortDateTime.Time)
	if workplacePoweroffDifference > poweroffInterval && actualWorkplaceState.Name != "Poweroff" {
		UpdateState(db, &workplace, workplace.PoweroffPortDateTime.Time, "Poweroff")
		actualWorkplaceState.Name = "Poweroff"

	}
	LogInfo(workplace.Name, "Data processed, elapsed: "+time.Since(timer).String())
}

func GetActualWorkplaceMode(db *gorm.DB, workplace *zapsi_database.Workplace) zapsi_database.WorkplaceMode {
	var actualWorkplaceMode zapsi_database.WorkplaceMode
	db.Where("id=?", workplace.ActualWorkplaceModeId).Find(&actualWorkplaceMode)
	return actualWorkplaceMode
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
	var lastWorkplaceState zapsi_database.StateRecord
	db.Where("workplace_id=?", (*workplace).ID).Last(&lastWorkplaceState)
	if lastWorkplaceState.ID != 0 {
		interval := stateChangeTime.Sub(lastWorkplaceState.DateTimeStart)
		lastWorkplaceState.DateTimeEnd = sql.NullTime{Time: stateChangeTime, Valid: true}
		lastWorkplaceState.Duration = interval
		db.Save(&lastWorkplaceState)
	}
	newWorkplaceState := zapsi_database.StateRecord{WorkplaceId: (*workplace).ID, StateId: state.ID, DateTimeStart: stateChangeTime}
	db.Save(&newWorkplaceState)
}
