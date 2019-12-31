package main

import (
	"time"
)

type IntermediateData struct {
	Type     IntermediateDataType
	DateTime time.Time
	RawData  string
}

type IntermediateDataType int

const (
	production IntermediateDataType = iota
	poweroff
	special
)
