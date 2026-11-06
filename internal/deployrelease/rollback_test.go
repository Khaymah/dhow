package deployrelease_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

const (
	oldRelease = "1-aaaaaaaa-1"
	midRelease = "2-bbbbbbbb-1"
	newRelease = "3-cccccccc-1"
)

func threeReleases(t *testing.T) *rig {
	t.Helper()
	r := newRig(t, 3)
	r.deployAll(oldRelease, "v1", 1)
	r.deployAll(midRelease, "v2", 1)
	r.deployAll(newRelease, "v3", 1)
	return r
}

func (r *rig) rollBackTo(doer deployrelease.Doer, o rollout, withArchive bool) fleetOutcome {
	r.t.Helper()
	body := "v1"
	o.edit = func(c *deployrelease.Config) {
		c.Rollback = true
		if !withArchive {
			c.ArchivePath, c.ArchiveDigest = "", ""
		}
	}
	return r.roll(oldRelease, body, doer, o)
}

func TestAManualRollbackOnlySwitchesTheServersThatStillHaveTheReleaseAndChecksTheirHealth(t *testing.T) {
	r := threeReleases(t)
	doer := &probe{status: healthy}

	out := r.rollBackTo(doer, rollout{size: 2, pause: rolloutPause}, false)
	require.Nil(t, out.failure)
	for i := range 3 {
		assert.Equal(t, "releases/"+oldRelease, r.current(i), "host %d", i)
		assert.Equal(t, "v1", r.served(i))
		assert.Equal(t, oldRelease+"\n", r.remotes[i].read("/srv/app/.dhow/verified"))
		assert.Equal(t, []string{"uploaded", "switched", "health", "verified", "pruned"}, out.events.forHost(hostID(i)))
		assert.Equal(t, newRelease, out.events.eventOf("switched", hostID(i)).PreviousReleaseID)
		assert.False(t, r.lockHeld(i))
		assert.Equal(t, 3, countURL(doer, hostName(i)), "the health check of the batch is kept")
	}
	assert.Equal(t, deployrelease.ModeRollback, out.events.groupEvent("release_started").Mode)
	assert.Equal(t, []string{"release_started", "batch_passed", "batch_passed", "verified"}, out.events.groupKinds())
}

func TestAManualRollbackPutsTheReleaseBackFromTheArtifactOnAServerThatPrunedIt(t *testing.T) {
	r := threeReleases(t)
	require.NoError(t, os.RemoveAll(r.remotes[1].path("/srv/app/releases/"+oldRelease)))
	require.NoDirExists(t, r.remotes[1].path("/srv/app/releases/"+oldRelease))

	out := r.rollBackTo(&probe{status: healthy}, rollout{}, true)
	require.Nil(t, out.failure)
	for i := range 3 {
		assert.Equal(t, "releases/"+oldRelease, r.current(i))
		assert.Equal(t, "v1", r.served(i), "host %d serves the old files", i)
	}
	assert.DirExists(t, r.remotes[1].path("/srv/app/releases/"+oldRelease))
	assert.Equal(t, []string{"uploaded", "switched", "health", "verified", "pruned"}, out.events.forHost(hostID(1)))
}

func TestAManualRollbackWithoutTheArtifactRefusesBeforeAnythingChangesWhenAServerLostTheRelease(t *testing.T) {
	r := threeReleases(t)
	require.NoError(t, os.RemoveAll(r.remotes[2].path("/srv/app/releases/"+oldRelease)))

	out := r.rollBackTo(&probe{status: healthy}, rollout{}, false)
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitFailed, out.failure.Exit)
	require.ErrorIs(t, out.failure, deployrelease.ErrReleaseGone)
	for i := range 3 {
		assert.Equal(t, "releases/"+newRelease, r.current(i), "host %d is untouched", i)
		assert.False(t, r.lockHeld(i))
		assert.Equal(t, newRelease+"\n", r.remotes[i].read("/srv/app/.dhow/verified"))
	}
	assert.DirExists(t, r.remotes[0].path("/srv/app/releases/"+oldRelease), "the directory that is there is not removed")
	assert.Equal(t, []string{"release_started", "failed"}, out.events.groupKinds())
}

func TestAManualRollbackThatFailsOnAServerLeavesTheGroupRollbackFailedAndDoesNotTryAgain(t *testing.T) {
	r := threeReleases(t)

	out := r.rollBackTo(&probe{status: unhealthyHosts("web-2")}, rollout{}, true)
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitRollbackFailed, out.failure.Exit)
	assert.Equal(t, deployrelease.CauseHealth, out.failure.Reason)
	assert.Equal(t, "releases/"+oldRelease, r.current(0), "the first server stays on the release it was rolled back to")
	assert.Equal(t, "releases/"+oldRelease, r.current(1), "the server that failed is not put back by itself")
	assert.Equal(t, "releases/"+newRelease, r.current(2), "the third server was never touched")
	assert.Equal(t, newRelease+"\n", r.remotes[0].read("/srv/app/.dhow/verified"), "nothing was recorded as verified")
	assert.Equal(t, []string{"uploaded", "switched", "health"}, out.events.forHost(hostID(0)))
	assert.Equal(t, []string{"uploaded", "switched", "health", "rollback_failed"}, out.events.forHost(hostID(1)))
	assert.Equal(t, []string{"uploaded", "skipped"}, out.events.forHost(hostID(2)))
	assert.Equal(t, []string{"release_started", "batch_passed", "rollback_failed"}, out.events.groupKinds())
	assert.Equal(t, deployrelease.CauseHealth, out.events.groupEvent("rollback_failed").Reason)
	for i := range 3 {
		assert.False(t, r.lockHeld(i))
		assert.DirExists(t, r.remotes[i].path("/srv/app/releases/"+oldRelease), "no release directory is removed by the failure")
	}
}

func TestAManualRollbackToTheReleaseAServerIsAlreadyOnJustChecksItsHealth(t *testing.T) {
	r := threeReleases(t)
	require.NoError(t, r.remotes[0].Remove("/srv/app/current"))
	require.NoError(t, r.remotes[0].Symlink("releases/"+oldRelease, "/srv/app/current"))

	out := r.rollBackTo(&probe{status: healthy}, rollout{}, false)
	require.Nil(t, out.failure)
	for i := range 3 {
		assert.Equal(t, "releases/"+oldRelease, r.current(i))
	}
	assert.Equal(t, []string{"uploaded", "switched", "health", "verified", "pruned"}, out.events.forHost(hostID(0)))
}

func TestARetaggedReportKeepsTheReleaseOfTheServerOutOfTheEvents(t *testing.T) {
	rec := &recorder{}
	tagged := deployrelease.NewRetagReporter(rec, oldRelease, "9-nosha-1")
	tagged.Emit(deployrelease.Event{Event: "switched", ReleaseID: oldRelease, PreviousReleaseID: newRelease})
	tagged.Emit(deployrelease.Event{Event: "verified", ReleaseID: "other"})

	assert.Equal(t, "9-nosha-1", rec.events[0].ReleaseID)
	assert.Equal(t, newRelease, rec.events[0].PreviousReleaseID, "the previous release is the one on the server")
	assert.Equal(t, "other", rec.events[1].ReleaseID)
}
