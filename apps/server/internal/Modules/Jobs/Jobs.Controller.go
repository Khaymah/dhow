package jobs

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs/Models"
)

const (
	messageListed          = "jobs.list.loaded"
	messageLoaded          = "jobs.job.loaded"
	messageCreated         = "jobs.job.created"
	messageCancelRequested = "jobs.job.cancelRequested"
)

type Controller struct {
	service *Service
	guard   Reauthorizer
}

type Reauthorizer interface {
	Reauthorize(ctx context.Context, action policy.Action, org uuid.UUID) error
	AuthorizeProject(ctx context.Context, action policy.Action, org, project uuid.UUID) error
}

func (c *Controller) authorizeJob(ctx context.Context, action policy.Action, org, job uuid.UUID) error {
	project, found, err := c.service.ProjectOfJob(ctx, org, job)
	if err != nil || !found {
		return err
	}
	return c.guard.AuthorizeProject(ctx, action, org, project)
}

var _ gen.StrictServerInterface = (*Controller)(nil)

func NewController(service *Service, guard Reauthorizer) *Controller {
	return &Controller{service: service, guard: guard}
}

func (c *Controller) ListJobs(ctx context.Context, req gen.ListJobsRequestObject) (gen.ListJobsResponseObject, error) {
	page, limit := response.PageParams(req.Params.Page, req.Params.Limit)
	var raw []string
	if req.Params.State != nil {
		raw = *req.Params.State
	}
	states, err := stateFilter(raw)
	if err != nil {
		return nil, err
	}
	list, total, err := c.service.List(ctx, req.Id, states, page, limit)
	if err != nil {
		return nil, err
	}
	items := make([]gen.Job, 0, len(list))
	for _, j := range list {
		items = append(items, jobToGen(j))
	}
	return gen.ListJobs200JSONResponse{
		Success: true, Message: ptrOf(messageListed), Data: &items, Meta: &gen.PageMeta{Page: page, Limit: limit, Total: int(total)},
	}, nil
}

func stateFilter(raw []string) ([]string, error) {
	out := []string{}
	for _, part := range raw {
		for v := range strings.SplitSeq(part, ",") {
			v = strings.TrimSpace(v)
			if !slices.Contains(models.JobStates(), v) {
				return nil, apperrors.ValidationFailed(invalid("State"))
			}
			if !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	return out, nil
}

func (c *Controller) CreateJob(ctx context.Context, req gen.CreateJobRequestObject) (gen.CreateJobResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	spec, err := specFromGen(req.Body.Spec)
	if err != nil {
		return nil, err
	}
	d, err := c.service.Create(ctx, req.Id, models.CreateInput{
		Name: req.Body.Name, Spec: spec, Priority: req.Body.Priority, MaxInfraRetries: req.Body.MaxInfraRetries,
	})
	if err != nil {
		return nil, err
	}
	data := detailToGen(d)
	return gen.CreateJob201JSONResponse{Success: true, Message: ptrOf(messageCreated), Data: &data}, nil
}

func (c *Controller) GetJob(ctx context.Context, req gen.GetJobRequestObject) (gen.GetJobResponseObject, error) {
	if err := c.authorizeJob(ctx, policy.JobRead, req.Id, req.JobId); err != nil {
		return nil, err
	}
	d, err := c.service.Get(ctx, req.Id, req.JobId)
	if err != nil {
		return nil, err
	}
	data := detailToGen(d)
	if err := c.withOrigin(ctx, req.Id, req.JobId, &data); err != nil {
		return nil, err
	}
	return gen.GetJob200JSONResponse{Success: true, Message: ptrOf(messageLoaded), Data: &data}, nil
}

func (c *Controller) CancelJob(ctx context.Context, req gen.CancelJobRequestObject) (gen.CancelJobResponseObject, error) {
	if err := c.authorizeJob(ctx, policy.JobCancel, req.Id, req.JobId); err != nil {
		return nil, err
	}
	j, err := c.service.Cancel(ctx, req.Id, req.JobId)
	if err != nil {
		return nil, err
	}
	data := jobToGen(j)
	return gen.CancelJob200JSONResponse{Success: true, Message: ptrOf(messageCancelRequested), Data: &data}, nil
}
