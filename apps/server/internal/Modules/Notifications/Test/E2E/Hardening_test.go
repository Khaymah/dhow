package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

func TestARemovedChannelKeepsNoSecretInItsRow(t *testing.T) {
	w := newWorld(t)
	created := w.webhook("ops", hookURL)
	id := uuid.MustParse(created.Channel.Id)
	assert.Equal(t, 1, w.count(`SELECT count(*) FROM "NotificationChannels" WHERE "Id" = $1 AND "ConfigSealed" IS NOT NULL`, id))

	res := w.owner.Do(apptest.Request{
		Method: http.MethodPost, Path: w.projectPath("/notification-channels/" + created.Channel.Id + "/remove"),
		Body: map[string]any{"CurrentFactor": w.factor()},
	})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	assert.Equal(t, 1, w.count(`SELECT count(*) FROM "NotificationChannels" WHERE "Id" = $1 AND "IsDeleted" AND "ConfigSealed" IS NULL`, id),
		"the sealed address and secret are erased with the removal")
	assert.Equal(t, 0, w.count(`SELECT count(*) FROM "NotificationChannels" WHERE "IsDeleted" AND "ConfigSealed" IS NOT NULL`))
}

func TestTheFactorIsAskedBeforeAnyNameIsResolved(t *testing.T) {
	w := newWorld(t)
	w.dns.set("internal.example", "192.168.1.10")
	wrong := apptest.PasswordFactor("not the password at all")
	for _, host := range []string{"internal.example", "no-such-name.example"} {
		_, res := w.create(w.owner, w.projectPath("/notification-channels"), map[string]any{
			"Kind": "webhook", "Name": "w-" + host, "Url": "https://" + host + "/hook", "CurrentFactor": wrong,
		})
		require.NotNil(t, res.Error, host)
		assert.Equal(t, http.StatusForbidden, res.Status, host)
		assert.Equal(t, "INVALID_CURRENT_FACTOR", res.Error.Code, "%s: a caller who cannot prove a factor learns nothing about the network", host)
	}
}

func TestANameThatDoesNotResolveAndANameThatResolvesToAPrivateAddressAnswerAlike(t *testing.T) {
	w := newWorld(t)
	w.dns.set("internal.example", "192.168.1.10")
	var answers []string
	for _, host := range []string{"internal.example", "no-such-name.example"} {
		_, res := w.create(w.owner, w.projectPath("/notification-channels"), map[string]any{
			"Kind": "webhook", "Name": "a-" + host, "Url": "https://" + host + "/hook", "CurrentFactor": w.factor(),
		})
		require.NotNil(t, res.Error, host)
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status, host)
		answers = append(answers, res.Error.Code)
	}
	assert.Equal(t, "CHANNEL_HOST_BLOCKED", answers[0])
	assert.Equal(t, answers[0], answers[1], "the answer does not say which internal names exist")
}

func TestNewDeviceMailOfOneUserStopsAtItsCap(t *testing.T) {
	w := newWorld(t)
	require.Equal(t, http.StatusOK, w.signIn(apptest.UserAgent).Status)
	for range config.AccountMailMaxPending {
		_, err := w.env.Pool.Exec(context.Background(), `INSERT INTO "NotificationOutbox" ("Kind", "RecipientUserId", "EventId", "EventName", "Payload")
			VALUES ('internal-email', $1, $2, 'account.new_device_login', '{"Subject":"s","Body":"b"}')`, w.ownerID, uuid.New())
		require.NoError(t, err)
	}
	require.Equal(t, http.StatusOK, w.signIn(firefox).Status)
	assert.Equal(t, config.AccountMailMaxPending,
		w.count(`SELECT count(*) FROM "NotificationOutbox" WHERE "RecipientUserId" = $1 AND "EventName" = 'account.new_device_login'`, w.ownerID),
		"a ninth new-device mail is not queued while eight wait")
}
