package app_test

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
)

func secretsPath(env *apptest.Env, path string, org uuid.UUID, project *uuid.UUID) string {
	if !strings.Contains(path, "{secretName}") && !strings.Contains(path, "{variableName}") && !strings.Contains(path, "{credentialId}") {
		return path
	}
	if project != nil {
		require.NoError(env.T, env.Pool.QueryRow(context.Background(),
			`SELECT "OrganizationId" FROM "Projects" WHERE "Id" = $1`, *project).Scan(&org))
	}
	environment := ""
	if strings.Contains(path, "/environments/") {
		environment = apptest.RouteTableEnvironment
	}
	ref := apptest.SecretRef(org, project, environment)
	if strings.Contains(path, "{secretName}") {
		env.SeedSecret(ref, apptest.RouteTableSecret, "a route table secret value")
		path = strings.ReplaceAll(path, "{secretName}", apptest.RouteTableSecret)
	}
	if strings.Contains(path, "{variableName}") {
		env.SeedVariable(ref, apptest.RouteTableVariable, "route table")
		path = strings.ReplaceAll(path, "{variableName}", apptest.RouteTableVariable)
	}
	if strings.Contains(path, "{credentialId}") {
		path = strings.ReplaceAll(path, "{credentialId}", registryCredential(env, ref).String())
	}
	return path
}

func registryCredential(env *apptest.Env, ref models.Ref) uuid.UUID {
	var id uuid.UUID
	err := env.Pool.QueryRow(context.Background(),
		`SELECT "Id" FROM "RegistryCredentials" WHERE "OrganizationId" = $1 AND "ProjectId" IS NOT DISTINCT FROM $2 AND NOT "IsDeleted" LIMIT 1`,
		ref.OrganizationID, ref.ProjectID).Scan(&id)
	if err == nil {
		return id
	}
	return env.SeedRegistryCredential(ref, "rt-"+randomSuffix(env.T)+".example.com", "robot", "a registry password").ID
}

func secretsBody(env *apptest.Env, rule policy.Rule) any {
	path := rule.Path
	switch {
	case rule.Method == http.MethodPut && strings.HasSuffix(path, "/secrets/{secretName}"):
		return map[string]string{"Value": "a route table secret value"}
	case rule.Method == http.MethodPost && strings.HasSuffix(path, "/secrets/{secretName}/restore-version"):
		return map[string]int{"Version": 1}
	case rule.Method == http.MethodPut && strings.HasSuffix(path, "/variables/{variableName}"):
		return map[string]string{"Value": "route table"}
	case rule.Method == http.MethodPost && strings.HasSuffix(path, "/registry-credentials"):
		return map[string]string{"Host": "rt-" + randomSuffix(env.T) + ".example.com", "Username": "robot", "Password": "a registry password"}
	case rule.Method == http.MethodPut && strings.HasSuffix(path, "/registry-credentials/{credentialId}"):
		return map[string]string{"Username": "robot", "Password": "a new registry password"}
	}
	return nil
}

func projectParams(env *apptest.Env, path string, project uuid.UUID) string {
	if strings.Contains(path, "{runId}") {
		path = strings.ReplaceAll(path, "{runId}", env.SeedRun(project, "awaiting_approval").String())
	}
	if strings.Contains(path, "{artifactId}") {
		path = strings.ReplaceAll(path, "{artifactId}", env.SeedArtifact(project, "web", []byte("archive"), false).ID.String())
	}
	if strings.Contains(path, "{environmentName}") {
		env.SeedEnvironment(project, apptest.RouteTableEnvironment)
		path = strings.ReplaceAll(path, "{environmentName}", apptest.RouteTableEnvironment)
	}
	return path
}
