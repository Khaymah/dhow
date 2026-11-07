package notifications

import (
	"context"

	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Gen"
)

func (c *Controller) ListNotificationChannels(ctx context.Context, req gen.ListNotificationChannelsRequestObject) (gen.ListNotificationChannelsResponseObject, error) {
	r, err := c.channelList(ctx, projectScope(req.Id, req.ProjectId), req.Params.Page, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return gen.ListNotificationChannels200JSONResponse(r), nil
}

func (c *Controller) CreateNotificationChannel(ctx context.Context, req gen.CreateNotificationChannelRequestObject) (gen.CreateNotificationChannelResponseObject, error) {
	r, err := c.channelCreate(ctx, projectScope(req.Id, req.ProjectId), req.Body)
	if err != nil {
		return nil, err
	}
	return gen.CreateNotificationChannel201JSONResponse(r), nil
}

func (c *Controller) GetNotificationChannel(ctx context.Context, req gen.GetNotificationChannelRequestObject) (gen.GetNotificationChannelResponseObject, error) {
	r, err := c.channelGet(ctx, projectScope(req.Id, req.ProjectId), req.ChannelId)
	if err != nil {
		return nil, err
	}
	return gen.GetNotificationChannel200JSONResponse(r), nil
}

func (c *Controller) UpdateNotificationChannel(ctx context.Context, req gen.UpdateNotificationChannelRequestObject) (gen.UpdateNotificationChannelResponseObject, error) {
	r, err := c.channelUpdate(ctx, projectScope(req.Id, req.ProjectId), req.ChannelId, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.UpdateNotificationChannel200JSONResponse(r), nil
}

func (c *Controller) RemoveNotificationChannel(ctx context.Context, req gen.RemoveNotificationChannelRequestObject) (gen.RemoveNotificationChannelResponseObject, error) {
	r, err := c.channelRemove(ctx, projectScope(req.Id, req.ProjectId), req.ChannelId, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.RemoveNotificationChannel200JSONResponse(r), nil
}

func (c *Controller) TestNotificationChannel(ctx context.Context, req gen.TestNotificationChannelRequestObject) (gen.TestNotificationChannelResponseObject, error) {
	r, err := c.channelTest(ctx, projectScope(req.Id, req.ProjectId), req.ChannelId)
	if err != nil {
		return nil, err
	}
	return gen.TestNotificationChannel202JSONResponse(r), nil
}

func (c *Controller) GetNotificationRules(ctx context.Context, req gen.GetNotificationRulesRequestObject) (gen.GetNotificationRulesResponseObject, error) {
	r, err := c.rulesGet(ctx, projectScope(req.Id, req.ProjectId))
	if err != nil {
		return nil, err
	}
	return gen.GetNotificationRules200JSONResponse(r), nil
}

func (c *Controller) PutNotificationRules(ctx context.Context, req gen.PutNotificationRulesRequestObject) (gen.PutNotificationRulesResponseObject, error) {
	r, err := c.rulesPut(ctx, projectScope(req.Id, req.ProjectId), req.Body)
	if err != nil {
		return nil, err
	}
	return gen.PutNotificationRules200JSONResponse(r), nil
}

func (c *Controller) ListNotificationDeliveries(ctx context.Context, req gen.ListNotificationDeliveriesRequestObject) (gen.ListNotificationDeliveriesResponseObject, error) {
	r, err := c.deliveries(ctx, projectScope(req.Id, req.ProjectId), req.Params.Page, req.Params.Limit, req.Params.State)
	if err != nil {
		return nil, err
	}
	return gen.ListNotificationDeliveries200JSONResponse(r), nil
}

func (c *Controller) RetryNotificationDelivery(ctx context.Context, req gen.RetryNotificationDeliveryRequestObject) (gen.RetryNotificationDeliveryResponseObject, error) {
	r, err := c.retry(ctx, projectScope(req.Id, req.ProjectId), req.DeliveryId)
	if err != nil {
		return nil, err
	}
	return gen.RetryNotificationDelivery200JSONResponse(r), nil
}

func (c *Controller) ListOrgNotificationChannels(ctx context.Context, req gen.ListOrgNotificationChannelsRequestObject) (gen.ListOrgNotificationChannelsResponseObject, error) {
	r, err := c.channelList(ctx, orgScope(req.Id), req.Params.Page, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return gen.ListOrgNotificationChannels200JSONResponse(r), nil
}

func (c *Controller) CreateOrgNotificationChannel(ctx context.Context, req gen.CreateOrgNotificationChannelRequestObject) (gen.CreateOrgNotificationChannelResponseObject, error) {
	r, err := c.channelCreate(ctx, orgScope(req.Id), req.Body)
	if err != nil {
		return nil, err
	}
	return gen.CreateOrgNotificationChannel201JSONResponse(r), nil
}

func (c *Controller) GetOrgNotificationChannel(ctx context.Context, req gen.GetOrgNotificationChannelRequestObject) (gen.GetOrgNotificationChannelResponseObject, error) {
	r, err := c.channelGet(ctx, orgScope(req.Id), req.ChannelId)
	if err != nil {
		return nil, err
	}
	return gen.GetOrgNotificationChannel200JSONResponse(r), nil
}

func (c *Controller) UpdateOrgNotificationChannel(ctx context.Context, req gen.UpdateOrgNotificationChannelRequestObject) (gen.UpdateOrgNotificationChannelResponseObject, error) {
	r, err := c.channelUpdate(ctx, orgScope(req.Id), req.ChannelId, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.UpdateOrgNotificationChannel200JSONResponse(r), nil
}

func (c *Controller) RemoveOrgNotificationChannel(ctx context.Context, req gen.RemoveOrgNotificationChannelRequestObject) (gen.RemoveOrgNotificationChannelResponseObject, error) {
	r, err := c.channelRemove(ctx, orgScope(req.Id), req.ChannelId, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.RemoveOrgNotificationChannel200JSONResponse(r), nil
}

func (c *Controller) TestOrgNotificationChannel(ctx context.Context, req gen.TestOrgNotificationChannelRequestObject) (gen.TestOrgNotificationChannelResponseObject, error) {
	r, err := c.channelTest(ctx, orgScope(req.Id), req.ChannelId)
	if err != nil {
		return nil, err
	}
	return gen.TestOrgNotificationChannel202JSONResponse(r), nil
}

func (c *Controller) GetOrgNotificationRules(ctx context.Context, req gen.GetOrgNotificationRulesRequestObject) (gen.GetOrgNotificationRulesResponseObject, error) {
	r, err := c.rulesGet(ctx, orgScope(req.Id))
	if err != nil {
		return nil, err
	}
	return gen.GetOrgNotificationRules200JSONResponse(r), nil
}

func (c *Controller) PutOrgNotificationRules(ctx context.Context, req gen.PutOrgNotificationRulesRequestObject) (gen.PutOrgNotificationRulesResponseObject, error) {
	r, err := c.rulesPut(ctx, orgScope(req.Id), req.Body)
	if err != nil {
		return nil, err
	}
	return gen.PutOrgNotificationRules200JSONResponse(r), nil
}

func (c *Controller) ListOrgNotificationDeliveries(ctx context.Context, req gen.ListOrgNotificationDeliveriesRequestObject) (gen.ListOrgNotificationDeliveriesResponseObject, error) {
	r, err := c.deliveries(ctx, orgScope(req.Id), req.Params.Page, req.Params.Limit, req.Params.State)
	if err != nil {
		return nil, err
	}
	return gen.ListOrgNotificationDeliveries200JSONResponse(r), nil
}

func (c *Controller) RetryOrgNotificationDelivery(ctx context.Context, req gen.RetryOrgNotificationDeliveryRequestObject) (gen.RetryOrgNotificationDeliveryResponseObject, error) {
	r, err := c.retry(ctx, orgScope(req.Id), req.DeliveryId)
	if err != nil {
		return nil, err
	}
	return gen.RetryOrgNotificationDelivery200JSONResponse(r), nil
}
