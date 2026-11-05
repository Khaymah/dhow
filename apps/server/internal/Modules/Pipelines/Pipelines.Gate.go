package pipelines

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	deployments "github.com/Khaymah/dhow/apps/server/internal/Core/Deployments"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
	"github.com/Khaymah/dhow/internal/pipelinespec"
)

const (
	approvalTarget   = "deployment_approval"
	deploymentTarget = "deployment"
)

type passage struct {
	gate     string
	opens    *time.Time
	state    string
	reason   string
	approval *uuid.UUID
}

func (s *Service) publishDeployment(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, name, target string, id uuid.UUID, data map[string]string) error {
	if s.bus == nil {
		return nil
	}
	data["RunId"] = run.Id.String()
	data["ProjectId"] = run.ProjectId.String()
	return s.bus.Publish(ctx, eventbus.Event{Name: name, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{run.OrganizationId}, TargetType: target, TargetID: id.String(), Data: data, Tx: q,
	}})
}

func (s *Service) approve(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, env deployments.Environment) (deployments.Approval, error) {
	a, err := s.gate.Approve(ctx, q, deployments.ApprovalRequest{
		OrganizationID: run.OrganizationId, ProjectID: run.ProjectId, EnvironmentID: env.ID, RunID: run.Id, RunNumber: run.Number,
		Ref: run.Ref, Round: run.Round, RuleVersion: env.RuleVersion, TimeoutHours: env.ApprovalTimeoutHours, SupersedePending: env.SupersedePending,
	})
	if err != nil {
		return a, err
	}
	data := map[string]string{"EnvironmentId": env.ID.String()}
	switch {
	case a.Requested:
		err = s.publishDeployment(ctx, q, run, eventbus.DeploymentApprovalRequested, approvalTarget, a.ID, data)
	case a.Expired:
		err = s.publishDeployment(ctx, q, run, eventbus.DeploymentApprovalExpired, approvalTarget, a.ID, data)
	}
	return a, err
}

func (s *Service) gateOf(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, rj sqlc.RunJob, env deployments.Environment) (passage, error) {
	if env.Paused {
		return passage{gate: deployments.GatePaused, opens: rj.GateOpensAt}, nil
	}
	out := passage{opens: rj.GateOpensAt}
	bypassed := false
	if env.HasReviewers {
		a, err := s.approve(ctx, q, run, env)
		if err != nil {
			return passage{}, err
		}
		switch a.Outcome {
		case deployments.ApprovalWaiting:
			return passage{gate: deployments.GateApproval, opens: rj.GateOpensAt, approval: &a.ID}, nil
		case deployments.ApprovalRejected:
			return passage{state: models.JobFailed, reason: models.ReasonDeploymentRejected}, nil
		case deployments.ApprovalExpired:
			return passage{state: models.JobFailed, reason: models.ReasonApprovalExpired}, nil
		case deployments.ApprovalSuperseded:
			return passage{state: models.JobCanceled, reason: models.ReasonSuperseded}, nil
		}
		bypassed = a.Outcome == deployments.ApprovalBypassed
		out.approval = &a.ID
	}
	if env.WaitMinutes > 0 && !bypassed {
		if out.opens == nil {
			t := s.now().Add(time.Duration(env.WaitMinutes) * time.Minute)
			out.opens = &t
		}
		if s.now().Before(*out.opens) {
			out.gate = deployments.GateTimer
			return out, nil
		}
	}
	if env.LockedByRunID != nil {
		if *env.LockedByRunID != run.Id {
			out.gate = deployments.GateEnvironmentBusy
		}
		return out, nil
	}
	ahead, err := q.EnvironmentQueueAhead(ctx, sqlc.EnvironmentQueueAheadParams{
		EnvironmentID: &env.ID, OrganizationID: run.OrganizationId, RunID: run.Id, Number: run.Number,
	})
	if err != nil {
		return passage{}, err
	}
	if ahead {
		out.gate = deployments.GateEnvironmentBusy
	}
	return out, nil
}

func (s *Service) holdAtGate(ctx context.Context, q *sqlc.Queries, st *step, i int, env deployments.Environment, p passage) error {
	rj := st.rows[i]
	same := rj.Gate.Valid && rj.Gate.String == p.gate && rj.EnvironmentId != nil && *rj.EnvironmentId == env.ID &&
		((p.opens == nil) == (rj.GateOpensAt == nil)) && (p.opens == nil || p.opens.Equal(*rj.GateOpensAt))
	if same {
		return nil
	}
	if err := q.SetRunJobGate(ctx, sqlc.SetRunJobGateParams{
		Gate: text(p.gate), GateOpensAt: p.opens, EnvironmentID: &env.ID, ID: rj.Id, OrganizationID: st.run.OrganizationId,
	}); err != nil {
		return err
	}
	st.rows[i].Gate, st.rows[i].GateOpensAt, st.rows[i].EnvironmentId = text(p.gate), p.opens, &env.ID
	return nil
}

func (s *Service) deployed(ctx context.Context, q *sqlc.Queries, st *step, i int, env deployments.Environment, approval *uuid.UUID) error {
	rj := st.rows[i]
	if rj.CurrentJobId == nil {
		return nil
	}
	if err := s.gate.Started(ctx, q, deployments.Deployment{
		OrganizationID: st.run.OrganizationId, EnvironmentID: env.ID, RunID: st.run.Id, RunNumber: st.run.Number,
		RunJobID: rj.Id, JobID: *rj.CurrentJobId, JobKey: rj.Key, Ref: st.run.Ref, Sha: st.run.Sha.String, RuleVersion: env.RuleVersion,
		ApprovalID: approval,
	}); err != nil {
		return err
	}
	data := map[string]string{"EnvironmentId": env.ID.String(), "Ref": st.run.Ref}
	if approval != nil {
		data["ApprovalId"] = approval.String()
	}
	return s.publishDeployment(ctx, q, st.run, eventbus.DeploymentStarted, deploymentTarget, rj.Id, data)
}

func (s *Service) publishFinished(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, done []deployments.Finished) error {
	for _, d := range done {
		data := map[string]string{"EnvironmentId": d.EnvironmentID.String(), "State": d.State}
		if err := s.publishDeployment(ctx, q, run, eventbus.DeploymentFinished, deploymentTarget, d.DeploymentID, data); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) settleEnvironments(ctx context.Context, q *sqlc.Queries, st *step) error {
	if s.gate == nil || !slices.ContainsFunc(st.c.plan.Jobs, func(j *pipelinespec.RunJob) bool { return j.Environment != "" }) {
		return nil
	}
	keep := []string{}
	for _, rj := range st.rows {
		plan, ok := st.c.byKey[rj.Key]
		if ok && plan.Environment != "" && !models.JobFinished(rj.State) && !slices.Contains(keep, plan.Environment) {
			keep = append(keep, plan.Environment)
		}
	}
	done, err := s.gate.Settle(ctx, q, st.run.OrganizationId, st.run.Id, keep)
	if err != nil {
		return err
	}
	return s.publishFinished(ctx, q, st.run, done)
}

func (s *Service) releaseRun(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun) error {
	if s.gate == nil {
		return nil
	}
	if err := s.gate.CancelApprovals(ctx, q, run.OrganizationId, run.Id); err != nil {
		return err
	}
	done, err := s.gate.Settle(ctx, q, run.OrganizationId, run.Id, nil)
	if err != nil {
		return err
	}
	return s.publishFinished(ctx, q, run, done)
}
