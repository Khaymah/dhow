package e2e_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

func TestATargetKeepsTheOrderOfItsHostsAndEachHostPinsItsOwnKey(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	servers := []sshServer{startSSH(t), startSSH(t), startSSH(t)}
	hosts := make([]map[string]any, 0, len(servers))
	for _, s := range servers {
		port, err := strconv.Atoi(s.port)
		require.NoError(t, err)
		hosts = append(hosts, hostEntry(publicHost, port))
	}
	target := w.newTarget(t, "production", "web", hosts...)
	require.Len(t, target.Hosts, 3)
	assert.False(t, target.Ready)
	assert.ElementsMatch(t, []string{"host_key_unconfirmed", "key_missing"}, target.NotReady)
	for i, h := range target.Hosts {
		assert.Equal(t, i+1, h.Position)
		assert.False(t, h.KeyPinned)
		assert.Equal(t, servers[i].port, strconv.Itoa(h.Port))
	}

	key := decodeTarget(t, w.targetCall(w.owner, http.MethodPost, "production", "/web/generate-key",
		map[string]any{"CurrentFactor": w.ownerFactor()}), http.StatusOK)
	require.NotNil(t, key.PublicKey)
	assert.Contains(t, *key.PublicKey, "ssh-ed25519 ")
	require.NotNil(t, key.KeySecretName)
	assert.Equal(t, "WEB_SSH_KEY", *key.KeySecretName)

	for i, h := range target.Hosts {
		fetched := decodeTarget(t, w.fetchKey(w.owner, "production", "web", h.Id), http.StatusOK)
		require.NotNil(t, fetched.Hosts[i].PendingHostKeyFingerprint)
		assert.Equal(t, servers[i].fingerprint, *fetched.Hosts[i].PendingHostKeyFingerprint)
		assert.False(t, fetched.Hosts[i].KeyPinned, "a fetched key pins nothing")
		confirmed := decodeTarget(t, w.confirmKey(w.owner, "production", "web", h.Id, servers[i].fingerprint), http.StatusOK)
		assert.True(t, confirmed.Hosts[i].KeyPinned)
		require.NotNil(t, confirmed.Hosts[i].HostKeyFingerprint)
		assert.Equal(t, servers[i].fingerprint, *confirmed.Hosts[i].HostKeyFingerprint)
		assert.Nil(t, confirmed.Hosts[i].PendingHostKeyFingerprint)
		assert.Equal(t, i == len(servers)-1, confirmed.Ready, "ready only when every host is pinned")
	}

	final := decodeTarget(t, w.targetCall(w.owner, http.MethodGet, "production", "/web", nil), http.StatusOK)
	assert.True(t, final.Ready)
	assert.Empty(t, final.NotReady)
	for i, h := range final.Hosts {
		assert.Equal(t, target.Hosts[i].Id, h.Id, "the order is kept")
		require.NotNil(t, h.HostKeyFingerprint)
		assert.Equal(t, servers[i].fingerprint, *h.HostKeyFingerprint, "each host has its own key")
	}
}

func TestAnEleventhHostIsRefused(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	hosts := make([]map[string]any, 0, 11)
	for i := range 11 {
		hosts = append(hosts, hostEntry(publicHost, 2300+i))
	}
	res := w.createTarget(w.owner, "production", w.targetBody("fleet", hosts...))
	requireError(t, res, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	assert.Contains(t, detailFields(res), "Hosts")

	tenHosts := hosts[:10]
	target := w.newTarget(t, "production", "fleet", tenHosts...)
	require.Len(t, target.Hosts, 10)
	add := w.targetCall(w.owner, http.MethodPost, "production", "/fleet/hosts", map[string]any{
		"Version": target.Version, "Host": publicHost, "Port": 2400, "CurrentFactor": w.ownerFactor(),
	})
	requireError(t, add, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	assert.Contains(t, detailFields(add), "Hosts")
	assert.Equal(t, 0, w.env.CountRows(`SELECT count(*) FROM "DeployTargetHosts" WHERE "Port" = 2400`))
}

func TestRemovingAHostOrChangingItsAddressDropsItsKey(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	first, second, third := startSSH(t), startSSH(t), startSSH(t)
	ports := []int{}
	for _, s := range []sshServer{first, second, third} {
		p, err := strconv.Atoi(s.port)
		require.NoError(t, err)
		ports = append(ports, p)
	}
	target := w.newTarget(t, "production", "web", hostEntry(publicHost, ports[0]), hostEntry(publicHost, ports[1]), hostEntry(publicHost, ports[2]))
	for i, s := range []sshServer{first, second, third} {
		require.Equal(t, http.StatusOK, w.fetchKey(w.owner, "production", "web", target.Hosts[i].Id).Status)
		target = decodeTarget(t, w.confirmKey(w.owner, "production", "web", target.Hosts[i].Id, s.fingerprint), http.StatusOK)
	}

	moved := decodeTarget(t, w.targetCall(w.owner, http.MethodPut, "production", "/web/hosts/"+target.Hosts[1].Id, map[string]any{
		"Version": target.Version, "Port": ports[1] + 1, "CurrentFactor": w.ownerFactor(),
	}), http.StatusOK)
	assert.False(t, moved.Hosts[1].KeyPinned, "a new address drops the key of the old one")
	assert.Nil(t, moved.Hosts[1].HostKeyFingerprint)
	assert.True(t, moved.Hosts[0].KeyPinned)
	assert.True(t, moved.Hosts[2].KeyPinned)
	assert.False(t, moved.Ready)
	assert.Equal(t, 1, w.env.AuditCount("target.host_updated"))

	removed := decodeTarget(t, w.targetCall(w.owner, http.MethodPost, "production", "/web/hosts/"+moved.Hosts[0].Id+"/remove", map[string]any{
		"Version": moved.Version, "CurrentFactor": w.ownerFactor(),
	}), http.StatusOK)
	require.Len(t, removed.Hosts, 2)
	assert.Equal(t, moved.Hosts[1].Id, removed.Hosts[0].Id)
	assert.Equal(t, 1, removed.Hosts[0].Position, "the order closes the gap")
	assert.Equal(t, 2, removed.Hosts[1].Position)
	assert.Equal(t, 1, w.env.CountRows(`SELECT count(*) FROM "DeployTargetHosts" WHERE "IsDeleted" AND "HostKey" IS NOT NULL`),
		"the removed host keeps its row marked deleted")

	again := w.targetCall(w.owner, http.MethodPost, "production", "/web/hosts/"+removed.Hosts[0].Id+"/remove", map[string]any{
		"Version": removed.Version, "CurrentFactor": w.ownerFactor(),
	})
	require.Equal(t, http.StatusOK, again.Status, string(again.Raw))
	last := decodeTarget(t, again, http.StatusOK)
	requireError(t, w.targetCall(w.owner, http.MethodPost, "production", "/web/hosts/"+last.Hosts[0].Id+"/remove", map[string]any{
		"Version": last.Version, "CurrentFactor": w.ownerFactor(),
	}), http.StatusUnprocessableEntity, "VALIDATION_FAILED")
}

func TestReorderingTheHostsChangesThePositions(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	target := w.newTarget(t, "production", "web", hostEntry(publicHost, 2201), hostEntry(publicHost, 2202), hostEntry(publicHost, 2203))
	ids := []string{target.Hosts[2].Id, target.Hosts[0].Id, target.Hosts[1].Id}
	res := w.targetCall(w.owner, http.MethodPut, "production", "/web/hosts-order", map[string]any{
		"Version": target.Version, "HostIds": ids, "CurrentFactor": w.ownerFactor(),
	})
	reordered := decodeTarget(t, res, http.StatusOK)
	for i, h := range reordered.Hosts {
		assert.Equal(t, ids[i], h.Id)
		assert.Equal(t, i+1, h.Position)
	}
	assert.Greater(t, reordered.Version, target.Version)

	stale := w.targetCall(w.owner, http.MethodPut, "production", "/web/hosts-order", map[string]any{
		"Version": target.Version, "HostIds": ids, "CurrentFactor": w.ownerFactor(),
	})
	requireError(t, stale, http.StatusConflict, "TARGET_VERSION_CONFLICT")
	partial := w.targetCall(w.owner, http.MethodPut, "production", "/web/hosts-order", map[string]any{
		"Version": reordered.Version, "HostIds": ids[:2], "CurrentFactor": w.ownerFactor(),
	})
	requireError(t, partial, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
}

func TestABadTargetIsRefused(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	w.env.SeedSecret(apptest.SecretRef(w.org, &w.project, "production"), "EXISTING_KEY", "an existing private key value")
	tests := []struct {
		name   string
		change func(b map[string]any)
		status int
		code   string
		field  string
	}{
		{"a name with capitals", func(b map[string]any) { b["Name"] = "Web" }, 422, "VALIDATION_FAILED", "Name"},
		{"an unknown kind", func(b map[string]any) { b["Kind"] = "ftp" }, 422, "VALIDATION_FAILED", "Kind"},
		{"a username with a space", func(b map[string]any) { b["Username"] = "dep loy" }, 422, "VALIDATION_FAILED", "Username"},
		{"root without the explicit permission", func(b map[string]any) { b["Username"] = "root" }, 422, "VALIDATION_FAILED", "Username"},
		{"a relative path", func(b map[string]any) { b["DeployPath"] = "srv/shop" }, 422, "DEPLOY_PATH_INVALID", "DeployPath"},
		{"the root path", func(b map[string]any) { b["DeployPath"] = "/" }, 422, "DEPLOY_PATH_INVALID", "DeployPath"},
		{"a system path", func(b map[string]any) { b["DeployPath"] = "/etc/shop" }, 422, "DEPLOY_PATH_INVALID", "DeployPath"},
		{"the docker folder", func(b map[string]any) { b["DeployPath"] = "/var/lib/docker/shop" }, 422, "DEPLOY_PATH_INVALID", "DeployPath"},
		{"a path that climbs", func(b map[string]any) { b["DeployPath"] = "/srv/../etc" }, 422, "DEPLOY_PATH_INVALID", "DeployPath"},
		{"a path with a shell character", func(b map[string]any) { b["DeployPath"] = "/srv/shop;ls" }, 422, "DEPLOY_PATH_INVALID", "DeployPath"},
		{"a one-level path", func(b map[string]any) { b["DeployPath"] = "/shop" }, 422, "DEPLOY_PATH_INVALID", "DeployPath"},
		{"too few releases", func(b map[string]any) { b["KeepReleases"] = 1 }, 422, "VALIDATION_FAILED", "KeepReleases"},
		{"a port out of range", func(b map[string]any) { b["Hosts"] = []map[string]any{hostEntry(publicHost, 70000)} }, 422, "VALIDATION_FAILED", "Hosts[0].Port"},
		{"a host with a scheme", func(b map[string]any) { b["Hosts"] = []map[string]any{hostEntry("ssh://deploy.example.test", 22)} }, 422, "VALIDATION_FAILED", "Hosts[0].Host"},
		{"a metadata address", func(b map[string]any) { b["Hosts"] = []map[string]any{hostEntry("169.254.169.254", 22)} }, 422, "TARGET_HOST_BLOCKED", "Hosts[0].Host"},
		{"a loopback address", func(b map[string]any) { b["Hosts"] = []map[string]any{hostEntry("127.0.0.1", 22)} }, 422, "TARGET_HOST_BLOCKED", "Hosts[0].Host"},
		{"the same host twice", func(b map[string]any) {
			b["Hosts"] = []map[string]any{hostEntry(publicHost, 22), hostEntry(publicHost, 22)}
		}, 409, "TARGET_DUPLICATE", ""},
		{"no hosts", func(b map[string]any) { b["Hosts"] = []map[string]any{} }, 422, "VALIDATION_FAILED", ""},
		{"a key secret that does not exist", func(b map[string]any) { b["KeySecretName"] = "NO_SUCH_KEY" }, 422, "TARGET_KEY_NOT_FOUND", "KeySecretName"},
		{"a reserved key secret name", func(b map[string]any) { b["KeySecretName"] = "DHOW_KEY" }, 422, "VALIDATION_FAILED", "KeySecretName"},
		{"settings of the other kind", func(b map[string]any) {
			b["Settings"] = map[string]any{"Compose": map[string]any{"ProjectName": "shop"}}
		}, 422, "VALIDATION_FAILED", "Settings.Compose"},
		{"a hook with a line break", func(b map[string]any) {
			b["Settings"] = map[string]any{"Ssh": map[string]any{"ReloadCommand": "systemctl reload shop\nrm -rf /"}}
		}, 422, "VALIDATION_FAILED", "Settings.Ssh.ReloadCommand"},
		{"a batch bigger than the hosts", func(b map[string]any) {
			b["Settings"] = map[string]any{"Rolling": map[string]any{"BatchSize": 2}}
		}, 422, "VALIDATION_FAILED", "Settings.Rolling.BatchSize"},
		{"a shared name that climbs", func(b map[string]any) {
			b["Settings"] = map[string]any{"Ssh": map[string]any{"Shared": []string{"..", "ok"}}}
		}, 422, "VALIDATION_FAILED", "Settings.Ssh.Shared[0]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := w.targetBody("web")
			tt.change(body)
			res := w.createTarget(w.owner, "production", body)
			requireError(t, res, tt.status, tt.code)
			if tt.field != "" {
				assert.Contains(t, detailFields(res), tt.field)
			}
		})
	}
	assert.Zero(t, w.env.CountRows(`SELECT count(*) FROM "DeployTargets"`), "nothing was stored")

	body := w.targetBody("web")
	body["KeySecretName"] = "EXISTING_KEY"
	assert.Equal(t, http.StatusCreated, w.createTarget(w.owner, "production", body).Status, "an existing secret of the environment is accepted")
}

func TestRootNeedsTheExplicitPermission(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	body := w.targetBody("web")
	body["Username"] = "root"
	body["Settings"] = map[string]any{"AllowRoot": true}
	target := decodeTarget(t, w.createTarget(w.owner, "production", body), http.StatusCreated)
	assert.Equal(t, "root", target.Username)
	require.NotNil(t, target.Settings.AllowRoot)
	assert.True(t, *target.Settings.AllowRoot)
}

func TestTargetNamesAndAddressesAreUnique(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	w.newTarget(t, "production", "web")
	requireError(t, w.createTarget(w.owner, "production", w.targetBody("web", hostEntry(publicHost, 2299))), http.StatusConflict, "TARGET_EXISTS")
	requireError(t, w.createTarget(w.owner, "production", w.targetBody("api")), http.StatusConflict, "TARGET_DUPLICATE")

	other := w.targetBody("api")
	other["DeployPath"] = "/srv/api"
	decodeTarget(t, w.createTarget(w.owner, "production", other), http.StatusCreated)

	staging := decodeEnvironment(t, w)
	assert.NotEmpty(t, staging.Id)
	requireError(t, w.createTarget(w.owner, "staging", w.targetBody("web")), http.StatusConflict, "TARGET_DUPLICATE")
}

func decodeEnvironment(t *testing.T, w world) environmentView {
	t.Helper()
	return decode(t, w.create(w.owner, map[string]any{"Name": "staging"}), http.StatusCreated)
}

func TestUpdatingATargetNeedsTheCurrentVersionAndAFreshFactor(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	target := w.newTarget(t, "production", "web")
	update := func(version int64, factor map[string]any, change map[string]any) apptest.Response {
		change["Version"], change["CurrentFactor"] = version, factor
		return w.targetCall(w.owner, http.MethodPut, "production", "/web", change)
	}
	requireError(t, update(target.Version, wrongFactor(), map[string]any{"Username": "ops"}), http.StatusForbidden, "INVALID_CURRENT_FACTOR")
	requireError(t, update(target.Version+7, w.ownerFactor(), map[string]any{"Username": "ops"}), http.StatusConflict, "TARGET_VERSION_CONFLICT")
	requireError(t, update(target.Version, w.ownerFactor(), map[string]any{}), http.StatusBadRequest, "BAD_REQUEST")

	changed := decodeTarget(t, update(target.Version, w.ownerFactor(), map[string]any{
		"Username": "ops", "KeepReleases": 8, "DeployPath": "/srv/shop2",
		"Settings": map[string]any{"Ssh": map[string]any{"ReloadCommand": "systemctl reload shop", "Shared": []string{".env"}}},
	}), http.StatusOK)
	assert.Equal(t, "ops", changed.Username)
	assert.Equal(t, 8, changed.KeepReleases)
	assert.Equal(t, "/srv/shop2", changed.DeployPath)
	assert.Greater(t, changed.Version, target.Version)
	require.NotNil(t, changed.Settings.Ssh)
	assert.Equal(t, "systemctl reload shop", changed.Settings.Ssh.ReloadCommand)
	assert.Equal(t, []string{".env"}, changed.Settings.Ssh.Shared)
	assert.Equal(t, 1, w.env.CountRows(`SELECT count(*) FROM "DeployTargetHosts" WHERE "DeployPath" = '/srv/shop2'`), "the hosts follow the path")

	same := decodeTarget(t, update(changed.Version, w.ownerFactor(), map[string]any{"Username": "ops"}), http.StatusOK)
	assert.Equal(t, changed.Version, same.Version, "a change that changes nothing keeps the version")
	assert.Equal(t, 1, w.env.AuditCount("target.updated"))
}

func TestOnlyAnAdminWritesTargetsAndEveryoneWhoReadsTheProjectReadsThem(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	target := w.newTarget(t, "production", "web")
	for _, role := range []string{"maintainer", "developer", "viewer"} {
		t.Run(role, func(t *testing.T) {
			c, _ := w.member(t, role)
			assert.Equal(t, http.StatusOK, w.targetCall(c, http.MethodGet, "production", "", nil).Status)
			assert.Equal(t, http.StatusOK, w.targetCall(c, http.MethodGet, "production", "/web", nil).Status)
			body := w.targetBody("other")
			body["CurrentFactor"] = memberFactor()
			requireError(t, w.createTarget(c, "production", body), http.StatusForbidden, "FORBIDDEN")
			requireError(t, w.targetCall(c, http.MethodPut, "production", "/web", map[string]any{
				"Version": target.Version, "Username": "ops", "CurrentFactor": memberFactor(),
			}), http.StatusForbidden, "FORBIDDEN")
			requireError(t, w.targetCall(c, http.MethodPost, "production", "/web/remove", map[string]any{"CurrentFactor": memberFactor()}),
				http.StatusForbidden, "FORBIDDEN")
			requireError(t, w.targetCall(c, http.MethodPost, "production", "/web/generate-key", map[string]any{"CurrentFactor": memberFactor()}),
				http.StatusForbidden, "FORBIDDEN")
			requireError(t, w.fetchKey(c, "production", "web", target.Hosts[0].Id), http.StatusForbidden, "FORBIDDEN")
			requireError(t, w.targetCall(c, http.MethodPut, "production", "/web/hosts/"+target.Hosts[0].Id+"/pending-host-key",
				map[string]any{"HostKey": "ssh-ed25519 AAAA"}), http.StatusForbidden, "FORBIDDEN")
			requireError(t, w.confirmKey(c, "production", "web", target.Hosts[0].Id, "SHA256:"+strings.Repeat("A", 43)),
				http.StatusForbidden, "FORBIDDEN")
		})
	}
	admin, _ := w.member(t, "admin")
	body := w.targetBody("other")
	body["DeployPath"] = "/srv/other"
	body["CurrentFactor"] = memberFactor()
	assert.Equal(t, http.StatusCreated, w.createTarget(admin, "production", body).Status)
	assert.Equal(t, 0, w.env.AuditCount("target.removed"))
}

func TestAPITokensReadTargetsButNeverChangeThem(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	target := w.newTarget(t, "production", "web")
	token := w.env.SeedAPIToken(w.ownerID, w.org, "project.read", "project.manage")
	read := w.env.Bearer(token.Value, apptest.Request{Method: http.MethodGet, Path: w.targetsPath("production", "/web"), Headers: map[string]string{"Origin": ""}})
	require.Equal(t, http.StatusOK, read.Status, string(read.Raw))
	list := w.env.Bearer(token.Value, apptest.Request{Method: http.MethodGet, Path: w.targetsPath("production", ""), Headers: map[string]string{"Origin": ""}})
	require.Equal(t, http.StatusOK, list.Status, string(list.Raw))
	for _, req := range []apptest.Request{
		{Method: http.MethodPost, Path: w.targetsPath("production", ""), Body: w.targetBody("other")},
		{Method: http.MethodPost, Path: w.targetsPath("production", "/web/generate-key"), Body: map[string]any{"CurrentFactor": w.ownerFactor()}},
		{Method: http.MethodPost, Path: w.targetsPath("production", "/web/hosts/"+target.Hosts[0].Id+"/fetch-host-key")},
	} {
		req.Headers = map[string]string{"Origin": ""}
		requireError(t, w.env.Bearer(token.Value, req), http.StatusForbidden, "SESSION_REQUIRED")
	}
}

func TestRemovingAnEnvironmentRemovesItsTargetsAndSecrets(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	w.newTarget(t, "production", "web")
	decodeTarget(t, w.targetCall(w.owner, http.MethodPost, "production", "/web/generate-key", map[string]any{"CurrentFactor": w.ownerFactor()}), http.StatusOK)
	require.Equal(t, 1, w.env.CountRows(`SELECT count(*) FROM "Secrets" WHERE "Name" = 'WEB_SSH_KEY' AND NOT "IsDeleted"`))

	removed := w.post(w.owner, "production", "remove", map[string]any{"CurrentFactor": w.ownerFactor()})
	require.Equal(t, http.StatusOK, removed.Status, string(removed.Raw))
	assert.Zero(t, w.env.CountRows(`SELECT count(*) FROM "DeployTargets" WHERE NOT "IsDeleted"`))
	assert.Zero(t, w.env.CountRows(`SELECT count(*) FROM "DeployTargetHosts" WHERE NOT "IsDeleted"`))
	assert.Zero(t, w.env.CountRows(`SELECT count(*) FROM "Secrets" WHERE "Name" = 'WEB_SSH_KEY' AND NOT "IsDeleted"`))
	assert.Equal(t, 1, w.env.CountRows(`SELECT count(*) FROM "DeployTargets" WHERE "IsDeleted"`), "the rows stay, marked deleted")

	again := w.create(w.owner, map[string]any{"Name": "production"})
	require.Equal(t, http.StatusCreated, again.Status, string(again.Raw))
	reused := w.newTarget(t, "production", "web")
	assert.Equal(t, "web", reused.Name, "the name and the address are free again")
}

func TestRemovingATargetFreesItsNameAndKeepsTheSecret(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	target := w.newTarget(t, "production", "web")
	decodeTarget(t, w.targetCall(w.owner, http.MethodPost, "production", "/web/generate-key", map[string]any{"CurrentFactor": w.ownerFactor()}), http.StatusOK)
	res := w.targetCall(w.owner, http.MethodPost, "production", "/web/remove", map[string]any{"CurrentFactor": w.ownerFactor()})
	removed := decodeTarget(t, res, http.StatusOK)
	assert.Equal(t, target.Id, removed.Id)
	requireError(t, w.targetCall(w.owner, http.MethodGet, "production", "/web", nil), http.StatusNotFound, "NOT_FOUND")
	assert.Equal(t, 1, w.env.CountRows(`SELECT count(*) FROM "Secrets" WHERE "Name" = 'WEB_SSH_KEY' AND NOT "IsDeleted"`))
	w.newTarget(t, "production", "web")
	assert.Equal(t, 1, w.env.AuditCount("target.removed"))
}

func TestATargetOfAnotherProjectOrEnvironmentIsNotFound(t *testing.T) {
	w := newTargetWorld(t)
	w.production(t)
	w.create(w.owner, map[string]any{"Name": "staging"})
	w.newTarget(t, "production", "web")
	requireError(t, w.targetCall(w.owner, http.MethodGet, "staging", "/web", nil), http.StatusNotFound, "NOT_FOUND")
	requireError(t, w.targetCall(w.owner, http.MethodGet, "nowhere", "/web", nil), http.StatusNotFound, "NOT_FOUND")
	other := w.env.SeedProject(w.org, w.ownerID)
	res := w.owner.Do(apptest.Request{Method: http.MethodGet, Path: w.path(other, "/production/targets/web")})
	requireError(t, res, http.StatusNotFound, "NOT_FOUND")
}

func TestAllowExtraDeployEgressIsARuleOfTheEnvironment(t *testing.T) {
	w := newTargetWorld(t)
	e := w.production(t)
	assert.False(t, e.Rules.AllowExtraDeployEgress)
	updated := decode(t, w.update(w.owner, "production", map[string]any{
		"Version": e.Version, "Rules": map[string]any{"AllowExtraDeployEgress": true}, "CurrentFactor": w.ownerFactor(),
	}), http.StatusOK)
	assert.True(t, updated.Rules.AllowExtraDeployEgress)
	assert.Greater(t, updated.RuleVersion, e.RuleVersion, "it is a rule, so approvals given before it no longer count")
}
