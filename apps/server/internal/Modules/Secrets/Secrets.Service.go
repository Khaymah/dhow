package secrets

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
)

const projectArchived = "archived"

type DB interface {
	sqlc.DBTX
	dbtx.Beginner
}

type Publisher interface {
	Publish(ctx context.Context, event eventbus.Event) error
}

type Options struct {
	DB     DB
	Values *keys.Values
	Bus    Publisher
	Log    *slog.Logger
	Now    func() time.Time
}

type Service struct {
	db       DB
	secrets  *keys.Column
	registry *keys.Column
	envelope *keys.Envelope
	bus      Publisher
	log      *slog.Logger
	now      func() time.Time
}

func NewService(o Options) (*Service, error) {
	if o.Values == nil {
		return nil, errors.New("secrets: the keyring values are required")
	}
	secretColumn, err := o.Values.Column(SecretValueSpec())
	if err != nil {
		return nil, err
	}
	registryColumn, err := o.Values.Column(RegistryPasswordSpec())
	if err != nil {
		return nil, err
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	log := o.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Service{
		db: o.DB, secrets: secretColumn, registry: registryColumn, envelope: o.Values.Envelope(), bus: o.Bus, log: log, now: now,
	}, nil
}

func principalOf(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return auth.Principal{}, apperrors.Unauthenticated()
	}
	return p, nil
}

func notFoundOr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.NotFound()
	}
	return err
}

func (s *Service) target(ctx context.Context, q *sqlc.Queries, ref models.Ref, write bool) (models.Target, error) {
	t := models.Target{Scope: models.ScopeOrganization, OrganizationID: ref.OrganizationID}
	if ref.ProjectID == nil {
		return t, nil
	}
	state, err := q.GetSecretsProject(ctx, sqlc.GetSecretsProjectParams{ProjectID: *ref.ProjectID, OrganizationID: ref.OrganizationID})
	if err != nil {
		return models.Target{}, notFoundOr(err)
	}
	if write && state == projectArchived {
		return models.Target{}, apperrors.ProjectArchived()
	}
	t.Scope, t.ProjectID = models.ScopeProject, ref.ProjectID
	if ref.EnvironmentName == "" {
		return t, nil
	}
	env, err := q.GetSecretsEnvironment(ctx, sqlc.GetSecretsEnvironmentParams{
		ProjectID: *ref.ProjectID, OrganizationID: ref.OrganizationID, Name: ref.EnvironmentName,
	})
	if err != nil {
		return models.Target{}, notFoundOr(err)
	}
	t.Scope, t.EnvironmentID = models.ScopeEnvironment, &env
	return t, nil
}

func lockKey(kind string, t models.Target) string {
	parts := []string{"secrets", kind, t.OrganizationID.String()}
	if t.ProjectID != nil {
		parts = append(parts, t.ProjectID.String())
	}
	if t.EnvironmentID != nil {
		parts = append(parts, t.EnvironmentID.String())
	}
	return strings.Join(parts, ":")
}

func ownerOf(scope string, org uuid.UUID, project, env *uuid.UUID) keys.Owner {
	switch {
	case scope == models.ScopeEnvironment && env != nil:
		return keys.EnvironmentOwner(*env)
	case scope == models.ScopeProject && project != nil:
		return keys.ProjectOwner(*project)
	default:
		return keys.OrganizationOwner(org)
	}
}

func secretLimit(scope string) int64 {
	switch scope {
	case models.ScopeEnvironment:
		return config.SecretsPerEnvironment
	case models.ScopeProject:
		return config.SecretsPerProject
	default:
		return config.SecretsPerOrganization
	}
}

func overridesOf(scope, lower string, wider map[string][]string) []string {
	out := []string{}
	if scope == models.ScopeOrganization {
		return out
	}
	for _, w := range []string{models.ScopeProject, models.ScopeOrganization} {
		if w == scope {
			continue
		}
		for _, n := range wider[w] {
			if n == lower {
				out = append(out, w)
				break
			}
		}
	}
	return out
}

func (s *Service) widerSecrets(ctx context.Context, q *sqlc.Queries, t models.Target) (map[string][]string, error) {
	out := map[string][]string{}
	if t.Scope == models.ScopeOrganization {
		return out, nil
	}
	rows, err := q.VisibleSecretNames(ctx, sqlc.VisibleSecretNamesParams{OrganizationID: t.OrganizationID, ProjectID: t.ProjectID})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.Scope == models.ScopeProject && t.Scope == models.ScopeProject {
			continue
		}
		out[r.Scope] = append(out[r.Scope], r.LowerName)
	}
	return out, nil
}

func valueOr[T any](v *T, fallback T) T {
	if v == nil {
		return fallback
	}
	return *v
}

func itoa(v int32) string { return strconv.FormatInt(int64(v), 10) }
