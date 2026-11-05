package app_test

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
)

func targetsPath(env *apptest.Env, path string, project *uuid.UUID) string {
	if !strings.Contains(path, "{targetName}") || project == nil {
		return path
	}
	_, host := env.SeedDeployTarget(*project, apptest.RouteTableEnvironment, apptest.RouteTableTarget)
	path = strings.ReplaceAll(path, "{targetName}", apptest.RouteTableTarget)
	return strings.ReplaceAll(path, "{hostId}", host.String())
}

func targetsBody(env *apptest.Env, rule policy.Rule) any {
	wrong := map[string]any{"Method": "password", "Password": "not the password"}
	path := rule.Path
	switch {
	case rule.Method == http.MethodPost && strings.HasSuffix(path, "/targets"):
		return map[string]any{
			"Name": "rt-" + randomSuffix(env.T), "Kind": "ssh", "Username": "deploy", "DeployPath": "/srv/rt-created",
			"Hosts": []map[string]any{{"Host": "10.0.0.6"}}, "CurrentFactor": wrong,
		}
	case rule.Method == http.MethodPut && strings.HasSuffix(path, "/targets/{targetName}"):
		return map[string]any{"Version": 1, "Username": "ops", "CurrentFactor": wrong}
	case rule.Method == http.MethodPost && (strings.HasSuffix(path, "/targets/{targetName}/remove") || strings.HasSuffix(path, "/generate-key")):
		return map[string]any{"CurrentFactor": wrong}
	case rule.Method == http.MethodPost && strings.HasSuffix(path, "/targets/{targetName}/hosts"):
		return map[string]any{"Version": 1, "Host": "10.0.0.7", "CurrentFactor": wrong}
	case rule.Method == http.MethodPut && strings.HasSuffix(path, "/hosts-order"):
		return map[string]any{"Version": 1, "HostIds": []string{uuid.NewString()}, "CurrentFactor": wrong}
	case rule.Method == http.MethodPut && strings.HasSuffix(path, "/hosts/{hostId}"):
		return map[string]any{"Version": 1, "Port": 2222, "CurrentFactor": wrong}
	case rule.Method == http.MethodPost && strings.HasSuffix(path, "/hosts/{hostId}/remove"):
		return map[string]any{"Version": 1, "CurrentFactor": wrong}
	case rule.Method == http.MethodPut && strings.HasSuffix(path, "/pending-host-key"):
		return map[string]string{"HostKey": "not a key"}
	case rule.Method == http.MethodPost && strings.HasSuffix(path, "/confirm-host-key"):
		return map[string]any{"Fingerprint": "SHA256:" + strings.Repeat("A", 43), "CurrentFactor": wrong}
	}
	return secretsBody(env, rule)
}
