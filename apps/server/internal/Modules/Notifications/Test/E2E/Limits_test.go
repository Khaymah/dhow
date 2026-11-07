package e2e_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
)

func (w *world) testPath(channel string) string {
	return w.projectPath("/notification-channels/" + channel + "/test")
}

func (w *world) emailChannelBy(c *apptest.Client, password, name string) string {
	w.t.Helper()
	out, res := w.create(c, w.projectPath("/notification-channels"), map[string]any{
		"Kind": "email", "Name": name, "CurrentFactor": apptest.PasswordFactor(password),
	})
	require.Equal(w.t, http.StatusCreated, res.Status, string(res.Raw))
	return out.Channel.Id
}

func TestTestMessagesAreLimitedForEachUserAndForEachOrganizationAcrossChannels(t *testing.T) {
	w := newWorld(t)
	admin, _ := w.env.SignedIn(w.org, "admin@example.com", "admin")
	maintainer, _ := w.env.SignedIn(w.org, "maint@example.com", "maintainer")
	var channels []string
	for i := range 6 {
		channels = append(channels, w.emailChannelBy(w.owner, w.password, "c"+strconv.Itoa(i)))
	}
	send := func(c *apptest.Client, channel string) apptest.Response {
		return c.Do(apptest.Request{Method: http.MethodPost, Path: w.testPath(channel)})
	}
	for i := range config.NotifyTestPerUser {
		res := send(w.owner, channels[i/config.NotifyTestPerMinute])
		require.Equal(t, http.StatusAccepted, res.Status, "owner test %d: %s", i, res.Raw)
	}
	limited := send(w.owner, channels[2])
	assert.Equal(t, http.StatusTooManyRequests, limited.Status, "the owner used the share of one user, whatever the channel")
	require.NotNil(t, limited.Error)
	assert.Equal(t, "TEST_RATE_LIMITED", limited.Error.Code)

	for i := range config.NotifyTestPerUser {
		require.Equal(t, http.StatusAccepted, send(admin, channels[3+i/config.NotifyTestPerMinute]).Status, "admin test %d", i)
	}
	third := send(maintainer, channels[5])
	assert.Equal(t, http.StatusTooManyRequests, third.Status, "the organization used its share, so even a user who sent nothing is held")
}

func (w *world) members(count int) {
	w.t.Helper()
	_, err := w.env.Pool.Exec(context.Background(), `
		WITH u AS (
		  INSERT INTO "Users" ("Email", "Name", "PasswordHash")
		  SELECT 'bulk' || g || '@example.com', 'Bulk ' || g, 'x' FROM generate_series(1, $2::int) g RETURNING "Id")
		INSERT INTO "Memberships" ("UserId", "OrganizationId", "Role") SELECT "Id", $1, 'maintainer' FROM u`, w.org, count)
	require.NoError(w.t, err)
}

func TestAMemberWithoutAccessToTheProjectGetsNoEmail(t *testing.T) {
	w := newWorld(t)
	w.emailChannelBy(w.owner, w.password, "team")
	otherOrg := w.env.SeedOrganization("Other", "other-org")
	w.env.SeedUser(otherOrg, "stranger@example.com", "a long password for stranger", "owner")
	gone := w.env.SeedUser(w.org, "gone@example.com", "a long password for gone", "admin")
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "Memberships" SET "IsDeleted" = true WHERE "UserId" = $1`, gone)
	require.NoError(t, err)
	disabled := w.env.SeedUser(w.org, "off@example.com", "a long password for off", "admin")
	_, err = w.env.Pool.Exec(context.Background(), `UPDATE "Users" SET "Disabled" = true WHERE "Id" = $1`, disabled)
	require.NoError(t, err)
	w.env.SeedUser(w.org, "kept@example.com", "a long password for kept", "maintainer")

	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	assert.ElementsMatch(t, []string{w.email, "kept@example.com"}, recipients(w.env.SentMail(w.mail)),
		"only members who can read the project; not another organization, a removed member or a disabled account")
}

func TestOneEventMailsAtMostTheCapAndOneUserAtMostTheirHourlyShare(t *testing.T) {
	w := newWorld(t)
	w.emailChannelBy(w.owner, w.password, "team")
	w.members(config.NotifyRecipientsPerEvent + 20)
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	assert.Equal(t, config.NotifyRecipientsPerEvent, w.count(`SELECT count(*) FROM "NotificationOutbox" WHERE "Kind" = 'email'`),
		"a large organization cannot turn one event into a flood")

	w2 := newWorld(t)
	w2.emailChannelBy(w2.owner, w2.password, "team")
	for range config.NotifyEmailPerUserHour + 5 {
		w2.deploymentEvent(eventbus.DeploymentFinished, "failed")
	}
	assert.Equal(t, config.NotifyEmailPerUserHour, w2.count(`SELECT count(*) FROM "NotificationOutbox" WHERE "RecipientUserId" = $1`, w2.ownerID),
		"one person gets at most their hourly share across all events and channels")
}

func TestFailingMailForManyRecipientsDoesNotStopTheChannel(t *testing.T) {
	w := newWorld(t)
	channel := w.emailChannelBy(w.owner, w.password, "team")
	w.members(config.NotifyBreakerFailures + 5)
	w.mail.FailNext(true)
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	assert.Positive(t, w.count(`SELECT count(*) FROM "NotificationOutbox" WHERE "LastError" = 'mail_send'`), "the failures are in the delivery log")
	got := w.owner.Get(w.projectPath("/notification-channels/" + channel))
	require.Equal(t, http.StatusOK, got.Status)
	var view channelView
	got.Decode(t, &view)
	assert.True(t, view.Enabled, "a mail server that is down is not the fault of the channel")
	assert.Zero(t, view.ConsecutiveFailures)
	assert.Zero(t, w.env.AuditCount("notification.channel_disabled"))
	w.mail.FailNext(false)
	w.env.Clock.Advance(2 * time.Minute)
	w.drain()
	assert.Len(t, w.env.SentMail(w.mail), config.NotifyBreakerFailures+5+1, "everyone gets the mail once the server is back")
}

func TestReadersSeeNeitherTheHostNorTheCreatorOfAChannel(t *testing.T) {
	w := newWorld(t)
	hook := w.webhook("ops", hookURL)
	maintainer, _ := w.env.SignedIn(w.org, "maint@example.com", "maintainer")
	developer, _ := w.env.SignedIn(w.org, "dev@example.com", "developer")
	viewer, _ := w.env.SignedIn(w.org, "view@example.com", "viewer")
	orgHook, res := w.create(w.owner, w.orgPath("/notification-channels"), map[string]any{
		"Kind": "webhook", "Name": "org-ops", "Url": hookURL, "CurrentFactor": w.factor(),
	})
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))

	paths := []string{
		w.projectPath("/notification-channels"), w.projectPath("/notification-channels/" + hook.Channel.Id),
		w.orgPath("/notification-channels"), w.orgPath("/notification-channels/" + orgHook.Channel.Id),
	}
	for name, c := range map[string]*apptest.Client{"developer": developer, "viewer": viewer} {
		for _, p := range paths {
			got := c.Get(p)
			require.Equal(t, http.StatusOK, got.Status, name+" "+p)
			assert.NotContains(t, string(got.Raw), `"Host"`, name+" "+p)
			assert.NotContains(t, string(got.Raw), "CreatedByUserId", name+" "+p)
			assert.Contains(t, string(got.Raw), `"Name"`)
			assert.Contains(t, string(got.Raw), `"Enabled"`)
		}
	}
	managed := maintainer.Get(paths[1])
	assert.Contains(t, string(managed.Raw), `"Host":"example.com"`)
	assert.Contains(t, string(managed.Raw), w.ownerID.String())
	assert.Contains(t, string(w.owner.Get(paths[3]).Raw), `"Host":"example.com"`)
	assert.NotContains(t, string(maintainer.Get(paths[3]).Raw), `"Host"`, "a Maintainer manages project channels, not those of the organization")
}
