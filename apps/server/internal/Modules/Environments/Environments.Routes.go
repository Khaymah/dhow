package environments

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Gen"
)

const (
	environmentsPath = "/organizations/{id}/projects/{projectId}/environments"
	environmentPath  = environmentsPath + "/{environmentName}"
	orgParam         = "id"
	projectParam     = "projectId"
)

type Deps struct {
	Service *Service
	Guard   *auth.Guard
	Log     *slog.Logger
}

func Policies() []policy.Rule {
	return []policy.Rule{
		policy.Project(http.MethodGet, environmentsPath, policy.ProjectRead, orgParam, projectParam).AllowTokens(),
		policy.Project(http.MethodPost, environmentsPath, policy.EnvironmentManage, orgParam, projectParam),
		policy.Project(http.MethodGet, environmentPath, policy.ProjectRead, orgParam, projectParam).AllowTokens(),
		policy.Project(http.MethodPut, environmentPath, policy.EnvironmentManage, orgParam, projectParam),
		policy.Project(http.MethodPost, environmentPath+"/remove", policy.EnvironmentManage, orgParam, projectParam),
		policy.Project(http.MethodPost, environmentPath+"/pause", policy.EnvironmentPause, orgParam, projectParam),
		policy.Project(http.MethodPost, environmentPath+"/resume", policy.EnvironmentManage, orgParam, projectParam),
		policy.Project(http.MethodGet, environmentPath+"/deployments", policy.RunRead, orgParam, projectParam).AllowTokens(),
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
		Middlewares:      []gen.MiddlewareFunc{guard},
		ErrorHandlerFunc: middleware.StrictRequestError,
	})
}
