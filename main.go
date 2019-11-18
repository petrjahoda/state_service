package main

import (
	"github.com/jinzhu/gorm"
	"strconv"
	"sync"
	"time"
)

const version = "2019.4.2.3"
const deleteLogsAfter = 240 * time.Hour
const downloadInSeconds = 10

var (
	activeWorkplaces  []Workplace
	runningWorkplaces []Workplace
	workplaceSync     sync.Mutex
)

func main() {
	LogDirectoryFileCheck("MAIN")
	LogInfo("MAIN", "Program version "+version+" started")
	CreateConfigIfNotExists()
	LoadSettingsFromConfigFile()
	LogDebug("MAIN", "Using ["+DatabaseType+"] on "+DatabaseIpAddress+":"+DatabasePort+" with database "+DatabaseName)
	SendMail("Program started", "State Service version "+version+" started")
	for {
		start := time.Now()
		LogInfo("MAIN", "Program running")
		CheckDatabase()
		CheckTables()
		UpdateActiveWorkplaces("MAIN")
		DeleteOldLogFiles()
		LogInfo("MAIN", "Active workplaces: "+strconv.Itoa(len(activeWorkplaces))+", running workplaces: "+strconv.Itoa(len(runningWorkplaces)))
		//AddTestWorkplace("MAIN", "CNC1", "192.168.0.1")
		//AddTestWorkplace("MAIN", "CNC2", "192.168.0.2")
		//AddTestWorkplace("MAIN", "CNC3", "192.168.0.3")
		for _, activeWorkplace := range activeWorkplaces {
			activeWorkplaceIsRunning := CheckWorkplace(activeWorkplace)
			if !activeWorkplaceIsRunning {
				go RunWorkplace(activeWorkplace)
			}
		}
		if time.Since(start) < (downloadInSeconds * time.Second) {
			sleepTime := downloadInSeconds*time.Second - time.Since(start)
			LogInfo("MAIN", "Sleeping for "+sleepTime.String())
			time.Sleep(sleepTime)
		}
	}
}

func AddTestWorkplace(reference string, workplaceName string, ipAddress string) {
	connectionString, dialect := CheckDatabaseType()
	db, err := gorm.Open(dialect, connectionString)
	if err != nil {
		LogError(reference, "Problem opening "+DatabaseName+" database: "+err.Error())
		return
	}
	defer db.Close()
	var deviceType DeviceType
	db.Where("name=?", "Zapsi").Find(&deviceType)
	newDevice := Device{Name: workplaceName, DeviceType: deviceType.ID, IpAddress: ipAddress, TypeName: "Zapsi", Activated: true}
	db.Create(&newDevice)
	var device Device
	db.Where("name=?", workplaceName).Find(&device)
	deviceDigitalPort := DevicePort{Name: "Production", Unit: "ks", PortNumber: 1, DevicePortTypeId: 1, DeviceId: device.ID}
	deviceAnalogPort := DevicePort{Name: "Amperage", Unit: "A", PortNumber: 3, DevicePortTypeId: 2, DeviceId: device.ID}
	db.Create(&deviceDigitalPort)
	db.Create(&deviceAnalogPort)
	var section WorkplaceSection
	db.Where("name=?", "Machines").Find(&section)
	var state State
	db.Where("name=?", "Offline").Find(&state)
	var mode WorkplaceMode
	db.Where("name=?", "Production").Find(&mode)
	newWorkplace := Workplace{Name: workplaceName, Code: workplaceName, WorkplaceSectionId: section.ID, ActualStateId: state.ID, ActualWorkplaceModeId: mode.ID}
	db.Create(&newWorkplace)
	var workplace Workplace
	db.Where("name=?", workplaceName).Find(&workplace)
	var devicePortDigital DevicePort
	db.Where("name=?", "Production").Where("device_id=?", device.ID).Find(&devicePortDigital)
	var productionState State
	db.Where("name=?", "Production").Find(&productionState)
	digitalPort := WorkplacePort{Name: "Production", DevicePortId: devicePortDigital.ID, WorkplaceId: workplace.ID, StateId: productionState.ID}
	db.Create(&digitalPort)
	var devicePortAnalog DevicePort
	db.Where("name=?", "Amperage").Where("device_id=?", device.ID).Find(&devicePortAnalog)
	var offlineState State
	db.Where("name=?", "Offline").Find(&offlineState)
	analogPort := WorkplacePort{Name: "Amperage", DevicePortId: devicePortAnalog.ID, WorkplaceId: workplace.ID, StateId: offlineState.ID}
	db.Create(&analogPort)

}

func CheckWorkplace(workplace Workplace) bool {
	for _, runningWorkplace := range runningWorkplaces {
		if runningWorkplace.Name == workplace.Name {
			return true
		}
	}
	return false
}

func RunWorkplace(workplace Workplace) {
	LogInfo(workplace.Name, "Workplace started running")
	workplaceSync.Lock()
	runningWorkplaces = append(runningWorkplaces, workplace)
	workplaceSync.Unlock()
	workplaceIsActive := true
	for workplaceIsActive {
		start := time.Now()
		intermediateData := workplace.AddData()
		LogInfo(workplace.Name, "Download and sort takes: "+time.Since(start).String())
		ProcessData(&workplace, intermediateData)
		LogInfo(workplace.Name, "Processing takes "+time.Since(start).String())
		workplace.Sleep(start)
		workplaceIsActive = CheckActive(workplace)
	}
	RemoveWorkplaceFromRunningWorkplaces(workplace)
	LogInfo(workplace.Name, "Workplace not active, stopped running")

}

func CheckActive(workplace Workplace) bool {
	for _, activeWorkplace := range activeWorkplaces {
		if activeWorkplace.Name == workplace.Name {
			LogInfo(workplace.Name, "Workplace still active")
			return true
		}
	}
	LogInfo(workplace.Name, "Workplace not active")
	return false
}

func RemoveWorkplaceFromRunningWorkplaces(workplace Workplace) {
	for idx, runningWorkplace := range runningWorkplaces {
		if workplace.Name == runningWorkplace.Name {
			workplaceSync.Lock()
			runningWorkplaces = append(runningWorkplaces[0:idx], runningWorkplaces[idx+1:]...)
			workplaceSync.Unlock()
		}
	}
}

func UpdateActiveWorkplaces(reference string) {
	connectionString, dialect := CheckDatabaseType()
	db, err := gorm.Open(dialect, connectionString)
	if err != nil {
		LogError(reference, "Problem opening "+DatabaseName+" database: "+err.Error())
		activeWorkplaces = nil
		return
	}
	defer db.Close()
	db.Find(&activeWorkplaces)
}
