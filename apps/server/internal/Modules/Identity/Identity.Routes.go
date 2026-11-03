package identity

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Identity/Gen"
)

type Deps struct {
	Service *Service
	Guard   *auth.Guard
	Limits  *ratelimit.PerClient
	Log     *slog.Logger
}

const (
	limitInvitation = "invitation"
	limitSetup      = "setup"
)

func RateLimited() []ratelimit.Route {
	return []ratelimit.Route{
		{Method: http.MethodPost, Path: "/setup", Group: limitSetup},
		{Method: http.MethodPost, Path: "/invitations/preview", Group: limitInvitation},
		{Method: http.MethodPost, Path: "/invitations/accept", Group: limitInvitation},
	}
}

const (
	orgPath        = "/organizations/{id}"
	memberPath     = orgPath + "/members/{userId}"
	teamsPath      = orgPath + "/teams"
	teamPath       = teamsPath + "/{teamId}"
	bindingPath    = teamPath + "/role-bindings/{bindingId}"
	invitationPath = orgPath + "/invitations"
)

func Policies() []policy.Rule {
	org := func(method, path string, action policy.Action) policy.Rule {
		return policy.Organization(method, path, action, "id").AllowTokens()
	}
	browser := func(method, path string, action policy.Action) policy.Rule {
		return policy.Organization(method, path, action, "id")
	}
	return []policy.Rule{
		policy.Public(http.MethodPost, "/setup"),
		policy.Instance(http.MethodPost, "/organizations", policy.OrgCreate),
		org(http.MethodGet, orgPath, policy.OrgRead),
		org(http.MethodPatch, orgPath, policy.OrgUpdate),
		browser(http.MethodPost, orgPath+"/leave", policy.OrgLeave),
		browser(http.MethodPost, orgPath+"/transfer-ownership", policy.OrgTransfer),
		browser(http.MethodPut, orgPath+"/second-factor-rule", policy.OrgSecurity),
		browser(http.MethodPut, orgPath+"/log-retention", policy.OrgSecurity),
		org(http.MethodGet, orgPath+"/members", policy.MemberRead),
		org(http.MethodPatch, memberPath, policy.MemberManage),
		org(http.MethodDelete, memberPath, policy.MemberManage),
		browser(http.MethodPost, memberPath+"/mfa-reset", policy.MemberManage),
		org(http.MethodGet, teamsPath, policy.MemberRead),
		org(http.MethodPost, teamsPath, policy.MemberManage),
		org(http.MethodGet, teamPath, policy.MemberRead),
		org(http.MethodPatch, teamPath, policy.MemberManage),
		org(http.MethodDelete, teamPath, policy.MemberManage),
		org(http.MethodPost, teamPath+"/members", policy.MemberManage),
		org(http.MethodDelete, teamPath+"/members/{userId}", policy.MemberManage),
		org(http.MethodPost, teamPath+"/role-bindings", policy.MemberManage),
		org(http.MethodPatch, bindingPath, policy.MemberManage),
		org(http.MethodDelete, bindingPath, policy.MemberManage),
		org(http.MethodGet, invitationPath, policy.MemberManage),
		org(http.MethodPost, invitationPath, policy.MemberManage),
		org(http.MethodDelete, invitationPath+"/{invitationId}", policy.MemberManage),
		org(http.MethodPost, invitationPath+"/{invitationId}/resend", policy.MemberManage),
		policy.Public(http.MethodPost, "/invitations/preview"),
		policy.Public(http.MethodPost, "/invitations/accept"),
		policy.Authenticated(http.MethodPost, "/me/invitations/accept", policy.SessionManageSelf),
	}
}

func Routes(r chi.Router, d Deps) {
	strict := gen.NewStrictHandlerWithOptions(NewController(d.Service), nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  middleware.StrictRequestError,
		ResponseErrorHandlerFunc: middleware.StrictResponseError(d.Log),
	})
	guard := d.Guard.Middleware(policy.NewTable(Policies()...))
	gen.HandlerWithOptions(strict, gen.ChiServerOptions{
		BaseRouter:       r,
		Middlewares:      []gen.MiddlewareFunc{middleware.BodyLimit(config.AuthBodyMaxBytes), d.Limits.Middleware(RateLimited()...), guard},
		ErrorHandlerFunc: middleware.StrictRequestError,
	})
}
