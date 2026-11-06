package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

const (
	v1 = "1-aaaaaaaa-1"
	v2 = "2-bbbbbbbb-1"
	v3 = "3-cccccccc-1"
	v4 = "4-dddddddd-1"
)

func (g *group) requireOn(release string, hosts ...int) {
	g.t.Helper()
	for _, i := range hosts {
		assert.Equal(g.t, "releases/"+release, g.current(i), "host %d", i+1)
		assert.Equal(g.t, "v"+release[:1], g.served(i, "index.html"), "host %d serves the release", i+1)
	}
}

func (g *group) requireNoLocks() {
	g.t.Helper()
	for i := range g.hosts {
		assert.False(g.t, g.exists(i, ".dhow/lock"), "the lock of host %d is released", i+1)
	}
}

func TestARollingDeployMovesThreeServersInOrderInBatchesOfOneThenTwo(t *testing.T) {
	g := newGroup(t, "rollok")
	g.deployAll(v1, 1)

	one := g.deploy("ok-batch-1", rolling{release: v2, files: page("v2", true), batch: 1})
	require.Equal(t, 0, one.exit, one.output)
	g.requireOn(v2, 0, 1, 2)
	for i := range 3 {
		assert.Equal(t, []string{"uploaded", "switched", "health", "verified", "pruned"}, one.forHost(i))
		assert.ElementsMatch(t, []string{v1, v2}, g.releases(i), "the old release stays")
		assert.Equal(t, v2, g.target(i).readRemote(".dhow/verified"))
	}
	assert.Less(t, one.indexOf("switched", 0), one.indexOf("switched", 1))
	assert.Less(t, one.indexOf("switched", 1), one.indexOf("switched", 2), "the servers switch in the order of the target")
	assert.Equal(t, []string{"release_started", "batch_passed", "batch_passed", "batch_passed", "verified"}, one.groupKinds())
	for i, e := range []int{1, 2, 3} {
		assert.Equal(t, e, one.events[one.indexOf("switched", i)].Batch)
	}
	g.requireNoLocks()

	two := g.deploy("ok-batch-2", rolling{release: v3, files: page("v3", true), batch: 2, pause: 1})
	require.Equal(t, 0, two.exit, two.output)
	g.requireOn(v3, 0, 1, 2)
	assert.Equal(t, []string{"release_started", "batch_passed", "batch_passed", "verified"}, two.groupKinds())
	assert.Equal(t, 1, two.events[two.indexOf("switched", 1)].Batch)
	assert.Equal(t, 2, two.events[two.indexOf("switched", 2)].Batch)
}

func TestAFailureOnTheSecondServerPutsTheFirstBackAndNeverTouchesTheThird(t *testing.T) {
	g := newGroup(t, "rollmid")
	g.deployAll(v1, 1)
	g.deployAll(v2, 1)
	g.provideShared("healthz", 0, 2)

	out := g.deploy("fail-middle", rolling{release: v3, files: map[string]string{"index.html": "v3"}, shared: []string{"healthz"}})
	assert.Equal(t, deployrelease.ExitRolledBack, out.exit, out.output)
	assert.Contains(t, out.output, "DEPLOY_ROLLED_BACK")
	g.requireOn(v2, 0, 1, 2)
	assert.False(t, g.exists(2, "releases/"+v3), "the third server never got the release")
	assert.Equal(t, []string{"uploaded", "switched", "health", "rolled_back"}, out.forHost(0))
	assert.Equal(t, []string{"uploaded", "switched", "health", "rolled_back"}, out.forHost(1))
	assert.Equal(t, []string{"uploaded", "skipped"}, out.forHost(2))
	assert.Equal(t, []string{"release_started", "batch_passed", "rolled_back"}, out.groupKinds())
	assert.Less(t, out.indexOf("rolled_back", 1), out.indexOf("rolled_back", 0), "the newest server goes back first")
	health := out.events[out.indexOf("health", 1)]
	require.NotNil(t, health.Health)
	assert.False(t, health.Health.Passed)
	assert.Equal(t, 404, health.Health.LastStatus)
	for i := range 3 {
		assert.Equal(t, v2, g.target(i).readRemote(".dhow/verified"))
	}
	g.requireNoLocks()
}

func TestAFailureInsideABatchOfTwoPutsBackBothServersAndALaterBatchGoesBackFirst(t *testing.T) {
	g := newGroup(t, "rollbatch")
	g.deployAll(v1, 1)
	g.provideShared("healthz", 0, 2)

	inside := g.deploy("fail-in-batch", rolling{release: v2, files: map[string]string{"index.html": "v2"}, shared: []string{"healthz"}, batch: 2})
	assert.Equal(t, deployrelease.ExitRolledBack, inside.exit, inside.output)
	g.requireOn(v1, 0, 1, 2)
	assert.Contains(t, inside.forHost(0), "rolled_back")
	assert.Contains(t, inside.forHost(1), "rolled_back")
	assert.Equal(t, []string{"uploaded", "skipped"}, inside.forHost(2))
	assert.False(t, g.exists(2, "releases/"+v2))

	h := newGroup(t, "rolllater")
	h.deployAll(v1, 1)
	h.provideShared("healthz", 0, 1)
	later := h.deploy("fail-later-batch", rolling{release: v3, files: map[string]string{"index.html": "v3"}, shared: []string{"healthz"}, batch: 2})
	assert.Equal(t, deployrelease.ExitRolledBack, later.exit, later.output)
	h.requireOn(v1, 0, 1, 2)
	assert.Less(t, later.indexOf("rolled_back", 2), later.indexOf("rolled_back", 0))
	assert.Less(t, later.indexOf("rolled_back", 2), later.indexOf("rolled_back", 1))
	h.requireNoLocks()
}

func TestAFailedRollbackOnTheFirstServerStillPutsTheOthersBackAndALaterDeployFixesTheGroup(t *testing.T) {
	g := newGroup(t, "rollfail")
	g.deployAll(v1, 1)
	g.deployAll(v2, 1)
	g.provideShared("healthz", 0, 1)
	g.putFile(0, ".no-rollback", "x")
	reload := `if [ -e "$DHOW_DEPLOY_PATH/.no-rollback" ] && [ "$DHOW_RELEASE_ID" != ` + v3 + ` ]; then exit 1; fi`

	out := g.deploy("rollback-failed", rolling{release: v3, files: map[string]string{"index.html": "v3"}, shared: []string{"healthz"}, reload: reload})
	assert.Equal(t, deployrelease.ExitRollbackFailed, out.exit, out.output)
	assert.Contains(t, out.output, "DEPLOY_ROLLBACK_FAILED")
	g.requireOn(v2, 1, 2)
	assert.Equal(t, []string{"uploaded", "switched", "health", "rollback_failed"}, out.forHost(0))
	assert.Contains(t, out.forHost(1), "rolled_back")
	assert.Contains(t, out.forHost(2), "rolled_back")
	assert.Equal(t, []string{"release_started", "batch_passed", "batch_passed", "rollback_failed"}, out.groupKinds())
	g.requireNoLocks()

	fixed := g.deploy("after-rollback-failed", rolling{release: v4, files: page("v4", true)})
	require.Equal(t, 0, fixed.exit, fixed.output)
	g.requireOn(v4, 0, 1, 2)
}

func TestAServerThatCannotBeReachedFailsTheReleaseAndNothingSwitches(t *testing.T) {
	g := newGroup(t, "rollgone")
	g.deployAll(v1, 1)

	out := g.deploy("unreachable", rolling{release: v2, files: page("v2", true), port: map[int]int{2: 2222}})
	assert.Equal(t, deployrelease.ExitConnect, out.exit, out.output)
	assert.Contains(t, out.output, "TARGET_HOST_UNREACHABLE")
	g.requireOn(v1, 0, 1, 2)
	for i := range 3 {
		assert.False(t, g.exists(i, "releases/"+v2), "host %d got nothing", i+1)
	}
	assert.Equal(t, []string{"release_started", "failed"}, out.groupKinds())
	assert.Equal(t, []string{"failed"}, out.forHost(2))
	assert.Equal(t, []string{"skipped"}, out.forHost(0))
	g.requireNoLocks()
}

func TestAWrongHostKeyOnOneServerIsRefusedBeforeAnythingSwitches(t *testing.T) {
	g := newGroup(t, "rollkey")
	g.deployAll(v1, 1)

	out := g.deploy("wrong-host-key", rolling{release: v2, files: page("v2", true), known: map[int]string{2: otherHostKey(t, g.hosts[2])}})
	assert.Equal(t, deployrelease.ExitHostKeyMismatch, out.exit, out.output)
	assert.Contains(t, out.output, "HOST_KEY_MISMATCH")
	g.requireOn(v1, 0, 1, 2)
	for i := range 3 {
		assert.ElementsMatch(t, []string{v1}, g.releases(i), "nothing was uploaded to host %d", i+1)
	}
	assert.Equal(t, []string{"failed"}, out.forHost(2))
	g.requireNoLocks()
}

func TestATermSignalDuringARolloutPutsBackTheSwitchedServers(t *testing.T) {
	g := newGroup(t, "rollterm")
	g.deployAll(v1, 1)
	g.deployAll(v2, 1)
	g.provideShared("healthz", 0)

	out := g.deploy("term-canceled", rolling{
		release: v3, files: map[string]string{"index.html": "v3"}, shared: []string{"healthz"}, retries: 40,
		stopAtHost: 1, stopAt: "releases/" + v3,
	})
	assert.Equal(t, deployrelease.ExitRolledBack, out.exit, out.output)
	g.requireOn(v2, 0, 1, 2)
	assert.False(t, g.exists(2, "releases/"+v3))
	last := out.events[len(out.events)-1]
	assert.Equal(t, deployrelease.EventRolledBack, last.Event)
	assert.Equal(t, deployrelease.CauseCanceled, last.Reason)
	assert.Contains(t, out.forHost(0), "rolled_back")
	assert.Contains(t, out.forHost(1), "rolled_back")
	g.requireNoLocks()
}
