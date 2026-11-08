package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Events/Models"
)

type scope int

const (
	scopeNone scope = iota
	scopeData
	scopeTarget
	scopeUser
)

type rule struct {
	typ    string
	status string
	field  string
	scope  scope
}

func rules() map[string]rule {
	return map[string]rule{
		eventbus.DeploymentStarted:               {typ: models.TypeDeploymentStarted, status: "running", scope: scopeData},
		eventbus.DeploymentFinished:              {typ: models.TypeDeploymentFinished, field: "State", scope: scopeData},
		eventbus.DeploymentApprovalRequested:     {typ: models.TypeApprovalRequested, status: "pending", scope: scopeData},
		eventbus.DeploymentApprovalExpired:       {typ: models.TypeApprovalExpired, status: "expired", scope: scopeData},
		eventbus.DeploymentApproved:              {typ: models.TypeApprovalDecided, status: "approved", scope: scopeData},
		eventbus.DeploymentRejected:              {typ: models.TypeApprovalDecided, status: "rejected", scope: scopeData},
		eventbus.EnvironmentBypassed:             {typ: models.TypeApprovalDecided, status: "bypassed", scope: scopeData},
		eventbus.DeploymentReleaseStarted:        {typ: models.TypeReleaseStarted, status: "running", scope: scopeTarget},
		eventbus.DeploymentReleaseSwitched:       {typ: models.TypeReleaseSwitched, status: "running", scope: scopeTarget},
		eventbus.DeploymentReleaseSucceeded:      {typ: models.TypeReleaseSucceeded, status: "succeeded", scope: scopeTarget},
		eventbus.DeploymentReleaseRolledBack:     {typ: models.TypeReleaseRolledBack, status: "rolled_back", scope: scopeTarget},
		eventbus.DeploymentReleaseRollbackFailed: {typ: models.TypeReleaseRollbackFail, status: "rollback_failed", scope: scopeTarget},
		eventbus.DeploymentReleaseUnknown:        {typ: models.TypeReleaseUnknown, status: "unknown", scope: scopeTarget},
		eventbus.DeploymentReleaseRepaired:       {typ: models.TypeReleaseRepaired, field: "State", scope: scopeTarget},
		eventbus.DeploymentTargetChecked:         {typ: models.TypeTargetChecked, field: "Reason", scope: scopeTarget},
		eventbus.DeploymentTargetCheckRequested:  {typ: models.TypeTargetCheckRequested, status: "requested", scope: scopeTarget},
		eventbus.DeploymentRollbackRequested:     {typ: models.TypeTargetRollbackAsked, status: "requested", scope: scopeTarget},
		eventbus.JobCreated:                      {typ: models.TypeOverviewChanged},
		eventbus.JobCancelRequested:              {typ: models.TypeOverviewChanged},
		eventbus.RunCreated:                      {typ: models.TypeOverviewChanged},
		eventbus.RunFinished:                     {typ: models.TypeOverviewChanged},
		eventbus.RunnerRegistered:                {typ: models.TypeOverviewChanged},
		eventbus.RunnerRevoked:                   {typ: models.TypeOverviewChanged},
		eventbus.IdentityInvitationCreated:       {typ: models.TypeOverviewChanged},
		eventbus.IdentityInvitationAccepted:      {typ: models.TypeOverviewChanged},
		eventbus.IdentityInvitationRevoked:       {typ: models.TypeOverviewChanged},
		eventbus.NotificationChannelDisabled:     {typ: models.TypeOverviewChanged},
		eventbus.NotificationInboxChanged:        {typ: models.TypeInboxChanged, scope: scopeUser},
	}
}

func HandledEvents() []string {
	out := make([]string, 0, len(rules()))
	for name := range rules() {
		out = append(out, name)
	}
	return out
}

func ActionOf(typ string) policy.Action {
	switch typ {
	case models.TypeApprovalRequested:
		return policy.DeploymentApprove
	case models.TypeOverviewChanged, models.TypeInboxChanged:
		return policy.OrgRead
	case models.TypeTargetChecked, models.TypeTargetCheckRequested, models.TypeTargetRollbackAsked:
		return policy.ProjectRead
	default:
		return policy.RunRead
	}
}

func (s *Service) Handle(ctx context.Context, ev eventbus.Event) error {
	r, ok := s.rules[ev.Name]
	if !ok {
		return nil
	}
	a, ok := ev.Payload.(eventbus.Audited)
	if !ok || len(a.OrganizationIDs) != 1 {
		return nil
	}
	q := s.queries(a)
	n, err := s.noticeOf(ctx, q, r, a)
	if err != nil {
		s.log.WarnContext(ctx, "a live event could not be built", "event", ev.Name, "error", err)
		return nil
	}
	raw, err := json.Marshal(n)
	if err != nil || len(raw) > config.OrgEventsMaxPayload {
		s.log.WarnContext(ctx, "a live event was dropped", "event", ev.Name, "bytes", len(raw))
		return nil
	}
	if err := q.NotifyOrgEvent(ctx, string(raw)); err != nil {
		return fmt.Errorf("publish the %s live event: %w", ev.Name, err)
	}
	return nil
}

func (s *Service) queries(a eventbus.Audited) *sqlc.Queries {
	if a.Tx != nil {
		return a.Tx
	}
	return sqlc.New(s.db)
}

func (s *Service) noticeOf(ctx context.Context, q *sqlc.Queries, r rule, a eventbus.Audited) (models.Notice, error) {
	org := a.OrganizationIDs[0]
	n := models.Notice{Organization: org, Type: r.typ, Status: r.status, At: s.now().UTC()}
	if r.field != "" {
		n.Status = safeStatus(a.Data[r.field])
	}
	switch r.scope {
	case scopeData:
		n.ProjectID, n.EnvironmentID = idOf(a.Data["ProjectId"]), idOf(a.Data["EnvironmentId"])
		n.SubjectID = idOf(a.TargetID)
	case scopeTarget:
		target := idOf(a.Data["TargetId"])
		if target == nil {
			target = idOf(a.TargetID)
		}
		if target == nil {
			return models.Notice{}, errors.New("the event names no target")
		}
		row, err := q.EventTargetScope(ctx, sqlc.EventTargetScopeParams{ID: *target, OrganizationID: org})
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Notice{}, errors.New("the target of the event is gone")
		}
		if err != nil {
			return models.Notice{}, err
		}
		n.TargetID, n.ProjectID, n.EnvironmentID = target, &row.ProjectId, &row.EnvironmentId
		n.SubjectID = idOf(a.TargetID)
	case scopeUser:
		user := idOf(a.TargetID)
		if user == nil {
			return models.Notice{}, errors.New("the event names no user")
		}
		n.User = user
	case scopeNone:
	}
	return n, nil
}

func idOf(raw string) *uuid.UUID {
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		return nil
	}
	return &id
}

var allowedStatuses = map[string]bool{
	"queued": true, "running": true, "succeeded": true, "failed": true, "canceled": true, "rolled_back": true, "rollback_failed": true,
	"unknown": true, "ok": true, "host_key_mismatch": true, "unreachable": true, "auth_failed": true, "path_missing": true,
	"current_not_symlink": true, "state_corrupt": true, "disk_low": true, "locked": true, "diverged": true,
}

func safeStatus(v string) string {
	if allowedStatuses[v] {
		return v
	}
	return "unknown"
}

func knownTypes() map[string]bool {
	out := map[string]bool{}
	for _, r := range rules() {
		out[r.typ] = true
	}
	return out
}
