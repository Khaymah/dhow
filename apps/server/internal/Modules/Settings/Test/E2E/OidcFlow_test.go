package e2e_test

import (
	"crypto/rand"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	oidcmock "github.com/Khaymah/dhow/apps/server/internal/Core/Oidc/Mock"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

type testResult struct {
	Passed bool
	Checks []struct {
		Check  string
		Passed bool
	}
}

func testOidc(t *testing.T, c *apptest.Client, kind, issuer string) (testResult, apptest.Response) {
	t.Helper()
	res := c.Do(apptest.Request{Method: http.MethodPost, Path: oidcPath + "/test", Body: map[string]any{"Type": kind, "Issuer": issuer}})
	var out testResult
	if res.Status == http.StatusOK {
		res.Decode(t, &out)
	}
	return out, res
}

func passed(r testResult) map[string]bool {
	out := map[string]bool{}
	for _, c := range r.Checks {
		out[c.Check] = c.Passed
	}
	return out
}

func TestTestingAConnectionReachesTheIssuerWithoutTheSecret(t *testing.T) {
	o := newOidcEnv(t, apptest.Options{})
	plain := oidcmock.New(t, false, o.env.Clock.Now)
	plain.PlainEndpoint = "jwks_uri"
	tests := []struct {
		name   string
		kind   string
		issuer string
		passed bool
		checks map[string]bool
	}{
		{"a working issuer", "oidc", o.idp.Issuer(), true, map[string]bool{"discovery": true, "endpoints": true, "keys": true}},
		{"keys over plain http", "oidc", plain.Issuer(), false, map[string]bool{"discovery": true, "endpoints": false, "keys": false}},
		{"nothing listens", "oidc", "https://127.0.0.1:1", false, map[string]bool{"discovery": false, "endpoints": false, "keys": false}},
		{"github", "github", o.idp.Issuer(), true, map[string]bool{"api": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, res := testOidc(t, o.owner, tt.kind, tt.issuer)
			require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
			assert.Equal(t, tt.passed, got.Passed)
			assert.Equal(t, tt.checks, passed(got))
			assert.NotContains(t, string(res.Raw), oidcmock.ClientSecret)
		})
	}
	assert.Zero(t, o.idp.Count("token"), "a test never exchanges a code or sends the client secret")
	assert.Equal(t, 1, o.idp.Count("github_meta"))
	assert.Positive(t, o.idp.Count("jwks"))
	assert.Contains(t, o.env.Logs.String(), "the sign-in provider test failed")

	o.env.Clock.Advance(11 * time.Minute)
	for _, issuer := range []string{"https://169.254.169.254", "https://[fe80::1]/realm", "https://metadata.google.internal", "http://sso.example.com", "ftp://x"} {
		_, res := testOidc(t, o.owner, "oidc", issuer)
		assert.Equal(t, http.StatusBadRequest, res.Status, issuer)
		assert.Equal(t, []string{"Issuer"}, detailFields(res), issuer)
	}
	_, limited := testOidc(t, o.owner, "oidc", o.idp.Issuer())
	assert.Equal(t, http.StatusTooManyRequests, limited.Status, "five tests in ten minutes")

	member := o.env.SeedSession(o.seedMember(t, "dev@example.com"))
	_, refused := testOidc(t, member, "oidc", o.idp.Issuer())
	assert.Equal(t, http.StatusForbidden, refused.Status)
}

func TestAProviderAddedInTheDashboardSignsPeopleIn(t *testing.T) {
	o := newOidcEnv(t, apptest.Options{})
	o.addCorp(t, map[string]any{"AllowedDomains": []string{"corp.example"}})
	_, missing := o.env.Login("dev@example.com", "a long password for dev")
	require.Equal(t, http.StatusUnauthorized, missing.Status, "the member does not exist yet")
	o.seedMember(t, "dev@example.com")
	member, login := o.env.Login("dev@example.com", "a long password for dev")
	require.Equal(t, http.StatusOK, login.Status, string(login.Raw))

	claims := oidcmock.Claims{Subject: "dev-1", Email: "dev@corp.example", EmailVerified: true, Name: "Dev"}
	linked := member.LinkOidc(o.idp, apptest.PasswordFactor("a long password for dev"), claims)
	require.Equal(t, http.StatusOK, linked.Status, string(linked.Raw))
	assert.Equal(t, int64(1), readOidc(t, o.owner).find(t, "corp").LinkedIdentities)

	_, res := o.env.OidcLogin(o.idp, claims)
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	assert.Equal(t, "session.login.succeeded", res.Message)

	outside := oidcmock.Claims{Subject: "dev-1", Email: "dev@elsewhere.example", EmailVerified: true}
	_, refused := o.env.OidcLogin(o.idp, outside)
	assert.Equal(t, http.StatusForbidden, refused.Status, "the dashboard domains apply")
}

func TestAFlowInProgressFailsSafelyWhenItsProviderChanges(t *testing.T) {
	tests := []struct {
		name   string
		change func(o oidcEnv, version int64) apptest.Response
		status int
		code   string
	}{
		{"removed", func(o oidcEnv, v int64) apptest.Response {
			return removeOidc(o.owner, "corp", v, o.fresh())
		}, http.StatusBadRequest, "OIDC_FLOW_INVALID"},
		{"turned off", func(o oidcEnv, v int64) apptest.Response {
			return updateOidc(o.owner, "corp", v, o.fresh(), map[string]any{"ClientSecret": ""})
		}, http.StatusBadRequest, "OIDC_FLOW_INVALID"},
		{"another secret", func(o oidcEnv, v int64) apptest.Response {
			return updateOidc(o.owner, "corp", v, o.fresh(), map[string]any{"ClientSecret": "Pq7wE3rT9yU1iO5pA2sD8fG4hJ6kL0zX"})
		}, http.StatusUnauthorized, "OIDC_SIGN_IN_FAILED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := newOidcEnv(t, apptest.Options{})
			p := o.addCorp(t, nil)
			anonymous := &apptest.Client{Env: o.env}
			start := anonymous.BeginOidc("/oidc/authorize", map[string]string{"Provider": "corp"})
			require.Equal(t, http.StatusOK, start.Res.Status, string(start.Res.Raw))
			code, state := o.idp.Authorize(start.URL, oidcmock.Claims{Subject: "x", Email: "x@corp.example", EmailVerified: true})

			require.Equal(t, http.StatusOK, tt.change(o, p.Version).Status)
			res := anonymous.FinishOidc("/oidc/callback", start.Cookie, code, state, nil)
			assert.Equal(t, tt.status, res.Status, string(res.Raw))
			assert.Equal(t, tt.code, errorCode(res))
		})
	}
}

func TestRemovalNeverLocksAnAccountOut(t *testing.T) {
	o := newOidcEnv(t, apptest.Options{})
	p := o.addCorp(t, nil)
	lone := o.seedMember(t, "lone@example.com")
	o.linkIdentity(t, lone, "lone-1")
	o.dropPassword(t, lone)
	keeper := o.seedMember(t, "keeper@example.com")
	o.linkIdentity(t, keeper, "keeper-1")

	view := readOidc(t, o.owner).find(t, "corp")
	assert.Equal(t, int64(2), view.LinkedIdentities)
	assert.Equal(t, int64(1), view.OnlySignInUsers)

	for name, res := range map[string]apptest.Response{
		"remove":   removeOidc(o.owner, "corp", p.Version, o.fresh()),
		"turn off": updateOidc(o.owner, "corp", p.Version, o.fresh(), map[string]any{"ClientSecret": ""}),
	} {
		assert.Equal(t, http.StatusConflict, res.Status, name)
		assert.Equal(t, "OIDC_PROVIDER_ONLY_SIGN_IN", errorCode(res), name)
		require.NotNil(t, res.Error)
		require.Len(t, res.Error.Details, 1)
		assert.Equal(t, "Users", res.Error.Details[0].Field)
		assert.Equal(t, "1", res.Error.Details[0].Message)
	}
	assert.Equal(t, []string{"corp"}, publicProviders(t, o.env))

	other := createOidc(o.owner, o.fresh(), corp(o.idp, map[string]any{"Id": "backup", "Issuer": "https://backup.example.com"}))
	require.Equal(t, http.StatusOK, other.Status, string(other.Raw))
	_, err := o.env.Pool.Exec(t.Context(),
		`INSERT INTO "UserIdentities" ("UserId", "Provider", "Issuer", "Subject") VALUES ($1, 'backup', 'https://backup.example.com', 'lone-2')`, lone)
	require.NoError(t, err)
	assert.Zero(t, readOidc(t, o.owner).find(t, "corp").OnlySignInUsers, "another enabled provider is a way in")

	removed := decodeOidc(t, removeOidc(o.owner, "corp", p.Version, o.fresh()))
	assert.Len(t, removed.Providers, 1)

	keeperClient, login := o.env.Login("keeper@example.com", "a long password for keeper")
	require.Equal(t, http.StatusOK, login.Status)
	list := keeperClient.Get("/me/identities")
	var identities []struct{ Id, Provider, ProviderName string }
	list.Decode(t, &identities)
	require.Len(t, identities, 1, "a link to a removed provider stays visible")
	assert.Equal(t, "corp", identities[0].ProviderName)
	unlink := keeperClient.Do(apptest.Request{
		Method: http.MethodPost, Path: "/me/identities/" + identities[0].Id + "/remove",
		Body: map[string]any{"CurrentFactor": apptest.PasswordFactor("a long password for keeper")},
	})
	assert.Equal(t, http.StatusOK, unlink.Status, string(unlink.Raw))
}

func TestALinkToARemovedProviderIsNoLongerASignInMethod(t *testing.T) {
	o := newOidcEnv(t, apptest.Options{})
	p := o.addCorp(t, nil)
	user := o.seedMember(t, "keys@example.com")
	client, login := o.env.Login("keys@example.com", "a long password for keys")
	require.Equal(t, http.StatusOK, login.Status)
	enrollment := client.EnableTotp(apptest.PasswordFactor("a long password for keys"))
	o.linkIdentity(t, user, "keys-1")
	o.env.SeedPasskey(user)
	o.dropPassword(t, user)

	decodeOidc(t, removeOidc(o.owner, "corp", p.Version, o.fresh()))
	var passkey string
	require.NoError(t, o.env.Pool.QueryRow(t.Context(), `SELECT "Id"::text FROM "Passkeys" WHERE "UserId" = $1`, user).Scan(&passkey))
	o.env.Clock.Advance(totpStep)
	res := client.Do(apptest.Request{
		Method: http.MethodPost, Path: "/me/passkeys/" + passkey + "/remove",
		Body: map[string]any{"CurrentFactor": apptest.CodeFactor("totp", o.env.TotpCode(enrollment.Secret, 0))},
	})
	assert.Equal(t, http.StatusConflict, res.Status, string(res.Raw))
	assert.Equal(t, "LAST_SIGN_IN_METHOD", errorCode(res), "the passkey is the only way in left")
}

func TestASaveReachesTheSignInPageOfEveryInstance(t *testing.T) {
	o := newOidcEnv(t, apptest.Options{})
	second := apptest.NewWithOptions(t, apptest.Options{
		Pool: o.env.Pool, Clock: o.env.Clock, OidcTransport: o.idp.Transport(),
		Profile: func(c *config.Config) { c.SecretKey = o.env.Config.SecretKey },
	})
	p := o.addCorp(t, nil)
	require.Eventually(t, func() bool { return len(publicProviders(t, second)) == 1 }, 5*time.Second, 20*time.Millisecond)

	anonymous := &apptest.Client{Env: second}
	begin := func() {
		t.Helper()
		start := anonymous.BeginOidc("/oidc/authorize", map[string]string{"Provider": "corp"})
		require.Equal(t, http.StatusOK, start.Res.Status, string(start.Res.Raw))
	}
	begin()
	begin()
	assert.Equal(t, 1, o.idp.Count("discovery"), "the discovery document is cached")

	renamed := decodeOidc(t, updateOidc(o.owner, "corp", p.Version, o.fresh(), map[string]any{"Name": "Corp login"})).find(t, "corp")
	require.Eventually(t, func() bool {
		res := second.Do(apptest.Request{Method: http.MethodGet, Path: "/oidc/providers"})
		var list []struct{ Id, Name string }
		res.Decode(t, &list)
		return len(list) == 1 && list[0].Name == "Corp login"
	}, 5*time.Second, 20*time.Millisecond)
	begin()
	assert.Equal(t, 2, o.idp.Count("discovery"), "a changed provider drops its discovery cache")

	decodeOidc(t, removeOidc(o.owner, "corp", renamed.Version, o.fresh()))
	require.Eventually(t, func() bool { return len(publicProviders(t, second)) == 0 }, 5*time.Second, 20*time.Millisecond)
	gone := anonymous.Do(apptest.Request{Method: http.MethodPost, Path: "/oidc/authorize", Body: map[string]string{"Provider": "corp"}})
	assert.Equal(t, http.StatusNotFound, gone.Status)
}

func TestAStoredProviderThatIsNotValidStopsStartUp(t *testing.T) {
	tests := []struct {
		name   string
		issuer string
		secret []byte
		want   string
	}{
		{"plain http outside development", "http://sso.example.com", nil, "sign-in provider corp (from the database): Issuer must use https"},
		{"a secret sealed with another key", "https://sso.example.com", append([]byte{3}, make([]byte, 47)...), "cannot be decrypted with the keyring"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := dbtest.NewPool(t)
			_, err := pool.Exec(t.Context(), `INSERT INTO "OidcProviders" ("ProviderId", "Type", "Name", "Issuer", "ClientId", "Secret")
				VALUES ('corp', 'oidc', 'Corp', $1, 'dhow', $2)`, tt.issuer, tt.secret)
			require.NoError(t, err)
			key := make([]byte, 32)
			_, err = rand.Read(key)
			require.NoError(t, err)
			err = build(t, key, nil, pool)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
