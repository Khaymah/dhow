package projects

import (
	"context"

	"github.com/google/uuid"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Projects/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Projects/Models"
)

const (
	messageListed   = "projects.list.loaded"
	messageLoaded   = "projects.project.loaded"
	messageCreated  = "projects.project.created"
	messageUpdated  = "projects.project.updated"
	messageArchived = "projects.project.archived"
)

type Controller struct {
	service *Service
}

var _ gen.StrictServerInterface = (*Controller)(nil)

func NewController(service *Service) *Controller { return &Controller{service: service} }

func ptrOf[T any](v T) *T { return &v }

func toGen(p models.Project, linked bool) gen.Project {
	return gen.Project{
		Id: p.ID, OrganizationId: p.OrganizationID, Slug: p.Slug, Name: p.Name, PipelinePath: p.PipelinePath,
		ForkPolicy: gen.ForkPolicy(p.ForkPolicy), State: gen.ProjectState(p.State), RepositoryLinked: linked,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, ArchivedAt: p.ArchivedAt,
	}
}

func (c *Controller) one(ctx context.Context, org uuid.UUID, p models.Project) (gen.Project, error) {
	linked, err := c.service.Linked(ctx, org, p.ID)
	if err != nil {
		return gen.Project{}, err
	}
	return withCallerRole(ctx, org, p.ID, toGen(p, linked[p.ID])), nil
}

func withCallerRole(ctx context.Context, org, id uuid.UUID, data gen.Project) gen.Project {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return data
	}
	if role := projectRole(p, org, id); role != "" {
		data.CallerRole = ptrOf(gen.Role(role))
	}
	return data
}

func policyOf(v *gen.ForkPolicy) *string {
	if v == nil {
		return nil
	}
	return ptrOf(string(*v))
}

func (c *Controller) ListProjects(ctx context.Context, req gen.ListProjectsRequestObject) (gen.ListProjectsResponseObject, error) {
	page, limit := response.PageParams(req.Params.Page, req.Params.Limit)
	state := ""
	if req.Params.State != nil {
		state = string(*req.Params.State)
		if state != models.StateActive && state != models.StateArchived {
			return nil, apperrors.ValidationFailed(invalid("State"))
		}
	}
	list, err := c.service.List(ctx, req.Id, state, page, limit)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(list.Items))
	for _, p := range list.Items {
		ids = append(ids, p.ID)
	}
	linked, err := c.service.Linked(ctx, req.Id, ids...)
	if err != nil {
		return nil, err
	}
	items := make([]gen.Project, 0, len(list.Items))
	for _, p := range list.Items {
		items = append(items, toGen(p, linked[p.ID]))
	}
	return gen.ListProjects200JSONResponse{
		Success: true, Message: ptrOf(messageListed), Data: &items, Meta: &gen.PageMeta{Page: page, Limit: limit, Total: int(list.Total)},
	}, nil
}

func (c *Controller) CreateProject(ctx context.Context, req gen.CreateProjectRequestObject) (gen.CreateProjectResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	p, err := c.service.Create(ctx, req.Id, models.CreateInput{
		Name: req.Body.Name, Slug: req.Body.Slug, PipelinePath: req.Body.PipelinePath, ForkPolicy: policyOf(req.Body.ForkPolicy),
	})
	if err != nil {
		return nil, err
	}
	data := toGen(p, false)
	return gen.CreateProject201JSONResponse{Success: true, Message: ptrOf(messageCreated), Data: &data}, nil
}

func (c *Controller) GetProject(ctx context.Context, req gen.GetProjectRequestObject) (gen.GetProjectResponseObject, error) {
	p, err := c.service.Get(ctx, req.Id, req.ProjectId)
	if err != nil {
		return nil, err
	}
	data, err := c.one(ctx, req.Id, p)
	if err != nil {
		return nil, err
	}
	return gen.GetProject200JSONResponse{Success: true, Message: ptrOf(messageLoaded), Data: &data}, nil
}

func (c *Controller) UpdateProject(ctx context.Context, req gen.UpdateProjectRequestObject) (gen.UpdateProjectResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	p, err := c.service.Update(ctx, req.Id, req.ProjectId, models.UpdateInput{
		Name: req.Body.Name, PipelinePath: req.Body.PipelinePath, ForkPolicy: policyOf(req.Body.ForkPolicy),
	})
	if err != nil {
		return nil, err
	}
	data, err := c.one(ctx, req.Id, p)
	if err != nil {
		return nil, err
	}
	return gen.UpdateProject200JSONResponse{Success: true, Message: ptrOf(messageUpdated), Data: &data}, nil
}

func (c *Controller) ArchiveProject(ctx context.Context, req gen.ArchiveProjectRequestObject) (gen.ArchiveProjectResponseObject, error) {
	p, err := c.service.Archive(ctx, req.Id, req.ProjectId)
	if err != nil {
		return nil, err
	}
	data, err := c.one(ctx, req.Id, p)
	if err != nil {
		return nil, err
	}
	return gen.ArchiveProject200JSONResponse{Success: true, Message: ptrOf(messageArchived), Data: &data}, nil
}
