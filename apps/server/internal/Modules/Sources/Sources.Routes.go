package sources

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources/Models"
)

const (
	repositoryPath = "/organizations/{id}/projects/{projectId}/repository"
	GitHookPath    = config.WebhookPrefix + "/git/{hookId}"
)

type Deps struct {
	Service *Service
	Guard   *auth.Guard
	Log     *slog.Logger
}

func Policies() []policy.Rule {
	return []policy.Rule{
		policy.Project(http.MethodGet, repositoryPath, policy.ProjectRead, "id", "projectId").AllowTokens(),
		policy.Project(http.MethodPut, repositoryPath, policy.ProjectManage, "id", "projectId").AllowTokens(),
		policy.Project(http.MethodDelete, repositoryPath, policy.ProjectManage, "id", "projectId").AllowTokens(),
		policy.Project(http.MethodPost, repositoryPath+"/confirm-host-key", policy.ProjectManage, "id", "projectId").AllowTokens(),
		policy.Project(http.MethodPost, repositoryPath+"/rotate-deploy-key", policy.ProjectManage, "id", "projectId").AllowTokens(),
		policy.Project(http.MethodPost, repositoryPath+"/confirm-deploy-key", policy.ProjectManage, "id", "projectId").AllowTokens(),
		policy.Project(http.MethodPost, repositoryPath+"/rotate-webhook-secret", policy.ProjectManage, "id", "projectId").AllowTokens(),
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

type WebhookDeps struct {
	Service       *Service
	Limiter       ratelimit.Limiter
	Log           *slog.Logger
	MaxBytes      int64
	InflightBytes int64
}

type webhookRoute struct {
	Rule    policy.Rule
	Handler http.HandlerFunc
}

func WebhookPolicies() []policy.Rule {
	return []policy.Rule{policy.Webhook(http.MethodPost, GitHookPath)}
}

func webhookRoutes(h *webhookHandler) []webhookRoute {
	handlers := map[string]http.HandlerFunc{GitHookPath: h.provider(models.ProviderGeneric)}
	out := make([]webhookRoute, 0, len(handlers))
	for _, rule := range WebhookPolicies() {
		if handler, ok := handlers[rule.Path]; ok {
			out = append(out, webhookRoute{Rule: rule, Handler: handler})
		}
	}
	return out
}

func webhookGate(table *policy.Table, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := table.Lookup(r.Method, chi.RouteContext(r.Context()).RoutePattern()); !ok {
				log.ErrorContext(r.Context(), "webhook route has no valid policy declaration", "method", r.Method)
				response.Error(w, r, apperrors.NotFound())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func WebhookRoutes(r chi.Router, d WebhookDeps) {
	table := policy.NewTable(WebhookPolicies()...)
	maxBytes := d.MaxBytes
	if maxBytes <= 0 {
		maxBytes = config.WebhookMaxBytesDefault
	}
	inflight := d.InflightBytes
	if inflight <= 0 {
		inflight = config.WebhookInflightDefault
	}
	h := &webhookHandler{service: d.Service, limiter: d.Limiter, log: d.Log, maxBytes: maxBytes, budget: NewByteBudget(inflight)}
	gate := webhookGate(table, d.Log)
	for _, route := range webhookRoutes(h) {
		r.With(gate).Method(route.Rule.Method, route.Rule.Path, route.Handler)
	}
}
