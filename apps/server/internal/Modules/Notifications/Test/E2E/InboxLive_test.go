package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
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
