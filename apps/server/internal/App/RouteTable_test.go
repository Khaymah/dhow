package app_test

import (
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
)

type route struct {
	Method  string
	Pattern string
}

func (r route) key() string { return r.Method + " " + r.Pattern }

func registeredRoutes(t *testing.T, h http.Handler) []route {
	t.Helper()
	mux, ok := h.(chi.Routes)
	require.True(t, ok, "the application handler must expose its chi routes")
	var out []route
	err := chi.Walk(mux, func(method, pattern string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		pattern = strings.ReplaceAll(pattern, "/*/", "/")
		if rest, found := strings.CutPrefix(pattern, config.APIPrefix); found {
			out = append(out, route{Method: method, Pattern: rest})
		}
		return nil
	})
	require.NoError(t, err)
	sort.Slice(out, func(i, j int) bool { return out[i].key() < out[j].key() })
	return out
}

func missingPolicies(routes []route, rules []policy.Rule) (undeclared, unrouted []string) {
	declared := map[string]bool{}
	for _, r := range rules {
		declared[r.Key()] = true
	}
	routed := map[string]bool{}
	for _, r := range routes {
		routed[r.key()] = true
		if !declared[r.key()] {
			undeclared = append(undeclared, r.key())
		}
	}
	for _, r := range rules {
		if !routed[r.Key()] {
			unrouted = append(unrouted, r.Key())
		}
	}
	return undeclared, unrouted
}

func TestEveryRegisteredRouteDeclaresAPolicy(t *testing.T) {
	env := apptest.New(t)
	routes := registeredRoutes(t, env.Handler)
	require.NotEmpty(t, routes)

	undeclared, unrouted := missingPolicies(routes, app.Policies())
	assert.Empty(t, undeclared, "these routes are registered without a declared policy")
	assert.Empty(t, unrouted, "these policies are declared for routes that do not exist")

	rules := app.Policies()
	seen := map[string]bool{}
	for _, r := range rules {
		assert.True(t, r.Valid(), "invalid rule %s", r.Key())
		assert.False(t, seen[r.Key()], "duplicate rule %s", r.Key())
		seen[r.Key()] = true
	}
	t.Logf("%d routes, %d policies", len(routes), len(rules))
}

func TestARouteWithoutAPolicyFailsTheCheck(t *testing.T) {
	routes := []route{{"GET", "/me"}, {"GET", "/secret"}, {"POST", "/sessions"}}
	rules := []policy.Rule{
		policy.Authenticated(http.MethodGet, "/me", policy.SessionManageSelf),
		policy.Public(http.MethodPost, "/sessions"),
		policy.Public(http.MethodGet, "/ghost"),
	}
	undeclared, unrouted := missingPolicies(routes, rules)
	assert.Equal(t, []string{"GET /secret"}, undeclared)
	assert.Equal(t, []string{"GET /ghost"}, unrouted)
}

type principal struct {
	name string
	kind string
	role policy.Role
}

type fixture struct {
	orgA       string
	orgB       string
	byRole     map[policy.Role]string
	outsiderID string
	loneID     string
	runnerID   string
	tokenID    string
	jobID      string
}

func buildFixture(t *testing.T, env *apptest.Env) fixture {
	t.Helper()
	d := apptest.DefaultSetup()
	require.Equal(t, http.StatusOK, env.Setup(d).Status)
	orgA := env.OrganizationID(d.OrgSlug)
	orgB := env.SeedOrganization("Other Org", "other-org")
	fx := fixture{orgA: orgA.String(), orgB: orgB.String(), byRole: map[policy.Role]string{}}
	var ownerID string
	require.NoError(t, env.Pool.QueryRow(t.Context(), `SELECT "Id"::text FROM "Users" WHERE "Email" = 'owner@example.com'`).Scan(&ownerID))
	fx.byRole[policy.RoleOwner] = ownerID
	for _, role := range []policy.Role{policy.RoleAdmin, policy.RoleMaintainer, policy.RoleDeveloper, policy.RoleViewer} {
		fx.byRole[role] = env.SeedUser(orgA, string(role)+"@example.com", "a long password for "+string(role), string(role)).String()
	}
	fx.outsiderID = env.SeedUser(orgB, "outsider@example.com", "a long password for outsider", "owner").String()
	fx.loneID = env.SeedUserWithoutMembership("lone@example.com", "a long password for lone").String()
	fx.runnerID = env.SeedRunner(orgA, mustUUID(ownerID)).String()
	fx.tokenID = env.SeedRegistrationToken(orgA, mustUUID(ownerID)).String()
	fx.jobID = env.SeedJob(orgA, mustUUID(ownerID)).String()
	return fx
}

func principals(fx fixture) []principal {
	out := []principal{{name: "anonymous", kind: "anonymous"}}
	for _, role := range policy.Roles() {
		out = append(out, principal{name: string(role) + " of the organization", kind: "member", role: role})
	}
	out = append(out,
		principal{name: "owner of another organization", kind: "outsider"},
		principal{name: "user without any membership", kind: "lone"},
	)
	return out
}

func (fx fixture) client(env *apptest.Env, p principal) *apptest.Client {
	switch p.kind {
	case "anonymous":
		return &apptest.Client{Env: env}
	case "member":
		return env.SeedSession(mustUUID(fx.byRole[p.role]))
	case "outsider":
		return env.SeedSession(mustUUID(fx.outsiderID))
	default:
		return env.SeedSession(mustUUID(fx.loneID))
	}
}

type outcome struct {
	status int
	code   string
}

var (
	allowed         = outcome{}
	unauthenticated = outcome{http.StatusUnauthorized, "UNAUTHENTICATED"}
	forbidden       = outcome{http.StatusForbidden, "FORBIDDEN"}
	notFound        = outcome{http.StatusNotFound, "NOT_FOUND"}
)

func expected(rule policy.Rule, p principal) outcome {
	if rule.Scope == policy.ScopePublic {
		return allowed
	}
	if p.kind == "anonymous" {
		return unauthenticated
	}
	if rule.Scope == policy.ScopeAuthenticated {
		return allowed
	}
	if rule.Scope == policy.ScopeInstance {
		if p.kind == "member" && p.role == policy.RoleOwner {
			return allowed
		}
		return forbidden
	}
	if p.kind != "member" {
		return notFound
	}
	if policy.Allows(p.role, rule.Action) {
		return allowed
	}
	return forbidden
}

func policyFailure(res apptest.Response) bool {
	if res.Error == nil {
		return false
	}
	switch res.Error.Code {
	case "UNAUTHENTICATED", "FORBIDDEN", "CSRF_FAILED", "NOT_FOUND", "SESSION_REQUIRED":
		return true
	default:
		return false
	}
}

func requestFor(rule policy.Rule, fx fixture, c *apptest.Client) apptest.Request {
	path := strings.NewReplacer(
		"{id}", fx.orgA, "{runnerId}", fx.runnerID, "{tokenId}", fx.tokenID, "{jobId}", fx.jobID, "{providerId}", "corp",
	).Replace(rule.Path)
	if strings.HasPrefix(rule.Path, "/organizations/{id}/") && !strings.Contains(rule.Path, "{runnerId}") && !strings.Contains(rule.Path, "{tokenId}") &&
		!strings.Contains(rule.Path, "{jobId}") {
		path = organizationPath(c.Env, fx, rule)
	}
	if strings.Contains(rule.Path, "{notificationId}") {
		path = strings.ReplaceAll(path, "{notificationId}", inboxItemFor(c))
	}
	if rule.Path == "/sessions/{id}" {
		path = "/sessions/00000000-0000-7000-8000-000000000001"
		if c.UserID != "" {
			path = "/sessions/" + sessionIDOf(c)
		}
	}
	if strings.HasPrefix(rule.Path, "/organizations/{id}/api-tokens/{tokenId}") {
		token := c.Env.SeedAPIToken(mustUUID(fx.byRole[policy.RoleOwner]), mustUUID(fx.orgA))
		path = strings.NewReplacer("{id}", fx.orgA, "{tokenId}", token.ID.String()).Replace(rule.Path)
	}
	if strings.HasPrefix(rule.Path, "/me/api-tokens/{id}") {
		path = strings.Replace(rule.Path, "{id}", "00000000-0000-7000-8000-000000000005", 1)
		if c.UserID != "" {
			path = strings.Replace(rule.Path, "{id}", c.Env.SeedAPIToken(mustUUID(c.UserID), mustUUID(fx.orgB)).ID.String(), 1)
		}
	}
	if strings.HasPrefix(rule.Path, "/me/passkeys/{id}") {
		path = strings.Replace(rule.Path, "{id}", "00000000-0000-7000-8000-000000000002", 1)
		if c.UserID != "" {
			path = strings.Replace(rule.Path, "{id}", passkeyIDOf(c), 1)
		}
	}
	req := apptest.Request{Method: rule.Method, Path: path, Body: mfaBodyFor(rule)}
	switch {
	case rule.Method == http.MethodPost && rule.Path == "/sessions":
		req.Body = map[string]string{"Email": "nobody@example.com", "Password": "whatever whatever"}
	case rule.Method == http.MethodPost && rule.Path == "/setup":
		req.Body = map[string]string{"SetupToken": "x", "OwnerName": "x", "Email": "x@example.com", "Password": "x", "OrganizationName": "x", "OrganizationSlug": "x"}
	case rule.Method == http.MethodPut && rule.Path == "/me/password":
		req.Body = map[string]string{"CurrentPassword": "not the current one", "NewPassword": "a brand new passphrase"}
	case rule.Method == http.MethodPost && rule.Path == "/organizations/{id}/runner-registration-tokens":
		req.Body = map[string]any{}
	case rule.Method == http.MethodPost && rule.Path == "/organizations/{id}/jobs":
		req.Body = map[string]any{"Name": "Route table job", "Spec": apptest.SimpleSpec()}
	case rule.Method == http.MethodPatch:
		req.Body = map[string]string{"Name": "Route table was here"}
	}
	if body := organizationBody(c.Env, fx, rule); body != nil {
		req.Body = body
	}
	return req
}

func TestRouteTable(t *testing.T) {
	env := apptest.New(t)
	fx := buildFixture(t, env)
	rules := app.Policies()
	sort.Slice(rules, func(i, j int) bool { return rules[i].Key() < rules[j].Key() })
	for _, rule := range rules {
		for _, p := range principals(fx) {
			t.Run(rule.Key()+"/"+p.name, func(t *testing.T) {
				c := fx.client(env, p)
				if rule.Path == "/organizations/{id}/leave" && p.kind == "member" {
					c = env.SeedSession(env.SeedUser(mustUUID(fx.orgA), randomEmail(t), "a long password for leaving", string(p.role)))
				}
				res := c.Do(requestFor(rule, fx, c))
				want := expected(rule, p)
				if want == allowed {
					assert.False(t, policyFailure(res), "must pass authorization, got %d %s", res.Status, res.Raw)
					assert.Less(t, res.Status, 500, string(res.Raw))
					return
				}
				assert.Equal(t, want.status, res.Status, string(res.Raw))
				require.NotNil(t, res.Error)
				assert.Equal(t, want.code, res.Error.Code)
			})
		}
	}
}

func TestRouteTableStateChangesAreGuardedAgainstCSRF(t *testing.T) {
	env := apptest.New(t)
	fx := buildFixture(t, env)
	for _, rule := range app.Policies() {
		if !rule.Unsafe() {
			continue
		}
		variants := map[string]map[string]string{
			"no token":       {config.CSRFHeaderName: ""},
			"foreign origin": {"Origin": "https://evil.example"},
			"no origin":      {"Origin": ""},
		}
		for name, headers := range variants {
			t.Run(rule.Key()+"/"+name, func(t *testing.T) {
				c := env.SeedSession(mustUUID(fx.byRole[policy.RoleOwner]))
				req := requestFor(rule, fx, c)
				req.Headers = headers
				res := c.Do(req)
				if rule.Scope == policy.ScopePublic && name == "no token" {
					assert.False(t, policyFailure(res), "public routes need no token: %d %s", res.Status, res.Raw)
					return
				}
				assert.Equal(t, http.StatusForbidden, res.Status, string(res.Raw))
				require.NotNil(t, res.Error)
				assert.Equal(t, "CSRF_FAILED", res.Error.Code)
			})
		}
	}
}

func TestRouteTableCoversEveryRoleAndScope(t *testing.T) {
	scopes := map[policy.Scope]int{}
	actions := map[policy.Action]bool{}
	for _, r := range app.Policies() {
		scopes[r.Scope]++
		if r.Action != "" {
			actions[r.Action] = true
		}
	}
	assert.Positive(t, scopes[policy.ScopePublic])
	assert.Positive(t, scopes[policy.ScopeAuthenticated])
	assert.Positive(t, scopes[policy.ScopeOrganization])
	assert.Positive(t, scopes[policy.ScopeInstance])
	assert.Positive(t, scopes[policy.ScopeProject])
	for _, a := range policy.Actions() {
		if slices.Contains(actionsAheadOfTheirRoutes(), a) {
			assert.False(t, actions[a], "%s has a route now: drop it from actionsAheadOfTheirRoutes", a)
			continue
		}
		assert.True(t, actions[a], "no route uses %s", a)
	}
	assert.Len(t, policy.Roles(), 5, fmt.Sprint(policy.Roles()))
}

func actionsAheadOfTheirRoutes() []policy.Action {
	return nil
}
