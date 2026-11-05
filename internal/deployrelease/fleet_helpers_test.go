package deployrelease_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

const rolloutPause = 7 * time.Second

type flaky struct {
	*localRemote
	failRename func(oldName, newName string) error
}

func (f *flaky) Rename(oldName, newName string) error {
	if f.failRename != nil {
		if err := f.failRename(oldName, newName); err != nil {
			return err
		}
	}
	return f.localRemote.Rename(oldName, newName)
}

type rig struct {
	t       *testing.T
	remotes []*localRemote
	wrap    map[int]func(*localRemote) deployrelease.Remote
	refuse  map[int]error
	mu      sync.Mutex
	sleeps  []time.Duration
}

func newRig(t *testing.T, n int) *rig {
	t.Helper()
	r := &rig{t: t, wrap: map[int]func(*localRemote) deployrelease.Remote{}, refuse: map[int]error{}}
	for range n {
		r.remotes = append(r.remotes, newLocalRemote(t))
	}
	return r
}

func hostID(i int) string   { return fmt.Sprintf("0190a1b2-0000-7000-8000-00000000000%d", i+1) }
func hostName(i int) string { return fmt.Sprintf("web-%d.example.com", i+1) }

func (r *rig) host(i int) deployrelease.Host {
	return deployrelease.Host{
		ID: hostID(i), Name: hostName(i),
		Connect: func(context.Context) (deployrelease.Remote, error) {
			if err := r.refuse[i]; err != nil {
				return nil, err
			}
			if w := r.wrap[i]; w != nil {
				return w(r.remotes[i]), nil
			}
			return r.remotes[i], nil
		},
	}
}

func (r *rig) clock(onPause func()) deployrelease.Clock {
	c := fastClock()
	return deployrelease.Clock{Now: c.Now, Sleep: func(ctx context.Context, d time.Duration) error {
		if d == rolloutPause {
			r.mu.Lock()
			r.sleeps = append(r.sleeps, d)
			r.mu.Unlock()
			if onPause != nil {
				onPause()
			}
		}
		return ctx.Err()
	}}
}

type rollout struct {
	hosts         []int
	size          int
	pause         time.Duration
	cancelOnPause bool
	edit          func(*deployrelease.Config)
}

type fleetOutcome struct {
	failure *deployrelease.Failure
	events  *recorder
}

func (r *rig) roll(id, body string, doer deployrelease.Doer, o rollout) fleetOutcome {
	r.t.Helper()
	path, digest := makeArchive(r.t, map[string]string{"index.html": body})
	cfg := baseConfig(path, digest, id)
	cfg.Health.URL = "http://{host}/healthz"
	if o.edit != nil {
		o.edit(&cfg)
	}
	if o.hosts == nil {
		for i := range r.remotes {
			o.hosts = append(o.hosts, i)
		}
	}
	rec := &recorder{}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	var onPause func()
	if o.cancelOnPause {
		onPause = stop
	}
	f := &deployrelease.Fleet{
		Config: cfg, BatchSize: max(o.size, 1), BatchPause: o.pause, Doer: doer, Report: rec, Clock: r.clock(onPause),
	}
	for _, i := range o.hosts {
		f.Hosts = append(f.Hosts, r.host(i))
	}
	return fleetOutcome{failure: f.Run(ctx), events: rec}
}

func (r *rig) deployAll(id, body string, size int) fleetOutcome {
	r.t.Helper()
	out := r.roll(id, body, &probe{status: healthy}, rollout{size: size})
	require.Nil(r.t, out.failure, "the deploy of %s must succeed", id)
	return out
}

func (r *rig) current(i int) string { return r.remotes[i].currentTarget() }

func (r *rig) served(i int) string { return r.remotes[i].read("/srv/app/current/index.html") }

func (r *rig) lockHeld(i int) bool {
	_, err := os.Lstat(r.remotes[i].path("/srv/app/.dhow/lock"))
	return err == nil
}

func (r *recorder) forHost(id string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		if e.HostID == id {
			out = append(out, e.Event)
		}
	}
	return out
}

func (r *recorder) groupKinds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		if e.HostID == "" {
			out = append(out, e.Event)
		}
	}
	return out
}

func (r *recorder) index(kind, id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.IndexFunc(r.events, func(e deployrelease.Event) bool { return e.Event == kind && e.HostID == id })
}

func (r *recorder) eventOf(kind, id string) deployrelease.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e.Event == kind && e.HostID == id {
			return e
		}
	}
	return deployrelease.Event{}
}

func (r *recorder) groupEvent(kind string) deployrelease.Event {
	return r.eventOf(kind, "")
}

func unhealthyHosts(names ...string) func(int, string) int {
	return func(_ int, url string) int {
		for _, n := range names {
			if strings.Contains(url, n) {
				return 503
			}
		}
		return 200
	}
}
