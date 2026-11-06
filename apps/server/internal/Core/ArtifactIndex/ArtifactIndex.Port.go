package artifactindex

import (
	"context"

	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const (
	ReasonUntrustedForDeploy = "ARTIFACT_UNTRUSTED"
	ReasonIngestedGone       = "INGESTED_ARTIFACT_GONE"
)

type Request struct {
	OrganizationID uuid.UUID
	RunID          uuid.UUID
	RunJobID       uuid.UUID
	Needs          []string
	Ingested       []string
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

type DigestRequest struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	RunJobID       uuid.UUID
	Digest         []byte
}

type Index interface {
	Prepare(ctx context.Context, tx *sqlc.Queries, r Request) (Prepared, error)
	PrepareDigest(ctx context.Context, tx *sqlc.Queries, r DigestRequest) (Input, bool, error)
}
