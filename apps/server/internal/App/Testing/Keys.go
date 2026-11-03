package apptest

import (
	"crypto/sha256"
	"encoding/base64"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
)

const testKeyringSeed = "dhow apptest keyring, version 1"

func TestKeyring(t testing.TB) *keys.Keyring {
	t.Helper()
	sum := sha256.Sum256([]byte(testKeyringSeed))
	ring, err := keys.ParseKeyring([]byte("kek 1 " + base64.RawURLEncoding.EncodeToString(sum[:]) + "\n"))
	require.NoError(t, err)
	return ring
}

func NewKeys(t testing.TB, pool *pgxpool.Pool, log *slog.Logger) *keys.Manager {
	t.Helper()
	provider, err := keys.NewFakeFrom(TestKeyring(t))
	require.NoError(t, err)
	manager := keys.NewManager(pool, provider, log)
	_, err = manager.Register(t.Context())
	require.NoError(t, err)
	return manager
}
