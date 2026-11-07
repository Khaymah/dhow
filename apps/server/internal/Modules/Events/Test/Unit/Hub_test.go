package unit_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	events "github.com/Khaymah/dhow/apps/server/internal/Modules/Events"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Events/Models"
)

var base = time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)

func clock() time.Time { return base }

func notice(org uuid.UUID, typ, status string) models.Notice {
	return models.Notice{Organization: org, Type: typ, Status: status, At: base}
}

func TestTheHubNumbersEventsAndKeepsThemPerOrganization(t *testing.T) {
	hub := events.NewHub(8, clock)
	a, b := uuid.New(), uuid.New()
	hub.Append(notice(a, models.TypeDeploymentStarted, "running"))
	hub.Append(notice(b, models.TypeDeploymentStarted, "running"))
	hub.Append(notice(a, models.TypeDeploymentFinished, "succeeded"))

	got := hub.Since(a, events.Position{Epoch: hub.Now().Epoch}, 10)
	require.False(t, got.Lost)
	require.Len(t, got.Events(), 2)
	assert.Equal(t, uint64(1), got.Events()[0].Seq)
	assert.Equal(t, uint64(3), got.Events()[1].Seq)
	assert.Equal(t, uint64(3), got.Cursor.Seq)

	other := hub.Since(b, events.Position{Epoch: hub.Now().Epoch}, 10)
	require.Len(t, other.Events(), 1)
	assert.Equal(t, models.TypeDeploymentStarted, other.Events()[0].Type)
}

func TestTheHubResumesAfterAKnownPositionAndNotBefore(t *testing.T) {
	hub := events.NewHub(8, clock)
	org := uuid.New()
	for range 5 {
		hub.Append(notice(org, models.TypeOverviewChanged, ""))
	}
	epoch := hub.Now().Epoch
	assert.True(t, hub.Resumable(events.Position{Epoch: epoch, Seq: 3}))
	assert.True(t, hub.Resumable(events.Position{Epoch: epoch, Seq: 5}))
	assert.False(t, hub.Resumable(events.Position{Epoch: epoch, Seq: 6}), "an id from the future belongs to another start")
	assert.False(t, hub.Resumable(events.Position{Epoch: epoch + 1, Seq: 3}), "another epoch cannot be resumed")

	got := hub.Since(org, events.Position{Epoch: epoch, Seq: 3}, 10)
	require.Len(t, got.Events(), 2)
	assert.Equal(t, uint64(4), got.Events()[0].Seq)
}

func TestAConsumerThatFellOffTheRingIsToldItLostEvents(t *testing.T) {
	hub := events.NewHub(4, clock)
	org := uuid.New()
	for range 9 {
		hub.Append(notice(org, models.TypeOverviewChanged, ""))
	}
	epoch := hub.Now().Epoch
	assert.False(t, hub.Resumable(events.Position{Epoch: epoch, Seq: 2}))
	assert.True(t, hub.Resumable(events.Position{Epoch: epoch, Seq: 5}), "the ring still holds 6 to 9")
	assert.True(t, hub.Since(org, events.Position{Epoch: epoch, Seq: 2}, 10).Lost)
	assert.False(t, hub.Since(org, events.Position{Epoch: epoch, Seq: 5}, 10).Lost)
}

func TestARestartMakesEveryOlderPositionLost(t *testing.T) {
	hub := events.NewHub(4, clock)
	org := uuid.New()
	hub.Append(notice(org, models.TypeOverviewChanged, ""))
	before := hub.Now()
	hub.Restart()
	assert.NotEqual(t, before.Epoch, hub.Now().Epoch)
	assert.True(t, hub.Since(org, before, 10).Lost)
}

func TestABatchStopsAtTheLimitAndSaysThereIsMore(t *testing.T) {
	hub := events.NewHub(16, clock)
	org := uuid.New()
	for range 5 {
		hub.Append(notice(org, models.TypeOverviewChanged, ""))
	}
	first := hub.Since(org, events.Position{Epoch: hub.Now().Epoch}, 2)
	require.Len(t, first.Events(), 2)
	assert.True(t, first.More)
	assert.Equal(t, uint64(2), first.Cursor.Seq)
	rest := hub.Since(org, first.Cursor, 10)
	require.Len(t, rest.Events(), 3)
	assert.False(t, rest.More)
}

func TestEveryEventTypeNeedsTheActionItsReaderMustHave(t *testing.T) {
	assert.Equal(t, "deployment.approve", string(events.ActionOf(models.TypeApprovalRequested)))
	assert.Equal(t, "run.read", string(events.ActionOf(models.TypeApprovalDecided)))
	assert.Equal(t, "run.read", string(events.ActionOf(models.TypeReleaseRollbackFail)))
	assert.Equal(t, "project.read", string(events.ActionOf(models.TypeTargetChecked)))
	assert.Equal(t, "org.read", string(events.ActionOf(models.TypeOverviewChanged)))
}

func TestTheEventIDIsTheEpochAndTheSequence(t *testing.T) {
	assert.Equal(t, "1731000000-42", events.Position{Epoch: 1731000000, Seq: 42}.ID())
	p, err := events.ParseLastEventID("1731000000-42")
	require.NoError(t, err)
	assert.Equal(t, events.Position{Epoch: 1731000000, Seq: 42}, *p)
	none, err := events.ParseLastEventID("")
	require.NoError(t, err)
	assert.Nil(t, none)
	for _, bad := range []string{"42", "a-1", "1-b", "-1-2", "1-", "1--2"} {
		_, err := events.ParseLastEventID(bad)
		assert.Error(t, err, bad)
	}
}
