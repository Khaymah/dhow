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

func TestPendingAlertsAreCapped(t *testing.T) {
	held := &heldMailer{release: make(chan struct{})}
	env := apptest.NewWithOptions(t, apptest.Options{Mailer: held})
	user := ownerID(t, env)

	const limit = config.AlertMaxPending
	held.started.Add(limit)
	for range limit + 5 {
		require.NoError(t, env.Wired.Alerts.Handle(context.Background(), passwordChanged(user)))
	}
	assert.Contains(t, env.Logs.String(), "credential alert not sent: too many pending")
	var queued int
	require.NoError(t, env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "NotificationOutbox" WHERE "Kind" = 'internal-email'`).Scan(&queued))
	assert.Equal(t, limit, queued, "the queue holds no more than the cap, and nothing was sent yet")
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
