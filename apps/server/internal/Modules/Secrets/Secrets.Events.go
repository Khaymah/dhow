package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

var ErrNoTransaction = errors.New("secrets: the environment removal event carries no transaction")

type Subscriber interface {
	Subscribe(name string, h eventbus.Handler)
}

func Subscribe(bus Subscriber, s *Service) {
	bus.Subscribe(eventbus.EnvironmentRemoved, s.environmentRemoved)
}

func (s *Service) environmentRemoved(ctx context.Context, event eventbus.Event) error {
	payload, ok := event.Payload.(eventbus.Audited)
	if !ok || payload.Tx == nil || len(payload.OrganizationIDs) != 1 {
		return ErrNoTransaction
	}
	env, err := uuid.Parse(payload.TargetID)
	if err != nil {
		return fmt.Errorf("secrets: the removed environment id %q: %w", payload.TargetID, err)
	}
	return s.RemoveEnvironment(ctx, payload.Tx, payload.OrganizationIDs[0], env)
}

func (s *Service) RemoveEnvironment(ctx context.Context, q *sqlc.Queries, org, env uuid.UUID) error {
	now := s.now()
	ids, err := q.RemoveEnvironmentSecrets(ctx, sqlc.RemoveEnvironmentSecretsParams{EnvironmentID: &env, OrganizationID: org, Now: now})
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		if _, err := q.EraseSecretVersionsOf(ctx, sqlc.EraseSecretVersionsOfParams{SecretIds: ids, Now: now}); err != nil {
			return err
		}
	}
	if _, err := q.RemoveEnvironmentVariables(ctx, sqlc.RemoveEnvironmentVariablesParams{EnvironmentID: &env, OrganizationID: org, Now: now}); err != nil {
		return err
	}
	_, err = s.envelope.DestroyIn(ctx, q, keys.EnvironmentOwner(env))
	return err
}
