package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
)

func blockedAddresses() map[string]string {
	return map[string]string{
		"loopback":            "127.0.0.1",
		"loopback range":      "127.8.9.10",
		"ipv6 loopback":       "::1",
		"unspecified":         "0.0.0.0",
		"private 192.168":     "192.168.1.10",
		"private 172.16":      "172.16.5.5",
		"carrier grade nat":   "100.64.0.1",
		"link local":          "169.254.1.1",
		"cloud metadata":      "169.254.169.254",
		"ecs metadata":        "169.254.170.2",
		"azure wire server":   "168.63.129.16",
		"alibaba metadata":    "100.100.100.200",
		"ipv6 metadata":       "fd00:ec2::254",
		"ipv4 mapped":         "::ffff:127.0.0.1",
		"ipv4 mapped private": "::ffff:192.168.0.1",
		"nat64 of loopback":   "64:ff9b::7f00:1",
		"6to4 of private":     "2002:c0a8:0101::",
		"unique local":        "fc00::1",
		"link local ipv6":     "fe80::1",
		"multicast":           "224.0.0.1",
	}
}

func literal(addr string) string {
	if len(addr) > 0 && (addr[0] == ':' || (len(addr) > 4 && (addr[2] == ':' || addr[3] == ':' || addr[4] == ':'))) {
		return "[" + addr + "]"
	}
	for _, r := range addr {
		if r == ':' {
			return "[" + addr + "]"
		}
	}
	return addr
}

func TestEveryBlockedTargetIsRefusedWhenTheChannelIsSavedAndNothingConnects(t *testing.T) {
	w := newWorld(t)
	for name, addr := range blockedAddresses() {
		_, res := w.create(w.owner, w.projectPath("/notification-channels"), map[string]any{
			"Kind": "webhook", "Name": "t-" + name, "Url": "https://" + literal(addr) + "/hook", "CurrentFactor": w.factor(),
		})
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status, "%s: %s", name, res.Raw)
		require.NotNil(t, res.Error, name)
		assert.Equal(t, "CHANNEL_HOST_BLOCKED", res.Error.Code, name)
	}
	for name, host := range map[string]string{
		"decimal": "2130706433", "hex": "0x7f000001", "dotted hex": "0x7f.0.0.1", "short": "127.1", "octal": "0177.0.0.1",
	} {
		_, res := w.create(w.owner, w.projectPath("/notification-channels"), map[string]any{
			"Kind": "webhook", "Name": "n-" + name, "Url": "https://" + host + "/hook", "CurrentFactor": w.factor(),
		})
		require.NotNil(t, res.Error, name)
		assert.Equal(t, "CHANNEL_HOST_NOT_ALLOWED", res.Error.Code, name)
	}
	for name, raw := range map[string]string{
		"plain http": "http://example.com/x", "user info": "https://user:pw@example.com/x", "other scheme": "ftp://example.com/x",
		"at sign trick": "https://example.com@127.0.0.1/x",
	} {
		_, res := w.create(w.owner, w.projectPath("/notification-channels"), map[string]any{
			"Kind": "webhook", "Name": "s-" + name, "Url": raw, "CurrentFactor": w.factor(),
		})
		require.NotNil(t, res.Error, name)
		assert.Equal(t, "CHANNEL_HOST_NOT_ALLOWED", res.Error.Code, name)
	}
	w.dns.set("metadata.internal.example", "169.254.169.254")
	w.dns.set("mixed.example", "203.0.113.9", "10.0.0.1", "192.168.0.1")
	for _, host := range []string{"metadata.internal.example", "mixed.example"} {
		_, res := w.create(w.owner, w.projectPath("/notification-channels"), map[string]any{
			"Kind": "webhook", "Name": "d-" + host, "Url": "https://" + host + "/hook", "CurrentFactor": w.factor(),
		})
		require.NotNil(t, res.Error, host)
		assert.Equal(t, "CHANNEL_HOST_BLOCKED", res.Error.Code, host+" resolves to a blocked address")
	}
	assert.Zero(t, w.dials.Load(), "no connection was attempted")
	assert.Zero(t, w.count(`SELECT count(*) FROM "NotificationChannels"`))
}

func TestAnAddressAllowedByTheOperatorIsAcceptedAndOthersStayBlocked(t *testing.T) {
	w := newWorld(t)
	w.dns.set("inside.example", "10.1.2.3")
	w.dns.set("outside.example", "192.168.1.1")
	created := w.webhook("inside", "https://inside.example/hook")
	assert.Equal(t, "inside.example", created.Channel.Host)
	_, res := w.create(w.owner, w.projectPath("/notification-channels"), map[string]any{
		"Kind": "webhook", "Name": "outside", "Url": "https://outside.example/hook", "CurrentFactor": w.factor(),
	})
	require.NotNil(t, res.Error)
	assert.Equal(t, "CHANNEL_HOST_BLOCKED", res.Error.Code)
}

func TestADnsAnswerThatTurnsPrivateAfterTheSaveIsRefusedAtSendTimeWithoutAConnection(t *testing.T) {
	for name, addr := range blockedAddresses() {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			w.dns.set("rebind.example", publicAddr)
			w.webhook("rebind", "https://rebind.example/hook")
			w.dns.set("rebind.example", addr)
			w.deploymentEvent(eventbus.DeploymentFinished, "failed")
			w.drain()

			assert.Zero(t, w.dials.Load(), "the address is checked again when Dhow connects, so nothing was dialed")
			assert.Empty(t, w.recv.requests())
			list := w.deliveries(w.projectPath("/notification-deliveries"))
			require.Len(t, list, 1)
			assert.Equal(t, "dead", list[0].State)
			assert.Equal(t, "blocked", list[0].LastError)
			assert.Equal(t, 1, list[0].Attempts, "a blocked address is final")
		})
	}
}

func TestAMixedDnsAnswerIsRefusedAsAWhole(t *testing.T) {
	w := newWorld(t)
	w.dns.set("mixed.example", publicAddr)
	w.webhook("mixed", "https://mixed.example/hook")
	w.dns.set("mixed.example", publicAddr, "169.254.169.254")
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	assert.Zero(t, w.dials.Load())
}

func TestOnlyAMaintainerOrAboveCanManageChannelsAndEveryoneElseSeesNoSecret(t *testing.T) {
	w := newWorld(t)
	created := w.webhook("ops", hookURL)
	maintainer, _ := w.env.SignedIn(w.org, "maint@example.com", "maintainer")
	developer, _ := w.env.SignedIn(w.org, "dev@example.com", "developer")
	viewer, _ := w.env.SignedIn(w.org, "view@example.com", "viewer")

	for name, c := range map[string]*apptest.Client{"developer": developer, "viewer": viewer} {
		res := c.Do(apptest.Request{Method: http.MethodPost, Path: w.projectPath("/notification-channels"), Body: map[string]any{
			"Kind": "webhook", "Name": "x-" + name, "Url": hookURL, "CurrentFactor": apptest.PasswordFactor("a long password for " + name + "@example.com"),
		}})
		assert.Equal(t, http.StatusForbidden, res.Status, name)
		list := c.Get(w.projectPath("/notification-channels"))
		assert.Equal(t, http.StatusOK, list.Status, name+" reads the list")
		assert.NotContains(t, string(list.Raw), "SECRETPATHTOKEN")
		assert.NotContains(t, string(list.Raw), created.Secret)
		assert.Equal(t, http.StatusForbidden, c.Get(w.projectPath("/notification-deliveries")).Status, name)
		assert.Equal(t, http.StatusForbidden, c.Do(apptest.Request{Method: http.MethodPost, Path: w.projectPath("/notification-channels/" + created.Channel.Id + "/test")}).Status)
	}
	res := maintainer.Do(apptest.Request{Method: http.MethodPost, Path: w.projectPath("/notification-channels"), Body: map[string]any{
		"Kind": "webhook", "Name": "by-maintainer", "Url": hookURL, "CurrentFactor": apptest.PasswordFactor("a long password for maint@example.com"),
	}})
	assert.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	orgRes := maintainer.Do(apptest.Request{Method: http.MethodPost, Path: w.orgPath("/notification-channels"), Body: map[string]any{
		"Kind": "webhook", "Name": "org-hook", "Url": hookURL, "CurrentFactor": apptest.PasswordFactor("a long password for maint@example.com"),
	}})
	assert.Equal(t, http.StatusForbidden, orgRes.Status, "the channels of the organization need an Owner or an Admin")
	token := w.env.SeedAPIToken(w.ownerID, w.org, "org.read", "project.read", "project.manage")
	assert.Equal(t, http.StatusUnauthorized, w.env.Do(apptest.Request{
		Method: http.MethodGet, Path: w.projectPath("/notification-channels"), Headers: map[string]string{"Authorization": "Bearer " + token.Value},
	}).Status, "an API token never reaches the notification channels")
}

func TestAChannelNeedsTheCurrentFactorToChangeOrRemove(t *testing.T) {
	w := newWorld(t)
	created := w.webhook("ops", hookURL)
	wrong := apptest.PasswordFactor("not the password at all")
	path := w.projectPath("/notification-channels/" + created.Channel.Id)
	res := w.owner.Do(apptest.Request{Method: http.MethodPut, Path: path, Body: map[string]any{
		"Name": "renamed", "Locale": "ar", "Version": created.Channel.Version, "Url": "https://example.com/other", "CurrentFactor": wrong,
	}})
	assert.Equal(t, http.StatusUnauthorized, res.Status, string(res.Raw))
	res = w.owner.Do(apptest.Request{Method: http.MethodPost, Path: path + "/remove", Body: map[string]any{"CurrentFactor": wrong}})
	assert.Equal(t, http.StatusUnauthorized, res.Status, string(res.Raw))
	assert.Equal(t, 1, w.count(`SELECT count(*) FROM "NotificationChannels" WHERE NOT "IsDeleted" AND "Name" = 'ops'`))
}

func TestAChannelStopsWhenItsCreatorLosesTheRoleOrLeaves(t *testing.T) {
	w := newWorld(t)
	maintainer, maintainerID := w.env.SignedIn(w.org, "maint@example.com", "maintainer")
	var created createdView
	created, res := w.create(maintainer, w.projectPath("/notification-channels"), map[string]any{
		"Kind": "webhook", "Name": "by-maintainer", "Url": hookURL, "CurrentFactor": apptest.PasswordFactor("a long password for maint@example.com"),
	})
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))

	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	require.Len(t, w.recv.requests(), 1, "while the creator may manage channels, the channel works")

	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "Memberships" SET "Role" = 'viewer' WHERE "UserId" = $1`, maintainerID)
	require.NoError(t, err)
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	assert.Len(t, w.recv.requests(), 1, "a creator who lost the role sends nothing more")
	got := w.owner.Get(w.projectPath("/notification-channels/" + created.Channel.Id))
	require.Equal(t, http.StatusOK, got.Status)
	var view channelView
	got.Decode(t, &view)
	assert.False(t, view.Enabled)
	assert.Equal(t, "creator-removed", view.DisabledReason)
	assert.Equal(t, 1, w.env.AuditCount("notification.channel_disabled"))

	enabled := w.owner.Do(apptest.Request{Method: http.MethodPut, Path: w.projectPath("/notification-channels/" + created.Channel.Id), Body: map[string]any{
		"Name": "by-maintainer", "Locale": "ar", "Version": view.Version, "Enable": true, "CurrentFactor": w.factor(),
	}})
	require.Equal(t, http.StatusOK, enabled.Status, string(enabled.Raw))
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	assert.Len(t, w.recv.requests(), 2, "the owner took the channel over and turned it on")
}

func TestARemovedMemberStopsTheirChannelsAtOnce(t *testing.T) {
	w := newWorld(t)
	maintainer, maintainerID := w.env.SignedIn(w.org, "maint@example.com", "maintainer")
	_, res := w.create(maintainer, w.projectPath("/notification-channels"), map[string]any{
		"Kind": "webhook", "Name": "by-maintainer", "Url": hookURL, "CurrentFactor": apptest.PasswordFactor("a long password for maint@example.com"),
	})
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	w.publish(eventbus.Event{Name: eventbus.IdentityMemberRemoved, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{w.org}, ActorUserID: &w.ownerID, TargetType: "user", TargetID: maintainerID.String(),
	}})
	assert.Zero(t, w.count(`SELECT count(*) FROM "NotificationChannels" WHERE "Enabled"`))
	assert.Equal(t, 1, w.env.AuditCount("notification.channel_disabled"))
}

func TestTenFailuresInARowStopTheChannelAndTellTheCreator(t *testing.T) {
	w := newWorld(t, 500)
	created := w.webhook("flaky", hookURL)
	for range 10 {
		w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	}
	for range 10 {
		w.drain()
		w.env.Clock.Advance(0)
	}
	var view channelView
	res := w.owner.Get(w.projectPath("/notification-channels/" + created.Channel.Id))
	if res.Status == http.StatusUnauthorized {
		w.relogin()
		res = w.owner.Get(w.projectPath("/notification-channels/" + created.Channel.Id))
	}
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	res.Decode(t, &view)
	assert.False(t, view.Enabled)
	assert.Equal(t, "failures", view.DisabledReason)
	assert.Len(t, w.recv.requests(), 10, "the breaker opened after the tenth failure")
	w.drain()
	sent := w.env.SentMail(w.mail)
	require.NotEmpty(t, sent)
	assert.Contains(t, sent[len(sent)-1].Subject, "flaky")
	assert.NotContains(t, sent[len(sent)-1].Body, "SECRETPATHTOKEN")
	assert.Equal(t, 1, w.env.AuditCount("notification.channel_disabled"))
}
