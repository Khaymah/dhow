package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	outbox "github.com/Khaymah/dhow/apps/server/internal/Core/Outbox"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Models"
)

const (
	inboxKeyPrefix  = "inbox."
	inboxValueBytes = 120
	inboxParamsMax  = 1800
)

var inboxReason = regexp.MustCompile(`^[a-z0-9_.-]{1,64}$`)

type inboxDeliverer struct{ s *Service }

type inboxEntry struct {
	user  uuid.UUID
	org   *uuid.UUID
	event uuid.UUID
	p     Payload
	self  bool
	at    time.Time
}

func inboxTitleKey(p Payload, self bool) string {
	key := inboxKeyPrefix + p.Event
	if p.Event == EventDeploymentFinished && p.Status != "" {
		key += "." + p.Status
	}
	if self {
		key += ".self"
	}
	return key
}

func clipValue(v string) string {
	v = clip(v)
	for len(v) > inboxValueBytes {
		_, size := utf8.DecodeLastRuneInString(v)
		v = v[:len(v)-size]
	}
	return v
}

type inboxValues struct {
	Project     string `json:"Project,omitempty"`
	Environment string `json:"Environment,omitempty"`
	Ref         string `json:"Ref,omitempty"`
	Run         string `json:"Run,omitempty"`
	Runner      string `json:"Runner,omitempty"`
	Browser     string `json:"Browser,omitempty"`
	Release     string `json:"Release,omitempty"`
	Reason      string `json:"Reason,omitempty"`
	Change      string `json:"Change,omitempty"`
}

func inboxParams(p Payload) inboxValues {
	out := inboxValues{
		Project: clipValue(p.Project), Environment: clipValue(p.Environment),
		Ref: clipValue(p.Ref), Runner: clipValue(p.Runner),
		Browser: clipValue(p.Browser), Release: clipValue(p.Release),
	}
	if p.Run > 0 {
		out.Run = strconv.FormatInt(p.Run, 10)
	}
	if inboxReason.MatchString(p.Reason) {
		out.Reason = p.Reason
	}
	if inboxReason.MatchString(p.Change) {
		out.Change = p.Change
	}
	return out
}

func inboxLinkOf(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	path := u.RequestURI()
	if path == "" || path[0] != '/' || strings.HasPrefix(path, "//") || len(path) > config.NotifyInboxLinkMax || strings.ContainsAny(path, "\\\r\n\t") {
		return ""
	}
	return path
}

func (s *Service) addInbox(ctx context.Context, q *sqlc.Queries, e inboxEntry) error {
	raw, err := encodeInboxParams(inboxParams(e.p))
	if err != nil {
		return err
	}
	added, err := q.InsertNotificationInbox(ctx, sqlc.InsertNotificationInboxParams{
		UserID: e.user, OrganizationID: e.org, EventID: e.event, EventName: e.p.Event, TitleKey: inboxTitleKey(e.p, e.self), Params: raw,
		Link: inboxLinkOf(e.p.URL), IsAccount: e.self, CreatedAt: e.at,
	})
	if err != nil || added == 0 {
		return err
	}
	keep := int32(config.NotifyInboxPerUser)
	if e.self {
		keep = config.NotifyInboxAccountPerUser
	}
	if _, err = q.TrimNotificationInbox(ctx, sqlc.TrimNotificationInboxParams{UserID: e.user, IsAccount: e.self, Keep: keep}); err != nil {
		return err
	}
	s.inboxChanged(ctx, q, e.user)
	return nil
}

func encodeInboxParams(values inboxValues) ([]byte, error) {
	optional := []*string{&values.Release, &values.Browser, &values.Runner, &values.Ref, &values.Environment, &values.Project}
	for {
		raw, err := json.Marshal(values)
		if err != nil {
			return nil, fmt.Errorf("encode the values of an inbox notification: %w", err)
		}
		if len(raw) <= inboxParamsMax || len(optional) == 0 {
			return raw, nil
		}
		*optional[0] = ""
		optional = optional[1:]
	}
}

func isCheckViolation(err error) bool {
	var pe interface{ SQLState() string }
	return errors.As(err, &pe) && pe.SQLState() == "23514"
}

func (d inboxDeliverer) Deliver(ctx context.Context, item outbox.Item) outbox.Result {
	s := d.s
	if _, res, ok := s.liveChannel(ctx, item); !ok {
		return res
	}
	if item.RecipientUserID == nil {
		return outbox.Result{Code: CodeNoRecipient, Permanent: true}
	}
	q := s.queries()
	_, err := q.GetNotificationUser(ctx, *item.RecipientUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return outbox.Result{Code: CodeNoRecipient, Permanent: true}
	}
	if err != nil {
		return outbox.Result{Code: CodeConfig}
	}
	p, err := DecodePayload(item.Payload)
	if err != nil {
		return outbox.Result{Code: CodeConfig, Permanent: true}
	}
	err = s.addInbox(ctx, q, inboxEntry{user: *item.RecipientUserID, org: item.OrganizationID, event: item.EventID, p: p, at: s.now()})
	if err != nil {
		s.log.WarnContext(ctx, "inbox notification not saved", "delivery_id", item.ID.String())
		return outbox.Result{Code: CodeConfig, Permanent: isCheckViolation(err)}
	}
	return outbox.Result{}
}

func (s *Service) inboxAccount(ctx context.Context, q *sqlc.Queries, user uuid.UUID, n Notice) error {
	return s.addInbox(ctx, q, inboxEntry{user: user, event: n.EventID, p: n.Payload, self: true, at: n.At})
}

func (s *Service) inboxRoleChange(ctx context.Context, q *sqlc.Queries, a eventbus.Audited, n Notice) error {
	user, ok := parseID(a.TargetID)
	if !ok || len(a.OrganizationIDs) != 1 {
		return nil
	}
	org := a.OrganizationIDs[0]
	return s.addInbox(ctx, q, inboxEntry{user: user, org: &org, event: n.EventID, p: n.Payload, self: true, at: n.At})
}

func inboxItemOf(id uuid.UUID, at time.Time, org *uuid.UUID, event uuid.UUID, name, key string, params []byte, link string, read bool, readAt *time.Time) models.InboxItem {
	values := map[string]string{}
	if err := json.Unmarshal(params, &values); err != nil {
		values = map[string]string{}
	}
	return models.InboxItem{
		ID: id, OrganizationID: org, EventID: event, Event: name, TitleKey: key, Params: values, Link: link, Read: read, ReadAt: readAt, CreatedAt: at,
	}
}

func (s *Service) ListInbox(ctx context.Context, unreadOnly bool, page, limit int) (models.InboxPage, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.InboxPage{}, err
	}
	q := s.queries()
	total, err := q.CountNotificationInbox(ctx, sqlc.CountNotificationInboxParams{UserID: p.UserID, UnreadOnly: unreadOnly})
	if err != nil {
		return models.InboxPage{}, err
	}
	rows, err := q.ListNotificationInbox(ctx, sqlc.ListNotificationInboxParams{
		UserID: p.UserID, UnreadOnly: unreadOnly, PageLimit: int32(limit), PageOffset: int32((page - 1) * limit), //nolint:gosec // Bounded by the page parameters.
	})
	if err != nil {
		return models.InboxPage{}, err
	}
	items := make([]models.InboxItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, inboxItemOf(r.Id, r.CreatedAt, r.OrganizationId, r.EventId, r.EventName, r.TitleKey, r.Params, r.Link, r.IsRead, r.ReadAt))
	}
	return models.InboxPage{Items: items, Total: total}, nil
}

func (s *Service) InboxUnread(ctx context.Context) (int64, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return 0, err
	}
	return s.queries().CountNotificationInbox(ctx, sqlc.CountNotificationInboxParams{UserID: p.UserID, UnreadOnly: true})
}

func (s *Service) MarkInboxRead(ctx context.Context, id uuid.UUID) (models.InboxItem, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.InboxItem{}, err
	}
	r, err := s.queries().MarkNotificationInboxRead(ctx, sqlc.MarkNotificationInboxReadParams{Now: s.now(), ID: id, UserID: p.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return models.InboxItem{}, apperrors.NotFound()
	}
	if err != nil {
		return models.InboxItem{}, err
	}
	s.inboxChanged(ctx, s.queries(), p.UserID)
	return inboxItemOf(r.Id, r.CreatedAt, r.OrganizationId, r.EventId, r.EventName, r.TitleKey, r.Params, r.Link, r.IsRead, r.ReadAt), nil
}

func (s *Service) MarkAllInboxRead(ctx context.Context) (int64, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return 0, err
	}
	n, err := s.queries().MarkAllNotificationInboxRead(ctx, sqlc.MarkAllNotificationInboxReadParams{Now: s.now(), UserID: p.UserID})
	if err == nil && n > 0 {
		s.inboxChanged(ctx, s.queries(), p.UserID)
	}
	return n, err
}

func (s *Service) PurgeInbox(ctx context.Context) error {
	before := s.now().AddDate(0, 0, -s.cfg.RetentionDays)
	for {
		n, err := s.queries().PurgeNotificationInbox(ctx, sqlc.PurgeNotificationInboxParams{Before: before, Batch: config.NotifyInboxPurgeBatch})
		if err != nil {
			return fmt.Errorf("drop the old inbox notifications: %w", err)
		}
		if n == 0 {
			return nil
		}
	}
}
