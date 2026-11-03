package app_test

import (
	"bytes"
	"context"
	"io"
	"testing"

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
	stop, err := app.StartHeartbeat(ctx, pool, buildinfo.Info{Version: "1.2.3"}, 50, quietLog())
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
			require.NoError(t, app.Migrate(ctx, m, pool, tt.cmd, &out))
			for _, want := range tt.want {
				assert.Contains(t, out.String(), want)
			}
		})
	}
	state, err := m.State(ctx)
	require.NoError(t, err)
	assert.Equal(t, migrations.SchemaPending, state, "the dry run applied nothing")
	assert.ErrorIs(t, app.Migrate(ctx, m, pool, "up", io.Discard), app.ErrMigrateUsage)
	assert.ErrorIs(t, app.RunMigrate(ctx, []string{"status", "extra"}, io.Discard), app.ErrMigrateUsage)
}

func TestMigrateDryRunWithNothingPending(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewPool(t)
	var out bytes.Buffer
	require.NoError(t, app.Migrate(ctx, newMigrator(t, pool, migrations.FS, migrations.Binary{}), pool, app.MigrateDryRun, &out))
	assert.Contains(t, out.String(), "no migration is pending")
}
