package pipelines

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	commitstatus "github.com/Khaymah/dhow/apps/server/internal/Core/CommitStatus"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	jobqueue "github.com/Khaymah/dhow/apps/server/internal/Core/JobQueue"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
)

func runJobOf(rj sqlc.RunJob) (models.RunJob, error) {
	matrix, err := matrixMap(rj.Matrix)
	if err != nil {
		return models.RunJob{}, err
	}
	return models.RunJob{
		ID: rj.Id, Key: rj.Key, JobID: rj.JobId, Name: rj.JobName, Matrix: matrix, Needs: rj.Needs, State: rj.State,
		FailureReason: rj.FailureReason, CurrentJobID: rj.CurrentJobId, Attempts: int(rj.Attempts), StartedAt: rj.StartedAt,
		FinishedAt: rj.FinishedAt,
	}, nil
}

func (s *Service) detail(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun) (models.Detail, error) {
	rows, err := q.ListRunJobs(ctx, run.Id)
	if err != nil {
		return models.Detail{}, err
	}
	issues, err := decodeIssues(run.Issues)
	if err != nil {
		return models.Detail{}, err
	}
	pins, err := decodePins(run.ImagePins)
	if err != nil {
		return models.Detail{}, err
	}
	out := models.Detail{Run: runOf(run), Jobs: make([]models.RunJob, 0, len(rows)), Issues: issues, ImagePins: pinList(pins)}
	for _, rj := range rows {
		j, err := runJobOf(rj)
		if err != nil {
			return models.Detail{}, err
		}
		out.Jobs = append(out.Jobs, j)
	}
	return out, nil
}

func (s *Service) loadRun(ctx context.Context, q *sqlc.Queries, org, project, id uuid.UUID) (sqlc.PipelineRun, error) {
	run, err := q.GetRun(ctx, sqlc.GetRunParams{ID: id, OrganizationID: org, ProjectID: project})
	if errors.Is(err, pgx.ErrNoRows) {
		return run, apperrors.NotFound()
	}
	return run, err
}

func (s *Service) Get(ctx context.Context, org, project, id uuid.UUID) (models.Detail, error) {
	q := s.queries()
	run, err := s.loadRun(ctx, q, org, project, id)
	if err != nil {
		return models.Detail{}, err
	}
	return s.detail(ctx, q, run)
}

func (s *Service) List(ctx context.Context, org, project uuid.UUID, states []string, ref string, page, limit int) (models.Page, error) {
	q := s.queries()
	total, err := q.CountRuns(ctx, sqlc.CountRunsParams{ProjectID: project, OrganizationID: org, States: nonNil(states), Ref: ref})
	if err != nil {
		return models.Page{}, err
	}
	rows, err := q.ListRuns(ctx, sqlc.ListRunsParams{
		ProjectID: project, OrganizationID: org, States: nonNil(states), Ref: ref,
		PageLimit: int32(limit), PageOffset: int32((page - 1) * limit), //nolint:gosec // limit and page are bounded by response.PageParams.
	})
	if err != nil {
		return models.Page{}, err
	}
	out := models.Page{Items: make([]models.Run, 0, len(rows)), Total: total}
	for _, r := range rows {
		out.Items = append(out.Items, runOf(r))
	}
	return out, nil
}

func (s *Service) cancelLocked(ctx context.Context, q *sqlc.Queries, id uuid.UUID, user *uuid.UUID) error {
	run, err := q.LockRun(ctx, id)
	if err != nil || models.RunFinished(run.State) {
		return err
	}
	if err := q.RequestRunCancel(ctx, sqlc.RequestRunCancelParams{ID: id, UserID: user}); err != nil {
		return err
	}
	rows, err := q.ListRunJobs(ctx, id)
	if err != nil {
		return err
	}
	active := false
	for _, rj := range rows {
		switch rj.State {
		case models.JobWaiting:
			if _, err := q.FinishRunJob(ctx, sqlc.FinishRunJobParams{
				ID: rj.Id, State: models.JobCanceled, FailureReason: models.ReasonCanceled, Outputs: []byte("{}"),
			}); err != nil {
				return err
			}
		case models.JobActive:
			active = true
		}
	}
	if active && run.State == models.RunRunning {
		return nil
	}
	if err := q.FinishRun(ctx, sqlc.FinishRunParams{ID: id, State: models.RunCanceled, FailureReason: models.ReasonCanceled}); err != nil {
		return err
	}
	return s.recordStatus(ctx, q, run, commitstatus.StateCanceled)
}

func (s *Service) Cancel(ctx context.Context, org, project, id uuid.UUID) (models.Detail, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Detail{}, err
	}
	var before, after sqlc.PipelineRun
	err = s.inTx(ctx, func(q *sqlc.Queries) error {
		var err error
		if before, err = s.loadRun(ctx, q, org, project, id); err != nil {
			return err
		}
		if err := s.cancelLocked(ctx, q, id, &p.UserID); err != nil {
			return err
		}
		after, err = q.LockRun(ctx, id)
		return err
	})
	if err != nil {
		return models.Detail{}, err
	}
	if !models.RunFinished(before.State) && before.CancelRequestedAt == nil {
		s.auditByUser(ctx, eventbus.RunCancelRequested, after, p)
	}
	if err := s.afterCancel(ctx); err != nil {
		return models.Detail{}, err
	}
	return s.Get(ctx, org, project, id)
}

func (s *Service) afterCancel(ctx context.Context) error {
	rows, err := s.queries().ListRunCancellations(ctx, config.PipelineBatch)
	if err != nil {
		return err
	}
	for _, row := range rows {
		by := uuid.Nil
		if row.CancelRequestedBy != nil {
			by = *row.CancelRequestedBy
		}
		err := s.inTx(ctx, func(q *sqlc.Queries) error {
			_, err := s.queue.Cancel(ctx, q, jobqueue.Cancellation{OrganizationID: row.OrganizationId, JobID: row.JobId, RequestedBy: by})
			return err
		})
		if err != nil && ctx.Err() == nil {
			s.log.ErrorContext(ctx, "a job of a canceled run was not canceled yet; the reconciler tries again",
				"job_id", row.JobId.String(), "error", err)
		}
	}
	return nil
}

func rerunnable(rj sqlc.RunJob) bool {
	if rj.Kind != models.KindJob {
		return false
	}
	return rj.State == models.JobFailed || rj.State == models.JobCanceled ||
		(rj.State == models.JobSkipped && rj.FailureReason == models.ReasonNeedsFailed)
}

func (s *Service) Rerun(ctx context.Context, org, project, id uuid.UUID) (models.Detail, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Detail{}, err
	}
	var after sqlc.PipelineRun
	err = s.inTx(ctx, func(q *sqlc.Queries) error {
		if _, err := s.loadRun(ctx, q, org, project, id); err != nil {
			return err
		}
		run, err := q.LockRun(ctx, id)
		if err != nil {
			return err
		}
		rows, err := q.ListRunJobs(ctx, id)
		if err != nil {
			return err
		}
		if (run.State != models.RunFailed && run.State != models.RunCanceled) || !run.DefinitionSource.Valid ||
			!slices.ContainsFunc(rows, rerunnable) {
			return apperrors.RunNotRerunnable()
		}
		for _, rj := range rows {
			if rerunnable(rj) {
				if err := q.ResetRunJob(ctx, rj.Id); err != nil {
					return err
				}
			}
		}
		if err := q.ReopenRun(ctx, id); err != nil {
			return err
		}
		if after, err = q.LockRun(ctx, id); err != nil {
			return err
		}
		if err := s.recordStatus(ctx, q, after, commitstatus.StateRunning); err != nil {
			return err
		}
		return s.resume(ctx, q, after)
	})
	if err != nil {
		return models.Detail{}, err
	}
	s.auditByUser(ctx, eventbus.RunRerunRequested, after, p)
	return s.Get(ctx, org, project, id)
}

func (s *Service) Approve(ctx context.Context, org, project, id uuid.UUID) (models.Detail, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Detail{}, err
	}
	var after sqlc.PipelineRun
	err = s.inTx(ctx, func(q *sqlc.Queries) error {
		if _, err := s.loadRun(ctx, q, org, project, id); err != nil {
			return err
		}
		run, err := q.LockRun(ctx, id)
		if err != nil {
			return err
		}
		if run.State != models.RunAwaitingApproval {
			return apperrors.RunNotAwaitingApproval()
		}
		if err := q.ApproveRun(ctx, sqlc.ApproveRunParams{ID: id, UserID: &p.UserID}); err != nil {
			return err
		}
		after, err = q.LockRun(ctx, id)
		return err
	})
	if err != nil {
		return models.Detail{}, err
	}
	s.auditByUser(ctx, eventbus.RunApproved, after, p)
	return s.Get(ctx, org, project, id)
}
