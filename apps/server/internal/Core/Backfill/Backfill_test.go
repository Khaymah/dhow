package backfill_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	backfill "github.com/Khaymah/dhow/apps/server/internal/Core/Backfill"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

const jobName = "test.rows"

var errInjected = errors.New("injected failure")

type rowsJob struct {
	name      string
	failAfter atomic.Int64
	hold      time.Duration
}

func key(id int64) string { return fmt.Sprintf("%012d", id) }

func (j *rowsJob) Name() string {
	if j.name == "" {
		return jobName
	}
	return j.name
}

func (j *rowsJob) Step(ctx context.Context, tx sqlc.DBTX, after string, limit int32) (backfill.Batch, error) {
	if j.failAfter.Load() > 0 && j.failAfter.Add(-1) == 0 {
		return backfill.Batch{}, errInjected
	}
	start := int64(0)
	if after != "" {
		n, err := strconv.ParseInt(after, 10, 64)
		if err != nil {
			return backfill.Batch{}, err
		}
		start = n
	}
	rows, err := tx.Query(ctx, `UPDATE "TestRows" SET "Converted" = true, "Times" = "Times" + 1
		WHERE "Id" IN (SELECT "Id" FROM "TestRows" WHERE "Id" > $1 AND NOT "Converted" ORDER BY "Id" LIMIT $2 FOR UPDATE SKIP LOCKED)
		RETURNING "Id"`, start, limit)
	if err != nil {
		return backfill.Batch{}, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return backfill.Batch{}, err
	}
	if j.hold > 0 {
		time.Sleep(j.hold)
	}
	out := backfill.Batch{Rows: len(ids), Changed: int64(len(ids))}
	for _, id := range ids {
		if k := key(id); k > out.Last {
			out.Last = k
		}
	}
	return out, nil
}

func (j *rowsJob) Remaining(ctx context.Context, tx sqlc.DBTX) (int64, error) {
	var n int64
	err := tx.QueryRow(ctx, `SELECT count(*) FROM "TestRows" WHERE NOT "Converted"`).Scan(&n)
	return n, err
}

func seedRows(t testing.TB, pool *pgxpool.Pool, n int) {
	t.Helper()
	_, err := pool.Exec(t.Context(), `CREATE TABLE "TestRows" ("Id" bigint PRIMARY KEY, "Value" integer NOT NULL,
		"Converted" boolean NOT NULL DEFAULT false, "Times" integer NOT NULL DEFAULT 0)`)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO "TestRows" ("Id", "Value") SELECT g, g FROM generate_series(1, $1) AS g`, n)
	require.NoError(t, err)
}

func progress(t testing.TB, pool *pgxpool.Pool) backfill.Progress {
	t.Helper()
	list, err := backfill.List(t.Context(), pool)
	require.NoError(t, err)
	for _, p := range list {
		if p.Name == jobName {
			return p
		}
	}
	return backfill.Progress{}
}

func noWait(ctx context.Context, _ time.Duration) bool { return ctx.Err() == nil }

func newRunner(t *testing.T, pool *pgxpool.Pool, batch int32, jobs ...backfill.Job) *backfill.Runner {
	t.Helper()
	r, err := backfill.New(backfill.Options{DB: pool, Jobs: jobs, BatchSize: batch, Wait: noWait})
	require.NoError(t, err)
	return r
}

func TestABackfillConvertsEveryRowOnceAndResumesAfterAFailure(t *testing.T) {
	pool := dbtest.NewPool(t)
	seedRows(t, pool, 250)
	job := &rowsJob{}
	job.failAfter.Store(3)
	r := newRunner(t, pool, 40, job)

	require.ErrorIs(t, r.Drain(t.Context()), errInjected)
	p := progress(t, pool)
	assert.EqualValues(t, 80, p.Processed, "the failed batch rolled back with its progress")
	assert.Equal(t, key(80), p.LastKey)
	assert.False(t, p.Done())

	require.NoError(t, r.Drain(t.Context()))
	p = progress(t, pool)
	assert.True(t, p.Done())
	assert.EqualValues(t, 250, p.Processed)
	require.NotNil(t, p.Remaining)
	assert.Zero(t, *p.Remaining)
	var twice int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM "TestRows" WHERE "Times" <> 1`).Scan(&twice))
	assert.Zero(t, twice)
	require.NoError(t, backfill.RequireDone(t.Context(), pool, jobName))

	res, err := r.Step(t.Context(), job)
	require.NoError(t, err)
	assert.True(t, res.Done, "a finished backfill does nothing more")
}

func TestABackfillSkipsLockedRowsAndOnlyFinishesWhenNoneIsLeft(t *testing.T) {
	pool := dbtest.NewPool(t)
	seedRows(t, pool, 30)
	tx, err := pool.Begin(t.Context())
	require.NoError(t, err)
	_, err = tx.Exec(t.Context(), `SELECT 1 FROM "TestRows" WHERE "Id" = 7 FOR UPDATE`)
	require.NoError(t, err)
	r := newRunner(t, pool, 10, &rowsJob{})

	require.NoError(t, r.Drain(t.Context()))
	p := progress(t, pool)
	assert.False(t, p.Done(), "a row held by a request is not skipped for good")
	require.NotNil(t, p.Remaining)
	assert.EqualValues(t, 1, *p.Remaining)
	assert.Empty(t, p.LastKey, "the next pass starts from the beginning")
	err = backfill.RequireDone(t.Context(), pool, jobName)
	var notDone *backfill.NotDoneError
	require.ErrorAs(t, err, &notDone)
	assert.Contains(t, err.Error(), backfill.CodeNotDone)

	require.NoError(t, tx.Rollback(t.Context()))
	require.NoError(t, r.Drain(t.Context()))
	assert.True(t, progress(t, pool).Done())
	assert.EqualValues(t, 30, progress(t, pool).Processed)
}

func TestTheContractGuardStopsAMigrationUntilTheBackfillIsDone(t *testing.T) {
	pool := dbtest.NewPool(t)
	_, err := pool.Exec(t.Context(), `SELECT "RequireBackfillDone"('keys.envelope.Example.Column')`)
	require.ErrorContains(t, err, "BACKFILL_NOT_DONE")
	require.ErrorContains(t, backfill.RequireDone(t.Context(), pool, "keys.envelope.Example.Column"), "BACKFILL_NOT_DONE")
	_, err = pool.Exec(t.Context(), `INSERT INTO "Backfills" ("Name", "DoneAt") VALUES ('keys.envelope.Example.Column', now())`)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `SELECT "RequireBackfillDone"('keys.envelope.Example.Column')`)
	require.NoError(t, err)
	require.NoError(t, backfill.RequireDone(t.Context(), pool, "keys.envelope.Example.Column"))
	require.NoError(t, backfill.RequireDone(t.Context(), pool))
}

func TestOnlyOneServerRunsABackfillBatchAtATime(t *testing.T) {
	pool := dbtest.NewPool(t)
	seedRows(t, pool, 10)
	r := newRunner(t, pool, 5, &rowsJob{})
	tx, err := pool.Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(t.Context()) }()
	locked, err := sqlc.New(tx).TryLockBackfill(t.Context(), jobName)
	require.NoError(t, err)
	require.True(t, locked)
	res, err := r.Step(t.Context(), &rowsJob{})
	require.NoError(t, err)
	assert.True(t, res.Locked)
	assert.Zero(t, progress(t, pool).Processed)
}

func TestBackfillNamesAreCheckedWhenTheRunnerIsMade(t *testing.T) {
	_, err := backfill.New(backfill.Options{Jobs: []backfill.Job{&rowsJob{name: "has space"}}})
	require.ErrorIs(t, err, backfill.ErrBadName)
	_, err = backfill.New(backfill.Options{Jobs: []backfill.Job{&rowsJob{}, &rowsJob{}}})
	require.ErrorIs(t, err, backfill.ErrDuplicate)
}

type stuckJob struct{}

func (*stuckJob) Name() string { return jobName }

func (*stuckJob) Remaining(context.Context, sqlc.DBTX) (int64, error) { return 0, nil }

func (*stuckJob) Step(context.Context, sqlc.DBTX, string, int32) (backfill.Batch, error) {
	return backfill.Batch{Rows: 1, Last: ""}, nil
}

func TestABatchThatDoesNotMoveItsCursorIsAnError(t *testing.T) {
	pool := dbtest.NewPool(t)
	r := newRunner(t, pool, 5, &stuckJob{})
	_, err := r.Step(t.Context(), &stuckJob{})
	require.ErrorIs(t, err, backfill.ErrKeyNotAhead)
}

type stuckRowJob struct{}

func (*stuckRowJob) Name() string { return jobName }

func (*stuckRowJob) Step(_ context.Context, _ sqlc.DBTX, after string, _ int32) (backfill.Batch, error) {
	if after != "" {
		return backfill.Batch{}, nil
	}
	return backfill.Batch{Rows: 1, Last: key(1), Failed: 1}, nil
}

func (*stuckRowJob) Remaining(context.Context, sqlc.DBTX) (int64, error) { return 1, nil }

func TestTheFailedCountIsForTheLastPassOnly(t *testing.T) {
	pool := dbtest.NewPool(t)
	var done []string
	r, err := backfill.New(backfill.Options{
		DB: pool, Jobs: []backfill.Job{&stuckRowJob{}}, Wait: noWait,
		OnDone: func(name string) { done = append(done, name) },
	})
	require.NoError(t, err)
	for range 3 {
		require.NoError(t, r.Drain(t.Context()))
	}
	p := progress(t, pool)
	assert.EqualValues(t, 1, p.Failed, "one value fails on each pass, so the count stays one")
	assert.EqualValues(t, 3, p.Passes)
	assert.False(t, p.Done())
	assert.Empty(t, done)
}

func TestTheRunnerReportsEveryFinishedBackfill(t *testing.T) {
	pool := dbtest.NewPool(t)
	seedRows(t, pool, 5)
	var done []string
	r, err := backfill.New(backfill.Options{
		DB: pool, Jobs: []backfill.Job{&rowsJob{}}, Wait: noWait,
		OnDone: func(name string) { done = append(done, name) },
	})
	require.NoError(t, err)
	require.NoError(t, r.Drain(t.Context()))
	require.NoError(t, r.Drain(t.Context()))
	assert.Equal(t, []string{jobName, jobName}, done, "a later pass that finds the backfill done reports it too")
}
