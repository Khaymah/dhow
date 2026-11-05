package secretstore

import (
	"context"

	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

type EnvironmentSecret struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
	Name           string
	Description    string
	Value          Value
}

type Written struct {
	Version int32
	Created bool
}

type EnvironmentWriter interface {
	PutEnvironmentSecret(ctx context.Context, q *sqlc.Queries, w EnvironmentSecret) (Written, error)
}
