package notifications

import (
	"context"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

func (s *Service) inboxChanged(ctx context.Context, q *sqlc.Queries, user uuid.UUID) {
	if s.bus == nil {
		return
	}
	orgs, err := q.ListNotificationUserOrganizations(ctx, sqlc.ListNotificationUserOrganizationsParams{
		UserID: user, MaxOrganizations: config.NotifyInboxLiveOrgs,
	})
	if err != nil {
		s.log.WarnContext(ctx, "the live inbox signal could not find the organizations of the user", "error", err)
		return
	}
	for _, org := range orgs {
		err := s.bus.Publish(ctx, eventbus.Event{Name: eventbus.NotificationInboxChanged, Payload: eventbus.Audited{
			OrganizationIDs: []uuid.UUID{org}, TargetType: targetInbox, TargetID: user.String(), Tx: q,
		}})
		if err != nil {
			s.log.WarnContext(ctx, "the live inbox signal was not sent", "error", err)
		}
	}
}
