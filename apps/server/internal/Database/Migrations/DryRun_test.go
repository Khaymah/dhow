package migrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

func countTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname = current_schema()").Scan(&n))
	return n
}

func dbVersion(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int64 {
	t.Helper()
	var v int64
	require.NoError(t, pool.QueryRow(ctx, "SELECT max(version_id) FROM goose_db_version").Scan(&v))
	return v
}

func TestDryRunAppliesNothing(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewPool(t)
	m := migrator(t, pool, withFiles(t, map[string]string{
		"99001_dry_table.sql":  "-- dhow:phase expand\n-- +goose Up\nCREATE TABLE \"DryOne\" (\"Id\" int);\nCREATE TABLE \"DryTwo\" (\"Id\" int);\n",
		"99002_dry_index.sql":  "-- +goose NO TRANSACTION\n-- dhow:phase expand\n-- +goose Up\nCREATE INDEX CONCURRENTLY IF NOT EXISTS \"IX_DryOne\" ON \"DryOne\" (\"Id\");\n",
		"99003_dry_column.sql": "-- dhow:phase expand\n-- +goose Up\nALTER TABLE \"DryTwo\" ADD COLUMN \"Name\" TEXT;\n",
	}), migrations.Binary{})
	tables, version := countTables(t, ctx, pool), dbVersion(t, ctx, pool)

	res, err := m.DryRun(ctx, migrations.DryRunOptions{})
	require.NoError(t, err)
	outcomes := map[string]migrations.DryRunOutcome{}
	for _, s := range res.Steps {
		outcomes[s.Name] = s.Outcome
	}
	assert.Equal(t, map[string]migrations.DryRunOutcome{
		"99001_dry_table.sql":  migrations.DryRunApplied,
		"99002_dry_index.sql":  migrations.DryRunNotTried,
		"99003_dry_column.sql": migrations.DryRunApplied,
	}, outcomes)
	assert.Equal(t, tables, countTables(t, ctx, pool), "the dry run rolls back")
	assert.Equal(t, version, dbVersion(t, ctx, pool))
}

func TestDryRunReportsAFailingMigrationAndChangesNothing(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewPool(t)
	m := migrator(t, pool, withFiles(t, map[string]string{
		"99001_dry_ok.sql":    "-- dhow:phase expand\n-- +goose Up\nCREATE TABLE \"DryOk\" (\"Id\" int);\n",
		"99002_dry_bad.sql":   "-- dhow:phase expand\n-- +goose Up\nCREATE TABLE \"DryBad\" (\"Id\" int);\nALTER TABLE \"NoSuchTable\" ADD COLUMN \"X\" TEXT;\n",
		"99003_dry_after.sql": "-- dhow:phase expand\n-- +goose Up\nCREATE TABLE \"DryAfter\" (\"Id\" int);\n",
	}), migrations.Binary{})
	tables, version := countTables(t, ctx, pool), dbVersion(t, ctx, pool)

	res, err := m.DryRun(ctx, migrations.DryRunOptions{})
	var preflight *migrations.PreflightError
	require.ErrorAs(t, err, &preflight)
	assert.Equal(t, migrations.CodeMigrationFailed, preflight.Code)
	assert.Equal(t, "99002_dry_bad.sql", preflight.Name)
	assert.Equal(t, 2, preflight.Statement)
	assert.Contains(t, err.Error(), "NoSuchTable")
	require.Len(t, res.Steps, 3)
	assert.Equal(t, []migrations.DryRunOutcome{migrations.DryRunApplied, migrations.DryRunFailed, migrations.DryRunNotReached},
		[]migrations.DryRunOutcome{res.Steps[0].Outcome, res.Steps[1].Outcome, res.Steps[2].Outcome})

	assert.Equal(t, tables, countTables(t, ctx, pool), "a failed dry run leaves no table behind")
	assert.Equal(t, version, dbVersion(t, ctx, pool))
}

func TestDryRunGivesUpOnALockAfterThreeSeconds(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewPool(t)
	m := migrator(t, pool, withFiles(t, map[string]string{
		"99001_dry_lock.sql": "-- dhow:phase expand\n-- +goose Up\nALTER TABLE \"Instance\" ADD COLUMN \"DryLock\" TEXT;\n",
	}), migrations.Binary{})

	holder, err := pool.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, holder.Rollback(context.Background())) })
	_, err = holder.Exec(ctx, `LOCK TABLE "Instance" IN ACCESS SHARE MODE`)
	require.NoError(t, err)

	started := time.Now()
	_, err = m.DryRun(ctx, migrations.DryRunOptions{})
	elapsed := time.Since(started)
	var preflight *migrations.PreflightError
	require.ErrorAs(t, err, &preflight)
	assert.Equal(t, migrations.CodeLockTimeout, preflight.Code)
	assert.Contains(t, err.Error(), "PREFLIGHT_LOCK_TIMEOUT")
	assert.GreaterOrEqual(t, elapsed, 2900*time.Millisecond, "the lock timeout is 3 seconds")
	assert.Less(t, elapsed, 6*time.Second, "the dry run does not wait past the lock timeout")
}

func TestDryRunRefusesTransactionControl(t *testing.T) {
	for name, body := range map[string]string{
		"COMMIT in the middle": "CREATE TABLE \"DryC1\" (\"Id\" int);\nCOMMIT;\nCREATE TABLE \"DryC2\" (\"Id\" int);\n",
		"END":                  "CREATE TABLE \"DryC1\" (\"Id\" int);\nEND;\nCREATE TABLE \"DryC2\" (\"Id\" int);\n",
		"ROLLBACK in a block": "-- +goose StatementBegin\nCREATE TABLE \"DryC1\" (\"Id\" int); ROLLBACK; CREATE TABLE \"DryC2\" (\"Id\" int);\n" +
			"-- +goose StatementEnd\n",
		"a savepoint": "SAVEPOINT \"S\";\nCREATE TABLE \"DryC1\" (\"Id\" int);\nRELEASE SAVEPOINT \"S\";\n",
	} {
		t.Run(name, func(t *testing.T) {
			pool := dbtest.NewPool(t)
			ctx := context.Background()
			m := migrator(t, pool, withFiles(t, map[string]string{
				"99001_dry_ok.sql":      "-- dhow:phase expand\n-- +goose Up\nCREATE TABLE \"DryFirst\" (\"Id\" int);\n",
				"99002_dry_control.sql": "-- dhow:phase expand\n-- +goose Up\n" + body,
			}), migrations.Binary{})
			tables, version := countTables(t, ctx, pool), dbVersion(t, ctx, pool)

			_, err := m.DryRun(ctx, migrations.DryRunOptions{})
			var preflight *migrations.PreflightError
			require.ErrorAs(t, err, &preflight)
			assert.Equal(t, migrations.CodeTransactionControl, preflight.Code)
			assert.Equal(t, "99002_dry_control.sql", preflight.Name)
			assert.Equal(t, tables, countTables(t, ctx, pool), "nothing was committed")
			assert.Equal(t, version, dbVersion(t, ctx, pool))
		})
	}
}

func TestDryRunStopsAtItsDeadline(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewPool(t)
	m := migrator(t, pool, withFiles(t, map[string]string{
		"99001_dry_slow.sql": "-- dhow:phase expand\n-- +goose Up\nSET statement_timeout = 0;\nSET lock_timeout = 0;\nCREATE TABLE \"DrySlow\" (\"Id\" int);\nSELECT pg_sleep(30);\n",
	}), migrations.Binary{})
	tables := countTables(t, ctx, pool)

	started := time.Now()
	_, err := m.DryRun(ctx, migrations.DryRunOptions{Timeout: 2 * time.Second})
	elapsed := time.Since(started)
	var preflight *migrations.PreflightError
	require.ErrorAs(t, err, &preflight)
	assert.Equal(t, migrations.CodeDeadline, preflight.Code)
	assert.Less(t, elapsed, 8*time.Second, "a file's own SET statement_timeout = 0 does not outlive the deadline")
	assert.Equal(t, tables, countTables(t, ctx, pool))
	assert.Equal(t, time.Minute, migrations.DefaultDryRunTimeout)
}
