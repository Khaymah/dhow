package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	"github.com/Khaymah/dhow/internal/buildinfo"
)

func Run(ctx context.Context) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}
	log := config.NewLogger(cfg)
	build := buildinfo.Get()
	log.Info("starting dhow-server", "version", build.Version, "commit", build.Commit, "go", build.GoVersion)
	if cfg.Dev {
		log.Warn("DHOW_DEV=true: plain HTTP and unencrypted database connections are allowed. Never use this in production.")
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	provider, err := OpenKeyring(cfg.Keyring)
	if err != nil {
		return holdKeyring(ctx, err, cfg, log)
	}

	pool, err := config.OpenDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	migrator, err := migrations.NewMigrator(pool, schemaBinary(cfg, build))
	if err != nil {
		return err
	}
	defer func() {
		if err := migrator.Close(); err != nil {
			log.Warn("close migrator", "error", err)
		}
	}()
	if err := keys.NewManager(pool, provider, log).Preflight(ctx); err != nil {
		return holdKeyring(ctx, err, cfg, log)
	}
	if err := PrepareSchema(ctx, migrator, log); err != nil {
		return HoldIncompatible(ctx, err, cfg.OnIncompatible, log)
	}
	keyManager, stopInstance, err := StartInstance(ctx, pool, provider, ServerIdentity(build, migrator.Latest(), cfg.InstanceID, os.Hostname, log), log)
	if err != nil {
		return HoldIncompatible(ctx, err, cfg.OnIncompatible, log)
	}
	defer stopInstance()

	wired, err := Build(ctx, Infra{Config: cfg, Log: log, Pool: pool, Migrations: migrator, Build: build, Keys: keyManager})
	if err != nil {
		return err
	}
	defer wired.Close()
	stopSettings := StartSettings(ctx, wired.Settings)
	defer stopSettings()
	setupURL, created, err := wired.Identity.EnsureSetupToken(ctx)
	if err != nil {
		return fmt.Errorf("prepare first-run setup: %w", err)
	}
	if created {
		log.Info("first-run setup is required: open this link within one hour to create the owner account", "setup_url", setupURL)
	}
	stopJobs, err := StartJobs(ctx, wired.Jobs, log)
	if err != nil {
		return err
	}
	defer stopJobs()
	stopSources, err := StartSources(ctx, wired.Sources, log)
	if err != nil {
		return err
	}
	defer stopSources()
	stopPipelines, err := StartPipelines(ctx, wired.Pipelines, log)
	if err != nil {
		return err
	}
	defer stopPipelines()
	stopArtifacts := StartArtifacts(ctx, wired.Artifacts, log)
	defer stopArtifacts()
	stopMaintenance := StartMaintenance(ctx, wired.Retention, config.RetentionEvery)
	defer stopMaintenance()
	stopNotifications := StartNotifications(ctx, wired.Notifications)
	defer stopNotifications()
	servers := []served{{srv: NewHTTPServer(cfg, wired.Handler, log), attrs: []any{"public_url", cfg.PublicURL.String()}}}
	if wired.RunnerHandler != nil {
		runner := NewHTTPServer(cfg, wired.RunnerHandler, log)
		runner.Addr = cfg.RunnerListenAddr
		servers = append(servers, served{srv: runner, cert: cfg.RunnerTLSCertFile, key: cfg.RunnerTLSKeyFile, attrs: []any{"plane", "runner", "tls", cfg.RunnerPlaneTLS()}})
		if !cfg.RunnerPlaneTLS() {
			log.Warn("the runner listener serves plain HTTP because DHOW_DEV=true. Never use this in production.")
		}
	}
	for _, s := range servers {
		s.srv.RegisterOnShutdown(wired.Jobs.Drain)
	}
	return serveAll(ctx, servers, log)
}

type served struct {
	srv       *http.Server
	cert, key string
	attrs     []any
}

func (s served) listen() error {
	if s.cert != "" {
		return s.srv.ListenAndServeTLS(s.cert, s.key)
	}
	return s.srv.ListenAndServe()
}

func serveAll(ctx context.Context, servers []served, log *slog.Logger) error {
	serveErr := make(chan error, len(servers))
	for _, s := range servers {
		go func() {
			log.Info("listening", append([]any{"addr", s.srv.Addr}, s.attrs...)...)
			serveErr <- s.listen()
		}()
	}
	var failure error
	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			failure = fmt.Errorf("serve: %w", err)
		}
	case <-ctx.Done():
	}
	log.Info("shutting down", "timeout", config.ShutdownTimeout.String())
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), config.ShutdownTimeout)
	defer cancel()
	var errs []error
	for _, s := range servers {
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("graceful shutdown of %s: %w", s.srv.Addr, err))
		}
	}
	if failure != nil || len(errs) > 0 {
		return errors.Join(append([]error{failure}, errs...)...)
	}
	log.Info("stopped")
	return nil
}

func NewHTTPServer(cfg config.Config, handler http.Handler, log *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: config.ReadHeaderTimeout,
		ReadTimeout:       config.ReadTimeout,
		WriteTimeout:      config.WriteTimeout,
		IdleTimeout:       config.IdleTimeout,
		MaxHeaderBytes:    config.MaxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
}

func holdKeyring(ctx context.Context, err error, cfg config.Config, log *slog.Logger) error {
	if cfg.OnIncompatible != config.OnIncompatibleWait {
		log.Error("the keyring does not fit; stopping", "code", keys.CodeOf(err), "error", err.Error())
	}
	return HoldIncompatible(ctx, err, cfg.OnIncompatible, log)
}
