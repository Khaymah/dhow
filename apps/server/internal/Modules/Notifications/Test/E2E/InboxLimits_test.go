package e2e_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
)

func TestFloodingDeployNotificationsNeverEvictAnAccountNotification(t *testing.T) {
	w := newWorld(t)
	w.inappChannel("inbox")
	require.Equal(t, http.StatusOK, w.signIn(apptest.UserAgent).Status)
	require.Equal(t, http.StatusOK, w.signIn(firefox).Status)
	w.drain()
	for range config.NotifyInboxPerUser + 50 {
		w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	}
	w.drain()

	_, total := inbox(t, w.owner, "?Limit=1")
	assert.Equal(t, config.NotifyInboxPerUser+1, total, "the deploy cap and the account cap are separate")
	assert.Equal(t, 1, w.count(
		`SELECT count(*) FROM "NotificationInbox" WHERE "UserId" = $1 AND "IsAccount" AND NOT "IsDeleted" AND "TitleKey" = 'inbox.account.new_device_login.self'`,
		w.ownerID))
}

func TestARemovedMemberDoesNotSeeTheOldNotificationsAgainWhenAddedBack(t *testing.T) {
	w := newWorld(t)
	admin, _ := w.env.SignedIn(w.org, "admin@example.com", "admin")
	w.inappChannel("inbox")
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	require.Equal(t, 1, unread(t, admin))

	adminID, err := uuid.Parse(admin.UserID)
	require.NoError(t, err)
	w.publish(eventbus.Event{Name: eventbus.IdentityMemberRemoved, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{w.org}, ActorUserID: &w.ownerID, TargetType: "user", TargetID: admin.UserID,
	}})
	assert.Positive(t, w.count(`SELECT count(*) FROM "NotificationInbox" WHERE "UserId" = $1 AND "IsDeleted"`, adminID))
	assert.Zero(t, unread(t, admin), "the membership is still there, but the old rows are gone")
}

func TestLongMultibyteNamesFitTheInboxLimitAndAreStillDelivered(t *testing.T) {
	w := newWorld(t)
	w.inappChannel("inbox")
	emoji := strings.Repeat("😀", 100)
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "Projects" SET "Name" = $1 WHERE "Id" = $2`, emoji, w.project)
	require.NoError(t, err)
	_, err = w.env.Pool.Exec(context.Background(), `UPDATE "PipelineRuns" SET "Ref" = $1 WHERE "Id" = $2`, emoji, w.run)
	require.NoError(t, err)
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()

	items, _ := inbox(t, w.owner, "")
	require.Len(t, items, 1)
	for k, v := range items[0].Params {
		assert.LessOrEqual(t, len(v), 120, k)
		assert.True(t, utf8.ValidString(v), k)
	}
	assert.Zero(t, w.count(`SELECT count(*) FROM "NotificationOutbox" WHERE "Kind" = 'inapp' AND "State" <> 'sent'`))
}
