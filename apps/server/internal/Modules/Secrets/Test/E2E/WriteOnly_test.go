package e2e_test

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Gen"
)

const planted = "planted-value-7f3c9e1d-never-shown"

func TestASecretValueNeverComesBack(t *testing.T) {
	w := newWorld(t)
	_ = w.env.SeedEnvironment(w.project, "production")
	paths := []string{w.orgPath("/secrets"), w.projectPath("/secrets"), w.envPath("production", "/secrets")}
	var raws [][]byte
	for i, base := range paths {
		value := planted + "-" + string(rune('a'+i))
		created := put(w.owner, base+"/DEPLOY_TOKEN", map[string]any{"Value": value, "Description": "for the deploy"})
		s := secretOf(t, created, http.StatusOK)
		assert.Equal(t, 1, s.Version)
		assert.Equal(t, len(value), s.SizeBytes)
		raws = append(raws, created.Raw)
		updated := put(w.owner, base+"/DEPLOY_TOKEN", map[string]any{"Value": value + "-v2"})
		assert.Equal(t, 2, secretOf(t, updated, http.StatusOK).Version)
		raws = append(raws, updated.Raw)
		listed := w.owner.Get(base)
		require.Equal(t, http.StatusOK, listed.Status, string(listed.Raw))
		raws = append(raws, listed.Raw)
		restored := post(w.owner, base+"/DEPLOY_TOKEN/restore-version", map[string]int{"Version": 1})
		assert.Equal(t, 3, secretOf(t, restored, http.StatusOK).Version)
		raws = append(raws, restored.Raw)
		removed := post(w.owner, base+"/DEPLOY_TOKEN/remove", nil)
		require.Equal(t, http.StatusOK, removed.Status, string(removed.Raw))
		raws = append(raws, removed.Raw)
	}
	for _, raw := range raws {
		assertNoValue(t, raw, planted)
	}
	assert.NotContains(t, w.env.Logs.String(), planted, "server and access logs")
	var audit string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT coalesce(string_agg("Data"::text, ' '), '') FROM "AuditEvents" WHERE "Action" LIKE 'secret.%'`).Scan(&audit))
	assert.NotEmpty(t, audit)
	assert.NotContains(t, audit, planted)
	assert.Equal(t, 3, w.env.AuditCount("secret.created"))
	assert.Equal(t, 3, w.env.AuditCount("secret.version_restored"))
}

func TestNoResponseTypeHasAValueField(t *testing.T) {
	for _, typ := range []any{gen.Secret{}, gen.SecretResponse{}, gen.SecretListResponse{}, gen.RegistryCredential{}, gen.RegistryCredentialResponse{}} {
		assertNoSecretField(t, reflect.TypeOf(typ), reflect.TypeOf(typ).Name())
	}
}

func assertNoSecretField(t *testing.T, typ reflect.Type, path string) {
	t.Helper()
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || typ.PkgPath() != reflect.TypeOf(gen.Secret{}).PkgPath() {
		return
	}
	for i := range typ.NumField() {
		f := typ.Field(i)
		name := strings.ToLower(f.Name)
		for _, bad := range []string{"value", "password", "hash", "digest", "fingerprint", "sealed"} {
			assert.NotContains(t, name, bad, "%s.%s", path, f.Name)
		}
		assertNoSecretField(t, f.Type, path+"."+f.Name)
	}
}

func TestARegistryPasswordNeverComesBack(t *testing.T) {
	w := newWorld(t)
	created := post(w.owner, w.projectPath("/registry-credentials"), map[string]string{
		"Host": "Registry.Example.com:5000", "Username": "robot", "Password": planted,
	})
	require.Equal(t, http.StatusCreated, created.Status, string(created.Raw))
	var r registryView
	created.Decode(t, &r)
	assert.Equal(t, "registry.example.com:5000", r.Host, "the host is stored in lower case")
	assert.Equal(t, "project", r.Scope)
	updated := put(w.owner, w.projectPath("/registry-credentials/"+r.Id), map[string]string{"Username": "robot2", "Password": planted + "2"})
	require.Equal(t, http.StatusOK, updated.Status, string(updated.Raw))
	listed := w.owner.Get(w.projectPath("/registry-credentials"))
	for _, raw := range [][]byte{created.Raw, updated.Raw, listed.Raw} {
		assertNoValue(t, raw, planted)
	}
	requireError(t, post(w.owner, w.projectPath("/registry-credentials"), map[string]string{
		"Host": "registry.example.com:5000", "Username": "x", "Password": "y",
	}), http.StatusConflict, "REGISTRY_CREDENTIAL_EXISTS")

	login, ok, err := w.service().RegistryLogin(context.Background(), w.org, w.project, "registry.example.com:5000")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "robot2", login.Username)
	assert.Equal(t, planted+"2", string(login.Password))

	removed := post(w.owner, w.projectPath("/registry-credentials/"+r.Id+"/remove"), nil)
	require.Equal(t, http.StatusOK, removed.Status, string(removed.Raw))
	_, ok, err = w.service().RegistryLogin(context.Background(), w.org, w.project, "registry.example.com:5000")
	require.NoError(t, err)
	assert.False(t, ok)
	var left int
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM "RegistryCredentials" WHERE "PasswordSealed" IS NOT NULL`).Scan(&left))
	assert.Zero(t, left, "the password is erased with the credential")
	assert.NotContains(t, w.env.Logs.String(), planted)
}

func TestRegistryHosts(t *testing.T) {
	w := newWorld(t)
	for host, ok := range map[string]bool{
		"docker.io": true, "ghcr.io": true, "https://ghcr.io": false, "ghcr.io/team": false, "10.0.0.5": false,
		"localhost": false, "*.example.com": false, "registry.example.com:99999": false, "user@ghcr.io": false,
	} {
		res := post(w.owner, w.orgPath("/registry-credentials"), map[string]string{"Host": host, "Username": "robot", "Password": "pw"})
		if ok {
			assert.Equal(t, http.StatusCreated, res.Status, "%s: %s", host, res.Raw)
			continue
		}
		requireError(t, res, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	}
	var hosts []registryView
	res := w.owner.Get(w.orgPath("/registry-credentials"))
	res.Decode(t, &hosts)
	got := []string{}
	for _, h := range hosts {
		got = append(got, h.Host)
	}
	assert.ElementsMatch(t, []string{"registry-1.docker.io", "ghcr.io"}, got, "docker.io is the registry it pulls from")
}
