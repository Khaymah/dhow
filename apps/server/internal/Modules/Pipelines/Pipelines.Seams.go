package pipelines

import (
	"context"
	"errors"

	"github.com/google/uuid"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	OutputDefinition = "dhow.definition"
	OutputSha        = "dhow.sha"
	OutputFound      = "dhow.found"

	discoveryKey      = "dhow.discovery"
	discoveryName     = "Read the pipeline file"
	discoveryPriority = 50
)

var (
	ErrNeedsDiscovery      = errors.New("the provider has no API for files; a discovery job reads the pipeline file")
	ErrProviderUnavailable = errors.New("the Git provider of this repository is not available yet")
	ErrCheckoutUnavailable = errors.New("runners cannot check a repository out yet")
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

type CheckoutRequest struct {
	RunID          uuid.UUID
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	RepositoryID   uuid.UUID
	Ref            string
	Sha            string
	Path           string
}

type CheckoutSeam interface {
	DiscoverySpec(r CheckoutRequest) (*runnerv1.JobSpec, []string, error)
	ForJob(spec *runnerv1.JobSpec, r CheckoutRequest) ([]string, error)
}

type CheckoutPending struct{}

func (CheckoutPending) DiscoverySpec(CheckoutRequest) (*runnerv1.JobSpec, []string, error) {
	return nil, nil, ErrCheckoutUnavailable
}

func (CheckoutPending) ForJob(*runnerv1.JobSpec, CheckoutRequest) ([]string, error) { return nil, nil }

type ImageResolver interface {
	Resolve(ctx context.Context, image string) (string, error)
}
