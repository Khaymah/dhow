package environments

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	deployments "github.com/Khaymah/dhow/apps/server/internal/Core/Deployments"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const (
	approvalPending    = "pending"
	approvalApproved   = "approved"
	approvalRejected   = "rejected"
	approvalExpired    = "expired"
	approvalSuperseded = "superseded"
	approvalBypassed   = "bypassed"
	approvalCanceled   = "canceled"
)

func (s *Service) approverEligible(ctx context.Context, q *sqlc.Queries, org, project, env, user uuid.UUID) (bool, error) {
	reviewer, err := q.IsEnvironmentReviewer(ctx, sqlc.IsEnvironmentReviewerParams{EnvironmentID: env, OrganizationID: org, UserID: &user})
	if err != nil || !reviewer {
		return false, err
	}
	rows, err := q.ReviewerUserRoles(ctx, sqlc.ReviewerUserRolesParams{ProjectID: &project, OrganizationID: org, UserIds: []uuid.UUID{user}})
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		role, ok := policy.ParseRole(row.Role)
		if !ok {
			continue
		}
		bindings := make([]policy.Role, 0, len(row.Bindings))
		for _, b := range row.Bindings {
			if br, ok := policy.ParseRole(b); ok {
				bindings = append(bindings, br)
			}
		}
		if policy.AtLeast(policy.ProjectRole(role, bindings), policy.RoleDeveloper) {
			return true, nil
		}
	}
	return false, nil
}

func (g *Gate) close(ctx context.Context, q *sqlc.Queries, a sqlc.DeploymentApproval, state string, by *uuid.UUID) error {
	_, err := q.CloseApproval(ctx, sqlc.CloseApprovalParams{
		State: state, Now: g.s.now(), SupersededByRunID: by, ID: a.Id, OrganizationID: a.OrganizationId,
	})
	return err
}

func (g *Gate) Approve(ctx context.Context, q *sqlc.Queries, r deployments.ApprovalRequest) (deployments.Approval, error) {
	key := sqlc.LiveApprovalParams{RunID: r.RunID, EnvironmentID: r.EnvironmentID, Round: r.Round, OrganizationID: r.OrganizationID}
	live, err := q.LiveApproval(ctx, key)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return g.afterClosed(ctx, q, r)
	case err != nil:
		return deployments.Approval{}, err
	}
	out, keep, err := g.judge(ctx, q, r, live)
	if err != nil || keep {
		return out, err
	}
	return g.request(ctx, q, r)
}

func (g *Gate) judge(ctx context.Context, q *sqlc.Queries, r deployments.ApprovalRequest, a sqlc.DeploymentApproval) (deployments.Approval, bool, error) {
	same := a.RuleVersion == r.RuleVersion
	switch a.State {
	case approvalPending:
		if !g.s.now().Before(a.ExpiresAt) {
			if err := g.close(ctx, q, a, approvalExpired, nil); err != nil {
				return deployments.Approval{}, false, err
			}
			return deployments.Approval{ID: a.Id, Outcome: deployments.ApprovalExpired, Expired: true}, true, nil
		}
		if same {
			return deployments.Approval{ID: a.Id, Outcome: deployments.ApprovalWaiting}, true, nil
		}
		return deployments.Approval{}, false, g.close(ctx, q, a, approvalSuperseded, nil)
	case approvalBypassed:
		if same {
			return deployments.Approval{ID: a.Id, Outcome: deployments.ApprovalBypassed}, true, nil
		}
	case approvalApproved:
		ok := same && a.DecidedByUserId != nil
		if ok {
			var err error
			if ok, err = g.s.approverEligible(ctx, q, r.OrganizationID, r.ProjectID, r.EnvironmentID, *a.DecidedByUserId); err != nil {
				return deployments.Approval{}, false, err
			}
		}
		if ok {
			return deployments.Approval{ID: a.Id, Outcome: deployments.ApprovalPassed}, true, nil
		}
	}
	_, err := q.SupersedeDecidedApproval(ctx, sqlc.SupersedeDecidedApprovalParams{Now: g.s.now(), ID: a.Id, OrganizationID: a.OrganizationId})
	return deployments.Approval{}, false, err
}

func (g *Gate) afterClosed(ctx context.Context, q *sqlc.Queries, r deployments.ApprovalRequest) (deployments.Approval, error) {
	last, err := q.LatestApproval(ctx, sqlc.LatestApprovalParams{
		RunID: r.RunID, EnvironmentID: r.EnvironmentID, Round: r.Round, OrganizationID: r.OrganizationID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return g.request(ctx, q, r)
	}
	if err != nil {
		return deployments.Approval{}, err
	}
	switch {
	case last.State == approvalRejected:
		return deployments.Approval{ID: last.Id, Outcome: deployments.ApprovalRejected}, nil
	case last.State == approvalExpired:
		return deployments.Approval{ID: last.Id, Outcome: deployments.ApprovalExpired}, nil
	case last.State == approvalSuperseded && last.SupersededByRunId != nil:
		return deployments.Approval{ID: last.Id, Outcome: deployments.ApprovalSuperseded}, nil
	}
	return g.request(ctx, q, r)
}

func (g *Gate) request(ctx context.Context, q *sqlc.Queries, r deployments.ApprovalRequest) (deployments.Approval, error) {
	now := g.s.now()
	row, err := q.InsertApproval(ctx, sqlc.InsertApprovalParams{
		OrganizationID: r.OrganizationID, EnvironmentID: r.EnvironmentID, RunID: r.RunID, RunNumber: r.RunNumber, Ref: r.Ref,
		Round: r.Round, RuleVersion: r.RuleVersion, ExpiresAt: now.Add(time.Duration(r.TimeoutHours) * time.Hour), Now: now,
	})
	if err != nil {
		return deployments.Approval{}, err
	}
	out := deployments.Approval{ID: row.Id, Outcome: deployments.ApprovalWaiting, Requested: true}
	if !r.SupersedePending {
		return out, nil
	}
	older, err := q.SupersedeOlderPending(ctx, sqlc.SupersedeOlderPendingParams{
		Now: now, RunID: r.RunID, EnvironmentID: r.EnvironmentID, OrganizationID: r.OrganizationID, Ref: r.Ref, RunNumber: r.RunNumber,
	})
	if err != nil {
		return deployments.Approval{}, err
	}
	for _, o := range older {
		out.Superseded = append(out.Superseded, o.Id)
	}
	return out, nil
}

func (g *Gate) CancelApprovals(ctx context.Context, q *sqlc.Queries, org, run uuid.UUID) error {
	return q.CancelRunApprovals(ctx, sqlc.CancelRunApprovalsParams{Now: g.s.now(), RunID: run, OrganizationID: org})
}
