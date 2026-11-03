package migrations_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

const (
	futureExpand   = "-- dhow:phase expand\n-- +goose Up\nCREATE TABLE \"FutureThings\" (\"Id\" int);\n-- +goose Down\nDROP TABLE \"FutureThings\";\n"
	futureContract = "-- dhow:phase contract\n-- dhow:floor 2.0.0\n-- +goose Up\nUPDATE \"Instance\" SET \"SchemaFloor\" = '2.0.0';\n-- +goose Down\nUPDATE \"Instance\" SET \"SchemaFloor\" = '0.0.0';\n"
)

func withFiles(t *testing.T, extra map[string]string) fs.FS {
	t.Helper()
	out := fstest.MapFS{}
	names, err := fs.Glob(migrations.FS, "*.sql")
	require.NoError(t, err)
	for _, name := range names {
		body, err := fs.ReadFile(migrations.FS, name)
		require.NoError(t, err)
		out[name] = &fstest.MapFile{Data: body}
	}
	for name, body := range extra {
		out[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return out
}

func migrator(t *testing.T, pool *pgxpool.Pool, fsys fs.FS, bin migrations.Binary) *migrations.Migrator {
	t.Helper()
	m, err := migrations.NewMigratorFS(pool, fsys, bin)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, m.Close()) })
	return m
}

func upNewer(t *testing.T, extra map[string]string) *pgxpool.Pool {
	t.Helper()
	pool := dbtest.NewEmptyPool(t)
	_, err := migrator(t, pool, withFiles(t, extra), migrations.Binary{}).Up(t.Context())
	require.NoError(t, err)
	return pool
}

func TestOlderBinaryRunsOnAnExpandOnlyNewerSchema(t *testing.T) {
	ctx := context.Background()
	pool := upNewer(t, map[string]string{"99001_future_expand.sql": futureExpand})
	older := migrator(t, pool, migrations.FS, migrations.Binary{Version: "1.0.0"})

	st, err := older.CheckCompatible(ctx)
	require.NoError(t, err)
	assert.Equal(t, migrations.SchemaAhead, st.State)
	assert.Equal(t, int64(99001), st.DBVersion)
	assert.Equal(t, "0.0.0", st.Floor)

	v, err := older.Up(ctx)
	require.NoError(t, err, "Up on a newer schema is a no-op")
	assert.Equal(t, int64(99001), v)
	state, err := older.State(ctx)
	require.NoError(t, err)
	assert.True(t, state.Ready(), "an older binary is ready on an expand-only newer schema")
}

func TestOlderBinaryRefusesASchemaWhoseFloorIsAboveIt(t *testing.T) {
	ctx := context.Background()
	pool := upNewer(t, map[string]string{"99001_future_expand.sql": futureExpand, "99002_future_contract.sql": futureContract})

	tests := []struct {
		name string
		bin  migrations.Binary
		want migrations.SchemaState
	}{
		{"a release below the floor", migrations.Binary{Version: "1.9.3"}, migrations.SchemaIncompatible},
		{"a release at the floor", migrations.Binary{Version: "2.0.0"}, migrations.SchemaAhead},
		{"a dev build", migrations.Binary{Version: "dev"}, migrations.SchemaIncompatible},
		{"a dev build with DHOW_DEV", migrations.Binary{Version: "dev", Dev: true}, migrations.SchemaAhead},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, err := migrator(t, pool, migrations.FS, tt.bin).CheckCompatible(ctx)
			assert.Equal(t, tt.want, st.State)
			if tt.want != migrations.SchemaIncompatible {
				require.NoError(t, err)
				return
			}
			var incompatible *migrations.IncompatibleError
			require.ErrorAs(t, err, &incompatible)
			assert.Equal(t, 78, incompatible.ExitCode())
			assert.Contains(t, err.Error(), "2.0.0", "the message names the version the database needs")
			assert.ErrorIs(t, err, migrations.ErrSchemaTooNew)
		})
	}
}

func indexValid(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) (bool, bool) {
	t.Helper()
	var valid bool
	err := pool.QueryRow(ctx, `SELECT i.indisvalid FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
		WHERE c.relname = $1 AND c.relnamespace = current_schema()::regnamespace`, name).Scan(&valid)
	if err != nil {
		return false, false
	}
	return true, valid
}

func TestUpRebuildsAnIndexLeftInvalidByAnInterruptedRun(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewPool(t)
	_, err := pool.Exec(ctx, `CREATE TABLE "DupProbe" ("V" int)`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO "DupProbe" VALUES (1), (1)`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `CREATE UNIQUE INDEX CONCURRENTLY "IX_DupProbe" ON "DupProbe" ("V")`)
	require.Error(t, err, "the duplicate interrupts the build")
	exists, valid := indexValid(t, ctx, pool, "IX_DupProbe")
	require.True(t, exists)
	require.False(t, valid, "an interrupted concurrent build leaves an invalid index")
	_, err = pool.Exec(ctx, `TRUNCATE "DupProbe"`)
	require.NoError(t, err)

	m := migrator(t, pool, withFiles(t, map[string]string{
		"99001_dup_index.sql": "-- +goose NO TRANSACTION\n-- dhow:phase expand\n-- +goose Up\nCREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS \"IX_DupProbe\" ON \"DupProbe\" (\"V\");\n",
	}), migrations.Binary{})
	_, err = m.Up(ctx)
	require.NoError(t, err)
	exists, valid = indexValid(t, ctx, pool, "IX_DupProbe")
	assert.True(t, exists)
	assert.True(t, valid, "Up drops the invalid index first, so IF NOT EXISTS builds it again")
}

func TestUpLeavesValidIndexesAlone(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewPool(t)
	_, err := pool.Exec(ctx, `CREATE TABLE "KeepProbe" ("V" int)`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `CREATE INDEX "IX_KeepProbe" ON "KeepProbe" ("V")`)
	require.NoError(t, err)
	var before uint32
	require.NoError(t, pool.QueryRow(ctx, `SELECT '"IX_KeepProbe"'::regclass::oid`).Scan(&before))

	m := migrator(t, pool, withFiles(t, map[string]string{
		"99001_keep_index.sql": "-- +goose NO TRANSACTION\n-- dhow:phase expand\n-- +goose Up\nCREATE INDEX CONCURRENTLY IF NOT EXISTS \"IX_KeepProbe\" ON \"KeepProbe\" (\"V\");\n",
	}), migrations.Binary{})
	_, err = m.Up(ctx)
	require.NoError(t, err)
	var after uint32
	require.NoError(t, pool.QueryRow(ctx, `SELECT '"IX_KeepProbe"'::regclass::oid`).Scan(&after))
	assert.Equal(t, before, after, "a valid index is not dropped")
}

func TestStatusOfAnEmptyDatabase(t *testing.T) {
	ctx := context.Background()
	m := migrator(t, dbtest.NewEmptyPool(t), migrations.FS, migrations.Binary{Version: "1.0.0"})
	st, err := m.CheckCompatible(ctx)
	require.NoError(t, err)
	assert.Equal(t, migrations.SchemaPending, st.State)
	assert.Equal(t, migrations.DefaultFloor, st.Floor)
	entries, err := m.Plan(ctx)
	require.NoError(t, err)
	assert.Len(t, migrations.Pending(entries), len(entries))
	assert.False(t, errors.Is(err, migrations.ErrSchemaTooNew))
}
