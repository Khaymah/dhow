package app_test

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
)

func seedNotificationChannel(env *apptest.Env, org uuid.UUID, project *uuid.UUID) (channel, owningOrg uuid.UUID) {
	env.T.Helper()
	require.NoError(env.T, env.Pool.QueryRow(context.Background(),
		`INSERT INTO "NotificationChannels" ("OrganizationId", "ProjectId", "Kind", "Name", "CreatedByUserId")
		 SELECT o."Id", p."Id", 'email', 'rt-' || substr(gen_random_uuid()::text, 1, 8),
		        (SELECT m."UserId" FROM "Memberships" m WHERE m."OrganizationId" = o."Id" AND m."Role" = 'owner' AND NOT m."IsDeleted" LIMIT 1)
		 FROM "Organizations" o
		 LEFT JOIN "Projects" p ON p."Id" = $2 AND p."OrganizationId" = o."Id"
		 WHERE o."Id" = COALESCE((SELECT "OrganizationId" FROM "Projects" WHERE "Id" = $2), $1)
		 RETURNING "Id", "OrganizationId"`, org, project).Scan(&channel, &owningOrg))
	return channel, owningOrg
}

func notificationsPath(env *apptest.Env, path string, org uuid.UUID, project *uuid.UUID) string {
	path = strings.ReplaceAll(path, "{notificationId}", uuid.NewString())
	if !strings.Contains(path, "{channelId}") && !strings.Contains(path, "{deliveryId}") {
		return path
	}
	channel, owningOrg := seedNotificationChannel(env, org, project)
	path = strings.ReplaceAll(path, "{channelId}", channel.String())
	if strings.Contains(path, "{deliveryId}") {
		var delivery uuid.UUID
		require.NoError(env.T, env.Pool.QueryRow(context.Background(),
			`INSERT INTO "NotificationOutbox" ("OrganizationId", "ProjectId", "ChannelId", "Kind", "EventId", "EventName", "Payload", "State")
			 SELECT c."OrganizationId", c."ProjectId", c."Id", 'email', gen_random_uuid(), 'deployment.finished', '{}', 'dead'
			 FROM "NotificationChannels" c WHERE c."Id" = $1 AND c."OrganizationId" = $2 RETURNING "Id"`, channel, owningOrg).Scan(&delivery))
		path = strings.ReplaceAll(path, "{deliveryId}", delivery.String())
	}
	return path
}

func notificationsBody(env *apptest.Env, rule policy.Rule) any {
	wrong := map[string]any{"Method": "password", "Password": "not the password"}
	path := rule.Path
	switch {
	case rule.Method == http.MethodPost && strings.HasSuffix(path, "/notification-channels"):
		return map[string]any{"Kind": "email", "Name": "rt-" + randomSuffix(env.T), "CurrentFactor": wrong}
	case rule.Method == http.MethodPut && strings.HasSuffix(path, "/notification-channels/{channelId}"):
		return map[string]any{"Name": "rt-" + randomSuffix(env.T), "Locale": "ar", "Version": 1, "CurrentFactor": wrong}
	case rule.Method == http.MethodPost && strings.HasSuffix(path, "/notification-channels/{channelId}/remove"):
		return map[string]any{"CurrentFactor": wrong}
	case rule.Method == http.MethodPut && strings.HasSuffix(path, "/notification-rules"):
		return map[string]any{"Rules": []map[string]any{{
			"ChannelId": uuid.NewString(), "Events": []string{"deployment.finished"}, "Statuses": []string{"failed"},
		}}}
	case rule.Method == http.MethodPut && path == "/me/notification-mute":
		return map[string]any{"Muted": true}
	}
	return targetsBody(env, rule)
}
