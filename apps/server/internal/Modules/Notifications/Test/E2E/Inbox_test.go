package e2e_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
)

type inboxView struct {
	Id        string
	Event     string
	TitleKey  string
	Params    map[string]string
	Link      string
	Read      bool
	CreatedAt time.Time
}

func (w *world) inappChannel(name string) createdView {
	w.t.Helper()
	out, res := w.create(w.owner, w.projectPath("/notification-channels"), map[string]any{
		"Kind": "inapp", "Name": name, "Locale": "en", "CurrentFactor": w.factor(),
	})
	require.Equal(w.t, http.StatusCreated, res.Status, string(res.Raw))
	return out
}

func inbox(t *testing.T, c *apptest.Client, query string) ([]inboxView, int) {
	t.Helper()
	res := c.Get("/me/notifications" + query)
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var items []inboxView
	res.Decode(t, &items)
	require.NotNil(t, res.Meta)
	return items, res.Meta.Total
}

func unread(t *testing.T, c *apptest.Client) int {
	t.Helper()
	res := c.Get("/me/notifications/unread-count")
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var data struct{ Unread int }
	res.Decode(t, &data)
	return data.Unread
}

func TestAnInAppChannelFillsTheInboxOfTheMembersOfTheChosenRolesWithoutAnyAddressOrSecret(t *testing.T) {
	w := newWorld(t)
	admin, _ := w.env.SignedIn(w.org, "admin@example.com", "admin")
	dev, _ := w.env.SignedIn(w.org, "dev@example.com", "developer")
	created := w.inappChannel("inbox")
	assert.False(t, created.Channel.HasSecret)
	assert.Empty(t, created.Secret)

	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()

	items, total := inbox(t, admin, "")
	require.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "deployment.finished", items[0].Event)
	assert.Equal(t, "inbox.deployment.finished.failed", items[0].TitleKey)
	assert.Equal(t, "production", items[0].Params["environment"])
	assert.Equal(t, "/projects/"+w.project.String()+"/runs/1", items[0].Link)
	assert.False(t, items[0].Read)
	for _, v := range items[0].Params {
		assert.NotContains(t, v, apptest.PublicOrigin)
	}
	ownerItems, _ := inbox(t, w.owner, "")
	assert.Len(t, ownerItems, 1)
	devItems, devTotal := inbox(t, dev, "")
	assert.Empty(t, devItems, "the developer is not on the default roles")
	assert.Zero(t, devTotal)
	assert.Empty(t, w.recv.requests())
}

func TestAMutedMemberStillGetsTheInboxNotification(t *testing.T) {
	w := newWorld(t)
	admin, _ := w.env.SignedIn(w.org, "admin@example.com", "admin")
	w.inappChannel("inbox")
	res := admin.Do(apptest.Request{Method: http.MethodPut, Path: "/me/notification-mute", Body: map[string]any{"Muted": true}})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))

	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	assert.Equal(t, 1, unread(t, admin))
}

func TestAnInAppChannelRefusesAnAddressAndATokenBecauseItHasNoSecret(t *testing.T) {
	w := newWorld(t)
	for name, extra := range map[string]map[string]any{
		"address": {"Url": hookURL},
		"token":   {"BotToken": "123456:abcdefghijklmnopqrstuvwxyz0123456789", "ChatId": "42"},
	} {
		body := map[string]any{"Kind": "inapp", "Name": "bad-" + name, "CurrentFactor": w.factor()}
		for k, v := range extra {
			body[k] = v
		}
		_, res := w.create(w.owner, w.projectPath("/notification-channels"), body)
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status, name+": "+string(res.Raw))
	}
}

func TestReadingMarkingAndCountingTheInboxStaysWithTheOwnerOfTheNotification(t *testing.T) {
	w := newWorld(t)
	admin, _ := w.env.SignedIn(w.org, "admin@example.com", "admin")
	w.inappChannel("inbox")
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.deploymentEvent(eventbus.DeploymentFinished, "rolled_back")
	w.drain()

	items, _ := inbox(t, admin, "")
	require.Len(t, items, 2)
	assert.Equal(t, 2, unread(t, admin))

	foreign := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: "/me/notifications/" + items[0].Id + "/read"})
	assert.Equal(t, http.StatusNotFound, foreign.Status, "a notification of another user is not found")
	assert.Equal(t, 2, unread(t, admin), "marking the id of another user changes nothing for this user")

	read := admin.Do(apptest.Request{Method: http.MethodPost, Path: "/me/notifications/" + items[0].Id + "/read"})
	require.Equal(t, http.StatusOK, read.Status, string(read.Raw))
	assert.Equal(t, 1, unread(t, admin))
	onlyUnread, total := inbox(t, admin, "?Unread=true")
	assert.Equal(t, 1, total)
	require.Len(t, onlyUnread, 1)
	assert.Equal(t, items[1].Id, onlyUnread[0].Id)

	missing := admin.Do(apptest.Request{Method: http.MethodPost, Path: "/me/notifications/" + uuid.NewString() + "/read"})
	assert.Equal(t, http.StatusNotFound, missing.Status)

	all := admin.Do(apptest.Request{Method: http.MethodPost, Path: "/me/notifications/read-all"})
	require.Equal(t, http.StatusOK, all.Status, string(all.Raw))
	assert.Zero(t, unread(t, admin))
	assert.Equal(t, 2, unread(t, w.owner), "the owner still has both unread")
}

func TestTheInboxIsNotReadableWithoutASession(t *testing.T) {
	w := newWorld(t)
	for _, path := range []string{"/me/notifications", "/me/notifications/unread-count"} {
		res := w.env.Do(apptest.Request{Method: http.MethodGet, Path: path})
		assert.Equal(t, http.StatusUnauthorized, res.Status, path)
	}
	res := w.env.Do(apptest.Request{Method: http.MethodPost, Path: "/me/notifications/read-all"})
	assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, res.Status)
}

func TestTheTestMessageOfAnInAppChannelLandsInTheInboxOfTheCaller(t *testing.T) {
	w := newWorld(t)
	created := w.inappChannel("inbox")
	res := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: w.projectPath("/notification-channels/" + created.Channel.Id + "/test")})
	require.Equal(t, http.StatusAccepted, res.Status, string(res.Raw))
	w.drain()
	items, _ := inbox(t, w.owner, "")
	require.Len(t, items, 1)
	assert.Equal(t, "inbox.notification.test", items[0].TitleKey)
}

func TestAccountEventsLandInTheInboxOfTheMemberWithoutAnyAddressOrToken(t *testing.T) {
	w := newWorld(t)
	require.Equal(t, http.StatusOK, w.signIn(apptest.UserAgent).Status)
	require.Equal(t, http.StatusOK, w.signIn(firefox).Status)
	w.drain()

	items, _ := inbox(t, w.owner, "")
	require.Len(t, items, 1)
	assert.Equal(t, "inbox.account.new_device_login.self", items[0].TitleKey)
	assert.Equal(t, "Firefox · Linux", items[0].Params["browser"])
	assert.Equal(t, "/?stack=account", items[0].Link)
	raw := items[0].TitleKey + items[0].Link
	for k, v := range items[0].Params {
		raw += k + v
	}
	for _, forbidden := range []string{"203.0.113", w.password, "Mozilla", "Cookie", "CsrfToken", "network"} {
		assert.NotContains(t, raw, forbidden)
	}
}

func TestARoleChangeLandsInTheInboxOfTheMemberWhoseRoleChanged(t *testing.T) {
	w := newWorld(t)
	admin, _ := w.env.SignedIn(w.org, "admin@example.com", "admin")
	w.publish(eventbus.Event{Name: eventbus.IdentityMemberRoleChanged, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{w.org}, ActorUserID: &w.ownerID, TargetType: "user", TargetID: admin.UserID,
		Data: map[string]string{"From": "admin", "To": "maintainer"},
	}})
	items, _ := inbox(t, admin, "")
	require.Len(t, items, 1)
	assert.Equal(t, "inbox.member.role_changed.self", items[0].TitleKey)
	assert.Equal(t, "maintainer", items[0].Params["reason"])
	other, _ := inbox(t, w.owner, "")
	assert.Empty(t, other)
}

func TestTheInboxKeepsOnlyTheNewestNotificationsOfAUserAndDropsTheOldOnes(t *testing.T) {
	w := newWorld(t)
	w.inappChannel("inbox")
	for range config.NotifyInboxPerUser + 3 {
		w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	}
	w.drain()
	_, total := inbox(t, w.owner, "?Limit=1")
	assert.Equal(t, config.NotifyInboxPerUser, total)

	old := time.Now().AddDate(0, 0, -(config.NotifyRetentionDefault + 1))
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "NotificationInbox" SET "CreatedAt" = $1 WHERE "UserId" = $2`, old, w.ownerID)
	require.NoError(t, err)
	require.NoError(t, w.env.Wired.Notifications.PurgeInbox(context.Background()))
	_, total = inbox(t, w.owner, "")
	assert.Zero(t, total)
	assert.Positive(t, w.count(`SELECT count(*) FROM "NotificationInbox" WHERE "IsDeleted"`), "dropped rows are kept as deleted, never removed")
}

func TestAMemberWhoLeftTheOrganizationNoLongerSeesItsNotifications(t *testing.T) {
	w := newWorld(t)
	admin, _ := w.env.SignedIn(w.org, "admin@example.com", "admin")
	w.inappChannel("inbox")
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	assert.Equal(t, 1, unread(t, admin))

	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "Memberships" SET "IsDeleted" = true WHERE "UserId" = $1`, admin.UserID)
	require.NoError(t, err)
	assert.Zero(t, unread(t, admin))
	assert.False(t, strings.Contains(string(admin.Get("/me/notifications").Raw), "inbox.deployment"))
}
