package main

import (
	"time"
)

func (workplace Workplace) Sleep(start time.Time) {
	if time.Since(start) < (downloadInSeconds * time.Second) {
		sleepTime := downloadInSeconds*time.Second - time.Since(start)
		LogInfo(workplace.Name, "Sleeping for "+sleepTime.String())
		time.Sleep(sleepTime)
	}
}

//
//func (workplace Workplace) AddData() IntermediateData {
//
//
//}
//
//func (workplace Workplace) ProcessData(data IntermediateData) {
//
//}
