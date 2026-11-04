package jobs

import (
	"context"
	"errors"

	"github.com/google/uuid"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var (
	ErrSecretsUnavailable     = errors.New("no secret store is configured")
	ErrEnvironmentRuleChanged = errors.New("the environment of the job or its rules changed after the gate")
)

type SecretRequest struct {
	OrganizationID uuid.UUID
	JobID          uuid.UUID
	AttemptID      uuid.UUID
	Names          []string
	Checkout       *runnerv1.Checkout
	Registries     map[string]string
}

const (
	OriginManual   = "manual"
	OriginPipeline = "pipeline"
)

type Origin struct {
	OrganizationID uuid.UUID
	Kind           string
	ID             uuid.UUID
}

type SecretResolver interface {
	Accepts(ctx context.Context, origin Origin, names []string) error
	Resolve(ctx context.Context, req SecretRequest) (map[string][]byte, error)
}

type StartGuard interface {
	Admit(ctx context.Context, req SecretRequest) error
}

type NoSecrets struct{}

func (NoSecrets) Accepts(context.Context, Origin, []string) error { return ErrSecretsUnavailable }

func (NoSecrets) Resolve(context.Context, SecretRequest) (map[string][]byte, error) {
	return nil, ErrSecretsUnavailable
}
