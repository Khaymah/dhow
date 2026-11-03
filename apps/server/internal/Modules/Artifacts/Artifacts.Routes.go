package artifacts

import (
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	runnerauth "github.com/Khaymah/dhow/apps/server/internal/Core/RunnerAuth"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Artifacts/Gen"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

const (
	runArtifactsPath = "/organizations/{id}/projects/{projectId}/runs/{runId}/artifacts"
	downloadPath     = "/organizations/{id}/projects/{projectId}/artifacts/{artifactId}/download"
	BlobPattern      = policy.RunnerBlobPrefix + "{" + digestParam + "}"
)

type Deps struct {
	Service *Service
	Guard   *auth.Guard
	Gate    *runnerauth.Gate
	Log     *slog.Logger
}

func Policies() []policy.Rule {
	return []policy.Rule{
		policy.Project(http.MethodGet, runArtifactsPath, policy.ArtifactRead, "id", "projectId"),
		policy.Project(http.MethodGet, downloadPath, policy.ArtifactRead, "id", "projectId"),
	}
}

func Procedures() []policy.Rule {
	return []policy.Rule{
		policy.Runner(runnerv1connect.ArtifactServiceCommitProcedure, policy.RunnerSession),
		policy.Runner(runnerv1connect.CacheServiceLookupProcedure, policy.RunnerSession),
		policy.Runner(runnerv1connect.CacheServiceCommitProcedure, policy.RunnerSession),
		policy.RunnerHTTP(http.MethodPut, BlobPattern, policy.RunnerSession),
		policy.RunnerHTTP(http.MethodGet, BlobPattern, policy.RunnerSession),
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
		Middlewares:      []gen.MiddlewareFunc{guard, RouteTimeout},
		ErrorHandlerFunc: middleware.StrictRequestError,
	})
}

func RouteTimeout(next http.Handler) http.Handler {
	timed := middleware.Timeout(config.APITimeout)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() == config.APIPrefix+downloadPath {
			next.ServeHTTP(w, r)
			return
		}
		timed.ServeHTTP(w, r)
	})
}

func ConnectRoutes(mux *http.ServeMux, d Deps) {
	table := policy.NewTable(Procedures()...)
	options := []connect.HandlerOption{
		d.Gate.RequestGate(table),
		connect.WithReadMaxBytes(config.RunnerReadMaxBytes),
		connect.WithInterceptors(runnerauth.Interceptor(d.Log)),
	}
	mux.Handle(runnerv1connect.NewArtifactServiceHandler(artifactHandler{service: d.Service}, options...))
	mux.Handle(runnerv1connect.NewCacheServiceHandler(cacheHandler{service: d.Service}, options...))
	mux.Handle(http.MethodPut+" "+BlobPattern, d.Gate.HTTP(table, http.MethodPut, BlobPattern)(http.HandlerFunc(d.Service.PutBlob)))
	mux.Handle(http.MethodGet+" "+BlobPattern, d.Gate.HTTP(table, http.MethodGet, BlobPattern)(http.HandlerFunc(d.Service.GetBlob)))
}
