package outbox_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	outbox "github.com/Khaymah/dhow/apps/server/internal/Core/Outbox"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

type probe struct {
	mu       sync.Mutex
	inflight map[uuid.UUID]int
	peak     map[uuid.UUID]int
	started  map[outbox.Kind]int
	hold     chan struct{}
}

func newProbe() *probe {
	return &probe{inflight: map[uuid.UUID]int{}, peak: map[uuid.UUID]int{}, started: map[outbox.Kind]int{}, hold: make(chan struct{})}
}

func (p *probe) Deliver(_ context.Context, item outbox.Item) outbox.Result {
	if item.Kind == outbox.KindInternalEmail {
		p.mu.Lock()
		p.started[item.Kind]++
		p.mu.Unlock()
		return outbox.Result{}
	}
	org := *item.OrganizationID
	p.mu.Lock()
	p.inflight[org]++
	p.peak[org] = max(p.peak[org], p.inflight[org])
	p.started[item.Kind]++
	p.mu.Unlock()
	<-p.hold
	p.mu.Lock()
	p.inflight[org]--
	p.mu.Unlock()
	return outbox.Result{}
}

func (p *probe) count(kind outbox.Kind) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.started[kind]
}

func (w world) otherOrg(t *testing.T, name string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	var org uuid.UUID
	require.NoError(t, w.pool.QueryRow(t.Context(), `INSERT INTO "Organizations" ("Name", "Slug") VALUES ($1, $1) RETURNING "Id"`, name).Scan(&org))
	return org, w.channelIn(t, org, name)
}

func (w world) channelIn(t *testing.T, org uuid.UUID, name string) uuid.UUID {
	t.Helper()
	var channel uuid.UUID
	require.NoError(t, w.pool.QueryRow(t.Context(), `INSERT INTO "NotificationChannels" ("OrganizationId", "Kind", "Name", "ConfigSealed", "CreatedByUserId")
		VALUES ($1, 'webhook', $2, '\x03', $3) RETURNING "Id"`, org, name, w.user).Scan(&channel))
	return channel
}

func (w world) queue(t *testing.T, org, channel uuid.UUID) {
	t.Helper()
	_, err := outbox.Enqueue(t.Context(), sqlc.New(w.pool), outbox.Entry{
		OrganizationID: org, ChannelID: &channel, Kind: outbox.KindWebhook, EventID: uuid.New(), EventName: "x", Payload: []byte(`{}`), NextAttemptAt: w.clock.Now(),
	})
	require.NoError(t, err)
}

func (w world) queueMail(t *testing.T) {
	t.Helper()
	_, err := outbox.Enqueue(t.Context(), sqlc.New(w.pool), outbox.Entry{
		Kind: outbox.KindInternalEmail, RecipientUserID: &w.user, EventID: uuid.New(), EventName: "mail", Payload: []byte(`{}`), NextAttemptAt: w.clock.Now(),
	})
	require.NoError(t, err)
}

func (w world) run(t *testing.T, o outbox.Options) (cancel func(), done <-chan struct{}) {
	t.Helper()
	o.Pool, o.Log, o.Now = w.pool, slog.New(slog.NewTextHandler(io.Discard, nil)), w.clock.Now
	worker, err := outbox.NewWorker(o)
	require.NoError(t, err)
	ctx, stop := context.WithCancel(t.Context())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		worker.Run(ctx)
	}()
	return stop, finished
}

func (w world) count(t *testing.T, query string) int {
	t.Helper()
	var n int
	require.NoError(t, w.pool.QueryRow(t.Context(), query).Scan(&n))
	return n
}

func TestOneOrganizationCannotTakeTheWholePoolAndAccountMailAlwaysGetsASlot(t *testing.T) {
	w := seed(t)
	orgB, channelB := w.otherOrg(t, "beta")
	for i := range 8 {
		w.queue(t, w.org, w.channelIn(t, w.org, "a"+string(rune('a'+i))))
	}
	w.queue(t, orgB, channelB)
	p := newProbe()
	stop, done := w.run(t, outbox.Options{
		Deliverers: map[outbox.Kind]outbox.Deliverer{outbox.KindWebhook: p, outbox.KindInternalEmail: p},
		Poll:       20 * time.Millisecond, Global: 5,
	})
	require.Eventually(t, func() bool { return p.count(outbox.KindWebhook) >= 5 }, 10*time.Second, 10*time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	p.mu.Lock()
	assert.Equal(t, 4, p.inflight[w.org], "the busy organization is held at its share")
	assert.Equal(t, 1, p.inflight[orgB], "the other organization still gets a slot while the first is saturated")
	p.mu.Unlock()

	w.queueMail(t)
	require.Eventually(t, func() bool { return p.count(outbox.KindInternalEmail) == 1 }, 10*time.Second, 10*time.Millisecond,
		"every webhook slot is taken, and the account email still goes out")
	close(p.hold)
	require.Eventually(t, func() bool {
		return w.count(t, `SELECT count(*) FROM "NotificationOutbox" WHERE "State" = 'sent'`) == 10
	}, 20*time.Second, 20*time.Millisecond)
	stop()
	<-done
	p.mu.Lock()
	defer p.mu.Unlock()
	assert.LessOrEqual(t, p.peak[w.org], 4)
}

type slow struct {
	started chan struct{}
	release chan struct{}
	result  outbox.Result
}

func (s slow) Deliver(_ context.Context, _ outbox.Item) outbox.Result {
	close(s.started)
	<-s.release
	return s.result
}

func TestAWorkerThatLostItsLeaseCannotSettleTheRowAnotherWorkerOwnsNow(t *testing.T) {
	w := seed(t)
	id := w.enqueue(t)
	stale := slow{started: make(chan struct{}), release: make(chan struct{}), result: failure}
	first := w.worker(t, stale, nil)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, _ = first.Drain(t.Context())
	}()
	<-stale.started
	w.clock.Advance(3 * time.Minute)
	second := w.worker(t, &script{}, nil)
	n, err := second.Drain(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	assert.Equal(t, "sent", w.row(t, id).State)

	close(stale.release)
	<-finished
	r := w.row(t, id)
	assert.Equal(t, "sent", r.State, "the late answer of the first worker changed nothing")
	assert.Equal(t, 2, r.Attempts)
	assert.Empty(t, r.Error)
}

type broken struct{}

func (broken) Deliver(_ context.Context, _ outbox.Item) outbox.Result {
	empty := []int{}
	index := len(empty)
	return outbox.Result{Status: empty[index]}
}

func TestAnAdapterThatFailsUnexpectedlyFailsTheAttemptAndNotTheServer(t *testing.T) {
	w := seed(t)
	id := w.enqueue(t)
	worker := w.worker(t, broken{}, nil)
	n, err := worker.Drain(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	r := w.row(t, id)
	assert.Equal(t, "pending", r.State)
	assert.Equal(t, "adapter_failed", r.Error)
	w.clock.Advance(time.Minute)
	n, err = worker.Drain(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the worker is still alive and tries again")
}

func TestShuttingDownSendsWaitingAccountMailAndNeverWaitsForAStuckWebhook(t *testing.T) {
	w := seed(t)
	w.enqueue(t)
	stuck := slow{started: make(chan struct{}), release: make(chan struct{}), result: outbox.Result{}}
	t.Cleanup(func() { close(stuck.release) })
	mail := &script{}
	stop, done := w.run(t, outbox.Options{
		Deliverers: map[outbox.Kind]outbox.Deliverer{outbox.KindWebhook: stuck, outbox.KindInternalEmail: mail},
		Poll:       time.Hour, ShutdownWait: 300 * time.Millisecond,
	})
	<-stuck.started
	w.queueMail(t)
	begun := time.Now()
	stop()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the worker waited for a stuck webhook")
	}
	assert.Less(t, time.Since(begun), 5*time.Second)
	assert.Equal(t, 1, mail.calls, "the account email that was waiting went out during the shutdown")
	assert.Equal(t, 1, w.count(t, `SELECT count(*) FROM "NotificationOutbox" WHERE "Kind" = 'internal-email' AND "State" = 'sent'`))
}
