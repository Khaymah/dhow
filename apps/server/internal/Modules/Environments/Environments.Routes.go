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
	targetsPath      = environmentPath + "/targets"
	targetPath       = targetsPath + "/{targetName}"
	hostPath         = targetPath + "/hosts/{hostId}"
	approvalsPath    = "/organizations/{id}/projects/{projectId}/runs/{runId}/approvals"
	approvalPath     = approvalsPath + "/{approvalId}"
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
		policy.Project(http.MethodGet, targetsPath, policy.ProjectRead, orgParam, projectParam).AllowTokens(),
		policy.Project(http.MethodPost, targetsPath, policy.EnvironmentManage, orgParam, projectParam),
		policy.Project(http.MethodGet, targetPath, policy.ProjectRead, orgParam, projectParam).AllowTokens(),
		policy.Project(http.MethodPut, targetPath, policy.EnvironmentManage, orgParam, projectParam),
		policy.Project(http.MethodPost, targetPath+"/remove", policy.EnvironmentManage, orgParam, projectParam),
		policy.Project(http.MethodPost, targetPath+"/generate-key", policy.EnvironmentManage, orgParam, projectParam),
		policy.Project(http.MethodGet, targetPath+"/check", policy.ProjectRead, orgParam, projectParam).AllowTokens(),
		policy.Project(http.MethodPost, targetPath+"/check", policy.DeploymentCheck, orgParam, projectParam),
		policy.Project(http.MethodPost, targetPath+"/rollback", policy.DeploymentRollback, orgParam, projectParam),
		policy.Project(http.MethodGet, targetPath+"/releases", policy.ProjectRead, orgParam, projectParam).AllowTokens(),
		policy.Project(http.MethodPost, targetPath+"/hosts", policy.EnvironmentManage, orgParam, projectParam),
		policy.Project(http.MethodPut, targetPath+"/hosts-order", policy.EnvironmentManage, orgParam, projectParam),
		policy.Project(http.MethodPut, hostPath, policy.EnvironmentManage, orgParam, projectParam),
		policy.Project(http.MethodPost, hostPath+"/remove", policy.EnvironmentManage, orgParam, projectParam),
		policy.Project(http.MethodPost, hostPath+"/fetch-host-key", policy.EnvironmentManage, orgParam, projectParam),
		policy.Project(http.MethodPut, hostPath+"/pending-host-key", policy.EnvironmentManage, orgParam, projectParam),
		policy.Project(http.MethodPost, hostPath+"/confirm-host-key", policy.EnvironmentManage, orgParam, projectParam),
		policy.Project(http.MethodGet, approvalsPath, policy.RunRead, orgParam, projectParam).AllowTokens(),
		policy.Project(http.MethodPost, approvalPath+"/approve", policy.DeploymentApprove, orgParam, projectParam),
		policy.Project(http.MethodPost, approvalPath+"/reject", policy.DeploymentApprove, orgParam, projectParam),
		policy.Project(http.MethodPost, approvalPath+"/bypass", policy.EnvironmentBypass, orgParam, projectParam),
		policy.Organization(http.MethodGet, "/organizations/{id}/deployment-approvals", policy.RunRead, orgParam).AllowTokens(),
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
