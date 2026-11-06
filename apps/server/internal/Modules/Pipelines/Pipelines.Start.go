package pipelines

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	commitstatus "github.com/Khaymah/dhow/apps/server/internal/Core/CommitStatus"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
	"github.com/Khaymah/dhow/internal/pipelinespec"
)

type prepared struct {
	source   []byte
	sha      string
	pins     map[string]string
	issues   []models.Issue
	inputs   map[string]any
	compiled compiled
	group    string
	discover bool
	state    string
	reason   string
}

func (p *prepared) finish(state, reason string) *prepared {
	p.state, p.reason = state, reason
	return p
}

func (s *Service) StartPending(ctx context.Context) (int, error) {
	done := 0
	for done < config.PipelineBatch {
		run, err := s.queries().ClaimPendingRun(ctx, sqlc.ClaimPendingRunParams{ClaimedUntil: s.now().Add(config.PipelineClaim), Now: s.now()})
		if errors.Is(err, pgx.ErrNoRows) {
			return done, nil
		}
		if err != nil {
			return done, err
		}
		done++
		if err := s.startRun(ctx, run); err != nil {
			s.isolate(ctx, run.Id, "start", err, nil)
		}
	}
	return done, nil
}

func (s *Service) startRun(ctx context.Context, run sqlc.PipelineRun) error {
	project, err := s.queries().GetRunProject(ctx, sqlc.GetRunProjectParams{ProjectID: run.ProjectId, OrganizationID: run.OrganizationId})
	if err != nil {
		return err
	}
	p, err := s.prepare(ctx, run, project)
	if err != nil {
		return err
	}
	var started bool
	var after sqlc.PipelineRun
	err = s.inTx(ctx, func(q *sqlc.Queries) error {
		var err error
		after, started, err = s.commitStart(ctx, q, run, project, p)
		return err
	})
	if err != nil {
		return err
	}
	if started {
		s.audit(ctx, auditEntry{name: eventbus.RunStarted, run: after, actor: after.TriggeredByUserId})
	}
	return s.afterCancel(ctx)
}

func (s *Service) prepare(ctx context.Context, run sqlc.PipelineRun, project sqlc.GetRunProjectRow) (*prepared, error) {
	pins, err := decodePins(run.ImagePins)
	if err != nil {
		return nil, err
	}
	p := &prepared{pins: pins, sha: run.Sha.String}
	if project.State != projectActive {
		return p.finish(models.RunCanceled, models.ReasonProjectArchived), nil
	}
	if run.DefinitionSource.Valid {
		p.source = []byte(run.DefinitionSource.String)
	} else {
		done, err := s.fetch(ctx, run, p)
		if err != nil || done {
			return p, err
		}
	}
	spec, issues, err := checkSource(p.source)
	p.issues = issues
	if err != nil {
		return p.finish(models.RunFailed, models.ReasonPipelineInvalid), nil
	}
	if !enabled(spec.On, run) {
		return p.finish(models.RunSkipped, models.ReasonTriggerNotEnabled), nil
	}
	given, err := decodeInputs(run.Inputs)
	if err != nil {
		return nil, err
	}
	inputs, bad := checkInputs(spec, given)
	if len(bad) > 0 {
		p.issues = append(p.issues, bad...)
		return p.finish(models.RunFailed, models.ReasonInputsInvalid), nil
	}
	p.inputs = inputs
	if s.pin(ctx, spec, p, pinScope{org: run.OrganizationId, project: run.ProjectId, fork: run.IsFork}) {
		return p.finish(models.RunFailed, models.ReasonImagePinFailed), nil
	}
	c, warnings, err := compileSpec(spec, p.pins)
	if errors.Is(err, errInvalidPipeline) {
		p.issues = append(p.issues, warnings...)
		return p.finish(models.RunFailed, models.ReasonPipelineInvalid), nil
	}
	if err != nil {
		return nil, err
	}
	p.compiled = c
	run.Inputs = encodeInputs(inputs)
	vars, err := s.variablesOf(ctx, nil, run)
	if err != nil {
		return nil, err
	}
	groupCtx, err := evalContext(run, project.Name, nil, vars)
	if err != nil {
		return nil, err
	}
	group, err := c.plan.EvalGroup(ctx, groupCtx)
	if err != nil {
		return p.finish(models.RunFailed, models.ReasonExpressionFailed), nil
	}
	p.group = group
	return p, nil
}

func (s *Service) fetch(ctx context.Context, run sqlc.PipelineRun, p *prepared) (bool, error) {
	ref := run.Ref
	if run.Trigger == triggers.EventArtifact {
		ref, _ = definitionTarget(run)
	}
	req := DefinitionRequest{
		OrganizationID: run.OrganizationId, ProjectID: run.ProjectId, Provider: run.Provider.String, Ref: ref,
		Sha: run.DefinitionSha.String, Path: run.DefinitionPath,
	}
	if run.RepositoryId != nil {
		req.RepositoryID = *run.RepositoryId
	}
	fetchCtx, cancel := context.WithTimeout(ctx, config.PipelineFetchTimeout)
	defer cancel()
	got, err := s.definitions.Fetch(fetchCtx, req)
	switch {
	case errors.Is(err, ErrNeedsDiscovery):
		p.discover = true
		return true, nil
	case errors.Is(err, ErrProviderUnavailable):
		p.finish(models.RunFailed, models.ReasonProviderUnavailable)
		return true, nil
	case err != nil:
		return true, err
	case !got.Found:
		p.finish(models.RunSkipped, models.ReasonDefinitionNotFound)
		return true, nil
	case len(got.Source) > pipelinespec.MaxFileBytes:
		p.issues = []models.Issue{{Code: pipelinespec.CodeFileTooLarge, Severity: string(pipelinespec.SeverityError)}}
		p.finish(models.RunFailed, models.ReasonPipelineInvalid)
		return true, nil
	}
	p.source = got.Source
	if p.sha == "" {
		p.sha = got.Sha
	}
	return false, nil
}

func encodeInputs(in map[string]any) []byte {
	raw, err := json.Marshal(in)
	if err != nil || len(in) == 0 {
		return []byte("{}")
	}
	return raw
}

func (s *Service) commitStart(
	ctx context.Context, q *sqlc.Queries, claimed sqlc.PipelineRun, project sqlc.GetRunProjectRow, p *prepared,
) (sqlc.PipelineRun, bool, error) {
	if p.group != "" && p.state == "" && !p.discover {
		if err := q.LockConcurrencyGroup(ctx, sqlc.LockConcurrencyGroupParams{ProjectID: claimed.ProjectId, ConcurrencyGroup: p.group}); err != nil {
			return claimed, false, err
		}
	}
	run, err := q.LockRun(ctx, claimed.Id)
	if err != nil {
		return run, false, err
	}
	if run.State != models.RunPending || run.DefinitionSource.Valid != claimed.DefinitionSource.Valid {
		return run, false, nil
	}
	if p.state != "" {
		return run, false, s.failStart(ctx, q, run, p)
	}
	if p.discover {
		if err := s.launchDiscovery(ctx, q, run); err != nil {
			return run, false, err
		}
		return run, false, q.ReleaseRunClaim(ctx, run.Id)
	}
	digest := sha256.Sum256(p.source)
	err = q.SetRunDefinition(ctx, sqlc.SetRunDefinitionParams{
		ID: run.Id, DefinitionSource: text(string(p.source)), DefinitionDigest: digest[:], ImagePins: encodePins(p.pins),
		Issues: encodeIssues(p.issues), Inputs: encodeInputs(p.inputs), ConcurrencyGroup: text(p.group),
		CancelInProgress: p.compiled.plan.Concurrency != nil && p.compiled.plan.Concurrency.CancelInProgress,
	})
	if err != nil {
		return run, false, err
	}
	if p.sha != "" && !run.Sha.Valid {
		if err := q.SetRunSource(ctx, sqlc.SetRunSourceParams{
			ID: run.Id, DefinitionSource: text(string(p.source)), DefinitionDigest: digest[:], Sha: text(p.sha),
		}); err != nil {
			return run, false, err
		}
	}
	if run, err = q.LockRun(ctx, run.Id); err != nil {
		return run, false, err
	}
	started, err := s.admit(ctx, q, run, project, p.compiled)
	return run, started, err
}

func (s *Service) failStart(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, p *prepared) error {
	if len(p.issues) > 0 {
		if err := q.SetRunIssues(ctx, sqlc.SetRunIssuesParams{ID: run.Id, Issues: encodeIssues(p.issues)}); err != nil {
			return err
		}
	}
	if err := q.FinishRun(ctx, sqlc.FinishRunParams{ID: run.Id, State: p.state, FailureReason: p.reason}); err != nil {
		return err
	}
	if p.state == models.RunFailed {
		return s.recordStatus(ctx, q, run, commitstatus.StateFailed)
	}
	return nil
}

func (s *Service) admit(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, project sqlc.GetRunProjectRow, c compiled) (bool, error) {
	if run.ConcurrencyGroup.Valid {
		group := run.ConcurrencyGroup.String
		if err := q.LockConcurrencyGroup(ctx, sqlc.LockConcurrencyGroupParams{ProjectID: run.ProjectId, ConcurrencyGroup: group}); err != nil {
			return false, err
		}
		if run.CancelInProgress {
			older, err := q.OlderGroupRuns(ctx, sqlc.OlderGroupRunsParams{
				ProjectID: run.ProjectId, ConcurrencyGroup: group, ID: run.Id, Number: run.Number, Ref: run.Ref,
			})
			if err != nil {
				return false, err
			}
			for _, id := range older {
				if err := s.cancelLocked(ctx, q, id, nil, models.ReasonCanceled); err != nil {
					return false, err
				}
			}
		}
		blocked, err := q.GroupBlocked(ctx, sqlc.GroupBlockedParams{ProjectID: run.ProjectId, ConcurrencyGroup: group, ID: run.Id, Number: run.Number})
		if err != nil {
			return false, err
		}
		if blocked {
			if run.State == models.RunPending {
				return false, q.QueueRun(ctx, run.Id)
			}
			return false, nil
		}
	}
	if err := q.StartRun(ctx, run.Id); err != nil {
		return false, err
	}
	if err := s.expand(ctx, q, run, c); err != nil {
		return false, err
	}
	if err := s.recordStatus(ctx, q, run, commitstatus.StateRunning); err != nil {
		return false, err
	}
	run.State = models.RunRunning
	return true, s.advance(ctx, q, run, project.Name, c)
}

func (s *Service) expand(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, c compiled) error {
	for i, rj := range c.plan.Jobs {
		_, err := q.InsertRunJob(ctx, sqlc.InsertRunJobParams{
			OrganizationID: run.OrganizationId, RunID: run.Id, Kind: models.KindJob, Key: rj.Key, JobName: rj.Name, JobID: rj.JobID,
			Matrix: matrixOf(rj), Needs: nonNil(rj.Needs), Position: int32(i), Digest: rj.Digest,
		})
		if err != nil {
			return fmt.Errorf("expand run job %d: %w", i, err)
		}
	}
	return nil
}

func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
