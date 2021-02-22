package main

import (
	"database/sql"
	"github.com/TwinProduction/go-color"
	"github.com/petrjahoda/database"
	"gorm.io/gorm"
	"sort"
	"strconv"
	"time"
)

func readDataForProcessing(workplace database.Workplace, db *gorm.DB, analogDateTime time.Time, digitalDateTime time.Time) []IntermediateData {
	logInfo(workplace.Name, "Reading data for processing")
	timer := time.Now()
	workplaceState := readLatestWorkplaceState(db, workplace)
	poweroffRecords := readPoweroffRecords(workplace, db, workplaceState, analogDateTime)
	productionRecords := readProductionRecords(workplace, db, workplaceState, digitalDateTime)
	intermediateData := createIntermediateData(workplace, poweroffRecords, productionRecords)
	logInfo(workplace.Name, "Data for processing read in "+time.Since(timer).String())
	return intermediateData
}

func readLatestWorkplaceState(db *gorm.DB, workplace database.Workplace) database.StateRecord {
	logInfo(workplace.Name, "Reading latest workplace state")
	timer := time.Now()
	var workplaceState database.StateRecord
	db.Where("workplace_id=?", workplace.ID).Last(&workplaceState)
	logInfo(workplace.Name, "Latest workplace state  read in "+time.Since(timer).String())
	return workplaceState
}

func createIntermediateData(workplace database.Workplace, poweroffRecords []database.DevicePortAnalogRecord, productionRecords []database.DevicePortDigitalRecord) []IntermediateData {
	logInfo(workplace.Name, "Creating intermediate data")
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
	logInfo(workplace.Name, "Intermediate data created in "+time.Since(timer).String())
	return intermediateData
}

func readProductionRecords(workplace database.Workplace, db *gorm.DB, workplaceState database.StateRecord, digitalDateTime time.Time) []database.DevicePortDigitalRecord {
	logInfo(workplace.Name, "Reading production records")
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
	logInfo(workplace.Name, "Production records count: "+strconv.Itoa(len(productionRecords))+" read in "+time.Since(timer).String())
	return productionRecords
}

func readPoweroffRecords(workplace database.Workplace, db *gorm.DB, workplaceState database.StateRecord, analogDateTime time.Time) []database.DevicePortAnalogRecord {
	logInfo(workplace.Name, "Reading poweroff records")
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
	logInfo(workplace.Name, "Poweroff records count: "+strconv.Itoa(len(poweroffRecords))+", read in "+time.Since(timer).String())
	return poweroffRecords
}

func readLatestWorkplaceStateId(db *gorm.DB, workplace *database.Workplace) int {
	var workplaceState database.StateRecord
	db.Where("workplace_id=?", workplace.ID).Last(&workplaceState)
	return workplaceState.StateID
}

func readActualState(db *gorm.DB, latestworkplaceStateId int) database.State {
	var actualState database.State
	db.Where("id=?", latestworkplaceStateId).Find(&actualState)
	return actualState
}

func processData(workplace *database.Workplace, db *gorm.DB, data []IntermediateData, analogDateTime time.Time, digitalDateTime time.Time) (time.Time, time.Time) {
	logInfo(workplace.Name, "Processing data started")
	timer := time.Now()
	actualWorkplaceMode := readActualWorkplaceMode(db, workplace)
	latestworkplaceStateId := readLatestWorkplaceStateId(db, workplace)
	actualWorkplaceState := readActualState(db, latestworkplaceStateId)
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
					createNewStateRecordIntoDatabase(db, &workplace, actualData.DateTime, "Production")
					actualWorkplaceState.Name = "Production"
					break
				}
				if actualData.Type == poweroff {
					createNewStateRecordIntoDatabase(db, &workplace, actualData.DateTime, "Downtime")
					actualWorkplaceState.Name = "Downtime"
					break
				}
			}
		case "Production":
			{
				workplacePoweroffDifference := actualData.DateTime.Sub(workplace.PowerOffPortDateTime.Time)
				if workplacePoweroffDifference > poweroffInterval {
					createNewStateRecordIntoDatabase(db, &workplace, workplace.PowerOffPortDateTime.Time, "Poweroff")
					actualWorkplaceState.Name = "Poweroff"
					if actualData.Type == production && actualData.RawData == "1" {
						createNewStateRecordIntoDatabase(db, &workplace, actualData.DateTime, "Production")
						actualWorkplaceState.Name = "Production"
						break
					}
					createNewStateRecordIntoDatabase(db, &workplace, actualData.DateTime, "Downtime")
					actualWorkplaceState.Name = "Downtime"

				} else {
					workplaceDowntimeDifference := actualData.DateTime.Sub(workplace.ProductionPortDateTime.Time)
					if workplace.ProductionPortValue.Int32 == 0 && workplaceDowntimeDifference > downtimeInterval {
						createNewStateRecordIntoDatabase(db, &workplace, workplace.ProductionPortDateTime.Time, "Downtime")
						actualWorkplaceState.Name = "Downtime"
						break
					}
				}
			}
		case "Downtime":
			{
				workplacePoweroffDifference := actualData.DateTime.Sub(workplace.PowerOffPortDateTime.Time)
				if workplacePoweroffDifference > poweroffInterval {
					createNewStateRecordIntoDatabase(db, &workplace, workplace.PowerOffPortDateTime.Time, "Poweroff")
					actualWorkplaceState.Name = "Poweroff"
					if actualData.Type == production && actualData.RawData == "1" {
						createNewStateRecordIntoDatabase(db, &workplace, actualData.DateTime, "Production")
						actualWorkplaceState.Name = "Production"
						break
					}
					createNewStateRecordIntoDatabase(db, &workplace, actualData.DateTime, "Downtime")
					actualWorkplaceState.Name = "Downtime"

					break
				} else {
					if actualData.Type == production && actualData.RawData == "1" {
						createNewStateRecordIntoDatabase(db, &workplace, actualData.DateTime, "Production")
						actualWorkplaceState.Name = "Production"
						break
					}
				}
			}
		default:
			{
				if actualData.Type == production && actualData.RawData == "1" {
					createNewStateRecordIntoDatabase(db, &workplace, actualData.DateTime, "Production")
					actualWorkplaceState.Name = "Production"
					break
				}
				if actualData.Type == poweroff {
					createNewStateRecordIntoDatabase(db, &workplace, actualData.DateTime, "Downtime")
					actualWorkplaceState.Name = "Downtime"
					break
				}
			}
		}
	}
	workplacePoweroffDifference := time.Now().UTC().Sub(workplace.PowerOffPortDateTime.Time)
	if workplacePoweroffDifference > poweroffInterval && actualWorkplaceState.Name != "Poweroff" {
		createNewStateRecordIntoDatabase(db, &workplace, workplace.PowerOffPortDateTime.Time, "Poweroff")
		actualWorkplaceState.Name = "Poweroff"

	}
	logInfo(workplace.Name, "Processing data ended in "+time.Since(timer).String())
	return analogDateTime, digitalDateTime
}

func readActualWorkplaceMode(db *gorm.DB, workplace *database.Workplace) database.WorkplaceMode {
	var actualWorkplaceMode database.WorkplaceMode
	db.Where("id=?", workplace.WorkplaceModeID).Find(&actualWorkplaceMode)
	return actualWorkplaceMode
}

func createNewStateRecordIntoDatabase(db *gorm.DB, workplace **database.Workplace, stateChangeTime time.Time, stateName string) {
	logInfo((*workplace).Name, "Creating new state record")
	timer := time.Now()
	var stateNameColored string
	if stateName == "Poweroff" {
		stateNameColored = color.Ize(color.Red, stateName)
	} else if stateName == "Downtime" {
		stateNameColored = color.Ize(color.Yellow, stateName)
	} else {
		stateNameColored = color.Ize(color.White, stateName)
	}
	logInfo((*workplace).Name, "Changing state to "+stateNameColored)
	var workplaceMode database.WorkplaceMode
	db.Where("Name = ?", "Production").Find(&workplaceMode)
	var state database.State
	db.Where("name=?", stateName).Last(&state)

	var actualWorkplace database.Workplace
	db.Where("id = ?", (*workplace).ID).Find(&actualWorkplace)
	(*workplace).WorkplaceModeID = int(workplaceMode.ID)
	(*workplace).Code = actualWorkplace.Code
	(*workplace).Name = actualWorkplace.Name
	(*workplace).WorkplaceSectionID = actualWorkplace.WorkplaceSectionID
	db.Save(&workplace)
	newWorkplaceState := database.StateRecord{WorkplaceID: int((*workplace).ID), StateID: int(state.ID), DateTimeStart: stateChangeTime}
	db.Save(&newWorkplaceState)
	logInfo((*workplace).Name, "New state record created in "+time.Since(timer).String())
}
