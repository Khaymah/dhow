package migrations_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

const beforeRekey = 53

func TestTheRekeyMigrationRollsBackOnlyWithoutReplacedDataKeys(t *testing.T) {
	pool := dbtest.NewPool(t)
	_, err := pool.Exec(t.Context(), `INSERT INTO "KeyEncryptionKeys" ("Version", "CheckValue", "State", "ActivatedAt") VALUES (1, $1, 'active', now())`, make([]byte, 32))
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO "DataKeys" ("Id", "OwnerKind", "OwnerId", "KekVersion", "Wrapped", "State")
		VALUES ($1, 'project', $2, 1, $3, 'superseded')`, uuid.New(), uuid.New(), make([]byte, 60))
	require.NoError(t, err)
	m, err := migrations.NewMigrator(pool, migrations.Binary{})
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()

	err = m.DownTo(t.Context(), beforeRekey)
	require.ErrorContains(t, err, "finish dhow-server keys rekey")

	_, err = pool.Exec(t.Context(), `UPDATE "DataKeys" SET "State" = 'active'`)
	require.NoError(t, err)
	require.NoError(t, m.DownTo(t.Context(), beforeRekey), "with no replaced data key the rollback works")
}
