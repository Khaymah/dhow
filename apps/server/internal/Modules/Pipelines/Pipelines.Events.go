package pipelines

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	commitstatus "github.com/Khaymah/dhow/apps/server/internal/Core/CommitStatus"
	jobqueue "github.com/Khaymah/dhow/apps/server/internal/Core/JobQueue"
	runevents "github.com/Khaymah/dhow/apps/server/internal/Core/RunEvents"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
	"github.com/Khaymah/dhow/internal/gitproof"
	"github.com/Khaymah/dhow/internal/pipelinespec"
)

const (
	jobSucceeded   = "succeeded"
	jobCanceled    = "canceled"
	maxOutputs     = 64
	maxOutputSize  = 1 << 10
	maxOutputsJSON = 60 << 10
	reasonRefMoved = "CHECKOUT_REF_MOVED"
)

func runJobOutcome(f runevents.Finished) (string, string) {
	switch f.State {
	case jobSucceeded:
		return models.JobSucceeded, ""
	case jobCanceled:
		return models.JobCanceled, models.ReasonCanceled
	default:
		reason := f.FailureReason
		if reason == "" {
			reason = models.ReasonJobsFailed
		}
		return models.JobFailed, reason
	}
}

func boundedOutputs(in map[string]string) ([]byte, bool) {
	keys := make([]string, 0, len(in))
	for k := range in {
		if k != OutputFound {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := map[string]string{}
	size, dropped := 2, false
	for _, k := range keys {
		v := in[k]
		entry, err := json.Marshal(map[string]string{k: v})
		if err != nil || len(out) >= maxOutputs || len(k) > pipelinespec.MaxContextTextBytes || len(v) > maxOutputSize ||
			size+len(entry) > maxOutputsJSON {
			dropped = true
			continue
		}
		size += len(entry)
		out[k] = v
	}
	raw, err := json.Marshal(out)
	if err != nil || len(raw) > maxOutputsJSON {
		return []byte("{}"), true
	}
	return raw, dropped
}

func (s *Service) JobFinished(ctx context.Context, q *sqlc.Queries, f runevents.Finished) error {
	rj, err := q.GetRunJob(ctx, sqlc.GetRunJobParams{ID: f.OriginID, OrganizationID: f.OrganizationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	run, err := q.LockRun(ctx, rj.RunId)
	if err != nil {
		return err
	}
	if rj, err = q.GetRunJob(ctx, sqlc.GetRunJobParams{ID: f.OriginID, OrganizationID: f.OrganizationID}); err != nil {
		return err
	}
	if rj.State != models.JobActive || rj.CurrentJobId == nil || *rj.CurrentJobId != f.JobID {
		return nil
	}
	state, reason := runJobOutcome(f)
	outputs, dropped := boundedOutputs(f.Outputs)
	if dropped && reason == "" {
		reason = models.ReasonOutputsTruncated
	}
	if _, err := q.FinishRunJob(ctx, sqlc.FinishRunJobParams{
		ID: rj.Id, State: state, FailureReason: reason, Outputs: outputs,
	}); err != nil {
		return err
	}
	if rj.Kind == models.KindDiscovery {
		return s.discovered(ctx, q, run, f)
	}
	if models.RunFinished(run.State) {
		return nil
	}
	return s.resume(ctx, q, run)
}

func (s *Service) resume(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun) error {
	project, err := q.GetRunProject(ctx, sqlc.GetRunProjectParams{ProjectID: run.ProjectId, OrganizationID: run.OrganizationId})
	if err != nil {
		return err
	}
	c, err := s.compileRun(run)
	switch {
	case errors.Is(err, errInvalidPipeline):
		return s.abandon(ctx, q, run, models.ReasonDefinitionChanged)
	case errors.Is(err, errStoredState):
		return s.abandon(ctx, q, run, models.ReasonStateInvalid)
	case err != nil:
		return err
	}
	return s.advance(ctx, q, run, project.Name, c)
}

func (s *Service) abandon(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, reason string) error {
	if err := q.RequestRunCancel(ctx, sqlc.RequestRunCancelParams{ID: run.Id}); err != nil {
		return err
	}
	rows, err := q.ListRunJobs(ctx, run.Id)
	if err != nil {
		return err
	}
	for _, rj := range rows {
		if rj.State == models.JobWaiting {
			if _, err := q.FinishRunJob(ctx, sqlc.FinishRunJobParams{
				ID: rj.Id, State: models.JobCanceled, FailureReason: models.ReasonCanceled, Outputs: []byte("{}"),
			}); err != nil {
				return err
			}
		}
	}
	if err := q.FinishRun(ctx, sqlc.FinishRunParams{ID: run.Id, State: models.RunFailed, FailureReason: reason}); err != nil {
		return err
	}
	return s.recordStatus(ctx, q, run, commitstatus.StateFailed)
}

func (s *Service) discovered(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, f runevents.Finished) error {
	if run.State != models.RunPending {
		return nil
	}
	switch {
	case f.State != jobSucceeded && f.FailureReason == reasonRefMoved:
		return s.endPending(ctx, q, run, models.RunFailed, models.ReasonShaNotOnRef)
	case f.State != jobSucceeded:
		return s.endPending(ctx, q, run, models.RunFailed, models.ReasonDiscoveryFailed)
	}
	claim := f.Outputs[OutputFound]
	if claim != foundTrue && claim != foundFalse {
		return s.endPending(ctx, q, run, models.RunFailed, models.ReasonDiscoveryFailed)
	}
	proof, err := s.readDiscovered(ctx, q, run, f.JobID)
	if err == nil && proof.Found != (claim == foundTrue) {
		err = fmt.Errorf("%w: the job reported dhow_found=%s", gitproof.ErrUnverified, claim)
	}
	if errors.Is(err, gitproof.ErrUnverified) {
		s.log.WarnContext(ctx, "a discovery job handed over git objects that do not prove the pipeline file", "run_id", run.Id.String(), "error", err)
		return s.endPending(ctx, q, run, models.RunFailed, models.ReasonDefinitionUnverified)
	}
	if err != nil {
		return err
	}
	if !proof.Found {
		return s.endPending(ctx, q, run, models.RunSkipped, models.ReasonDefinitionNotFound)
	}
	digest := sha256.Sum256(proof.Source)
	return q.SetRunSource(ctx, sqlc.SetRunSourceParams{ID: run.Id, DefinitionSource: text(string(proof.Source)), DefinitionDigest: digest[:]})
}

func (s *Service) endPending(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, state, reason string) error {
	if err := q.FinishRun(ctx, sqlc.FinishRunParams{ID: run.Id, State: state, FailureReason: reason}); err != nil {
		return err
	}
	if state == models.RunFailed {
		return s.recordStatus(ctx, q, run, commitstatus.StateFailed)
	}
	return nil
}

func (s *Service) launchDiscovery(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun) error {
	spec, err := s.discoverySpec(ctx, q, run)
	switch {
	case errors.Is(err, ErrCheckoutUnavailable):
		return s.endPending(ctx, q, run, models.RunFailed, models.ReasonCheckoutUnavailable)
	case errors.Is(err, errRepositoryUnavailable):
		return s.endPending(ctx, q, run, models.RunFailed, models.ReasonRepositoryUnavailable)
	case err != nil:
		return err
	}
	rj, err := q.InsertRunJob(ctx, sqlc.InsertRunJobParams{
		OrganizationID: run.OrganizationId, RunID: run.Id, Kind: models.KindDiscovery, Key: discoveryKey, JobName: discoveryName,
		JobID: discoveryKey, Matrix: []byte("[]"), Needs: []string{}, Position: 0,
	})
	if err != nil {
		return err
	}
	out, err := s.queue.Enqueue(ctx, q, jobqueue.Request{
		OrganizationID: run.OrganizationId, OriginKind: jobqueue.OriginPipeline, OriginID: rj.Id, Name: discoveryName, Spec: spec,
		Priority: discoveryPriority,
	})
	if reason, refused := refusal(err); refused {
		return s.endPending(ctx, q, run, models.RunFailed, reason)
	}
	if err != nil {
		return err
	}
	_, err = q.ActivateRunJob(ctx, sqlc.ActivateRunJobParams{ID: rj.Id, JobID: &out.JobID})
	return err
}
