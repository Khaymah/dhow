package e2e_test

import (
	"crypto/rand"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	oidcmock "github.com/Khaymah/dhow/apps/server/internal/Core/Oidc/Mock"
)

func TestChangingTheIssuerOrTypeAsksForTheSecretAgain(t *testing.T) {
	o := newOidcEnv(t, apptest.Options{})
	o.addCorp(t, nil)
	tests := []struct {
		name   string
		fields map[string]any
		status int
	}{
		{"a new issuer without the secret", map[string]any{"Issuer": "https://elsewhere.example.com"}, http.StatusBadRequest},
		{"a new type without the secret", map[string]any{"Type": "github"}, http.StatusBadRequest},
		{"a new name keeps the secret", map[string]any{"Name": "Corp login"}, http.StatusOK},
		{"a new issuer with the secret", map[string]any{"Issuer": "https://elsewhere.example.com", "ClientSecret": "Rf4tG7yH2jK9lZ3xC6vB8nM1qW5eA0sD"}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := readOidc(t, o.owner).find(t, "corp")
			res := updateOidc(o.owner, "corp", current.Version, o.fresh(), tt.fields)
			require.Equal(t, tt.status, res.Status, string(res.Raw))
			if tt.status == http.StatusBadRequest {
				assert.Equal(t, []string{"ClientSecret"}, detailFields(res))
				assert.Equal(t, o.idp.Issuer(), readOidc(t, o.owner).find(t, "corp").Issuer, "nothing was saved")
			}
		})
	}
	assert.Equal(t, "Rf4tG7yH2jK9lZ3xC6vB8nM1qW5eA0sD", o.env.Wired.Settings.Current().Oidc.Active()[0].ClientSecret)
}

func TestNarrowingTheDomainsNeverLocksAnAccountOut(t *testing.T) {
	o := newOidcEnv(t, apptest.Options{})
	p := o.addCorp(t, map[string]any{"AllowedDomains": []string{"corp.example"}})
	lone := o.seedMember(t, "lone@example.com")
	o.linkIdentity(t, lone, "lone")
	o.dropPassword(t, lone)
	keeper := o.seedMember(t, "keeper@example.com")
	o.linkIdentity(t, keeper, "keeper")
	tests := []struct {
		name    string
		domains []string
		status  int
	}{
		{"a list without the account's domain", []string{"other.example"}, http.StatusConflict},
		{"the domain in another case among others", []string{"other.example", "CORP.example"}, http.StatusOK},
		{"no list at all", []string{}, http.StatusOK},
		{"the domain alone again", []string{"corp.example"}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := readOidc(t, o.owner).find(t, "corp")
			res := updateOidc(o.owner, "corp", current.Version, o.fresh(), map[string]any{"AllowedDomains": tt.domains})
			require.Equal(t, tt.status, res.Status, string(res.Raw))
			if tt.status == http.StatusConflict {
				assert.Equal(t, "OIDC_PROVIDER_ONLY_SIGN_IN", errorCode(res))
				require.Len(t, res.Error.Details, 1)
				assert.Equal(t, "1", res.Error.Details[0].Message, "the account with a password does not count")
			}
		})
	}
	assert.Greater(t, readOidc(t, o.owner).find(t, "corp").Version, p.Version)
}

func TestTheDatabaseDecidesWhichLinksStillSignIn(t *testing.T) {
	o := newOidcEnv(t, apptest.Options{})
	o.addCorp(t, nil)
	user := o.seedMember(t, "keys@example.com")
	client, login := o.env.Login("keys@example.com", "a long password for keys")
	require.Equal(t, http.StatusOK, login.Status)
	enrollment := client.EnableTotp(apptest.PasswordFactor("a long password for keys"))
	o.linkIdentity(t, user, "keys-1")
	o.env.SeedPasskey(user)
	o.dropPassword(t, user)
	_, err := o.env.Pool.Exec(t.Context(), `UPDATE "OidcProviders" SET "IsDeleted" = true WHERE "ProviderId" = 'corp'`)
	require.NoError(t, err)
	require.Equal(t, []string{"corp"}, publicProviders(t, o.env), "this instance has not reloaded yet")

	var passkey string
	require.NoError(t, o.env.Pool.QueryRow(t.Context(), `SELECT "Id"::text FROM "Passkeys" WHERE "UserId" = $1`, user).Scan(&passkey))
	o.env.Clock.Advance(totpStep)
	res := client.Do(apptest.Request{
		Method: http.MethodPost, Path: "/me/passkeys/" + passkey + "/remove",
		Body: map[string]any{"CurrentFactor": apptest.CodeFactor("totp", o.env.TotpCode(enrollment.Secret, 0))},
	})
	assert.Equal(t, http.StatusConflict, res.Status, string(res.Raw))
	assert.Equal(t, "LAST_SIGN_IN_METHOD", errorCode(res))
}

func TestAFlowEndsWhenItsProviderMovesToAnotherIssuer(t *testing.T) {
	o := newOidcEnv(t, apptest.Options{})
	p := o.addCorp(t, nil)
	other := oidcmock.New(t, false, o.env.Clock.Now)
	anonymous := &apptest.Client{Env: o.env}
	start := anonymous.BeginOidc("/oidc/authorize", map[string]string{"Provider": "corp"})
	require.Equal(t, http.StatusOK, start.Res.Status, string(start.Res.Raw))
	code, state := o.idp.Authorize(start.URL, oidcmock.Claims{Subject: "x", Email: "x@corp.example", EmailVerified: true})

	moved := updateOidc(o.owner, "corp", p.Version, o.fresh(), map[string]any{"Issuer": other.Issuer(), "ClientSecret": oidcmock.ClientSecret})
	require.Equal(t, http.StatusOK, moved.Status, string(moved.Raw))
	res := anonymous.FinishOidc("/oidc/callback", start.Cookie, code, state, nil)
	assert.Equal(t, http.StatusBadRequest, res.Status, string(res.Raw))
	assert.Equal(t, "OIDC_FLOW_INVALID", errorCode(res))
	assert.Zero(t, other.Count("token"), "the code and the verifier never reach the new issuer")
	assert.Zero(t, o.idp.Count("token"))
}

func TestALinkChecksTheIssuerSavedInTheDatabase(t *testing.T) {
	o := newOidcEnv(t, apptest.Options{})
	o.addCorp(t, nil)
	o.seedMember(t, "link@example.com")
	member, login := o.env.Login("link@example.com", "a long password for link")
	require.Equal(t, http.StatusOK, login.Status)
	start := member.BeginOidc("/me/identities/authorize", map[string]any{
		"Provider": "corp", "CurrentFactor": apptest.PasswordFactor("a long password for link"),
	})
	require.Equal(t, http.StatusOK, start.Res.Status, string(start.Res.Raw))
	code, state := o.idp.Authorize(start.URL, oidcmock.Claims{Subject: "link-1", Email: "link@corp.example", EmailVerified: true})
	_, err := o.env.Pool.Exec(t.Context(), `UPDATE "OidcProviders" SET "Issuer" = 'https://moved.example.com' WHERE "ProviderId" = 'corp'`)
	require.NoError(t, err)

	res := member.FinishOidc("/me/identities/callback", start.Cookie, code, state, nil)
	assert.Equal(t, http.StatusBadRequest, res.Status, string(res.Raw))
	assert.Equal(t, "OIDC_FLOW_INVALID", errorCode(res))
	var n int
	require.NoError(t, o.env.Pool.QueryRow(t.Context(), `SELECT count(*) FROM "UserIdentities" WHERE NOT "IsDeleted"`).Scan(&n))
	assert.Zero(t, n, "no identity is linked under an issuer the database no longer names")
}

func TestAnUnreadableSecretBlocksWritesButNotItsOwnRepair(t *testing.T) {
	o := newOidcEnv(t, apptest.Options{})
	o.addCorp(t, nil)
	second := decodeOidc(t, createOidc(o.owner, o.fresh(), corp(o.idp, map[string]any{"Id": "second", "Issuer": "https://second.example.com"}))).find(t, "second")
	broken := func(id string) {
		t.Helper()
		junk := make([]byte, 48)
		_, err := rand.Read(junk)
		require.NoError(t, err)
		junk[0] = 3
		_, err = o.env.Pool.Exec(t.Context(), `UPDATE "OidcProviders" SET "Secret" = $1 WHERE "ProviderId" = $2 AND NOT "IsDeleted"`, junk, id)
		require.NoError(t, err)
	}
	broken("corp")
	blocked := updateOidc(o.owner, "second", second.Version, o.fresh(), map[string]any{"Name": "Second"})
	assert.Equal(t, http.StatusInternalServerError, blocked.Status, "other writes stay closed while a secret cannot be read")
	assert.NotContains(t, string(blocked.Raw), oidcmock.ClientSecret)

	corpRow := readOidc(t, o.owner).find(t, "corp")
	removed := removeOidc(o.owner, "corp", corpRow.Version, o.fresh())
	require.Equal(t, http.StatusOK, removed.Status, string(removed.Raw))

	broken("second")
	cleared := updateOidc(o.owner, "second", second.Version, o.fresh(), map[string]any{"ClientSecret": ""})
	require.Equal(t, http.StatusOK, cleared.Status, string(cleared.Raw))
	view := decodeOidc(t, cleared).find(t, "second")
	assert.False(t, view.Enabled)
	renamed := updateOidc(o.owner, "second", view.Version, o.fresh(), map[string]any{"Name": "Second"})
	assert.Equal(t, http.StatusOK, renamed.Status, string(renamed.Raw), "writes open again once the secret is gone")
}
