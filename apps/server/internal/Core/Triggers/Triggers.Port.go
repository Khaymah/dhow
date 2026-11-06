package triggers

import (
	"context"

	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const (
	EventPush        = "push"
	EventTag         = "tag"
	EventPullRequest = "pull_request"
	EventManual      = "manual"
	EventAPI         = "api"
	EventArtifact    = "artifact"
)

const (
	ProviderGitHub  = "github"
	ProviderGitLab  = "gitlab"
	ProviderGeneric = "generic"
	ProviderIngest  = "ingest"
)

type PullOrigin uint8

const (
	PullOriginUnknown PullOrigin = iota
	PullOriginSameRepository
	PullOriginFork
)

type Trigger struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	RepositoryID   uuid.UUID
	DeliveryID     uuid.UUID
	Provider       string
	Event          string
	Ref            string
	Sha            string
	BaseRef        string
	BaseSha        string
	PrNumber       int
	Origin         PullOrigin
	Actor          string
	Artifacts      []IngestedRef
}

type IngestedRef struct {
	ID   uuid.UUID
	Name string
}

const (
	ReasonRepositoryNotLinked  = "REPOSITORY_NOT_LINKED"
	ReasonDefaultBranchUnknown = "DEFAULT_BRANCH_UNKNOWN"
	ReasonProjectArchived      = "PROJECT_ARCHIVED"
	ReasonNoArtifactTrigger    = "NO_ARTIFACT_TRIGGER"
)

type Accepted struct {
	RunID   uuid.UUID
	Created bool
	Reason  string
}

type Sink interface {
	Deliver(ctx context.Context, tx *sqlc.Queries, t Trigger) (Accepted, error)
	Unlinked(ctx context.Context, tx *sqlc.Queries, organization, project uuid.UUID) error
}
