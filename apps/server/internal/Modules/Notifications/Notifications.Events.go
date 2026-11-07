package notifications

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	outbox "github.com/Khaymah/dhow/apps/server/internal/Core/Outbox"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Models"
)

const maxEmailRecipients = 200

func Subscribe(bus Subscriber, s *Service) {
	bus.Subscribe(eventbus.DeploymentFinished, s.onDeploymentFinished)
	bus.Subscribe(eventbus.DeploymentApprovalRequested, s.onApprovalRequested)
	bus.Subscribe(eventbus.DeploymentReleaseRolledBack, s.onRelease(StatusRolledBack))
	bus.Subscribe(eventbus.DeploymentReleaseRollbackFailed, s.onRelease(StatusRollbackFailed))
	bus.Subscribe(eventbus.RunFinished, s.onRunFinished)
	bus.Subscribe(eventbus.AuthLogin, s.onLogin)
	for _, name := range []string{
		eventbus.AuthTotpEnabled, eventbus.AuthTotpDisabled, eventbus.AuthPasskeyAdded, eventbus.AuthPasskeyRemoved,
		eventbus.AuthRecoveryCodesRegenerated,
	} {
		bus.Subscribe(name, s.onSecondFactor)
	}
	bus.Subscribe(eventbus.IdentityMemberRemoved, s.onMemberRemoved)
	bus.Subscribe(eventbus.IdentityMemberLeft, s.onMemberGone)
	bus.Subscribe(eventbus.IdentityMemberRoleChanged, s.onRoleChanged)
}

func auditedOf(ev eventbus.Event) (eventbus.Audited, error) {
	a, ok := ev.Payload.(eventbus.Audited)
	if !ok {
		return eventbus.Audited{}, fmt.Errorf("notifications: the %s event carries an unexpected payload", ev.Name)
	}
	return a, nil
}

func (s *Service) tx(a eventbus.Audited) *sqlc.Queries {
	if a.Tx != nil {
		return a.Tx
	}
	return s.queries()
}

func (s *Service) quiet(ctx context.Context, a eventbus.Audited, event string, err error) error {
	if err == nil {
		return nil
	}
	if a.Tx != nil {
		return fmt.Errorf("queue the %s notification: %w", event, err)
	}
	s.log.ErrorContext(ctx, "notification could not be queued", "event", event, "error", err)
	return nil
}

func (s *Service) link(path string) string {
	if s.publicURL == "" {
		return ""
	}
	return s.publicURL + path
}

func (s *Service) runLink(project uuid.UUID, number int64) string {
	if number < 1 {
		return s.link("/projects/" + project.String())
	}
	return s.link("/projects/" + project.String() + "/runs/" + strconv.FormatInt(number, 10))
}

func parseID(raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	return id, err == nil && id != uuid.Nil
}

type deployInfo struct {
	project     uuid.UUID
	environment string
	runNumber   int64
	ref         string
}

func (s *Service) infoOf(ctx context.Context, q *sqlc.Queries, org uuid.UUID, data map[string]string) (deployInfo, bool, error) {
	project, ok := parseID(data["ProjectId"])
	env, envOK := parseID(data["EnvironmentId"])
	run, runOK := parseID(data["RunId"])
	if !ok || !envOK || !runOK {
		return deployInfo{}, false, nil
	}
	out := deployInfo{project: project}
	name, err := q.NotificationEnvironmentName(ctx, sqlc.NotificationEnvironmentNameParams{ID: env, OrganizationID: org})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return deployInfo{}, false, err
	}
	out.environment = name
	r, err := q.NotificationRunContext(ctx, sqlc.NotificationRunContextParams{ID: run, OrganizationID: org})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return deployInfo{}, false, err
	}
	out.runNumber, out.ref = r.Number, r.Ref
	return out, true, nil
}

func (s *Service) projectNotice(
	ctx context.Context, q *sqlc.Queries, org uuid.UUID, info deployInfo, event, status string, id uuid.UUID,
) (Notice, error) {
	p, err := q.NotificationProjectContext(ctx, sqlc.NotificationProjectContextParams{ProjectID: info.project, OrganizationID: org})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Notice{}, err
	}
	now := s.now().UTC()
	return Notice{
		EventID: id, Event: event, Status: status, Environment: info.environment, At: now,
		Payload: Payload{
			Event: event, EventID: id.String(), Time: now.Format(time.RFC3339), Status: status, Project: p.ProjectName,
			Environment: info.environment, Ref: info.ref, Run: info.runNumber, URL: s.runLink(info.project, info.runNumber),
		},
	}, nil
}

func (s *Service) deploymentEvent(
	ctx context.Context, ev eventbus.Event, event, status string, id uuid.UUID,
) error {
	a, err := auditedOf(ev)
	if err != nil {
		return err
	}
	if len(a.OrganizationIDs) != 1 {
		return nil
	}
	org := a.OrganizationIDs[0]
	q := s.tx(a)
	info, ok, err := s.infoOf(ctx, q, org, a.Data)
	if err != nil || !ok {
		return s.quiet(ctx, a, ev.Name, err)
	}
	n, err := s.projectNotice(ctx, q, org, info, event, status, id)
	if err != nil {
		return s.quiet(ctx, a, ev.Name, err)
	}
	project := info.project
	return s.quiet(ctx, a, ev.Name, s.fanOut(ctx, q, org, &project, n))
}

func (s *Service) onDeploymentFinished(ctx context.Context, ev eventbus.Event) error {
	a, err := auditedOf(ev)
	if err != nil {
		return err
	}
	status := a.Data["State"]
	if !ValidStatus(status) {
		return nil
	}
	id, ok := parseID(a.TargetID)
	if !ok {
		return nil
	}
	return s.deploymentEvent(ctx, ev, EventDeploymentFinished, status, id)
}

func (s *Service) onApprovalRequested(ctx context.Context, ev eventbus.Event) error {
	a, err := auditedOf(ev)
	if err != nil {
		return err
	}
	id, ok := parseID(a.TargetID)
	if !ok {
		return nil
	}
	return s.deploymentEvent(ctx, ev, EventApprovalRequested, StatusAwaitingApprove, id)
}

func (s *Service) onRunFinished(ctx context.Context, ev eventbus.Event) error {
	a, err := auditedOf(ev)
	if err != nil {
		return err
	}
	if a.Data["State"] != StatusFailed || len(a.OrganizationIDs) != 1 {
		return nil
	}
	project, ok := parseID(a.Data["ProjectId"])
	id, idOK := parseID(a.TargetID)
	if !ok || !idOK {
		return nil
	}
	number, _ := strconv.ParseInt(a.Data["Number"], 10, 64)
	org, q := a.OrganizationIDs[0], s.tx(a)
	n, err := s.projectNotice(ctx, q, org, deployInfo{project: project, runNumber: number, ref: a.Data["Ref"]}, EventRunFinished, StatusFailed, id)
	if err != nil {
		return s.quiet(ctx, a, ev.Name, err)
	}
	n.Payload.Reason = a.Data["Reason"]
	return s.quiet(ctx, a, ev.Name, s.fanOut(ctx, q, org, &project, n))
}

func (s *Service) onRelease(status string) eventbus.Handler {
	return func(ctx context.Context, ev eventbus.Event) error {
		a, err := auditedOf(ev)
		if err != nil {
			return err
		}
		deployment, ok := parseID(a.Data["DeploymentId"])
		release, relOK := parseID(a.TargetID)
		if !ok || !relOK || len(a.OrganizationIDs) != 1 {
			return nil
		}
		org, q := a.OrganizationIDs[0], s.tx(a)
		d, err := q.NotificationDeploymentContext(ctx, sqlc.NotificationDeploymentContextParams{ID: deployment, OrganizationID: org})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return s.quiet(ctx, a, ev.Name, err)
		}
		info := deployInfo{project: d.ProjectId, environment: d.EnvironmentName, runNumber: d.RunNumber, ref: d.Ref}
		n, err := s.projectNotice(ctx, q, org, info, EventDeploymentFinished, status, release)
		if err != nil {
			return s.quiet(ctx, a, ev.Name, err)
		}
		n.Payload.Reason = a.Data["Reason"]
		n.Payload.Release = shortDigest(a.Data["Digest"])
		project := info.project
		return s.quiet(ctx, a, ev.Name, s.fanOut(ctx, q, org, &project, n))
	}
}

func (s *Service) fanOut(ctx context.Context, q *sqlc.Queries, org uuid.UUID, project *uuid.UUID, n Notice) error {
	rules, err := q.ListRoutingRules(ctx, sqlc.ListRoutingRulesParams{OrganizationID: org, ProjectID: project})
	if err != nil {
		return err
	}
	raw, err := n.Payload.Encode()
	if err != nil {
		return err
	}
	for _, r := range rules {
		if !(RuleSpec{Events: r.Events, Environments: r.Environments, Statuses: r.Statuses}).Matches(n) {
			continue
		}
		channel := r.ChannelId
		base := outbox.Entry{
			OrganizationID: org, ProjectID: r.ChannelProjectId, ChannelID: &channel, Kind: outbox.Kind(r.Kind),
			EventID: n.EventID, EventName: n.Event, Payload: raw, NextAttemptAt: s.now(),
		}
		if err := s.enqueueRule(ctx, q, base, r.Kind, r.Roles); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) enqueueRule(ctx context.Context, q *sqlc.Queries, base outbox.Entry, kind string, roles []string) error {
	if kind != models.KindEmail {
		_, err := outbox.Enqueue(ctx, q, base)
		return err
	}
	recipients, err := q.ListEmailRecipients(ctx, sqlc.ListEmailRecipientsParams{OrganizationID: base.OrganizationID, Roles: roles})
	if err != nil {
		return err
	}
	if len(recipients) > maxEmailRecipients {
		recipients = recipients[:maxEmailRecipients]
	}
	for _, rcpt := range recipients {
		entry, user := base, rcpt.UserId
		entry.RecipientUserID = &user
		if _, err := outbox.Enqueue(ctx, q, entry); err != nil {
			return err
		}
	}
	return nil
}
