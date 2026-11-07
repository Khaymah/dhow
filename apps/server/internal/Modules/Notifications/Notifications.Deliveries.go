package notifications

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	outbox "github.com/Khaymah/dhow/apps/server/internal/Core/Outbox"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Models"
)

func deliveryOf(o sqlc.NotificationOutbox, name, kind string) models.Delivery {
	d := models.Delivery{
		ID: o.Id, ChannelID: valueOf(o.ChannelId), ChannelName: name, ChannelKind: kind, Event: o.EventName, EventID: o.EventId, State: o.State,
		Attempts: int(o.Attempts), NextAttemptAt: o.NextAttemptAt, LastAttemptAt: o.LastAttemptAt, SentAt: o.SentAt, LastError: o.LastError,
		CreatedAt: o.CreatedAt,
	}
	if o.LastStatus.Valid {
		status := int(o.LastStatus.Int32)
		d.LastStatus = &status
	}
	return d
}

func stateText(state *string) pgtype.Text {
	if state == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *state, Valid: true}
}

func valueOf(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}

func (s *Service) ListDeliveries(ctx context.Context, scope models.Scope, state *string, page, limit int) (models.DeliveryPage, error) {
	q := s.queries()
	total, err := q.CountNotificationOutboxByScope(ctx, sqlc.CountNotificationOutboxByScopeParams{OrganizationID: &scope.OrganizationID, ProjectID: scope.ProjectID, State: stateText(state)})
	if err != nil {
		return models.DeliveryPage{}, err
	}
	rows, err := q.NotificationOutboxByScope(ctx, sqlc.NotificationOutboxByScopeParams{
		OrganizationID: &scope.OrganizationID, ProjectID: scope.ProjectID, State: stateText(state), PageLimit: int32(limit), PageOffset: int32((page - 1) * limit), //nolint:gosec // Bounded by the page parameters.
	})
	if err != nil {
		return models.DeliveryPage{}, err
	}
	items := make([]models.Delivery, 0, len(rows))
	for _, r := range rows {
		o := sqlc.NotificationOutbox{
			Id: r.Id, ChannelId: r.ChannelId, EventName: r.EventName, EventId: r.EventId, State: r.State, Attempts: r.Attempts,
			NextAttemptAt: r.NextAttemptAt, LastAttemptAt: r.LastAttemptAt, SentAt: r.SentAt, LastError: r.LastError, LastStatus: r.LastStatus, CreatedAt: r.CreatedAt,
		}
		items = append(items, deliveryOf(o, r.ChannelName, r.ChannelKind))
	}
	return models.DeliveryPage{Items: items, Total: total}, nil
}

func (s *Service) RetryDelivery(ctx context.Context, scope models.Scope, id uuid.UUID) error {
	return dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		row, err := q.GetNotificationDelivery(ctx, sqlc.GetNotificationDeliveryParams{ID: id, OrganizationID: &scope.OrganizationID, ProjectID: scope.ProjectID})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound()
		}
		if err != nil {
			return err
		}
		if row.State != outbox.StateDead {
			return apperrors.DeliveryNotRetryable()
		}
		if !row.ChannelEnabled {
			return apperrors.ChannelDisabled()
		}
		ok, err := outbox.Requeue(ctx, q, scope.OrganizationID, id, s.now())
		if err != nil {
			return err
		}
		if !ok {
			return apperrors.DeliveryNotRetryable()
		}
		return nil
	})
}

func (s *Service) TestChannel(ctx context.Context, scope models.Scope, id uuid.UUID) (uuid.UUID, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	if p.ViaToken() {
		return uuid.Nil, apperrors.SessionRequired()
	}
	if err := s.takeTest(ctx, scope, p.UserID); err != nil {
		return uuid.Nil, err
	}
	event := uuid.New()
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		ch, err := q.GetNotificationChannelForUpdate(ctx, sqlc.GetNotificationChannelForUpdateParams{ID: id, OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound()
		}
		if err != nil {
			return err
		}
		if !ch.Enabled {
			return apperrors.ChannelDisabled()
		}
		now := s.now()
		since := now.Add(-config.NotifyTestWindow)
		sent, err := q.CountNotificationTestsSince(ctx, sqlc.CountNotificationTestsSinceParams{ChannelID: &ch.Id, Since: since})
		if err != nil {
			return err
		}
		if sent >= config.NotifyTestPerMinute {
			return apperrors.TestRateLimited(config.NotifyTestWindow)
		}
		if err := s.enqueueTest(ctx, q, ch, event, p.UserID, now); err != nil {
			return err
		}
		actor := p.UserID
		return s.audit(ctx, auditEntry{
			name: eventbus.NotificationTestSent, org: scope.OrganizationID, actor: &actor, ip: auth.RequestMetaFrom(ctx).IPString(),
			target: targetTest, targetID: ch.Id, tx: q, data: s.channelAudit(ch, scope),
		})
	})
	if err != nil {
		return uuid.Nil, err
	}
	if s.worker != nil {
		s.worker.Wake()
	}
	return event, nil
}

func (s *Service) enqueueTest(ctx context.Context, q *sqlc.Queries, ch sqlc.NotificationChannel, event, actor uuid.UUID, now time.Time) error {
	raw, err := Payload{
		Event: EventTest, EventID: event.String(), Time: now.UTC().Format(time.RFC3339), Status: StatusTest, URL: s.publicURL,
	}.Encode()
	if err != nil {
		return err
	}
	entry := outbox.Entry{
		OrganizationID: ch.OrganizationId, ProjectID: ch.ProjectId, ChannelID: &ch.Id, Kind: outbox.Kind(ch.Kind), EventID: event,
		EventName: EventTest, Payload: raw, NextAttemptAt: now,
	}
	if ch.Kind == models.KindEmail {
		entry.RecipientUserID = &actor
	}
	_, err = outbox.Enqueue(ctx, q, entry)
	return err
}

func (s *Service) SetMute(ctx context.Context, muted bool) error {
	p, err := principalOf(ctx)
	if err != nil {
		return err
	}
	return s.queries().UpsertNotificationMute(ctx, sqlc.UpsertNotificationMuteParams{UserID: p.UserID, Muted: muted})
}

func (s *Service) Muted(ctx context.Context) (bool, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return false, err
	}
	muted, err := s.queries().GetNotificationMute(ctx, p.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return muted, err
}

func (s *Service) takeTest(ctx context.Context, scope models.Scope, user uuid.UUID) error {
	if s.limiter == nil {
		return nil
	}
	for _, l := range []struct {
		limit   ratelimit.Limit
		subject string
	}{
		{ratelimit.Limit{Name: "notify-test-user", Max: config.NotifyTestPerUser, Window: config.NotifyTestWindow}, user.String()},
		{ratelimit.Limit{Name: "notify-test-org", Max: config.NotifyTestPerOrg, Window: config.NotifyTestWindow}, scope.OrganizationID.String()},
	} {
		d, err := s.limiter.Take(ctx, l.limit, l.subject)
		if err != nil {
			return err
		}
		if !d.Allowed {
			return apperrors.TestRateLimited(d.RetryAfter)
		}
	}
	return nil
}
