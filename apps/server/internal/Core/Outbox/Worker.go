package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

var ErrNoPool = errors.New("the outbox worker needs a database pool")

type Options struct {
	Pool         *pgxpool.Pool
	Log          *slog.Logger
	Now          func() time.Time
	Deliverers   map[Kind]Deliverer
	Observer     Observer
	Global       int
	PerChannel   int
	PerOrg       int
	MailSlots    int
	ShutdownWait time.Duration
	Lease        time.Duration
	Poll         time.Duration
	Schedule     []time.Duration
	MaxAttempts  int
	LockKey      int64
}

type Worker struct {
	pool         *pgxpool.Pool
	log          *slog.Logger
	now          func() time.Time
	deliverers   map[Kind]Deliverer
	observer     Observer
	global       int
	perChannel   int
	perOrg       int
	mailSlots    int
	shutdownWait time.Duration
	lease        time.Duration
	poll         time.Duration
	schedule     []time.Duration
	maxAttempts  int
	lockKey      int64

	mu      sync.Mutex
	running int
	mail    int
	byChan  map[uuid.UUID]int
	byOrg   map[uuid.UUID]int
	wg      sync.WaitGroup
	wake    chan struct{}
	lock    *pgxpool.Conn
}

func NewWorker(o Options) (*Worker, error) {
	if o.Pool == nil {
		return nil, ErrNoPool
	}
	w := &Worker{
		pool: o.Pool, log: o.Log, now: o.Now, deliverers: o.Deliverers, observer: o.Observer, global: o.Global, perChannel: o.PerChannel, perOrg: o.PerOrg, mailSlots: o.MailSlots, shutdownWait: o.ShutdownWait,
		lease: o.Lease, poll: o.Poll, schedule: o.Schedule, maxAttempts: o.MaxAttempts, lockKey: o.LockKey,
		byChan: map[uuid.UUID]int{}, byOrg: map[uuid.UUID]int{}, wake: make(chan struct{}, 1),
	}
	if w.now == nil {
		w.now = time.Now
	}
	if w.global <= 0 {
		w.global = config.NotifyGlobalConcurrency
	}
	if w.perChannel <= 0 {
		w.perChannel = config.NotifyChannelConcurrency
	}
	if w.perOrg <= 0 {
		w.perOrg = config.NotifyOrgConcurrency
	}
	if w.mailSlots <= 0 {
		w.mailSlots = config.NotifyMailSlots
	}
	if w.shutdownWait <= 0 {
		w.shutdownWait = config.NotifyShutdownWait
	}
	if w.lease <= 0 {
		w.lease = config.NotifyClaimLease
	}
	if w.poll <= 0 {
		w.poll = config.NotifyPollInterval
	}
	if len(w.schedule) == 0 {
		w.schedule = config.NotifyRetrySchedule()
	}
	if w.maxAttempts <= 0 {
		w.maxAttempts = config.NotifyMaxAttempts
	}
	if w.lockKey == 0 {
		w.lockKey = config.NotifyLockKey
	}
	return w, nil
}

func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Worker) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			w.shutdown(ctx)
			return
		case <-timer.C:
		case <-w.wake:
		}
		if w.leader(ctx) {
			w.fill(ctx)
		}
		timer.Reset(w.poll)
	}
}

func (w *Worker) leader(ctx context.Context) bool {
	if w.lock != nil {
		if err := w.lock.Ping(ctx); err == nil {
			return true
		}
		w.drop(ctx)
	}
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		w.log.WarnContext(ctx, "notification worker cannot reach the database", "error", err)
		return false
	}
	locked, err := sqlc.New(conn).TryOutboxLock(ctx, w.lockKey)
	if err != nil || !locked {
		conn.Release()
		return false
	}
	w.lock = conn
	return true
}

func (w *Worker) drop(ctx context.Context) {
	if w.lock == nil {
		return
	}
	if err := w.lock.Hijack().Close(context.WithoutCancel(ctx)); err != nil {
		w.log.Warn("notification worker closed a broken lock connection", "error", err)
	}
	w.lock = nil
}

func (w *Worker) release(ctx context.Context) {
	if w.lock == nil {
		return
	}
	ctx = context.WithoutCancel(ctx)
	released, err := sqlc.New(w.lock).ReleaseOutboxLock(ctx, w.lockKey)
	if err != nil || !released {
		w.drop(ctx)
		return
	}
	w.lock.Release()
	w.lock = nil
}

func (w *Worker) shutdown(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(w.shutdownWait):
		w.log.Warn("notification worker stopped while deliveries were still running; their leases expire and they are tried again")
	}
	if w.lock != nil {
		flush, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.shutdownWait)
		if _, err := w.drain(flush, true); err != nil {
			w.log.Warn("notification worker could not send the account emails that were waiting", "error", err)
		}
		cancel()
	}
	w.release(ctx)
}

type limits struct {
	channels []uuid.UUID
	orgs     []uuid.UUID
}

func (w *Worker) limits(internal bool) (limits, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if internal {
		return limits{channels: []uuid.UUID{}, orgs: []uuid.UUID{}}, w.mail < w.mailSlots
	}
	if w.running >= w.global {
		return limits{}, false
	}
	l := limits{channels: []uuid.UUID{}, orgs: []uuid.UUID{}}
	for id, n := range w.byChan {
		if n >= w.perChannel {
			l.channels = append(l.channels, id)
		}
	}
	for id, n := range w.byOrg {
		if n >= w.perOrg {
			l.orgs = append(l.orgs, id)
		}
	}
	return l, true
}

func (w *Worker) fill(ctx context.Context) {
	for ctx.Err() == nil {
		item, ok, err := w.next(ctx)
		if err != nil {
			w.log.ErrorContext(ctx, "notification worker cannot claim a delivery", "error", err)
			return
		}
		if !ok {
			return
		}
		w.start(ctx, item)
	}
}

func (w *Worker) next(ctx context.Context) (Item, bool, error) {
	for _, internal := range []bool{true, false} {
		l, free := w.limits(internal)
		if !free {
			continue
		}
		item, ok, err := w.claim(ctx, internal, l)
		if err != nil || ok {
			return item, ok, err
		}
	}
	return Item{}, false, nil
}

func (w *Worker) start(ctx context.Context, item Item) {
	w.mu.Lock()
	if item.Kind == KindInternalEmail {
		w.mail++
	} else {
		w.running++
	}
	if item.ChannelID != nil {
		w.byChan[*item.ChannelID]++
	}
	if item.OrganizationID != nil && item.Kind != KindInternalEmail {
		w.byOrg[*item.OrganizationID]++
	}
	w.mu.Unlock()
	w.wg.Go(func() {
		defer func() {
			w.finish(item)
			w.Wake()
		}()
		w.process(ctx, item)
	})
}

func (w *Worker) finish(item Item) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if item.Kind == KindInternalEmail {
		w.mail--
	} else {
		w.running--
	}
	if item.ChannelID != nil {
		if w.byChan[*item.ChannelID]--; w.byChan[*item.ChannelID] <= 0 {
			delete(w.byChan, *item.ChannelID)
		}
	}
	if item.OrganizationID != nil && item.Kind != KindInternalEmail {
		if w.byOrg[*item.OrganizationID]--; w.byOrg[*item.OrganizationID] <= 0 {
			delete(w.byOrg, *item.OrganizationID)
		}
	}
}

func (w *Worker) claim(ctx context.Context, internal bool, l limits) (Item, bool, error) {
	now := w.now()
	if l.channels == nil {
		l.channels = []uuid.UUID{}
	}
	if l.orgs == nil {
		l.orgs = []uuid.UUID{}
	}
	row, err := sqlc.New(w.pool).ClaimOutbox(ctx, sqlc.ClaimOutboxParams{
		Now: now, LeaseUntil: now.Add(w.lease), Internal: internal, Busy: l.channels, BusyOrgs: l.orgs,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, false, nil
	}
	if err != nil {
		return Item{}, false, fmt.Errorf("claim a delivery: %w", err)
	}
	return itemOf(row), true, nil
}

func (w *Worker) Drain(ctx context.Context) (int, error) { return w.drain(ctx, false) }

func (w *Worker) DrainInternal(ctx context.Context) (int, error) { return w.drain(ctx, true) }

func (w *Worker) drain(ctx context.Context, onlyInternal bool) (int, error) {
	n := 0
	for ctx.Err() == nil {
		item, ok, err := w.claim(ctx, true, limits{})
		if err == nil && !ok && !onlyInternal {
			item, ok, err = w.claim(ctx, false, limits{})
		}
		if err != nil || !ok {
			return n, err
		}
		w.process(ctx, item)
		n++
	}
	return n, ctx.Err()
}

func (w *Worker) Wait() { w.wg.Wait() }

func (w *Worker) process(ctx context.Context, item Item) {
	if item.Attempts > w.maxAttempts {
		w.settle(ctx, item, Result{Code: ErrLeaseExpired, Permanent: true})
		return
	}
	d, ok := w.deliverers[item.Kind]
	if !ok {
		w.settle(ctx, item, Result{Code: ErrNoAdapter, Permanent: true})
		return
	}
	res := w.deliver(ctx, d, item)
	if ctx.Err() != nil && !res.OK() {
		return
	}
	w.settle(ctx, item, res)
}

func (w *Worker) deliver(ctx context.Context, d Deliverer, item Item) (res Result) {
	defer func() {
		if recover() != nil {
			w.log.ErrorContext(ctx, "a notification adapter failed unexpectedly", "delivery_id", item.ID.String(), "kind", string(item.Kind))
			res = Result{Code: ErrAdapterFailed}
		}
	}()
	return d.Deliver(ctx, item)
}

func statusOf(n int) pgtype.Int4 {
	if n < 100 || n > 599 {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(n), Valid: true}
}

func (w *Worker) settle(ctx context.Context, item Item, res Result) {
	ctx = context.WithoutCancel(ctx)
	now := w.now()
	q := sqlc.New(w.pool)
	attempts := int32(item.Attempts) //nolint:gosec // Bounded by the attempt limit.
	final := false
	var n int64
	var err error
	switch {
	case res.OK():
		n, err = q.CompleteOutbox(ctx, sqlc.CompleteOutboxParams{ID: item.ID, Now: now, Attempts: attempts, Status: statusOf(res.Status)})
	case Final(item.Attempts, w.maxAttempts, res):
		final = true
		n, err = q.KillOutbox(ctx, sqlc.KillOutboxParams{ID: item.ID, Now: now, Attempts: attempts, LastError: res.Code, Status: statusOf(res.Status)})
	default:
		n, err = q.RescheduleOutbox(ctx, sqlc.RescheduleOutboxParams{
			ID: item.ID, Now: now, Attempts: attempts, NextAttemptAt: now.Add(NextDelay(w.schedule, item.Attempts)), LastError: res.Code, Status: statusOf(res.Status),
		})
	}
	if err != nil {
		w.log.ErrorContext(ctx, "notification worker cannot record an attempt", "delivery_id", item.ID.String(), "error", err)
		return
	}
	if n == 0 {
		w.log.WarnContext(ctx, "notification attempt ignored: the delivery was claimed again meanwhile", "delivery_id", item.ID.String())
		return
	}
	if w.observer != nil {
		w.observer.Settled(ctx, item, res, final)
	}
}
