package app_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"filippo.io/age/armor"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

type keysCLI struct {
	t    *testing.T
	pool *pgxpool.Pool
	file string
	out  bytes.Buffer
}

func (c *keysCLI) run(args ...string) error {
	c.t.Helper()
	opts, err := app.ParseKeysArgs(append(args, "--file", c.file))
	if err != nil {
		return err
	}
	return app.KeysRunner{
		Opts: opts, Out: &c.out, Stdout: &c.out, Getenv: func(string) string { return "" },
		Connect: func(ctx context.Context) (*pgxpool.Pool, error) {
			return pgxpool.NewWithConfig(ctx, c.pool.Config())
		},
	}.Run(c.t.Context())
}

func (c *keysCLI) ring() *keys.Keyring {
	c.t.Helper()
	ring, err := keys.ReadKeyringFile(c.file, keys.FileRules{})
	require.NoError(c.t, err)
	return ring
}

func auditActions(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT "Action" FROM "AuditEvents" WHERE "Action" LIKE 'system.keys.%' ORDER BY "Id"`)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a string
		require.NoError(t, rows.Scan(&a))
		out = append(out, a)
	}
	return out
}

func TestParseKeysArgs(t *testing.T) {
	good := [][]string{
		{"init"},
		{"init", "--file", "/k"},
		{"init", "--discard-unreadable"},
		{"init", "--discard-unreadable", "--confirm", "destroy-12-keys"},
		{"status"},
		{"generate"},
		{"activate", "2"},
		{"activate", "2", "--file", "/k"},
		{"export", "--recipient", "age1x", "--out", "-"},
		{"rekey"},
		{"rekey", "--grace", "0s", "--file", "/k"},
	}
	for _, args := range good {
		_, err := app.ParseKeysArgs(args)
		require.NoError(t, err, "%v", args)
	}
	bad := [][]string{
		{},
		{"rotate"},
		{"activate"},
		{"activate", "0"},
		{"activate", "two"},
		{"activate", "-1"},
		{"status", "extra"},
		{"init", "--confirm", "destroy-12-keys"},
		{"export"},
		{"export", "--recipient", "age1x"},
		{"export", "--out", "x"},
		{"generate", "--recipient", "age1x"},
		{"status", "--discard-unreadable"},
		{"rekey", "--grace", "-1s"},
		{"rekey", "now"},
		{"activate", "2", "--grace", "1s"},
	}
	for _, args := range bad {
		_, err := app.ParseKeysArgs(args)
		require.ErrorIs(t, err, app.ErrKeysUsage, "%v", args)
	}
}

func TestKeysCommandsRotateExportAndNeverPrintAKey(t *testing.T) {
	pool := dbtest.NewPool(t)
	cli := &keysCLI{t: t, pool: pool, file: filepath.Join(apptest.PrivateDir(t), "dhow-keyring")}

	require.NoError(t, cli.run("init"))
	info, err := os.Stat(cli.file)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	first := cli.ring()
	require.Error(t, cli.run("init"), "an existing keyring is never replaced")
	assert.Equal(t, first.Marshal(), cli.ring().Marshal())

	provider, err := keys.NewFileProvider(first)
	require.NoError(t, err)
	server := keys.NewManager(pool, provider, nil)
	_, err = server.Register(t.Context())
	require.NoError(t, err)
	sealed, err := server.Envelope().Seal(t.Context(), keys.InstanceOwner(), "dhow/instance-settings/v1", []byte("mail"), []byte("smtp password"))
	require.NoError(t, err)

	require.NoError(t, cli.run("generate"))
	assert.Contains(t, cli.out.String(), "keys activate 2")
	both := cli.ring()
	assert.Equal(t, []int32{1, 2}, both.Versions())
	err = cli.run("activate", "2")
	assert.Equal(t, keys.CodeKekNotFound, keys.CodeOf(err), "the servers have not registered version 2 yet")

	bothProvider, err := keys.NewFileProvider(both)
	require.NoError(t, err)
	restarted := keys.NewManager(pool, bothProvider, nil)
	_, err = restarted.Register(t.Context())
	require.NoError(t, err)
	require.NoError(t, cli.run("activate", "2"))
	_, err = restarted.Rewrap(t.Context())
	require.NoError(t, err)
	got, err := restarted.Envelope().Open(t.Context(), keys.InstanceOwner(), "dhow/instance-settings/v1", []byte("mail"), sealed)
	require.NoError(t, err)
	assert.Equal(t, "smtp password", string(got))

	require.NoError(t, cli.run("status"))
	assert.Contains(t, cli.out.String(), "Active KEK version:")
	assert.Contains(t, cli.out.String(), "retired")
	assert.Contains(t, cli.out.String(), "never exported")

	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	exported := filepath.Join(apptest.PrivateDir(t), "dhow-keyring.age")
	require.Error(t, cli.run("export", "--recipient", identity.String(), "--out", exported), "a private key is refused")
	_, err = os.Stat(exported)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, cli.run("export", "--recipient", identity.Recipient().String(), "--out", exported))
	require.Error(t, cli.run("export", "--recipient", identity.Recipient().String(), "--out", exported), "never overwrites")

	data, err := os.ReadFile(exported)
	require.NoError(t, err)
	assert.False(t, bytes.Contains(data, []byte("kek ")))
	decrypted, err := age.Decrypt(armor.NewReader(bytes.NewReader(data)), identity)
	require.NoError(t, err)
	plain, err := io.ReadAll(decrypted)
	require.NoError(t, err)
	assert.Equal(t, both.Marshal(), plain)

	st, err := restarted.Status(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, st.LastExportedAt)
	assert.Equal(t, []string{"system.keys.activated", "system.keys.exported"}, auditActions(t, pool))

	cli.out.Reset()
	require.NoError(t, cli.run("status"))
	assert.NotContains(t, cli.out.String(), "never exported")
	assertNoKeys(t, cli.out.String()+string(data), first, both)
}

func discardSetup(t *testing.T) (*pgxpool.Pool, *keys.Manager, keys.Owner, []byte) {
	t.Helper()
	pool := dbtest.NewPool(t)
	original, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	provider, err := keys.NewFileProvider(original)
	require.NoError(t, err)
	server := keys.NewManager(pool, provider, nil)
	_, err = server.Register(t.Context())
	require.NoError(t, err)
	user := keys.UserOwner(uuid.New())
	sealed, err := server.Envelope().Seal(t.Context(), user, config.TotpSealLabel, []byte(user.ID.String()), []byte("JBSWY3DPEHPK3PXP"))
	require.NoError(t, err)
	return pool, server, user, sealed
}

func activeDataKeys(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM "DataKeys" WHERE "State" = 'active'`).Scan(&n))
	return n
}

func newKeyringFile(t *testing.T) string {
	t.Helper()
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	return keyringFile(t, ring)
}

func TestDiscardNeverMakesAKeyringForAnExplicitFile(t *testing.T) {
	pool, _, _, _ := discardSetup(t)
	cli := &keysCLI{t: t, pool: pool, file: filepath.Join(apptest.PrivateDir(t), "dhow-keyring")}
	err := cli.run("init", "--discard-unreadable")
	require.Error(t, err)
	assert.Equal(t, keys.CodeKeyringMissing, keys.CodeOf(err))
	_, statErr := os.Stat(cli.file)
	require.ErrorIs(t, statErr, os.ErrNotExist)
	assert.Equal(t, 1, activeDataKeys(t, pool))
}

func TestDiscardIsRefusedWhileARunningServerHasTheVersions(t *testing.T) {
	pool, _, _, _ := discardSetup(t)
	_, err := pool.Exec(t.Context(), `INSERT INTO "ServerInstances" ("InstanceKey", "Hostname", "Version", "SchemaVersion", "KekVersions")
		VALUES ('still-running', 'still-running', 'test', 52, '{1}')`)
	require.NoError(t, err)
	cli := &keysCLI{t: t, pool: pool, file: newKeyringFile(t)}
	err = cli.run("init", "--discard-unreadable", "--confirm", "destroy-1-keys")
	require.Error(t, err)
	assert.Equal(t, keys.CodeKeysInUse, keys.CodeOf(err))
	assert.Contains(t, err.Error(), "still-running")
	assert.Equal(t, 1, activeDataKeys(t, pool))
	assert.Empty(t, auditActions(t, pool))
}

func TestDiscardConfirmationNamesTheCountAndTheAuditCommitsWithIt(t *testing.T) {
	pool, _, user, sealed := discardSetup(t)
	cli := &keysCLI{t: t, pool: pool, file: newKeyringFile(t)}
	for _, confirm := range []string{"", "discard-unreadable-secrets", "destroy-2-keys", "destroy-0-keys", "destroy-01-keys"} {
		args := []string{"init", "--discard-unreadable"}
		if confirm != "" {
			args = append(args, "--confirm", confirm)
		}
		err := cli.run(args...)
		require.ErrorIs(t, err, app.ErrDiscardUnconfirm, confirm)
		assert.Equal(t, 1, activeDataKeys(t, pool), confirm)
	}
	assert.Contains(t, cli.out.String(), "--confirm destroy-1-keys")

	_, err := pool.Exec(t.Context(), `CREATE FUNCTION refuse_discard() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'audit refused'; END $$;
		CREATE TRIGGER refuse_discard BEFORE INSERT ON "AuditEvents" FOR EACH ROW
		WHEN (NEW."Action" = 'system.keys.discarded') EXECUTE FUNCTION refuse_discard()`)
	require.NoError(t, err)
	require.Error(t, cli.run("init", "--discard-unreadable", "--confirm", "destroy-1-keys"))
	assert.Equal(t, 1, activeDataKeys(t, pool), "without its audit entry the discard does not happen")
	_, err = pool.Exec(t.Context(), `DROP TRIGGER refuse_discard ON "AuditEvents"`)
	require.NoError(t, err)

	require.NoError(t, cli.run("init", "--discard-unreadable", "--confirm", "destroy-1-keys"))
	assert.Zero(t, activeDataKeys(t, pool))
	assert.Equal(t, []string{"system.keys.discarded"}, auditActions(t, pool))
	var severity, count string
	require.NoError(t, pool.QueryRow(t.Context(),
		`SELECT "Data"->>'Severity', "Data"->>'DataKeys' FROM "AuditEvents" WHERE "Action" = 'system.keys.discarded'`).Scan(&severity, &count))
	assert.Equal(t, "high", severity)
	assert.Equal(t, "1", count)

	replacement, err := keys.NewFileProvider(cli.ring())
	require.NoError(t, err)
	booted := keys.NewManager(pool, replacement, nil)
	_, err = booted.Register(t.Context())
	require.NoError(t, err, "the server starts with the new keyring")
	_, err = booted.Envelope().Open(t.Context(), user, config.TotpSealLabel, []byte(user.ID.String()), sealed)
	require.ErrorIs(t, err, keys.ErrShredded)
	fresh, err := booted.Envelope().Seal(t.Context(), user, config.TotpSealLabel, []byte(user.ID.String()), []byte("a new secret"))
	require.NoError(t, err)
	got, err := booted.Envelope().Open(t.Context(), user, config.TotpSealLabel, []byte(user.ID.String()), fresh)
	require.NoError(t, err)
	assert.Equal(t, "a new secret", string(got))

	cli.out.Reset()
	require.NoError(t, cli.run("init", "--discard-unreadable"))
	assert.Contains(t, cli.out.String(), "Nothing to discard")
	assertNoKeys(t, cli.out.String(), cli.ring())
}
