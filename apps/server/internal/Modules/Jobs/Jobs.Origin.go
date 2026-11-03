package jobs

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs/Gen"
)

func (s *Service) RunOf(ctx context.Context, org, job uuid.UUID) (*gen.JobRun, error) {
	row, err := s.queries().GetJobRun(ctx, sqlc.GetJobRunParams{JobID: job, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &gen.JobRun{ProjectId: row.ProjectId, ProjectName: row.ProjectName, RunId: row.RunId, RunNumber: row.RunNumber}, nil
}

func (s *Service) RunnerNames(ctx context.Context, org uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	rows, err := s.queries().GetRunnerNames(ctx, sqlc.GetRunnerNamesParams{OrganizationID: org, Ids: ids})
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		out[r.Id] = r.Name
	}
	return out, nil
}

func (c *Controller) withOrigin(ctx context.Context, org, job uuid.UUID, data *gen.JobDetail) error {
	run, err := c.service.RunOf(ctx, org, job)
	if err != nil {
		return err
	}
	data.Run = run
	ids := make([]uuid.UUID, 0, len(data.Attempts))
	for _, a := range data.Attempts {
		ids = append(ids, a.RunnerId)
	}
	names, err := c.service.RunnerNames(ctx, org, ids)
	if err != nil {
		return err
	}
	for i, a := range data.Attempts {
		if name, ok := names[a.RunnerId]; ok {
			data.Attempts[i].RunnerName = &name
		}
	}
	return nil
}
