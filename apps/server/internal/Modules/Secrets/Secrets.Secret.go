package secrets

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
)

func secretOf(row sqlc.Secret, size int32, projects []uuid.UUID, wider map[string][]string) models.Secret {
	if projects == nil {
		projects = []uuid.UUID{}
	}
	return models.Secret{
		Name: row.Name, Scope: row.Scope, Description: row.Description, Version: row.CurrentVersion, SizeBytes: size,
		AvailableToPullRequests: row.AvailableToPullRequests, Visibility: row.Visibility, ProjectIDs: projects,
		Overrides: overridesOf(row.Scope, strings.ToLower(row.Name), wider), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		UpdatedByUserID: row.UpdatedByUserId, LastUsedAt: row.LastUsedAt,
	}
}

func sealContext(id uuid.UUID, version int32) []byte {
	return []byte("secret:" + id.String() + ":v" + itoa(version))
}

func (s *Service) ListSecrets(ctx context.Context, ref models.Ref, page, limit int) (models.Page[models.Secret], error) {
	q := sqlc.New(s.db)
	t, err := s.target(ctx, q, ref, false)
	if err != nil {
		return models.Page[models.Secret]{}, err
	}
	total, err := q.CountSecretsInScope(ctx, sqlc.CountSecretsInScopeParams{
		OrganizationID: t.OrganizationID, ProjectID: t.ProjectID, EnvironmentID: t.EnvironmentID,
	})
	if err != nil {
		return models.Page[models.Secret]{}, err
	}
	rows, err := q.ListSecretsInScope(ctx, sqlc.ListSecretsInScopeParams{
		OrganizationID: t.OrganizationID, ProjectID: t.ProjectID, EnvironmentID: t.EnvironmentID,
		PageLimit: int32(limit), PageOffset: int32((page - 1) * limit), //nolint:gosec // limit and page are bounded by response.PageParams.
	})
	if err != nil {
		return models.Page[models.Secret]{}, err
	}
	wider, err := s.widerSecrets(ctx, q, t)
	if err != nil {
		return models.Page[models.Secret]{}, err
	}
	access, err := s.projectAccess(ctx, q, rows)
	if err != nil {
		return models.Page[models.Secret]{}, err
	}
	items := make([]models.Secret, 0, len(rows))
	for _, r := range rows {
		items = append(items, secretOf(r.Secret, r.SizeBytes, access[r.Secret.Id], wider))
	}
	return models.Page[models.Secret]{Items: items, Total: total}, nil
}

func (s *Service) projectAccess(ctx context.Context, q *sqlc.Queries, rows []sqlc.ListSecretsInScopeRow) (map[uuid.UUID][]uuid.UUID, error) {
	var ids []uuid.UUID
	for _, r := range rows {
		if r.Secret.Visibility == models.VisibilitySelected {
			ids = append(ids, r.Secret.Id)
		}
	}
	out := map[uuid.UUID][]uuid.UUID{}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := q.ListSecretProjectAccess(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, a := range list {
		out[a.SecretId] = append(out[a.SecretId], a.ProjectId)
	}
	return out, nil
}

type writeResult struct {
	row     sqlc.Secret
	size    int32
	created bool
}

func (s *Service) PutSecret(ctx context.Context, ref models.Ref, name string, in models.PutSecretInput) (models.Secret, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Secret{}, err
	}
	if err := checkName(name); err != nil {
		return models.Secret{}, err
	}
	var out models.Secret
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		t, err := s.target(ctx, q, ref, true)
		if err != nil {
			return err
		}
		if err := checkSecretInput(t.Scope, in); err != nil {
			return err
		}
		res, err := s.writeSecret(ctx, q, p, t, name, in, []byte(in.Value))
		if err != nil {
			return err
		}
		if out, err = s.finish(ctx, q, t, res); err != nil {
			return err
		}
		event := eventbus.SecretUpdated
		if res.created {
			event = eventbus.SecretCreated
		}
		return s.auditSecret(ctx, q, p, event, t, res.row, nil)
	})
	return out, err
}

func (s *Service) writeSecret(ctx context.Context, q *sqlc.Queries, p auth.Principal, t models.Target, name string, in models.PutSecretInput, value []byte) (writeResult, error) {
	if err := q.LockSecretsScope(ctx, lockKey("secret", t)); err != nil {
		return writeResult{}, err
	}
	now := s.now()
	row, err := q.GetSecretForUpdate(ctx, sqlc.GetSecretForUpdateParams{
		OrganizationID: t.OrganizationID, ProjectID: t.ProjectID, EnvironmentID: t.EnvironmentID, Name: name,
	})
	created := errors.Is(err, pgx.ErrNoRows)
	switch {
	case created:
		if row, err = s.insertSecret(ctx, q, p, t, name, in); err != nil {
			return writeResult{}, err
		}
	case err != nil:
		return writeResult{}, err
	default:
		if row, err = q.UpdateSecret(ctx, updateParams(row, in, row.CurrentVersion+1, p.UserID, now)); err != nil {
			return writeResult{}, err
		}
	}
	if t.Scope == models.ScopeOrganization && in.Visibility != nil {
		if err := s.setProjects(ctx, q, row, in); err != nil {
			return writeResult{}, err
		}
	}
	size := int32(len(value)) //nolint:gosec // the value is at most 32 KiB.
	if err := s.storeVersion(ctx, q, p, row, value); err != nil {
		return writeResult{}, err
	}
	return writeResult{row: row, size: size, created: created}, nil
}

func (s *Service) insertSecret(ctx context.Context, q *sqlc.Queries, p auth.Principal, t models.Target, name string, in models.PutSecretInput) (sqlc.Secret, error) {
	count, err := q.CountSecretsInScope(ctx, sqlc.CountSecretsInScopeParams{
		OrganizationID: t.OrganizationID, ProjectID: t.ProjectID, EnvironmentID: t.EnvironmentID,
	})
	if err != nil {
		return sqlc.Secret{}, err
	}
	if count >= secretLimit(t.Scope) {
		return sqlc.Secret{}, apperrors.SecretLimitReached()
	}
	visibility := models.VisibilityAll
	if in.Visibility != nil {
		visibility = *in.Visibility
	}
	return q.InsertSecret(ctx, sqlc.InsertSecretParams{
		OrganizationID: t.OrganizationID, ProjectID: t.ProjectID, EnvironmentID: t.EnvironmentID, Scope: t.Scope, Name: name,
		Description: valueOr(in.Description, ""), Visibility: visibility,
		AvailableToPullRequests: valueOr(in.AvailableToPullRequests, false), UserID: p.UserID, Now: s.now(),
	})
}

func updateParams(row sqlc.Secret, in models.PutSecretInput, version int32, user uuid.UUID, now time.Time) sqlc.UpdateSecretParams {
	return sqlc.UpdateSecretParams{
		ID: row.Id, OrganizationID: row.OrganizationId, Description: valueOr(in.Description, row.Description),
		Visibility: valueOr(in.Visibility, row.Visibility), AvailableToPullRequests: valueOr(in.AvailableToPullRequests, row.AvailableToPullRequests),
		CurrentVersion: version, UserID: user, Now: now,
	}
}

func (s *Service) setProjects(ctx context.Context, q *sqlc.Queries, row sqlc.Secret, in models.PutSecretInput) error {
	now := s.now()
	if err := q.ClearSecretProjectAccess(ctx, sqlc.ClearSecretProjectAccessParams{SecretID: row.Id, Now: now}); err != nil {
		return err
	}
	if *in.Visibility != models.VisibilitySelected || in.ProjectIDs == nil || len(*in.ProjectIDs) == 0 {
		return nil
	}
	ids := *in.ProjectIDs
	found, err := q.CountOrganizationProjects(ctx, sqlc.CountOrganizationProjectsParams{OrganizationID: row.OrganizationId, ProjectIds: ids})
	if err != nil {
		return err
	}
	if found != int64(len(ids)) {
		return invalid(fieldProjectIDs, messageInvalid)
	}
	return q.InsertSecretProjectAccess(ctx, sqlc.InsertSecretProjectAccessParams{
		OrganizationID: row.OrganizationId, SecretID: row.Id, ProjectIds: ids, Now: now,
	})
}

func (s *Service) storeVersion(ctx context.Context, q *sqlc.Queries, p auth.Principal, row sqlc.Secret, value []byte) error {
	defer clear(value)
	owner := ownerOf(row.Scope, row.OrganizationId, row.ProjectId, row.EnvironmentId)
	sealed, err := s.secrets.SealIn(ctx, q, owner, sealContext(row.Id, row.CurrentVersion), value)
	if err != nil {
		return err
	}
	now := s.now()
	if err := q.InsertSecretVersion(ctx, sqlc.InsertSecretVersionParams{
		OrganizationID: row.OrganizationId, SecretID: row.Id, Version: row.CurrentVersion, ValueSealed: sealed,
		SizeBytes: int32(len(value)), UserID: p.UserID, Now: now, //nolint:gosec // the value is at most 32 KiB.
	}); err != nil {
		return err
	}
	if err := q.SupersedeSecretVersions(ctx, sqlc.SupersedeSecretVersionsParams{SecretID: row.Id, Version: row.CurrentVersion, Now: now}); err != nil {
		return err
	}
	if oldest := row.CurrentVersion - config.SecretVersionsKept; oldest > 0 {
		_, err = q.EraseSecretVersionsBelow(ctx, sqlc.EraseSecretVersionsBelowParams{SecretID: row.Id, Version: oldest, Now: now})
	}
	return err
}

func (s *Service) finish(ctx context.Context, q *sqlc.Queries, t models.Target, res writeResult) (models.Secret, error) {
	wider, err := s.widerSecrets(ctx, q, t)
	if err != nil {
		return models.Secret{}, err
	}
	var projects []uuid.UUID
	if res.row.Visibility == models.VisibilitySelected {
		list, err := q.ListSecretProjectAccess(ctx, []uuid.UUID{res.row.Id})
		if err != nil {
			return models.Secret{}, err
		}
		for _, a := range list {
			projects = append(projects, a.ProjectId)
		}
	}
	return secretOf(res.row, res.size, projects, wider), nil
}
