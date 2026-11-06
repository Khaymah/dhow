package deployrelease

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	UploadParallel   = 4
	MaxRollingBudget = 60 * time.Minute
)

var ErrRollout = errors.New("the rollout settings are not valid")

type Host struct {
	ID      string
	Name    string
	Connect func(ctx context.Context) (Remote, error)
}

type Fleet struct {
	Config     Config
	Hosts      []Host
	BatchSize  int
	BatchPause time.Duration
	Doer       Doer
	Report     Reporter
	Log        Logger
	Clock      Clock

	LockRenew      time.Duration
	LockTimeout    time.Duration
	RollbackBudget time.Duration
	CleanupBudget  time.Duration
}

func ValidateRollout(hosts, batchSize int, pause time.Duration, h Health) error {
	switch {
	case hosts < 1 || hosts > jobspec.MaxTargetHosts:
		return fmt.Errorf("%w: the number of servers", ErrRollout)
	case batchSize < 1 || batchSize > hosts:
		return fmt.Errorf("%w: the batch size", ErrRollout)
	case pause < 0 || pause > jobspec.MaxBatchPause*time.Second:
		return fmt.Errorf("%w: the pause between batches", ErrRollout)
	}
	if err := h.Validate(); err != nil {
		return err
	}
	batches := (hosts + batchSize - 1) / batchSize
	perBatch := time.Duration(0)
	if h.Enabled() {
		perBatch = h.Budget()
	}
	if time.Duration(batches)*perBatch+time.Duration(batches-1)*pause > MaxRollingBudget {
		return fmt.Errorf("%w: the whole rollout may not take more than 60 minutes", ErrRollout)
	}
	return nil
}

func (f *Fleet) validate() error {
	seen := map[string]bool{}
	for _, h := range f.Hosts {
		if h.ID == "" || h.Name == "" || h.Connect == nil || seen[h.ID] {
			return fmt.Errorf("%w: a server", ErrRollout)
		}
		seen[h.ID] = true
	}
	if err := ValidateRollout(len(f.Hosts), f.BatchSize, f.BatchPause, f.Config.Health); err != nil {
		return err
	}
	cfg := f.Config
	cfg.HostID, cfg.HostName = f.Hosts[0].ID, f.Hosts[0].Name
	return cfg.Validate()
}

func (f *Fleet) Run(ctx context.Context) *Failure {
	g := f.newGroup()
	defer g.release()
	if err := f.validate(); err != nil {
		return g.stop(ctx, fail(ExitInput, CauseOther, "the deployment settings are not valid", err), nil)
	}
	return g.execute(ctx)
}

type syncReporter struct {
	mu sync.Mutex
	r  Reporter
}

func (s *syncReporter) Emit(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.r.Emit(e)
}

type hostLog struct {
	Logger
	host string
}

func (h hostLog) Info(msg string, args ...any) {
	h.Logger.Info(msg, append(args, "host", h.host)...)
}

func (h hostLog) Warn(msg string, args ...any) {
	h.Logger.Warn(msg, append(args, "host", h.host)...)
}

type member struct {
	host       Host
	d          *run
	staging    bool
	err        *Failure
	collateral bool
}

type group struct {
	f       *Fleet
	members []*member
	rep     Reporter
	log     Logger
	started bool
}

func (f *Fleet) newGroup() *group {
	if f.Report == nil {
		f.Report = DiscardReporter{}
	}
	if f.Clock.Now == nil {
		f.Clock = RealClock()
	}
	if f.Log == nil {
		f.Log = slog.Default()
	}
	g := &group{f: f, rep: &syncReporter{r: f.Report}, log: f.Log}
	for _, h := range f.Hosts {
		cfg := f.Config
		cfg.HostID, cfg.HostName = h.ID, h.Name
		e := &Engine{
			Config: cfg, Connect: h.Connect, Doer: f.Doer, Report: g.rep, Log: hostLog{Logger: f.Log, host: h.Name},
			Clock: f.Clock, LockRenew: f.LockRenew, LockTimeout: f.LockTimeout,
		}
		g.members = append(g.members, &member{host: h, d: newRun(e)})
	}
	return g
}

func (g *group) release() {
	var wg sync.WaitGroup
	for _, m := range g.members {
		wg.Go(func() {
			m.d.unlock()
			if m.d.r != nil {
				ctx, cancel := context.WithTimeout(context.Background(), m.d.lockTimeout())
				defer cancel()
				_ = boundRemote{Remote: m.d.r, ctx: ctx}.Close()
			}
		})
	}
	wg.Wait()
}

func (g *group) releaseID() string { return g.f.Config.ReleaseID }

func (g *group) previous() string {
	for _, m := range g.members {
		if m.d.known != "" {
			return m.d.known
		}
	}
	return ""
}

func (g *group) emitStarted() {
	if g.started {
		return
	}
	g.started = true
	g.rep.Emit(Event{Event: EventStarted, ReleaseID: g.releaseID(), PreviousReleaseID: g.previous(), Mode: ModeDeploy})
}

func (g *group) execute(ctx context.Context) *Failure {
	totals, f := g.members[0].d.scanArchive()
	if f != nil {
		return g.stop(ctx, f, nil)
	}
	for _, m := range g.members {
		m.d.totals = totals
	}
	if f, bad := g.preflight(ctx); f != nil {
		return g.stop(ctx, f, bad)
	}
	g.emitStarted()
	if f, bad := g.parallel(ctx, g.members, UploadParallel, func(ctx context.Context, m *member) *Failure {
		m.staging = true
		return m.d.stageRelease(ctx)
	}); f != nil {
		return g.stop(ctx, f, bad)
	}
	return g.rollout(ctx)
}

func (g *group) preflight(ctx context.Context) (*Failure, *member) {
	for _, m := range g.members {
		r, err := m.host.Connect(ctx)
		if err != nil {
			return connectFailure(err), m
		}
		m.d.r = r
		if f := m.d.preflight(ctx); f != nil {
			return f, m
		}
	}
	return nil, nil
}

func (g *group) parallel(ctx context.Context, ms []*member, limit int, fn func(context.Context, *member) *Failure) (*Failure, *member) {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, max(limit, 1))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first *Failure
	var culprit *member
	started := 0
	for _, m := range ms {
		select {
		case sem <- struct{}{}:
		case <-cctx.Done():
		}
		if cctx.Err() != nil {
			break
		}
		started++
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			f := fn(cctx, m)
			if f == nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			m.err = f
			m.collateral = first != nil && cctx.Err() != nil
			if first == nil {
				first, culprit = f, m
				cancel()
			}
		}()
	}
	wg.Wait()
	if first == nil && started < len(ms) {
		return fail(ExitFailed, CauseCanceled, "the deployment was canceled", ctx.Err()), nil
	}
	return first, culprit
}

func (g *group) rollout(ctx context.Context) *Failure {
	batches := g.batches()
	for i, batch := range batches {
		n := i + 1
		if f, bad := g.runBatch(ctx, batch, n); f != nil {
			return g.stop(ctx, f, bad)
		}
		if len(g.members) > 1 {
			g.rep.Emit(Event{Event: EventBatchPassed, ReleaseID: g.releaseID(), Batch: n})
		}
		if i == len(batches)-1 {
			break
		}
		if err := g.f.Clock.Sleep(ctx, g.f.BatchPause); err != nil {
			return g.stop(ctx, fail(ExitFailed, CauseCanceled, "the deployment was canceled", err), nil)
		}
	}
	return g.commit(ctx)
}

func PlanBatches(hosts, size int) [][2]int {
	if hosts < 1 || size < 1 {
		return nil
	}
	var out [][2]int
	for from := 0; from < hosts; from += size {
		out = append(out, [2]int{from, min(from+size, hosts)})
	}
	return out
}

func (g *group) batches() [][]*member {
	var out [][]*member
	for _, r := range PlanBatches(len(g.members), g.f.BatchSize) {
		out = append(out, g.members[r[0]:r[1]])
	}
	return out
}

func (g *group) sharedHealth() bool {
	h := g.f.Config.Health
	return h.Enabled() && !strings.Contains(h.URL, jobspec.HealthHostPlaceholder)
}

func (g *group) runBatch(ctx context.Context, batch []*member, n int) (*Failure, *member) {
	shared := g.sharedHealth()
	f, bad := g.parallel(ctx, batch, len(batch), func(ctx context.Context, m *member) *Failure {
		if f := m.d.switchOver(n); f != nil {
			return f
		}
		if f := m.d.activate(ctx); f != nil {
			return f
		}
		if shared {
			return nil
		}
		return m.d.checkHealth(ctx)
	})
	if f != nil || !shared {
		return f, bad
	}
	res, err := CheckHealth(ctx, g.f.Doer, batch[0].d.cfg.HostName, g.f.Config.Health, g.f.Clock)
	var first *Failure
	var culprit *member
	for _, m := range batch {
		if f := m.d.healthVerdict(ctx, res, err); f != nil && first == nil {
			first, culprit = f, m
		}
	}
	return first, culprit
}

func (g *group) commit(ctx context.Context) *Failure {
	if err := ctx.Err(); err != nil {
		return g.stop(ctx, fail(ExitFailed, CauseCanceled, "the deployment was canceled", err), nil)
	}
	for _, m := range g.members {
		if err := m.d.markVerified(); err != nil {
			return g.stop(ctx, fail(ExitFailed, CauseOther, "cannot record the verified release", err), nil)
		}
	}
	for _, m := range g.members {
		m.d.completeRelease()
	}
	g.rep.Emit(Event{Event: EventVerified, ReleaseID: g.releaseID()})
	for _, m := range g.members {
		m.d.pruneOld()
	}
	return nil
}
