package keys_test

import (
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

type kekRow struct {
	Version int32
	State   string
}

func kekRows(t *testing.T, pool *pgxpool.Pool) []kekRow {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT "Version", "State" FROM "KeyEncryptionKeys" ORDER BY "Version"`)
	require.NoError(t, err)
	defer rows.Close()
	var out []kekRow
	for rows.Next() {
		var r kekRow
		require.NoError(t, rows.Scan(&r.Version, &r.State))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

func managerFor(t testing.TB, pool *pgxpool.Pool, ring *keys.Keyring) *keys.Manager {
	t.Helper()
	provider, err := keys.NewFileProvider(ring)
	require.NoError(t, err)
	return keys.NewManager(pool, provider, slog.New(slog.DiscardHandler))
}

func TestFirstStartRegistersAndActivatesTheHighestVersion(t *testing.T) {
	pool := dbtest.NewPool(t)
	ring, err := keys.GenerateKeyring(1, 2)
	require.NoError(t, err)
	reg, err := managerFor(t, pool, ring).Register(t.Context())
	require.NoError(t, err)
	assert.True(t, reg.Activated)
	assert.Equal(t, int32(2), reg.Active)
	assert.Equal(t, []int32{1, 2}, reg.Registered)
	assert.Equal(t, []int32{1, 2}, reg.Versions)
	assert.Equal(t, []kekRow{{1, keys.KekPending}, {2, keys.KekActive}}, kekRows(t, pool))

	again, err := managerFor(t, pool, ring).Register(t.Context())
	require.NoError(t, err)
	assert.False(t, again.Activated)
	assert.Empty(t, again.Registered)
	assert.Equal(t, int32(2), again.Active)

	var check []byte
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT "CheckValue" FROM "KeyEncryptionKeys" WHERE "Version" = 2`).Scan(&check))
	assert.Len(t, check, keys.CheckBytes)
	assert.NotContains(t, string(ring.Marshal()), string(check))
}

func TestBootStopsWhenTheKeyringDoesNotFitTheDatabase(t *testing.T) {
	pool := dbtest.NewPool(t)
	ring, err := keys.GenerateKeyring(1, 2)
	require.NoError(t, err)
	_, err = managerFor(t, pool, ring).Register(t.Context())
	require.NoError(t, err)

	onlyOne, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	_, err = managerFor(t, pool, onlyOne).Register(t.Context())
	require.Error(t, err)
	assert.Equal(t, keys.CodeKeyringMismatch, keys.CodeOf(err), "a different key under version 1")

	missing := keyringWithout(t, ring, 2)
	_, err = managerFor(t, pool, missing).Register(t.Context())
	require.Error(t, err)
	assert.Equal(t, keys.CodeKeyringMissingVersion, keys.CodeOf(err))
	assert.Contains(t, err.Error(), "KEK version 2")
	var ke *keys.Error
	require.ErrorAs(t, err, &ke)
	assert.Equal(t, int32(2), ke.Version)
	assert.Equal(t, keys.ExitKeyring, ke.ExitCode())

	swapped := keyringWithout(t, ring, 1)
	_, err = managerFor(t, pool, swapped).Register(t.Context())
	require.Error(t, err)
	assert.Equal(t, keys.CodeKeyringMissingVersion, keys.CodeOf(err), "the pending version 1 is needed too")

	assert.Equal(t, []kekRow{{1, keys.KekPending}, {2, keys.KekActive}}, kekRows(t, pool), "a refused boot changes nothing")
	require.NoError(t, managerFor(t, pool, ring).Verify(t.Context()))
	assert.Equal(t, keys.CodeKeyringMismatch, keys.CodeOf(managerFor(t, pool, onlyOne).Verify(t.Context())))
}

func TestANewVersionInTheFileIsRegisteredPending(t *testing.T) {
	pool := dbtest.NewPool(t)
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	_, err = managerFor(t, pool, ring).Register(t.Context())
	require.NoError(t, err)
	next, v, err := ring.WithNextVersion()
	require.NoError(t, err)
	reg, err := managerFor(t, pool, next).Register(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []int32{v}, reg.Registered)
	assert.Equal(t, int32(1), reg.Active)
	assert.Equal(t, []kekRow{{1, keys.KekActive}, {2, keys.KekPending}}, kekRows(t, pool))
	require.NoError(t, managerFor(t, pool, ring).Verify(t.Context()), "a server that has not loaded the pending version stays ready")
}

func TestPostgresEnvelopeBindsOwnersAndShreds(t *testing.T) {
	pool := dbtest.NewPool(t)
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	m := managerFor(t, pool, ring)
	_, err = m.Register(t.Context())
	require.NoError(t, err)
	env := m.Envelope()
	orgA, orgB := keys.OrganizationOwner(uuid.New()), keys.OrganizationOwner(uuid.New())
	sealedA, err := env.Seal(t.Context(), orgA, "dhow/git-connection-token/v1", []byte("connection:1"), []byte("token of A"))
	require.NoError(t, err)
	_, err = env.Seal(t.Context(), orgB, "dhow/git-connection-token/v1", []byte("connection:1"), []byte("token of B"))
	require.NoError(t, err)
	instance, err := env.Seal(t.Context(), keys.InstanceOwner(), "dhow/instance-settings/v1", []byte("mail"), []byte("smtp password"))
	require.NoError(t, err)

	_, err = env.Open(t.Context(), orgB, "dhow/git-connection-token/v1", []byte("connection:1"), sealedA)
	require.ErrorIs(t, err, keys.ErrSealed, "copied to another organization with the same context")
	_, err = env.Open(t.Context(), orgA, "dhow/git-connection-token/v1", []byte("connection:2"), sealedA)
	require.ErrorIs(t, err, keys.ErrSealed, "copied to another row")
	got, err := env.Open(t.Context(), keys.InstanceOwner(), "dhow/instance-settings/v1", []byte("mail"), instance)
	require.NoError(t, err)
	assert.Equal(t, "smtp password", string(got))

	var count int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM "DataKeys" WHERE "State" = 'active'`).Scan(&count))
	assert.Equal(t, 3, count)

	n, err := env.Destroy(t.Context(), orgA)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	_, err = env.Open(t.Context(), orgA, "dhow/git-connection-token/v1", []byte("connection:1"), sealedA)
	require.ErrorIs(t, err, keys.ErrShredded)
	var wrappedNull bool
	require.NoError(t, pool.QueryRow(t.Context(),
		`SELECT "Wrapped" IS NULL FROM "DataKeys" WHERE "OwnerKind" = 'organization' AND "OwnerId" = $1`, orgA.ID).Scan(&wrappedNull))
	assert.True(t, wrappedNull)
	_, err = env.Destroy(t.Context(), keys.InstanceOwner())
	require.NoError(t, err)
	_, err = env.Open(t.Context(), keys.InstanceOwner(), "dhow/instance-settings/v1", []byte("mail"), instance)
	require.ErrorIs(t, err, keys.ErrShredded)
}

func keyringWithout(t *testing.T, ring *keys.Keyring, drop int32) *keys.Keyring {
	t.Helper()
	var kept []byte
	for _, l := range splitLines(ring.Marshal()) {
		if l.version != drop {
			kept = append(kept, l.text...)
		}
	}
	out, err := keys.ParseKeyring(kept)
	require.NoError(t, err)
	return out
}

type keyLine struct {
	version int32
	text    []byte
}

func splitLines(data []byte) []keyLine {
	var out []keyLine
	start := 0
	for i, b := range data {
		if b != '\n' {
			continue
		}
		text := data[start : i+1]
		var v int32
		for _, c := range text[4:] {
			if c == ' ' {
				break
			}
			v = v*10 + int32(c-'0')
		}
		out = append(out, keyLine{version: v, text: text})
		start = i + 1
	}
	return out
}
