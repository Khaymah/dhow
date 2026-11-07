package alerts

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	mailer "github.com/Khaymah/dhow/apps/server/internal/Core/Mail"
	outbox "github.com/Khaymah/dhow/apps/server/internal/Core/Outbox"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

type Options struct {
	DB        sqlc.DBTX
	Mailer    mailer.Sender
	PublicURL string
	Log       *slog.Logger
	Now       func() time.Time
}

type Service struct {
	db     sqlc.DBTX
	mailer mailer.Sender
	link   string
	log    *slog.Logger
	now    func() time.Time
	closed atomic.Bool
	flush  func(context.Context) error
}

type mailBody struct {
	Subject string `json:"Subject"`
	Body    string `json:"Body"`
}

func NewService(o Options) *Service {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Service{db: o.DB, mailer: o.Mailer, link: o.PublicURL + config.AccountSecurityPath, log: o.Log, now: now}
}

func (s *Service) UseFlush(fn func(context.Context) error) { s.flush = fn }

func (s *Service) Wait() {
	if s.flush == nil {
		return
	}
	if err := s.flush(context.Background()); err != nil {
		s.log.Warn("credential alerts could not be flushed", "error", err)
	}
}

func (s *Service) Close() { s.closed.Store(true) }

func (s *Service) Handle(ctx context.Context, ev eventbus.Event) error {
	ctx = context.WithoutCancel(ctx)
	change, ok := changeOf(ev.Name)
	if !ok || !s.mailer.Enabled() {
		return nil
	}
	if s.closed.Load() {
		s.log.WarnContext(ctx, "credential alert not sent: shutting down", "event", ev.Name)
		return nil
	}
	a, ok := ev.Payload.(eventbus.Audited)
	if !ok {
		s.log.WarnContext(ctx, "credential alert skipped: unexpected payload", "event", ev.Name)
		return nil
	}
	user, ok := recipientOf(ev.Name, a)
	if !ok {
		s.log.WarnContext(ctx, "credential alert skipped: no recipient", "event", ev.Name)
		return nil
	}
	q := a.Tx
	if q == nil {
		q = sqlc.New(s.db)
	}
	r, err := q.GetAlertRecipient(ctx, user)
	if err != nil {
		s.log.WarnContext(ctx, "credential alert skipped: recipient lookup failed", "event", ev.Name, "user_id", user.String(), "error", err)
		return nil
	}
	msg := Message(i18n.Locale(r.Locale), Details{
		To: r.Email, Name: r.Name, Change: change, At: s.now(), IP: a.IP, UserAgent: auth.RequestMetaFrom(ctx).UserAgent, Link: s.link,
	})
	s.queue(ctx, q, ev.Name, a, user, msg)
	return nil
}

func (s *Service) queue(ctx context.Context, q *sqlc.Queries, event string, a eventbus.Audited, user uuid.UUID, msg mailer.Message) {
	pending, err := q.CountPendingInternalEmails(ctx)
	if err != nil {
		s.log.WarnContext(ctx, "credential alert not sent: the queue cannot be read", "event", event, "user_id", user.String(), "error", err)
		return
	}
	if pending >= config.AlertMaxPending {
		s.log.WarnContext(ctx, "credential alert not sent: too many pending", "event", event, "user_id", user.String())
		return
	}
	raw, err := json.Marshal(mailBody{Subject: msg.Subject, Body: msg.Body})
	if err != nil {
		s.log.WarnContext(ctx, "credential alert not sent: the message cannot be encoded", "event", event, "user_id", user.String(), "error", err)
		return
	}
	entry := outbox.Entry{
		Kind: outbox.KindInternalEmail, RecipientUserID: &user, EventID: uuid.New(), EventName: event, Payload: raw, NextAttemptAt: s.now(),
	}
	if len(a.OrganizationIDs) > 0 {
		entry.OrganizationID = a.OrganizationIDs[0]
	}
	if _, err := outbox.Enqueue(ctx, q, entry); err != nil {
		s.log.WarnContext(ctx, "credential alert not sent", "event", event, "user_id", user.String(), "error", err)
	}
}

func recipientOf(event string, a eventbus.Audited) (uuid.UUID, bool) {
	var raw string
	switch event {
	case eventbus.IdentityMemberMfaReset:
		raw = a.TargetID
	case eventbus.AuthApiTokenCreated:
		raw = a.Data[ownerKey]
	default:
		if a.ActorUserID == nil {
			return uuid.Nil, false
		}
		return *a.ActorUserID, true
	}
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, false
	}
	return id, true
}

func Subscribe(bus *eventbus.Bus, s *Service) {
	for _, name := range Events() {
		bus.Subscribe(name, s.Handle)
	}
}
