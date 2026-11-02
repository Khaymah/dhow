package artifactindex

import (
	"context"

	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const ReasonUntrustedForDeploy = "ARTIFACT_UNTRUSTED"

type Request struct {
	OrganizationID uuid.UUID
	RunID          uuid.UUID
	RunJobID       uuid.UUID
	Needs          []string
	FeedsDeploy    bool
}

type Input struct {
	Name          string
	Digest        string
	SizeBytes     int64
	DownloadToken string
	Untrusted     bool
}

type Prepared struct {
	Inputs           []Input
	Refused          string
	ArtifactMaxBytes int64
	CacheMaxBytes    int64
}

type Index interface {
	Prepare(ctx context.Context, tx *sqlc.Queries, r Request) (Prepared, error)
}
