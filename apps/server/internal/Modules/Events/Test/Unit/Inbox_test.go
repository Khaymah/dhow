package unit_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	events "github.com/Khaymah/dhow/apps/server/internal/Modules/Events"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Events/Models"
)

func (r *rig) neighbour() *rig {
	other := &rig{t: r.t, service: r.service, org: r.org, user: uuid.New(), who: r.who}
	other.srv = httptest.NewServer(http.HandlerFunc(other.handle))
	r.t.Cleanup(other.srv.Close)
	return other
}

func (r *rig) inbox(user *uuid.UUID) {
	r.service.Hub().Append(models.Notice{Organization: r.org, Type: models.TypeInboxChanged, User: user, At: base})
}

func TestAnInboxSignalReachesOnlyItsRecipient(t *testing.T) {
	r := newRig(t, events.Options{})
	other := r.neighbour()
	mine, theirs := r.open(""), other.open("")

	recipient := r.user
	r.inbox(&recipient)

	got := mine.next(t)
	assert.Equal(t, "change", got.Name)
	assert.Equal(t, models.TypeInboxChanged, decode(t, got).Type)
	theirs.silent(t, 300*time.Millisecond)

	r.publish(models.TypeOverviewChanged, "", nil)
	assert.Equal(t, models.TypeOverviewChanged, decode(t, theirs.next(t)).Type, "the other member still hears organization events")
}

func TestAnInboxSignalIsNeverFoundOnTheWireWithTheRecipient(t *testing.T) {
	r := newRig(t, events.Options{})
	c := r.open("")
	recipient := r.user
	r.inbox(&recipient)

	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(c.next(t).Data), &raw))
	assert.NotContains(t, raw, "User")
	for key := range raw {
		assert.Contains(t, []string{"Seq", "Type", "At"}, key)
	}
}

func TestAnInboxSignalIsNotReplayedToAnotherUserOnResume(t *testing.T) {
	r := newRig(t, events.Options{})
	other := r.neighbour()
	epoch := r.service.Hub().Now().Epoch
	recipient := r.user
	r.inbox(&recipient)

	theirs := other.open(events.Position{Epoch: epoch, Seq: 0}.ID())
	theirs.silent(t, 300*time.Millisecond)

	mine := r.open(events.Position{Epoch: epoch, Seq: 0}.ID())
	assert.Equal(t, models.TypeInboxChanged, decode(t, mine.next(t)).Type)
}

func TestAnInboxSignalWithoutAUserIsDropped(t *testing.T) {
	hub := events.NewHub(8)
	org := uuid.New()
	hub.Append(models.Notice{Organization: org, Type: models.TypeInboxChanged, At: base})
	got := hub.Since(org, events.Position{Epoch: hub.Now().Epoch}, 10)
	assert.Empty(t, got.Entries, "a signal that names no user is dropped, never sent to everyone")
}
