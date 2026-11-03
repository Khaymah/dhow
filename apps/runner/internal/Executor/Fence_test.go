package executor

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

type beatAPI struct {
	API
	beats   atomic.Int32
	fail    atomic.Bool
	release chan struct{}
}

func (b *beatAPI) Heartbeat(context.Context, *runnerv1.LeaseRef) (*runnerv1.LeaseControl, error) {
	b.beats.Add(1)
	if b.release != nil {
		<-b.release
	}
	if b.fail.Load() {
		return nil, errors.New("unavailable")
	}
	return &runnerv1.LeaseControl{LeaseTtlSeconds: 45}, nil
}

func quiet() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestAFenceWhoseDeadlineHasPassedLosesWithoutARace(t *testing.T) {
	for range 200 {
		lostWith := make(chan error, 1)
		f := newFence(&beatAPI{}, &runnerv1.LeaseRef{}, DefaultTiming(), time.Now().Add(-time.Hour), 45*time.Second, quiet(),
			func(err error) { lostWith <- err }, nil)
		select {
		case err := <-lostWith:
			assert.ErrorIs(t, err, ErrSelfFenced)
		case <-time.After(5 * time.Second):
			t.Fatal("a fence past its deadline did not fire")
		}
		<-f.Lost()
	}
}

func TestTheFirstHeartbeatGoesOutAtOnce(t *testing.T) {
	api := &beatAPI{release: make(chan struct{})}
	f := newFence(api, &runnerv1.LeaseRef{}, DefaultTiming(), time.Now().Add(-20*time.Second), 45*time.Second, quiet(), nil, nil)
	go f.run(context.Background())
	defer f.Stop()
	require.Eventually(t, func() bool { return api.beats.Load() > 0 }, time.Second, 10*time.Millisecond,
		"a grant that waited in a long poll gets its heartbeat right away, not after a full interval")
	assert.Less(t, time.Until(f.Deadline()), 30*time.Second, "the deadline moves only once the server answered")
	close(api.release)
	require.Eventually(t, func() bool { return time.Until(f.Deadline()) > 30*time.Second }, time.Second, 10*time.Millisecond,
		"the answer to the heartbeat moved the deadline")
}

func TestHeartbeatsComeFasterAsTheDeadlineNears(t *testing.T) {
	api := &beatAPI{}
	api.fail.Store(true)
	f := newFence(api, &runnerv1.LeaseRef{}, DefaultTiming(), time.Now().Add(-30*time.Second), 45*time.Second, quiet(), nil, nil)
	assert.LessOrEqual(t, f.nextWait(), 5*time.Second/3+time.Millisecond)
	assert.GreaterOrEqual(t, f.nextWait(), minBeatWait)
}

func TestSuspendedTimeCountsAgainstTheDeadline(t *testing.T) {
	var boot atomic.Int64
	boot.Store(int64(time.Hour))
	clock := func() (time.Duration, bool) { return time.Duration(boot.Load()), true }
	api := &beatAPI{}
	f := newFence(api, &runnerv1.LeaseRef{}, DefaultTiming(), time.Now(), 45*time.Second, quiet(), nil, clock)
	go f.run(context.Background())
	defer f.Stop()
	require.Eventually(t, func() bool { return api.beats.Load() > 0 }, time.Second, 10*time.Millisecond)
	api.fail.Store(true)
	boot.Add(int64(40 * time.Second))
	select {
	case <-f.Lost():
		assert.ErrorIs(t, f.LostErr(), ErrSelfFenced, "40 seconds of suspend passed the deadline although the monotonic clock did not")
	case <-time.After(2 * time.Second):
		t.Fatal("the fence ignored time spent suspended")
	}
}
