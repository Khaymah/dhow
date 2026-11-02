package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	jobqueue "github.com/Khaymah/dhow/apps/server/internal/Core/JobQueue"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	pipelines "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines"
	sources "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources"
	"github.com/Khaymah/dhow/internal/netguard"
)

func sourcesFor(in Infra, bus *eventbus.Bus, limiter ratelimit.Limiter, runs triggers.Sink) (*sources.Service, error) {
	sink := in.Triggers
	if sink == nil {
		sink = runs
	}
	return sources.NewService(sources.Options{
		DB: in.Pool, Bus: bus, Log: in.Log, Now: in.Now, SecretKey: in.Config.SecretKey, PublicURL: in.Config.PublicURL.String(),
		Triggers: sink, Limiter: limiter, Dialer: netguard.Dialer{Policy: netguard.Policy{Allowed: in.Config.Sources.AllowPrivate}, Lookup: in.GitLookup, Dial: in.GitDial},
	})
}

func webhookStack(o Options) []func(http.Handler) http.Handler {
	maxBytes := o.Config.Sources.WebhookMaxBytes
	if maxBytes <= 0 {
		maxBytes = config.WebhookMaxBytesDefault
	}
	return []func(http.Handler) http.Handler{
		middleware.RequestID,
		middleware.ClientIP(o.Config.TrustedProxies),
		middleware.Lang,
		middleware.AccessLog(o.Log),
		middleware.Recover(o.Log),
		middleware.NoStore,
		middleware.Timeout(config.WebhookTimeout),
		middleware.BodyLimit(maxBytes),
	}
}

func mountWebhooks(root chi.Router, o Options) {
	if o.Sources == nil {
		return
	}
	root.Group(func(r chi.Router) {
		r.Use(webhookStack(o)...)
		sources.WebhookRoutes(r, sources.WebhookDeps{
			Service: o.Sources, Limiter: o.Webhooks, Log: o.Log, MaxBytes: o.Config.Sources.WebhookMaxBytes,
			InflightBytes: o.Config.Sources.WebhookInflightBytes,
		})
	})
}

func pipelinesFor(in Infra, bus *eventbus.Bus, queue jobqueue.Queue) *pipelines.Service {
	images := in.Images
	if images == nil {
		images = pipelines.Registry{Dialer: netguard.Dialer{
			Policy: netguard.Policy{Allowed: in.Config.Sources.AllowPrivate}, Lookup: in.GitLookup, Dial: in.GitDial,
		}, TLS: in.RegistryTLS}
	}
	return pipelines.NewService(pipelines.Options{
		DB: in.Pool, Bus: bus, Log: in.Log, Now: in.Now, Queue: queue, Definitions: in.Definitions, Images: images,
		Checkout: in.Checkout, PublicURL: in.Config.PublicURL.String(),
	})
}

func StartPipelines(ctx context.Context, p *pipelines.Service, log *slog.Logger) (func(), error) {
	res, err := p.Reconcile(ctx)
	if err != nil {
		return nil, fmt.Errorf("reconcile pipeline runs before serving: %w", err)
	}
	log.Info("pipeline runs reconciled", "unqueued", res.Unqueued, "drifted", res.Drifted,
		"stalled", res.Stalled, "unsettled", res.Unsettled)
	bgCtx, cancel := context.WithCancel(ctx)
	var background sync.WaitGroup
	background.Go(func() { p.RunWorker(bgCtx) })
	return func() {
		cancel()
		background.Wait()
	}, nil
}

func StartSources(ctx context.Context, s *sources.Service, log *slog.Logger) (func(), error) {
	res, err := s.Maintain(ctx)
	if err != nil {
		return nil, fmt.Errorf("prepare webhook delivery partitions before serving: %w", err)
	}
	log.Info("webhook delivery partitions ready", "created", res.Created, "dropped", res.Dropped, "skipped", res.Skipped)
	bgCtx, cancel := context.WithCancel(ctx)
	var background sync.WaitGroup
	background.Go(func() { s.RunMaintenance(bgCtx) })
	background.Go(func() { s.RunStatusWorker(bgCtx) })
	return func() {
		cancel()
		background.Wait()
	}, nil
}
