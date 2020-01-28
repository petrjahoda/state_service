package main

import (
	"github.com/jinzhu/gorm"
	"sort"
	"strconv"
	"time"
)

func (workplace Workplace) Sleep(start time.Time) {
	if time.Since(start) < (downloadInSeconds * time.Second) {
		sleepTime := downloadInSeconds*time.Second - time.Since(start)
		LogInfo(workplace.Name, "Sleeping for "+sleepTime.String())
		time.Sleep(sleepTime)
	}
}

func (workplace Workplace) AddData() []IntermediateData {
	connectionString, dialect := CheckDatabaseType()
	db, err := gorm.Open(dialect, connectionString)
	if err != nil {
		LogError(workplace.Name, "Problem opening "+DatabaseName+" database: "+err.Error())
		return nil
	}
	defer db.Close()
	var workplaceState WorkplaceState
	db.Where("workplace_id=?", workplace.ID).Last(&workplaceState)
	poweroffRecords := workplace.DownloadPoweroffRecords(db, workplaceState)
	productionRecords := workplace.DownloadProductionRecords(db, workplaceState)
	intermediateData := workplace.CreateIntermediateData(poweroffRecords, productionRecords)
	return intermediateData
}

func (workplace Workplace) CreateIntermediateData(poweroffRecords []DeviceAnalogRecord, productionRecords []DeviceDigitalRecord) []IntermediateData {
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

func (workplace Workplace) DownloadProductionRecords(db *gorm.DB, workplaceState WorkplaceState) []DeviceDigitalRecord {
	var production State
	db.Where("name=?", "Production").Find(&production)
	var productionPort WorkplacePort
	db.Where("workplace_id=?", workplace.ID).Where("state_id=?", production.ID).First(&productionPort)
	var productionRecords []DeviceDigitalRecord
	db.Where("device_port_id=?", productionPort.DevicePortId).Where("date_time > ?", workplaceState.DateTimeStart).Find(&productionRecords)
	return productionRecords
}

func (workplace Workplace) DownloadPoweroffRecords(db *gorm.DB, workplaceState WorkplaceState) []DeviceAnalogRecord {
	var poweroff State
	db.Where("name=?", "Poweroff").Find(&poweroff)
	var poweroffPort WorkplacePort
	db.Where("workplace_id=?", workplace.ID).Where("state_id=?", poweroff.ID).First(&poweroffPort)
	var poweroffRecords []DeviceAnalogRecord
	db.Where("device_port_id=?", poweroffPort.DevicePortId).Where("date_time > ?", workplaceState.DateTimeStart).Find(&poweroffRecords)
	return poweroffRecords
}

func (workplace Workplace) GetLatestWorkplaceStateId(db *gorm.DB) int {
	var workplaceState WorkplaceState
	db.Where("workplace_id=?", workplace.ID).Last(&workplaceState)
	return int(workplaceState.StateId)
}

func (workplace Workplace) GetActualState(latestworkplaceStateId int, db *gorm.DB) State {
	var actualState State
	db.Where("id=?", latestworkplaceStateId).Find(&actualState)
	return actualState
}

func ProcessData(workplace *Workplace, data []IntermediateData) {
	connectionString, dialect := CheckDatabaseType()
	db, err := gorm.Open(dialect, connectionString)
	if err != nil {
		LogError(workplace.Name, "Problem opening "+DatabaseName+" database: "+err.Error())
		return
	}
	defer db.Close()
	var actualWorkplaceMode WorkplaceMode
	db.Where("id=?", workplace.ActualWorkplaceModeId).Find(&actualWorkplaceMode)
	poweroffInterval := actualWorkplaceMode.PoweroffInterval
	downtimeInterval := actualWorkplaceMode.DowntimeInterval
	var actualState State
	latestworkplaceStateId := workplace.GetLatestWorkplaceStateId(db)
	actualState = workplace.GetActualState(latestworkplaceStateId, db)
	for _, actualData := range data {
		if actualData.Type == poweroff {
			workplace.PoweroffPortDateTime = actualData.DateTime
		} else if actualData.Type == production {
			workplace.PoweroffPortDateTime = actualData.DateTime
			workplace.ProductionPortDateTime = actualData.DateTime
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
				workplacePoweroffDifference := int(actualData.DateTime.Sub(workplace.PoweroffPortDateTime).Seconds())
				if workplacePoweroffDifference > poweroffInterval {
					UpdateState(db, &workplace, workplace.PoweroffPortDateTime, "Poweroff")
					actualState.Name = "Poweroff"

					if actualData.Type == production && actualData.RawData == "1" {
						UpdateState(db, &workplace, actualData.DateTime, "Production")
						actualState.Name = "Production"

						break
					}
					UpdateState(db, &workplace, actualData.DateTime, "Downtime")
					actualState.Name = "Downtime"

				} else {
					workplaceDowntimeDifference := int(actualData.DateTime.Sub(workplace.ProductionPortDateTime).Seconds())
					if workplace.ProductionPortValue == 0 && workplaceDowntimeDifference > downtimeInterval {
						UpdateState(db, &workplace, workplace.ProductionPortDateTime, "Downtime")
						actualState.Name = "Downtime"
						break
					}
				}
			}
		case "Downtime":
			{
				workplacePoweroffDifference := int(actualData.DateTime.Sub(workplace.PoweroffPortDateTime).Seconds())
				if workplacePoweroffDifference > poweroffInterval {
					UpdateState(db, &workplace, workplace.PoweroffPortDateTime, "Poweroff")
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
	workplacePoweroffDifference := int(time.Now().UTC().Sub(workplace.PoweroffPortDateTime).Seconds())
	if workplacePoweroffDifference > poweroffInterval && actualState.Name != "Poweroff" {
		UpdateState(db, &workplace, workplace.PoweroffPortDateTime, "Poweroff")
		actualState.Name = "Poweroff"

	}
}

func UpdateState(db *gorm.DB, workplace **Workplace, stateChangeTime time.Time, stateName string) {
	LogInfo((*workplace).Name, "Changing state ==> "+stateName+" at "+stateChangeTime.String())
	var state State
	db.Where("name=?", stateName).Last(&state)
	(*workplace).ActualStateDateTime = stateChangeTime
	(*workplace).ActualStateId = state.ID
	db.Save(&workplace)
	var lastWorkplaceState WorkplaceState
	db.Where("workplace_id=?", (*workplace).ID).Last(&lastWorkplaceState)
	if lastWorkplaceState.Id != 0 {
		interval := stateChangeTime.Sub(lastWorkplaceState.DateTimeStart)
		lastWorkplaceState.DateTimeEnd = stateChangeTime
		lastWorkplaceState.Interval = float32(interval.Seconds())
		db.Save(&lastWorkplaceState)
	}
	newWorkplaceState := WorkplaceState{WorkplaceId: (*workplace).ID, StateId: state.ID, DateTimeStart: stateChangeTime}
	db.Save(&newWorkplaceState)
}
