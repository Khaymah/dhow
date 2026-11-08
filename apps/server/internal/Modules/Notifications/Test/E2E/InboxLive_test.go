package e2e_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	events "github.com/Khaymah/dhow/apps/server/internal/Modules/Events"
	eventmodels "github.com/Khaymah/dhow/apps/server/internal/Modules/Events/Models"
)

const liveWait = 5 * time.Second

func inboxSignals(t *testing.T, s *apptest.Stream, within time.Duration) int {
	t.Helper()
	count := 0
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		ev, err := s.Next(time.Until(deadline))
		if err != nil {
			break
		}
		if ev.Name != "change" {
			continue
		}
		var got eventmodels.Event
		require.NoError(t, json.Unmarshal([]byte(ev.Data), &got))
		if got.Type == eventmodels.TypeInboxChanged {
			assert.NotContains(t, ev.Data, "User", "the signal carries no recipient")
			count++
		}
	}
	return count
}

func TestANewInboxNotificationSignalsOnlyItsOwnerOnTheLiveStream(t *testing.T) {
	w := newWorld(t)
	require.Eventually(t, w.env.Wired.Options.Events.Hub().Healthy, liveWait, 10*time.Millisecond)
	admin, _ := w.env.SignedIn(w.org, "admin@example.com", "admin")
	w.inappChannel("inbox")
	srv := w.env.Serve()
	path := config.APIPrefix + "/organizations/" + w.org.String() + "/events"
	mine := w.owner.OpenStream(t, srv, path, nil)
	theirs := admin.OpenStream(t, srv, path, nil)
	require.Equal(t, http.StatusOK, mine.Status, mine.Body)
	require.Equal(t, http.StatusOK, theirs.Status, theirs.Body)

	require.Equal(t, http.StatusOK, w.signIn(firefox).Status)
	w.drain()

	assert.Equal(t, 1, inboxSignals(t, mine, 1500*time.Millisecond), "the member whose account changed hears about it")
	assert.Zero(t, inboxSignals(t, theirs, 500*time.Millisecond), "another member never hears about it")
}

func TestReadingTheInboxSignalsOnlyTheReader(t *testing.T) {
	w := newWorld(t)
	require.Eventually(t, w.env.Wired.Options.Events.Hub().Healthy, liveWait, 10*time.Millisecond)
	admin, _ := w.env.SignedIn(w.org, "admin@example.com", "admin")
	w.inappChannel("inbox")
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	srv := w.env.Serve()
	path := config.APIPrefix + "/organizations/" + w.org.String() + "/events"
	mine := w.owner.OpenStream(t, srv, path, nil)
	theirs := admin.OpenStream(t, srv, path, nil)
	require.Equal(t, http.StatusOK, mine.Status, mine.Body)

	res := admin.Do(apptest.Request{Method: http.MethodPost, Path: "/me/notifications/read-all"})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))

	assert.Equal(t, 1, inboxSignals(t, theirs, 1500*time.Millisecond))
	assert.Zero(t, inboxSignals(t, mine, 500*time.Millisecond))
}

func inboxEntries(w *world, since events.Position) int {
	count := 0
	for _, ev := range w.env.Wired.Options.Events.Hub().Since(w.org, since, 1000).Events() {
		if ev.Type == eventmodels.TypeInboxChanged {
			count++
		}
	}
	return count
}

func TestMarkingAnItemThatIsAlreadyReadPublishesNothing(t *testing.T) {
	w := newWorld(t)
	require.Eventually(t, w.env.Wired.Options.Events.Hub().Healthy, liveWait, 10*time.Millisecond)
	admin, _ := w.env.SignedIn(w.org, "admin@example.com", "admin")
	w.inappChannel("inbox")
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	items, _ := inbox(t, admin, "")
	require.NotEmpty(t, items)
	hub := w.env.Wired.Options.Events.Hub()

	start := hub.Now()
	first := admin.Do(apptest.Request{Method: http.MethodPost, Path: "/me/notifications/" + items[0].Id + "/read"})
	require.Equal(t, http.StatusOK, first.Status, string(first.Raw))
	require.Eventually(t, func() bool { return inboxEntries(w, start) == 1 }, liveWait, 20*time.Millisecond)

	again := hub.Now()
	second := admin.Do(apptest.Request{Method: http.MethodPost, Path: "/me/notifications/" + items[0].Id + "/read"})
	require.Equal(t, http.StatusOK, second.Status, string(second.Raw))
	time.Sleep(500 * time.Millisecond)
	assert.Zero(t, inboxEntries(w, again), "an item that was already read signals nobody")

	all := hub.Now()
	for range 3 {
		res := admin.Do(apptest.Request{Method: http.MethodPost, Path: "/me/notifications/read-all"})
		require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	}
	time.Sleep(500 * time.Millisecond)
	assert.LessOrEqual(t, inboxEntries(w, all), 1, "read-all signals only when it changed a row")
}

func TestOneInboxInsertTakesOneSlotOfTheRing(t *testing.T) {
	w := newWorld(t)
	require.Eventually(t, w.env.Wired.Options.Events.Hub().Healthy, liveWait, 10*time.Millisecond)
	start := w.env.Wired.Options.Events.Hub().Now()
	require.Equal(t, http.StatusOK, w.signIn(firefox).Status)
	w.drain()
	require.Eventually(t, func() bool { return inboxEntries(w, start) >= 1 }, liveWait, 20*time.Millisecond)
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, 1, inboxEntries(w, start))
}

func TestTheInboxWritesAreLimitedPerUser(t *testing.T) {
	w := newWorld(t)
	var last apptest.Response
	for i := range 61 {
		last = w.owner.Do(apptest.Request{Method: http.MethodPost, Path: "/me/notifications/read-all"})
		if i < 60 {
			require.Equal(t, http.StatusOK, last.Status, strconv.Itoa(i))
		}
	}
	assert.Equal(t, http.StatusTooManyRequests, last.Status)
	require.NotNil(t, last.Error)
	assert.Equal(t, "RATE_LIMITED", last.Error.Code)
	read := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: "/me/notifications/" + uuid.NewString() + "/read"})
	assert.Equal(t, http.StatusTooManyRequests, read.Status)
}

func TestAFailingLiveSignalInsideTheEventTransactionRollsTheInboxBack(t *testing.T) {
	w := newWorld(t)
	admin, _ := w.env.SignedIn(w.org, "admin@example.com", "admin")
	w.env.Wired.Bus.Subscribe(eventbus.NotificationInboxChanged, func(context.Context, eventbus.Event) error {
		return errors.New("the signal cannot be sent")
	})
	err := dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
		return w.env.Wired.Notifications.Handle(context.Background(), eventbus.Event{Name: eventbus.IdentityMemberRoleChanged, Payload: eventbus.Audited{
			OrganizationIDs: []uuid.UUID{w.org}, ActorUserID: &w.ownerID, TargetType: "user", TargetID: admin.UserID,
			Data: map[string]string{"From": "admin", "To": "maintainer"}, Tx: q,
		}})
	})
	require.Error(t, err)
	items, _ := inbox(t, admin, "")
	assert.Empty(t, items, "the event failed as a whole, so no inbox row was kept")
}
