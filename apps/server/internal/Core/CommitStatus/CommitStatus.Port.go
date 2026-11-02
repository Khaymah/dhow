package commitstatus

import (
	"context"

	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const (
	StatePending  = "pending"
	StateRunning  = "running"
	StateSuccess  = "success"
	StateFailed   = "failed"
	StateCanceled = "canceled"
)

type Desired struct {
	OrganizationID uuid.UUID
	RepositoryID   uuid.UUID
	RunID          uuid.UUID
	Sha            string
	State          string
	TargetURL      string
}

type Recorder interface {
	Record(ctx context.Context, tx *sqlc.Queries, d Desired) error
}
