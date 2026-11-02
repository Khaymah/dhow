package identity

import (
	"context"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Identity/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Identity/Models"
)

const (
	messageSetup   = "identity.setup.completed"
	messageOrg     = "identity.organization.loaded"
	messageOrgSave = "identity.organization.updated"
	messageMembers = "identity.members.listed"
	messageRule    = "identity.secondFactor.saved"
)

type Controller struct {
	service *Service
}

var _ gen.StrictServerInterface = (*Controller)(nil)

func NewController(service *Service) *Controller { return &Controller{service: service} }

func (c *Controller) SetupInstance(ctx context.Context, req gen.SetupInstanceRequestObject) (gen.SetupInstanceResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	b := req.Body
	locale := string(i18n.LocaleFrom(ctx))
	if b.Locale != nil {
		locale = string(*b.Locale)
	}
	res, err := c.service.Setup(ctx, models.SetupInput{
		Token: b.SetupToken, OwnerName: b.OwnerName, Email: b.Email, Password: b.Password,
		OrganizationName: b.OrganizationName, OrganizationSlug: b.OrganizationSlug, Locale: locale,
	})
	if err != nil {
		return nil, err
	}
	data := gen.SetupResult{
		User:         gen.User{Id: res.User.ID, Email: res.User.Email, Name: res.User.Name, Locale: gen.Locale(res.User.Locale)},
		Organization: orgToGen(res.Organization),
	}
	return gen.SetupInstance200JSONResponse{Success: true, Message: ptr(messageSetup), Data: &data}, nil
}

func (c *Controller) GetOrganization(ctx context.Context, req gen.GetOrganizationRequestObject) (gen.GetOrganizationResponseObject, error) {
	org, err := c.service.GetOrganization(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	data := orgToGen(org)
	return gen.GetOrganization200JSONResponse{Success: true, Message: ptr(messageOrg), Data: &data}, nil
}

func (c *Controller) UpdateOrganization(ctx context.Context, req gen.UpdateOrganizationRequestObject) (gen.UpdateOrganizationResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	org, err := c.service.UpdateOrganization(ctx, req.Id, req.Body.Name)
	if err != nil {
		return nil, err
	}
	data := orgToGen(org)
	return gen.UpdateOrganization200JSONResponse{Success: true, Message: ptr(messageOrgSave), Data: &data}, nil
}

func (c *Controller) UpdateLogRetention(ctx context.Context, req gen.UpdateLogRetentionRequestObject) (gen.UpdateLogRetentionResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	org, err := c.service.UpdateLogRetention(ctx, req.Id, req.Body.LogRetentionDays)
	if err != nil {
		return nil, err
	}
	data := orgToGen(org)
	return gen.UpdateLogRetention200JSONResponse{Success: true, Message: ptr(messageOrgSave), Data: &data}, nil
}

func (c *Controller) UpdateSecondFactorRule(ctx context.Context, req gen.UpdateSecondFactorRuleRequestObject) (gen.UpdateSecondFactorRuleResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	org, err := c.service.UpdateSecondFactorRule(ctx, req.Id, string(req.Body.RequireSecondFactor), factorOf(req.Body.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := orgToGen(org)
	return gen.UpdateSecondFactorRule200JSONResponse{Success: true, Message: ptr(messageRule), Data: &data}, nil
}

func (c *Controller) ListMembers(ctx context.Context, req gen.ListMembersRequestObject) (gen.ListMembersResponseObject, error) {
	page, limit := response.PageParams(req.Params.Page, req.Params.Limit)
	members, total, err := c.service.ListMembers(ctx, req.Id, page, limit)
	if err != nil {
		return nil, err
	}
	items := make([]gen.Member, 0, len(members))
	for _, m := range members {
		teams := make([]gen.MemberTeam, 0, len(m.Teams))
		for _, team := range m.Teams {
			teams = append(teams, gen.MemberTeam{Id: team.ID, Name: team.Name})
		}
		item := gen.Member{
			UserId: m.UserID, Name: m.Name, Email: m.Email, Role: gen.Role(m.Role), EffectiveRole: gen.Role(m.EffectiveRole), JoinedAt: m.JoinedAt,
			LastActiveAt: m.LastActiveAt, Teams: teams,
		}
		if m.Mfa != "" {
			item.Mfa = ptr(gen.MfaState(m.Mfa))
			item.MfaResetPending = ptr(m.MfaResetPending)
		}
		items = append(items, item)
	}
	return gen.ListMembers200JSONResponse{
		Success: true, Message: ptr(messageMembers), Data: &items,
		Meta: &gen.PageMeta{Page: page, Limit: limit, Total: int(total)},
	}, nil
}

func orgToGen(o models.Organization) gen.Organization {
	out := gen.Organization{
		Id: o.ID, Name: o.Name, Slug: o.Slug, CreatedAt: o.CreatedAt,
		RequireSecondFactor: gen.SecondFactorRule(o.RequireSecondFactor), RequireSecondFactorSince: o.RequireSecondFactorSince,
	}
	if o.LogRetentionDays != nil {
		out.LogRetentionDays = ptr(int(*o.LogRetentionDays))
	}
	return out
}

func ptr[T any](v T) *T { return &v }
