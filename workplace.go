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
// download last state datetime
// download data for offline port
// download data for production port
// download data for special port
// data: datetime, data, stateid

//}

//func (workplace Workplace) ProcessData(data IntermediateData) {
// get actual offline difference
// get actual downtime difference
// for every record in date
//		get actual state
//  	case actual state == null -> change to downtime
//      case actual state == offline && downtime difference> downtime -> change to offline, change to downtime
//      case actual state == offline && port datatype == offlineport -> change to downtime
//      case actual state == downtime && port datatype == productionport && data == 1 -> change to production
//      case actual state == production && downtime difference> downtime -> change to downtime
//      case actual state != special &&  port datatype == specialport && data == 1 -> change to special
//      case actual state == special &&  port datatype == specialport && data == 0 -> change to downtime
// get actual state
// if actualstate.time minus actual datime > offline difference -> change to offline
//}
