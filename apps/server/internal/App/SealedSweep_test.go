package app_test

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	"github.com/Khaymah/dhow/internal/leaksweep"
)

func dumpTables(t *testing.T, env *apptest.Env, dir string) int {
	t.Helper()
	ctx := t.Context()
	rows, err := env.Pool.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname = current_schema() ORDER BY 1`)
	require.NoError(t, err)
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	conn, err := env.Pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()
	for _, table := range tables {
		f, err := os.Create(filepath.Join(dir, table+".sql"))
		require.NoError(t, err)
		_, err = conn.Conn().PgConn().CopyTo(ctx, f, "COPY (SELECT * FROM "+pgx.Identifier{table}.Sanitize()+") TO STDOUT")
		require.NoError(t, err, table)
		require.NoError(t, f.Close())
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "server.log"), []byte(env.Logs.String()), 0o600))
	return len(tables)
}

func needle(t *testing.T, name string, value []byte) leaksweep.Needle {
	t.Helper()
	n, err := leaksweep.Encoded(name, value)
	require.NoError(t, err)
	return n
}

func TestTheDatabaseDumpHoldsNoSealedValueNoDataKeyAndNoKeyringKey(t *testing.T) {
	w := newSealedWorld(t)
	makeLegacy(t, w.env)
	drainBackfills(t, w.env)
	dir := apptest.PrivateDir(t)
	require.Positive(t, dumpTables(t, w.env, dir))

	needles := []leaksweep.Needle{
		needle(t, "TOTP secret", w.totp.Secret),
		needle(t, "TOTP secret as text", []byte(auth.TotpSecretText(w.totp.Secret))),
		needle(t, "SMTP password", []byte(smtpPassword)),
		needle(t, "sign-in provider secret", []byte(oidcSecret)),
		needle(t, "webhook secret", []byte(w.hookKey)),
		needle(t, "seeded webhook secret", []byte("seeded-webhook-secret")),
	}
	deploy, err := leaksweep.PEMKey("deploy key", w.ssh.PrivateKey)
	require.NoError(t, err)
	needles = append(needles, deploy...)
	provider, err := keys.NewFakeFrom(apptest.TestKeyring(t))
	require.NoError(t, err)
	deks, err := apptest.DataKeysInClear(t.Context(), w.env.Pool, provider)
	require.NoError(t, err)
	require.Len(t, deks, 4)
	for _, dek := range deks {
		needles = append(needles, needle(t, "data key", dek))
	}
	keks, err := apptest.KeyringKeys(apptest.TestKeyring(t))
	require.NoError(t, err)
	for _, kek := range keks {
		needles = append(needles, needle(t, "keyring key", kek))
	}
	needles = append(needles, needle(t, "server secret key", w.env.Config.SecretKey))

	hits, stats, err := leaksweep.Sweep([]string{dir}, needles, nil, leaksweep.DefaultLimits())
	require.NoError(t, err)
	assert.Positive(t, stats.Files)
	assert.Empty(t, hits, "a plain value, a data key or a keyring key is in the dump")

	planted := apptest.PrivateDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(planted, "dump.sql"), []byte("\\\\x"+hex.EncodeToString(deks[0])), 0o600))
	hits, _, err = leaksweep.Sweep([]string{planted}, needles, nil, leaksweep.DefaultLimits())
	require.NoError(t, err)
	require.Len(t, hits, 1, "the sweep finds a data key planted on purpose")
	assert.Equal(t, "data key", hits[0].Name)
}
