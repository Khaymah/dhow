package models

import (
	"time"

	"github.com/google/uuid"
)

type ReleaseHost struct {
	HostID         uuid.UUID
	Position       int
	State          string
	RollbackReason string
	Message        string
}

type TargetRelease struct {
	ID                uuid.UUID
	ReleaseID         string
	Kind              string
	State             string
	RestoredReleaseID string
	PreviousReleaseID string
	RollbackReason    string
	Message           string
	StartedAt         time.Time
	FinishedAt        *time.Time
	RollbackTarget    bool
	ArtifactKept      bool
	Hosts             []ReleaseHost
}

type TargetReleases struct {
	CurrentReleaseID string
	Partial          bool
	Releases         []TargetRelease
}
