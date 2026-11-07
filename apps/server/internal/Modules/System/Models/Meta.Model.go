package models

import "time"

type Build struct {
	Version string
	Commit  string
	BuiltAt time.Time
}

type Meta struct {
	Version       string
	Commit        string
	Build         Build
	SetupRequired bool
}

type DoctorStatus string

const (
	DoctorOK   DoctorStatus = "ok"
	DoctorInfo DoctorStatus = "info"
	DoctorWarn DoctorStatus = "warn"
	DoctorFail DoctorStatus = "fail"
)

type DoctorCheck struct {
	Code   string
	Status DoctorStatus
	Title  string
	Fix    string
}

type DoctorReport struct {
	Build  Build
	Health HealthStatus
	Checks []DoctorCheck
}
