package pipelines

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

const (
	OutputFound  = "dhow_found"
	OutputDigest = "dhow_sha256"

	discoveryKey      = "dhow.discovery"
	discoveryName     = "Read the pipeline file"
	discoveryPriority = 50
)

var (
	ErrNeedsDiscovery      = errors.New("the provider has no API for files; a discovery job reads the pipeline file")
	ErrProviderUnavailable = errors.New("the Git provider of this repository is not available yet")
	ErrCheckoutUnavailable = errors.New("this server has no checkout image for discovery jobs")
)

type DefinitionRequest struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	RepositoryID   uuid.UUID
	Provider       string
	Ref            string
	Sha            string
	Path           string
}

type Fetched struct {
	Source []byte
	Sha    string
	Found  bool
}

type Definitions interface {
	Fetch(ctx context.Context, r DefinitionRequest) (Fetched, error)
}

type ProviderDefinitions struct{}

func (ProviderDefinitions) Fetch(_ context.Context, r DefinitionRequest) (Fetched, error) {
	if r.Provider == providerGeneric {
		return Fetched{}, ErrNeedsDiscovery
	}
	return Fetched{}, ErrProviderUnavailable
}

type ImageResolver interface {
	Resolve(ctx context.Context, image string) (string, error)
}
