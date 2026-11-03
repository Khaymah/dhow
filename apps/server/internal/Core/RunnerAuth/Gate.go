package runnerauth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	"github.com/Khaymah/dhow/internal/logging"
)

type GateOptions struct {
	Verifier Verifier
	Log      *slog.Logger
	Now      func() time.Time
}

type Gate struct {
	verifier Verifier
	log      *slog.Logger
	now      func() time.Time
}

func NewGate(o GateOptions) *Gate {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Gate{verifier: o.Verifier, log: o.Log, now: now}
}

func (g *Gate) RequestGate(table *policy.Table) connect.HandlerOption {
	return connect.WithRequestGate(func(ctx context.Context, spec connect.Spec, _ connect.Peer, header http.Header) (context.Context, error) {
		return g.check(ctx, table, http.MethodPost, spec.Procedure, header)
	})
}

func (g *Gate) HTTP(table *policy.Table, method, pattern string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != method {
				writeHTTPError(w, http.StatusMethodNotAllowed, CodePermissionDenied)
				return
			}
			ctx, err := g.check(r.Context(), table, method, pattern, r.Header)
			if err != nil {
				var ce *connect.Error
				if !errors.As(err, &ce) {
					writeHTTPError(w, http.StatusInternalServerError, CodeInternal)
					return
				}
				writeHTTPError(w, httpStatus(ce.Code()), ce.Message())
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func httpStatus(c connect.Code) int {
	switch c {
	case connect.CodeUnauthenticated:
		return http.StatusUnauthorized
	case connect.CodePermissionDenied:
		return http.StatusForbidden
	case connect.CodeFailedPrecondition:
		return http.StatusPreconditionFailed
	default:
		return http.StatusInternalServerError
	}
}

type httpError struct {
	Code string `json:"Code"`
}

func writeHTTPError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(httpError{Code: code})
}

func (g *Gate) check(ctx context.Context, table *policy.Table, method, procedure string, header http.Header) (context.Context, error) {
	ctx = WithRequestMeta(ctx, RequestMeta{
		IP: middleware.ClientIPFrom(ctx), RunnerVersion: logging.UserValue(header.Get(config.RunnerVersionHeader)),
	})
	rule, ok := table.Lookup(method, procedure)
	if !ok || rule.Scope != policy.ScopeRunner {
		g.log.ErrorContext(ctx, "runner procedure has no valid policy declaration",
			"request_id", middleware.RequestIDFrom(ctx), "procedure", logging.UserValue(procedure))
		return ctx, PermissionDenied()
	}
	if header.Get(config.RunnerProtocolHeader) != config.RunnerProtocolVersion {
		return ctx, UpgradeRequired()
	}
	if carriesBrowserSession(header) {
		return ctx, Unauthenticated()
	}
	if rule.Runner != policy.RunnerSession {
		return ctx, nil
	}
	hash, ok := HashAccessToken(BearerToken(header))
	if !ok {
		return ctx, Unauthenticated()
	}
	p, err := g.verifier.Resolve(ctx, hash, g.now())
	switch {
	case errors.Is(err, ErrRevoked):
		return ctx, RunnerRevoked()
	case errors.Is(err, ErrNoSession):
		return ctx, Unauthenticated()
	case err != nil:
		g.log.ErrorContext(ctx, "resolve runner session", "request_id", middleware.RequestIDFrom(ctx), "error", err)
		return ctx, Internal()
	}
	return WithPrincipal(ctx, p), nil
}

func carriesBrowserSession(header http.Header) bool {
	r := http.Request{Header: header}
	for _, name := range []string{config.SessionCookieName, config.MfaCookieName} {
		if _, err := r.Cookie(name); err == nil {
			return true
		}
	}
	return false
}
