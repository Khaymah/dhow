package app_test

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
	"github.com/Khaymah/dhow/internal/buildinfo"
)

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func migrationsWith(t *testing.T, extra map[string]string) fs.FS {
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

func newMigrator(t *testing.T, pool *pgxpool.Pool, fsys fs.FS, bin migrations.Binary) *migrations.Migrator {
	t.Helper()
	m, err := migrations.NewMigratorFS(pool, fsys, bin)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, m.Close()) })
	return m
}

const raiseFloor = "-- dhow:phase contract\n-- dhow:floor 3.1.0\n-- +goose Up\nUPDATE \"Instance\" SET \"SchemaFloor\" = '3.1.0';\n"

func TestPrepareSchemaRefusesASchemaTooNewForTheBinary(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewEmptyPool(t)
	_, err := newMigrator(t, pool, migrationsWith(t, map[string]string{"99001_raise_floor.sql": raiseFloor}), migrations.Binary{}).Up(ctx)
	require.NoError(t, err)

	err = app.PrepareSchema(ctx, newMigrator(t, pool, migrations.FS, migrations.Binary{Version: "3.0.4"}), quietLog())
	var incompatible *migrations.IncompatibleError
	require.ErrorAs(t, err, &incompatible)
	assert.Equal(t, 78, incompatible.ExitCode())
	assert.Contains(t, err.Error(), "3.1.0")
	assert.Contains(t, err.Error(), "SCHEMA_TOO_NEW")

	require.NoError(t, app.PrepareSchema(ctx, newMigrator(t, pool, migrations.FS, migrations.Binary{Version: "3.1.0"}), quietLog()),
		"a binary at the floor starts")
}

func TestPrepareSchemaMigratesAnEmptyDatabase(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewEmptyPool(t)
	m := newMigrator(t, pool, migrations.FS, migrations.Binary{Version: "1.0.0"})
	require.NoError(t, app.PrepareSchema(ctx, m, quietLog()))
	state, err := m.State(ctx)
	require.NoError(t, err)
	assert.Equal(t, migrations.SchemaCurrent, state)
}

func TestHeartbeatRegistersAndRetiresThisServer(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewPool(t)
	stop, err := app.StartHeartbeat(ctx, pool, buildinfo.Info{Version: "1.2.3", Commit: "abc"}, 50, quietLog())
	require.NoError(t, err)
	live, err := sqlc.New(pool).ListLiveServerInstances(ctx, config.ServerInstanceStaleAfter.Seconds())
	require.NoError(t, err)
	require.Len(t, live, 1)
	assert.Equal(t, "1.2.3", live[0].Version)
	assert.Equal(t, int64(50), live[0].SchemaVersion)

	stop()
	live, err = sqlc.New(pool).ListLiveServerInstances(ctx, config.ServerInstanceStaleAfter.Seconds())
	require.NoError(t, err)
	assert.Empty(t, live, "a stopped server retires its row")
}
