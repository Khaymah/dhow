package deployrelease_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

type stubActivator struct {
	mu          sync.Mutex
	calls       []string
	inspectErr  error
	prepareErr  error
	activateErr map[string]error
}

func (s *stubActivator) record(call string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
}

func (s *stubActivator) Inspect(context.Context, deployrelease.Config) error {
	s.record("inspect")
	return s.inspectErr
}

func (s *stubActivator) Prepare(_ context.Context, _ deployrelease.Remote, rel deployrelease.Release) error {
	s.record("prepare " + rel.ID + " prev=" + rel.Previous)
	return s.prepareErr
}

func (s *stubActivator) Activate(_ context.Context, _ deployrelease.Remote, rel deployrelease.Release) error {
	s.record("activate " + rel.ID + " prev=" + rel.Previous)
	return s.activateErr[rel.ID]
}

func withActivator(t *testing.T, r *localRemote, a deployrelease.Activator, id string) outcome {
	t.Helper()
	path, digest := makeArchive(t, map[string]string{"index.html": id})
	cfg := baseConfig(path, digest, id)
	cfg.Health = deployrelease.Health{}
	cfg.Activator = a
	return deploy(t, r, cfg, nil)
}

func TestAnActivatorIsInspectedThenPreparedThenActivatedAfterTheSwitch(t *testing.T) {
	r := newLocalRemote(t)
	a := &stubActivator{}
	out := withActivator(t, r, a, "1-aaaaaaaa-1")
	require.Nil(t, out.failure)
	assert.Equal(t, []string{"inspect", "prepare 1-aaaaaaaa-1 prev=", "activate 1-aaaaaaaa-1 prev="}, a.calls)
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.currentTarget())
}

func TestAFailedActivationRollsBackThroughTheActivatorWithThePreviousRelease(t *testing.T) {
	r := newLocalRemote(t)
	require.Nil(t, withActivator(t, r, &stubActivator{}, "1-aaaaaaaa-1").failure)
	a := &stubActivator{activateErr: map[string]error{"2-bbbbbbbb-1": errors.New("unhealthy")}}
	out := withActivator(t, r, a, "2-bbbbbbbb-1")
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitRolledBack, out.failure.Exit)
	assert.Equal(t, deployrelease.CauseReload, out.failure.Reason)
	assert.Equal(t, []string{
		"inspect", "prepare 2-bbbbbbbb-1 prev=1-aaaaaaaa-1", "activate 2-bbbbbbbb-1 prev=1-aaaaaaaa-1", "activate 1-aaaaaaaa-1 prev=2-bbbbbbbb-1",
	}, a.calls)
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.currentTarget())
}

func TestAFailedPreparationNeverSwitchesAndCleansTheRelease(t *testing.T) {
	r := newLocalRemote(t)
	require.Nil(t, withActivator(t, r, &stubActivator{}, "1-aaaaaaaa-1").failure)
	a := &stubActivator{prepareErr: errors.New("manifest unknown")}
	out := withActivator(t, r, a, "2-bbbbbbbb-1")
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitFailed, out.failure.Exit)
	assert.Equal(t, deployrelease.CauseHook, out.failure.Reason)
	assert.Equal(t, []string{"inspect", "prepare 2-bbbbbbbb-1 prev=1-aaaaaaaa-1"}, a.calls)
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.currentTarget())
	assert.Equal(t, []string{"1-aaaaaaaa-1"}, r.releaseNames())
}

func TestARefusalOfTheInspectionHappensBeforeAnyConnection(t *testing.T) {
	connected := false
	a := &stubActivator{inspectErr: &deployrelease.Failure{Exit: 30, Reason: deployrelease.CauseOther, Text: "refused"}}
	path, digest := makeArchive(t, map[string]string{"index.html": "x"})
	cfg := baseConfig(path, digest, "1-aaaaaaaa-1")
	cfg.Health = deployrelease.Health{}
	cfg.Activator = a
	rec := &recorder{}
	e := &deployrelease.Engine{
		Config: cfg, Report: rec, Clock: fastClock(),
		Connect: func(context.Context) (deployrelease.Remote, error) { connected = true; return newLocalRemote(t), nil },
	}
	f := e.Run(context.Background())
	require.NotNil(t, f)
	assert.Equal(t, 30, f.Exit)
	assert.False(t, connected)
	assert.Equal(t, []string{"inspect"}, a.calls)
	assert.Contains(t, rec.kinds(), deployrelease.EventFailed)
}

func TestAnInspectionErrorThatIsNotAFailureBecomesOne(t *testing.T) {
	a := &stubActivator{inspectErr: errors.New("boom")}
	out := withActivator(t, newLocalRemote(t), a, "1-aaaaaaaa-1")
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitFailed, out.failure.Exit)
}
