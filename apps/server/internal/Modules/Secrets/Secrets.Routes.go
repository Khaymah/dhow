package secrets

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Gen"
)

const (
	orgPath      = "/organizations/{id}"
	projectPath  = orgPath + "/projects/{projectId}"
	envPath      = projectPath + "/environments/{environmentName}"
	orgParam     = "id"
	projectParam = "projectId"
)

type Deps struct {
	Service *Service
	Guard   *auth.Guard
	Log     *slog.Logger
}

func orgRules() []policy.Rule {
	return []policy.Rule{
		policy.Organization(http.MethodGet, orgPath+"/secrets", policy.SecretRead, orgParam).AllowTokens(),
		policy.Organization(http.MethodPut, orgPath+"/secrets/{secretName}", policy.OrgSecretManage, orgParam),
		policy.Organization(http.MethodPost, orgPath+"/secrets/{secretName}/remove", policy.OrgSecretManage, orgParam),
		policy.Organization(http.MethodPost, orgPath+"/secrets/{secretName}/restore-version", policy.OrgSecretManage, orgParam),
		policy.Organization(http.MethodGet, orgPath+"/variables", policy.ProjectRead, orgParam).AllowTokens(),
		policy.Organization(http.MethodPut, orgPath+"/variables/{variableName}", policy.OrgSecretManage, orgParam),
		policy.Organization(http.MethodPost, orgPath+"/variables/{variableName}/remove", policy.OrgSecretManage, orgParam),
		policy.Organization(http.MethodGet, orgPath+"/registry-credentials", policy.SecretRead, orgParam).AllowTokens(),
		policy.Organization(http.MethodPost, orgPath+"/registry-credentials", policy.OrgSecretManage, orgParam),
		policy.Organization(http.MethodPut, orgPath+"/registry-credentials/{credentialId}", policy.OrgSecretManage, orgParam),
		policy.Organization(http.MethodPost, orgPath+"/registry-credentials/{credentialId}/remove", policy.OrgSecretManage, orgParam),
	}
}

func projectRules() []policy.Rule {
	p := func(method, path string, action policy.Action) policy.Rule {
		return policy.Project(method, path, action, orgParam, projectParam)
	}
	return []policy.Rule{
		p(http.MethodGet, projectPath+"/secrets", policy.SecretRead).AllowTokens(),
		p(http.MethodPut, projectPath+"/secrets/{secretName}", policy.SecretManage),
		p(http.MethodPost, projectPath+"/secrets/{secretName}/remove", policy.SecretManage),
		p(http.MethodPost, projectPath+"/secrets/{secretName}/restore-version", policy.SecretManage),
		p(http.MethodGet, projectPath+"/variables", policy.ProjectRead).AllowTokens(),
		p(http.MethodPut, projectPath+"/variables/{variableName}", policy.SecretManage),
		p(http.MethodPost, projectPath+"/variables/{variableName}/remove", policy.SecretManage),
		p(http.MethodGet, projectPath+"/registry-credentials", policy.SecretRead).AllowTokens(),
		p(http.MethodPost, projectPath+"/registry-credentials", policy.SecretManage),
		p(http.MethodPut, projectPath+"/registry-credentials/{credentialId}", policy.SecretManage),
		p(http.MethodPost, projectPath+"/registry-credentials/{credentialId}/remove", policy.SecretManage),
		p(http.MethodGet, envPath+"/secrets", policy.SecretRead).AllowTokens(),
		p(http.MethodPut, envPath+"/secrets/{secretName}", policy.EnvironmentManage),
		p(http.MethodPost, envPath+"/secrets/{secretName}/remove", policy.EnvironmentManage),
		p(http.MethodPost, envPath+"/secrets/{secretName}/restore-version", policy.EnvironmentManage),
		p(http.MethodGet, envPath+"/variables", policy.ProjectRead).AllowTokens(),
		p(http.MethodPut, envPath+"/variables/{variableName}", policy.EnvironmentManage),
		p(http.MethodPost, envPath+"/variables/{variableName}/remove", policy.EnvironmentManage),
	}
}

func Policies() []policy.Rule {
	return append(orgRules(), projectRules()...)
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
