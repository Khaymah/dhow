package secrets

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
)

const (
	uniqueViolation    = "23505"
	registryConstraint = "UX_RegistryCredentials_Scope_Host"
)

type registryRow struct {
	ID              uuid.UUID
	ProjectID       *uuid.UUID
	Host, Username  string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	UpdatedByUserID uuid.UUID
}

func registryContext(id uuid.UUID) []byte { return []byte("registry:" + id.String()) }

func registryOf(r registryRow) models.RegistryCredential {
	scope := models.ScopeOrganization
	if r.ProjectID != nil {
		scope = models.ScopeProject
	}
	return models.RegistryCredential{
		ID: r.ID, Scope: scope, Host: r.Host, Username: r.Username, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		UpdatedByUserID: r.UpdatedByUserID,
	}
}

func hostTaken(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == registryConstraint
}

func (s *Service) ListRegistryCredentials(ctx context.Context, ref models.Ref, page, limit int) (models.Page[models.RegistryCredential], error) {
	q := sqlc.New(s.db)
	t, err := s.target(ctx, q, ref, false)
	if err != nil {
		return models.Page[models.RegistryCredential]{}, err
	}
	total, err := q.CountRegistryCredentialsInScope(ctx, sqlc.CountRegistryCredentialsInScopeParams{OrganizationID: t.OrganizationID, ProjectID: t.ProjectID})
	if err != nil {
		return models.Page[models.RegistryCredential]{}, err
	}
	rows, err := q.ListRegistryCredentialsInScope(ctx, sqlc.ListRegistryCredentialsInScopeParams{
		OrganizationID: t.OrganizationID, ProjectID: t.ProjectID,
		PageLimit: int32(limit), PageOffset: int32((page - 1) * limit), //nolint:gosec // limit and page are bounded by response.PageParams.
	})
	if err != nil {
		return models.Page[models.RegistryCredential]{}, err
	}
	items := make([]models.RegistryCredential, 0, len(rows))
	for _, r := range rows {
		items = append(items, registryOf(registryRow{r.Id, r.ProjectId, r.Host, r.Username, r.CreatedAt, r.UpdatedAt, r.UpdatedByUserId}))
	}
	return models.Page[models.RegistryCredential]{Items: items, Total: total}, nil
}

func (s *Service) CreateRegistryCredential(ctx context.Context, ref models.Ref, in models.RegistryInput) (models.RegistryCredential, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.RegistryCredential{}, err
	}
	host, err := checkRegistryInput(in, true)
	if err != nil {
		return models.RegistryCredential{}, err
	}
	var out models.RegistryCredential
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		t, err := s.target(ctx, q, ref, true)
		if err != nil {
			return err
		}
		if err := q.LockSecretsScope(ctx, lockKey("registry", t)); err != nil {
			return err
		}
		count, err := q.CountRegistryCredentialsInScope(ctx, sqlc.CountRegistryCredentialsInScopeParams{OrganizationID: t.OrganizationID, ProjectID: t.ProjectID})
		if err != nil {
			return err
		}
		if count >= config.RegistryCredentialsMax {
			return apperrors.RegistryCredentialLimitReached()
		}
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		sealed, err := s.sealPassword(ctx, q, t, id, in.Password)
		if err != nil {
			return err
		}
		row, err := q.InsertRegistryCredential(ctx, sqlc.InsertRegistryCredentialParams{
			ID: id, OrganizationID: t.OrganizationID, ProjectID: t.ProjectID, Host: host, Username: in.Username,
			PasswordSealed: sealed, UserID: p.UserID, Now: s.now(),
		})
		if hostTaken(err) {
			return apperrors.RegistryCredentialExists()
		}
		if err != nil {
			return err
		}
		out = registryOf(registryRow{row.Id, row.ProjectId, row.Host, row.Username, row.CreatedAt, row.UpdatedAt, row.UpdatedByUserId})
		return s.auditNamed(ctx, q, p, eventbus.RegistryCredentialCreated, targetRegistry, row.Id, t, "", map[string]string{"Host": row.Host})
	})
	return out, err
}

func (s *Service) sealPassword(ctx context.Context, q *sqlc.Queries, t models.Target, id uuid.UUID, password string) ([]byte, error) {
	plain := []byte(password)
	defer clear(plain)
	return s.registry.SealIn(ctx, q, ownerOf(t.Scope, t.OrganizationID, t.ProjectID, nil), registryContext(id), plain)
}

func (s *Service) UpdateRegistryCredential(ctx context.Context, ref models.Ref, id uuid.UUID, in models.RegistryInput) (models.RegistryCredential, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.RegistryCredential{}, err
	}
	if _, err := checkRegistryInput(in, false); err != nil {
		return models.RegistryCredential{}, err
	}
	var out models.RegistryCredential
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		t, err := s.target(ctx, q, ref, true)
		if err != nil {
			return err
		}
		current, err := q.GetRegistryCredentialForUpdate(ctx, sqlc.GetRegistryCredentialForUpdateParams{ID: id, OrganizationID: t.OrganizationID, ProjectID: t.ProjectID})
		if err != nil {
			return notFoundOr(err)
		}
		sealed, err := s.sealPassword(ctx, q, t, current.Id, in.Password)
		if err != nil {
			return err
		}
		row, err := q.UpdateRegistryCredential(ctx, sqlc.UpdateRegistryCredentialParams{
			ID: current.Id, OrganizationID: t.OrganizationID, Username: in.Username, PasswordSealed: sealed, UserID: p.UserID, Now: s.now(),
		})
		if err != nil {
			return err
		}
		out = registryOf(registryRow{row.Id, row.ProjectId, row.Host, row.Username, row.CreatedAt, row.UpdatedAt, row.UpdatedByUserId})
		return s.auditNamed(ctx, q, p, eventbus.RegistryCredentialUpdated, targetRegistry, row.Id, t, "", map[string]string{"Host": row.Host})
	})
	return out, err
}

func (s *Service) RemoveRegistryCredential(ctx context.Context, ref models.Ref, id uuid.UUID) (models.RegistryCredential, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.RegistryCredential{}, err
	}
	var out models.RegistryCredential
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		t, err := s.target(ctx, q, ref, true)
		if err != nil {
			return err
		}
		if _, err := q.GetRegistryCredentialForUpdate(ctx, sqlc.GetRegistryCredentialForUpdateParams{ID: id, OrganizationID: t.OrganizationID, ProjectID: t.ProjectID}); err != nil {
			return notFoundOr(err)
		}
		row, err := q.RemoveRegistryCredential(ctx, sqlc.RemoveRegistryCredentialParams{ID: id, OrganizationID: t.OrganizationID, UserID: p.UserID, Now: s.now()})
		if err != nil {
			return err
		}
		out = registryOf(registryRow{row.Id, row.ProjectId, row.Host, row.Username, row.CreatedAt, row.UpdatedAt, row.UpdatedByUserId})
		return s.auditNamed(ctx, q, p, eventbus.RegistryCredentialRemoved, targetRegistry, row.Id, t, "", map[string]string{"Host": row.Host})
	})
	return out, err
}
