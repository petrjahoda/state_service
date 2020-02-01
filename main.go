package main

import (
	"github.com/jinzhu/gorm"
	"strconv"
	"sync"
	"time"
)

const version = "2020.1.2.1"
const programName = "State Service"
const deleteLogsAfter = 240 * time.Hour
const downloadInSeconds = 10

var (
	activeWorkplaces  []Workplace
	runningWorkplaces []Workplace
	workplaceSync     sync.Mutex
)

func main() {
	LogDirectoryFileCheck("MAIN")
	LogInfo("MAIN", programName+" version "+version+" started")
	CreateConfigIfNotExists()
	LoadSettingsFromConfigFile()
	LogDebug("MAIN", "Using ["+DatabaseType+"] on "+DatabaseIpAddress+":"+DatabasePort+" with database "+DatabaseName)
	CompleteDatabaseCheck()
	for {
		start := time.Now()
		LogInfo("MAIN", "Program running")
		UpdateActiveWorkplaces("MAIN")
		DeleteOldLogFiles()
		LogInfo("MAIN", "Active workplaces: "+strconv.Itoa(len(activeWorkplaces))+", running workplaces: "+strconv.Itoa(len(runningWorkplaces)))
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

func CompleteDatabaseCheck() {
	firstRunCheckComplete := false
	for firstRunCheckComplete == false {
		databaseOk := CheckDatabase()
		tablesOk := CheckTables()
		if databaseOk && tablesOk {
			WriteProgramVersionIntoSettings()
			firstRunCheckComplete = true
		}
	}
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
		LogInfo(workplace.Name, "Download and sort of length "+strconv.Itoa(len(intermediateData))+" takes: "+time.Since(start).String())
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

func WriteProgramVersionIntoSettings() {
	connectionString, dialect := CheckDatabaseType()
	db, err := gorm.Open(dialect, connectionString)
	if err != nil {
		LogError("MAIN", "Problem opening "+DatabaseName+" database: "+err.Error())
		return
	}
	defer db.Close()
	var settings Setting
	db.Where("key=?", programName).Find(&settings)
	settings.Key = programName
	settings.Value = version
	db.Save(&settings)
	LogDebug("MAIN", "Updated version in database for "+programName)
}
