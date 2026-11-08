package notifications

import (
	"context"

	"github.com/google/uuid"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Models"
)

const (
	messageChannelsListed  = "notifications.channels.listed"
	messageChannelLoaded   = "notifications.channel.loaded"
	messageChannelCreated  = "notifications.channel.created"
	messageChannelUpdated  = "notifications.channel.updated"
	messageChannelRemoved  = "notifications.channel.removed"
	messageTestQueued      = "notifications.test.queued"
	messageRulesLoaded     = "notifications.rules.loaded"
	messageRulesSaved      = "notifications.rules.saved"
	messageDeliveriesListd = "notifications.deliveries.listed"
	messageDeliveryRetried = "notifications.delivery.retried"
	messageMuteSaved       = "notifications.mute.saved"
)

type Controller struct {
	service *Service
}

var _ gen.StrictServerInterface = (*Controller)(nil)

func NewController(service *Service) *Controller { return &Controller{service: service} }

func ptrOf[T any](v T) *T { return &v }

func orgScope(org uuid.UUID) models.Scope { return models.Scope{OrganizationID: org} }

func projectScope(org, project uuid.UUID) models.Scope {
	return models.Scope{OrganizationID: org, ProjectID: &project}
}

func factorOf(f gen.CurrentFactor) auth.Factor {
	out := auth.Factor{Method: string(f.Method)}
	if f.Password != nil {
		out.Password = *f.Password
	}
	if f.Code != nil {
		out.Code = *f.Code
	}
	if f.ChallengeId != nil {
		out.ChallengeID = *f.ChallengeId
	}
	if f.Credential != nil {
		out.Credential = *f.Credential
	}
	return out
}

func channelView(c models.Channel) gen.NotificationChannel {
	out := gen.NotificationChannel{
		Id: c.ID, ProjectId: c.ProjectID, Kind: gen.NotificationChannelKind(c.Kind), Name: c.Name, Locale: gen.NotificationChannelLocale(c.Locale),
		Enabled: c.Enabled, HasSecret: c.HasSecret, Version: c.Version, ConsecutiveFailures: c.ConsecutiveFailures,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
	if c.Host != "" {
		out.Host = &c.Host
	}
	if c.CreatedByUserID != uuid.Nil {
		out.CreatedByUserId = &c.CreatedByUserID
	}
	if c.DisabledReason != "" {
		out.DisabledReason = ptrOf(gen.NotificationChannelDisabledReason(c.DisabledReason))
	}
	return out
}

func ruleView(r models.Rule) gen.NotificationRule {
	events := make([]gen.NotificationRuleEvents, 0, len(r.Events))
	for _, e := range r.Events {
		events = append(events, gen.NotificationRuleEvents(e))
	}
	statuses := make([]gen.NotificationRuleStatuses, 0, len(r.Statuses))
	for _, v := range r.Statuses {
		statuses = append(statuses, gen.NotificationRuleStatuses(v))
	}
	return gen.NotificationRule{
		ChannelId: r.ChannelID, Events: events, Environments: nonNil(r.Environments), Statuses: statuses, Roles: nonNil(r.Roles), Version: r.Version,
	}
}

func deliveryView(d models.Delivery) gen.NotificationDelivery {
	return gen.NotificationDelivery{
		Id: d.ID, ChannelId: d.ChannelID, ChannelName: d.ChannelName, ChannelKind: gen.NotificationChannelKind(d.ChannelKind), Event: d.Event,
		EventId: d.EventID, State: gen.NotificationDeliveryState(d.State), Attempts: d.Attempts, NextAttemptAt: d.NextAttemptAt,
		LastAttemptAt: d.LastAttemptAt, SentAt: d.SentAt, LastError: d.LastError, LastStatus: d.LastStatus, CreatedAt: d.CreatedAt,
	}
}

func ruleInputs(in []gen.NotificationRuleInput) []models.Rule {
	out := make([]models.Rule, 0, len(in))
	for _, r := range in {
		rule := models.Rule{ChannelID: r.ChannelId}
		for _, e := range r.Events {
			rule.Events = append(rule.Events, string(e))
		}
		for _, v := range r.Statuses {
			rule.Statuses = append(rule.Statuses, string(v))
		}
		if r.Environments != nil {
			rule.Environments = *r.Environments
		}
		if r.Roles != nil {
			for _, v := range *r.Roles {
				rule.Roles = append(rule.Roles, string(v))
			}
		}
		out = append(out, rule)
	}
	return out
}

func (c *Controller) channelList(ctx context.Context, scope models.Scope, page, limit *int) (gen.NotificationChannelListResponse, error) {
	p, l := response.PageParams(page, limit)
	list, err := c.service.ListChannels(ctx, scope, p, l)
	if err != nil {
		return gen.NotificationChannelListResponse{}, err
	}
	items := make([]gen.NotificationChannel, 0, len(list.Items))
	for _, ch := range list.Items {
		items = append(items, channelView(ch))
	}
	return gen.NotificationChannelListResponse{
		Success: true, Message: ptrOf(messageChannelsListed), Data: &items, Meta: &gen.PageMeta{Page: p, Limit: l, Total: int(list.Total)},
	}, nil
}

func (c *Controller) channelCreate(ctx context.Context, scope models.Scope, b *gen.CreateNotificationChannelRequest) (gen.CreatedNotificationChannelResponse, error) {
	if b == nil {
		return gen.CreatedNotificationChannelResponse{}, apperrors.BadRequest()
	}
	in := models.CreateChannelInput{Kind: string(b.Kind), Name: b.Name}
	if b.Locale != nil {
		in.Locale = string(*b.Locale)
	}
	if b.Url != nil {
		in.URL = *b.Url
	}
	if b.BotToken != nil {
		in.BotToken = *b.BotToken
	}
	if b.ChatId != nil {
		in.ChatID = *b.ChatId
	}
	created, err := c.service.CreateChannel(ctx, scope, in, factorOf(b.CurrentFactor))
	if err != nil {
		return gen.CreatedNotificationChannelResponse{}, err
	}
	return createdView(created, messageChannelCreated), nil
}

func createdView(created models.CreatedChannel, message string) gen.CreatedNotificationChannelResponse {
	data := gen.CreatedNotificationChannel{Channel: channelView(created.Channel)}
	if created.Secret != "" {
		data.Secret = &created.Secret
	}
	return gen.CreatedNotificationChannelResponse{Success: true, Message: &message, Data: &data}
}

func (c *Controller) channelGet(ctx context.Context, scope models.Scope, id uuid.UUID) (gen.NotificationChannelResponse, error) {
	ch, err := c.service.GetChannel(ctx, scope, id)
	if err != nil {
		return gen.NotificationChannelResponse{}, err
	}
	view := channelView(ch)
	return gen.NotificationChannelResponse{Success: true, Message: ptrOf(messageChannelLoaded), Data: &view}, nil
}

func (c *Controller) channelUpdate(ctx context.Context, scope models.Scope, id uuid.UUID, b *gen.UpdateNotificationChannelRequest) (gen.CreatedNotificationChannelResponse, error) {
	if b == nil {
		return gen.CreatedNotificationChannelResponse{}, apperrors.BadRequest()
	}
	in := models.UpdateChannelInput{Name: b.Name, Locale: string(b.Locale), Version: b.Version, URL: b.Url, BotToken: b.BotToken, ChatID: b.ChatId}
	if b.RotateSecret != nil {
		in.RotateSecret = *b.RotateSecret
	}
	if b.Enable != nil {
		in.Enable = *b.Enable
	}
	updated, err := c.service.UpdateChannel(ctx, scope, id, in, factorOf(b.CurrentFactor))
	if err != nil {
		return gen.CreatedNotificationChannelResponse{}, err
	}
	return createdView(updated, messageChannelUpdated), nil
}

func (c *Controller) channelRemove(ctx context.Context, scope models.Scope, id uuid.UUID, b *gen.CurrentFactorRequest) (gen.NotificationChannelResponse, error) {
	if b == nil {
		return gen.NotificationChannelResponse{}, apperrors.BadRequest()
	}
	ch, err := c.service.RemoveChannel(ctx, scope, id, factorOf(b.CurrentFactor))
	if err != nil {
		return gen.NotificationChannelResponse{}, err
	}
	view := channelView(ch)
	return gen.NotificationChannelResponse{Success: true, Message: ptrOf(messageChannelRemoved), Data: &view}, nil
}

func (c *Controller) channelTest(ctx context.Context, scope models.Scope, id uuid.UUID) (gen.NotificationTestResponse, error) {
	event, err := c.service.TestChannel(ctx, scope, id)
	if err != nil {
		return gen.NotificationTestResponse{}, err
	}
	out := gen.NotificationTestResponse{Success: true, Message: ptrOf(messageTestQueued)}
	out.Data = &struct {
		EventId uuid.UUID `json:"EventId"`
	}{EventId: event}
	return out, nil
}

func (c *Controller) rulesGet(ctx context.Context, scope models.Scope) (gen.NotificationRuleListResponse, error) {
	rules, err := c.service.GetRules(ctx, scope)
	if err != nil {
		return gen.NotificationRuleListResponse{}, err
	}
	return rulesResponse(rules, messageRulesLoaded), nil
}

func rulesResponse(rules []models.Rule, message string) gen.NotificationRuleListResponse {
	items := make([]gen.NotificationRule, 0, len(rules))
	for _, r := range rules {
		items = append(items, ruleView(r))
	}
	return gen.NotificationRuleListResponse{Success: true, Message: &message, Data: &items}
}

func (c *Controller) rulesPut(ctx context.Context, scope models.Scope, b *gen.PutNotificationRulesRequest) (gen.NotificationRuleListResponse, error) {
	if b == nil {
		return gen.NotificationRuleListResponse{}, apperrors.BadRequest()
	}
	saved, err := c.service.PutRules(ctx, scope, ruleInputs(b.Rules))
	if err != nil {
		return gen.NotificationRuleListResponse{}, err
	}
	return rulesResponse(saved, messageRulesSaved), nil
}

func (c *Controller) deliveries(ctx context.Context, scope models.Scope, page, limit *int, state *gen.DeliveryState) (gen.NotificationDeliveryListResponse, error) {
	p, l := response.PageParams(page, limit)
	var filter *string
	if state != nil {
		filter = ptrOf(string(*state))
	}
	list, err := c.service.ListDeliveries(ctx, scope, filter, p, l)
	if err != nil {
		return gen.NotificationDeliveryListResponse{}, err
	}
	items := make([]gen.NotificationDelivery, 0, len(list.Items))
	for _, d := range list.Items {
		items = append(items, deliveryView(d))
	}
	return gen.NotificationDeliveryListResponse{
		Success: true, Message: ptrOf(messageDeliveriesListd), Data: &items, Meta: &gen.PageMeta{Page: p, Limit: l, Total: int(list.Total)},
	}, nil
}

func (c *Controller) retry(ctx context.Context, scope models.Scope, id uuid.UUID) (gen.NotificationAckResponse, error) {
	if err := c.service.RetryDelivery(ctx, scope, id); err != nil {
		return gen.NotificationAckResponse{}, err
	}
	out := gen.NotificationAckResponse{Success: true, Message: ptrOf(messageDeliveryRetried)}
	out.Data = &struct {
		Retried *bool `json:"Retried,omitempty"`
	}{Retried: ptrOf(true)}
	return out, nil
}

func (c *Controller) GetNotificationMute(ctx context.Context, _ gen.GetNotificationMuteRequestObject) (gen.GetNotificationMuteResponseObject, error) {
	muted, err := c.service.Muted(ctx)
	if err != nil {
		return nil, err
	}
	return gen.GetNotificationMute200JSONResponse{Success: true, Data: &gen.NotificationMute{Muted: muted}}, nil
}

func (c *Controller) SetNotificationMute(ctx context.Context, req gen.SetNotificationMuteRequestObject) (gen.SetNotificationMuteResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	if err := c.service.SetMute(ctx, req.Body.Muted); err != nil {
		return nil, err
	}
	return gen.SetNotificationMute200JSONResponse{Success: true, Message: ptrOf(messageMuteSaved), Data: &gen.NotificationMute{Muted: req.Body.Muted}}, nil
}
