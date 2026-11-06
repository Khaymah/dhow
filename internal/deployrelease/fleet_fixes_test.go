package deployrelease_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

type scripted struct {
	*localRemote
	rename    func(oldName, newName string) (apply bool, err error)
	removeAll func(name string) bool
	readFile  func(name string) bool
	exec      func(ctx context.Context, argv []string) (deployrelease.ExecResult, bool, error)
	hang      chan struct{}
}

func (s *scripted) Rename(oldName, newName string) error {
	if s.rename == nil {
		return s.localRemote.Rename(oldName, newName)
	}
	apply, err := s.rename(oldName, newName)
	if !apply && err == nil {
		<-s.hang
		return nil
	}
	if apply {
		if e := s.localRemote.Rename(oldName, newName); e != nil {
			return e
		}
	}
	return err
}

func (s *scripted) RemoveAll(name string) error {
	if s.removeAll != nil && s.removeAll(name) {
		<-s.hang
	}
	return s.localRemote.RemoveAll(name)
}

func (s *scripted) ReadFile(name string, limit int64) ([]byte, error) {
	if s.readFile != nil && s.readFile(name) {
		<-s.hang
	}
	return s.localRemote.ReadFile(name, limit)
}

func (s *scripted) Exec(ctx context.Context, argv []string) (deployrelease.ExecResult, error) {
	if s.exec != nil {
		if res, ok, err := s.exec(ctx, argv); ok {
			return res, err
		}
	}
	return s.localRemote.Exec(ctx, argv)
}

func (r *rig) script(t *testing.T, i int, edit func(*scripted)) {
	t.Helper()
	hang := make(chan struct{})
	t.Cleanup(func() { close(hang) })
	r.wrap[i] = func(l *localRemote) deployrelease.Remote {
		s := &scripted{localRemote: l, hang: hang}
		edit(s)
		return s
	}
}

func TestAHostThatStallsWhileBeingCleanedNeverBlocksTheRollback(t *testing.T) {
	r := newRig(t, 3)
	r.deployAll("1-aaaaaaaa-1", "v1", 1)
	r.script(t, 2, func(s *scripted) {
		s.removeAll = func(name string) bool { return strings.Contains(name, "/releases/") }
	})
	start := time.Now()
	out := r.roll("2-bbbbbbbb-1", "v2", &probe{status: unhealthyHosts("web-2")}, rollout{
		fleet: func(f *deployrelease.Fleet) { f.CleanupBudget = 200 * time.Millisecond },
	})
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitRolledBack, out.failure.Exit)
	assert.Less(t, time.Since(start), 5*time.Second)
	for i := range 3 {
		assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(i))
	}
	assert.Equal(t, []string{"uploaded", "skipped"}, out.events.forHost(hostID(2)))
	assert.Equal(t, []string{"release_started", "batch_passed", "rolled_back"}, out.events.groupKinds())
}

func TestASiblingCanceledWhileCheckingItsUploadIsSkippedAndNotFailed(t *testing.T) {
	r := newRig(t, 2)
	started := make(chan struct{})
	r.script(t, 0, func(s *scripted) {
		s.exec = func(_ context.Context, argv []string) (deployrelease.ExecResult, bool, error) {
			if argv[0] != "sha256sum" {
				return deployrelease.ExecResult{}, false, nil
			}
			<-started
			return deployrelease.ExecResult{ExitCode: 1}, true, nil
		}
	})
	r.script(t, 1, func(s *scripted) {
		s.exec = func(ctx context.Context, argv []string) (deployrelease.ExecResult, bool, error) {
			if argv[0] != "sha256sum" {
				return deployrelease.ExecResult{}, false, nil
			}
			close(started)
			<-ctx.Done()
			return deployrelease.ExecResult{}, true, ctx.Err()
		}
	})
	out := r.roll("1-aaaaaaaa-1", "v1", &probe{status: healthy}, rollout{})
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.CauseUpload, out.failure.Reason)
	assert.Equal(t, []string{"failed"}, out.events.forHost(hostID(0)))
	assert.Equal(t, []string{"skipped"}, out.events.forHost(hostID(1)))
	assert.Equal(t, deployrelease.CauseUpload, out.events.eventOf("failed", hostID(0)).Reason)
}

func TestASwitchThatLandedButReturnedAnErrorIsRolledBack(t *testing.T) {
	r := newRig(t, 2)
	r.deployAll("1-aaaaaaaa-1", "v1", 1)
	r.script(t, 1, func(s *scripted) {
		s.rename = func(oldName, newName string) (bool, error) {
			if strings.HasSuffix(newName, "/current") && strings.Contains(oldName, "tmp-2-bbbbbbbb-1") {
				return true, errConnect
			}
			return true, nil
		}
	})
	out := r.roll("2-bbbbbbbb-1", "v2", &probe{status: healthy}, rollout{})
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitRolledBack, out.failure.Exit)
	assert.Equal(t, deployrelease.CauseSwitch, out.failure.Reason)
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(1))
	assert.Equal(t, []string{"uploaded", "switched", "rolled_back"}, out.events.forHost(hostID(1)))
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(0))
}

type cancelOn struct {
	recorder
	kind   string
	cancel context.CancelFunc
}

func (c *cancelOn) Emit(e deployrelease.Event) {
	c.recorder.Emit(e)
	if e.Event == c.kind {
		c.cancel()
	}
}

func TestACancelAfterTheLastBatchRollsBackInsteadOfRecordingTheRelease(t *testing.T) {
	r := newRig(t, 2)
	r.deployAll("1-aaaaaaaa-1", "v1", 1)
	path, digest := makeArchive(t, map[string]string{"index.html": "v2"})
	cfg := baseConfig(path, digest, "2-bbbbbbbb-1")
	cfg.Health = deployrelease.Health{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rec := &cancelOn{kind: deployrelease.EventBatchPassed, cancel: cancel}
	f := &deployrelease.Fleet{Config: cfg, BatchSize: 2, Report: rec, Clock: fastClock(), Doer: &probe{status: healthy}}
	f.Hosts = []deployrelease.Host{r.host(0), r.host(1)}
	failure := f.Run(ctx)
	require.NotNil(t, failure)
	assert.Equal(t, deployrelease.ExitRolledBack, failure.Exit)
	assert.Equal(t, deployrelease.CauseCanceled, failure.Reason)
	for i := range 2 {
		assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(i))
		assert.Equal(t, "1-aaaaaaaa-1\n", r.remotes[i].read("/srv/app/.dhow/verified"))
		assert.Equal(t, []string{"uploaded", "switched", "rolled_back"}, rec.forHost(hostID(i)))
	}
	assert.NotContains(t, rec.groupKinds(), "verified")
}

func TestAHungLockDoesNotHoldTheReleaseOfTheOtherLocks(t *testing.T) {
	r := newRig(t, 3)
	r.script(t, 1, func(s *scripted) {
		s.readFile = func(name string) bool { return strings.HasSuffix(name, "lock/owner") }
	})
	start := time.Now()
	out := r.roll("1-aaaaaaaa-1", "v1", &probe{status: healthy}, rollout{
		fleet: func(f *deployrelease.Fleet) { f.LockTimeout = 150 * time.Millisecond },
	})
	require.Nil(t, out.failure)
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.False(t, r.lockHeld(0))
	assert.False(t, r.lockHeld(2))
}

func TestARollbackStepThatHangsEndsWithTheRollbackBudget(t *testing.T) {
	r := newRig(t, 2)
	r.deployAll("1-aaaaaaaa-1", "v1", 1)
	r.script(t, 0, func(s *scripted) {
		s.rename = func(oldName, _ string) (bool, error) {
			return !strings.Contains(oldName, "tmp-1-aaaaaaaa-1"), nil
		}
	})
	start := time.Now()
	out := r.roll("2-bbbbbbbb-1", "v2", &probe{status: unhealthyHosts("web-2")}, rollout{
		fleet: func(f *deployrelease.Fleet) {
			f.RollbackBudget = 300 * time.Millisecond
			f.LockTimeout = 150 * time.Millisecond
		},
	})
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitRollbackFailed, out.failure.Exit)
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.current(1))
	assert.Contains(t, out.events.forHost(hostID(0)), "rollback_failed")
	assert.Contains(t, out.events.forHost(hostID(1)), "rolled_back")
}
