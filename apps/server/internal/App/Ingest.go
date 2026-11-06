package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	ingest "github.com/Khaymah/dhow/apps/server/internal/Modules/Ingest"
)

const ingestRoot = "/ingest"

func IngestPolicies() []policy.Rule { return ingest.IngestPolicies() }

func ingestFor(in Infra, runs runServices, bus *eventbus.Bus, limiter ratelimit.Limiter, factors auth.FactorConfirmer) (*ingest.Service, error) {
	var sink triggers.Sink = runs.pipelines
	if in.Triggers != nil {
		sink = in.Triggers
	}
	return ingest.NewService(ingest.Options{
		DB: in.Pool, Store: runs.artifacts.Store(), Triggers: sink, Bus: bus, Factors: factors, Limiter: limiter,
		Config: in.Config.Artifacts, PublicURL: in.Config.PublicURL.String(), Log: in.Log, Now: in.Now,
	})
}

func ingestStack(o Options) []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		middleware.RequestID,
		middleware.ClientIP(o.Config.TrustedProxies),
		middleware.Lang,
		middleware.AccessLog(o.Log),
		middleware.Recover(o.Log),
		middleware.NoStore,
	}
}

func mountIngest(root chi.Router, o Options) {
	if o.Ingest == nil {
		return
	}
	root.Group(func(r chi.Router) {
		r.Use(ingestStack(o)...)
		ingest.UploadRoutes(r, o.Ingest)
	})
}
