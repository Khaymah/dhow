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

const barrierWait = 10 * time.Second

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
	seen  *barrier
	moved *barrier
	mu    sync.Mutex
	hits  int
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
		first := r.c.hits <= 2
		r.c.mu.Unlock()
		if first {
			r.c.seen.wait()
		}
	}
	return raw, err
}

func (r *contendedRemote) Rename(oldName, newName string) error {
	err := r.localRemote.Rename(oldName, newName)
	if strings.HasSuffix(oldName, "/.dhow/lock") {
		r.c.moved.wait()
	}
	return err
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

func TestTwoTakersOfAnExpiredLockNeverBothWin(t *testing.T) {
	r := newLocalRemote(t)
	require.NoError(t, os.MkdirAll(r.path("/srv/app/.dhow/lock"), 0o700))
	old := fmt.Sprintf(`{"Release":"9-zzzzzzzz-1","ExpiresAt":%q}`, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339))
	require.NoError(t, os.WriteFile(r.path("/srv/app/.dhow/lock/owner"), []byte(old), 0o600))
	shared := &contention{seen: newBarrier(2), moved: newBarrier(2)}
	gate := &gatedHealth{reached: make(chan struct{}), release: make(chan struct{})}

	results := make(chan *deployrelease.Failure, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"1-aaaaaaaa-1", "2-bbbbbbbb-1"} {
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
	var blocked *deployrelease.Failure
	select {
	case blocked = <-results:
	case <-time.After(barrierWait):
		require.Fail(t, "the second taker did not give up while the first held the lock")
	}
	close(gate.release)
	wg.Wait()
	close(results)

	require.NotNil(t, blocked, "the loser of the takeover must fail, not run next to the winner")
	assert.Equal(t, deployrelease.ExitTargetLocked, blocked.Exit)
	for winner := range results {
		assert.Nil(t, winner, "the one that took the lock deploys")
	}

	entries, err := os.ReadDir(r.path("/srv/app/.dhow"))
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".stale-", "the moved aside copy of the expired lock is removed")
	}
	assert.Len(t, r.releaseNames(), 1, "only the winner made a release")
}
