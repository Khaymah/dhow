package app

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	runnerauth "github.com/Khaymah/dhow/apps/server/internal/Core/RunnerAuth"
	artifacts "github.com/Khaymah/dhow/apps/server/internal/Modules/Artifacts"
	audit "github.com/Khaymah/dhow/apps/server/internal/Modules/Audit"
	environments "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments"
	identity "github.com/Khaymah/dhow/apps/server/internal/Modules/Identity"
	ingest "github.com/Khaymah/dhow/apps/server/internal/Modules/Ingest"
	jobs "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs"
	overview "github.com/Khaymah/dhow/apps/server/internal/Modules/Overview"
	pipelines "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines"
	preferences "github.com/Khaymah/dhow/apps/server/internal/Modules/Preferences"
	projects "github.com/Khaymah/dhow/apps/server/internal/Modules/Projects"
	runners "github.com/Khaymah/dhow/apps/server/internal/Modules/Runners"
	secrets "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets"
	session "github.com/Khaymah/dhow/apps/server/internal/Modules/Session"
	settings "github.com/Khaymah/dhow/apps/server/internal/Modules/Settings"
	sources "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
	tokens "github.com/Khaymah/dhow/apps/server/internal/Modules/Tokens"
)

type Options struct {
	Config    config.Config
	Log       *slog.Logger
	System    system.Deps
	Identity  *identity.Service
	Session   *session.Service
	Audit     *audit.Service
	Runners   *runners.Service
	Jobs      *jobs.Service
	Projects  *projects.Service
	Envs      *environments.Service
	Secrets   *secrets.Service
	Sources   *sources.Service
	Pipelines *pipelines.Service
	Artifacts *artifacts.Service
	Ingest    *ingest.Service
	Overview  *overview.Service
	Tokens    *tokens.Service
	Prefs     *preferences.Service
	Gate      *runnerauth.Gate
	CSRF      *auth.CSRF
	APITokens *auth.APITokens
	Limits    *ratelimit.PerClient
	Webhooks  ratelimit.Limiter
	Settings  *settings.Service
	Instance  func() policy.InstanceSettings
	Now       func() time.Time
	Web       http.Handler
}

func Policies() []policy.Rule {
	var rules []policy.Rule
	rules = append(rules, system.Policies()...)
	rules = append(rules, identity.Policies()...)
	rules = append(rules, session.Policies()...)
	rules = append(rules, audit.Policies()...)
	rules = append(rules, runners.Policies()...)
	rules = append(rules, jobs.Policies()...)
	rules = append(rules, projects.Policies()...)
	rules = append(rules, environments.Policies()...)
	rules = append(rules, secrets.Policies()...)
	rules = append(rules, sources.Policies()...)
	rules = append(rules, pipelines.Policies()...)
	rules = append(rules, artifacts.Policies()...)
	rules = append(rules, ingest.Policies()...)
	rules = append(rules, overview.Policies()...)
	rules = append(rules, tokens.Policies()...)
	rules = append(rules, preferences.Policies()...)
	rules = append(rules, settings.Policies()...)
	return rules
}

func RateLimited() []ratelimit.Route {
	return append(identity.RateLimited(), session.RateLimited()...)
}

func RunnerPolicies() []policy.Rule {
	return append(append(append(runners.Procedures(), jobs.Procedures()...), artifacts.Procedures()...), environments.Procedures()...)
}

type noSessions struct{}

func (noSessions) Resolve(context.Context, []byte, time.Time) (auth.Resolved, error) {
	return auth.Resolved{}, auth.ErrNoSession
}

func (noSessions) Peek(context.Context, []byte, time.Time) (auth.Resolved, error) {
	return auth.Resolved{}, auth.ErrNoSession
}

func (o Options) guard() *auth.Guard {
	var resolver auth.Resolver = noSessions{}
	if o.Session != nil {
		resolver = o.Session
	}
	csrf := o.CSRF
	if csrf == nil {
		csrf = auth.NewCSRF(o.Config.SecretKey)
	}
	var members auth.MembershipToucher
	if o.Session != nil {
		members = o.Session
	}
	var tokenResolver auth.TokenResolver
	if o.Tokens != nil {
		tokenResolver = o.Tokens
	}
	var projectResolver auth.ProjectResolver
	if o.Projects != nil {
		projectResolver = o.Projects
	}
	return auth.NewGuard(auth.GuardOptions{
		Resolver: resolver, Members: members, Tokens: tokenResolver, Projects: projectResolver, APITokens: o.APITokens, CSRF: csrf, PublicURL: o.Config.PublicURL, Log: o.Log, Now: o.Now,
		Instance: o.Instance,
	})
}

func NewRouter(o Options) http.Handler {
	public := publicRouter(o)
	root := chi.NewRouter()
	root.MethodNotAllowed(methodNotAllowed)
	if o.Runners != nil && !o.Config.RunnerPlaneSeparate() {
		root.Mount(config.RunnerAPIPrefix, RunnerPlane(o))
	}
	mountWebhooks(root, o)
	mountIngest(root, o)
	root.Mount("/", public)
	return root
}

func publicRouter(o Options) chi.Router {
	r := chi.NewRouter()
	r.Use(
		middleware.RequestID,
		middleware.ClientIP(o.Config.TrustedProxies),
		middleware.Lang,
		middleware.AccessLog(o.Log),
		middleware.Recover(o.Log),
		middleware.SecurityHeaders(o.Config.PublicURL.Scheme == "https"),
		middleware.BodyLimit(config.MaxBodyBytes),
	)
	r.MethodNotAllowed(methodNotAllowed)

	guard := o.guard()
	r.Route(config.APIPrefix, func(api chi.Router) {
		api.Use(middleware.NoStore)
		api.NotFound(notFound)
		api.Group(func(timed chi.Router) {
			timed.Use(middleware.Timeout(config.APITimeout))
			o.System.Guard = guard
			system.Routes(timed, o.System)
			if o.Identity != nil {
				identity.Routes(timed, identity.Deps{Service: o.Identity, Guard: guard, Limits: o.Limits, Log: o.Log})
			}
			if o.Session != nil {
				session.Routes(timed, session.Deps{Service: o.Session, Guard: guard, Limits: o.Limits, Log: o.Log})
			}
			if o.Audit != nil {
				audit.Routes(timed, audit.Deps{Service: o.Audit, Guard: guard, Log: o.Log})
			}
			if o.Runners != nil {
				runners.Routes(timed, runners.Deps{Service: o.Runners, Guard: guard, Log: o.Log})
			}
			if o.Projects != nil {
				projects.Routes(timed, projects.Deps{Service: o.Projects, Guard: guard, Log: o.Log})
			}
			if o.Envs != nil {
				environments.Routes(timed, environments.Deps{Service: o.Envs, Guard: guard, Log: o.Log})
			}
			if o.Secrets != nil {
				secrets.Routes(timed, secrets.Deps{Service: o.Secrets, Guard: guard, Log: o.Log})
			}
			if o.Sources != nil {
				sources.Routes(timed, sources.Deps{Service: o.Sources, Guard: guard, Log: o.Log})
			}
			if o.Ingest != nil {
				ingest.Routes(timed, ingest.Deps{Service: o.Ingest, Guard: guard, Log: o.Log})
			}
			if o.Pipelines != nil {
				pipelines.Routes(timed, pipelines.Deps{Service: o.Pipelines, Guard: guard, Log: o.Log})
			}
			if o.Overview != nil {
				overview.Routes(timed, overview.Deps{Service: o.Overview, Guard: guard, Log: o.Log})
			}
			if o.Settings != nil {
				settings.Routes(timed, settings.Deps{Service: o.Settings, Guard: guard, Log: o.Log})
			}
		})
		if o.Jobs != nil {
			jobs.Routes(api, jobs.Deps{Service: o.Jobs, Guard: guard, Log: o.Log})
		}
		if o.Artifacts != nil {
			artifacts.Routes(api, artifacts.Deps{Service: o.Artifacts, Guard: guard, Log: o.Log})
		}
		if o.Tokens != nil {
			tokens.Routes(api, tokens.Deps{Service: o.Tokens, Guard: guard, Log: o.Log})
		}
		if o.Prefs != nil {
			preferences.Routes(api, preferences.Deps{Service: o.Prefs, Guard: guard, Log: o.Log})
		}
	})

	web := o.Web
	if web == nil {
		web = WebHandler()
	}
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if isAPIPath(req.URL.Path) {
			notFound(w, req)
			return
		}
		web.ServeHTTP(w, req)
	})
	return r
}

func isAPIPath(p string) bool {
	for _, prefix := range []string{"/api", config.RunnerAPIPrefix, config.WebhookPrefix, ingestRoot} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

func notFound(w http.ResponseWriter, r *http.Request) {
	response.Error(w, r, apperrors.NotFound())
}

func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	response.Error(w, r, apperrors.MethodNotAllowed())
}
