package integration_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

func TestTheJobsMigrationRollsBackCompletely(t *testing.T) {
	pool := dbtest.NewEmptyPool(t)
	ctx := context.Background()
	m, err := migrations.NewMigrator(pool)
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()
	_, err = m.Up(ctx)
	require.NoError(t, err)

	objects := func() int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM pg_tables WHERE schemaname = current_schema()
			   AND tablename IN ('Jobs', 'JobAttempts', 'Leases', 'AttemptSteps')) +
			(SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
			   WHERE n.nspname = current_schema() AND p.proname IN ('GuardJobState', 'GuardAttemptState', 'GuardLeaseState', 'GuardStepState')) +
			(SELECT count(*) FROM pg_sequences WHERE schemaname = current_schema() AND sequencename = 'FencingTokens')`).Scan(&n))
		return n
	}
	assert.Equal(t, 9, objects())
	require.NoError(t, m.DownTo(ctx, 30))
	assert.Zero(t, objects())
	var runners int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname = current_schema() AND tablename = 'Runners'`).Scan(&runners))
	assert.Equal(t, 1, runners, "the runners migration is untouched")
	_, err = m.Up(ctx)
	require.NoError(t, err)
	assert.Equal(t, 9, objects())
}
