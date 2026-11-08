package notifications

import (
	"context"

	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Models"
)

const (
	messageInboxListed   = "notifications.inbox.listed"
	messageInboxUnread   = "notifications.inbox.unread"
	messageInboxRead     = "notifications.inbox.read"
	messageInboxReadAll  = "notifications.inbox.readAll"
	inboxParamsPerNotice = 16
)

func inboxView(i models.InboxItem) gen.InboxNotification {
	params := make(map[string]string, len(i.Params))
	for k, v := range i.Params {
		if len(params) < inboxParamsPerNotice {
			params[k] = v
		}
	}
	out := gen.InboxNotification{
		Id: i.ID, OrganizationId: i.OrganizationID, EventId: i.EventID, Event: i.Event, TitleKey: i.TitleKey, Params: params,
		Read: i.Read, ReadAt: i.ReadAt, CreatedAt: i.CreatedAt,
	}
	if i.Link != "" {
		out.Link = &i.Link
	}
	return out
}

func (c *Controller) ListInboxNotifications(ctx context.Context, req gen.ListInboxNotificationsRequestObject) (gen.ListInboxNotificationsResponseObject, error) {
	p, l := response.PageParams(req.Params.Page, req.Params.Limit)
	unread := req.Params.Unread != nil && *req.Params.Unread
	list, err := c.service.ListInbox(ctx, unread, p, l)
	if err != nil {
		return nil, err
	}
	items := make([]gen.InboxNotification, 0, len(list.Items))
	for _, i := range list.Items {
		items = append(items, inboxView(i))
	}
	return gen.ListInboxNotifications200JSONResponse{
		Success: true, Message: ptrOf(messageInboxListed), Data: &items, Meta: &gen.PageMeta{Page: p, Limit: l, Total: int(list.Total)},
	}, nil
}

func (c *Controller) GetInboxUnreadCount(ctx context.Context, _ gen.GetInboxUnreadCountRequestObject) (gen.GetInboxUnreadCountResponseObject, error) {
	n, err := c.service.InboxUnread(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.GetInboxUnreadCount200JSONResponse{Success: true, Message: ptrOf(messageInboxUnread)}
	out.Data = &struct {
		Unread int `json:"Unread"`
	}{Unread: int(n)}
	return out, nil
}

func (c *Controller) MarkInboxNotificationRead(ctx context.Context, req gen.MarkInboxNotificationReadRequestObject) (gen.MarkInboxNotificationReadResponseObject, error) {
	item, err := c.service.MarkInboxRead(ctx, req.NotificationId)
	if err != nil {
		return nil, err
	}
	view := inboxView(item)
	return gen.MarkInboxNotificationRead200JSONResponse{Success: true, Message: ptrOf(messageInboxRead), Data: &view}, nil
}

func (c *Controller) MarkAllInboxNotificationsRead(ctx context.Context, _ gen.MarkAllInboxNotificationsReadRequestObject) (gen.MarkAllInboxNotificationsReadResponseObject, error) {
	n, err := c.service.MarkAllInboxRead(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.MarkAllInboxNotificationsRead200JSONResponse{Success: true, Message: ptrOf(messageInboxReadAll)}
	out.Data = &struct {
		Marked int `json:"Marked"`
	}{Marked: int(n)}
	return out, nil
}
