package ingest

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Ingest/Models"
)

func unacknowledged(protected, acknowledged []string) []string {
	out := []string{}
	for _, name := range protected {
		if !slices.Contains(acknowledged, name) {
			out = append(out, name)
		}
	}
	return out
}

func (s *Service) viewsOf(ctx context.Context, q *sqlc.Queries, org, project uuid.UUID, rows []sqlc.IngestToken) ([]models.Token, error) {
	envs, err := s.environmentsOf(ctx, q, org, project)
	if err != nil {
		return nil, err
	}
	creators := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		creators = append(creators, row.CreatedByUserId)
	}
	allowed, err := s.creatorsAllowed(ctx, q, org, project, creators)
	if err != nil {
		return nil, err
	}
	out := make([]models.Token, 0, len(rows))
	for _, row := range rows {
		t, err := s.tokenOf(row)
		if err != nil {
			return nil, err
		}
		var protected []string
		t.Reaches, protected = reachOf(t.RefPatterns, envs)
		t.UnacknowledgedReach = unacknowledged(protected, row.AcknowledgedEnvironments)
		t.CreatorAllowed = allowed[row.CreatedByUserId]
		out = append(out, t)
	}
	return out, nil
}

func (s *Service) checkAcknowledged(ctx context.Context, tok tokenAuth) error {
	envs, err := s.environmentsOf(ctx, s.queries(), tok.row.OrganizationId, tok.row.ProjectId)
	if err != nil {
		return err
	}
	_, protected := reachOf(tok.patterns, envs)
	missing := unacknowledged(protected, tok.row.AcknowledgedEnvironments)
	if len(missing) == 0 {
		return nil
	}
	details := make([]apperrors.IErrorDetail, 0, len(missing))
	for _, name := range missing {
		details = append(details, apperrors.IErrorDetail{Field: apperrors.DetailProtectedReach, Message: name})
	}
	return apperrors.IngestReachNotAcknowledged(details...)
}

func (s *Service) AcknowledgeReach(ctx context.Context, org, project, id uuid.UUID, factor auth.Factor) (models.Token, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Token{}, err
	}
	if p.ViaToken() {
		return models.Token{}, apperrors.SessionRequired()
	}
	if err := s.factors.ConfirmFactor(ctx, p, factor); err != nil {
		return models.Token{}, err
	}
	var out models.Token
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		if err := q.LockIngestTokens(ctx, project.String()); err != nil {
			return err
		}
		row, err := q.GetIngestToken(ctx, sqlc.GetIngestTokenParams{ID: id, OrganizationID: org, ProjectID: project})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound()
		}
		if err != nil {
			return err
		}
		patterns, err := patternsOf(row.RefPatterns)
		if err != nil {
			return err
		}
		envs, err := s.environmentsOf(ctx, q, org, project)
		if err != nil {
			return err
		}
		_, protected := reachOf(patterns, envs)
		row, err = q.AcknowledgeIngestTokenReach(ctx, sqlc.AcknowledgeIngestTokenReachParams{
			ID: id, OrganizationID: org, ProjectID: project, Environments: protected, Now: s.now(),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.Conflict()
		}
		if err != nil {
			return err
		}
		views, err := s.viewsOf(ctx, q, org, project, []sqlc.IngestToken{row})
		if err != nil {
			return err
		}
		out = views[0]
		actor := p.UserID
		return s.audit(ctx, auditEntry{
			name: eventbus.IngestReachAcknowledged, org: org, actor: &actor, ip: auth.RequestMetaFrom(ctx).IPString(),
			target: targetIngestToken, targetID: row.Id, tx: q, data: map[string]string{
				"Name": row.Name, "ProjectId": project.String(), "Environments": cut(strings.Join(protected, " ")),
			},
		})
	})
	return out, err
}
