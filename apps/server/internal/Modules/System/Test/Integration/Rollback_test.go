package integration_test

import (
	"context"
	"io/fs"
	"net/http"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
	support "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Test/Support"
)

func newerFS(t *testing.T, extra map[string]string) fs.FS {
	t.Helper()
	out := fstest.MapFS{}
	names, err := fs.Glob(migrations.FS, "*.sql")
	require.NoError(t, err)
	for _, name := range names {
		body, err := fs.ReadFile(migrations.FS, name)
		require.NoError(t, err)
		out[name] = &fstest.MapFile{Data: body}
	}
	for name, body := range extra {
		out[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return out
}

func TestReadinessAfterABinaryRollback(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	newer, err := migrations.NewMigratorFS(pool, newerFS(t, map[string]string{
		"99001_newer_expand.sql": "-- dhow:phase expand\n-- +goose Up\nALTER TABLE \"Instance\" ADD COLUMN \"NewerThing\" TEXT;\n",
	}), migrations.Binary{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, newer.Close()) })
	_, err = newer.Up(ctx)
	require.NoError(t, err)

	older, err := migrations.NewMigrator(pool, migrations.Binary{Version: "1.2.0"})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, older.Close()) })
	h := support.Router(system.Deps{DB: pool, Migrations: older, Queries: sqlc.New(pool)})

	rec := support.Do(h, http.MethodGet, "/api/v1/health/ready", nil)
	assert.Equal(t, http.StatusOK, rec.Code, "an older binary is ready on an expand-only newer schema: %s", rec.Body.String())

	_, err = pool.Exec(ctx, `UPDATE "Instance" SET "SchemaFloor" = '1.3.0'`)
	require.NoError(t, err)
	rec = support.Do(h, http.MethodGet, "/api/v1/health/ready", nil)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "a floor above the binary makes it not ready")
	assert.Contains(t, rec.Body.String(), `"Name":"migrations","Ok":false`)
}
