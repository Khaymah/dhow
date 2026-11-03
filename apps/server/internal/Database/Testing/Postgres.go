package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
)

const connectTimeout = 15 * time.Second

func dsn(t testing.TB) string {
	t.Helper()
	v := config.TestDatabaseURL()
	if v == "" {
		if config.RunningInCI() {
			t.Fatal(config.EnvTestDatabaseURL + " must be set in CI")
		}
		t.Skip(config.EnvTestDatabaseURL + " not set")
	}
	return v
}

func NewEmptyPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	admin, err := pgxpool.New(ctx, dsn(t))
	require.NoError(t, err)
	defer admin.Close()

	suffix := make([]byte, 6)
	_, err = rand.Read(suffix)
	require.NoError(t, err)
	schema := "t_" + hex.EncodeToString(suffix)
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize())
	require.NoError(t, err)

	cfg, err := pgxpool.ParseConfig(dsn(t))
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 20
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		pool.Close()
		cleanup, cancelCleanup := context.WithTimeout(context.Background(), connectTimeout)
		defer cancelCleanup()
		dropper, dropErr := pgxpool.New(cleanup, dsn(t))
		if dropErr != nil {
			return
		}
		defer dropper.Close()
		_, dropErr = dropper.Exec(cleanup, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		require.NoError(t, dropErr)
	})
	return pool
}

func NewPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	pool := NewEmptyPool(t)
	m, err := migrations.NewMigrator(pool, migrations.Binary{})
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()
	_, err = m.Up(context.Background())
	require.NoError(t, err)
	return pool
}
