package app_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

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

func TestServerIdentity(t *testing.T) {
	build := buildinfo.Info{Version: "1.2.3", Commit: "abc"}
	host := func() (string, error) { return "web-1", nil }
	ident := app.ServerIdentity(build, 50, "", host, quietLog())
	assert.Equal(t, "web-1", ident.Key, "the host name is the default instance key")
	assert.Equal(t, "web-1", ident.Hostname)
	assert.Equal(t, "custom", app.ServerIdentity(build, 50, "custom", host, quietLog()).Key)

	var logs bytes.Buffer
	broken := func() (string, error) { return "", errors.New("no uts name") }
	ident = app.ServerIdentity(build, 50, "", broken, slog.New(slog.NewTextHandler(&logs, nil)))
	assert.Equal(t, "unknown", ident.Hostname)
	assert.Equal(t, "unknown", ident.Key)
	assert.Contains(t, logs.String(), "no uts name", "a host name error is logged, not swallowed")
	assert.Contains(t, logs.String(), config.EnvInstanceID)
}

func TestHoldIncompatible(t *testing.T) {
	tooNew := &migrations.IncompatibleError{Status: migrations.Evaluate(60, 50, "2.0.0", migrations.Binary{Version: "1.0.0"})}

	assert.Same(t, tooNew, app.HoldIncompatible(context.Background(), tooNew, config.OnIncompatibleExit, quietLog()),
		"the binary install exits with the error, so systemd sees 78")
	other := errors.New("database down")
	assert.Same(t, other, app.HoldIncompatible(context.Background(), other, config.OnIncompatibleWait, quietLog()))

	var logs bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := app.HoldIncompatible(ctx, tooNew, config.OnIncompatibleWait, slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err, "in the container the server waits for SIGTERM instead of exiting into a restart loop")
	assert.GreaterOrEqual(t, time.Since(started), 300*time.Millisecond)
	assert.Contains(t, logs.String(), "SCHEMA_TOO_NEW")
	assert.Equal(t, 1, strings.Count(logs.String(), "\n"), "logged once")
}

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
	ident := app.ServerIdentity(buildinfo.Info{Version: "1.2.3", Commit: "abc"}, 50, "", os.Hostname, quietLog())
	stop, err := app.StartHeartbeat(ctx, pool, ident, quietLog())
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
