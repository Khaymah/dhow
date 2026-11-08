package preferences

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Preferences/Gen"
)

type Deps struct {
	Service *Service
	Guard   *auth.Guard
	Log     *slog.Logger
}

func Policies() []policy.Rule {
	return []policy.Rule{
		policy.Authenticated(http.MethodGet, "/me/preferences", policy.SessionManageSelf),
		policy.Authenticated(http.MethodPut, "/me/preferences", policy.SessionManageSelf),
		policy.Authenticated(http.MethodPut, "/me/release-notes-seen", policy.SessionManageSelf),
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
		Middlewares:      []gen.MiddlewareFunc{middleware.BodyLimit(config.PreferencesBodyMaxBytes), guard},
		ErrorHandlerFunc: middleware.StrictRequestError,
	})
}
