package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	appcrypto "github.com/Khaymah/dhow/apps/server/internal/Core/Crypto"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	jobqueue "github.com/Khaymah/dhow/apps/server/internal/Core/JobQueue"
	mailer "github.com/Khaymah/dhow/apps/server/internal/Core/Mail"
	oidcauth "github.com/Khaymah/dhow/apps/server/internal/Core/Oidc"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	runnerauth "github.com/Khaymah/dhow/apps/server/internal/Core/RunnerAuth"
	instancesettings "github.com/Khaymah/dhow/apps/server/internal/Core/Settings"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	retention "github.com/Khaymah/dhow/apps/server/internal/Database/Retention"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	alerts "github.com/Khaymah/dhow/apps/server/internal/Modules/Alerts"
	artifacts "github.com/Khaymah/dhow/apps/server/internal/Modules/Artifacts"
	audit "github.com/Khaymah/dhow/apps/server/internal/Modules/Audit"
	identity "github.com/Khaymah/dhow/apps/server/internal/Modules/Identity"
	jobs "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs"
	overview "github.com/Khaymah/dhow/apps/server/internal/Modules/Overview"
	pipelines "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines"
	preferences "github.com/Khaymah/dhow/apps/server/internal/Modules/Preferences"
	projects "github.com/Khaymah/dhow/apps/server/internal/Modules/Projects"
	runners "github.com/Khaymah/dhow/apps/server/internal/Modules/Runners"
	session "github.com/Khaymah/dhow/apps/server/internal/Modules/Session"
	settings "github.com/Khaymah/dhow/apps/server/internal/Modules/Settings"
	sources "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
	systemmodels "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Models"
	tokens "github.com/Khaymah/dhow/apps/server/internal/Modules/Tokens"
	"github.com/Khaymah/dhow/internal/buildinfo"
	"github.com/Khaymah/dhow/internal/netguard"
)

type Infra struct {
	Config     config.Config
	Log        *slog.Logger
	Pool       *pgxpool.Pool
	Migrations system.MigrationStatus
	Build      buildinfo.Info
	Breach     auth.BreachChecker
	Secrets    jobs.SecretResolver
	HashParams *auth.Params
	Now        func() time.Time
	Web        http.Handler
	Mailer     mailer.Sender
	MailTLS    *tls.Config

	LogStreamWriteWait time.Duration
	LogReadWait        time.Duration
	OidcTransport      http.RoundTripper
	Triggers           triggers.Sink
	GitLookup          netguard.Lookup
	GitDial            netguard.DialFunc
	Definitions        pipelines.Definitions
	Images             pipelines.ImageResolver
	CheckoutImage      string
	RegistryTLS        *tls.Config
	ManualJobs         bool
	PinBudget          time.Duration
}

type Wired struct {
	Options       Options
	Handler       http.Handler
	RunnerHandler http.Handler
	Identity      *identity.Service
	Jobs          *jobs.Service
	JobQueue      jobqueue.Queue
	Projects      *projects.Service
	Sources       *sources.Service
	Pipelines     *pipelines.Service
	Artifacts     *artifacts.Service
	LogPool       *pgxpool.Pool
	Retention     *retention.Runner
	Alerts        *alerts.Service
	Settings      *instancesettings.Store
}

func (w *Wired) Close() {
	if w.Alerts != nil {
		w.Alerts.Close()
	}
	if w.LogPool != nil {
		w.LogPool.Close()
	}
}

func Build(ctx context.Context, in Infra) (*Wired, error) {
	params := auth.DefaultParams()
	if in.HashParams != nil {
		params = *in.HashParams
	}
	hasher, err := auth.NewHasher(ctx, params, runtime.NumCPU())
	if err != nil {
		return nil, err
	}
	store, err := settingsStore(ctx, in)
	if err != nil {
		return nil, err
	}
	passwords := passwordsFor(in, store)
	csrf := auth.NewCSRF(in.Config.SecretKey)
	keys, err := keysFor(in)
	if err != nil {
		return nil, err
	}
	sealer, recovery, passkeys, runnerKeys := keys.totp, keys.recovery, keys.passkeys, keys.runner
	external, err := externalAuth(in, store)
	if err != nil {
		return nil, err
	}
	bus := eventbus.New(in.Log)
	limits := func() config.RateLimits { return store.Current().Values.RateLimits }
	limiter := ratelimit.NewPostgres(in.Pool, in.Now)

	auditService := audit.NewService(in.Pool, in.Log, in.Now)
	audit.Subscribe(bus, auditService)
	sessionService := session.NewService(session.Options{
		DB: in.Pool, Hasher: hasher, Passwords: passwords, CSRF: csrf, Sealer: sealer, Recovery: recovery, WebAuthn: passkeys,
		Bus: bus, Instance: store.Instance, Log: in.Log, Now: in.Now,
		Oidc: external.providers, OidcKey: external.oidcKey, CallbackBase: in.Config.PublicURL.String() + config.OidcCallbackPath,
		EnvProviders: envProviderIDs(in.Config.OIDCProviders),
	})
	mail := mailerFor(in, store)
	alertsService := alertsFor(in, mail, bus)
	identityService := identity.NewService(identity.Options{
		DB: in.Pool, Hasher: hasher, Passwords: passwords, Bus: bus, Factors: sessionService, Mailer: mail,
		PublicURL: in.Config.PublicURL.String(), Limiter: limiter, Limits: limits, Log: in.Log, Now: in.Now,
	})

	runnersService := runners.NewService(runners.Options{
		DB: in.Pool, Keys: runnerKeys, Bus: bus, Origin: auth.OriginOf(in.Config.PublicURL), PublicURL: in.Config.PublicURL.String(),
		Dev: in.Config.Dev, Log: in.Log, Now: in.Now,
	})
	tokensService := tokens.NewService(tokens.Options{
		DB: in.Pool, Tokens: external.apiTokens, Factors: sessionService, Bus: bus, Log: in.Log, Now: in.Now,
	})
	gate := runnerauth.NewGate(runnerauth.GateOptions{Verifier: runnersService, Log: in.Log, Now: in.Now})
	secrets := newCheckoutSecrets(in)
	jobsService, logPool, err := buildJobs(ctx, in, runnerKeys, bus, store, secrets)
	if err != nil {
		return nil, err
	}
	pipelinesService, sourcesService, artifactsService, err := runsFor(in, bus, limiter, jobsService, secrets)
	if err != nil {
		return nil, err
	}

	opts := Options{
		Config: in.Config,
		Log:    in.Log,
		System: system.Deps{
			DB: in.Pool, Migrations: in.Migrations, Queries: sqlc.New(in.Pool), Build: in.Build, Log: in.Log,
			Checks: readinessChecks(jobsService, store),
		},
		Identity:  identityService,
		Session:   sessionService,
		Audit:     auditService,
		Runners:   runnersService,
		Jobs:      jobsService,
		Projects:  projects.NewService(projects.Options{DB: in.Pool, Bus: bus, Log: in.Log, Now: in.Now}),
		Sources:   sourcesService,
		Pipelines: pipelinesService,
		Artifacts: artifactsService,
		Overview:  overview.NewService(overview.Options{Counter: sqlc.New(in.Pool), Log: in.Log, Now: in.Now}),
		Tokens:    tokensService,
		Prefs:     preferences.NewService(in.Pool, in.Log, in.Now),
		Gate:      gate,
		CSRF:      csrf,
		APITokens: external.apiTokens,
		Limits:    ratelimit.NewLivePerClient(limiter, func() int { return limits().Public }, config.RateLimitPublicWindow, in.Log),
		Webhooks:  limiter,
		Settings:  settingsFor(in, store, sessionService, bus, mail, limiter, external.providers),
		Instance:  store.Instance,
		Now:       in.Now,
		Web:       in.Web,
	}
	wired := &Wired{
		Options: opts, Handler: NewRouter(opts), Identity: identityService, Jobs: jobsService, JobQueue: jobsService.Queue(),
		Projects: opts.Projects, Sources: sourcesService, Pipelines: pipelinesService, Artifacts: artifactsService, LogPool: logPool, Settings: store, Alerts: alertsService,
		Retention: retention.New(retention.Options{Pool: in.Pool, Log: in.Log, Now: in.Now}),
	}
	if in.Config.RunnerPlaneSeparate() {
		wired.RunnerHandler = RunnerPlaneServer(opts)
	}
	return wired, nil
}

type keyset struct {
	totp     *appcrypto.Sealer
	recovery *auth.RecoveryHasher
	passkeys *webauthn.WebAuthn
	runner   *runnerauth.Keys
}

func keysFor(in Infra) (keyset, error) {
	var k keyset
	var err error
	if k.totp, err = appcrypto.NewSealer(in.Config.SecretKey, config.TotpSealLabel); err != nil {
		return k, err
	}
	if k.recovery, err = auth.NewRecoveryHasher(in.Config.SecretKey); err != nil {
		return k, err
	}
	if k.passkeys, err = auth.NewWebAuthn(in.Config.PublicURL, config.WebAuthnDisplayName, config.WebAuthnChallengeTTL); err != nil {
		return k, err
	}
	k.runner, err = runnerauth.DeriveKeys(in.Config.SecretKey)
	return k, err
}

func readinessChecks(jobsService *jobs.Service, store *instancesettings.Store) []system.ReadinessCheck {
	return []system.ReadinessCheck{
		{Name: systemmodels.CheckLogPartitions, Run: jobsService.CheckLogPartition},
		{Name: systemmodels.CheckSettings, Run: store.Ready},
	}
}

func settingsFor(
	in Infra, store *instancesettings.Store, factors auth.FactorConfirmer, bus *eventbus.Bus, mail mailer.Sender,
	limiter ratelimit.Limiter, providers *oidcauth.Registry,
) *settings.Service {
	return settings.NewService(settings.Options{
		Store: store, DB: in.Pool, Factors: factors, Bus: bus, Mailer: mail, Limiter: limiter, Log: in.Log, Now: in.Now,
		Providers: providers, CallbackBase: in.Config.PublicURL.String() + config.OidcCallbackPath, Dev: in.Config.Dev,
	})
}

func buildJobs(
	ctx context.Context, in Infra, keys *runnerauth.Keys, bus *eventbus.Bus, store *instancesettings.Store, secrets jobs.SecretResolver,
) (*jobs.Service, *pgxpool.Pool, error) {
	logPool, err := logReadPool(ctx, in.Pool, in.Config.Logs)
	if err != nil {
		return nil, nil, err
	}
	opts := jobs.Options{
		DB: in.Pool, Keys: keys, Secrets: secrets, Bus: bus, Listen: listener(in.Pool),
		MaxPolls: in.Config.RunnerMaxPolls, Logs: in.Config.Logs, StreamWriteWait: in.LogStreamWriteWait,
		ReadWait: in.LogReadWait, Log: in.Log, ManualJobs: in.Config.Dev || in.ManualJobs,
		LogSettings: func() config.LogSettings { return store.Current().Values.Logs },
	}
	if logPool != nil {
		opts.Reads = logPool
	}
	service := jobs.NewService(opts)
	jobs.Subscribe(bus, service)
	return service, logPool, nil
}

func logReadPool(ctx context.Context, pool *pgxpool.Pool, logs config.Logs) (*pgxpool.Pool, error) {
	if pool == nil {
		return nil, nil
	}
	conns := logs.ReadConns
	if conns <= 0 {
		conns = config.LogReadConnsDefault
	}
	cfg := pool.Config().Copy()
	cfg.MaxConns = int32(conns)
	cfg.MinConns = 0
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(config.LogReadStatementWait.Milliseconds(), 10)
	cfg.ConnConfig.RuntimeParams["application_name"] = config.DatabaseAppName + "-logs"
	logPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open the job log read pool: %w", err)
	}
	return logPool, nil
}

type externalDeps struct {
	providers *oidcauth.Registry
	oidcKey   []byte
	apiTokens *auth.APITokens
}

func externalAuth(in Infra, store *instancesettings.Store) (externalDeps, error) {
	providers, err := oidcauth.NewRegistry(store.Current().Oidc.Active(), oidcauth.Options{Transport: in.OidcTransport, Now: in.Now, AllowHTTP: in.Config.Dev})
	if err != nil {
		return externalDeps{}, err
	}
	store.Watch(func(snap *instancesettings.Snapshot) {
		if err := providers.Replace(snap.Oidc.Active()); err != nil {
			in.Log.Error("sign-in providers were not rebuilt; the previous ones stay in effect", "error", err)
		}
	})
	oidcKey, err := appcrypto.DeriveKey(in.Config.SecretKey, config.OidcFlowKeyLabel)
	if err != nil {
		return externalDeps{}, err
	}
	apiTokens, err := auth.NewAPITokens(in.Config.SecretKey)
	if err != nil {
		return externalDeps{}, err
	}
	return externalDeps{providers: providers, oidcKey: oidcKey, apiTokens: apiTokens}, nil
}

func listener(pool *pgxpool.Pool) jobs.Listener {
	if pool == nil {
		return nil
	}
	return func(ctx context.Context) (*pgx.Conn, error) {
		return pgx.ConnectConfig(ctx, pool.Config().ConnConfig.Copy())
	}
}

func alertsFor(in Infra, mail mailer.Sender, bus *eventbus.Bus) *alerts.Service {
	service := alerts.NewService(alerts.Options{
		DB: in.Pool, Mailer: mail, PublicURL: in.Config.PublicURL.String(), Log: in.Log, Now: in.Now,
	})
	alerts.Subscribe(bus, service)
	return service
}

func mailerFor(in Infra, store *instancesettings.Store) mailer.Sender {
	if in.Mailer != nil {
		return in.Mailer
	}
	return instancesettings.NewMailer(store, in.MailTLS, in.Now)
}

func passwordsFor(in Infra, store *instancesettings.Store) *auth.PasswordPolicy {
	var breach auth.BreachChecker = auth.NewHIBP(config.BreachedRangeURL, config.BreachedCheckTimeout, nil)
	if in.Breach != nil {
		breach = in.Breach
	}
	return auth.NewPasswordPolicy(instancesettings.NewBreach(store, breach), in.Log, config.PasswordMinLength, config.PasswordMaxLength)
}

func settingsStore(ctx context.Context, in Infra) (*instancesettings.Store, error) {
	opts := instancesettings.Options{
		SecretKey: in.Config.SecretKey, Env: in.Config.Settings, EnvOIDC: in.Config.OIDCProviders, Dev: in.Config.Dev, Log: in.Log, Now: in.Now,
	}
	if in.Pool != nil {
		opts.DB = in.Pool
		opts.Listen = instancesettings.Listener(listener(in.Pool))
	}
	store, err := instancesettings.New(opts)
	if err != nil {
		return nil, fmt.Errorf("invalid instance settings:\n%w", err)
	}
	if err := store.Load(ctx); err != nil {
		return nil, fmt.Errorf("invalid instance settings:\n%w", err)
	}
	return store, nil
}

func envProviderIDs(providers []config.OIDCProvider) []string {
	out := make([]string, 0, len(providers))
	for _, p := range providers {
		out = append(out, p.ID)
	}
	return out
}
