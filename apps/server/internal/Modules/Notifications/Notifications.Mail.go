package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	mailer "github.com/Khaymah/dhow/apps/server/internal/Core/Mail"
	outbox "github.com/Khaymah/dhow/apps/server/internal/Core/Outbox"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

type emailDeliverer struct{ s *Service }

type internalMailDeliverer struct{ s *Service }

func line(locale i18n.Locale, field, value string) string {
	if value == "" {
		return ""
	}
	return i18n.Translate(locale, "notify.field."+field) + ": " + value + "\n"
}

func titleKey(p Payload) string {
	if p.Event == EventDeploymentFinished {
		return "notify.title." + p.Event + "." + p.Status
	}
	return "notify.title." + p.Event
}

func RenderMail(locale i18n.Locale, p Payload) (string, string) {
	title := i18n.Translate(locale, titleKey(p))
	subject := strings.NewReplacer("{title}", title).Replace(i18n.Translate(locale, "notify.mail.subject"))
	if p.Project != "" {
		subject += " · " + p.Project
	}
	status := ""
	if p.Status != "" {
		status = i18n.Translate(locale, "notify.status."+p.Status)
	}
	run := ""
	if p.Run > 0 {
		run = strconv.FormatInt(p.Run, 10)
	}
	var b strings.Builder
	b.WriteString(title + "\n\n")
	for _, f := range [][2]string{
		{"status", status}, {"project", p.Project}, {"environment", p.Environment}, {"ref", p.Ref}, {"run", run},
		{"runner", p.Runner}, {"browser", p.Browser}, {"network", p.Network}, {"time", p.Time},
	} {
		b.WriteString(line(locale, f[0], f[1]))
	}
	if p.URL != "" {
		b.WriteString("\n" + p.URL + "\n")
	}
	return oneLine(subject), b.String()
}

func oneLine(v string) string { return strings.NewReplacer("\r", " ", "\n", " ").Replace(v) }

func (d emailDeliverer) Deliver(ctx context.Context, item outbox.Item) outbox.Result {
	s := d.s
	ch, res, ok := s.liveChannel(ctx, item)
	if !ok {
		return res
	}
	if !s.mailer.Enabled() {
		return outbox.Result{Code: CodeMailOff, Permanent: true}
	}
	if item.RecipientUserID == nil {
		return outbox.Result{Code: CodeNoRecipient, Permanent: true}
	}
	q := s.queries()
	user, err := q.GetNotificationUser(ctx, *item.RecipientUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return outbox.Result{Code: CodeNoRecipient, Permanent: true}
	}
	if err != nil {
		return outbox.Result{Code: CodeConfig}
	}
	if muted, err := q.GetNotificationMute(ctx, user.Id); err == nil && muted && item.EventName != EventTest {
		return outbox.Result{}
	}
	p, err := DecodePayload(item.Payload)
	if err != nil {
		return outbox.Result{Code: CodeConfig, Permanent: true}
	}
	subject, body := RenderMail(i18n.Locale(ch.Locale), p)
	if err := s.mailer.Send(ctx, mailer.Message{To: user.Email, Subject: subject, Body: body}); err != nil {
		s.log.WarnContext(ctx, "notification email not sent", "delivery_id", item.ID.String(), "channel_id", ch.Id.String())
		return outbox.Result{Code: CodeMailSend}
	}
	return outbox.Result{}
}

func (d internalMailDeliverer) Deliver(ctx context.Context, item outbox.Item) outbox.Result {
	s := d.s
	if !s.mailer.Enabled() {
		return outbox.Result{Code: CodeMailOff, Permanent: true}
	}
	if item.RecipientUserID == nil {
		return outbox.Result{Code: CodeNoRecipient, Permanent: true}
	}
	user, err := s.queries().GetNotificationUser(ctx, *item.RecipientUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return outbox.Result{Code: CodeNoRecipient, Permanent: true}
	}
	if err != nil {
		return outbox.Result{Code: CodeConfig}
	}
	var m mailBody
	if err := json.Unmarshal(item.Payload, &m); err != nil {
		return outbox.Result{Code: CodeConfig, Permanent: true}
	}
	if err := s.mailer.Send(ctx, mailer.Message{To: user.Email, Subject: m.Subject, Body: m.Body}); err != nil {
		s.log.WarnContext(ctx, "account email not sent", "delivery_id", item.ID.String(), "user_id", user.Id.String())
		return outbox.Result{Code: CodeMailSend}
	}
	return outbox.Result{}
}

func (s *Service) mailCreatorDisabled(ctx context.Context, q *sqlc.Queries, ch sqlc.NotificationChannel, failures int) error {
	if !s.mailer.Enabled() {
		return nil
	}
	u, err := q.GetNotificationUser(ctx, ch.CreatedByUserId)
	if err != nil {
		return nil
	}
	locale := i18n.Locale(u.Locale)
	replacer := strings.NewReplacer("{name}", u.Name, "{channel}", ch.Name, "{count}", strconv.Itoa(failures), "{link}", s.link("/"))
	raw, err := json.Marshal(mailBody{
		Subject: oneLine(replacer.Replace(i18n.Translate(locale, "notify.disabled.subject"))),
		Body:    replacer.Replace(i18n.Translate(locale, "notify.disabled.body")),
	})
	if err != nil {
		return fmt.Errorf("encode the notice of a stopped channel: %w", err)
	}
	creator := ch.CreatedByUserId
	_, err = outbox.Enqueue(ctx, q, outbox.Entry{
		Kind: outbox.KindInternalEmail, RecipientUserID: &creator, EventID: uuid.New(), EventName: "notification.channel_disabled",
		Payload: raw, NextAttemptAt: s.now(), OrganizationID: ch.OrganizationId,
	})
	if err != nil {
		return err
	}
	return nil
}
