package integration_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

func TestTheOverviewIndexesRollBackCompletely(t *testing.T) {
	pool := dbtest.NewEmptyPool(t)
	ctx := context.Background()
	m, err := migrations.NewMigrator(pool, migrations.Binary{})
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()
	_, err = m.Up(ctx)
	require.NoError(t, err)

	indexes := func() []string {
		rows, err := pool.Query(ctx, `SELECT indexname FROM pg_indexes WHERE schemaname = current_schema()
			AND indexname IN ('IX_Jobs_Active', 'IX_Jobs_Failed_Finished') ORDER BY indexname`)
		require.NoError(t, err)
		defer rows.Close()
		var out []string
		for rows.Next() {
			var name string
			require.NoError(t, rows.Scan(&name))
			out = append(out, name)
		}
		require.NoError(t, rows.Err())
		return out
	}
	assert.Equal(t, []string{"IX_Jobs_Active", "IX_Jobs_Failed_Finished"}, indexes())
	require.NoError(t, m.DownTo(ctx, 35))
	assert.Empty(t, indexes())
	_, err = m.Up(ctx)
	require.NoError(t, err)
	assert.Len(t, indexes(), 2)
}
