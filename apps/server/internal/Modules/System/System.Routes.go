package system

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Gen"
	"github.com/Khaymah/dhow/internal/buildinfo"
)

type Deps struct {
	DB         Pinger
	Migrations MigrationStatus
	Queries    InstanceQuerier
	Build      buildinfo.Info
	Log        *slog.Logger
	Guard      *auth.Guard
	Checks     []ReadinessCheck
	Keys       KeyringReader
	Doctor     DoctorOptions
}

func Policies() []policy.Rule {
	return []policy.Rule{
		policy.Public(http.MethodGet, "/health/live"),
		policy.Public(http.MethodGet, "/health/ready"),
		policy.Public(http.MethodGet, "/meta"),
		policy.Instance(http.MethodGet, keysPath, policy.InstanceSettingsManage),
		policy.Authenticated(http.MethodGet, doctorPath, policy.SessionManageSelf),
	}
}

func Routes(r chi.Router, d Deps) {
	health := NewHealthService(d.DB, d.Migrations, d.Log, d.Checks...)
	controller := NewController(health, NewMetaService(d.Queries, d.Build)).
		WithKeys(d.Keys).
		WithDoctor(NewDoctorService(health, d.Build, d.Doctor))
	strict := gen.NewStrictHandlerWithOptions(controller, nil, gen.StrictHTTPServerOptions{
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
