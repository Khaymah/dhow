package models

import (
	"time"

	"github.com/google/uuid"
)

type CheckStarted struct {
	RunID     uuid.UUID
	RunNumber int64
}

type HostCheck struct {
	HostID            uuid.UUID
	Position          int
	Host              string
	Port              int
	Checked           bool
	Reason            string
	Message           string
	CurrentReleaseID  string
	VerifiedReleaseID string
	FreeBytes         *int64
	Locked            bool
	Repaired          bool
	CheckedAt         *time.Time
}

type TargetCheckResult struct {
	Hosts []HostCheck
}
