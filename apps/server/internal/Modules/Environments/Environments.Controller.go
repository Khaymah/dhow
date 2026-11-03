package environments

import (
	"context"

	"github.com/google/uuid"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
)

const (
	messageListed  = "environments.list.loaded"
	messageLoaded  = "environments.environment.loaded"
	messageCreated = "environments.environment.created"
	messageUpdated = "environments.environment.updated"
	messageRemoved = "environments.environment.removed"
	messagePaused  = "environments.environment.paused"
	messageResumed = "environments.environment.resumed"
)

type Controller struct {
	service *Service
}

var _ gen.StrictServerInterface = (*Controller)(nil)

func NewController(service *Service) *Controller { return &Controller{service: service} }

func ptrOf[T any](v T) *T { return &v }

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

func rulesInputOf(in *gen.EnvironmentRulesInput) *models.RulesInput {
	if in == nil {
		return nil
	}
	out := &models.RulesInput{
		AllowPullRequests: in.AllowPullRequests, RequiredApprovals: in.RequiredApprovals, AllowSelfApproval: in.AllowSelfApproval,
		WaitMinutes: in.WaitMinutes, ApprovalTimeoutHours: in.ApprovalTimeoutHours, RequirePinnedImages: in.RequirePinnedImages,
		AllowedPlugins: in.AllowedPlugins, AllowAdminBypass: in.AllowAdminBypass, SupersedePending: in.SupersedePending,
	}
	if in.AllowedRefs != nil {
		refs := make([]models.Ref, 0, len(*in.AllowedRefs))
		for _, r := range *in.AllowedRefs {
			refs = append(refs, models.Ref{Kind: string(r.Kind), Pattern: r.Pattern})
		}
		out.AllowedRefs = &refs
	}
	if in.Reviewers != nil {
		out.Reviewers = &models.Reviewers{UserIDs: in.Reviewers.UserIds, TeamIDs: in.Reviewers.TeamIds}
	}
	if in.RequiredIsolation != nil {
		out.RequiredIsolation = ptrOf(string(*in.RequiredIsolation))
	}
	return out
}

func showsReviewerIDs(ctx context.Context, org, project uuid.UUID) bool {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p.ViaToken() {
		return false
	}
	role, _ := p.RoleIn(org)
	if p.ProjectID == project && p.ProjectRole != "" {
		role = p.ProjectRole
	}
	return policy.Allows(role, policy.EnvironmentManage)
}

func reviewersOf(r models.Reviewers, show bool) gen.EnvironmentReviewersView {
	out := gen.EnvironmentReviewersView{
		UserIds: []uuid.UUID{}, TeamIds: []uuid.UUID{}, UserCount: len(r.UserIDs), TeamCount: len(r.TeamIDs), IdsShown: show,
	}
	if show {
		out.UserIds, out.TeamIds = r.UserIDs, r.TeamIDs
	}
	return out
}

func toGen(ctx context.Context, e models.Environment) gen.Environment {
	r := e.Rules
	refs := make([]gen.EnvironmentRef, 0, len(r.AllowedRefs))
	for _, ref := range r.AllowedRefs {
		refs = append(refs, gen.EnvironmentRef{Kind: gen.EnvironmentRefKind(ref.Kind), Pattern: ref.Pattern})
	}
	out := gen.Environment{
		Id: e.ID, OrganizationId: e.OrganizationID, ProjectId: e.ProjectID, Name: e.Name, Description: e.Description,
		Version: e.Version, RuleVersion: e.RuleVersion, Paused: e.PausedAt != nil, PausedAt: e.PausedAt, PausedByUserId: e.PausedByUserID,
		CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
		Rules: gen.EnvironmentRules{
			AllowedRefs: refs, AllowPullRequests: r.AllowPullRequests, RequiredApprovals: r.RequiredApprovals,
			Reviewers:         reviewersOf(r.Reviewers, showsReviewerIDs(ctx, e.OrganizationID, e.ProjectID)),
			AllowSelfApproval: r.AllowSelfApproval, WaitMinutes: r.WaitMinutes, ApprovalTimeoutHours: r.ApprovalTimeoutHours,
			RequirePinnedImages: r.RequirePinnedImages, AllowedPlugins: r.AllowedPlugins,
			RequiredIsolation: gen.EnvironmentIsolation(r.RequiredIsolation), AllowAdminBypass: r.AllowAdminBypass,
			SupersedePending: r.SupersedePending,
		},
	}
	if e.PausedReason != "" {
		out.PausedReason = ptrOf(e.PausedReason)
	}
	return out
}

func (c *Controller) ListEnvironments(ctx context.Context, req gen.ListEnvironmentsRequestObject) (gen.ListEnvironmentsResponseObject, error) {
	page, limit := response.PageParams(req.Params.Page, req.Params.Limit)
	list, err := c.service.List(ctx, req.Id, req.ProjectId, page, limit)
	if err != nil {
		return nil, err
	}
	items := make([]gen.Environment, 0, len(list.Items))
	for _, e := range list.Items {
		items = append(items, toGen(ctx, e))
	}
	return gen.ListEnvironments200JSONResponse{
		Success: true, Message: ptrOf(messageListed), Data: &items, Meta: &gen.PageMeta{Page: page, Limit: limit, Total: int(list.Total)},
	}, nil
}

func (c *Controller) CreateEnvironment(ctx context.Context, req gen.CreateEnvironmentRequestObject) (gen.CreateEnvironmentResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	e, err := c.service.Create(ctx, req.Id, req.ProjectId, models.CreateInput{
		Name: req.Body.Name, Description: req.Body.Description, Rules: rulesInputOf(req.Body.Rules),
	}, factorOf(req.Body.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := toGen(ctx, e)
	return gen.CreateEnvironment201JSONResponse{Success: true, Message: ptrOf(messageCreated), Data: &data}, nil
}

func (c *Controller) GetEnvironment(ctx context.Context, req gen.GetEnvironmentRequestObject) (gen.GetEnvironmentResponseObject, error) {
	e, err := c.service.Get(ctx, req.Id, req.ProjectId, req.EnvironmentName)
	if err != nil {
		return nil, err
	}
	data := toGen(ctx, e)
	return gen.GetEnvironment200JSONResponse{Success: true, Message: ptrOf(messageLoaded), Data: &data}, nil
}

func (c *Controller) UpdateEnvironment(ctx context.Context, req gen.UpdateEnvironmentRequestObject) (gen.UpdateEnvironmentResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	e, err := c.service.Update(ctx, req.Id, req.ProjectId, req.EnvironmentName, models.UpdateInput{
		Version: req.Body.Version, Description: req.Body.Description, Rules: rulesInputOf(req.Body.Rules),
	}, factorOf(req.Body.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := toGen(ctx, e)
	return gen.UpdateEnvironment200JSONResponse{Success: true, Message: ptrOf(messageUpdated), Data: &data}, nil
}

func (c *Controller) RemoveEnvironment(ctx context.Context, req gen.RemoveEnvironmentRequestObject) (gen.RemoveEnvironmentResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	e, err := c.service.Remove(ctx, req.Id, req.ProjectId, req.EnvironmentName, factorOf(req.Body.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := toGen(ctx, e)
	return gen.RemoveEnvironment200JSONResponse{Success: true, Message: ptrOf(messageRemoved), Data: &data}, nil
}

func (c *Controller) PauseEnvironment(ctx context.Context, req gen.PauseEnvironmentRequestObject) (gen.PauseEnvironmentResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	e, err := c.service.Pause(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	data := toGen(ctx, e)
	return gen.PauseEnvironment200JSONResponse{Success: true, Message: ptrOf(messagePaused), Data: &data}, nil
}

func (c *Controller) ResumeEnvironment(ctx context.Context, req gen.ResumeEnvironmentRequestObject) (gen.ResumeEnvironmentResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	e, err := c.service.Resume(ctx, req.Id, req.ProjectId, req.EnvironmentName, factorOf(req.Body.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := toGen(ctx, e)
	return gen.ResumeEnvironment200JSONResponse{Success: true, Message: ptrOf(messageResumed), Data: &data}, nil
}
