package secrets

import (
	"context"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
)

func (s *Service) lockedSecret(ctx context.Context, q *sqlc.Queries, ref models.Ref, name string) (models.Target, sqlc.Secret, error) {
	if !ValidName(name) {
		return models.Target{}, sqlc.Secret{}, apperrors.NotFound()
	}
	t, err := s.target(ctx, q, ref, true)
	if err != nil {
		return models.Target{}, sqlc.Secret{}, err
	}
	row, err := q.GetSecretForUpdate(ctx, sqlc.GetSecretForUpdateParams{
		OrganizationID: t.OrganizationID, ProjectID: t.ProjectID, EnvironmentID: t.EnvironmentID, Name: name,
	})
	if err != nil {
		return models.Target{}, sqlc.Secret{}, notFoundOr(err)
	}
	return t, row, nil
}

func (s *Service) RemoveSecret(ctx context.Context, ref models.Ref, name string) (models.Secret, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Secret{}, err
	}
	var out models.Secret
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		t, row, err := s.lockedSecret(ctx, q, ref, name)
		if err != nil {
			return err
		}
		current, err := q.GetSecretVersion(ctx, sqlc.GetSecretVersionParams{SecretID: row.Id, Version: row.CurrentVersion})
		if err != nil {
			return err
		}
		if out, err = s.finish(ctx, q, t, writeResult{row: row, size: current.SizeBytes}); err != nil {
			return err
		}
		now := s.now()
		if _, err := q.RemoveSecret(ctx, sqlc.RemoveSecretParams{ID: row.Id, OrganizationID: row.OrganizationId, UserID: p.UserID, Now: now}); err != nil {
			return err
		}
		if _, err := q.EraseAllSecretVersions(ctx, sqlc.EraseAllSecretVersionsParams{SecretID: row.Id, Now: now}); err != nil {
			return err
		}
		if err := q.ClearSecretProjectAccess(ctx, sqlc.ClearSecretProjectAccessParams{SecretID: row.Id, Now: now}); err != nil {
			return err
		}
		return s.auditSecret(ctx, q, p, eventbus.SecretRemoved, t, row, nil)
	})
	return out, err
}

func (s *Service) RestoreSecretVersion(ctx context.Context, ref models.Ref, name string, version int) (models.Secret, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Secret{}, err
	}
	var out models.Secret
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		t, row, err := s.lockedSecret(ctx, q, ref, name)
		if err != nil {
			return err
		}
		if version < 1 || version > int(row.CurrentVersion) {
			return apperrors.NotFound()
		}
		old, err := q.GetSecretVersion(ctx, sqlc.GetSecretVersionParams{SecretID: row.Id, Version: int32(version)}) //nolint:gosec // bounded by CurrentVersion above.
		if err != nil {
			return notFoundOr(err)
		}
		if old.ValueSealed == nil {
			return apperrors.SecretVersionErased()
		}
		owner := ownerOf(row.Scope, row.OrganizationId, row.ProjectId, row.EnvironmentId)
		plain, err := s.secrets.Open(ctx, owner, sealContext(row.Id, old.Version), old.ValueSealed)
		if err != nil {
			return err
		}
		defer clear(plain)
		next, err := q.UpdateSecret(ctx, updateParams(row, models.PutSecretInput{}, row.CurrentVersion+1, p.UserID, s.now()))
		if err != nil {
			return err
		}
		if err := s.storeVersion(ctx, q, p, next, append([]byte(nil), plain...)); err != nil {
			return err
		}
		if out, err = s.finish(ctx, q, t, writeResult{row: next, size: old.SizeBytes}); err != nil {
			return err
		}
		return s.auditSecret(ctx, q, p, eventbus.SecretVersionRestored, t, next, map[string]string{"RestoredFrom": itoa(old.Version)})
	})
	return out, err
}
