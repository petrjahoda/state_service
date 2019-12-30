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
	offlineRecords := workplace.DownloadOfflineRecords(db, workplaceState)
	productionRecords := workplace.DownloadProductionRecords(db, workplaceState)
	intermediateData := workplace.CreateIntermediateData(offlineRecords, productionRecords)
	return intermediateData
}

func (workplace Workplace) CreateIntermediateData(offlineRecords []DeviceAnalogRecord, productionRecords []DeviceDigitalRecord) []IntermediateData {
	var intermediateData []IntermediateData
	for _, offlineRecord := range offlineRecords {
		rawData := strconv.FormatFloat(float64(offlineRecord.Data), 'g', 15, 64)
		data := IntermediateData{DateTime: offlineRecord.DateTime, RawData: rawData, Type: offline}
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

func (workplace Workplace) DownloadOfflineRecords(db *gorm.DB, workplaceState WorkplaceState) []DeviceAnalogRecord {
	var offline State
	db.Where("name=?", "Offline").Find(&offline)
	var offlinePort WorkplacePort
	db.Where("workplace_id=?", workplace.ID).Where("state_id=?", offline.ID).First(&offlinePort)
	var offlineRecords []DeviceAnalogRecord
	db.Where("device_port_id=?", offlinePort.DevicePortId).Where("date_time > ?", workplaceState.DateTimeStart).Find(&offlineRecords)
	return offlineRecords
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
	offlineInterval := actualWorkplaceMode.OfflineInterval
	downtimeInterval := actualWorkplaceMode.DownTimeInterval
	for _, actualData := range data {
		if actualData.Type == offline {
			workplace.OfflinePortDateTime = actualData.DateTime
		} else if actualData.Type == production {
			workplace.ProductionPortDateTime = actualData.DateTime
		}
		LogInfo(workplace.Name, "Data: "+actualData.DateTime.UTC().String())
		var actualState State
		db.Where("id=?", workplace.ActualStateId).Find(&actualState)
		LogInfo(workplace.Name, "Actual workplace state: "+actualState.Name)
		switch actualState.Name {
		case "Offline":
			{
				if actualData.Type == production && actualData.RawData == "1" {
					UpdateState(db, &workplace, actualData.DateTime, "Production")
					break
				}
				if actualData.Type == offline {
					UpdateState(db, &workplace, actualData.DateTime, "Downtime")
					break
				}
			}
		case "Production":
			{
				workplaceOfflineDifference := int(actualData.DateTime.Sub(workplace.OfflinePortDateTime).Seconds())
				if workplaceOfflineDifference > offlineInterval {
					UpdateState(db, &workplace, workplace.OfflinePortDateTime, "Offline")
					if actualData.Type == production && actualData.RawData == "1" {
						UpdateState(db, &workplace, actualData.DateTime, "Production")
						break
					}
					UpdateState(db, &workplace, actualData.DateTime, "Downtime")
				} else {
					workplaceDowntimeDifference := int(actualData.DateTime.Sub(workplace.ProductionPortDateTime).Seconds())
					if workplace.ProductionPortValue == 0 && workplaceDowntimeDifference > downtimeInterval {
						UpdateState(db, &workplace, workplace.ProductionPortDateTime, "Downtime")
						break
					}
				}
			}
		case "Downtime":
			{
				workplaceOfflineDifference := int(actualData.DateTime.Sub(workplace.OfflinePortDateTime).Seconds())
				if workplaceOfflineDifference > offlineInterval {
					UpdateState(db, &workplace, workplace.OfflinePortDateTime, "Offline")
					if actualData.Type == production && actualData.RawData == "1" {
						UpdateState(db, &workplace, actualData.DateTime, "Production")
						break
					}
					UpdateState(db, &workplace, actualData.DateTime, "Downtime")
					break
				} else {
					if actualData.Type == production && actualData.RawData == "1" {
						UpdateState(db, &workplace, actualData.DateTime, "Production")
						break
					}
				}
			}
		default:
			{
				if actualData.Type == production && actualData.RawData == "1" {
					UpdateState(db, &workplace, actualData.DateTime, "Production")
					break
				}
				if actualData.Type == offline {
					UpdateState(db, &workplace, actualData.DateTime, "Downtime")
					break
				}
			}
		}
	}
	workplaceOfflineDifference := int(time.Now().UTC().Sub(workplace.OfflinePortDateTime).Seconds())
	var actualState State
	db.Where("id=?", workplace.ActualStateId).Find(&actualState)
	if workplaceOfflineDifference > offlineInterval && actualState.Name != "Offline" {
		UpdateState(db, &workplace, workplace.OfflinePortDateTime, "Offline")
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
	LogDebug((*workplace).Name, "Last workplace state ID: "+strconv.Itoa(int(lastWorkplaceState.Id)))
	if lastWorkplaceState.Id != 0 {
		interval := stateChangeTime.Sub(lastWorkplaceState.DateTimeStart)
		lastWorkplaceState.DateTimeEnd = stateChangeTime
		lastWorkplaceState.Interval = float32(interval.Seconds())
		db.Save(&lastWorkplaceState)
	}
	newWorkplaceState := WorkplaceState{WorkplaceId: (*workplace).ID, StateId: state.ID, DateTimeStart: stateChangeTime}
	db.Save(&newWorkplaceState)
}
