package models

import (
	"time"

	"github.com/google/uuid"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

type Job struct {
	ID                uuid.UUID
	OrganizationID    uuid.UUID
	Name              string
	State             string
	FailureReason     string
	QueuedReason      string
	Labels            []string
	Isolation         string
	Priority          int
	Image             string
	SpecDigest        []byte
	AttemptCount      int
	MaxInfraRetries   int
	QueuedAt          time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
	Deadline          *time.Time
	QueueDeadline     time.Time
	CancelRequestedAt *time.Time
	CreatedAt         time.Time
	CreatedByUserID   *uuid.UUID
}

type Step struct {
	Index         int
	Name          string
	Mutating      bool
	State         string
	ExitCode      *int
	FailureReason string
	StartedAt     time.Time
	FinishedAt    *time.Time
}

type Attempt struct {
	ID                uuid.UUID
	Number            int
	RunnerID          uuid.UUID
	FencingToken      int64
	State             string
	FailureReason     string
	ExitCode          *int
	LeasedAt          time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
	MutationStartedAt *time.Time
	Steps             []Step
}

type Detail struct {
	Job      Job
	Spec     *runnerv1.JobSpec
	Attempts []Attempt
}

type CreateInput struct {
	Name            string
	Spec            *runnerv1.JobSpec
	Priority        *int
	MaxInfraRetries *int
}

type Outcome struct {
	State         string
	FailureReason string
	ExitCode      *int32
	Digest        []byte
	LeaseState    string
	Outputs       map[string]string
}
