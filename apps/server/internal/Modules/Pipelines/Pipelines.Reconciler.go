package pipelines

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	runevents "github.com/Khaymah/dhow/apps/server/internal/Core/RunEvents"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
	"github.com/Khaymah/dhow/internal/pipelinespec"
)

const (
	pgDataException       = "22"
	pgIntegrityConstraint = "23"
)

var errStoredState = errors.New("the run's stored state cannot be read")

type ReconcileResult struct {
	Unqueued  int
	Drifted   int
	Stalled   int
	Unsettled int
	Failed    int
	Released  int
	Gated     int
}

func (r ReconcileResult) Total() int {
	return r.Unqueued + r.Drifted + r.Stalled + r.Unsettled + r.Failed + r.Released
}

func poisoned(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		class := pgErr.Code[:2]
		return class == pgDataException || class == pgIntegrityConstraint
	}
	return errors.Is(err, errInvalidPipeline) || errors.Is(err, errStoredState) || errors.Is(err, pipelinespec.ErrExpression)
}

func (s *Service) isolate(ctx context.Context, run uuid.UUID, step string, cause error, out *ReconcileResult) {
	if ctx.Err() != nil {
		return
	}
	s.log.ErrorContext(ctx, "a run step failed; the pass goes on with the next run", "step", step, "run_id", run.String(), "error", cause)
	if !poisoned(cause) {
		return
	}
	reason := models.ReasonExpansionFailed
	if errors.Is(cause, errStoredState) {
		reason = models.ReasonStateInvalid
	}
	err := s.inTx(ctx, func(q *sqlc.Queries) error {
		locked, err := q.LockRun(ctx, run)
		if err != nil || models.RunFinished(locked.State) {
			return err
		}
		return s.abandon(ctx, q, locked, reason)
	})
	if err != nil {
		s.log.ErrorContext(ctx, "a run that cannot go on was not marked failed", "run_id", run.String(), "error", err)
		return
	}
	if out != nil {
		out.Failed++
	}
}

func (s *Service) Reconcile(ctx context.Context) (ReconcileResult, error) {
	var out ReconcileResult
	errs := []error{
		s.repairDrift(ctx, &out), s.failStalled(ctx, &out), s.resumeUnsettled(ctx, &out), s.reopenGates(ctx, &out),
		s.releaseStale(ctx, &out), s.startQueued(ctx, &out),
	}
	if err := s.afterCancel(ctx); err != nil {
		errs = append(errs, err)
	}
	return out, errors.Join(errs...)
}

func (s *Service) repairDrift(ctx context.Context, out *ReconcileResult) error {
	rows, err := s.queries().ListDriftedRunJobs(ctx, sqlc.ListDriftedRunJobsParams{
		FinishedBefore: s.now().Add(-config.PipelineDriftGrace), Batch: config.PipelineBatch,
	})
	if err != nil {
		return err
	}
	for _, row := range rows {
		err := s.inTx(ctx, func(q *sqlc.Queries) error {
			return s.JobFinished(ctx, q, runevents.Finished{
				OrganizationID: row.OrganizationId, JobID: row.JobId, OriginID: row.Id, State: row.JobState, FailureReason: row.JobFailureReason,
			})
		})
		if err != nil {
			s.isolate(ctx, row.RunId, "drift", err, out)
			continue
		}
		out.Drifted++
	}
	return nil
}

func (s *Service) failStalled(ctx context.Context, out *ReconcileResult) error {
	ids, err := s.queries().ListStalledRuns(ctx, sqlc.ListStalledRunsParams{
		CreatedBefore: s.now().Add(-config.PipelineDefinitionTimeout), Batch: config.PipelineBatch,
	})
	if err != nil {
		return err
	}
	for _, id := range ids {
		err := s.inTx(ctx, func(q *sqlc.Queries) error {
			run, err := q.LockRun(ctx, id)
			if err != nil || run.State != models.RunPending || run.DefinitionSource.Valid {
				return err
			}
			return s.endPending(ctx, q, run, models.RunFailed, models.ReasonDefinitionTimeout)
		})
		if err != nil {
			s.isolate(ctx, id, "stalled", err, out)
			continue
		}
		out.Stalled++
	}
	return nil
}

func (s *Service) resumeUnsettled(ctx context.Context, out *ReconcileResult) error {
	ids, err := s.queries().ListUnsettledRuns(ctx, sqlc.ListUnsettledRunsParams{
		UpdatedBefore: s.now().Add(-config.PipelineDriftGrace), Batch: config.PipelineBatch,
	})
	if err != nil {
		return err
	}
	for _, id := range ids {
		err := s.inTx(ctx, func(q *sqlc.Queries) error {
			run, err := q.LockRun(ctx, id)
			if err != nil || run.State != models.RunRunning {
				return err
			}
			if err := s.resume(ctx, q, run); err != nil {
				return err
			}
			return q.TouchRun(ctx, id)
		})
		if err != nil {
			s.isolate(ctx, id, "unsettled", err, out)
			continue
		}
		out.Unsettled++
	}
	return nil
}

func (s *Service) reopenGates(ctx context.Context, out *ReconcileResult) error {
	ids, err := s.queries().ListGatedRuns(ctx, sqlc.ListGatedRunsParams{
		UpdatedBefore: s.now().Add(-config.PipelineGateInterval), Batch: config.PipelineBatch,
	})
	if err != nil {
		return err
	}
	for _, id := range ids {
		err := s.inTx(ctx, func(q *sqlc.Queries) error {
			run, err := q.LockRun(ctx, id)
			if err != nil || run.State != models.RunRunning {
				return err
			}
			if err := s.resume(ctx, q, run); err != nil {
				return err
			}
			return q.TouchRun(ctx, id)
		})
		if err != nil {
			s.isolate(ctx, id, "gated", err, out)
			continue
		}
		out.Gated++
	}
	return nil
}

func (s *Service) releaseStale(ctx context.Context, out *ReconcileResult) error {
	if s.gate == nil {
		return nil
	}
	return s.inTx(ctx, func(q *sqlc.Queries) error {
		n, err := s.gate.ReleaseStale(ctx, q)
		out.Released += int(n)
		return err
	})
}

func (s *Service) StartQueued(ctx context.Context) (int, error) {
	var out ReconcileResult
	err := s.startQueued(ctx, &out)
	return out.Unqueued, err
}

func (s *Service) startQueued(ctx context.Context, out *ReconcileResult) error {
	rows, err := s.queries().ListQueuedRuns(ctx, config.PipelineBatch)
	if err != nil {
		return err
	}
	for _, row := range rows {
		run, ok, err := s.startQueuedRun(ctx, row)
		if err != nil {
			s.isolate(ctx, row.Id, "queued", err, out)
			continue
		}
		if ok {
			out.Unqueued++
			s.audit(ctx, auditEntry{name: eventbus.RunStarted, run: run, actor: run.TriggeredByUserId})
		}
	}
	return nil
}

func (s *Service) startQueuedRun(ctx context.Context, row sqlc.ListQueuedRunsRow) (sqlc.PipelineRun, bool, error) {
	var run sqlc.PipelineRun
	var ok bool
	err := s.inTx(ctx, func(q *sqlc.Queries) error {
		if row.ConcurrencyGroup.Valid {
			if err := q.LockConcurrencyGroup(ctx, sqlc.LockConcurrencyGroupParams{
				ProjectID: row.ProjectId, ConcurrencyGroup: row.ConcurrencyGroup.String,
			}); err != nil {
				return err
			}
		}
		var err error
		if run, err = q.LockRun(ctx, row.Id); err != nil || run.State != models.RunQueued {
			return err
		}
		project, err := q.GetRunProject(ctx, sqlc.GetRunProjectParams{ProjectID: run.ProjectId, OrganizationID: run.OrganizationId})
		if err != nil {
			return err
		}
		c, err := s.compileRun(run)
		if err != nil {
			return err
		}
		ok, err = s.admit(ctx, q, run, project, c)
		return err
	})
	return run, ok, err
}

func (s *Service) RunStarter(ctx context.Context) {
	ticker := time.NewTicker(config.PipelineStartInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if _, err := s.StartPending(ctx); err != nil && ctx.Err() == nil {
			s.log.ErrorContext(ctx, "starting pending runs failed; the next pass tries again", "error", err)
		}
	}
}

func (s *Service) RunReconciler(ctx context.Context) {
	ticker := time.NewTicker(config.PipelineReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		res, err := s.Reconcile(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			s.log.ErrorContext(ctx, "the run reconciler pass had errors; it goes on next pass", "error", err)
		case res.Total() > 0:
			s.log.InfoContext(ctx, "run reconciler pass", "unqueued", res.Unqueued, "drifted", res.Drifted,
				"stalled", res.Stalled, "unsettled", res.Unsettled, "failed", res.Failed, "released", res.Released)
		}
	}
}
