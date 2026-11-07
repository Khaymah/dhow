package outbox_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	outbox "github.com/Khaymah/dhow/apps/server/internal/Core/Outbox"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type script struct {
	mu      sync.Mutex
	results []outbox.Result
	calls   int
	seen    []uuid.UUID
}

func (s *script) Deliver(_ context.Context, item outbox.Item) outbox.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.seen = append(s.seen, item.ID)
	if len(s.results) == 0 {
		return outbox.Result{}
	}
	r := s.results[0]
	if len(s.results) > 1 {
		s.results = s.results[1:]
	}
	return r
}

type world struct {
	pool    *pgxpool.Pool
	clock   *clock
	org     uuid.UUID
	channel uuid.UUID
	user    uuid.UUID
}

func seed(t *testing.T) world {
	t.Helper()
	pool := dbtest.NewPool(t)
	w := world{pool: pool, clock: &clock{t: time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)}}
	ctx := t.Context()
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO "Organizations" ("Name", "Slug") VALUES ('Acme', 'acme') RETURNING "Id"`).Scan(&w.org))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "Users" ("Email", "Name", "PasswordHash") VALUES ('a@example.com', 'A', 'x') RETURNING "Id"`).Scan(&w.user))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO "NotificationChannels" ("OrganizationId", "Kind", "Name", "ConfigSealed", "CreatedByUserId")
		VALUES ($1, 'webhook', 'hook', '\x01', $2) RETURNING "Id"`, w.org, w.user).Scan(&w.channel))
	return w
}

func (w world) enqueue(t *testing.T) uuid.UUID {
	t.Helper()
	event := uuid.New()
	ok, err := outbox.Enqueue(t.Context(), sqlc.New(w.pool), outbox.Entry{
		OrganizationID: w.org, ChannelID: &w.channel, Kind: outbox.KindWebhook, EventID: event, EventName: "deployment.finished",
		Payload: []byte(`{"Status":"failed"}`), NextAttemptAt: w.clock.Now(),
	})
	require.NoError(t, err)
	require.True(t, ok)
	var id uuid.UUID
	require.NoError(t, w.pool.QueryRow(t.Context(), `SELECT "Id" FROM "NotificationOutbox" WHERE "EventId" = $1`, event).Scan(&id))
	return id
}

func (w world) worker(t *testing.T, d outbox.Deliverer, observer outbox.Observer) *outbox.Worker {
	t.Helper()
	worker, err := outbox.NewWorker(outbox.Options{
		Pool: w.pool, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: w.clock.Now,
		Deliverers: map[outbox.Kind]outbox.Deliverer{outbox.KindWebhook: d}, Observer: observer,
	})
	require.NoError(t, err)
	return worker
}

type row struct {
	State    string
	Attempts int
	Next     time.Time
	Error    string
	Status   *int
}

func (w world) row(t *testing.T, id uuid.UUID) row {
	t.Helper()
	var r row
	require.NoError(t, w.pool.QueryRow(t.Context(),
		`SELECT "State", "Attempts", "NextAttemptAt", "LastError", "LastStatus" FROM "NotificationOutbox" WHERE "Id" = $1`, id,
	).Scan(&r.State, &r.Attempts, &r.Next, &r.Error, &r.Status))
	return r
}

var failure = outbox.Result{Code: "http_status", Status: 500}

func TestTheRetryScheduleFollowsTheBackoffWithAFakeClockAndEndsDeadAfterFiveAttempts(t *testing.T) {
	w := seed(t)
	id := w.enqueue(t)
	d := &script{results: []outbox.Result{failure}}
	worker := w.worker(t, d, nil)
	start := w.clock.Now()

	wait := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}
	for i, delay := range wait {
		n, err := worker.Drain(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 1, n, "attempt %d", i+1)
		r := w.row(t, id)
		assert.Equal(t, "pending", r.State)
		assert.Equal(t, i+1, r.Attempts)
		assert.True(t, w.clock.Now().Add(delay).Equal(r.Next), "the wait after attempt %d: %s", i+1, r.Next)
		assert.Equal(t, "http_status", r.Error)
		require.NotNil(t, r.Status)
		assert.Equal(t, 500, *r.Status)

		n, err = worker.Drain(t.Context())
		require.NoError(t, err)
		assert.Zero(t, n, "nothing is due before the wait is over")
		w.clock.Advance(delay - time.Second)
		n, err = worker.Drain(t.Context())
		require.NoError(t, err)
		assert.Zero(t, n, "one second early")
		w.clock.Advance(time.Second)
	}

	n, err := worker.Drain(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	r := w.row(t, id)
	assert.Equal(t, "dead", r.State)
	assert.Equal(t, 5, r.Attempts)
	assert.Equal(t, 5, d.calls)
	assert.Equal(t, start.Add(time.Minute+5*time.Minute+30*time.Minute+2*time.Hour), w.clock.Now())

	w.clock.Advance(24 * time.Hour)
	n, err = worker.Drain(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n, "a dead delivery is not tried again")
}

func TestThreeFailuresThenASuccessSendsOnTheFourthAttempt(t *testing.T) {
	w := seed(t)
	id := w.enqueue(t)
	d := &script{results: []outbox.Result{failure, failure, failure, {Status: 200}}}
	worker := w.worker(t, d, nil)
	for _, delay := range []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute} {
		_, err := worker.Drain(t.Context())
		require.NoError(t, err)
		w.clock.Advance(delay)
	}
	n, err := worker.Drain(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	r := w.row(t, id)
	assert.Equal(t, "sent", r.State)
	assert.Equal(t, 4, r.Attempts)
	assert.Empty(t, r.Error)
	assert.Equal(t, 4, d.calls)
	assert.Equal(t, 4, countOf(d.seen, id), "the delivery id stays the same across attempts")
}

func countOf(ids []uuid.UUID, id uuid.UUID) int {
	n := 0
	for _, v := range ids {
		if v == id {
			n++
		}
	}
	return n
}

func TestAPermanentFailureEndsDeadOnTheFirstAttempt(t *testing.T) {
	w := seed(t)
	id := w.enqueue(t)
	worker := w.worker(t, &script{results: []outbox.Result{{Code: "blocked", Permanent: true}}}, nil)
	_, err := worker.Drain(t.Context())
	require.NoError(t, err)
	r := w.row(t, id)
	assert.Equal(t, "dead", r.State)
	assert.Equal(t, 1, r.Attempts)
	assert.Equal(t, "blocked", r.Error)
}

type crashing struct{ started chan struct{} }

func (c crashing) Deliver(ctx context.Context, _ outbox.Item) outbox.Result {
	close(c.started)
	<-ctx.Done()
	return outbox.Result{Code: "canceled"}
}

func TestAClaimedDeliveryComesBackAfterTheLeaseWhenTheProcessDiesMidSend(t *testing.T) {
	w := seed(t)
	id := w.enqueue(t)
	ctx, cancel := context.WithCancel(t.Context())
	started := make(chan struct{})
	dying := w.worker(t, crashing{started: started}, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := dying.Drain(ctx)
		assert.Error(t, err)
	}()
	<-started
	cancel()
	<-done
	r := w.row(t, id)
	assert.Equal(t, "sending", r.State, "the row is left claimed, not lost")

	d := &script{}
	survivor := w.worker(t, d, nil)
	n, err := survivor.Drain(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n, "the lease is still held")
	w.clock.Advance(3 * time.Minute)
	n, err = survivor.Drain(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	r = w.row(t, id)
	assert.Equal(t, "sent", r.State)
	assert.Equal(t, 2, r.Attempts)
}

func TestTheSameEventAndChannelIsWrittenOnce(t *testing.T) {
	w := seed(t)
	event := uuid.New()
	entry := outbox.Entry{
		OrganizationID: w.org, ChannelID: &w.channel, Kind: outbox.KindWebhook, EventID: event, EventName: "deployment.finished",
		Payload: []byte(`{}`), NextAttemptAt: w.clock.Now(),
	}
	first, err := outbox.Enqueue(t.Context(), sqlc.New(w.pool), entry)
	require.NoError(t, err)
	second, err := outbox.Enqueue(t.Context(), sqlc.New(w.pool), entry)
	require.NoError(t, err)
	assert.True(t, first)
	assert.False(t, second)
}

func TestInvalidEntriesAreRefused(t *testing.T) {
	w := seed(t)
	base := outbox.Entry{
		OrganizationID: w.org, ChannelID: &w.channel, Kind: outbox.KindWebhook, EventID: uuid.New(), EventName: "x",
		Payload: []byte(`{}`), NextAttemptAt: w.clock.Now(),
	}
	tests := map[string]func(e *outbox.Entry){
		"no organization": func(e *outbox.Entry) { e.OrganizationID = uuid.Nil },
		"no event":        func(e *outbox.Entry) { e.EventID = uuid.Nil },
		"no name":         func(e *outbox.Entry) { e.EventName = "" },
		"no time":         func(e *outbox.Entry) { e.NextAttemptAt = time.Time{} },
		"not an object":   func(e *outbox.Entry) { e.Payload = []byte(`[1]`) },
		"not json":        func(e *outbox.Entry) { e.Payload = []byte(`{`) },
		"too big":         func(e *outbox.Entry) { e.Payload = append([]byte(`{"A":"`), make([]byte, outbox.PayloadMaxBytes)...) },
		"no channel":      func(e *outbox.Entry) { e.ChannelID = nil },
		"no recipient":    func(e *outbox.Entry) { e.Kind, e.ChannelID = outbox.KindInternalEmail, nil },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			e := base
			mutate(&e)
			_, err := outbox.Enqueue(t.Context(), sqlc.New(w.pool), e)
			require.Error(t, err)
		})
	}
}

type gate struct {
	mu      sync.Mutex
	current int
	peak    int
	byChan  map[uuid.UUID]int
	peakBy  map[uuid.UUID]int
	hold    chan struct{}
	entered atomic.Int32
}

func (g *gate) Deliver(_ context.Context, item outbox.Item) outbox.Result {
	g.mu.Lock()
	g.current++
	g.byChan[*item.ChannelID]++
	g.peak = max(g.peak, g.current)
	g.peakBy[*item.ChannelID] = max(g.peakBy[*item.ChannelID], g.byChan[*item.ChannelID])
	g.mu.Unlock()
	g.entered.Add(1)
	<-g.hold
	g.mu.Lock()
	g.current--
	g.byChan[*item.ChannelID]--
	g.mu.Unlock()
	return outbox.Result{Status: 200}
}

func TestTheWorkerKeepsToTheChannelAndGlobalConcurrencyLimits(t *testing.T) {
	w := seed(t)
	other := uuid.Nil
	require.NoError(t, w.pool.QueryRow(t.Context(), `INSERT INTO "NotificationChannels" ("OrganizationId", "Kind", "Name", "ConfigSealed", "CreatedByUserId")
		VALUES ($1, 'webhook', 'second', '\x01', $2) RETURNING "Id"`, w.org, w.user).Scan(&other))
	for range 6 {
		w.enqueue(t)
	}
	for range 6 {
		_, err := outbox.Enqueue(t.Context(), sqlc.New(w.pool), outbox.Entry{
			OrganizationID: w.org, ChannelID: &other, Kind: outbox.KindWebhook, EventID: uuid.New(), EventName: "x",
			Payload: []byte(`{}`), NextAttemptAt: w.clock.Now(),
		})
		require.NoError(t, err)
	}
	g := &gate{byChan: map[uuid.UUID]int{}, peakBy: map[uuid.UUID]int{}, hold: make(chan struct{})}
	worker, err := outbox.NewWorker(outbox.Options{
		Pool: w.pool, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: w.clock.Now, Poll: 20 * time.Millisecond,
		Deliverers: map[outbox.Kind]outbox.Deliverer{outbox.KindWebhook: g}, Global: 3, PerChannel: 2,
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()
	require.Eventually(t, func() bool { return g.entered.Load() >= 3 }, 10*time.Second, 10*time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	g.mu.Lock()
	assert.Equal(t, 3, g.current, "the global limit")
	g.mu.Unlock()
	close(g.hold)
	require.Eventually(t, func() bool {
		var n int
		require.NoError(t, w.pool.QueryRow(t.Context(), `SELECT count(*) FROM "NotificationOutbox" WHERE "State" = 'sent'`).Scan(&n))
		return n == 12
	}, 20*time.Second, 20*time.Millisecond)
	cancel()
	<-done
	g.mu.Lock()
	defer g.mu.Unlock()
	assert.LessOrEqual(t, g.peak, 3)
	for id, peak := range g.peakBy {
		assert.LessOrEqual(t, peak, 2, "channel %s", id)
	}
}

func TestOnlyOneWorkerHoldsTheInstanceLock(t *testing.T) {
	w := seed(t)
	for range 3 {
		w.enqueue(t)
	}
	d := &script{}
	first := w.worker(t, d, nil)
	second := w.worker(t, d, nil)
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	for _, worker := range []*outbox.Worker{first, second} {
		wg.Go(func() { worker.Run(ctx) })
	}
	require.Eventually(t, func() bool {
		var n int
		require.NoError(t, w.pool.QueryRow(t.Context(), `SELECT count(*) FROM "NotificationOutbox" WHERE "State" = 'sent'`).Scan(&n))
		return n == 3
	}, 20*time.Second, 20*time.Millisecond)
	var holders int
	require.NoError(t, w.pool.QueryRow(t.Context(),
		`SELECT count(DISTINCT pid) FROM pg_locks WHERE locktype = 'advisory' AND granted`).Scan(&holders))
	assert.Equal(t, 1, holders, "one connection holds the instance lock")
	cancel()
	wg.Wait()
	assert.Equal(t, 3, d.calls)
}

type recorder struct {
	mu     sync.Mutex
	finals []bool
	codes  []string
}

func (r *recorder) Settled(_ context.Context, _ outbox.Item, res outbox.Result, final bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finals = append(r.finals, final)
	r.codes = append(r.codes, res.Code)
}

func TestTheObserverSeesEveryAttemptAndWhichOneWasTheLast(t *testing.T) {
	w := seed(t)
	w.enqueue(t)
	rec := &recorder{}
	worker := w.worker(t, &script{results: []outbox.Result{failure, {Code: "blocked", Permanent: true}}}, rec)
	_, err := worker.Drain(t.Context())
	require.NoError(t, err)
	w.clock.Advance(time.Minute)
	_, err = worker.Drain(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []bool{false, true}, rec.finals)
	assert.Equal(t, []string{"http_status", "blocked"}, rec.codes)
}

func TestRequeueOnlyRevivesADeadDelivery(t *testing.T) {
	w := seed(t)
	id := w.enqueue(t)
	ok, err := outbox.Requeue(t.Context(), sqlc.New(w.pool), w.org, id, w.clock.Now())
	require.NoError(t, err)
	assert.False(t, ok, "a pending delivery is not requeued")
	worker := w.worker(t, &script{results: []outbox.Result{{Code: "blocked", Permanent: true}, {}}}, nil)
	_, err = worker.Drain(t.Context())
	require.NoError(t, err)
	ok, err = outbox.Requeue(t.Context(), sqlc.New(w.pool), uuid.New(), id, w.clock.Now())
	require.NoError(t, err)
	assert.False(t, ok, "another organization cannot requeue it")
	ok, err = outbox.Requeue(t.Context(), sqlc.New(w.pool), w.org, id, w.clock.Now())
	require.NoError(t, err)
	assert.True(t, ok)
	_, err = worker.Drain(t.Context())
	require.NoError(t, err)
	r := w.row(t, id)
	assert.Equal(t, "sent", r.State)
	assert.Equal(t, 1, r.Attempts, "the attempt count starts again")
}
