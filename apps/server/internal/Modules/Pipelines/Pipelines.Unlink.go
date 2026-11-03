package pipelines

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
)

func (s *Service) Unlinked(ctx context.Context, q *sqlc.Queries, org, project uuid.UUID) error {
	ids, err := q.ListOpenProjectRuns(ctx, sqlc.ListOpenProjectRunsParams{ProjectID: project, OrganizationID: org})
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.cancelLocked(ctx, q, id, nil, models.ReasonRepositoryUnlinked); err != nil {
			return err
		}
	}
	return nil
}

func requireRepository(ctx context.Context, q *sqlc.Queries, org, project uuid.UUID) error {
	row, err := q.GetRunProject(ctx, sqlc.GetRunProjectParams{ProjectID: project, OrganizationID: org})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return apperrors.NotFound()
	case err != nil:
		return err
	case row.RepositoryId == nil:
		return apperrors.RepositoryNotLinked()
	}
	return nil
}

func cancelReason(run sqlc.PipelineRun) string {
	if run.FailureReason != "" {
		return run.FailureReason
	}
	return models.ReasonCanceled
}
