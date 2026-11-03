package backfill_test

import (
	"context"
	"math/rand/v2"
	"os"
	"os/exec"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	backfill "github.com/Khaymah/dhow/apps/server/internal/Core/Backfill"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

const (
	envWorkerSchema = "DHOW_TEST_BACKFILL_WORKER_SCHEMA"
	killRows        = 100_000
	killBatch       = 1000
)

func TestBackfillWorkerProcess(t *testing.T) {
	schema := os.Getenv(envWorkerSchema)
	if schema == "" {
		t.Skip("runs only as the child of TestABackfillOf100000RowsSurvivesThreeKills")
	}
	cfg, err := pgxpool.ParseConfig(config.TestDatabaseURL())
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	defer pool.Close()
	r, err := backfill.New(backfill.Options{
		DB: pool, Jobs: []backfill.Job{&rowsJob{hold: 15 * time.Millisecond}}, BatchSize: killBatch,
		Pause: 5 * time.Millisecond, Every: 10 * time.Millisecond,
	})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for ctx.Err() == nil {
		require.NoError(t, r.Drain(ctx))
		if err := backfill.RequireDone(ctx, pool, jobName); err == nil {
			return
		}
	}
}

func startWorker(t *testing.T, schema string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestBackfillWorkerProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), envWorkerSchema+"="+schema)
	require.NoError(t, cmd.Start())
	return cmd
}

func waitForProcessed(t *testing.T, pool *pgxpool.Pool, atLeast int64) {
	t.Helper()
	require.Eventually(t, func() bool { return progress(t, pool).Processed >= atLeast }, time.Minute, 5*time.Millisecond)
}

type latencies struct {
	mu  sync.Mutex
	all []time.Duration
}

func (l *latencies) add(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.all = append(l.all, d)
}

func (l *latencies) p99() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.all) == 0 {
		return 0
	}
	s := slices.Clone(l.all)
	slices.Sort(s)
	return s[len(s)*99/100]
}

func serve(ctx context.Context, t *testing.T, pool *pgxpool.Pool, into *latencies) {
	for ctx.Err() == nil {
		id := rand.Int64N(killRows) + 1
		start := time.Now()
		var v int
		err := pool.QueryRow(ctx, `SELECT "Value" FROM "TestRows" WHERE "Id" = $1`, id).Scan(&v)
		if ctx.Err() != nil {
			return
		}
		assert.NoError(t, err)
		into.add(time.Since(start))
		time.Sleep(time.Millisecond)
	}
}

func measure(t *testing.T, pool *pgxpool.Pool, d time.Duration) time.Duration {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), d)
	defer cancel()
	var l latencies
	serve(ctx, t, pool, &l)
	return l.p99()
}

func TestABackfillOf100000RowsSurvivesThreeKills(t *testing.T) {
	if testing.Short() {
		t.Skip("starts and kills child processes")
	}
	pool := dbtest.NewPool(t)
	seedRows(t, pool, killRows)
	schema := pool.Config().ConnConfig.RuntimeParams["search_path"]
	baseline := measure(t, pool, time.Second)

	var during latencies
	ctx, stop := context.WithCancel(t.Context())
	var serving sync.WaitGroup
	serving.Go(func() { serve(ctx, t, pool, &during) })

	for _, at := range []int64{20_000, 45_000, 70_000} {
		cmd := startWorker(t, schema)
		waitForProcessed(t, pool, at)
		require.NoError(t, cmd.Process.Signal(syscall.SIGKILL))
		_ = cmd.Wait()
	}
	killedAt := progress(t, pool).Processed
	assert.Less(t, killedAt, int64(killRows), "the kills landed before the end")
	cmd := startWorker(t, schema)
	require.NoError(t, cmd.Wait(), "the last run finishes the backfill")
	stop()
	serving.Wait()

	p := progress(t, pool)
	assert.True(t, p.Done())
	assert.EqualValues(t, killRows, p.Processed, "the progress counts every row once")
	var converted, twice int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FILTER (WHERE "Converted"), count(*) FILTER (WHERE "Times" <> 1)
		FROM "TestRows"`).Scan(&converted, &twice))
	assert.Equal(t, killRows, converted)
	assert.Zero(t, twice, "no row was converted twice")
	limit := max(2*baseline, 20*time.Millisecond)
	t.Logf("read p99: %s before, %s while the backfill ran and was killed (limit %s)", baseline, during.p99(), limit)
	assert.Less(t, during.p99(), limit, "the server keeps serving while the backfill runs")
}
