package main

import (
	"github.com/kardianos/service"
	"github.com/petrjahoda/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"strconv"
	"sync"
	"time"
)

const version = "2020.3.1.30"
const programName = "State Service"
const programDescription = "Creates states for workplaces"
const downloadInSeconds = 10
const config = "user=postgres password=Zps05..... dbname=version3 host=database port=5432 sslmode=disable"

var serviceRunning = false

type program struct{}

func (p *program) Start(s service.Service) error {
	LogInfo("MAIN", "Starting "+programName+" on "+s.Platform())
	go p.run()
	serviceRunning = true
	return nil
}

func (p *program) run() {
	LogInfo("MAIN", programName+" version "+version+" started")
	WriteProgramVersionIntoSettings()
	for {
		LogInfo("MAIN", "Program running")
		start := time.Now()
		UpdateActiveWorkplaces("MAIN")
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
func (p *program) Stop(s service.Service) error {
	serviceRunning = false
	for len(runningWorkplaces) != 0 {
		LogInfo("MAIN", "Stopping, still running devices: "+strconv.Itoa(len(runningWorkplaces)))
		time.Sleep(1 * time.Second)
	}
	LogInfo("MAIN", "Stopped on platform "+s.Platform())
	return nil
}

var (
	activeWorkplaces  []database.Workplace
	runningWorkplaces []database.Workplace
	workplaceSync     sync.Mutex
)

func main() {
	serviceConfig := &service.Config{
		Name:        programName,
		DisplayName: programName,
		Description: programDescription,
	}
	prg := &program{}
	s, err := service.New(prg, serviceConfig)
	if err != nil {
		LogError("MAIN", err.Error())
	}
	err = s.Run()
	if err != nil {
		LogError("MAIN", "Problem starting "+serviceConfig.Name)
	}
}

func CheckWorkplace(workplace database.Workplace) bool {
	for _, runningWorkplace := range runningWorkplaces {
		if runningWorkplace.Name == workplace.Name {
			return true
		}
	}
	return false
}

func RunWorkplace(workplace database.Workplace) {
	LogInfo(workplace.Name, "Workplace started running")
	workplaceSync.Lock()
	runningWorkplaces = append(runningWorkplaces, workplace)
	workplaceSync.Unlock()
	workplaceIsActive := true
	var digitalDateTime time.Time
	var analogDateTime time.Time
	for workplaceIsActive && serviceRunning {
		LogInfo(workplace.Name, "Starting workplace loop")
		LogInfo(workplace.Name, "Analog datetime: "+analogDateTime.String())
		LogInfo(workplace.Name, "Digital datetime: "+digitalDateTime.String())
		timer := time.Now()
		intermediateData := AddData(workplace, analogDateTime, digitalDateTime)
		analogDateTime, digitalDateTime = ProcessData(&workplace, intermediateData, analogDateTime, digitalDateTime)
		LogInfo(workplace.Name, "Loop ended, elapsed: "+time.Since(timer).String())
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

func UpdateActiveWorkplaces(reference string) {
	LogInfo("MAIN", "Updating active workplaces")
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
	LogInfo("MAIN", "Active workplaces updated, elapsed: "+time.Since(timer).String())
}

func WriteProgramVersionIntoSettings() {
	LogInfo("MAIN", "Updating program version in database")
	timer := time.Now()
	db, err := gorm.Open(postgres.Open(config), &gorm.Config{})
	if err != nil {
		LogError("MAIN", "Problem opening database: "+err.Error())
		return
	}
	sqlDB, err := db.DB()
	defer sqlDB.Close()
	var settings database.Setting
	db.Where("name=?", programName).Find(&settings)
	settings.Name = programName
	settings.Value = version
	db.Save(&settings)
	LogInfo("MAIN", "Program version updated, elapsed: "+time.Since(timer).String())
}
