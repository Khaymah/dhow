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
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Ingest/Models"
)

func canAcknowledge(p auth.Principal) bool {
	return !p.ViaToken() && policy.Allows(p.ProjectRole, policy.EnvironmentManage)
}

func namesOf(envs []reachEnv) []string {
	out := make([]string, 0, len(envs))
	for _, e := range envs {
		out = append(out, e.name)
	}
	return out
}

func idsOf(envs []reachEnv) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(envs))
	for _, e := range envs {
		out = append(out, e.id)
	}
	return out
}

func unacknowledged(protected []reachEnv, acknowledged []uuid.UUID) []reachEnv {
	out := []reachEnv{}
	for _, e := range protected {
		if !slices.Contains(acknowledged, e.id) {
			out = append(out, e)
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
		var protected []reachEnv
		t.Reaches, protected = reachOf(t.RefPatterns, envs)
		t.UnacknowledgedReach = namesOf(unacknowledged(protected, row.AcknowledgedEnvironmentIds))
		t.CreatorAllowed = allowed[row.CreatedByUserId]
		out = append(out, t)
	}
	return out, nil
}

func (s *Service) checkAcknowledged(ctx context.Context, q *sqlc.Queries, tok tokenAuth, acknowledged []uuid.UUID) error {
	envs, err := s.environmentsOf(ctx, q, tok.row.OrganizationId, tok.row.ProjectId)
	if err != nil {
		return err
	}
	_, protected := reachOf(tok.patterns, envs)
	if len(unacknowledged(protected, acknowledged)) == 0 {
		return nil
	}
	return apperrors.IngestReachNotAcknowledged()
}

func sameNames(a, b []string) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

func (s *Service) AcknowledgeReach(
	ctx context.Context, org, project, id uuid.UUID, expected []string, factor auth.Factor,
) (models.Token, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Token{}, err
	}
	if p.ViaToken() {
		return models.Token{}, apperrors.SessionRequired()
	}
	if !canAcknowledge(p) {
		return models.Token{}, apperrors.IngestReachNeedsAdmin()
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
		if !sameNames(expected, namesOf(unacknowledged(protected, row.AcknowledgedEnvironmentIds))) {
			return apperrors.IngestReachChanged()
		}
		row, err = q.AcknowledgeIngestTokenReach(ctx, sqlc.AcknowledgeIngestTokenReachParams{
			ID: id, OrganizationID: org, ProjectID: project, EnvironmentIds: idsOf(protected), Now: s.now(),
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
				"Name": row.Name, "ProjectId": project.String(), "Environments": cut(strings.Join(namesOf(protected), " ")),
			},
		})
	})
	return out, err
}
