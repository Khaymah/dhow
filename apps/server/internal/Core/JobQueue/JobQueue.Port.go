package jobqueue

import (
	"context"

	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	OriginManual   = "manual"
	OriginPipeline = "pipeline"
)

const (
	FeatureCheckout  = "checkout"
	FeaturePlugins   = "plugins"
	FeatureArtifacts = "artifacts"
	FeatureCache     = "cache"
	FeatureRegistry  = "registry_auth"
	FeatureDeploy    = "deploy_targets"
)

type Request struct {
	OrganizationID   uuid.UUID
	CreatedByUserID  uuid.UUID
	OriginKind       string
	OriginID         uuid.UUID
	Name             string
	Spec             *runnerv1.JobSpec
	Priority         int
	MaxInfraRetries  *int
	RequiredFeatures []string
}

type Enqueued struct {
	JobID      uuid.UUID
	SpecDigest []byte
}

type Cancellation struct {
	OrganizationID uuid.UUID
	JobID          uuid.UUID
	RequestedBy    uuid.UUID
}

type Queue interface {
	Enqueue(ctx context.Context, tx *sqlc.Queries, r Request) (Enqueued, error)
	Cancel(ctx context.Context, tx *sqlc.Queries, c Cancellation) (bool, error)
}
