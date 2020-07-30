package main

import (
	"database/sql"
	"github.com/petrjahoda/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"sort"
	"strconv"
	"time"
)

func AddData(workplace database.Workplace, analogDateTime time.Time, digitalDateTime time.Time) []IntermediateData {
	LogInfo(workplace.Name, "Adding data")
	timer := time.Now()
	db, err := gorm.Open(postgres.Open(config), &gorm.Config{})
	if err != nil {
		LogError(workplace.Name, "Problem opening database: "+err.Error())
		return nil
	}
	sqlDB, err := db.DB()
	defer sqlDB.Close()
	workplaceState := DownloadLatestWorkplaceState(db, workplace)
	poweroffRecords := DownloadPoweroffRecords(workplace, db, workplaceState, analogDateTime)
	productionRecords := DownloadProductionRecords(workplace, db, workplaceState, digitalDateTime)
	intermediateData := CreateIntermediateData(workplace, poweroffRecords, productionRecords)
	LogInfo(workplace.Name, "Data added, elapsed: "+time.Since(timer).String())
	return intermediateData
}

func DownloadLatestWorkplaceState(db *gorm.DB, workplace database.Workplace) database.StateRecord {
	LogInfo(workplace.Name, "Downloading latest workplace state")
	timer := time.Now()
	var workplaceState database.StateRecord
	db.Where("workplace_id=?", workplace.ID).Last(&workplaceState)
	LogInfo(workplace.Name, "Latest workplace state downloaded, elapsed: "+time.Since(timer).String())
	return workplaceState
}

func CreateIntermediateData(workplace database.Workplace, poweroffRecords []database.DevicePortAnalogRecord, productionRecords []database.DevicePortDigitalRecord) []IntermediateData {
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

func DownloadProductionRecords(workplace database.Workplace, db *gorm.DB, workplaceState database.StateRecord, digitalDateTime time.Time) []database.DevicePortDigitalRecord {
	LogInfo(workplace.Name, "Downloading production records")
	timer := time.Now()
	var production database.State
	db.Where("name=?", "Production").Find(&production)
	var productionPort database.WorkplacePort
	db.Where("workplace_id=?", workplace.ID).Where("state_id=?", production.ID).First(&productionPort)
	var productionRecords []database.DevicePortDigitalRecord
	if workplaceState.DateTimeStart.Before(digitalDateTime) {
		db.Where("device_port_id=?", productionPort.DevicePortID).Where("date_time > ?", digitalDateTime).Find(&productionRecords)
	} else {
		db.Where("device_port_id=?", productionPort.DevicePortID).Where("date_time > ?", workplaceState.DateTimeStart).Find(&productionRecords)
	}
	LogInfo(workplace.Name, "Production records downloaded: "+strconv.Itoa(len(productionRecords))+" elapsed: "+time.Since(timer).String())
	return productionRecords
}

func DownloadPoweroffRecords(workplace database.Workplace, db *gorm.DB, workplaceState database.StateRecord, analogDateTime time.Time) []database.DevicePortAnalogRecord {
	LogInfo(workplace.Name, "Downloading poweroff records")
	timer := time.Now()
	var poweroff database.State
	db.Where("name=?", "Poweroff").Find(&poweroff)
	var poweroffPort database.WorkplacePort
	db.Where("workplace_id=?", workplace.ID).Where("state_id=?", poweroff.ID).First(&poweroffPort)
	var poweroffRecords []database.DevicePortAnalogRecord
	if workplaceState.DateTimeStart.Before(analogDateTime) {
		db.Where("device_port_id=?", poweroffPort.DevicePortID).Where("date_time > ?", analogDateTime).Find(&poweroffRecords)
	} else {
		db.Where("device_port_id=?", poweroffPort.DevicePortID).Where("date_time > ?", workplaceState.DateTimeStart).Find(&poweroffRecords)
	}
	LogInfo(workplace.Name, "Poweroff records downloaded, count: "+strconv.Itoa(len(poweroffRecords))+", elapsed: "+time.Since(timer).String())
	return poweroffRecords
}

//
func GetLatestWorkplaceStateId(db *gorm.DB, workplace *database.Workplace) int {
	var workplaceState database.StateRecord
	db.Where("workplace_id=?", workplace.ID).Last(&workplaceState)
	return workplaceState.StateID
}

func GetActualState(db *gorm.DB, latestworkplaceStateId int) database.State {
	var actualState database.State
	db.Where("id=?", latestworkplaceStateId).Find(&actualState)
	return actualState
}

func ProcessData(workplace *database.Workplace, data []IntermediateData, analogDateTime time.Time, digitalDateTime time.Time) (time.Time, time.Time) {
	LogInfo(workplace.Name, "Processing data")
	timer := time.Now()
	db, err := gorm.Open(postgres.Open(config), &gorm.Config{})
	if err != nil {
		LogError(workplace.Name, "Problem opening database: "+err.Error())
		return analogDateTime, digitalDateTime
	}
	sqlDB, err := db.DB()
	defer sqlDB.Close()
	actualWorkplaceMode := GetActualWorkplaceMode(db, workplace)
	latestworkplaceStateId := GetLatestWorkplaceStateId(db, workplace)
	actualWorkplaceState := GetActualState(db, latestworkplaceStateId)
	poweroffInterval := actualWorkplaceMode.PoweroffDuration
	downtimeInterval := actualWorkplaceMode.DowntimeDuration
	for _, actualData := range data {
		if actualData.Type == poweroff {
			workplace.PowerOffPortDateTime = sql.NullTime{Time: actualData.DateTime, Valid: true}
			analogDateTime = actualData.DateTime
		} else if actualData.Type == production {
			workplace.PowerOffPortDateTime = sql.NullTime{Time: actualData.DateTime, Valid: true}
			workplace.ProductionPortDateTime = sql.NullTime{Time: actualData.DateTime, Valid: true}
			digitalDateTime = actualData.DateTime
		}
		switch actualWorkplaceState.Name {
		case "Poweroff":
			{
				if actualData.Type == production && actualData.RawData == "1" {
					InsertStateIntoDatabase(db, &workplace, actualData.DateTime, "Production")
					actualWorkplaceState.Name = "Production"
					break
				}
				if actualData.Type == poweroff {
					InsertStateIntoDatabase(db, &workplace, actualData.DateTime, "Downtime")
					actualWorkplaceState.Name = "Downtime"
					break
				}
			}
		case "Production":
			{
				workplacePoweroffDifference := actualData.DateTime.Sub(workplace.PowerOffPortDateTime.Time)
				if workplacePoweroffDifference > poweroffInterval {
					InsertStateIntoDatabase(db, &workplace, workplace.PowerOffPortDateTime.Time, "Poweroff")
					actualWorkplaceState.Name = "Poweroff"
					if actualData.Type == production && actualData.RawData == "1" {
						InsertStateIntoDatabase(db, &workplace, actualData.DateTime, "Production")
						actualWorkplaceState.Name = "Production"
						break
					}
					InsertStateIntoDatabase(db, &workplace, actualData.DateTime, "Downtime")
					actualWorkplaceState.Name = "Downtime"

				} else {
					workplaceDowntimeDifference := actualData.DateTime.Sub(workplace.ProductionPortDateTime.Time)
					if workplace.ProductionPortValue.Int32 == 0 && workplaceDowntimeDifference > downtimeInterval {
						InsertStateIntoDatabase(db, &workplace, workplace.ProductionPortDateTime.Time, "Downtime")
						actualWorkplaceState.Name = "Downtime"
						break
					}
				}
			}
		case "Downtime":
			{
				workplacePoweroffDifference := actualData.DateTime.Sub(workplace.PowerOffPortDateTime.Time)
				if workplacePoweroffDifference > poweroffInterval {
					InsertStateIntoDatabase(db, &workplace, workplace.PowerOffPortDateTime.Time, "Poweroff")
					actualWorkplaceState.Name = "Poweroff"
					if actualData.Type == production && actualData.RawData == "1" {
						InsertStateIntoDatabase(db, &workplace, actualData.DateTime, "Production")
						actualWorkplaceState.Name = "Production"
						break
					}
					InsertStateIntoDatabase(db, &workplace, actualData.DateTime, "Downtime")
					actualWorkplaceState.Name = "Downtime"

					break
				} else {
					if actualData.Type == production && actualData.RawData == "1" {
						InsertStateIntoDatabase(db, &workplace, actualData.DateTime, "Production")
						actualWorkplaceState.Name = "Production"
						break
					}
				}
			}
		default:
			{
				if actualData.Type == production && actualData.RawData == "1" {
					InsertStateIntoDatabase(db, &workplace, actualData.DateTime, "Production")
					actualWorkplaceState.Name = "Production"
					break
				}
				if actualData.Type == poweroff {
					InsertStateIntoDatabase(db, &workplace, actualData.DateTime, "Downtime")
					actualWorkplaceState.Name = "Downtime"
					break
				}
			}
		}
	}
	workplacePoweroffDifference := time.Now().UTC().Sub(workplace.PowerOffPortDateTime.Time)
	if workplacePoweroffDifference > poweroffInterval && actualWorkplaceState.Name != "Poweroff" {
		InsertStateIntoDatabase(db, &workplace, workplace.PowerOffPortDateTime.Time, "Poweroff")
		actualWorkplaceState.Name = "Poweroff"

	}
	LogInfo(workplace.Name, "Data processed, elapsed: "+time.Since(timer).String())
	return analogDateTime, digitalDateTime
}

func GetActualWorkplaceMode(db *gorm.DB, workplace *database.Workplace) database.WorkplaceMode {
	var actualWorkplaceMode database.WorkplaceMode
	db.Where("id=?", workplace.WorkplaceModeID).Find(&actualWorkplaceMode)
	return actualWorkplaceMode
}

func InsertStateIntoDatabase(db *gorm.DB, workplace **database.Workplace, stateChangeTime time.Time, stateName string) {
	LogInfo((*workplace).Name, "Changing state ==> "+stateName+" at "+stateChangeTime.String())
	var workplaceMode database.WorkplaceMode
	db.Where("Name = ?", "Production").Find(&workplaceMode)
	var state database.State
	db.Where("name=?", stateName).Last(&state)
	(*workplace).WorkplaceModeID = int(workplaceMode.ID)
	db.Save(&workplace)
	newWorkplaceState := database.StateRecord{WorkplaceID: int((*workplace).ID), StateID: int(state.ID), DateTimeStart: stateChangeTime}
	db.Save(&newWorkplaceState)
}
