package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

var errInboxSignal = errors.New("the live inbox signal was not sent")

func (s *Service) inboxChanged(ctx context.Context, q *sqlc.Queries, user uuid.UUID, org *uuid.UUID) error {
	if s.bus == nil {
		return nil
	}
	target := org
	if target == nil {
		first, err := q.FirstNotificationUserOrganization(ctx, user)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: find the organization of the user: %w", errInboxSignal, err)
		}
		target = &first
	}
	err := s.bus.Publish(ctx, eventbus.Event{Name: eventbus.NotificationInboxChanged, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{*target}, TargetType: targetInbox, TargetID: user.String(), Tx: q,
	}})
	if err != nil {
		return fmt.Errorf("%w: %w", errInboxSignal, err)
	}
	return nil
}

func (s *Service) takeInboxWrite(ctx context.Context, user uuid.UUID) error {
	if s.limiter == nil {
		return nil
	}
	d, err := s.limiter.Take(ctx, ratelimit.Limit{Name: "inbox-write", Max: config.NotifyInboxWritesPerMinute, Window: time.Minute}, user.String())
	if err != nil {
		return fmt.Errorf("take the inbox write limit: %w", err)
	}
	if !d.Allowed {
		return apperrors.RateLimited(d.RetryAfter)
	}
	return nil
}

func (s *Service) signalInbox(ctx context.Context, q *sqlc.Queries, user uuid.UUID, org *uuid.UUID) {
	if err := s.inboxChanged(ctx, q, user, org); err != nil {
		s.log.WarnContext(ctx, "the live inbox signal was not sent", "error", err)
	}
}
