package environments

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Gen"
)

const (
	messageApprovalsListed = "approvals.list.loaded"
	messageApproved        = "approvals.approved"
	messageRejected        = "approvals.rejected"
	messageBypassed        = "approvals.bypassed"
	pendingListMax         = 100
)

type ApprovalView struct {
	Row         sqlc.DeploymentApproval
	Environment string
	Project     uuid.UUID
}

func (s *Service) RunApprovals(ctx context.Context, org, project, run uuid.UUID) ([]ApprovalView, error) {
	rows, err := s.queries().ListRunApprovals(ctx, sqlc.ListRunApprovalsParams{RunID: run, OrganizationID: org, ProjectID: project})
	if err != nil {
		return nil, err
	}
	out := make([]ApprovalView, 0, len(rows))
	for _, r := range rows {
		out = append(out, ApprovalView{Row: approvalOfRun(r), Environment: r.EnvironmentName, Project: project})
	}
	return out, nil
}

func approvalOfRun(r sqlc.ListRunApprovalsRow) sqlc.DeploymentApproval {
	return sqlc.DeploymentApproval{
		Id: r.Id, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, OrganizationId: r.OrganizationId, EnvironmentId: r.EnvironmentId,
		RunId: r.RunId, RunNumber: r.RunNumber, Ref: r.Ref, Round: r.Round, RuleVersion: r.RuleVersion, State: r.State,
		ExpiresAt: r.ExpiresAt, DecidedByUserId: r.DecidedByUserId, DecidedAt: r.DecidedAt, FactorKind: r.FactorKind,
		Comment: r.Comment, BypassReason: r.BypassReason, SupersededByRunId: r.SupersededByRunId,
	}
}

func approvalOfPending(r sqlc.ListPendingApprovalsRow) sqlc.DeploymentApproval {
	return sqlc.DeploymentApproval{
		Id: r.Id, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, OrganizationId: r.OrganizationId, EnvironmentId: r.EnvironmentId,
		RunId: r.RunId, RunNumber: r.RunNumber, Ref: r.Ref, Round: r.Round, RuleVersion: r.RuleVersion, State: r.State,
		ExpiresAt: r.ExpiresAt,
	}
}

func (s *Service) mayDecide(ctx context.Context, q *sqlc.Queries, r sqlc.ListPendingApprovalsRow, user uuid.UUID) (bool, error) {
	ok, err := s.approverEligible(ctx, q, r.OrganizationId, r.ProjectId, r.EnvironmentId, user)
	if err != nil || !ok {
		return false, err
	}
	allowSelf, err := q.EnvironmentAllowsSelfApproval(ctx, sqlc.EnvironmentAllowsSelfApprovalParams{ID: r.EnvironmentId, OrganizationID: r.OrganizationId})
	if err != nil || allowSelf {
		return allowSelf, err
	}
	run, err := q.ApprovalRun(ctx, sqlc.ApprovalRunParams{RunID: r.RunId, OrganizationID: r.OrganizationId})
	if err != nil {
		return false, err
	}
	self, err := s.isSelf(ctx, q, run, user)
	return !self, err
}

func (s *Service) PendingApprovals(ctx context.Context, org uuid.UUID, mine bool) ([]ApprovalView, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return nil, err
	}
	q := s.queries()
	projects, err := q.OrganizationProjectIDs(ctx, org)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListPendingApprovals(ctx, sqlc.ListPendingApprovalsParams{
		OrganizationID: org, Now: s.now(), ProjectIds: projects, PageLimit: pendingListMax,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ApprovalView, 0, len(rows))
	for _, r := range rows {
		if mine {
			ok, err := s.mayDecide(ctx, q, r, p.UserID)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
		}
		out = append(out, ApprovalView{Row: approvalOfPending(r), Environment: r.EnvironmentName, Project: r.ProjectId})
	}
	return out, nil
}

func approvalToGen(v ApprovalView) gen.DeploymentApproval {
	r := v.Row
	return gen.DeploymentApproval{
		Id: r.Id, ProjectId: v.Project, EnvironmentId: r.EnvironmentId, Environment: v.Environment, RunId: r.RunId, RunNumber: r.RunNumber,
		Ref: r.Ref, Round: int(r.Round), RuleVersion: r.RuleVersion, State: gen.DeploymentApprovalState(r.State), ExpiresAt: r.ExpiresAt,
		CreatedAt: r.CreatedAt, DecidedByUserId: r.DecidedByUserId, DecidedAt: r.DecidedAt, FactorKind: textPtr(r.FactorKind), Comment: textPtr(r.Comment),
		BypassReason: textPtr(r.BypassReason), SupersededByRunId: r.SupersededByRunId,
	}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func approvalsToGen(list []ApprovalView) []gen.DeploymentApproval {
	out := make([]gen.DeploymentApproval, 0, len(list))
	for _, v := range list {
		out = append(out, approvalToGen(v))
	}
	return out
}

func decisionOf(run, approval uuid.UUID, f gen.CurrentFactor, comment *string, reason string) Decision {
	d := Decision{Run: run, Approval: approval, Factor: factorOf(f), Reason: reason}
	if comment != nil {
		d.Comment = *comment
	}
	return d
}

func (c *Controller) ListRunApprovals(ctx context.Context, req gen.ListRunApprovalsRequestObject) (gen.ListRunApprovalsResponseObject, error) {
	list, err := c.service.RunApprovals(ctx, req.Id, req.ProjectId, req.RunId)
	if err != nil {
		return nil, err
	}
	items := approvalsToGen(list)
	return gen.ListRunApprovals200JSONResponse{Success: true, Message: ptrOf(messageApprovalsListed), Data: &items}, nil
}

func (c *Controller) ListPendingApprovals(ctx context.Context, req gen.ListPendingApprovalsRequestObject) (gen.ListPendingApprovalsResponseObject, error) {
	list, err := c.service.PendingApprovals(ctx, req.Id, req.Params.Mine != nil && *req.Params.Mine)
	if err != nil {
		return nil, err
	}
	items := approvalsToGen(list)
	return gen.ListPendingApprovals200JSONResponse{Success: true, Message: ptrOf(messageApprovalsListed), Data: &items}, nil
}

func (c *Controller) ApproveDeployment(ctx context.Context, req gen.ApproveDeploymentRequestObject) (gen.ApproveDeploymentResponseObject, error) {
	v, err := c.service.ApproveDeployment(ctx, req.Id, req.ProjectId, decisionOf(req.RunId, req.ApprovalId, req.Body.CurrentFactor, req.Body.Comment, ""))
	if err != nil {
		return nil, err
	}
	out := approvalToGen(v)
	return gen.ApproveDeployment200JSONResponse{Success: true, Message: ptrOf(messageApproved), Data: &out}, nil
}

func (c *Controller) RejectDeployment(ctx context.Context, req gen.RejectDeploymentRequestObject) (gen.RejectDeploymentResponseObject, error) {
	v, err := c.service.RejectDeployment(ctx, req.Id, req.ProjectId, decisionOf(req.RunId, req.ApprovalId, req.Body.CurrentFactor, req.Body.Comment, ""))
	if err != nil {
		return nil, err
	}
	out := approvalToGen(v)
	return gen.RejectDeployment200JSONResponse{Success: true, Message: ptrOf(messageRejected), Data: &out}, nil
}

func (c *Controller) BypassDeployment(ctx context.Context, req gen.BypassDeploymentRequestObject) (gen.BypassDeploymentResponseObject, error) {
	v, err := c.service.BypassDeployment(ctx, req.Id, req.ProjectId, decisionOf(req.RunId, req.ApprovalId, req.Body.CurrentFactor, nil, req.Body.Reason))
	if err != nil {
		return nil, err
	}
	out := approvalToGen(v)
	return gen.BypassDeployment200JSONResponse{Success: true, Message: ptrOf(messageBypassed), Data: &out}, nil
}
