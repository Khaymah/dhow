package models

import (
	"time"

	"github.com/google/uuid"
)

type Release struct {
	ID             uuid.UUID
	ReleaseID      string
	Kind           string
	State          string
	RollbackReason string
	StartedAt      time.Time
	FinishedAt     *time.Time
}

type Progress struct {
	ReleaseID string
	Hosts     int
	HostsDone int
	BatchSize int
	Batch     int
	Batches   int
}

type Target struct {
	ID               uuid.UUID
	Name             string
	Kind             string
	Hosts            int
	CurrentReleaseID string
	CurrentDigest    string
	Last             *Release
	Running          *Progress
	Partial          bool
	AutoRolledBack   bool
	RollbackFailed   bool
}

type Environment struct {
	ID               uuid.UUID
	Name             string
	Paused           bool
	PendingApprovals int
	Targets          []Target
}

type Board struct {
	Environments []Environment
	Truncated    bool
}
