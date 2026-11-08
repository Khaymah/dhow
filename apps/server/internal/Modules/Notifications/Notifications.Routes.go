package notifications

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Gen"
)

const (
	projectBase = "/organizations/{id}/projects/{projectId}"
	orgBase     = "/organizations/{id}"
	mutePath    = "/me/notification-mute"
	inboxPath   = "/me/notifications"
)

type Deps struct {
	Service *Service
	Guard   *auth.Guard
	Log     *slog.Logger
}

func projectRules(base string) []policy.Rule {
	read := func(method, path string) policy.Rule {
		return policy.Project(method, base+path, policy.ProjectRead, "id", "projectId")
	}
	manage := func(method, path string) policy.Rule {
		return policy.Project(method, base+path, policy.NotificationManage, "id", "projectId")
	}
	return []policy.Rule{
		read(http.MethodGet, "/notification-channels"),
		manage(http.MethodPost, "/notification-channels"),
		read(http.MethodGet, "/notification-channels/{channelId}"),
		manage(http.MethodPut, "/notification-channels/{channelId}"),
		manage(http.MethodPost, "/notification-channels/{channelId}/remove"),
		manage(http.MethodPost, "/notification-channels/{channelId}/test"),
		read(http.MethodGet, "/notification-rules"),
		manage(http.MethodPut, "/notification-rules"),
		manage(http.MethodGet, "/notification-deliveries"),
		manage(http.MethodPost, "/notification-deliveries/{deliveryId}/retry"),
	}
}

func orgRules(base string) []policy.Rule {
	read := func(method, path string) policy.Rule {
		return policy.Organization(method, base+path, policy.OrgRead, "id")
	}
	manage := func(method, path string) policy.Rule {
		return policy.Organization(method, base+path, policy.OrgNotificationManage, "id")
	}
	return []policy.Rule{
		read(http.MethodGet, "/notification-channels"),
		manage(http.MethodPost, "/notification-channels"),
		read(http.MethodGet, "/notification-channels/{channelId}"),
		manage(http.MethodPut, "/notification-channels/{channelId}"),
		manage(http.MethodPost, "/notification-channels/{channelId}/remove"),
		manage(http.MethodPost, "/notification-channels/{channelId}/test"),
		read(http.MethodGet, "/notification-rules"),
		manage(http.MethodPut, "/notification-rules"),
		manage(http.MethodGet, "/notification-deliveries"),
		manage(http.MethodPost, "/notification-deliveries/{deliveryId}/retry"),
	}
}

func Policies() []policy.Rule {
	rules := append(projectRules(projectBase), orgRules(orgBase)...)
	return append(rules,
		policy.Authenticated(http.MethodGet, mutePath, policy.SessionManageSelf),
		policy.Authenticated(http.MethodPut, mutePath, policy.SessionManageSelf),
		policy.Authenticated(http.MethodGet, inboxPath, policy.SessionManageSelf),
		policy.Authenticated(http.MethodGet, inboxPath+"/unread-count", policy.SessionManageSelf),
		policy.Authenticated(http.MethodPost, inboxPath+"/read-all", policy.SessionManageSelf),
		policy.Authenticated(http.MethodPost, inboxPath+"/{notificationId}/read", policy.SessionManageSelf),
	)
}

func Routes(r chi.Router, d Deps) {
	strict := gen.NewStrictHandlerWithOptions(NewController(d.Service), nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  middleware.StrictRequestError,
		ResponseErrorHandlerFunc: middleware.StrictResponseError(d.Log),
	})
	guard := d.Guard.Middleware(policy.NewTable(Policies()...))
	gen.HandlerWithOptions(strict, gen.ChiServerOptions{
		BaseRouter:       r,
		Middlewares:      []gen.MiddlewareFunc{middleware.BodyLimit(config.AuthBodyMaxBytes), guard},
		ErrorHandlerFunc: middleware.StrictRequestError,
	})
}
