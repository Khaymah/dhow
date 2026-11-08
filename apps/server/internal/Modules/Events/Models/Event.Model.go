package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	TypeDeploymentStarted    = "deployment.started"
	TypeDeploymentFinished   = "deployment.finished"
	TypeApprovalRequested    = "approval.requested"
	TypeApprovalDecided      = "approval.decided"
	TypeApprovalExpired      = "approval.expired"
	TypeReleaseStarted       = "release.started"
	TypeReleaseSwitched      = "release.switched"
	TypeReleaseSucceeded     = "release.succeeded"
	TypeReleaseRolledBack    = "release.rolled_back"
	TypeReleaseRollbackFail  = "release.rollback_failed"
	TypeReleaseUnknown       = "release.unknown"
	TypeReleaseRepaired      = "release.repaired"
	TypeTargetChecked        = "target.checked"
	TypeTargetCheckRequested = "target.check_requested"
	TypeTargetRollbackAsked  = "target.rollback_requested"
	TypeOverviewChanged      = "overview.changed"
	TypeCanary               = "canary"

	NameChange = "change"
	NameReset  = "reset"
)

type Event struct {
	Seq           uint64     `json:"Seq"`
	Type          string     `json:"Type"`
	Status        string     `json:"Status,omitempty"`
	ProjectID     *uuid.UUID `json:"ProjectId,omitempty"`
	EnvironmentID *uuid.UUID `json:"EnvironmentId,omitempty"`
	TargetID      *uuid.UUID `json:"TargetId,omitempty"`
	SubjectID     *uuid.UUID `json:"SubjectId,omitempty"`
	At            time.Time  `json:"At"`
}

type Notice struct {
	Organization  uuid.UUID  `json:"Organization"`
	Type          string     `json:"Type"`
	Status        string     `json:"Status,omitempty"`
	ProjectID     *uuid.UUID `json:"ProjectId,omitempty"`
	EnvironmentID *uuid.UUID `json:"EnvironmentId,omitempty"`
	TargetID      *uuid.UUID `json:"TargetId,omitempty"`
	SubjectID     *uuid.UUID `json:"SubjectId,omitempty"`
	At            time.Time  `json:"At"`
}

type Reset struct {
	Epoch int64 `json:"Epoch"`
}
