package ingest

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Ingest/Gen"
)

const (
	tokensPath    = "/organizations/{id}/projects/{projectId}/ingest-tokens"
	artifactsPath = "/organizations/{id}/projects/{projectId}/ingested-artifacts"
	UploadPattern = config.IngestPrefix + config.IngestArtifactsPath
)

type Deps struct {
	Service *Service
	Guard   *auth.Guard
	Log     *slog.Logger
}

func Policies() []policy.Rule {
	return []policy.Rule{
		policy.Project(http.MethodGet, tokensPath, policy.IngestManage, "id", "projectId"),
		policy.Project(http.MethodPost, tokensPath, policy.IngestManage, "id", "projectId"),
		policy.Project(http.MethodPost, tokensPath+"/{ingestTokenId}/revoke", policy.IngestManage, "id", "projectId"),
		policy.Project(http.MethodGet, artifactsPath, policy.ArtifactRead, "id", "projectId"),
	}
}

func IngestPolicies() []policy.Rule {
	return []policy.Rule{
		policy.Ingest(http.MethodPut, UploadPattern),
		policy.Ingest(http.MethodGet, UploadPattern),
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
		Middlewares:      []gen.MiddlewareFunc{middleware.BodyLimit(config.AuthBodyMaxBytes), guard},
		ErrorHandlerFunc: middleware.StrictRequestError,
	})
}

func UploadRoutes(r chi.Router, s *Service) {
	r.Put(UploadPattern, s.Put)
	r.Get(UploadPattern, s.Status)
}
