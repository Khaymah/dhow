package app_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
)

func randomSuffix(t testing.TB) string {
	b := make([]byte, 6)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return hex.EncodeToString(b)
}

func randomEmail(t testing.TB) string { return "rt-" + randomSuffix(t) + "@example.com" }

func seedTeam(env *apptest.Env, org string) string {
	var id string
	require.NoError(env.T, env.Pool.QueryRow(context.Background(),
		`INSERT INTO "Teams" ("OrganizationId", "Name") VALUES ($1, $2) RETURNING "Id"::text`, org, "Team "+randomSuffix(env.T)).Scan(&id))
	return id
}

func seedTeamMember(env *apptest.Env, org, team, user string) {
	_, err := env.Pool.Exec(context.Background(),
		`INSERT INTO "TeamMembers" ("OrganizationId", "TeamId", "UserId") VALUES ($1, $2, $3)`, org, team, user)
	require.NoError(env.T, err)
}

func seedBinding(env *apptest.Env, org, team, role string) string {
	var id string
	require.NoError(env.T, env.Pool.QueryRow(context.Background(),
		`INSERT INTO "RoleBindings" ("OrganizationId", "TeamId", "Scope", "Role") VALUES ($1, $2, 'org', $3) RETURNING "Id"::text`,
		org, team, role).Scan(&id))
	return id
}

func seedInvitation(env *apptest.Env, org, inviter, role string) string {
	hash := make([]byte, 32)
	_, err := rand.Read(hash)
	require.NoError(env.T, err)
	var id string
	require.NoError(env.T, env.Pool.QueryRow(context.Background(),
		`INSERT INTO "Invitations" ("OrganizationId", "Email", "Role", "TokenHash", "InvitedByUserId", "ExpiresAt")
		 VALUES ($1, $2, $3, $4, $5, now() + interval '1 day') RETURNING "Id"::text`,
		org, randomEmail(env.T), role, hash, inviter).Scan(&id))
	return id
}

func auditEventOf(env *apptest.Env, org string) string {
	var id string
	require.NoError(env.T, env.Pool.QueryRow(context.Background(),
		`SELECT "Id"::text FROM "AuditEvents" WHERE "OrganizationId" = $1 ORDER BY "Seq" LIMIT 1`, org).Scan(&id))
	return id
}

func freshMember(env *apptest.Env, org string) string {
	return env.SeedUser(uuid.MustParse(org), randomEmail(env.T), "a long password for a target", string(policy.RoleViewer)).String()
}

func organizationPath(env *apptest.Env, fx fixture, rule policy.Rule) string {
	path := strings.ReplaceAll(rule.Path, "{id}", fx.orgA)
	team := ""
	if strings.Contains(path, "{teamId}") {
		team = seedTeam(env, fx.orgA)
		path = strings.ReplaceAll(path, "{teamId}", team)
	}
	if strings.Contains(path, "{userId}") {
		user := freshMember(env, fx.orgA)
		if team != "" {
			seedTeamMember(env, fx.orgA, team, user)
		}
		path = strings.ReplaceAll(path, "{userId}", user)
	}
	if strings.Contains(path, "{bindingId}") {
		path = strings.ReplaceAll(path, "{bindingId}", seedBinding(env, fx.orgA, team, string(policy.RoleViewer)))
	}
	if strings.Contains(path, "{eventId}") {
		path = strings.ReplaceAll(path, "{eventId}", auditEventOf(env, fx.orgA))
	}
	if strings.Contains(path, "{projectId}") {
		project := env.SeedProject(uuid.MustParse(fx.orgA), uuid.MustParse(fx.byRole[policy.RoleOwner]))
		path = strings.ReplaceAll(path, "{projectId}", project.String())
		if strings.Contains(path, "{runId}") {
			path = strings.ReplaceAll(path, "{runId}", env.SeedRun(project, "awaiting_approval").String())
		}
	}
	if strings.Contains(path, "{invitationId}") {
		path = strings.ReplaceAll(path, "{invitationId}", seedInvitation(env, fx.orgA, fx.byRole[policy.RoleOwner], string(policy.RoleViewer)))
	}
	return path
}

func organizationBody(env *apptest.Env, fx fixture, rule policy.Rule) any {
	wrong := map[string]string{"Method": "password", "Password": "not the password"}
	switch rule.Method + " " + rule.Path {
	case "POST /organizations":
		return map[string]string{"Name": "Route table org", "Slug": "rt-" + randomSuffix(env.T)}
	case "POST /organizations/{id}/transfer-ownership":
		return map[string]any{"UserId": freshMember(env, fx.orgA), "CurrentFactor": wrong}
	case "PATCH /organizations/{id}/members/{userId}":
		return map[string]string{"Role": string(policy.RoleDeveloper)}
	case "POST /organizations/{id}/members/{userId}/mfa-reset":
		return map[string]any{"CurrentFactor": wrong}
	case "POST /organizations/{id}/teams", "PATCH /organizations/{id}/teams/{teamId}":
		return map[string]string{"Name": "Route table team " + randomSuffix(env.T)}
	case "POST /organizations/{id}/teams/{teamId}/members":
		return map[string]string{"UserId": freshMember(env, fx.orgA)}
	case "POST /organizations/{id}/teams/{teamId}/role-bindings":
		return map[string]string{"Scope": "org", "Role": string(policy.RoleViewer)}
	case "PATCH /organizations/{id}/teams/{teamId}/role-bindings/{bindingId}":
		return map[string]string{"Role": string(policy.RoleDeveloper)}
	case "POST /organizations/{id}/projects":
		return map[string]string{"Name": "Route table project", "Slug": "rt-" + randomSuffix(env.T)}
	case "POST /organizations/{id}/projects/{projectId}/runs":
		return map[string]string{"Ref": "main"}
	case "POST /organizations/{id}/invitations":
		return map[string]string{"Email": randomEmail(env.T), "Role": string(policy.RoleViewer)}
	case "POST /invitations/preview", "POST /me/invitations/accept":
		return map[string]string{"Token": "not-a-token"}
	case "POST /invitations/accept":
		return map[string]string{"Token": "not-a-token", "Name": "Route Table", "Password": "a long enough password"}
	default:
		return nil
	}
}
