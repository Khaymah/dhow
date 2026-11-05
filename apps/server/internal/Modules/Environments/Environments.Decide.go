package environments

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

const (
	targetApproval   = "deployment_approval"
	maxDecisionBytes = 1024
	runRunning       = "running"
)

type Decision struct {
	Run      uuid.UUID
	Approval uuid.UUID
	Comment  string
	Reason   string
	Factor   auth.Factor
}

func (s *Service) UseReopen(fn func(ctx context.Context, run uuid.UUID) error) { s.reopen = fn }

func secondFactor(f auth.Factor) bool {
	switch f.Method {
	case auth.FactorTotp, auth.FactorRecoveryCode, auth.FactorPasskey:
		return true
	default:
		return false
	}
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return strings.ToLower(u.Hostname())
	}
	rest := raw
	if _, after, ok := strings.Cut(raw, "@"); ok {
		rest = after
	}
	host, _, _ := strings.Cut(rest, ":")
	return strings.ToLower(host)
}

func (s *Service) isSelf(ctx context.Context, q *sqlc.Queries, run sqlc.ApprovalRunRow, user uuid.UUID) (bool, error) {
	if run.TriggeredByUserId != nil && *run.TriggeredByUserId == user {
		return true, nil
	}
	if !run.Actor.Valid || run.Actor.String == "" || run.CloneUrl == "" {
		return false, nil
	}
	logins, err := q.UserLogins(ctx, user)
	if err != nil {
		return false, err
	}
	repoHost := hostOf(run.CloneUrl)
	for _, l := range logins {
		if l.Login.Valid && strings.EqualFold(l.Login.String, run.Actor.String) && hostOf(l.Issuer) == repoHost {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) decide(ctx context.Context, org, project uuid.UUID, d Decision, state string) (ApprovalView, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return ApprovalView{}, err
	}
	if !secondFactor(d.Factor) {
		return ApprovalView{}, apperrors.ReviewerNeedsSecondFactor()
	}
	if len(d.Comment) > maxDecisionBytes || len(d.Reason) > maxDecisionBytes || (state == approvalBypassed && strings.TrimSpace(d.Reason) == "") {
		return ApprovalView{}, apperrors.ValidationFailed()
	}
	if err := s.confirm(ctx, p, d.Factor); err != nil {
		return ApprovalView{}, err
	}
	out := ApprovalView{Project: project}
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		a, err := s.lockDecision(ctx, q, org, project, d, p.UserID, state)
		if err != nil {
			return err
		}
		if err := s.record(ctx, q, a, d, p.UserID, state); err != nil {
			return err
		}
		out.Environment = a.EnvironmentName
		out.Row, err = q.LatestApproval(ctx, sqlc.LatestApprovalParams{RunID: a.RunId, EnvironmentID: a.EnvironmentId, Round: a.Round, OrganizationID: org})
		return err
	})
	if err != nil {
		return ApprovalView{}, err
	}
	if s.reopen != nil {
		if err := s.reopen(ctx, d.Run); err != nil && s.log != nil {
			s.log.WarnContext(ctx, "the run was not advanced after a deploy decision; the reconciler will", "run_id", d.Run.String(), "error", err)
		}
	}
	return out, nil
}

func (s *Service) lockDecision(
	ctx context.Context, q *sqlc.Queries, org, project uuid.UUID, d Decision, user uuid.UUID, state string,
) (sqlc.LockApprovalRow, error) {
	a, err := q.LockApproval(ctx, sqlc.LockApprovalParams{ID: d.Approval, RunID: d.Run, OrganizationID: org, ProjectID: project})
	if errors.Is(err, pgx.ErrNoRows) {
		return a, apperrors.NotFound()
	}
	if err != nil {
		return a, err
	}
	if a.State != approvalPending || !s.now().Before(a.ExpiresAt) || a.RuleVersion != a.CurrentRuleVersion {
		return a, apperrors.ApprovalNotPending()
	}
	run, err := q.ApprovalRun(ctx, sqlc.ApprovalRunParams{RunID: d.Run, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return a, apperrors.NotFound()
	}
	if err != nil {
		return a, err
	}
	if run.State != runRunning {
		return a, apperrors.ApprovalNotPending()
	}
	if state == approvalBypassed {
		if !a.AllowAdminBypass {
			return a, apperrors.BypassNotAllowed()
		}
		return a, nil
	}
	eligible, err := s.approverEligible(ctx, q, org, project, a.EnvironmentId, user)
	if err != nil {
		return a, err
	}
	if !eligible {
		return a, apperrors.ReviewerNotEligible()
	}
	if !a.AllowSelfApproval {
		self, err := s.isSelf(ctx, q, run, user)
		if err != nil {
			return a, err
		}
		if self {
			return a, apperrors.SelfReviewNotAllowed()
		}
	}
	return a, nil
}

func textOrNil(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }

func (s *Service) record(ctx context.Context, q *sqlc.Queries, a sqlc.LockApprovalRow, d Decision, user uuid.UUID, state string) error {
	reason, comment := textOrNil(d.Reason), textOrNil(d.Comment)
	if state == approvalBypassed {
		comment = pgtype.Text{}
	} else {
		reason = pgtype.Text{}
	}
	n, err := q.DecideApproval(ctx, sqlc.DecideApprovalParams{
		State: state, UserID: user, Now: s.now(), FactorKind: d.Factor.Method, Comment: comment, BypassReason: reason,
		ID: a.Id, OrganizationID: a.OrganizationId,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return apperrors.ApprovalNotPending()
	}
	name := map[string]string{
		approvalApproved: eventbus.DeploymentApproved, approvalRejected: eventbus.DeploymentRejected, approvalBypassed: eventbus.EnvironmentBypassed,
	}[state]
	data := auditData{
		"EnvironmentId": a.EnvironmentId.String(), "Name": a.EnvironmentName, "RunId": a.RunId.String(),
		"ProjectId": a.ProjectId.String(), "Method": d.Factor.Method,
	}
	if state == approvalBypassed {
		data.reason(d.Reason)
		data[eventbus.SeverityKey] = eventbus.SeverityHigh
	}
	return s.bus.Publish(ctx, eventbus.Event{Name: name, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{a.OrganizationId}, ActorUserID: &user, TargetType: targetApproval,
		TargetID: a.Id.String(), IP: auth.RequestMetaFrom(ctx).IPString(), Data: data, Tx: q,
	}})
}

func (s *Service) ApproveDeployment(ctx context.Context, org, project uuid.UUID, d Decision) (ApprovalView, error) {
	return s.decide(ctx, org, project, d, approvalApproved)
}

func (s *Service) RejectDeployment(ctx context.Context, org, project uuid.UUID, d Decision) (ApprovalView, error) {
	return s.decide(ctx, org, project, d, approvalRejected)
}

func (s *Service) BypassDeployment(ctx context.Context, org, project uuid.UUID, d Decision) (ApprovalView, error) {
	return s.decide(ctx, org, project, d, approvalBypassed)
}
