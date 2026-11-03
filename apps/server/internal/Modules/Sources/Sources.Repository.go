package sources

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources/Models"
	"github.com/Khaymah/dhow/internal/gitref"
	"github.com/Khaymah/dhow/internal/gitsource"
)

const (
	fieldCloneURL       = "CloneUrl"
	fieldDefaultBranch  = "DefaultBranch"
	fieldHostKey        = "HostKey"
	fieldDeployKey      = "DeployKey"
	projectStateActive  = "active"
	messageInvalidValue = "errors.validationFailed"
)

func invalid(field string) *apperrors.AppError {
	return apperrors.ValidationFailed(apperrors.IErrorDetail{Field: field, Message: messageInvalidValue})
}

func (s *Service) Get(ctx context.Context, org, project uuid.UUID) (models.Repository, error) {
	row, err := s.queries().GetRepositoryByProject(ctx, sqlc.GetRepositoryByProjectParams{ProjectID: project, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Repository{}, apperrors.RepositoryNotLinked()
	}
	if err != nil {
		return models.Repository{}, err
	}
	return s.repositoryOf(row), nil
}

func parseLink(in models.LinkInput) (gitsource.URL, string, *apperrors.AppError) {
	u, err := gitsource.Parse(strings.TrimSpace(in.CloneURL))
	if err != nil {
		return gitsource.URL{}, "", invalid(fieldCloneURL)
	}
	branch := models.DefaultBranch
	if in.DefaultBranch != nil {
		branch = *in.DefaultBranch
		if gitref.CheckName(branch) != nil {
			return gitsource.URL{}, "", invalid(fieldDefaultBranch)
		}
	}
	return u, branch, nil
}

func activeProject(ctx context.Context, q *sqlc.Queries, org, project uuid.UUID) error {
	state, err := q.GetProjectState(ctx, sqlc.GetProjectStateParams{ProjectID: project, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.NotFound()
	}
	if err != nil {
		return err
	}
	if state != projectStateActive {
		return apperrors.ProjectArchived()
	}
	return nil
}

type linkPlan struct {
	org, project uuid.UUID
	url          gitsource.URL
	branch       string
	hostKey      string
	actor        uuid.UUID
	now          time.Time
}

func (s *Service) Link(ctx context.Context, org, project uuid.UUID, in models.LinkInput) (models.Linked, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Linked{}, err
	}
	u, branch, ae := parseLink(in)
	if ae != nil {
		return models.Linked{}, ae
	}
	plan := linkPlan{org: org, project: project, url: u, branch: branch, actor: p.UserID}
	if u.Transport == gitsource.TransportSSH {
		if err := activeProject(ctx, s.queries(), org, project); err != nil {
			return models.Linked{}, err
		}
		if err := s.chargeDial(ctx, org, p.UserID); err != nil {
			return models.Linked{}, err
		}
		key, err := s.fetchHostKey(ctx, u)
		if err != nil {
			return models.Linked{}, err
		}
		plan.hostKey = authorizedLine(key)
	}
	plan.now = s.now()
	var out models.Linked
	var fields []string
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		if err := activeProject(ctx, q, org, project); err != nil {
			return err
		}
		current, err := q.LockRepositoryByProject(ctx, sqlc.LockRepositoryByProjectParams{ProjectID: project, OrganizationID: org})
		if errors.Is(err, pgx.ErrNoRows) {
			out, err = s.insert(ctx, q, plan)
			return err
		}
		if err != nil {
			return err
		}
		var row sqlc.Repository
		row, fields, err = s.relink(ctx, q, current, plan)
		out = models.Linked{Repository: s.repositoryOf(row)}
		return err
	})
	if err != nil {
		return models.Linked{}, err
	}
	data := map[string]string{"Host": u.Host, "Transport": string(u.Transport), "DefaultBranch": branch}
	if fp := out.Repository.PendingHostKeyFingerprint; fp != "" {
		data["Fingerprint"] = fp
	}
	if out.Created {
		return out, s.audit(ctx, p, eventbus.ProjectRepositoryLinked, out.Repository, data)
	}
	if len(fields) == 0 {
		return out, nil
	}
	data["Fields"] = strings.Join(fields, ",")
	return out, s.audit(ctx, p, eventbus.ProjectRepositoryUpdated, out.Repository, data)
}

func (s *Service) insert(ctx context.Context, q *sqlc.Queries, plan linkPlan) (models.Linked, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return models.Linked{}, err
	}
	hookID, err := newHookID()
	if err != nil {
		return models.Linked{}, err
	}
	secret, err := newWebhookSecret()
	if err != nil {
		return models.Linked{}, err
	}
	sealedSecret, err := s.sealWebhookSecret(ctx, q, plan.project, id, secret)
	if err != nil {
		return models.Linked{}, err
	}
	params := sqlc.InsertRepositoryParams{
		ID: id, OrganizationID: plan.org, ProjectID: plan.project, Provider: models.ProviderGeneric, FullName: plan.url.FullName(),
		CloneUrl: plan.url.String(), Transport: string(plan.url.Transport), DefaultBranch: plan.branch, WebhookSecretSealed: sealedSecret,
		HookID: text(hookID), CreatedByUserID: plan.actor, Now: plan.now,
	}
	if plan.url.Transport == gitsource.TransportSSH {
		key, err := s.newDeployKey(ctx, q, plan.project, id)
		if err != nil {
			return models.Linked{}, err
		}
		params.DeployKeyPublic, params.DeployKeySealed = text(key.public), key.sealed
		params.PendingHostKey, params.PendingHostKeyFetchedAt = text(plan.hostKey), &plan.now
	}
	row, err := q.InsertRepository(ctx, params)
	if err != nil {
		return models.Linked{}, err
	}
	return models.Linked{Repository: s.repositoryOf(row), WebhookSecret: secret, Created: true}, nil
}

func updateOf(row sqlc.Repository, now time.Time) sqlc.UpdateRepositoryParams {
	return sqlc.UpdateRepositoryParams{
		ID: row.Id, OrganizationID: row.OrganizationId, FullName: row.FullName, CloneUrl: row.CloneUrl, Transport: row.Transport,
		DefaultBranch: row.DefaultBranch, HostKey: row.HostKey, HostKeyConfirmedAt: row.HostKeyConfirmedAt,
		HostKeyConfirmedByUserID: row.HostKeyConfirmedByUserId, PendingHostKey: row.PendingHostKey,
		PendingHostKeyFetchedAt: row.PendingHostKeyFetchedAt, DeployKeyPublic: row.DeployKeyPublic, DeployKeySealed: row.DeployKeySealed,
		PreviousDeployKeyPublic: row.PreviousDeployKeyPublic, PreviousDeployKeySealed: row.PreviousDeployKeySealed,
		PreviousDeployKeyExpiresAt: row.PreviousDeployKeyExpiresAt, WebhookSecretSealed: row.WebhookSecretSealed, Now: now,
	}
}

func clearHostKey(u *sqlc.UpdateRepositoryParams) {
	u.HostKey, u.HostKeyConfirmedAt, u.HostKeyConfirmedByUserID = pgtype.Text{}, nil, nil
}

func clearSSH(u *sqlc.UpdateRepositoryParams) {
	clearHostKey(u)
	u.PendingHostKey, u.PendingHostKeyFetchedAt = pgtype.Text{}, nil
	u.DeployKeyPublic, u.DeployKeySealed = pgtype.Text{}, nil
	u.PreviousDeployKeyPublic, u.PreviousDeployKeySealed, u.PreviousDeployKeyExpiresAt = pgtype.Text{}, nil, nil
}

func (s *Service) relink(ctx context.Context, q *sqlc.Queries, current sqlc.Repository, plan linkPlan) (sqlc.Repository, []string, error) {
	u := updateOf(current, plan.now)
	u.CloneUrl, u.FullName, u.Transport, u.DefaultBranch = plan.url.String(), plan.url.FullName(), string(plan.url.Transport), plan.branch
	var fields []string
	if u.CloneUrl != current.CloneUrl {
		fields = append(fields, fieldCloneURL)
	}
	if u.DefaultBranch != current.DefaultBranch {
		fields = append(fields, fieldDefaultBranch)
	}
	if plan.url.Transport == gitsource.TransportHTTPS {
		clearSSH(&u)
	} else {
		previous, err := gitsource.Parse(current.CloneUrl)
		if err != nil || current.Transport != string(gitsource.TransportSSH) || previous.HostPort() != plan.url.HostPort() {
			clearHostKey(&u)
		}
		if !u.DeployKeyPublic.Valid {
			key, err := s.newDeployKey(ctx, q, current.ProjectId, current.Id)
			if err != nil {
				return sqlc.Repository{}, nil, err
			}
			u.DeployKeyPublic, u.DeployKeySealed = text(key.public), key.sealed
			fields = append(fields, fieldDeployKey)
		}
		u.PendingHostKey, u.PendingHostKeyFetchedAt = text(plan.hostKey), &plan.now
		if u.HostKey.String == plan.hostKey {
			u.PendingHostKey, u.PendingHostKeyFetchedAt = pgtype.Text{}, nil
		}
	}
	if u.HostKey != current.HostKey || u.PendingHostKey != current.PendingHostKey {
		fields = append(fields, fieldHostKey)
	}
	row, err := q.UpdateRepository(ctx, u)
	return row, fields, err
}
