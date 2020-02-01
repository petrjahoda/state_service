package main

import (
	"github.com/jinzhu/gorm"
	"github.com/petrjahoda/zapsi_database"
	"strconv"
	"sync"
	"time"
)

const version = "2020.1.2.1"
const programName = "State Service"
const deleteLogsAfter = 240 * time.Hour
const downloadInSeconds = 10

var (
	activeWorkplaces  []zapsi_database.Workplace
	runningWorkplaces []zapsi_database.Workplace
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
		databaseOk := zapsi_database.CheckDatabase(DatabaseType, DatabaseIpAddress, DatabasePort, DatabaseLogin, DatabaseName, DatabasePassword)
		tablesOk, err := zapsi_database.CheckTables(DatabaseType, DatabaseIpAddress, DatabasePort, DatabaseLogin, DatabaseName, DatabasePassword)
		if err != nil {
			LogInfo("MAIN", "Problem creating tables: "+err.Error())
		}
		if databaseOk && tablesOk {
			WriteProgramVersionIntoSettings()
			firstRunCheckComplete = true
		}
	}
}

func CheckWorkplace(workplace zapsi_database.Workplace) bool {
	for _, runningWorkplace := range runningWorkplaces {
		if runningWorkplace.Name == workplace.Name {
			return true
		}
	}
	return false
}

func RunWorkplace(workplace zapsi_database.Workplace) {
	LogInfo(workplace.Name, "Workplace started running")
	workplaceSync.Lock()
	runningWorkplaces = append(runningWorkplaces, workplace)
	workplaceSync.Unlock()
	workplaceIsActive := true
	for workplaceIsActive {
		start := time.Now()
		intermediateData := AddData(workplace)
		LogInfo(workplace.Name, "Download and sort of length "+strconv.Itoa(len(intermediateData))+" takes: "+time.Since(start).String())
		ProcessData(&workplace, intermediateData)
		LogInfo(workplace.Name, "Processing takes "+time.Since(start).String())
		Sleep(workplace, start)
		workplaceIsActive = CheckActive(workplace)
	}
	RemoveWorkplaceFromRunningWorkplaces(workplace)
	LogInfo(workplace.Name, "Workplace not active, stopped running")

}

func Sleep(workplace zapsi_database.Workplace, start time.Time) {
	if time.Since(start) < (downloadInSeconds * time.Second) {
		sleepTime := downloadInSeconds*time.Second - time.Since(start)
		LogInfo(workplace.Name, "Sleeping for "+sleepTime.String())
		time.Sleep(sleepTime)
	}
}

func CheckActive(workplace zapsi_database.Workplace) bool {
	for _, activeWorkplace := range activeWorkplaces {
		if activeWorkplace.Name == workplace.Name {
			LogInfo(workplace.Name, "Workplace still active")
			return true
		}
	}
	LogInfo(workplace.Name, "Workplace not active")
	return false
}

func RemoveWorkplaceFromRunningWorkplaces(workplace zapsi_database.Workplace) {
	for idx, runningWorkplace := range runningWorkplaces {
		if workplace.Name == runningWorkplace.Name {
			workplaceSync.Lock()
			runningWorkplaces = append(runningWorkplaces[0:idx], runningWorkplaces[idx+1:]...)
			workplaceSync.Unlock()
		}
	}
}

func UpdateActiveWorkplaces(reference string) {
	connectionString, dialect := zapsi_database.CheckDatabaseType(DatabaseType, DatabaseIpAddress, DatabasePort, DatabaseLogin, DatabaseName, DatabasePassword)
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
	connectionString, dialect := zapsi_database.CheckDatabaseType(DatabaseType, DatabaseIpAddress, DatabasePort, DatabaseLogin, DatabaseName, DatabasePassword)
	db, err := gorm.Open(dialect, connectionString)
	if err != nil {
		LogError("MAIN", "Problem opening "+DatabaseName+" database: "+err.Error())
		return
	}
	defer db.Close()
	var settings zapsi_database.Setting
	db.Where("key=?", programName).Find(&settings)
	settings.Key = programName
	settings.Value = version
	db.Save(&settings)
	LogDebug("MAIN", "Updated version in database for "+programName)
}
