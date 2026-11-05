package overview

import (
	"context"

	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Overview/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Overview/Models"
)

const messageLoaded = "overview.loaded"

type Controller struct {
	service *Service
}

var _ gen.StrictServerInterface = (*Controller)(nil)

func NewController(service *Service) *Controller { return &Controller{service: service} }

func ptr[T any](v T) *T { return &v }

func (c *Controller) GetOrganizationOverview(ctx context.Context, req gen.GetOrganizationOverviewRequestObject) (gen.GetOrganizationOverviewResponseObject, error) {
	o, err := c.service.Get(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.GetOrganizationOverview200JSONResponse{Success: true, Message: ptr(messageLoaded), Data: toGen(o)}, nil
}

func toGen(o models.Overview) *gen.OrganizationOverview {
	out := &gen.OrganizationOverview{}
	if o.Jobs != nil {
		out.Jobs = &gen.OverviewJobs{
			Running: o.Jobs.Running, Queued: o.Jobs.Queued, QueuedNoRunner: o.Jobs.QueuedNoRunner, FailedLast24h: o.Jobs.FailedLast24h,
		}
	}
	if o.Runners != nil {
		out.Runners = &gen.OverviewRunners{Online: o.Runners.Online, Offline: o.Runners.Offline, Paused: o.Runners.Paused}
	}
	if o.Approvals != nil {
		out.Approvals = &gen.OverviewApprovals{Pending: o.Approvals.Pending}
	}
	if o.Invitations != nil {
		out.Invitations = &gen.OverviewInvitations{Pending: o.Invitations.Pending}
	}
	return out
}
