package pipelines

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	commitstatus "github.com/Khaymah/dhow/apps/server/internal/Core/CommitStatus"
	deployments "github.com/Khaymah/dhow/apps/server/internal/Core/Deployments"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	jobqueue "github.com/Khaymah/dhow/apps/server/internal/Core/JobQueue"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
	"github.com/Khaymah/dhow/internal/pipelinespec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const projectActive = "active"

var errQueueFull = errors.New("the organization's job queue is full")

type step struct {
	run     sqlc.PipelineRun
	project string
	c       compiled
	rows    []sqlc.RunJob
	byKey   map[string]int
	vars    map[string]string
}

func (st *step) settled(rj sqlc.RunJob) (done, ok bool) {
	for _, need := range rj.Needs {
		i, found := st.byKey[need]
		if !found {
			return true, false
		}
		dep := st.rows[i]
		if !models.JobFinished(dep.State) {
			return false, false
		}
		if !models.Satisfied(dep.State, dep.FailureReason) {
			return true, false
		}
	}
	return true, true
}

func (st *step) busy(jobID string) int {
	n := 0
	for _, rj := range st.rows {
		if rj.JobId == jobID && rj.State == models.JobActive {
			n++
		}
	}
	return n
}

func (s *Service) advance(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, project string, c compiled) error {
	rows, err := q.ListRunJobs(ctx, run.Id)
	if err != nil {
		return err
	}
	vars, err := s.variablesOf(ctx, q, run)
	if err != nil {
		return err
	}
	st := &step{run: run, project: project, c: c, rows: rows, byKey: make(map[string]int, len(rows)), vars: vars}
	for i, rj := range rows {
		st.byKey[rj.Key] = i
	}
	for changed := true; changed; {
		changed = false
		for i := range st.rows {
			moved, err := s.consider(ctx, q, st, i)
			if errors.Is(err, errQueueFull) {
				return s.settle(ctx, q, st)
			}
			if err != nil {
				return err
			}
			changed = changed || moved
		}
	}
	if err := s.settleEnvironments(ctx, q, st); err != nil {
		return err
	}
	return s.settle(ctx, q, st)
}

func (s *Service) consider(ctx context.Context, q *sqlc.Queries, st *step, i int) (bool, error) {
	rj := st.rows[i]
	if rj.Kind != models.KindJob || rj.State != models.JobWaiting {
		return false, nil
	}
	if st.run.CancelRequestedAt != nil {
		return true, s.endRunJob(ctx, q, st, i, models.JobCanceled, models.ReasonCanceled)
	}
	done, ok := st.settled(rj)
	if !done {
		return false, nil
	}
	if !ok {
		return true, s.endRunJob(ctx, q, st, i, models.JobSkipped, models.ReasonNeedsFailed)
	}
	plan, found := st.c.byKey[rj.Key]
	if !found || plan.Digest != rj.Digest {
		return true, s.endRunJob(ctx, q, st, i, models.JobFailed, models.ReasonDefinitionChanged)
	}
	if plan.MaxParallel > 0 && st.busy(rj.JobId) >= plan.MaxParallel {
		return false, nil
	}
	return s.launch(ctx, q, st, i, plan)
}

func (s *Service) endRunJob(ctx context.Context, q *sqlc.Queries, st *step, i int, state, reason string) error {
	if _, err := q.FinishRunJob(ctx, sqlc.FinishRunJobParams{ID: st.rows[i].Id, State: state, FailureReason: reason, Outputs: []byte("{}")}); err != nil {
		return err
	}
	st.rows[i].State, st.rows[i].FailureReason = state, reason
	return nil
}

func (s *Service) environmentFor(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, name string) (*deployments.Environment, string, error) {
	switch {
	case run.IsFork:
		return nil, models.ReasonForkEnvironment, nil
	case s.gate == nil:
		return nil, models.ReasonEnvironments, nil
	}
	env, found, err := s.lockEnvironment(ctx, q, run, name)
	if err != nil {
		return nil, "", err
	}
	if !found {
		return nil, models.ReasonEnvironmentNotFound, nil
	}
	return &env, "", nil
}

func (s *Service) launch(ctx context.Context, q *sqlc.Queries, st *step, i int, plan *pipelinespec.RunJob) (bool, error) {
	var env *deployments.Environment
	vars := st.vars
	if plan.Environment != "" {
		e, reason, err := s.environmentFor(ctx, q, st.run, plan.Environment)
		if err != nil {
			return false, err
		}
		if reason != "" {
			state := models.JobSkipped
			if reason == models.ReasonEnvironmentNotFound {
				state = models.JobFailed
			}
			return true, s.endRunJob(ctx, q, st, i, state, reason)
		}
		env = e
		if vars, err = s.environmentVars(ctx, q, st.run, env.ID); err != nil {
			return false, err
		}
	}
	needs, err := needsContext(st.rows)
	if err != nil {
		return false, err
	}
	evalCtx, err := evalContext(st.run, st.project, needs, vars)
	if err != nil {
		return false, err
	}
	run, err := plan.EvalIf(ctx, evalCtx)
	if err != nil {
		return true, s.endRunJob(ctx, q, st, i, models.JobFailed, models.ReasonExpressionFailed)
	}
	if !run {
		return true, s.endRunJob(ctx, q, st, i, models.JobSkipped, models.ReasonConditionFalse)
	}
	if env != nil && !env.RefAllowed {
		return true, s.endWithDetail(ctx, q, st, i, models.JobSkipped, models.ReasonEnvironmentRef, refDetail(st.run, env.Name))
	}
	spec, err := plan.Build(ctx, evalCtx)
	if errors.Is(err, pipelinespec.ErrSecretScope) {
		s.log.ErrorContext(ctx, "a built job spec gives a step a secret it did not declare", "run_job_id", st.rows[i].Id.String())
		return true, s.endRunJob(ctx, q, st, i, models.JobFailed, models.ReasonJobSpecInvalid)
	}
	if err != nil {
		return true, s.endRunJob(ctx, q, st, i, models.JobFailed, models.ReasonExpressionFailed)
	}
	if len(spec.GetSteps()) == 0 {
		return true, s.endRunJob(ctx, q, st, i, models.JobSkipped, models.ReasonConditionFalse)
	}
	return s.admitJob(ctx, q, st, i, plan, spec, evalCtx, env)
}

func (s *Service) admitJob(
	ctx context.Context, q *sqlc.Queries, st *step, i int, plan *pipelinespec.RunJob, spec *runnerv1.JobSpec,
	evalCtx pipelinespec.Context, env *deployments.Environment,
) (bool, error) {
	var envID *uuid.UUID
	if env != nil {
		pins, err := decodePins(st.run.ImagePins)
		if err != nil {
			return false, err
		}
		if reason := staticRefusal(*env, spec, pins); reason != "" {
			return true, s.endRunJob(ctx, q, st, i, models.JobFailed, reason)
		}
		envID = &env.ID
	}
	if st.run.IsFork && (len(spec.GetSecrets()) > 0 || hasPluginSecrets(plan)) {
		return true, s.endRunJob(ctx, q, st, i, models.JobFailed, models.ReasonForkSecrets)
	}
	if err := s.attachRegistryAuth(ctx, q, st.run, spec); err != nil {
		return false, err
	}
	if reason, detail, err := s.checkSecrets(ctx, q, st.run, spec, envID); err != nil || reason != "" {
		if err != nil {
			return false, err
		}
		return true, s.endWithDetail(ctx, q, st, i, models.JobFailed, reason, detail)
	}
	var approval *uuid.UUID
	if env != nil {
		p, err := s.gateOf(ctx, q, st.run, st.rows[i], *env)
		if err != nil {
			return false, err
		}
		if p.state != "" {
			return true, s.endRunJob(ctx, q, st, i, p.state, p.reason)
		}
		if p.gate != "" {
			return false, s.holdAtGate(ctx, q, st, i, *env, p)
		}
		approval = p.approval
	}
	rj := st.rows[i]
	if reason, err := s.attachTransfers(ctx, q, st, rj, plan, spec, evalCtx); err != nil || reason != "" {
		if err != nil {
			return false, err
		}
		return true, s.endRunJob(ctx, q, st, i, models.JobFailed, reason)
	}
	reason, err := s.enqueue(ctx, q, st, rj, plan, spec, envID)
	if err != nil {
		return false, err
	}
	if reason != "" {
		return true, s.endRunJob(ctx, q, st, i, models.JobFailed, reason)
	}
	if env != nil {
		return true, s.deployed(ctx, q, st, i, *env, approval)
	}
	return true, nil
}

func (s *Service) endWithDetail(ctx context.Context, q *sqlc.Queries, st *step, i int, state, reason string, detail []byte) error {
	if err := s.endRunJob(ctx, q, st, i, state, reason); err != nil {
		return err
	}
	if len(detail) == 0 {
		return nil
	}
	return q.SetRunJobFailureDetail(ctx, sqlc.SetRunJobFailureDetailParams{FailureDetail: detail, ID: st.rows[i].Id, OrganizationID: st.run.OrganizationId})
}

func hasPluginSecrets(plan *pipelinespec.RunJob) bool {
	return slices.ContainsFunc(plan.Steps, func(sp pipelinespec.StepPlan) bool { return sp.Plugin != nil && len(sp.Plugin.Secrets) > 0 })
}

func jobName(run sqlc.PipelineRun, rj sqlc.RunJob) string {
	name := []rune("#" + strconv.FormatInt(run.Number, 10) + " " + rj.JobName)
	if len(name) > config.JobNameMaxLength {
		name = name[:config.JobNameMaxLength]
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(string(name)))
}

func (s *Service) enqueue(
	ctx context.Context, q *sqlc.Queries, st *step, rj sqlc.RunJob, plan *pipelinespec.RunJob, spec *runnerv1.JobSpec, env *uuid.UUID,
) (string, error) {
	creator := uuid.Nil
	if st.run.TriggeredByUserId != nil {
		creator = *st.run.TriggeredByUserId
		member, err := q.RunCreatorIsMember(ctx, sqlc.RunCreatorIsMemberParams{UserID: creator, OrganizationID: st.run.OrganizationId})
		if err != nil {
			return "", err
		}
		if !member {
			return models.ReasonCreatorNotMember, nil
		}
	}
	co, secrets, err := s.checkoutFor(ctx, q, st.run, st.run.Ref, st.run.Sha.String)
	if errors.Is(err, errRepositoryUnavailable) {
		return models.ReasonRepositoryUnavailable, nil
	}
	if err != nil {
		return "", err
	}
	spec.Checkout = co
	spec.Secrets = append(spec.Secrets, secrets...)
	out, err := s.queue.Enqueue(ctx, q, jobqueue.Request{
		OrganizationID: st.run.OrganizationId, CreatedByUserID: creator, OriginKind: jobqueue.OriginPipeline, OriginID: rj.Id,
		Name: jobName(st.run, rj), Spec: spec, RequiredFeatures: planFeatures(plan),
	})
	if reason, refused := refusal(err); refused {
		return reason, nil
	}
	var ae *apperrors.AppError
	if errors.As(err, &ae) && ae.Code == apperrors.CodeJobQueueFull {
		return "", errQueueFull
	}
	if err != nil {
		return "", err
	}
	if _, err := q.ActivateRunJob(ctx, sqlc.ActivateRunJobParams{ID: rj.Id, JobID: &out.JobID, EnvironmentID: env}); err != nil {
		return "", err
	}
	i := st.byKey[rj.Key]
	st.rows[i].State, st.rows[i].CurrentJobId = models.JobActive, &out.JobID
	return "", nil
}

func planFeatures(plan *pipelinespec.RunJob) []string {
	var out []string
	if slices.ContainsFunc(plan.Steps, func(sp pipelinespec.StepPlan) bool { return sp.Plugin != nil }) {
		out = append(out, jobqueue.FeaturePlugins)
	}
	if len(plan.Artifacts) > 0 {
		out = append(out, jobqueue.FeatureArtifacts)
	}
	if plan.Cache != nil {
		out = append(out, jobqueue.FeatureCache)
	}
	return out
}

func refusal(err error) (string, bool) {
	var ae *apperrors.AppError
	if err == nil || !errors.As(err, &ae) {
		return "", false
	}
	switch ae.Code {
	case apperrors.CodeJobQueueFull:
		return "", false
	case apperrors.CodeSecretsUnavailable:
		return models.ReasonSecretsUnavailable, true
	case apperrors.CodeValidationFailed:
		return models.ReasonJobSpecInvalid, true
	default:
		return models.ReasonEnqueueRefused, true
	}
}

func (s *Service) settle(ctx context.Context, q *sqlc.Queries, st *step) error {
	if models.RunFinished(st.run.State) {
		return nil
	}
	failed, canceled := false, false
	for _, rj := range st.rows {
		if !models.JobFinished(rj.State) {
			return nil
		}
		failed = failed || rj.State == models.JobFailed
		canceled = canceled || rj.State == models.JobCanceled
	}
	state, reason, status := models.RunSucceeded, "", commitstatus.StateSuccess
	switch {
	case failed:
		state, reason, status = models.RunFailed, models.ReasonJobsFailed, commitstatus.StateFailed
	case canceled || st.run.CancelRequestedAt != nil:
		state, reason, status = models.RunCanceled, cancelReason(st.run), commitstatus.StateCanceled
	}
	if err := q.FinishRun(ctx, sqlc.FinishRunParams{ID: st.run.Id, State: state, FailureReason: reason}); err != nil {
		return err
	}
	st.run.State = state
	return s.recordStatus(ctx, q, st.run, status)
}
