package integration_test

import (
	"context"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	succeeded = runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED
	noReason  = runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED
)

func mustUUID(v string) uuid.UUID { return uuid.MustParse(v) }

func partitions(t *testing.T, env *apptest.Env) []string {
	t.Helper()
	rows, err := env.Pool.Query(t.Context(), `SELECT c.relname FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
		WHERE i.inhparent = '"LogChunks"'::regclass ORDER BY c.relname`)
	require.NoError(t, err)
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	return names
}

func dayName(t *testing.T, env *apptest.Env, offset int) string {
	t.Helper()
	var name string
	require.NoError(t, env.Pool.QueryRow(t.Context(), `SELECT "LogPartitionName"("LogPartitionToday"() + $1::int)`, offset).Scan(&name))
	return name
}

func makeOldPartition(t *testing.T, env *apptest.Env, offset int) {
	t.Helper()
	var from, to string
	require.NoError(t, env.Pool.QueryRow(t.Context(),
		`SELECT "LogPartitionBound"("LogPartitionToday"() + $1::int), "LogPartitionBound"("LogPartitionToday"() + $1::int + 1)`, offset).Scan(&from, &to))
	_, err := env.Pool.Exec(t.Context(), `CREATE TABLE `+pgx.Identifier{dayName(t, env, offset)}.Sanitize()+
		` PARTITION OF "LogChunks" FOR VALUES FROM ('`+from+`') TO ('`+to+`')`)
	require.NoError(t, err)
}

func TestTheMigrationPreparesTodayAndThreeDaysAhead(t *testing.T) {
	env := apptest.New(t)
	want := []string{dayName(t, env, 0), dayName(t, env, 1), dayName(t, env, 2), dayName(t, env, 3)}
	assert.Equal(t, want, partitions(t, env))
	res, err := env.Wired.Jobs.MaintainLogPartitions(t.Context())
	require.NoError(t, err)
	assert.Zero(t, res.Created, "maintenance is idempotent")
	assert.Zero(t, res.Dropped)
}

func TestRetentionDropsOnlyPartitionsPastIt(t *testing.T) {
	env := apptest.NewWithOptions(t, apptest.Options{Profile: func(c *config.Config) {
		c.Logs = config.Logs{MaxStreams: 10}
		c.SetEnvSetting(config.EnvLogRetentionDays, "7")
	}})
	makeOldPartition(t, env, -40)
	makeOldPartition(t, env, -8)
	makeOldPartition(t, env, -7)
	_, err := env.Pool.Exec(t.Context(), `CREATE TABLE "LogChunksArchive" (LIKE "LogChunks")`)
	require.NoError(t, err)

	res, err := env.Wired.Jobs.MaintainLogPartitions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, res.Dropped)
	names := partitions(t, env)
	assert.NotContains(t, names, dayName(t, env, -40))
	assert.NotContains(t, names, dayName(t, env, -8))
	assert.Contains(t, names, dayName(t, env, -7), "a partition exactly at the retention stays")
	assert.Contains(t, names, dayName(t, env, 3))
	var archive bool
	require.NoError(t, env.Pool.QueryRow(t.Context(), `SELECT to_regclass('"LogChunksArchive"') IS NOT NULL`).Scan(&archive))
	assert.True(t, archive, "only attached partitions with the dated name are dropped")
}

func TestAMissingPartitionFailsReadinessAndAppendUntilMaintenance(t *testing.T) {
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	env.CreateJobOK(owner, org, apptest.SimpleSpec(), nil)
	r := env.SeedConnectedRunner(org, mustUUID(owner.UserID), apptest.SeedRunnerSpec{})
	g := r.MustStart(0)
	require.Equal(t, http.StatusOK, env.Do(apptest.Request{Method: http.MethodGet, Path: "/health/ready"}).Status)

	_, err := env.Pool.Exec(t.Context(), `DROP TABLE `+pgx.Identifier{dayName(t, env, 0)}.Sanitize())
	require.NoError(t, err)
	res := env.Do(apptest.Request{Method: http.MethodGet, Path: "/health/ready"})
	assert.Equal(t, http.StatusServiceUnavailable, res.Status)
	assert.Contains(t, string(res.Raw), `{"Name":"log_partitions","Ok":false}`)
	_, err = r.Append(g.Ref, apptest.Chunk(1, "x"))
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, connect.CodeUnavailable, ce.Code())
	assert.Equal(t, "LOG_STORAGE_UNAVAILABLE", ce.Message())

	maintained, err := env.Wired.Jobs.MaintainLogPartitions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, maintained.Created)
	res = env.Do(apptest.Request{Method: http.MethodGet, Path: "/health/ready"})
	assert.Equal(t, http.StatusOK, res.Status)
	assert.Contains(t, string(res.Raw), `{"Name":"log_partitions","Ok":true}`)
	ack := r.MustAppend(g.Ref, apptest.Chunk(1, "x"))
	assert.EqualValues(t, 1, ack.GetLastSeq(), "the runner retries the same seq once the partition is back")
}

func TestMaintenanceStepsAsideWhileAnotherInstanceHoldsTheLock(t *testing.T) {
	env := apptest.New(t)
	tx, err := env.Pool.Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(t.Context(), `SELECT pg_advisory_xact_lock(hashtextextended('dhow.log-partitions', 0))`)
	require.NoError(t, err)
	res, err := env.Wired.Jobs.MaintainLogPartitions(t.Context())
	require.NoError(t, err)
	assert.True(t, res.Skipped)
}

func TestAFinishedAttemptsLogOnlyGrows(t *testing.T) {
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	env.CreateJobOK(owner, org, apptest.SimpleSpec(), nil)
	r := env.SeedConnectedRunner(org, mustUUID(owner.UserID), apptest.SeedRunnerSpec{})
	g := r.MustStart(0)
	r.MustAppend(g.Ref, apptest.Chunk(1, "x"))
	require.NoError(t, r.Complete(g, succeeded, 0, noReason))
	id := g.Grant.GetAttemptId()

	exec := func(sql string) error {
		_, err := env.Pool.Exec(t.Context(), sql, id)
		return err
	}
	require.NoError(t, exec(`UPDATE "JobAttempts" SET "LastSeq" = "LastSeq" + 1, "LogBytes" = "LogBytes" + 5 WHERE "Id" = $1`))
	require.NoError(t, exec(`UPDATE "JobAttempts" SET "LateResultLoggedAt" = now() WHERE "Id" = $1`))
	for name, sql := range map[string]string{
		"the seq goes back":        `UPDATE "JobAttempts" SET "LastSeq" = 0 WHERE "Id" = $1`,
		"the byte count goes back": `UPDATE "JobAttempts" SET "LogBytes" = 0 WHERE "Id" = $1`,
		"a second late result":     `UPDATE "JobAttempts" SET "LateResultLoggedAt" = now() + interval '1 second' WHERE "Id" = $1`,
		"truncated after the end":  `UPDATE "JobAttempts" SET "LogTruncatedAt" = now() WHERE "Id" = $1`,
		"the result changes":       `UPDATE "JobAttempts" SET "ExitCode" = 3 WHERE "Id" = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			assert.ErrorContains(t, exec(sql), "SQLSTATE 23514")
		})
	}
}

func TestTheLogMigrationRollsBackCompletely(t *testing.T) {
	pool := dbtest.NewEmptyPool(t)
	ctx := context.Background()
	m, err := migrations.NewMigrator(pool, migrations.Binary{})
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()
	_, err = m.Up(ctx)
	require.NoError(t, err)
	objects := func() int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
			   WHERE n.nspname = current_schema() AND c.relname LIKE 'LogChunks%' AND c.relkind IN ('r', 'p')) +
			(SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
			   WHERE n.nspname = current_schema() AND p.proname LIKE '%LogPartition%') +
			(SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema()
			   AND table_name = 'JobAttempts' AND column_name IN ('LogTruncatedAt', 'LateResultLoggedAt'))`).Scan(&n))
		return n
	}
	assert.Equal(t, 5+8+2, objects(), "the parent, four partitions, eight functions and two columns")
	require.NoError(t, m.DownTo(ctx, 31))
	assert.Zero(t, objects())
	var jobs int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname = current_schema() AND tablename = 'JobAttempts'`).Scan(&jobs))
	assert.Equal(t, 1, jobs, "the jobs migration is untouched")
	_, err = m.Up(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5+8+2, objects())
}

func locksHeld(t *testing.T, tx pgx.Tx, table string) []string {
	t.Helper()
	rows, err := tx.Query(t.Context(), `SELECT l.mode FROM pg_locks l WHERE l.pid = pg_backend_pid() AND l.relation = to_regclass($1) ORDER BY 1`,
		pgx.Identifier{table}.Sanitize())
	require.NoError(t, err)
	modes, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	return modes
}

func TestPartitionDDLNeverLocksTheAttempts(t *testing.T) {
	env := apptest.New(t)
	_, err := env.Pool.Exec(t.Context(), `DROP TABLE `+pgx.Identifier{dayName(t, env, 3)}.Sanitize())
	require.NoError(t, err)
	makeOldPartition(t, env, -40)

	tx, err := env.Pool.Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	var created int
	require.NoError(t, tx.QueryRow(t.Context(), `SELECT "EnsureLogPartitions"(3)`).Scan(&created))
	assert.Equal(t, 1, created)
	assert.Empty(t, locksHeld(t, tx, "JobAttempts"), "creating a partition takes no lock on the attempts")
	assert.NotContains(t, locksHeld(t, tx, "LogChunks"), "AccessExclusiveLock", "attaching a new partition keeps reads and appends running")
	assert.Contains(t, locksHeld(t, tx, "LogChunks"), "ShareUpdateExclusiveLock")
	require.NoError(t, tx.Commit(t.Context()))

	tx, err = env.Pool.Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	var dropped int
	require.NoError(t, tx.QueryRow(t.Context(), `SELECT "DropLogPartitions"(30)`).Scan(&dropped))
	assert.Equal(t, 1, dropped)
	assert.Empty(t, locksHeld(t, tx, "JobAttempts"), "dropping a partition takes no lock on the attempts")
	assert.Contains(t, locksHeld(t, tx, "LogChunks"), "AccessExclusiveLock", "a drop briefly blocks the log table, bounded by lock_timeout (ADR-0052)")
}

func TestPartitionsWithUnreadableNamesAreSkippedNotFatal(t *testing.T) {
	env := apptest.New(t)
	makeOldPartition(t, env, -40)
	for name, from := range map[string]string{"LogChunks_20261399": "2001-01-01", "LogChunksManual": "2001-01-02"} {
		_, err := env.Pool.Exec(t.Context(), `CREATE TABLE `+pgx.Identifier{name}.Sanitize()+
			` PARTITION OF "LogChunks" FOR VALUES FROM ('`+from+` 00:00:00+00') TO ('`+from+` 12:00:00+00')`)
		require.NoError(t, err)
	}
	res, err := env.Wired.Jobs.MaintainLogPartitions(t.Context())
	require.NoError(t, err, "a partition with a bad name never wedges maintenance")
	assert.Equal(t, 1, res.Dropped, "the dated old partition is still dropped")
	assert.Equal(t, []string{"LogChunksManual", "LogChunks_20261399"}, res.Unreadable)
	names := partitions(t, env)
	assert.Contains(t, names, "LogChunks_20261399")
	assert.Contains(t, names, "LogChunksManual")
	assert.Contains(t, env.Logs.String(), "job log partitions without a dated name are never dropped")
}

func TestATableWithThePartitionNameThatIsNotAttachedFailsMaintenance(t *testing.T) {
	env := apptest.New(t)
	future := dayName(t, env, 3)
	_, err := env.Pool.Exec(t.Context(), `DROP TABLE `+pgx.Identifier{future}.Sanitize())
	require.NoError(t, err)
	_, err = env.Pool.Exec(t.Context(), `CREATE TABLE `+pgx.Identifier{future}.Sanitize()+` (LIKE "LogChunks" INCLUDING ALL)`)
	require.NoError(t, err)
	_, err = env.Wired.Jobs.MaintainLogPartitions(t.Context())
	require.ErrorContains(t, err, "is not a partition of")
	assert.NotContains(t, partitions(t, env), future)
}

func TestThePartitionFunctionsPinTheirSearchPath(t *testing.T) {
	env := apptest.New(t)
	var schema string
	require.NoError(t, env.Pool.QueryRow(t.Context(), `SELECT current_schema()`).Scan(&schema))
	rows, err := env.Pool.Query(t.Context(), `SELECT p.proname, COALESCE(array_to_string(p.proconfig, ';'), '')
		FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = current_schema() AND (p.proname LIKE '%LogPartition%') ORDER BY 1`)
	require.NoError(t, err)
	type fn struct {
		Name, Config string
	}
	fns, err := pgx.CollectRows(rows, pgx.RowToStructByPos[fn])
	require.NoError(t, err)
	require.Len(t, fns, 8)
	for _, f := range fns {
		assert.Equal(t, "search_path=pg_catalog, "+schema+", pg_temp", f.Config, f.Name)
	}
}
