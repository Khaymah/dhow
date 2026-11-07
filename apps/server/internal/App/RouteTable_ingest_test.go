package app_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
)

func ingestPath(env *apptest.Env, path string, project *uuid.UUID, owner uuid.UUID) string {
	if !strings.Contains(path, "{ingestTokenId}") || project == nil {
		return path
	}
	token := env.SeedIngestToken(*project, owner, apptest.IngestSeed{})
	return strings.ReplaceAll(path, "{ingestTokenId}", token.ID.String())
}

func ingestBody(env *apptest.Env, rule policy.Rule) any {
	wrong := map[string]any{"Method": "password", "Password": "not the password"}
	switch {
	case rule.Method == http.MethodPost && strings.HasSuffix(rule.Path, "/ingest-tokens"):
		return map[string]any{
			"Name": "rt-" + randomSuffix(env.T), "RefPatterns": []map[string]string{{"Kind": "branch", "Pattern": "main"}}, "CurrentFactor": wrong,
		}
	case rule.Method == http.MethodPost && (strings.HasSuffix(rule.Path, "/revoke") || strings.HasSuffix(rule.Path, "/acknowledge-reach")):
		return map[string]any{"CurrentFactor": wrong}
	default:
		return notificationsBody(env, rule)
	}
}

func ingestRoutes(t *testing.T, h http.Handler) []route {
	t.Helper()
	mux, ok := h.(chi.Routes)
	require.True(t, ok)
	var out []route
	require.NoError(t, chi.Walk(mux, func(method, pattern string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(pattern, policy.IngestPrefix) {
			out = append(out, route{Method: method, Pattern: pattern})
		}
		return nil
	}))
	sort.Slice(out, func(i, j int) bool { return out[i].key() < out[j].key() })
	return out
}

func TestEveryIngestRouteDeclaresAnIngestRule(t *testing.T) {
	env := apptest.New(t)
	routes := ingestRoutes(t, env.Handler)
	require.NotEmpty(t, routes)
	undeclared, unrouted := missingPolicies(routes, app.IngestPolicies())
	assert.Empty(t, undeclared)
	assert.Empty(t, unrouted)
	for _, r := range app.IngestPolicies() {
		assert.True(t, r.Valid(), r.Key())
		assert.Equal(t, policy.ScopeIngest, r.Scope)
	}
	for _, r := range app.Policies() {
		assert.NotEqual(t, policy.ScopeIngest, r.Scope, "%s: ingest rules never go through the session guard", r.Key())
	}
}

func TestRouteTableIngestRoutesAcceptNoSessionNoAPITokenAndNoRole(t *testing.T) {
	env := apptest.New(t)
	fx := buildFixture(t, env)
	apiToken := env.SeedAPIToken(mustUUID(fx.byRole[policy.RoleOwner]), mustUUID(fx.orgA), "org.read", "project.read", "project.manage")
	digest := strings.Repeat("a", 64)
	for _, rule := range app.IngestPolicies() {
		path := strings.ReplaceAll(rule.Path, "{sha256}", digest)
		for _, p := range principals(fx) {
			t.Run(rule.Key()+"/"+p.name, func(t *testing.T) {
				c := fx.client(env, p)
				headers := map[string]string{"Origin": apptest.PublicOrigin}
				if c.Cookie != "" {
					headers["Cookie"] = config.SessionCookieName + "=" + c.Cookie
					headers[config.CSRFHeaderName] = c.CSRFToken
				}
				res := env.WebhookFrom(apptest.WebhookRequest{Method: rule.Method, Path: path, Headers: headers, Body: []byte(`{}`)})
				assert.Equal(t, http.StatusUnauthorized, res.Status, string(res.Raw))
				require.NotNil(t, res.Error)
				assert.Equal(t, "INGEST_TOKEN_INVALID", res.Error.Code)
			})
		}
		t.Run(rule.Key()+"/api token", func(t *testing.T) {
			res := env.WebhookFrom(apptest.WebhookRequest{
				Method: rule.Method, Path: path, Headers: map[string]string{"Authorization": "Bearer " + apiToken.Value}, Body: []byte(`{}`),
			})
			assert.Equal(t, http.StatusUnauthorized, res.Status, string(res.Raw))
			require.NotNil(t, res.Error)
			assert.Equal(t, "INGEST_TOKEN_INVALID", res.Error.Code)
		})
	}
}

func TestAnIngestTokenNeverOpensTheSessionApi(t *testing.T) {
	env := apptest.New(t)
	fx := buildFixture(t, env)
	project := env.SeedProject(mustUUID(fx.orgA), mustUUID(fx.byRole[policy.RoleOwner]))
	token := env.SeedIngestToken(project, mustUUID(fx.byRole[policy.RoleOwner]), apptest.IngestSeed{})
	path := "/organizations/" + fx.orgA + "/projects/" + project.String() + "/ingest-tokens"
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		res := env.Bearer(token.Value, apptest.Request{Method: method, Path: path})
		assert.Equal(t, http.StatusUnauthorized, res.Status, string(res.Raw))
	}
}
