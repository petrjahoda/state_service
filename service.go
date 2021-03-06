package main

import (
	"github.com/petrjahoda/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"time"
)

func checkWorkplaceInRunningWorkplaces(workplace database.Workplace) bool {
	for _, runningWorkplace := range runningWorkplaces {
		if runningWorkplace.Name == workplace.Name {
			return true
		}
	}
	return false
}

func runWorkplace(workplace database.Workplace) {
	logInfo(workplace.Name, "Workplace active, started running")
	workplaceSync.Lock()
	runningWorkplaces = append(runningWorkplaces, workplace)
	workplaceSync.Unlock()
	workplaceIsActive := true
	var digitalDateTime time.Time
	var analogDateTime time.Time
	for workplaceIsActive && serviceRunning {
		logInfo(workplace.Name, "Workplace main loop started")
		timer := time.Now()
		logInfo(workplace.Name, "Analog datetime: "+analogDateTime.String())
		logInfo(workplace.Name, "Digital datetime: "+digitalDateTime.String())
		db, err := gorm.Open(postgres.Open(config), &gorm.Config{})
		sqlDB, _ := db.DB()

		if err != nil {
			logError(workplace.Name, "Problem opening database: "+err.Error())
			sleep(workplace, timer)
			continue
		}
		intermediateData := readDataForProcessing(workplace, db, analogDateTime, digitalDateTime)
		analogDateTime, digitalDateTime = processData(&workplace, db, intermediateData, analogDateTime, digitalDateTime)
		logInfo(workplace.Name, "Workplace main loop ended in "+time.Since(timer).String())
		sleep(workplace, timer)
		time.Sleep(10 * time.Second)
		workplaceIsActive = checkActive(workplace)
		sqlDB.Close()
	}
	removeWorkplaceFromRunningWorkplaces(workplace)
	logInfo(workplace.Name, "Workplace not active, stopped running")

}

func sleep(workplace database.Workplace, start time.Time) {
	if time.Since(start) < (downloadInSeconds * time.Second) {
		sleepTime := downloadInSeconds*time.Second - time.Since(start)
		logInfo(workplace.Name, "Sleeping for "+sleepTime.String())
		time.Sleep(sleepTime)
	}
}

func checkActive(workplace database.Workplace) bool {
	for _, activeWorkplace := range activeWorkplaces {
		if activeWorkplace.Name == workplace.Name {
			logInfo(workplace.Name, "Workplace still active")
			return true
		}
	}
	logInfo(workplace.Name, "Workplace not active")
	return false
}

func removeWorkplaceFromRunningWorkplaces(workplace database.Workplace) {
	workplaceSync.Lock()
	for idx, runningWorkplace := range runningWorkplaces {
		if workplace.Name == runningWorkplace.Name {
			runningWorkplaces = append(runningWorkplaces[0:idx], runningWorkplaces[idx+1:]...)
		}
	}
	workplaceSync.Unlock()
}

func readActiveWorkplaces(reference string) {
	logInfo("MAIN", "Reading active workplaces")
	timer := time.Now()
	db, err := gorm.Open(postgres.Open(config), &gorm.Config{})
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err != nil {
		logError(reference, "Problem opening database: "+err.Error())
		activeWorkplaces = nil
		return
	}
	db.Find(&activeWorkplaces)
	logInfo("MAIN", "Active workplaces read in "+time.Since(timer).String())
}

func updateProgramVersion() {
	logInfo("MAIN", "Writing program version into settings")
	timer := time.Now()
	db, err := gorm.Open(postgres.Open(config), &gorm.Config{})
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err != nil {
		logError("MAIN", "Problem opening database: "+err.Error())
		return
	}
	var existingSettings database.Setting
	db.Where("name=?", serviceName).Find(&existingSettings)
	existingSettings.Name = serviceName
	existingSettings.Value = version
	db.Save(&existingSettings)
	logInfo("MAIN", "Program version written into settings in "+time.Since(timer).String())
}
