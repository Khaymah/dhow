package deployrelease_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

func TestARollingDeployMovesEveryServerInOrderWithOneHealthCheckPerAddress(t *testing.T) {
	for _, size := range []int{1, 2} {
		r := newRig(t, 3)
		r.deployAll("1-aaaaaaaa-1", "v1", size)
		doer := &probe{status: healthy}
		out := r.roll("2-bbbbbbbb-1", "v2", doer, rollout{size: size, pause: rolloutPause})
		require.Nil(t, out.failure)
		for i := range 3 {
			assert.Equal(t, "releases/2-bbbbbbbb-1", r.current(i))
			assert.Equal(t, "v2", r.served(i))
			assert.Equal(t, []string{"uploaded", "switched", "health", "verified", "pruned"}, out.events.forHost(hostID(i)), "host %d", i)
			assert.Equal(t, "1-aaaaaaaa-1", out.events.eventOf("switched", hostID(i)).PreviousReleaseID)
			assert.False(t, r.lockHeld(i), "the lock of host %d is released", i)
			assert.Equal(t, "2-bbbbbbbb-1\n", r.remotes[i].read("/srv/app/.dhow/verified"))
		}
		for i := range 3 {
			assert.Equal(t, i/size+1, out.events.eventOf("switched", hostID(i)).Batch, "size %d host %d", size, i)
			if i > 0 && (i-1)/size != i/size {
				assert.Less(t, out.events.index("switched", hostID(i-1)), out.events.index("switched", hostID(i)))
			}
		}
		batches := (3 + size - 1) / size
		want := []string{"release_started"}
		for range batches {
			want = append(want, "batch_passed")
		}
		assert.Equal(t, append(want, "verified"), out.events.groupKinds())
		assert.Len(t, r.sleeps, batches-1, "the pause is kept between batches and not after the last one")
		for i := range 3 {
			assert.Equal(t, 3, countURL(doer, hostName(i)), "each address is checked, stable=3 times")
		}
	}
}

func countURL(p *probe, host string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, u := range p.urls {
		if strings.Contains(u, host) {
			n++
		}
	}
	return n
}

func TestAFailureOnTheSecondServerPutsTheFirstBackAndLeavesTheThirdAlone(t *testing.T) {
	r := newRig(t, 3)
	r.deployAll("1-aaaaaaaa-1", "v1", 1)
	r.deployAll("2-bbbbbbbb-1", "v2", 1)

	out := r.roll("3-cccccccc-1", "v3", &probe{status: unhealthyHosts("web-2")}, rollout{})
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitRolledBack, out.failure.Exit)
	assert.Equal(t, deployrelease.CauseHealth, out.failure.Reason)
	for i := range 3 {
		assert.Equal(t, "releases/2-bbbbbbbb-1", r.current(i), "host %d", i)
		assert.Equal(t, "v2", r.served(i))
		assert.Equal(t, "2-bbbbbbbb-1\n", r.remotes[i].read("/srv/app/.dhow/verified"))
		assert.False(t, r.lockHeld(i))
	}
	assert.NoDirExists(t, r.remotes[2].path("/srv/app/releases/3-cccccccc-1"), "the third server never got the release")
	assert.Equal(t, []string{"uploaded", "switched", "health", "rolled_back"}, out.events.forHost(hostID(0)))
	assert.Equal(t, []string{"uploaded", "switched", "health", "rolled_back"}, out.events.forHost(hostID(1)))
	assert.Equal(t, []string{"uploaded", "skipped"}, out.events.forHost(hostID(2)))
	assert.Equal(t, []string{"release_started", "batch_passed", "rolled_back"}, out.events.groupKinds())
	assert.Less(t, out.events.index("rolled_back", hostID(1)), out.events.index("rolled_back", hostID(0)), "the newest server goes back first")
	assert.Equal(t, deployrelease.CauseHealth, out.events.eventOf("rolled_back", hostID(0)).Reason)
	assert.Equal(t, deployrelease.CauseHealth, out.events.groupEvent("rolled_back").Reason)
}

func TestAFailureInsideABatchOfTwoPutsBackBothAndAFailureInALaterBatchPutsBackTheNewestFirst(t *testing.T) {
	r := newRig(t, 3)
	r.deployAll("1-aaaaaaaa-1", "v1", 1)

	inside := r.roll("2-bbbbbbbb-1", "v2", &probe{status: unhealthyHosts("web-2")}, rollout{size: 2})
	require.NotNil(t, inside.failure)
	assert.Equal(t, deployrelease.ExitRolledBack, inside.failure.Exit)
	for i := range 3 {
		assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(i), "host %d", i)
	}
	assert.Contains(t, inside.events.forHost(hostID(0)), "rolled_back")
	assert.Contains(t, inside.events.forHost(hostID(1)), "rolled_back")
	assert.Equal(t, []string{"uploaded", "skipped"}, inside.events.forHost(hostID(2)))
	assert.Equal(t, []string{"release_started", "rolled_back"}, inside.events.groupKinds())

	later := r.roll("3-cccccccc-1", "v3", &probe{status: unhealthyHosts("web-3")}, rollout{size: 2})
	require.NotNil(t, later.failure)
	assert.Equal(t, deployrelease.ExitRolledBack, later.failure.Exit)
	for i := range 3 {
		assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(i))
	}
	third := later.events.index("rolled_back", hostID(2))
	assert.Less(t, third, later.events.index("rolled_back", hostID(0)))
	assert.Less(t, third, later.events.index("rolled_back", hostID(1)))
	assert.Equal(t, 1, later.events.eventOf("switched", hostID(1)).Batch)
	assert.Equal(t, 2, later.events.eventOf("switched", hostID(2)).Batch)
}

func TestAFailedRollbackOfOneServerDoesNotStopTheOthers(t *testing.T) {
	r := newRig(t, 3)
	r.deployAll("1-aaaaaaaa-1", "v1", 1)
	r.wrap[0] = func(l *localRemote) deployrelease.Remote {
		return &flaky{localRemote: l, failRename: func(oldName, _ string) error {
			if strings.HasSuffix(oldName, "current.tmp-1-aaaaaaaa-1") {
				return errConnect
			}
			return nil
		}}
	}

	out := r.roll("2-bbbbbbbb-1", "v2", &probe{status: unhealthyHosts("web-3")}, rollout{})
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitRollbackFailed, out.failure.Exit)
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(1), "the second server went back although the first could not")
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(2))
	assert.Equal(t, "releases/2-bbbbbbbb-1", r.current(0), "the first server stays on the new release")
	assert.Equal(t, []string{"uploaded", "switched", "health", "rollback_failed"}, out.events.forHost(hostID(0)))
	assert.Contains(t, out.events.forHost(hostID(1)), "rolled_back")
	assert.Contains(t, out.events.forHost(hostID(2)), "rolled_back")
	assert.Equal(t, []string{"release_started", "batch_passed", "batch_passed", "rollback_failed"}, out.events.groupKinds())
	assert.Equal(t, deployrelease.CauseHealth, out.failure.Reason)
	for i := range 3 {
		assert.False(t, r.lockHeld(i))
	}
}

func TestAServerThatCannotBeReachedFailsTheReleaseBeforeAnythingSwitches(t *testing.T) {
	r := newRig(t, 3)
	r.deployAll("1-aaaaaaaa-1", "v1", 1)
	r.refuse[2] = errConnect

	out := r.roll("2-bbbbbbbb-1", "v2", &probe{status: healthy}, rollout{})
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitConnect, out.failure.Exit)
	for i := range 3 {
		assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(i))
		assert.False(t, r.lockHeld(i), "the locks taken before the failure are released")
		assert.NoDirExists(t, r.remotes[i].path("/srv/app/releases/2-bbbbbbbb-1"))
	}
	assert.Equal(t, []string{"release_started", "failed"}, out.events.groupKinds())
	assert.Equal(t, []string{"failed"}, out.events.forHost(hostID(2)))
	assert.Equal(t, []string{"skipped"}, out.events.forHost(hostID(0)))
	assert.Equal(t, "1-aaaaaaaa-1", out.events.groupEvent("release_started").PreviousReleaseID)
}

func TestAWrongHostKeyOnTheLastServerIsRefusedBeforeAnythingIsUploaded(t *testing.T) {
	r := newRig(t, 3)
	r.refuse[2] = deployrelease.ErrHostKeyMismatch

	out := r.roll("1-aaaaaaaa-1", "v1", &probe{status: healthy}, rollout{})
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitHostKeyMismatch, out.failure.Exit)
	for i := range 3 {
		assert.Empty(t, r.current(i))
		assert.Empty(t, r.remotes[i].releaseNames(), "nothing was uploaded to host %d", i)
		assert.False(t, r.lockHeld(i))
	}
}

func TestALockHeldOnOneServerStopsTheWholeReleaseAndFreesTheOtherLocks(t *testing.T) {
	r := newRig(t, 3)
	r.deployAll("1-aaaaaaaa-1", "v1", 1)
	require.NoError(t, r.remotes[1].MkdirAll("/srv/app/.dhow/lock", 0o700))

	out := r.roll("2-bbbbbbbb-1", "v2", &probe{status: healthy}, rollout{})
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitTargetLocked, out.failure.Exit)
	for i := range 3 {
		assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(i))
	}
	assert.False(t, r.lockHeld(0))
	assert.True(t, r.lockHeld(1), "the lock of the other deployment is left alone")
}

func TestCancelingBetweenTwoBatchesPutsTheSwitchedServersBack(t *testing.T) {
	r := newRig(t, 3)
	r.deployAll("1-aaaaaaaa-1", "v1", 1)

	out := r.roll("2-bbbbbbbb-1", "v2", &probe{status: healthy}, rollout{pause: rolloutPause, cancelOnPause: true})
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitRolledBack, out.failure.Exit)
	assert.Equal(t, deployrelease.CauseCanceled, out.failure.Reason)
	for i := range 3 {
		assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(i))
		assert.False(t, r.lockHeld(i))
	}
	assert.Contains(t, out.events.forHost(hostID(0)), "rolled_back")
	assert.Equal(t, []string{"uploaded", "skipped"}, out.events.forHost(hostID(1)))
	assert.Equal(t, deployrelease.CauseCanceled, out.events.groupEvent("rolled_back").Reason)
}

type cancelingDoer struct {
	cancel context.CancelFunc
	host   string
}

func (c cancelingDoer) Do(r *http.Request) (*http.Response, error) {
	if strings.Contains(r.URL.Host, c.host) {
		c.cancel()
		return nil, context.Canceled
	}
	return (&probe{status: healthy}).Do(r)
}

func TestCancelingDuringAHealthCheckRollsBackWithinTheBudget(t *testing.T) {
	r := newRig(t, 3)
	r.deployAll("1-aaaaaaaa-1", "v1", 1)
	path, digest := makeArchive(t, map[string]string{"index.html": "v2"})
	cfg := baseConfig(path, digest, "2-bbbbbbbb-1")
	cfg.Health.URL = "http://{host}/healthz"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rec := &recorder{}
	f := &deployrelease.Fleet{Config: cfg, BatchSize: 1, Doer: cancelingDoer{cancel: cancel, host: "web-2"}, Report: rec, Clock: fastClock()}
	for i := range 3 {
		f.Hosts = append(f.Hosts, r.host(i))
	}
	start := time.Now()
	failure := f.Run(ctx)
	require.NotNil(t, failure)
	assert.Equal(t, deployrelease.ExitRolledBack, failure.Exit)
	assert.Equal(t, deployrelease.CauseCanceled, failure.Reason)
	assert.Less(t, time.Since(start), deployrelease.RollbackBudget)
	for i := range 3 {
		assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(i))
	}
	assert.Equal(t, []string{"uploaded", "skipped"}, rec.forHost(hostID(2)))
}

func TestAFirstDeployThatFailsLeavesTheServersAsTheyAreAndFails(t *testing.T) {
	r := newRig(t, 3)
	out := r.roll("1-aaaaaaaa-1", "v1", &probe{status: unhealthyHosts("web-2")}, rollout{})
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitHealthFailed, out.failure.Exit)
	assert.Equal(t, []string{"uploaded", "switched", "health", "failed"}, out.events.forHost(hostID(0)))
	assert.Equal(t, []string{"uploaded", "skipped"}, out.events.forHost(hostID(2)))
	assert.Equal(t, []string{"release_started", "batch_passed", "failed"}, out.events.groupKinds())
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(0), "there is no earlier release to go back to")
}

func TestAServerWithoutAnEarlierReleaseMakesTheGroupRollbackFailed(t *testing.T) {
	r := newRig(t, 3)
	first := r.roll("1-aaaaaaaa-1", "v1", &probe{status: healthy}, rollout{hosts: []int{0, 1}})
	require.Nil(t, first.failure)

	out := r.roll("2-bbbbbbbb-1", "v2", &probe{status: unhealthyHosts("web-3")}, rollout{})
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitRollbackFailed, out.failure.Exit)
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(0))
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(1))
	assert.Equal(t, "releases/2-bbbbbbbb-1", r.current(2))
	assert.Equal(t, []string{"uploaded", "switched", "health", "rollback_failed"}, out.events.forHost(hostID(2)))
	assert.Equal(t, []string{"release_started", "batch_passed", "batch_passed", "rollback_failed"}, out.events.groupKinds())
}

func TestAHealthAddressWithoutAHostIsCheckedOncePerBatch(t *testing.T) {
	r := newRig(t, 3)
	doer := &probe{status: healthy}
	out := r.roll("1-aaaaaaaa-1", "v1", doer, rollout{size: 2, edit: func(c *deployrelease.Config) { c.Health.URL = "http://lb.example.com/healthz" }})
	require.Nil(t, out.failure)
	assert.Equal(t, 6, doer.calls, "two batches, three stable checks each")
	for i := range 3 {
		assert.Contains(t, out.events.forHost(hostID(i)), "health")
	}

	bad := newRig(t, 3)
	failed := bad.roll("1-aaaaaaaa-1", "v1", &probe{status: unhealthy}, rollout{size: 2, edit: func(c *deployrelease.Config) { c.Health.URL = "http://lb.example.com/healthz" }})
	require.NotNil(t, failed.failure)
	assert.Equal(t, deployrelease.ExitHealthFailed, failed.failure.Exit)
	assert.Equal(t, []string{"uploaded", "skipped"}, failed.events.forHost(hostID(2)))
}

func TestARolloutThatIsNotValidIsRefusedBeforeAnyServerIsTouched(t *testing.T) {
	for name, o := range map[string]rollout{
		"a batch larger than the group": {size: 4},
		"a pause that is too long":      {size: 1, pause: 601 * time.Second},
		"a negative pause":              {size: 1, pause: -time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, 3)
			out := r.roll("1-aaaaaaaa-1", "v1", &probe{status: healthy}, o)
			require.NotNil(t, out.failure)
			assert.Equal(t, deployrelease.ExitInput, out.failure.Exit)
			for i := range 3 {
				assert.Empty(t, r.remotes[i].releaseNames())
			}
		})
	}
	assert.ErrorIs(t, deployrelease.ValidateRollout(11, 1, 0, deployrelease.Health{}), deployrelease.ErrRollout)
	assert.ErrorIs(t, deployrelease.ValidateRollout(0, 1, 0, deployrelease.Health{}), deployrelease.ErrRollout)
	long := deployrelease.Health{URL: "http://x/", StatusMin: 200, StatusMax: 399, Timeout: 30 * time.Second, Retries: 10, Interval: 20 * time.Second, Stable: 1}
	assert.ErrorIs(t, deployrelease.ValidateRollout(10, 1, 600*time.Second, long), deployrelease.ErrRollout, "ten batches of five minutes and a pause is over an hour")
	assert.NoError(t, deployrelease.ValidateRollout(10, 5, 0, long))
}

func TestTwoServersWithTheSameIdentityAreRefused(t *testing.T) {
	r := newRig(t, 2)
	path, digest := makeArchive(t, map[string]string{"index.html": "v1"})
	f := &deployrelease.Fleet{Config: baseConfig(path, digest, "1-aaaaaaaa-1"), BatchSize: 1, Doer: &probe{status: healthy}, Clock: fastClock()}
	f.Hosts = []deployrelease.Host{r.host(0), r.host(0)}
	got := f.Run(context.Background())
	require.NotNil(t, got)
	assert.Equal(t, deployrelease.ExitInput, got.Exit)
}
