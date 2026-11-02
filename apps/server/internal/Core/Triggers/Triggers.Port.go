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
)

const (
	ProviderGitHub  = "github"
	ProviderGitLab  = "gitlab"
	ProviderGeneric = "generic"
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
	IsFork         bool
	Actor          string
}

type Accepted struct {
	RunID   uuid.UUID
	Created bool
}

type Sink interface {
	Deliver(ctx context.Context, tx *sqlc.Queries, t Trigger) (Accepted, error)
}
