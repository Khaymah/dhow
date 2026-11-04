package deployments

import (
	"context"

	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const (
	RefBranch = "branch"
	RefTag    = "tag"

	GateApproval        = "approval"
	GateTimer           = "timer"
	GatePaused          = "paused"
	GateEnvironmentBusy = "environment_busy"
)

type Target struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	Name           string
	RefKind        string
	RefName        string
	PullRequest    bool
}

type Environment struct {
	ID                  uuid.UUID
	Name                string
	RuleVersion         int64
	RefAllowed          bool
	HasReviewers        bool
	WaitMinutes         int
	RequirePinnedImages bool
	AllowedPlugins      []string
	Paused              bool
	LockedByRunID       *uuid.UUID
}

type Deployment struct {
	OrganizationID uuid.UUID
	EnvironmentID  uuid.UUID
	RunID          uuid.UUID
	RunNumber      int64
	RunJobID       uuid.UUID
	JobID          uuid.UUID
	JobKey         string
	Ref            string
	Sha            string
	RuleVersion    int64
}

type Gate interface {
	Lock(ctx context.Context, tx *sqlc.Queries, t Target) (Environment, bool, error)
	Started(ctx context.Context, tx *sqlc.Queries, d Deployment) error
	Settle(ctx context.Context, tx *sqlc.Queries, org, run uuid.UUID, keep []string) error
	ReleaseStale(ctx context.Context, tx *sqlc.Queries) (int64, error)
}
