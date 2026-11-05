package migrations_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

const (
	beforeSealedContract = 64
	checkViolation       = "23514"
)

func oldFormatValue() []byte { return append([]byte{1}, make([]byte, 60)...) }

func envelopeValue() []byte { return append([]byte{3}, make([]byte, 60)...) }

func TestTheSealedFormatContractWaitsForTheBackfillAndThenRefusesTheOldFormat(t *testing.T) {
	ctx := t.Context()
	pool := dbtest.NewPool(t)
	m, err := migrations.NewMigrator(pool, migrations.Binary{})
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()
	require.NoError(t, m.DownTo(ctx, beforeSealedContract))

	var user string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO "Users" ("Email", "Name", "PasswordHash") VALUES ('o@example.com', 'O', 'x') RETURNING "Id"::text`).Scan(&user))
	_, err = pool.Exec(ctx, `INSERT INTO "TotpFactors" ("UserId", "SecretSealed", "ExpiresAt") VALUES ($1, $2, now() + interval '1 day')`, user, oldFormatValue())
	require.NoError(t, err)

	_, err = m.Up(ctx)
	require.ErrorContains(t, err, "BACKFILL_NOT_DONE")
	require.ErrorContains(t, err, "keys.envelope.TotpFactors.SecretSealed")

	_, err = pool.Exec(ctx, `INSERT INTO "Backfills" ("Name", "DoneAt") VALUES ('keys.envelope.TotpFactors.SecretSealed', now())`)
	require.NoError(t, err)
	_, err = m.Up(ctx)
	require.Error(t, err, "a finished backfill that left an old value behind stops the validation")
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	assert.Equal(t, checkViolation, pgErr.Code)

	_, err = pool.Exec(ctx, `UPDATE "TotpFactors" SET "SecretSealed" = $1`, envelopeValue())
	require.NoError(t, err)
	_, err = m.Up(ctx)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `UPDATE "TotpFactors" SET "SecretSealed" = $1`, oldFormatValue())
	require.ErrorAs(t, err, &pgErr)
	assert.Equal(t, checkViolation, pgErr.Code, "the old format cannot be written any more")
}

func TestTheSealedFormatContractNeedsNoBackfillWhenNoOldValueIsLeft(t *testing.T) {
	ctx := t.Context()
	pool := dbtest.NewPool(t)
	m, err := migrations.NewMigrator(pool, migrations.Binary{})
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()
	require.NoError(t, m.DownTo(ctx, beforeSealedContract))
	_, err = m.Up(ctx)
	require.NoError(t, err, "a database with no value in the old format passes without any backfill record")
}
