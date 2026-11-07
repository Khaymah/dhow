package runners

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Runners/Models"
)

const (
	timePrecision = time.Microsecond
	timeFormat    = time.RFC3339
)

func (s *Service) List(ctx context.Context, org uuid.UUID, page, limit int) ([]models.Runner, int64, error) {
	q := s.queries()
	total, err := q.CountRunners(ctx, org)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListRunners(ctx, sqlc.ListRunnersParams{
		OrganizationID: org, PageLimit: int32(limit), PageOffset: int32((page - 1) * limit), //nolint:gosec // limit and page are bounded by response.PageParams.
	})
	if err != nil {
		return nil, 0, err
	}
	now := s.now()
	out := make([]models.Runner, 0, len(rows))
	for _, r := range rows {
		out = append(out, runnerOf(r, now))
	}
	return out, total, nil
}

func (s *Service) Get(ctx context.Context, org, id uuid.UUID) (models.Runner, error) {
	row, err := s.queries().GetRunner(ctx, sqlc.GetRunnerParams{ID: id, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Runner{}, apperrors.NotFound()
	}
	if err != nil {
		return models.Runner{}, err
	}
	return runnerOf(row, s.now()), nil
}

func (s *Service) Update(ctx context.Context, org, id uuid.UUID, in models.UpdateInput) (models.Runner, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Runner{}, err
	}
	if ae := validateUpdate(&in); ae != nil {
		return models.Runner{}, ae
	}
	now := s.now()
	params := sqlc.UpdateRunnerParams{ID: id, OrganizationID: org, Now: now}
	if in.Name != nil {
		params.Name = pgtype.Text{String: *in.Name, Valid: true}
	}
	if in.Disabled != nil {
		params.Disabled = pgtype.Bool{Bool: *in.Disabled, Valid: true}
	}
	if in.Labels != nil {
		params.Labels = *in.Labels
	}
	row, err := s.queries().UpdateRunner(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Runner{}, s.missingOrRevoked(ctx, org, id)
	}
	if err != nil {
		return models.Runner{}, err
	}
	data := map[string]string{}
	if in.Name != nil {
		data["Name"] = *in.Name
	}
	if in.Labels != nil {
		data["Labels"] = strings.Join(*in.Labels, ",")
	}
	if in.Disabled != nil {
		data["Disabled"] = strconv.FormatBool(*in.Disabled)
	}
	if err := s.publishByUser(ctx, nil, eventbus.RunnerUpdated, p, org, id, data); err != nil {
		return models.Runner{}, err
	}
	return runnerOf(row, now), nil
}

func (s *Service) missingOrRevoked(ctx context.Context, org, id uuid.UUID) error {
	row, err := s.queries().GetRunner(ctx, sqlc.GetRunnerParams{ID: id, OrganizationID: org})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return apperrors.NotFound()
	case err != nil:
		return err
	case row.State == models.StateRevoked:
		return apperrors.RunnerRevoked()
	default:
		return apperrors.Conflict()
	}
}

func (s *Service) Revoke(ctx context.Context, org, id uuid.UUID) (models.Runner, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Runner{}, err
	}
	now := s.now()
	var row sqlc.Runner
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		r, err := q.RevokeRunner(ctx, sqlc.RevokeRunnerParams{ID: id, OrganizationID: org, Now: now})
		if errors.Is(err, pgx.ErrNoRows) {
			row, err = q.GetRunner(ctx, sqlc.GetRunnerParams{ID: id, OrganizationID: org})
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.NotFound()
			}
			return err
		}
		if err != nil {
			return err
		}
		row = r
		if _, err = q.RevokeRunnerSessions(ctx, sqlc.RevokeRunnerSessionsParams{RunnerID: id, Now: now}); err != nil {
			return err
		}
		return s.publishByUser(ctx, q, eventbus.RunnerRevoked, p, org, id, map[string]string{"Name": row.Name})
	})
	if err != nil {
		return models.Runner{}, err
	}
	return runnerOf(row, now), nil
}

func (s *Service) publishByUser(ctx context.Context, q *sqlc.Queries, name string, p auth.Principal, org, id uuid.UUID, data map[string]string) error {
	actor := p.UserID
	return s.bus.Publish(ctx, eventbus.Event{Name: name, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{org}, ActorUserID: &actor, TargetType: "runner", TargetID: id.String(),
		IP: auth.RequestMetaFrom(ctx).IPString(), Data: data, Tx: q,
	}})
}
