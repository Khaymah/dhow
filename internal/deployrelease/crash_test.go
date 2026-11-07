package deployrelease_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

const (
	verifiedRelease    = "1-aaaaaaaa-1"
	interruptedRelease = "2-bbbbbbbb-1"
	nextRelease        = "3-cccccccc-1"
	reloadOfTheDeploy  = "systemctl reload app"
	afterTheLockExpiry = deployrelease.LockTTL + time.Minute
)

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	require.NoError(t, filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(to, rel)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(target, dst)
		case entry.IsDir():
			return os.MkdirAll(dst, info.Mode().Perm())
		default:
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(dst, raw, info.Mode().Perm())
		}
	}))
}

func lateClock(offset time.Duration) deployrelease.Clock {
	clock := fastClock()
	return deployrelease.Clock{
		Now:   func() time.Time { return clock.Now().Add(offset) },
		Sleep: clock.Sleep,
	}
}

func deployWithClock(t *testing.T, r *localRemote, id string, doer deployrelease.Doer, clock deployrelease.Clock) outcome {
	t.Helper()
	path, digest := makeArchive(t, map[string]string{"index.html": id, "assets/app.js": "js " + id})
	rec := &recorder{}
	e := &deployrelease.Engine{
		Config: baseConfig(path, digest, id), Doer: doer, Report: rec, Clock: clock,
		Connect: func(context.Context) (deployrelease.Remote, error) { return r, nil },
	}
	return outcome{failure: e.Run(context.Background()), events: rec}
}

func serverAfterTheProcessDiedAfterTheSwitch(t *testing.T) *localRemote {
	t.Helper()
	live := newLocalRemote(t)
	require.Nil(t, release(t, live, verifiedRelease, "v1", &probe{status: healthy}, nil).failure)

	var once sync.Once
	dead := newLocalRemote(t)
	live.hook = func(argv []string) deployrelease.ExecResult {
		if argv[0] == "env" {
			once.Do(func() { copyTree(t, live.root, dead.root) })
		}
		return deployrelease.ExecResult{}
	}
	require.Nil(t, release(t, live, interruptedRelease, "v2", &probe{status: healthy}, func(c *deployrelease.Config) {
		c.ReloadCommand = reloadOfTheDeploy
	}).failure)
	return dead
}

func TestAProcessKilledRightAfterTheSwitchLeavesTheServerSwitchingOnAnUnverifiedRelease(t *testing.T) {
	dead := serverAfterTheProcessDiedAfterTheSwitch(t)

	assert.Equal(t, "releases/"+interruptedRelease, dead.currentTarget(), "the switch landed before the process died")
	assert.Equal(t, verifiedRelease+"\n", dead.read("/srv/app/.dhow/verified"), "the release was never verified")
	assert.Contains(t, dead.read("/srv/app/.dhow/state.json"), `"State":"switching"`)
	assert.Contains(t, dead.read("/srv/app/.dhow/state.json"), `"Release":"`+interruptedRelease+`"`)
	assert.Contains(t, dead.read("/srv/app/.dhow/state.json"), `"Previous":"`+verifiedRelease+`"`)
	assert.DirExists(t, dead.path("/srv/app/.dhow/lock"), "a dead process cannot release its lock")
}

func TestTheNextDeployAfterAKillWaitsForTheDeadLockToExpire(t *testing.T) {
	dead := serverAfterTheProcessDiedAfterTheSwitch(t)

	blocked := deployWithClock(t, dead, nextRelease, &probe{status: healthy}, fastClock())
	require.NotNil(t, blocked.failure)
	assert.Equal(t, deployrelease.ExitTargetLocked, blocked.failure.Exit)
	assert.Equal(t, "releases/"+interruptedRelease, dead.currentTarget(), "a blocked deploy changes nothing")
	assert.NoDirExists(t, dead.path("/srv/app/releases/"+nextRelease))
}

func TestARollbackAfterAKillGoesToTheVerifiedReleaseAndNotToTheInterruptedOne(t *testing.T) {
	dead := serverAfterTheProcessDiedAfterTheSwitch(t)

	out := deployWithClock(t, dead, nextRelease, &probe{status: unhealthy}, lateClock(afterTheLockExpiry))
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitRolledBack, out.failure.Exit)
	assert.Equal(t, "releases/"+verifiedRelease, dead.currentTarget(), "the interrupted release is not trusted as a rollback target")
	assert.Equal(t, "v1", dead.read("/srv/app/current/index.html"))
	assert.Equal(t, verifiedRelease+"\n", dead.read("/srv/app/.dhow/verified"))
	switched, ok := out.events.find(deployrelease.EventSwitched)
	require.True(t, ok)
	assert.Equal(t, verifiedRelease, switched.PreviousReleaseID, "the server reports the verified release as the previous one")
}

func TestASuccessfulDeployAfterAKillVerifiesTheNewReleaseAndReportsTheVerifiedOneAsPrevious(t *testing.T) {
	dead := serverAfterTheProcessDiedAfterTheSwitch(t)

	out := deployWithClock(t, dead, nextRelease, &probe{status: healthy}, lateClock(afterTheLockExpiry))
	require.Nil(t, out.failure)
	assert.Equal(t, "releases/"+nextRelease, dead.currentTarget())
	assert.Equal(t, nextRelease+"\n", dead.read("/srv/app/.dhow/verified"))
	assert.Contains(t, dead.read("/srv/app/.dhow/state.json"), `"State":"idle"`)
	assert.NoDirExists(t, dead.path("/srv/app/.dhow/lock"))
	switched, ok := out.events.find(deployrelease.EventSwitched)
	require.True(t, ok)
	assert.Equal(t, verifiedRelease, switched.PreviousReleaseID)
}
