package e2e_test

import (
	"context"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	secretstore "github.com/Khaymah/dhow/apps/server/internal/Core/SecretStore"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Gen"
)

func portOf(t *testing.T, s sshServer) int {
	t.Helper()
	p, err := strconv.Atoi(s.port)
	require.NoError(t, err)
	return p
}

func TestAFetchedKeyPinsNothingUntilItsFingerprintIsConfirmed(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	server := startSSH(t)
	other := startSSH(t)
	target := w.newTarget(t, "production", "web", hostEntry(publicHost, portOf(t, server)), hostEntry(publicHost, portOf(t, other)))
	first := target.Hosts[0].Id

	requireError(t, w.confirmKey(w.owner, "production", "web", first, server.fingerprint), http.StatusConflict, "HOST_KEY_NOT_PENDING")

	fetched := decodeTarget(t, w.fetchKey(w.owner, "production", "web", first), http.StatusOK)
	require.NotNil(t, fetched.Hosts[0].PendingHostKeyFingerprint)
	assert.Equal(t, server.fingerprint, *fetched.Hosts[0].PendingHostKeyFingerprint)
	assert.False(t, fetched.Hosts[0].KeyPinned)

	requireError(t, w.confirmKey(w.owner, "production", "web", first, other.fingerprint), http.StatusConflict, "HOST_KEY_FINGERPRINT_MISMATCH")
	stillPending := decodeTarget(t, w.targetCall(w.owner, http.MethodGet, "production", "/web", nil), http.StatusOK)
	assert.False(t, stillPending.Hosts[0].KeyPinned, "a wrong fingerprint pins nothing")
	assert.NotNil(t, stillPending.Hosts[0].PendingHostKeyFingerprint)

	res := w.targetCall(w.owner, http.MethodPost, "production", "/web/hosts/"+first+"/confirm-host-key",
		map[string]any{"Fingerprint": server.fingerprint, "CurrentFactor": wrongFactor()})
	requireError(t, res, http.StatusForbidden, "INVALID_CURRENT_FACTOR")

	confirmed := decodeTarget(t, w.confirmKey(w.owner, "production", "web", first, server.fingerprint), http.StatusOK)
	assert.True(t, confirmed.Hosts[0].KeyPinned)
	require.NotNil(t, confirmed.Hosts[0].HostKeyConfirmedAt)
	require.NotNil(t, confirmed.Hosts[0].HostKeyConfirmedByUserId)
	assert.Equal(t, w.ownerID.String(), *confirmed.Hosts[0].HostKeyConfirmedByUserId)
	assert.False(t, confirmed.Hosts[1].KeyPinned, "the other host has its own key")
	assert.False(t, confirmed.Ready)
	assert.Equal(t, 1, w.env.AuditCount("target.host_key_confirmed"))
	assert.Equal(t, 1, w.env.AuditCount("target.host_key_fetched"))
}

func TestAPastedKeyWaitsForConfirmationToo(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	server := startSSH(t)
	target := w.newTarget(t, "production", "web", hostEntry(publicHost, portOf(t, server)))
	host := target.Hosts[0].Id
	paste := func(text string) apptest.Response {
		return w.targetCall(w.owner, http.MethodPut, "production", "/web/hosts/"+host+"/pending-host-key", map[string]any{"HostKey": text})
	}

	for _, bad := range []string{"not a key", "ssh-ed25519 AAAA", "@cert-authority * " + server.authorized, "# only a comment"} {
		res := paste(bad)
		requireError(t, res, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
		assert.Contains(t, detailFields(res), "HostKey")
	}

	pasted := decodeTarget(t, paste("deploy.example.test,203.0.113.20 "+server.authorized+"\n"), http.StatusOK)
	require.NotNil(t, pasted.Hosts[0].PendingHostKeyFingerprint)
	assert.Equal(t, server.fingerprint, *pasted.Hosts[0].PendingHostKeyFingerprint)
	assert.False(t, pasted.Hosts[0].KeyPinned)
	requireError(t, w.confirmKey(w.owner, "production", "web", host, "SHA256:"+strings.Repeat("A", 43)), http.StatusConflict, "HOST_KEY_FINGERPRINT_MISMATCH")
	assert.True(t, decodeTarget(t, w.confirmKey(w.owner, "production", "web", host, server.fingerprint), http.StatusOK).Hosts[0].KeyPinned)
}

func TestAChangedKeyStaysPendingWhileThePinnedOneKeepsWorking(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	server := startSSH(t)
	target := w.newTarget(t, "production", "web", hostEntry(publicHost, portOf(t, server)))
	host := target.Hosts[0].Id
	require.Equal(t, http.StatusOK, w.fetchKey(w.owner, "production", "web", host).Status)
	pinned := decodeTarget(t, w.confirmKey(w.owner, "production", "web", host, server.fingerprint), http.StatusOK)

	replacement := describe(newHostKey(t), server.port)
	res := w.targetCall(w.owner, http.MethodPut, "production", "/web/hosts/"+host+"/pending-host-key", map[string]any{"HostKey": replacement.authorized})
	changed := decodeTarget(t, res, http.StatusOK)
	assert.True(t, changed.Hosts[0].KeyPinned, "the old key keeps working")
	require.NotNil(t, changed.Hosts[0].HostKeyFingerprint)
	assert.Equal(t, server.fingerprint, *changed.Hosts[0].HostKeyFingerprint)
	require.NotNil(t, changed.Hosts[0].PendingHostKeyFingerprint)
	assert.Equal(t, replacement.fingerprint, *changed.Hosts[0].PendingHostKeyFingerprint, "the new one waits")
	assert.Equal(t, pinned.Version, changed.Version, "a pending key changes nothing a deploy depends on")

	same := decodeTarget(t, w.fetchKey(w.owner, "production", "web", host), http.StatusOK)
	assert.Nil(t, same.Hosts[0].PendingHostKeyFingerprint, "the server still presents the pinned key, so nothing waits")
	assert.True(t, same.Hosts[0].KeyPinned)

	decodeTarget(t, w.targetCall(w.owner, http.MethodPut, "production", "/web/hosts/"+host+"/pending-host-key", map[string]any{"HostKey": replacement.authorized}), http.StatusOK)
	confirmed := decodeTarget(t, w.confirmKey(w.owner, "production", "web", host, replacement.fingerprint), http.StatusOK)
	require.NotNil(t, confirmed.Hosts[0].HostKeyFingerprint)
	assert.Equal(t, replacement.fingerprint, *confirmed.Hosts[0].HostKeyFingerprint)
	assert.Greater(t, confirmed.Version, pinned.Version)

	var data string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT "Data"::text FROM "AuditEvents" WHERE "Action" = 'target.host_key_confirmed' ORDER BY "Seq" DESC LIMIT 1`).Scan(&data))
	assert.Contains(t, data, server.fingerprint, "the audit event has the old fingerprint")
	assert.Contains(t, data, replacement.fingerprint, "and the new one")
}

func TestAPrivateOrMetadataAddressIsNeverDialed(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	target := w.newTarget(t, "production", "web", hostEntry(privateHost, 2201), hostEntry(metadataHost, 2202), hostEntry("missing.example.test", 2203), hostEntry(publicHost, 22))
	for i, want := range []struct {
		status int
		code   string
	}{
		{http.StatusUnprocessableEntity, "TARGET_HOST_BLOCKED"},
		{http.StatusUnprocessableEntity, "TARGET_HOST_BLOCKED"},
		{http.StatusBadGateway, "TARGET_HOST_UNREACHABLE"},
		{http.StatusBadGateway, "TARGET_HOST_UNREACHABLE"},
	} {
		res := w.fetchKey(w.owner, "production", "web", target.Hosts[i].Id)
		requireError(t, res, want.status, want.code)
		for _, leak := range []string{"dial", "tcp", "connection refused", "127.0.0.1", "203.0.113.20", "10.1.2.3", "169.254", "lookup", "example.test"} {
			assert.NotContains(t, string(res.Raw), leak, "the dial error never reaches the body")
		}
	}
	after := decodeTarget(t, w.targetCall(w.owner, http.MethodGet, "production", "/web", nil), http.StatusOK)
	for _, h := range after.Hosts {
		assert.False(t, h.KeyPinned)
		assert.Nil(t, h.PendingHostKeyFingerprint)
	}
	server := startSSH(t)
	pasted := w.targetCall(w.owner, http.MethodPut, "production", "/web/hosts/"+target.Hosts[0].Id+"/pending-host-key", map[string]any{"HostKey": server.authorized})
	assert.Equal(t, http.StatusOK, pasted.Status, "a private address takes a pasted key")
}

func TestAHostThatIsNotInTheTargetIsNotFound(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	w.newTarget(t, "production", "web")
	missing := uuid.NewString()
	requireError(t, w.fetchKey(w.owner, "production", "web", missing), http.StatusNotFound, "NOT_FOUND")
	requireError(t, w.confirmKey(w.owner, "production", "web", missing, "SHA256:"+strings.Repeat("A", 43)), http.StatusNotFound, "NOT_FOUND")
	requireError(t, w.targetCall(w.owner, http.MethodPut, "production", "/web/hosts/"+missing+"/pending-host-key",
		map[string]any{"HostKey": describe(newHostKey(t), "1").authorized}), http.StatusNotFound, "NOT_FOUND")
}

func (w world) environmentID(t *testing.T, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT "Id" FROM "Environments" WHERE "ProjectId" = $1 AND "Name" = $2 AND NOT "IsDeleted"`, w.project, name).Scan(&id))
	return id
}

func TestTheDeployKeyIsAnEnvironmentSecretAndTheirPrivateHalfNeverComesBack(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	server := startSSH(t)
	target := w.newTarget(t, "production", "web", hostEntry(publicHost, portOf(t, server)))
	raws := [][]byte{}
	first := w.targetCall(w.owner, http.MethodPost, "production", "/web/generate-key", map[string]any{"CurrentFactor": w.ownerFactor()})
	raws = append(raws, first.Raw)
	generated := decodeTarget(t, first, http.StatusOK)
	require.NotNil(t, generated.PublicKey)
	require.NotNil(t, generated.KeySecretName)
	assert.Equal(t, "WEB_SSH_KEY", *generated.KeySecretName)
	assert.Equal(t, 1, w.env.AuditCount("secret.created"))

	env := w.environmentID(t, "production")
	resolve := func() string {
		got, err := w.env.Wired.Options.Secrets.Resolve(context.Background(), secretstore.Request{
			OrganizationID: w.org, ProjectID: w.project, EnvironmentID: &env, Names: []string{"WEB_SSH_KEY"},
		})
		require.NoError(t, err)
		require.Len(t, got, 1)
		defer secretstore.Wipe(got)
		return string(got[0].Value)
	}
	private := resolve()
	assert.Contains(t, private, "BEGIN OPENSSH PRIVATE KEY")
	signer, err := ssh.ParsePrivateKey([]byte(private))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(*generated.PublicKey, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))),
		"the public key shown is the public half of the stored secret")

	for _, call := range []apptest.Response{
		w.targetCall(w.owner, http.MethodGet, "production", "/web", nil),
		w.targetCall(w.owner, http.MethodGet, "production", "", nil),
		w.fetchKey(w.owner, "production", "web", target.Hosts[0].Id),
		w.owner.Get(apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/environments/production/secrets")),
		w.owner.Get(apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/environments/production")),
	} {
		raws = append(raws, call.Raw)
	}
	second := w.targetCall(w.owner, http.MethodPost, "production", "/web/generate-key", map[string]any{"CurrentFactor": w.ownerFactor()})
	raws = append(raws, second.Raw)
	renewed := decodeTarget(t, second, http.StatusOK)
	assert.NotEqual(t, *generated.PublicKey, *renewed.PublicKey, "doing it again makes a new key")
	assert.NotEqual(t, private, resolve())
	assert.Equal(t, 1, w.env.AuditCount("secret.updated"), "the new key is a new version of the same secret")
	assert.Equal(t, 2, w.env.AuditCount("target.key_generated"))

	var body strings.Builder
	for _, block := range strings.Split(private, "\n") {
		if len(block) > 20 && !strings.HasPrefix(block, "-----") {
			body.WriteString(block)
		}
	}
	require.NotEmpty(t, body.String())
	fragments := []string{private, strings.TrimSpace(body.String())[:60]}
	for _, raw := range raws {
		for _, f := range fragments {
			assert.NotContains(t, string(raw), f)
		}
		assert.NotContains(t, string(raw), "PRIVATE KEY")
	}
	var audit string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT coalesce(string_agg("Data"::text, ' '), '') FROM "AuditEvents"`).Scan(&audit))
	for _, f := range fragments {
		assert.NotContains(t, audit, f)
		assert.NotContains(t, w.env.Logs.String(), f)
	}
	assert.NotContains(t, audit, "PRIVATE KEY")
	assert.Contains(t, audit, "WEB_SSH_KEY")
}

func TestNoTargetResponseTypeHasAPrivateKeyOrValueField(t *testing.T) {
	for _, typ := range []any{
		gen.DeployTarget{}, gen.DeployTargetHost{}, gen.DeployTargetResponse{}, gen.DeployTargetListResponse{}, gen.DeployTargetSettings{},
	} {
		assertNoPrivateField(t, reflect.TypeOf(typ), reflect.TypeOf(typ).Name())
	}
	assertNoPrivateField(t, reflect.TypeFor[gen.DeployTarget](), "DeployTarget")
}

func assertNoPrivateField(t *testing.T, typ reflect.Type, path string) {
	t.Helper()
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || typ.PkgPath() != reflect.TypeFor[gen.DeployTarget]().PkgPath() {
		return
	}
	for i := range typ.NumField() {
		f := typ.Field(i)
		name := strings.ToLower(f.Name)
		for _, bad := range []string{"private", "value", "sealed", "password", "token", "hash", "passphrase"} {
			assert.NotContains(t, name, bad, "%s.%s", path, f.Name)
		}
		if strings.Contains(name, "secret") {
			assert.Equal(t, "keysecretname", name, "%s.%s is the name of the secret, never its value", path, f.Name)
		}
		assertNoPrivateField(t, f.Type, path+"."+f.Name)
	}
}

func TestGeneratingAKeyNeverOverwritesAnotherSecret(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	ref := apptest.SecretRef(w.org, &w.project, "production")
	w.env.SeedSecret(ref, "WEB_SSH_KEY", "a user secret that must survive")
	w.newTarget(t, "production", "web")
	body := map[string]any{"CurrentFactor": w.ownerFactor()}
	requireError(t, w.targetCall(w.owner, http.MethodPost, "production", "/web/generate-key", body), http.StatusConflict, "TARGET_KEY_NAME_TAKEN")

	env := w.environmentID(t, "production")
	got, err := w.env.Wired.Options.Secrets.Resolve(context.Background(), secretstore.Request{
		OrganizationID: w.org, ProjectID: w.project, EnvironmentID: &env, Names: []string{"WEB_SSH_KEY"},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "a user secret that must survive", string(got[0].Value))
	assert.Equal(t, int32(1), got[0].Version)
	secretstore.Wipe(got)
	still := decodeTarget(t, w.targetCall(w.owner, http.MethodGet, "production", "/web", nil), http.StatusOK)
	assert.Nil(t, still.KeySecretName)
	assert.Zero(t, w.env.AuditCount("target.key_generated"))
}

func TestTwoTargetsThatWouldShareAKeyNameAreRefused(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	w.newTarget(t, "production", "web-1")
	other := w.targetBody("web_1", hostEntry(publicHost, 2299))
	other["DeployPath"] = "/srv/other"
	requireError(t, w.createTarget(w.owner, "production", other), http.StatusConflict, "TARGET_DUPLICATE")
	third := w.targetBody("web-2", hostEntry(publicHost, 2298))
	third["DeployPath"] = "/srv/third"
	decodeTarget(t, w.createTarget(w.owner, "production", third), http.StatusCreated)
}

func TestRotatingTheKeyOfTheSameTargetStaysAllowed(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	w.newTarget(t, "production", "web")
	body := map[string]any{"CurrentFactor": w.ownerFactor()}
	first := decodeTarget(t, w.targetCall(w.owner, http.MethodPost, "production", "/web/generate-key", body), http.StatusOK)
	second := decodeTarget(t, w.targetCall(w.owner, http.MethodPost, "production", "/web/generate-key", body), http.StatusOK)
	assert.NotEqual(t, *first.PublicKey, *second.PublicKey)
	assert.Equal(t, "WEB_SSH_KEY", *second.KeySecretName)
}

func TestAnArchivedProjectTakesNoTargetWrites(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	server := startSSH(t)
	target := w.newTarget(t, "production", "web", hostEntry(publicHost, portOf(t, server)))
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "Projects" SET "State" = 'archived', "ArchivedAt" = now() WHERE "Id" = $1`, w.project)
	require.NoError(t, err)
	factor := map[string]any{"CurrentFactor": w.ownerFactor()}
	requireError(t, w.createTarget(w.owner, "production", w.targetBody("api")), http.StatusConflict, "PROJECT_ARCHIVED")
	requireError(t, w.targetCall(w.owner, http.MethodPost, "production", "/web/generate-key", factor), http.StatusConflict, "PROJECT_ARCHIVED")
	requireError(t, w.fetchKey(w.owner, "production", "web", target.Hosts[0].Id), http.StatusConflict, "PROJECT_ARCHIVED")
	requireError(t, w.targetCall(w.owner, http.MethodPost, "production", "/web/remove", factor), http.StatusConflict, "PROJECT_ARCHIVED")
	assert.Equal(t, http.StatusOK, w.targetCall(w.owner, http.MethodGet, "production", "/web", nil).Status, "reading still works")
}
