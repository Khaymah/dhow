package pipelines

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Gen"
)

const (
	runsPath   = "/organizations/{id}/projects/{projectId}/runs"
	runPath    = runsPath + "/{runId}"
	inputsPath = "/organizations/{id}/projects/{projectId}/manual-inputs"
)

type Deps struct {
	Service *Service
	Guard   *auth.Guard
	Log     *slog.Logger
}

func Policies() []policy.Rule {
	return []policy.Rule{
		policy.Project(http.MethodGet, runsPath, policy.RunRead, "id", "projectId").AllowTokens(),
		policy.Project(http.MethodPost, runsPath, policy.RunTrigger, "id", "projectId").AllowTokens(),
		policy.Project(http.MethodGet, runPath, policy.RunRead, "id", "projectId").AllowTokens(),
		policy.Project(http.MethodPost, runPath+"/cancel", policy.RunCancel, "id", "projectId").AllowTokens(),
		policy.Project(http.MethodPost, runPath+"/rerun-failed", policy.RunTrigger, "id", "projectId").AllowTokens(),
		policy.Project(http.MethodPost, runPath+"/approve", policy.RunApprove, "id", "projectId"),
		policy.Project(http.MethodGet, inputsPath, policy.RunTrigger, "id", "projectId").AllowTokens(),
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
