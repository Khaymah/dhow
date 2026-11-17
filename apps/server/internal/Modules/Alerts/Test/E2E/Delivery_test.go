package e2e_test

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	mailer "github.com/Khaymah/dhow/apps/server/internal/Core/Mail"
)

type heldMailer struct {
	release chan struct{}
	started sync.WaitGroup
	sent    atomic.Int32
}

func (h *heldMailer) Enabled() bool { return true }

func (h *heldMailer) Send(ctx context.Context, _ mailer.Message) error {
	h.started.Done()
	select {
	case <-h.release:
		h.sent.Add(1)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func passwordChanged(user uuid.UUID) eventbus.Event {
	return eventbus.Event{Name: eventbus.AuthPasswordChanged, Payload: eventbus.Audited{ActorUserID: &user, TargetType: "user", TargetID: user.String()}}
}

func ownerID(t *testing.T, env *apptest.Env) uuid.UUID {
	t.Helper()
	owner, _ := env.SetupAndLogin()
	require.Equal(t, http.StatusOK, owner.Get("/me").Status)
	return uuid.MustParse(owner.UserID)
}

func TestTheAlertIsSentEvenWhenTheRequestIsGone(t *testing.T) {
	mail := &apptest.FakeMailer{}
	env := apptest.NewWithOptions(t, apptest.Options{Mailer: mail})
	user := ownerID(t, env)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, env.Wired.Alerts.Handle(ctx, passwordChanged(user)))
	assert.Len(t, env.SentMail(mail), 1, "a client that drops the connection after the commit cannot skip the alert")
}

func fillMail(t *testing.T, env *apptest.Env, user uuid.UUID, event string, n int) {
	t.Helper()
	for range n {
		_, err := env.Pool.Exec(context.Background(), `INSERT INTO "NotificationOutbox" ("Kind", "RecipientUserId", "EventId", "EventName", "Payload")
			VALUES ('internal-email', $1, $2, $3, '{"Subject":"s","Body":"b"}')`, user, uuid.New(), event)
		require.NoError(t, err)
	}
}

func pendingOf(t *testing.T, env *apptest.Env, user uuid.UUID, like string) int {
	t.Helper()
	var n int
	require.NoError(t, env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "NotificationOutbox"
		WHERE "Kind" = 'internal-email' AND "RecipientUserId" = $1 AND "EventName" LIKE $2 AND "State" = 'pending' AND NOT "IsDeleted"`, user, like).Scan(&n))
	return n
}

func TestACredentialAlertIsQueuedWhateverElseIsPending(t *testing.T) {
	held := &heldMailer{release: make(chan struct{})}
	env := apptest.NewWithOptions(t, apptest.Options{Mailer: held})
	user := ownerID(t, env)

	fillMail(t, env, user, "account.new_device_login", 40)
	fillMail(t, env, user, "notification.channel_disabled", 40)
	before := pendingOf(t, env, user, "auth.%")
	require.NoError(t, env.Wired.Alerts.Handle(context.Background(), passwordChanged(user)))
	assert.Equal(t, before+1, pendingOf(t, env, user, "auth.%"), "80 other mails waiting do not stop a credential alert")
	assert.NotContains(t, env.Logs.String(), "credential alert not sent")
}

func TestPendingCredentialAlertsOfOneUserKeepTheNewestAndNeverDropTheLatest(t *testing.T) {
	held := &heldMailer{release: make(chan struct{})}
	env := apptest.NewWithOptions(t, apptest.Options{Mailer: held})
	user := ownerID(t, env)

	const limit = config.AlertMaxPendingPerUser
	held.started.Add(limit)
	for range limit + 5 {
		require.NoError(t, env.Wired.Alerts.Handle(context.Background(), passwordChanged(user)))
	}
	assert.Equal(t, limit, pendingOf(t, env, user, "auth.%"), "one user keeps the newest alerts, and the latest is always among them")
	assert.Contains(t, env.Logs.String(), "older pending credential alerts of the user were replaced by newer ones")
	close(held.release)
	env.Wired.Alerts.Wait()
	assert.Equal(t, int32(limit), held.sent.Load())
}

func TestNoAlertStartsAfterShutdown(t *testing.T) {
	mail := &apptest.FakeMailer{}
	env := apptest.NewWithOptions(t, apptest.Options{Mailer: mail})
	user := ownerID(t, env)
	env.Wired.Alerts.Close()

	require.NoError(t, env.Wired.Alerts.Handle(context.Background(), passwordChanged(user)))
	assert.Empty(t, env.SentMail(mail))
	assert.Contains(t, env.Logs.String(), "credential alert not sent: shutting down")
}
