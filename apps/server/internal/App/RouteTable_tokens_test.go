package app_test

import (
	"net/http"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
)

var sessionRequired = outcome{http.StatusForbidden, "SESSION_REQUIRED"}

func allScopes() []string {
	out := make([]string, 0, len(policy.TokenActions()))
	for _, a := range policy.TokenActions() {
		out = append(out, string(a))
	}
	return out
}

func expectedWithToken(rule policy.Rule, p principal, scopes []string) outcome {
	if !rule.Tokens {
		return sessionRequired
	}
	if p.kind != "member" {
		return notFound
	}
	granted := false
	for _, s := range scopes {
		if policy.Action(s) == rule.Action {
			granted = true
		}
	}
	if granted && policy.Allows(p.role, rule.Action) {
		return allowed
	}
	return forbidden
}

func tokenPrincipals() []principal {
	out := make([]principal, 0, len(policy.Roles())+1)
	for _, role := range policy.Roles() {
		out = append(out, principal{name: string(role) + " of the organization", kind: "member", role: role})
	}
	return append(out, principal{name: "owner of another organization", kind: "outsider"})
}

func (fx fixture) userOf(p principal) string {
	if p.kind == "outsider" {
		return fx.outsiderID
	}
	return fx.byRole[p.role]
}

func TestRouteTableWithAPITokens(t *testing.T) {
	env := apptest.New(t)
	fx := buildFixture(t, env)
	rules := app.Policies()
	sort.Slice(rules, func(i, j int) bool { return rules[i].Key() < rules[j].Key() })
	variants := []struct {
		name   string
		scopes []string
	}{
		{"every scope", allScopes()},
		{"org.read only", []string{string(policy.OrgRead)}},
	}
	for _, v := range variants {
		for _, p := range tokenPrincipals() {
			token := env.SeedAPIToken(mustUUID(fx.userOf(p)), mustUUID(fx.orgA), v.scopes...)
			for _, rule := range rules {
				t.Run(v.name+"/"+rule.Key()+"/"+p.name, func(t *testing.T) {
					anonymous := &apptest.Client{Env: env}
					req := requestFor(rule, fx, anonymous)
					req.Headers = map[string]string{"Origin": "", "Authorization": "Bearer " + token.Value}
					res := env.Do(req)
					want := expectedWithToken(rule, p, v.scopes)
					if want == allowed {
						assert.False(t, policyFailure(res), "must pass authorization, got %d %s", res.Status, res.Raw)
						assert.Less(t, res.Status, 500, string(res.Raw))
						return
					}
					assert.Equal(t, want.status, res.Status, string(res.Raw))
					require.NotNil(t, res.Error, string(res.Raw))
					assert.Equal(t, want.code, res.Error.Code)
				})
			}
		}
	}
}

func TestAPITokensNeverMixWithBrowserCredentials(t *testing.T) {
	env := apptest.New(t)
	fx := buildFixture(t, env)
	owner := mustUUID(fx.byRole[policy.RoleOwner])
	token := env.SeedAPIToken(owner, mustUUID(fx.orgA), allScopes()...)
	session := env.SeedSession(owner)
	members := "/organizations/" + fx.orgA + "/members"
	tests := []struct {
		name    string
		headers map[string]string
		want    outcome
	}{
		{"token alone", map[string]string{"Authorization": "Bearer " + token.Value}, allowed},
		{"token in lower-case scheme", map[string]string{"Authorization": "bearer " + token.Value}, allowed},
		{"token next to a session cookie", map[string]string{
			"Authorization": "Bearer " + token.Value, "Cookie": config.SessionCookieName + "=" + session.Cookie,
		}, unauthenticated},
		{"token next to a flow cookie", map[string]string{
			"Authorization": "Bearer " + token.Value, "Cookie": config.OidcCookieName + "=x",
		}, unauthenticated},
		{"basic authorization is not a token", map[string]string{"Authorization": "Basic b3duZXI6cGFzc3dvcmQ="}, unauthenticated},
		{"basic authorization next to a session is ignored", map[string]string{
			"Authorization": "Basic b3duZXI6cGFzc3dvcmQ=", "Cookie": config.SessionCookieName + "=" + session.Cookie,
		}, allowed},
		{"runner token prefix", map[string]string{"Authorization": "Bearer dhow_rat_" + token.Value[len(config.APITokenPrefix):]}, unauthenticated},
		{"truncated token", map[string]string{"Authorization": "Bearer " + token.Value[:len(token.Value)-1]}, unauthenticated},
		{"well-formed unknown token", map[string]string{"Authorization": "Bearer " + config.APITokenPrefix + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}, unauthenticated},
		{"empty bearer", map[string]string{"Authorization": "Bearer "}, unauthenticated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := env.Do(apptest.Request{Method: http.MethodGet, Path: members, Headers: tt.headers})
			if tt.want == allowed {
				assert.Equal(t, http.StatusOK, res.Status, string(res.Raw))
				return
			}
			assert.Equal(t, tt.want.status, res.Status, string(res.Raw))
			require.NotNil(t, res.Error)
			assert.Equal(t, tt.want.code, res.Error.Code)
		})
	}
	write := env.Do(apptest.Request{
		Method: http.MethodPatch, Path: "/organizations/" + fx.orgA, Body: map[string]string{"Name": "Renamed by CI"},
		Headers: map[string]string{"Authorization": "Bearer " + token.Value, "Origin": "https://evil.example"},
	})
	assert.Equal(t, http.StatusOK, write.Status, "a token needs neither Origin nor CSRF, and a foreign Origin does not matter: %s", write.Raw)
}

func TestTheRunnerPlaneRefusesAPITokens(t *testing.T) {
	env := apptest.New(t)
	fx := buildFixture(t, env)
	token := env.SeedAPIToken(mustUUID(fx.byRole[policy.RoleOwner]), mustUUID(fx.orgA), allScopes()...)
	for _, rule := range app.RunnerPolicies() {
		if rule.Runner != policy.RunnerSession {
			continue
		}
		t.Run(rule.Key(), func(t *testing.T) {
			res := callRule(t, env.Handler, rule, map[string]string{"Authorization": "Bearer " + token.Value})
			assert.Equal(t, "UNAUTHENTICATED", res.message, "status %d code %s", res.status, res.code)
		})
	}
}
