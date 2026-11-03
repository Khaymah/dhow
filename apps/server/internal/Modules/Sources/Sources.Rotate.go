package sources

import (
	"context"
	"errors"
	"net/url"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources/Models"
)

type change func(q *sqlc.Queries, current sqlc.Repository, u *sqlc.UpdateRepositoryParams) (bool, error)

func (s *Service) change(ctx context.Context, org, project uuid.UUID, apply change) (models.Repository, bool, auth.Principal, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Repository{}, false, p, err
	}
	var row sqlc.Repository
	changed := false
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		if err := activeProject(ctx, q, org, project); err != nil {
			return err
		}
		current, err := q.LockRepositoryByProject(ctx, sqlc.LockRepositoryByProjectParams{ProjectID: project, OrganizationID: org})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.RepositoryNotLinked()
		}
		if err != nil {
			return err
		}
		row = current
		u := updateOf(current, s.now())
		if changed, err = apply(q, current, &u); err != nil || !changed {
			return err
		}
		row, err = q.UpdateRepository(ctx, u)
		return err
	})
	if err != nil {
		return models.Repository{}, false, p, err
	}
	return s.repositoryOf(row), changed, p, nil
}

func (s *Service) ConfirmHostKey(ctx context.Context, org, project uuid.UUID, fingerprint string) (models.Repository, error) {
	var actor uuid.UUID
	if p, ok := auth.PrincipalFrom(ctx); ok {
		actor = p.UserID
	}
	repo, _, p, err := s.change(ctx, org, project, func(_ *sqlc.Queries, current sqlc.Repository, u *sqlc.UpdateRepositoryParams) (bool, error) {
		if !current.PendingHostKey.Valid {
			return false, apperrors.HostKeyNotPending()
		}
		if fingerprintOf(current.PendingHostKey.String) != fingerprint {
			return false, apperrors.HostKeyMismatch()
		}
		u.HostKey, u.HostKeyConfirmedAt, u.HostKeyConfirmedByUserID = current.PendingHostKey, &u.Now, &actor
		u.PendingHostKey, u.PendingHostKeyFetchedAt = pgtype.Text{}, nil
		return true, nil
	})
	if err != nil {
		return models.Repository{}, err
	}
	return repo, s.audit(ctx, p, eventbus.ProjectHostKeyConfirmed, repo, map[string]string{"Fingerprint": repo.HostKeyFingerprint})
}

func (s *Service) RotateDeployKey(ctx context.Context, org, project uuid.UUID) (models.Repository, error) {
	repo, _, p, err := s.change(ctx, org, project, func(q *sqlc.Queries, current sqlc.Repository, u *sqlc.UpdateRepositoryParams) (bool, error) {
		if current.Transport != models.TransportSSH || !current.DeployKeyPublic.Valid {
			return false, apperrors.DeployKeyNotApplicable()
		}
		key, err := s.newDeployKey(ctx, q, current.ProjectId, current.Id)
		if err != nil {
			return false, err
		}
		u.PreviousDeployKeyPublic, u.PreviousDeployKeySealed = current.DeployKeyPublic, current.DeployKeySealed
		u.PreviousDeployKeyExpiresAt = expiresAfter(u.Now)
		u.DeployKeyPublic, u.DeployKeySealed = text(key.public), key.sealed
		return true, nil
	})
	if err != nil {
		return models.Repository{}, err
	}
	return repo, s.audit(ctx, p, eventbus.ProjectDeployKeyRotated, repo, map[string]string{"Fingerprint": fingerprintOf(repo.DeployKeyPublic)})
}

func (s *Service) ConfirmDeployKey(ctx context.Context, org, project uuid.UUID) (models.Repository, error) {
	repo, changed, p, err := s.change(ctx, org, project, func(_ *sqlc.Queries, current sqlc.Repository, u *sqlc.UpdateRepositoryParams) (bool, error) {
		if !current.PreviousDeployKeyPublic.Valid {
			return false, nil
		}
		u.PreviousDeployKeyPublic, u.PreviousDeployKeySealed, u.PreviousDeployKeyExpiresAt = pgtype.Text{}, nil, nil
		return true, nil
	})
	if err != nil || !changed {
		return repo, err
	}
	return repo, s.audit(ctx, p, eventbus.ProjectDeployKeyConfirmed, repo, map[string]string{"Fingerprint": fingerprintOf(repo.DeployKeyPublic)})
}

func (s *Service) RotateWebhookSecret(ctx context.Context, org, project uuid.UUID) (models.Linked, error) {
	var secret string
	repo, _, p, err := s.change(ctx, org, project, func(q *sqlc.Queries, current sqlc.Repository, u *sqlc.UpdateRepositoryParams) (bool, error) {
		if !current.HookId.Valid {
			return false, apperrors.DeployKeyNotApplicable()
		}
		var err error
		if secret, err = newWebhookSecret(); err != nil {
			return false, err
		}
		u.WebhookSecretSealed, err = s.sealWebhookSecret(ctx, q, current.ProjectId, current.Id, secret)
		return err == nil, err
	})
	if err != nil {
		return models.Linked{}, err
	}
	return models.Linked{Repository: repo, WebhookSecret: secret}, s.audit(ctx, p, eventbus.ProjectWebhookSecretRotated, repo, nil)
}

func (s *Service) Unlink(ctx context.Context, org, project uuid.UUID) (models.Repository, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Repository{}, err
	}
	var row sqlc.Repository
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		if err := activeProject(ctx, q, org, project); err != nil {
			return err
		}
		current, err := q.LockRepositoryByProject(ctx, sqlc.LockRepositoryByProjectParams{ProjectID: project, OrganizationID: org})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.RepositoryNotLinked()
		}
		if err != nil {
			return err
		}
		if err := s.triggers.Unlinked(ctx, q, org, project); err != nil {
			return err
		}
		if row, err = q.UnlinkRepository(ctx, sqlc.UnlinkRepositoryParams{ID: current.Id, OrganizationID: org, Now: s.now()}); err != nil {
			return err
		}
		_, err = q.AbandonCommitStatuses(ctx, sqlc.AbandonCommitStatusesParams{RepositoryID: current.Id, OrganizationID: org, Now: s.now()})
		return err
	})
	if err != nil {
		return models.Repository{}, err
	}
	repo := s.repositoryOf(row)
	data := map[string]string{"Transport": repo.Transport}
	if u, err := url.Parse(repo.CloneURL); err == nil && u.Hostname() != "" {
		data["Host"] = u.Hostname()
	}
	return repo, s.audit(ctx, p, eventbus.ProjectRepositoryUnlinked, repo, data)
}
