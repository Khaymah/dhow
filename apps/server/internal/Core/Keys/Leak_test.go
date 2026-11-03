package keys_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

func encodings(raw []byte) []string {
	return []string{
		string(raw), hex.EncodeToString(raw), base64.StdEncoding.EncodeToString(raw), base64.RawURLEncoding.EncodeToString(raw),
		base64.RawStdEncoding.EncodeToString(raw)[:20], hex.EncodeToString(raw)[:24],
	}
}

func TestNoKeyMaterialLeavesTheKeyringInLogsErrorsOrStatus(t *testing.T) {
	pool := dbtest.NewPool(t)
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	both, _, err := ring.WithNextVersion()
	require.NoError(t, err)
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	provider, err := keys.NewFileProvider(both)
	require.NoError(t, err)
	m := keys.NewManager(pool, provider, log)
	_, err = m.Register(t.Context())
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `UPDATE "KeyEncryptionKeys" SET "State" = 'pending', "ActivatedAt" = NULL WHERE "Version" = 2`)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `UPDATE "KeyEncryptionKeys" SET "State" = 'active', "ActivatedAt" = now() WHERE "Version" = 1`)
	require.NoError(t, err)
	owner := keys.ProjectOwner(uuid.New())
	sealed, err := m.Envelope().Seal(t.Context(), owner, "dhow/secret/v1", []byte("row"), []byte("planted secret value"))
	require.NoError(t, err)
	_, err = m.Activate(t.Context(), 2, 90)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	m.RewrapLoop(ctx, 50*time.Millisecond)
	cancel()
	st, err := m.Status(t.Context())
	require.NoError(t, err)
	statusJSON, err := json.Marshal(st)
	require.NoError(t, err)

	var row struct {
		id      uuid.UUID
		version int32
		wrapped []byte
	}
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT "Id", "KekVersion", "Wrapped" FROM "DataKeys"`).Scan(&row.id, &row.version, &row.wrapped))
	require.Equal(t, int32(2), row.version)
	dek, err := provider.Unwrap(t.Context(), row.version, row.wrapped, keys.DekAADForTest(row.id, owner, row.version))
	require.NoError(t, err)

	other, err := keys.GenerateKeyring(1, 2)
	require.NoError(t, err)
	var errs []string
	for _, p := range []*keys.Keyring{other, keyringWithout(t, both, 2)} {
		bad, err := keys.NewFileProvider(p)
		require.NoError(t, err)
		_, err = keys.NewManager(pool, bad, log).Register(t.Context())
		require.Error(t, err)
		errs = append(errs, err.Error())
	}
	_, err = m.Envelope().Open(t.Context(), keys.ProjectOwner(uuid.New()), "dhow/secret/v1", []byte("row"), sealed)
	errs = append(errs, err.Error())

	var dbText strings.Builder
	rows, err := pool.Query(t.Context(), `SELECT "CheckValue" FROM "KeyEncryptionKeys"`)
	require.NoError(t, err)
	for rows.Next() {
		var check []byte
		require.NoError(t, rows.Scan(&check))
		dbText.Write(check)
	}
	rows.Close()
	dbText.Write(row.wrapped)
	dbText.Write(sealed)

	everything := logs.String() + string(statusJSON) + strings.Join(errs, "\n") + fmt.Sprintf("%v %+v", st, both)
	var secrets []string
	for _, l := range strings.Split(strings.TrimSpace(string(both.Marshal())), "\n") {
		encoded := l[strings.LastIndexByte(l, ' ')+1:]
		raw, err := base64.RawURLEncoding.DecodeString(encoded)
		require.NoError(t, err)
		secrets = append(secrets, encodings(raw)...)
	}
	secrets = append(secrets, encodings(dek)...)
	for _, s := range secrets {
		require.NotContains(t, everything, s, "key material in logs, errors or status")
		require.NotContains(t, dbText.String(), s, "key material stored in the database")
	}
	require.NotContains(t, dbText.String(), "planted secret value")
	require.Contains(t, logs.String(), "re-wrapped data keys")
}
