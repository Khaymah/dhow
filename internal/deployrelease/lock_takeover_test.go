package deployrelease_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

const (
	barrierWait = 10 * time.Second
	takers      = 3
)

type barrier struct {
	need    int
	mu      sync.Mutex
	arrived int
	open    chan struct{}
}

func newBarrier(need int) *barrier { return &barrier{need: need, open: make(chan struct{})} }

func (b *barrier) wait() {
	b.mu.Lock()
	b.arrived++
	if b.arrived == b.need {
		close(b.open)
	}
	b.mu.Unlock()
	select {
	case <-b.open:
	case <-time.After(barrierWait):
	}
}

type contention struct {
	seen *barrier
	need int
	mu   sync.Mutex
	hits int
}

type contendedRemote struct {
	*localRemote
	c *contention
}

func (r *contendedRemote) ReadFile(name string, limit int64) ([]byte, error) {
	raw, err := r.localRemote.ReadFile(name, limit)
	if strings.HasSuffix(name, "/lock/owner") {
		r.c.mu.Lock()
		r.c.hits++
		first := r.c.hits <= r.c.need
		r.c.mu.Unlock()
		if first {
			r.c.seen.wait()
		}
	}
	return raw, err
}

type gatedHealth struct {
	reached chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *gatedHealth) Do(r *http.Request) (*http.Response, error) {
	g.once.Do(func() { close(g.reached) })
	<-g.release
	return (&probe{status: healthy}).Do(r)
}

func TestThreeTakersOfAnExpiredLockNeverMoreThanOneWins(t *testing.T) {
	r := newLocalRemote(t)
	require.NoError(t, os.MkdirAll(r.path("/srv/app/.dhow/lock"), 0o700))
	old := fmt.Sprintf(`{"Release":"9-zzzzzzzz-1","ExpiresAt":%q}`, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339))
	require.NoError(t, os.WriteFile(r.path("/srv/app/.dhow/lock/owner"), []byte(old), 0o600))
	shared := &contention{seen: newBarrier(takers), need: takers}
	gate := &gatedHealth{reached: make(chan struct{}), release: make(chan struct{})}

	results := make(chan *deployrelease.Failure, takers)
	var wg sync.WaitGroup
	for _, id := range []string{"1-aaaaaaaa-1", "2-bbbbbbbb-1", "3-cccccccc-1"} {
		path, digest := makeArchive(t, map[string]string{"index.html": id})
		cfg := baseConfig(path, digest, id)
		mine := &contendedRemote{localRemote: &localRemote{t: t, root: r.root, free: r.free}, c: shared}
		e := &deployrelease.Engine{
			Config: cfg, Doer: gate, Report: &recorder{}, Clock: fastClock(),
			Connect: func(context.Context) (deployrelease.Remote, error) { return mine, nil },
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- e.Run(context.Background())
		}()
	}

	select {
	case <-gate.reached:
	case <-time.After(barrierWait):
		require.Fail(t, "no deployment took the expired lock")
	}
	var blocked []*deployrelease.Failure
	for range takers - 1 {
		select {
		case f := <-results:
			blocked = append(blocked, f)
		case <-time.After(barrierWait):
			require.Fail(t, "a taker did not give up while another held the lock")
		}
	}
	close(gate.release)
	wg.Wait()
	close(results)

	for _, f := range blocked {
		require.NotNil(t, f, "a loser of the takeover must fail, not run next to the winner")
		assert.Equal(t, deployrelease.ExitTargetLocked, f.Exit)
	}
	for winner := range results {
		assert.Nil(t, winner, "the one that took the lock deploys")
	}

	entries, err := os.ReadDir(r.path("/srv/app/.dhow"))
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".stale-", "the moved aside copy of the expired lock is removed")
		assert.NotContains(t, e.Name(), ".takeover", "the takeover gate is released")
	}
	assert.Len(t, r.releaseNames(), 1, "only the winner made a release")
}

type renewingRemote struct {
	*localRemote
	once  sync.Once
	fresh string
}

func (r *renewingRemote) ReadFile(name string, limit int64) ([]byte, error) {
	raw, err := r.localRemote.ReadFile(name, limit)
	if strings.HasSuffix(name, "/lock/owner") {
		r.once.Do(func() {
			_ = os.WriteFile(r.path(name), []byte(r.fresh), 0o600)
		})
	}
	return raw, err
}

func TestALockRenewedWhileItIsBeingTakenIsNeverDestroyed(t *testing.T) {
	r := newLocalRemote(t)
	require.NoError(t, os.MkdirAll(r.path("/srv/app/.dhow/lock"), 0o700))
	old := fmt.Sprintf(`{"Release":"9-zzzzzzzz-1","ExpiresAt":%q}`, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339))
	live := fmt.Sprintf(`{"Release":"9-zzzzzzzz-1","ExpiresAt":%q}`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	require.NoError(t, os.WriteFile(r.path("/srv/app/.dhow/lock/owner"), []byte(old), 0o600))

	path, digest := makeArchive(t, map[string]string{"index.html": "x"})
	mine := &renewingRemote{localRemote: &localRemote{t: t, root: r.root, free: r.free}, fresh: live}
	e := &deployrelease.Engine{
		Config: baseConfig(path, digest, "1-aaaaaaaa-1"), Doer: &probe{status: healthy}, Report: &recorder{}, Clock: fastClock(),
		Connect: func(context.Context) (deployrelease.Remote, error) { return mine, nil },
	}
	failure := e.Run(context.Background())

	require.NotNil(t, failure)
	assert.Equal(t, deployrelease.ExitTargetLocked, failure.Exit)
	assert.Equal(t, live, r.read("/srv/app/.dhow/lock/owner"), "the renewed lock is untouched")
	entries, err := os.ReadDir(r.path("/srv/app/.dhow"))
	require.NoError(t, err)
	for _, en := range entries {
		assert.NotContains(t, en.Name(), ".stale-")
		assert.NotContains(t, en.Name(), ".takeover")
	}
	assert.Empty(t, r.releaseNames())
}

func TestOldMovedAsideLocksAndAbandonedGatesAreSweptButFreshOnesStay(t *testing.T) {
	r := newLocalRemote(t)
	for _, name := range []string{"lock.stale-aaaa", "lock.stale-bbbb", "lock.takeover"} {
		require.NoError(t, os.MkdirAll(r.path("/srv/app/.dhow/"+name), 0o700))
	}
	old := time.Now().Add(-3 * time.Hour)
	require.NoError(t, os.Chtimes(r.path("/srv/app/.dhow/lock.stale-aaaa"), old, old))
	require.NoError(t, os.Chtimes(r.path("/srv/app/.dhow/lock.takeover"), old, old))

	path, digest := makeArchive(t, map[string]string{"index.html": "x"})
	e := &deployrelease.Engine{
		Config: baseConfig(path, digest, "1-aaaaaaaa-1"), Doer: &probe{status: healthy}, Report: &recorder{}, Clock: deployrelease.Clock{Now: time.Now, Sleep: func(ctx context.Context, _ time.Duration) error { return ctx.Err() }},
		Connect: func(context.Context) (deployrelease.Remote, error) { return r, nil },
	}
	require.Nil(t, e.Run(context.Background()))
	assert.NoDirExists(t, r.path("/srv/app/.dhow/lock.stale-aaaa"))
	assert.NoDirExists(t, r.path("/srv/app/.dhow/lock.takeover"))
	assert.DirExists(t, r.path("/srv/app/.dhow/lock.stale-bbbb"), "a recent copy may belong to a taker that is still working")
}
