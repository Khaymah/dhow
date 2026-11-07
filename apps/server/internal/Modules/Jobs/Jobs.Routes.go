package jobs

import (
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	runnerauth "github.com/Khaymah/dhow/apps/server/internal/Core/RunnerAuth"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs/Gen"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

const (
	LogStreamPath = "/organizations/{id}/jobs/{jobId}/logs/stream"
	LogRawPath    = "/organizations/{id}/jobs/{jobId}/logs/raw"
)

type Deps struct {
	Service *Service
	Guard   *auth.Guard
	Gate    *runnerauth.Gate
	Log     *slog.Logger
}

func Policies() []policy.Rule {
	return []policy.Rule{
		policy.Organization(http.MethodGet, "/organizations/{id}/jobs", policy.JobRead, "id").AllowTokens(),
		policy.Organization(http.MethodPost, "/organizations/{id}/jobs", policy.JobCreate, "id"),
		policy.Organization(http.MethodGet, "/organizations/{id}/jobs/{jobId}", policy.JobRead, "id").AllowTokens(),
		policy.Organization(http.MethodPost, "/organizations/{id}/jobs/{jobId}/cancel", policy.JobCancel, "id").AllowTokens(),
		policy.Organization(http.MethodGet, "/organizations/{id}/jobs/{jobId}/logs", policy.LogRead, "id"),
		policy.Organization(http.MethodGet, LogStreamPath, policy.LogRead, "id"),
		policy.Organization(http.MethodGet, LogRawPath, policy.LogRead, "id"),
	}
}

func Procedures() []policy.Rule {
	return []policy.Rule{
		policy.Runner(runnerv1connect.LeaseServiceLeaseProcedure, policy.RunnerSession),
		policy.Runner(runnerv1connect.LeaseServiceStartProcedure, policy.RunnerSession),
		policy.Runner(runnerv1connect.LeaseServiceHeartbeatProcedure, policy.RunnerSession),
		policy.Runner(runnerv1connect.LeaseServiceBeginStepProcedure, policy.RunnerSession),
		policy.Runner(runnerv1connect.LeaseServiceEndStepProcedure, policy.RunnerSession),
		policy.Runner(runnerv1connect.LeaseServiceCompleteProcedure, policy.RunnerSession),
		policy.Runner(runnerv1connect.LogServiceAppendProcedure, policy.RunnerSession),
	}
}

func Subscribe(bus *eventbus.Bus, s *Service) {
	bus.Subscribe(eventbus.RunnerRevoked, s.HandleRunnerRevoked)
}

func Routes(r chi.Router, d Deps) {
	strict := gen.NewStrictHandlerWithOptions(NewController(d.Service, d.Guard), nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  middleware.StrictRequestError,
		ResponseErrorHandlerFunc: middleware.StrictResponseError(d.Log),
	})
	guard := d.Guard.Middleware(policy.NewTable(Policies()...))
	gen.HandlerWithOptions(strict, gen.ChiServerOptions{
		BaseRouter:       r,
		Middlewares:      []gen.MiddlewareFunc{middleware.BodyLimit(config.JobBodyMaxBytes), guard, RouteTimeout},
		ErrorHandlerFunc: middleware.StrictRequestError,
	})
}

func RouteTimeout(next http.Handler) http.Handler {
	timed := middleware.Timeout(config.APITimeout)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rc := chi.RouteContext(r.Context()); rc != nil {
			switch rc.RoutePattern() {
			case config.APIPrefix + LogStreamPath, config.APIPrefix + LogRawPath:
				next.ServeHTTP(w, r)
				return
			}
		}
		timed.ServeHTTP(w, r)
	})
}

func ConnectRoutes(mux *http.ServeMux, d Deps) {
	options := []connect.HandlerOption{
		d.Gate.RequestGate(policy.NewTable(Procedures()...)),
		connect.WithReadMaxBytes(config.RunnerReadMaxBytes),
		connect.WithInterceptors(runnerauth.Interceptor(d.Log)),
	}
	mux.Handle(runnerv1connect.NewLeaseServiceHandler(connectHandler{service: d.Service}, options...))
	mux.Handle(runnerv1connect.NewLogServiceHandler(logHandler{service: d.Service},
		d.Gate.RequestGate(policy.NewTable(Procedures()...)),
		connect.WithReadMaxBytes(config.LogAppendMaxBytes),
		connect.WithInterceptors(runnerauth.Interceptor(d.Log)),
	))
}
