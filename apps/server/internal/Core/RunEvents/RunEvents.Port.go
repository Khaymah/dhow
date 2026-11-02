package runevents

import (
	"context"

	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

type Finished struct {
	OrganizationID uuid.UUID
	JobID          uuid.UUID
	OriginID       uuid.UUID
	State          string
	FailureReason  string
	Outputs        map[string]string
}

type Sink interface {
	JobFinished(ctx context.Context, tx *sqlc.Queries, f Finished) error
}
