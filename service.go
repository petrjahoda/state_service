package main

import (
	"github.com/petrjahoda/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"time"
)

func CheckWorkplaceInRunningWorkplaces(workplace database.Workplace) bool {
	for _, runningWorkplace := range runningWorkplaces {
		if runningWorkplace.Name == workplace.Name {
			return true
		}
	}
	return false
}

func RunWorkplace(workplace database.Workplace) {
	LogInfo(workplace.Name, "Workplace active, started running")
	workplaceSync.Lock()
	runningWorkplaces = append(runningWorkplaces, workplace)
	workplaceSync.Unlock()
	workplaceIsActive := true
	var digitalDateTime time.Time
	var analogDateTime time.Time
	for workplaceIsActive && serviceRunning {
		LogInfo(workplace.Name, "Workplace main loop started")
		timer := time.Now()
		LogInfo(workplace.Name, "Analog datetime: "+analogDateTime.String())
		LogInfo(workplace.Name, "Digital datetime: "+digitalDateTime.String())
		intermediateData := ReadDataForProcessing(workplace, analogDateTime, digitalDateTime)
		analogDateTime, digitalDateTime = ProcessData(&workplace, intermediateData, analogDateTime, digitalDateTime)
		LogInfo(workplace.Name, "Workplace main loop ended in "+time.Since(timer).String())
		Sleep(workplace, timer)
		workplaceIsActive = CheckActive(workplace)
	}
	RemoveWorkplaceFromRunningWorkplaces(workplace)
	LogInfo(workplace.Name, "Workplace not active, stopped running")

}

func Sleep(workplace database.Workplace, start time.Time) {
	if time.Since(start) < (downloadInSeconds * time.Second) {
		sleepTime := downloadInSeconds*time.Second - time.Since(start)
		LogInfo(workplace.Name, "Sleeping for "+sleepTime.String())
		time.Sleep(sleepTime)
	}
}

func CheckActive(workplace database.Workplace) bool {
	for _, activeWorkplace := range activeWorkplaces {
		if activeWorkplace.Name == workplace.Name {
			LogInfo(workplace.Name, "Workplace still active")
			return true
		}
	}
	LogInfo(workplace.Name, "Workplace not active")
	return false
}

func RemoveWorkplaceFromRunningWorkplaces(workplace database.Workplace) {
	workplaceSync.Lock()
	for idx, runningWorkplace := range runningWorkplaces {
		if workplace.Name == runningWorkplace.Name {
			runningWorkplaces = append(runningWorkplaces[0:idx], runningWorkplaces[idx+1:]...)
		}
	}
	workplaceSync.Unlock()
}

func ReadActiveWorkplaces(reference string) {
	LogInfo("MAIN", "Reading active workplaces")
	timer := time.Now()
	db, err := gorm.Open(postgres.Open(config), &gorm.Config{})
	if err != nil {
		LogError(reference, "Problem opening database: "+err.Error())
		activeWorkplaces = nil
		return
	}
	sqlDB, err := db.DB()
	defer sqlDB.Close()
	db.Find(&activeWorkplaces)
	LogInfo("MAIN", "Active workplaces read in "+time.Since(timer).String())
}

func UpdateProgramVersion() {
	LogInfo("MAIN", "Writing program version into settings")
	timer := time.Now()
	db, err := gorm.Open(postgres.Open(config), &gorm.Config{})
	if err != nil {
		LogError("MAIN", "Problem opening database: "+err.Error())
		return
	}
	sqlDB, err := db.DB()
	defer sqlDB.Close()
	var existingSettings database.Setting
	db.Where("name=?", serviceName).Find(&existingSettings)
	existingSettings.Name = serviceName
	existingSettings.Value = version
	db.Save(&existingSettings)
	LogInfo("MAIN", "Program version written into settings in "+time.Since(timer).String())
}
