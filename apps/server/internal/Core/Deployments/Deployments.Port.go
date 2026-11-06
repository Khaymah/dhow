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
	GateTargetBusy      = "target_busy"

	SyntheticCheck = "check"

	ApprovalPassed     = "passed"
	ApprovalBypassed   = "bypassed"
	ApprovalWaiting    = "waiting"
	ApprovalRejected   = "rejected"
	ApprovalExpired    = "expired"
	ApprovalSuperseded = "superseded"
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
	ID                     uuid.UUID
	Name                   string
	RuleVersion            int64
	RefAllowed             bool
	HasReviewers           bool
	WaitMinutes            int
	RequirePinnedImages    bool
	AllowedPlugins         []string
	Paused                 bool
	LockedByRunID          *uuid.UUID
	ApprovalTimeoutHours   int
	SupersedePending       bool
	AllowExtraDeployEgress bool
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
	ApprovalID     *uuid.UUID
}

type ApprovalRequest struct {
	OrganizationID   uuid.UUID
	ProjectID        uuid.UUID
	EnvironmentID    uuid.UUID
	RunID            uuid.UUID
	RunNumber        int64
	Ref              string
	Round            int32
	RuleVersion      int64
	TimeoutHours     int
	SupersedePending bool
}

type Approval struct {
	ID         uuid.UUID
	Outcome    string
	Requested  bool
	Expired    bool
	Superseded []uuid.UUID
}

type Finished struct {
	DeploymentID  uuid.UUID
	EnvironmentID uuid.UUID
	RunJobID      uuid.UUID
	State         string
}

type Gate interface {
	Lock(ctx context.Context, tx *sqlc.Queries, t Target) (Environment, bool, error)
	Started(ctx context.Context, tx *sqlc.Queries, d Deployment) error
	Approve(ctx context.Context, tx *sqlc.Queries, r ApprovalRequest) (Approval, error)
	CancelApprovals(ctx context.Context, tx *sqlc.Queries, org, run uuid.UUID) error
	Settle(ctx context.Context, tx *sqlc.Queries, org, run uuid.UUID, keep []string) ([]Finished, error)
	ReleaseStale(ctx context.Context, tx *sqlc.Queries) (int64, error)
}

type Hooks struct {
	Reload     string
	PreSwitch  string
	PostSwitch string
}

type TargetSettings struct {
	BatchSize         int
	BatchPauseSeconds int
	Hooks             Hooks
	Shared            []string
	ComposeProject    string
	ComposeFile       string
	AllowUnsafe       bool
}

type TargetHost struct {
	ID       uuid.UUID
	Position int
	Host     string
	Port     int
	HostKey  string
}

type DeployTarget struct {
	ID            uuid.UUID
	Name          string
	Kind          string
	Username      string
	DeployPath    string
	KeySecretName string
	KeyFound      bool
	KeyVersion    int32
	KeepReleases  int
	Version       int64
	AllowedImages []string
	Settings      TargetSettings
	Hosts         []TargetHost
}

type Targets interface {
	Find(ctx context.Context, tx *sqlc.Queries, org, environment uuid.UUID, name string) (DeployTarget, bool, error)
	KeyNames(ctx context.Context, tx *sqlc.Queries, org, environment uuid.UUID) ([]string, error)
}

type SyntheticRequest struct {
	OrganizationID  uuid.UUID
	ProjectID       uuid.UUID
	EnvironmentID   uuid.UUID
	EnvironmentName string
	TargetName      string
	Kind            string
}

type SyntheticRun struct {
	RunID    uuid.UUID
	Number   int64
	Existing bool
}

type Synthetic interface {
	StartSynthetic(ctx context.Context, r SyntheticRequest) (SyntheticRun, error)
}
