package pipelines

import (
	"context"
	"time"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	runevents "github.com/Khaymah/dhow/apps/server/internal/Core/RunEvents"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
)

type ReconcileResult struct {
	Unqueued  int
	Drifted   int
	Stalled   int
	Unsettled int
}

func (r ReconcileResult) Total() int {
	return r.Unqueued + r.Drifted + r.Stalled + r.Unsettled
}

func (s *Service) Reconcile(ctx context.Context) (ReconcileResult, error) {
	var out ReconcileResult
	var err error
	if out.Drifted, err = s.repairDrift(ctx); err != nil {
		return out, err
	}
	if out.Stalled, err = s.failStalled(ctx); err != nil {
		return out, err
	}
	if out.Unsettled, err = s.resumeUnsettled(ctx); err != nil {
		return out, err
	}
	if out.Unqueued, err = s.StartQueued(ctx); err != nil {
		return out, err
	}
	return out, s.afterCancel(ctx)
}

func (s *Service) repairDrift(ctx context.Context) (int, error) {
	rows, err := s.queries().ListDriftedRunJobs(ctx, sqlc.ListDriftedRunJobsParams{
		FinishedBefore: s.now().Add(-config.PipelineDriftGrace), Batch: config.PipelineBatch,
	})
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		err := s.inTx(ctx, func(q *sqlc.Queries) error {
			return s.JobFinished(ctx, q, runevents.Finished{
				OrganizationID: row.OrganizationId, JobID: row.JobId, OriginID: row.Id, State: row.JobState, FailureReason: row.JobFailureReason,
			})
		})
		if err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

func (s *Service) failStalled(ctx context.Context) (int, error) {
	ids, err := s.queries().ListStalledRuns(ctx, sqlc.ListStalledRunsParams{
		CreatedBefore: s.now().Add(-config.PipelineDefinitionTimeout), Batch: config.PipelineBatch,
	})
	if err != nil {
		return 0, err
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
			return 0, err
		}
	}
	return len(ids), nil
}

func (s *Service) resumeUnsettled(ctx context.Context) (int, error) {
	ids, err := s.queries().ListUnsettledRuns(ctx, sqlc.ListUnsettledRunsParams{
		UpdatedBefore: s.now().Add(-config.PipelineDriftGrace), Batch: config.PipelineBatch,
	})
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		err := s.inTx(ctx, func(q *sqlc.Queries) error {
			run, err := q.LockRun(ctx, id)
			if err != nil || run.State != models.RunRunning {
				return err
			}
			return s.resume(ctx, q, run)
		})
		if err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

func (s *Service) StartQueued(ctx context.Context) (int, error) {
	rows, err := s.queries().ListQueuedRuns(ctx, config.PipelineBatch)
	if err != nil {
		return 0, err
	}
	started := 0
	for _, row := range rows {
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
			c, err := compileSource([]byte(run.DefinitionSource.String), decodePins(run.ImagePins))
			if err != nil {
				return s.abandon(ctx, q, run, models.ReasonDefinitionChanged)
			}
			ok, err = s.admit(ctx, q, run, project, c)
			return err
		})
		if err != nil {
			return started, err
		}
		if ok {
			started++
			s.audit(ctx, auditEntry{name: eventbus.RunStarted, run: run, actor: run.TriggeredByUserId})
		}
	}
	return started, nil
}

func (s *Service) RunWorker(ctx context.Context) {
	start := time.NewTicker(config.PipelineStartInterval)
	defer start.Stop()
	reconcile := time.NewTicker(config.PipelineReconcileInterval)
	defer reconcile.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-start.C:
			if _, err := s.StartPending(ctx); err != nil && ctx.Err() == nil {
				s.log.ErrorContext(ctx, "starting pending runs failed; the next pass tries again", "error", err)
			}
		case <-reconcile.C:
			res, err := s.Reconcile(ctx)
			switch {
			case err != nil && ctx.Err() == nil:
				s.log.ErrorContext(ctx, "the run reconciler pass failed", "error", err)
			case res.Total() > 0:
				s.log.InfoContext(ctx, "run reconciler pass", "unqueued", res.Unqueued, "drifted", res.Drifted,
					"stalled", res.Stalled, "unsettled", res.Unsettled)
			}
		}
	}
}
