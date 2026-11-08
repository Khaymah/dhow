package notifications

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	alert "github.com/Khaymah/dhow/apps/server/internal/Core/Alert"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	outbox "github.com/Khaymah/dhow/apps/server/internal/Core/Outbox"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

const changeNewDevice = "new_device_login"

type mailBody struct {
	Subject string `json:"Subject"`
	Body    string `json:"Body"`
}

func secondFactorChange(event string) string {
	switch event {
	case eventbus.AuthTotpEnabled:
		return "totp_enabled"
	case eventbus.AuthTotpDisabled:
		return "totp_disabled"
	case eventbus.AuthPasskeyAdded:
		return "passkey_added"
	case eventbus.AuthPasskeyRemoved:
		return "passkey_removed"
	default:
		return "recovery_codes_regenerated"
	}
}

func (s *Service) accountNotice(event, change string, at time.Time) Notice {
	id := uuid.New()
	return Notice{
		EventID: id, Event: event, At: at,
		Payload: Payload{Event: event, EventID: id.String(), Time: at.UTC().Format(time.RFC3339), Change: change, URL: s.link(config.AccountSecurityPath)},
	}
}

func (s *Service) fanOutOrgs(ctx context.Context, q *sqlc.Queries, orgs []uuid.UUID, n Notice) error {
	for _, org := range orgs {
		if err := s.fanOut(ctx, q, org, nil, n); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) onLogin(ctx context.Context, ev eventbus.Event) error {
	a, err := auditedOf(ev)
	if err != nil || a.ActorUserID == nil {
		return err
	}
	meta := auth.RequestMetaFrom(ctx)
	label := alert.Browser(meta.UserAgent)
	sum := sha256.Sum256([]byte(a.ActorUserID.String() + "|" + label))
	q := s.tx(a)
	fresh, err := q.TouchNotificationDevice(ctx, sqlc.TouchNotificationDeviceParams{UserID: *a.ActorUserID, Fingerprint: sum[:], Now: s.now()})
	if err != nil {
		return s.quiet(ctx, a, ev.Name, err)
	}
	known, err := q.CountNotificationDevices(ctx, *a.ActorUserID)
	if err != nil || !fresh || known <= 1 {
		return s.quiet(ctx, a, ev.Name, err)
	}
	n := s.accountNotice(EventNewDeviceLogin, changeNewDevice, s.now())
	n.Payload.Browser, n.Payload.Network = label, alert.ApproximateIP(a.IP)
	err = s.mailUser(ctx, q, *a.ActorUserID, n.EventID, changeNewDevice, meta.UserAgent, a.IP)
	if err == nil {
		err = s.inboxAccount(ctx, q, *a.ActorUserID, n)
	}
	if err == nil {
		err = s.fanOutOrgs(ctx, q, a.OrganizationIDs, n)
	}
	return s.quiet(ctx, a, ev.Name, err)
}

func (s *Service) mailUser(ctx context.Context, q *sqlc.Queries, user, event uuid.UUID, change, userAgent, ip string) error {
	if !s.mailer.Enabled() {
		return nil
	}
	u, err := q.GetNotificationUser(ctx, user)
	if err != nil {
		return nil
	}
	msg := alert.Message(i18n.Locale(u.Locale), alert.Details{
		To: u.Email, Name: u.Name, Change: change, At: s.now(), IP: ip, UserAgent: userAgent, Link: s.link(config.AccountSecurityPath),
	})
	raw, err := json.Marshal(mailBody{Subject: msg.Subject, Body: msg.Body})
	if err != nil {
		return fmt.Errorf("encode an account mail: %w", err)
	}
	_, err = outbox.Enqueue(ctx, q, outbox.Entry{
		Kind: outbox.KindInternalEmail, RecipientUserID: &user, EventID: event, EventName: "account." + change, Payload: raw, NextAttemptAt: s.now(),
	})
	return err
}

func (s *Service) onSecondFactor(ctx context.Context, ev eventbus.Event) error {
	a, err := auditedOf(ev)
	if err != nil {
		return err
	}
	n := s.accountNotice(EventSecondFactor, secondFactorChange(ev.Name), s.now())
	q := s.tx(a)
	if a.ActorUserID != nil {
		err = s.inboxAccount(ctx, q, *a.ActorUserID, n)
	}
	if err == nil {
		err = s.fanOutOrgs(ctx, q, a.OrganizationIDs, n)
	}
	return s.quiet(ctx, a, ev.Name, err)
}

func (s *Service) onMemberRemoved(ctx context.Context, ev eventbus.Event) error {
	if err := s.onMemberGone(ctx, ev); err != nil {
		return err
	}
	a, err := auditedOf(ev)
	if err != nil {
		return err
	}
	n := s.accountNotice(EventMemberRemoved, "member_removed", s.now())
	return s.quiet(ctx, a, ev.Name, s.fanOutOrgs(ctx, s.tx(a), a.OrganizationIDs, n))
}

func (s *Service) onRoleChanged(ctx context.Context, ev eventbus.Event) error {
	a, err := auditedOf(ev)
	if err != nil {
		return err
	}
	n := s.accountNotice(EventRoleChanged, "role_changed", s.now())
	n.Payload.Reason = a.Data["To"]
	q := s.tx(a)
	err = s.inboxRoleChange(ctx, q, a, n)
	if err == nil {
		err = s.fanOutOrgs(ctx, q, a.OrganizationIDs, n)
	}
	return s.quiet(ctx, a, ev.Name, err)
}

func (s *Service) onMemberGone(ctx context.Context, ev eventbus.Event) error {
	a, err := auditedOf(ev)
	if err != nil {
		return err
	}
	if len(a.OrganizationIDs) != 1 {
		return nil
	}
	user, ok := parseID(a.TargetID)
	if !ok {
		return nil
	}
	q, org := s.tx(a), a.OrganizationIDs[0]
	_, err = q.HideNotificationInboxOfOrganization(ctx, sqlc.HideNotificationInboxOfOrganizationParams{UserID: user, OrganizationID: &org})
	if err == nil {
		err = s.disableCreatorChannels(ctx, q, a.OrganizationIDs[0], user, a.IP)
	}
	return s.quiet(ctx, a, ev.Name, err)
}

func (s *Service) disableCreatorChannels(ctx context.Context, q *sqlc.Queries, org, user uuid.UUID, ip string) error {
	rows, err := q.DisableNotificationChannelsOfCreator(ctx, sqlc.DisableNotificationChannelsOfCreatorParams{OrganizationID: org, UserID: user})
	if err != nil {
		return err
	}
	for _, row := range rows {
		err := s.audit(ctx, auditEntry{
			name: eventbus.NotificationChannelDisabled, org: org, ip: ip, target: targetChannel, targetID: row.Id, tx: q,
			data: map[string]string{"Name": row.Name, "Kind": row.Kind, "Reason": "creator-removed"},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ScanRunners(ctx context.Context) error {
	now := s.now()
	before := now.Add(-config.NotifyOfflineAfter)
	if _, err := s.queries().ResolveRunnerOutages(ctx, sqlc.ResolveRunnerOutagesParams{Now: now, Before: before}); err != nil {
		return fmt.Errorf("close the runner outages that ended: %w", err)
	}
	offline, err := s.queries().ListOfflineRunners(ctx, before)
	if err != nil {
		return fmt.Errorf("list the runners that stopped answering: %w", err)
	}
	for _, r := range offline {
		err := dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
			opened, err := q.OpenRunnerOutage(ctx, sqlc.OpenRunnerOutageParams{OrganizationID: r.OrganizationId, RunnerID: r.Id})
			if err != nil || opened == 0 {
				return err
			}
			n := s.accountNotice(EventRunnerOffline, "", now)
			n.Payload.Runner, n.Payload.URL = r.Name, s.link("/")
			return s.fanOut(ctx, q, r.OrganizationId, nil, n)
		})
		if err != nil {
			return fmt.Errorf("queue the notification of a runner outage: %w", err)
		}
	}
	return nil
}
