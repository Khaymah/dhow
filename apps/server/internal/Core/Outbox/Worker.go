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
	Pool        *pgxpool.Pool
	Log         *slog.Logger
	Now         func() time.Time
	Deliverers  map[Kind]Deliverer
	Observer    Observer
	Global      int
	PerChannel  int
	Lease       time.Duration
	Poll        time.Duration
	Schedule    []time.Duration
	MaxAttempts int
	LockKey     int64
}

type Worker struct {
	pool        *pgxpool.Pool
	log         *slog.Logger
	now         func() time.Time
	deliverers  map[Kind]Deliverer
	observer    Observer
	global      int
	perChannel  int
	lease       time.Duration
	poll        time.Duration
	schedule    []time.Duration
	maxAttempts int
	lockKey     int64

	mu      sync.Mutex
	running int
	byChan  map[uuid.UUID]int
	wg      sync.WaitGroup
	wake    chan struct{}
	lock    *pgxpool.Conn
}

func NewWorker(o Options) (*Worker, error) {
	if o.Pool == nil {
		return nil, ErrNoPool
	}
	w := &Worker{
		pool: o.Pool, log: o.Log, now: o.Now, deliverers: o.Deliverers, observer: o.Observer, global: o.Global, perChannel: o.PerChannel,
		lease: o.Lease, poll: o.Poll, schedule: o.Schedule, maxAttempts: o.MaxAttempts, lockKey: o.LockKey,
		byChan: map[uuid.UUID]int{}, wake: make(chan struct{}, 1),
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
			w.wg.Wait()
			w.release(ctx)
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

func (w *Worker) busy() ([]uuid.UUID, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running >= w.global {
		return nil, false
	}
	out := make([]uuid.UUID, 0, len(w.byChan))
	for id, n := range w.byChan {
		if n >= w.perChannel {
			out = append(out, id)
		}
	}
	return out, true
}

func (w *Worker) fill(ctx context.Context) {
	for ctx.Err() == nil {
		busy, free := w.busy()
		if !free {
			return
		}
		item, ok, err := w.claim(ctx, busy)
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

func (w *Worker) start(ctx context.Context, item Item) {
	w.mu.Lock()
	w.running++
	if item.ChannelID != nil {
		w.byChan[*item.ChannelID]++
	}
	w.mu.Unlock()
	w.wg.Go(func() {
		defer func() {
			w.mu.Lock()
			w.running--
			if item.ChannelID != nil {
				if w.byChan[*item.ChannelID]--; w.byChan[*item.ChannelID] <= 0 {
					delete(w.byChan, *item.ChannelID)
				}
			}
			w.mu.Unlock()
			w.Wake()
		}()
		w.process(ctx, item)
	})
}

func (w *Worker) claim(ctx context.Context, busy []uuid.UUID) (Item, bool, error) {
	now := w.now()
	if busy == nil {
		busy = []uuid.UUID{}
	}
	row, err := sqlc.New(w.pool).ClaimOutbox(ctx, sqlc.ClaimOutboxParams{Now: now, LeaseUntil: now.Add(w.lease), Busy: busy})
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, false, nil
	}
	if err != nil {
		return Item{}, false, fmt.Errorf("claim a delivery: %w", err)
	}
	return itemOf(row), true, nil
}

func (w *Worker) Drain(ctx context.Context) (int, error) {
	n := 0
	for ctx.Err() == nil {
		item, ok, err := w.claim(ctx, nil)
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
	res := d.Deliver(ctx, item)
	if ctx.Err() != nil && !res.OK() {
		return
	}
	w.settle(ctx, item, res)
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
	final := false
	var err error
	switch {
	case res.OK():
		_, err = q.CompleteOutbox(ctx, sqlc.CompleteOutboxParams{ID: item.ID, Now: now, Status: statusOf(res.Status)})
	case Final(item.Attempts, w.maxAttempts, res):
		final = true
		_, err = q.KillOutbox(ctx, sqlc.KillOutboxParams{ID: item.ID, Now: now, LastError: res.Code, Status: statusOf(res.Status)})
	default:
		_, err = q.RescheduleOutbox(ctx, sqlc.RescheduleOutboxParams{
			ID: item.ID, Now: now, NextAttemptAt: now.Add(NextDelay(w.schedule, item.Attempts)), LastError: res.Code, Status: statusOf(res.Status),
		})
	}
	if err != nil {
		w.log.ErrorContext(ctx, "notification worker cannot record an attempt", "delivery_id", item.ID.String(), "error", err)
		return
	}
	if w.observer != nil {
		w.observer.Settled(ctx, item, res, final)
	}
}
