package apptest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
)

const dekAADLabel = "dhow/dek/v1"

type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func dekAAD(id uuid.UUID, kind string, owner *uuid.UUID, version int32) []byte {
	out := append([]byte(dekAADLabel), 0)
	out = append(out, id[:]...)
	out = append(out, byte(len(kind))) //nolint:gosec // Owner kinds are short constants.
	out = append(out, kind...)
	var ownerID uuid.UUID
	if owner != nil {
		ownerID = *owner
	}
	out = append(out, ownerID[:]...)
	return binary.BigEndian.AppendUint32(out, uint32(version)) //nolint:gosec // KEK versions are positive.
}

func DataKeysInClear(ctx context.Context, db Querier, provider keys.KeyProvider) ([][]byte, error) {
	rows, err := db.Query(ctx, `SELECT "Id", "OwnerKind", "OwnerId", "KekVersion", "Wrapped" FROM "DataKeys" WHERE "Wrapped" IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var id uuid.UUID
		var kind string
		var owner *uuid.UUID
		var version int32
		var wrapped []byte
		if err := rows.Scan(&id, &kind, &owner, &version, &wrapped); err != nil {
			return nil, err
		}
		dek, err := provider.Unwrap(ctx, version, wrapped, dekAAD(id, kind, owner, version))
		if err != nil {
			return nil, fmt.Errorf("unwrap data key %s: %w", id, err)
		}
		out = append(out, dek)
	}
	return out, rows.Err()
}

func KeyringKeys(ring *keys.Keyring) ([][]byte, error) {
	var out [][]byte
	for line := range bytes.Lines(ring.Marshal()) {
		fields := bytes.Fields(line)
		if len(fields) != 3 {
			return nil, fmt.Errorf("a keyring line has %d fields", len(fields))
		}
		key, err := base64.RawURLEncoding.DecodeString(string(fields[2]))
		if err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, nil
}

func (e *Env) Registry() *keys.Registry {
	e.T.Helper()
	reg, err := app.ValueRegistry()
	require.NoError(e.T, err)
	return reg
}

func (e *Env) AllValues(t testing.TB) map[string][]keys.Value {
	t.Helper()
	out := map[string][]keys.Value{}
	for _, l := range e.Registry().Locations() {
		after := ""
		for {
			var batch []keys.Value
			require.NoError(t, pgx.BeginFunc(context.Background(), e.Pool, func(tx pgx.Tx) error {
				var err error
				batch, err = l.Scan(context.Background(), tx, after, 500)
				return err
			}))
			out[l.Spec().Name()] = append(out[l.Spec().Name()], batch...)
			if len(batch) < 500 {
				break
			}
			after = batch[len(batch)-1].Key
		}
	}
	return out
}
