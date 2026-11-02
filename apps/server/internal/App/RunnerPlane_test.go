package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

type rpcResult struct {
	status  int
	code    string
	message string
}

func callProcedure(t *testing.T, h http.Handler, procedure string, headers map[string]string) rpcResult {
	t.Helper()
	return callRunnerPlane(t, h, http.MethodPost, procedure, headers)
}

func callRule(t *testing.T, h http.Handler, rule policy.Rule, headers map[string]string) rpcResult {
	t.Helper()
	return callRunnerPlane(t, h, rule.Method, strings.ReplaceAll(rule.Path, "{digest}", strings.Repeat("a", 64)), headers)
}

func callRunnerPlane(t *testing.T, h http.Handler, method, path string, headers map[string]string) rpcResult {
	t.Helper()
	req := httptest.NewRequest(method, config.RunnerAPIPrefix+path, bytes.NewReader(nil))
	req.RemoteAddr = "198.51.100.20:6000"
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set(config.RunnerProtocolHeader, config.RunnerProtocolVersion)
	for k, v := range headers {
		if v == "" {
			req.Header.Del(k)
			continue
		}
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out := rpcResult{status: rec.Code}
	if rec.Code != http.StatusOK && strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		var body struct{ Code, Message string }
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
		out.code, out.message = body.Code, body.Message
		if out.message == "" {
			out.message = body.Code
		}
	}
	return out
}

func runnerProcedures(t *testing.T) []string {
	t.Helper()
	var out []string
	protoregistry.GlobalFiles.RangeFilesByPackage("dhow.runner.v1", func(fd protoreflect.FileDescriptor) bool {
		for i := range fd.Services().Len() {
			svc := fd.Services().Get(i)
			for j := range svc.Methods().Len() {
				out = append(out, "/"+string(svc.FullName())+"/"+string(svc.Methods().Get(j).Name()))
			}
		}
		return true
	})
	require.NotEmpty(t, out)
	sort.Strings(out)
	return out
}

func TestEveryMountedRunnerProcedureDeclaresAPolicy(t *testing.T) {
	env := apptest.New(t)
	declared := map[string]bool{}
	for _, r := range app.RunnerPolicies() {
		assert.True(t, r.Valid(), "invalid rule %s", r.Key())
		assert.Equal(t, policy.ScopeRunner, r.Scope, r.Key())
		assert.False(t, declared[r.Key()], "duplicate rule %s", r.Key())
		declared[r.Key()] = true
	}
	mounted := map[string]bool{}
	for _, p := range runnerProcedures(t) {
		res := callProcedure(t, env.Handler, p, nil)
		if res.status != http.StatusNotFound {
			mounted[http.MethodPost+" "+p] = true
			assert.True(t, declared[http.MethodPost+" "+p], "%s is mounted without a declared policy", p)
		}
	}
	for _, r := range app.RunnerPolicies() {
		if r.Method != http.MethodPost && callRule(t, env.Handler, r, nil).status != http.StatusNotFound {
			mounted[r.Key()] = true
		}
	}
	for p := range declared {
		assert.True(t, mounted[p], "%s has a policy but is not mounted", p)
	}
	assert.Len(t, mounted, 17, "RegistrationService, AuthService, LeaseService, LogService, ArtifactService, CacheService and the blob routes are mounted")
}

func TestTheBlobRoutesRefuseOtherMethods(t *testing.T) {
	env := apptest.New(t)
	for _, method := range []string{http.MethodPost, http.MethodDelete, http.MethodPatch} {
		res := callRunnerPlane(t, env.Handler, method, "/blobs/"+strings.Repeat("a", 64), nil)
		assert.Contains(t, []int{http.StatusMethodNotAllowed, http.StatusNotFound}, res.status, method)
	}
}

type runnerCaller struct {
	name    string
	headers func(*runnerFixture) map[string]string
}

type runnerFixture struct {
	valid, stale, revoked, cookie string
}

func buildRunnerFixture(t *testing.T, env *apptest.Env) runnerFixture {
	t.Helper()
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	token, _ := env.CreateRegistrationToken(owner, org, apptest.RegistrationTokenSpec{MaxUses: 3})
	rc := env.RunnerClient(apptest.RunnerClientOptions{})
	var fx runnerFixture
	id, reg := env.RegisterRunner(token)
	stale, err := env.Authenticate(rc, id, reg.GetRunnerId())
	require.NoError(t, err)
	fx.stale = stale
	fx.valid, err = env.Authenticate(rc, id, reg.GetRunnerId())
	require.NoError(t, err)
	revokedID, revokedReg := env.RegisterRunner(token)
	fx.revoked, err = env.Authenticate(rc, revokedID, revokedReg.GetRunnerId())
	require.NoError(t, err)
	res := owner.Do(apptest.Request{Method: http.MethodPost, Path: "/organizations/" + org.String() + "/runners/" + revokedReg.GetRunnerId() + "/revoke"})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	fx.cookie = config.SessionCookieName + "=" + owner.Cookie
	return fx
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

var runnerCallers = []runnerCaller{
	{"no credentials", func(*runnerFixture) map[string]string { return nil }},
	{"browser session cookie", func(fx *runnerFixture) map[string]string { return map[string]string{"Cookie": fx.cookie} }},
	{"session cookie next to a valid runner token", func(fx *runnerFixture) map[string]string {
		return map[string]string{"Cookie": fx.cookie, "Authorization": "Bearer " + fx.valid}
	}},
	{"valid runner token", func(fx *runnerFixture) map[string]string { return bearer(fx.valid) }},
	{"replaced runner token", func(fx *runnerFixture) map[string]string { return bearer(fx.stale) }},
	{"token of a revoked runner", func(fx *runnerFixture) map[string]string { return bearer(fx.revoked) }},
	{"malformed bearer", func(*runnerFixture) map[string]string { return bearer("dhow_rat_short") }},
	{"no protocol header", func(fx *runnerFixture) map[string]string {
		return map[string]string{"Authorization": "Bearer " + fx.valid, config.RunnerProtocolHeader: ""}
	}},
	{"unsupported protocol", func(fx *runnerFixture) map[string]string {
		return map[string]string{"Authorization": "Bearer " + fx.valid, config.RunnerProtocolHeader: "2"}
	}},
}

var gateMessages = map[string]bool{
	"UNAUTHENTICATED": true, "PERMISSION_DENIED": true, "RUNNER_REVOKED": true, "RUNNER_UPGRADE_REQUIRED": true,
}

func expectedGate(rule policy.Rule, caller string) string {
	switch caller {
	case "no protocol header", "unsupported protocol":
		return "RUNNER_UPGRADE_REQUIRED"
	case "browser session cookie", "session cookie next to a valid runner token":
		return "UNAUTHENTICATED"
	}
	if rule.Runner != policy.RunnerSession {
		return ""
	}
	switch caller {
	case "valid runner token":
		return ""
	case "token of a revoked runner":
		return "RUNNER_REVOKED"
	default:
		return "UNAUTHENTICATED"
	}
}

func TestRunnerProcedureTable(t *testing.T) {
	env := apptest.New(t)
	fx := buildRunnerFixture(t, env)
	for _, rule := range app.RunnerPolicies() {
		for _, caller := range runnerCallers {
			t.Run(rule.Key()+"/"+caller.name, func(t *testing.T) {
				res := callRule(t, env.Handler, rule, caller.headers(&fx))
				want := expectedGate(rule, caller.name)
				if want == "" {
					assert.False(t, gateMessages[res.message], "must pass the gate, got %d %s %s", res.status, res.code, res.message)
					assert.Less(t, res.status, 500, res.message)
					return
				}
				assert.Equal(t, want, res.message, "status %d code %s", res.status, res.code)
			})
		}
	}
}

func TestRESTRoutesRefuseRunnerTokens(t *testing.T) {
	env := apptest.New(t)
	fx := buildFixture(t, env)
	rf := buildRunnerFixtureFor(t, env, fx)
	for _, rule := range app.Policies() {
		if rule.Scope == policy.ScopePublic {
			continue
		}
		t.Run(rule.Key(), func(t *testing.T) {
			anon := &apptest.Client{Env: env}
			req := requestFor(rule, fx, anon)
			req.Headers = map[string]string{"Authorization": "Bearer " + rf}
			res := anon.Do(req)
			assert.Equal(t, http.StatusUnauthorized, res.Status, string(res.Raw))
			require.NotNil(t, res.Error)
			assert.Equal(t, "UNAUTHENTICATED", res.Error.Code)
		})
	}
}

func buildRunnerFixtureFor(t *testing.T, env *apptest.Env, fx fixture) string {
	t.Helper()
	owner := env.SeedSession(mustUUID(fx.byRole[policy.RoleOwner]))
	token, _ := env.CreateRegistrationToken(owner, mustUUID(fx.orgA), apptest.RegistrationTokenSpec{})
	id, reg := env.RegisterRunner(token)
	access, err := env.Authenticate(env.RunnerClient(apptest.RunnerClientOptions{}), id, reg.GetRunnerId())
	require.NoError(t, err)
	return access
}

func TestSeparateRunnerListenerTakesTheRunnerPlaneOffThePublicHandler(t *testing.T) {
	env := apptest.NewConfigured(t, dbtest.NewPool(t), apptest.TestParams(), func(c *config.Config) {
		c.RunnerListenAddr = "127.0.0.1:7443"
	})
	procedure := "/" + string(runnerv1.File_dhow_runner_v1_auth_proto.Services().Get(0).FullName()) + "/Challenge"
	res := callProcedure(t, env.Handler, procedure, nil)
	assert.Equal(t, http.StatusNotFound, res.status)
	require.NotNil(t, env.Wired.RunnerHandler)
	res = callProcedure(t, env.Wired.RunnerHandler, procedure, nil)
	assert.Equal(t, http.StatusBadRequest, res.status)
	assert.Equal(t, "VALIDATION_FAILED: Runner", res.message)

	api := httptest.NewRecorder()
	env.Wired.RunnerHandler.ServeHTTP(api, httptest.NewRequest(http.MethodGet, config.APIPrefix+"/meta", http.NoBody))
	assert.Equal(t, http.StatusNotFound, api.Code, "the runner listener serves the runner plane only")
}

func TestTheRunnerPlaneIsMountedOnThePublicHandlerByDefault(t *testing.T) {
	env := apptest.New(t)
	assert.Nil(t, env.Wired.RunnerHandler)
	res := callProcedure(t, env.Handler, "/dhow.runner.v1.AuthService/Challenge", nil)
	assert.Equal(t, "VALIDATION_FAILED: Runner", res.message)
	res = callProcedure(t, env.Handler, "/dhow.runner.v1.LeaseService/Lease", nil)
	assert.Equal(t, "UNAUTHENTICATED", res.message, "the lease service is mounted behind the gate")
	res = callProcedure(t, env.Handler, "/dhow.runner.v1.LogService/Append", nil)
	assert.Equal(t, "UNAUTHENTICATED", res.message, "the log service is mounted behind the gate")
}

type protocolTransport struct {
	next  http.RoundTripper
	proto *int
}

func (p protocolTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set(config.RunnerProtocolHeader, config.RunnerProtocolVersion)
	res, err := p.next.RoundTrip(r)
	if err == nil {
		*p.proto = res.ProtoMajor
	}
	return res, err
}

func TestTheSeparateRunnerListenerSpeaksConnectOverTLSAndHTTP2(t *testing.T) {
	env := apptest.NewConfigured(t, dbtest.NewPool(t), apptest.TestParams(), func(c *config.Config) {
		c.RunnerListenAddr = "127.0.0.1:7443"
	})
	srv := httptest.NewUnstartedServer(env.Wired.RunnerHandler)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	hc := srv.Client()
	var major int
	hc.Transport = protocolTransport{next: hc.Transport, proto: &major}
	client := runnerv1connect.NewAuthServiceClient(hc, srv.URL+config.RunnerAPIPrefix)
	res, err := client.Challenge(t.Context(), connect.NewRequest(&runnerv1.ChallengeRequest{
		Runner: &runnerv1.RunnerRef{RunnerId: "00000000-0000-7000-8000-000000000001"},
	}))
	require.NoError(t, err)
	assert.NotEmpty(t, res.Msg.GetChallenge().GetMac())
	assert.Equal(t, "no-store", res.Header().Get("Cache-Control"))
	assert.Equal(t, 2, major, "the TLS listener negotiates HTTP/2")
}
