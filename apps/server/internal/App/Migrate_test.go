package app_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
	"github.com/Khaymah/dhow/internal/buildinfo"
)

func TestMigrateCommands(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewPool(t)
	ident := app.ServerIdentity(buildinfo.Info{Version: "1.2.3"}, 50, "web-1", os.Hostname, quietLog())
	stop, err := app.StartHeartbeat(ctx, pool, ident, quietLog())
	require.NoError(t, err)
	t.Cleanup(stop)
	m := newMigrator(t, pool, migrationsWith(t, map[string]string{
		"99001_next.sql":  "-- dhow:phase expand\n-- +goose Up\nCREATE TABLE \"NextThing\" (\"Id\" int);\n",
		"99002_index.sql": "-- +goose NO TRANSACTION\n-- dhow:phase expand\n-- +goose Up\nCREATE INDEX CONCURRENTLY IF NOT EXISTS \"IX_Next\" ON \"NextThing\" (\"Id\");\n",
	}), migrations.Binary{Version: "1.2.3"})

	tests := []struct {
		cmd  string
		want []string
	}{
		{app.MigrateStatus, []string{"State:", "pending", "Schema floor:", "0.0.0", "Pending:", "99001_next.sql, 99002_index.sql", "Running servers:", "1.2.3"}},
		{app.MigratePlan, []string{"00001_instance.sql", "expand (before the rule)", "00050_schema_floor_server_instances.sql", "applied", "99002_index.sql", "pending", "no"}},
		{app.MigrateDryRun, []string{"99001_next.sql", "applied", "99002_index.sql", "not tried (NO TRANSACTION)", "rolled back"}},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			var out bytes.Buffer
			require.NoError(t, app.Migrate(ctx, m, pool, app.MigrateOptions{Command: tt.cmd}, &out))
			for _, want := range tt.want {
				assert.Contains(t, out.String(), want)
			}
		})
	}
	state, err := m.State(ctx)
	require.NoError(t, err)
	assert.Equal(t, migrations.SchemaPending, state, "the dry run applied nothing")
	assert.ErrorIs(t, app.Migrate(ctx, m, pool, app.MigrateOptions{Command: "up"}, io.Discard), app.ErrMigrateUsage)
	for _, args := range [][]string{{"status", "extra"}, {"status", "--timeout", "5s"}, {"dry-run", "--timeout", "soon"}, {"dry-run", "--timeout", "-1s"}} {
		assert.ErrorIs(t, app.RunMigrate(ctx, args, io.Discard), app.ErrMigrateUsage, args)
	}

	for _, cmd := range []string{app.MigrateStatus, app.MigratePlan, app.MigrateDryRun} {
		err := app.Migrate(ctx, m, pool, app.MigrateOptions{Command: cmd}, failingWriter{})
		assert.ErrorIs(t, err, errNoSpace, "%s reports a write error", cmd)
	}
}

func TestMigrateDryRunWithNothingPending(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewPool(t)
	var out bytes.Buffer
	require.NoError(t, app.Migrate(ctx, newMigrator(t, pool, migrations.FS, migrations.Binary{}), pool, app.MigrateOptions{Command: app.MigrateDryRun}, &out))
	assert.Contains(t, out.String(), "no migration is pending")
}

var errNoSpace = errors.New("no space left on device")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errNoSpace }

func TestParseMigrateArgs(t *testing.T) {
	opts, err := app.ParseMigrateArgs([]string{"dry-run", "--timeout", "90s"})
	require.NoError(t, err)
	assert.Equal(t, app.MigrateOptions{Command: app.MigrateDryRun, Timeout: 90 * time.Second}, opts)
	opts, err = app.ParseMigrateArgs([]string{"dry-run"})
	require.NoError(t, err)
	assert.Equal(t, migrations.DefaultDryRunTimeout, opts.Timeout)
}
