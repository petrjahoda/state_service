package main

import (
	"github.com/jinzhu/gorm"
	"github.com/kardianos/service"
	"github.com/petrjahoda/zapsi_database"
	"strconv"
	"sync"
	"time"
)

const version = "2020.2.2.18"
const programName = "State Service"
const programDesription = "Creates states for workplaces"
const deleteLogsAfter = 240 * time.Hour
const downloadInSeconds = 10

var serviceRunning = false
var serviceDirectory string

type program struct{}

func (p *program) Start(s service.Service) error {
	LogInfo("MAIN", "Starting "+programName+" on "+s.Platform())
	go p.run()
	serviceRunning = true
	return nil
}

func (p *program) run() {
	time.Sleep(time.Second * 5)
	LogDirectoryFileCheck("MAIN")
	LogInfo("MAIN", programName+" version "+version+" started")
	CreateConfigIfNotExists()
	LoadSettingsFromConfigFile()
	LogDebug("MAIN", "Using ["+DatabaseType+"] on "+DatabaseIpAddress+":"+DatabasePort+" with database "+DatabaseName)
	WriteProgramVersionIntoSettings()
	for {
		LogInfo("MAIN", "Program running")
		start := time.Now()
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
	activeWorkplaces  []zapsi_database.Workplace
	runningWorkplaces []zapsi_database.Workplace
	workplaceSync     sync.Mutex
)

func init() {
	serviceDirectory = GetDirectory()
}

func main() {
	serviceConfig := &service.Config{
		Name:        programName,
		DisplayName: programName,
		Description: programDesription,
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
	connectionString, dialect := zapsi_database.CheckDatabaseType(DatabaseType, DatabaseIpAddress, DatabasePort, DatabaseLogin, DatabaseName, DatabasePassword)
	db, err := gorm.Open(dialect, connectionString)
	if err != nil {
		LogError(reference, "Problem opening "+DatabaseName+" database: "+err.Error())
		activeWorkplaces = nil
		return
	}
	db.LogMode(false)
	defer db.Close()
	db.Find(&activeWorkplaces)
	LogInfo("MAIN", "Active workplaces updated, elapsed: "+time.Since(timer).String())
}

func WriteProgramVersionIntoSettings() {
	LogInfo("MAIN", "Updating program version in database")
	timer := time.Now()
	connectionString, dialect := zapsi_database.CheckDatabaseType(DatabaseType, DatabaseIpAddress, DatabasePort, DatabaseLogin, DatabaseName, DatabasePassword)
	db, err := gorm.Open(dialect, connectionString)
	if err != nil {
		LogError("MAIN", "Problem opening "+DatabaseName+" database: "+err.Error())
		return
	}
	db.LogMode(false)
	defer db.Close()
	var settings zapsi_database.Setting
	db.Where("name=?", programName).Find(&settings)
	settings.Name = programName
	settings.Value = version
	db.Save(&settings)
	LogInfo("MAIN", "Program version updated, elapsed: "+time.Since(timer).String())
}
