package serverinstances_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serverinstances "github.com/Khaymah/dhow/apps/server/internal/Core/ServerInstances"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

const staleAfter = 90 * time.Second

func heartbeat(pool *pgxpool.Pool, host string) *serverinstances.Heartbeat {
	return serverinstances.New(sqlc.New(pool), serverinstances.Identity{Key: host, Hostname: host, Version: "1.0.0", SchemaVersion: 50},
		staleAfter, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func liveHosts(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := sqlc.New(pool).ListLiveServerInstances(ctx, staleAfter.Seconds())
	require.NoError(t, err)
	var out []string
	for _, r := range rows {
		out = append(out, r.Hostname)
	}
	return out
}

func TestHeartbeat(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewPool(t)
	a, b := heartbeat(pool, "a"), heartbeat(pool, "b")
	require.NoError(t, a.Register(ctx))
	require.NoError(t, b.Register(ctx))
	assert.NotEqual(t, a.ID(), b.ID())
	assert.Equal(t, []string{"a", "b"}, liveHosts(t, ctx, pool))

	_, err := pool.Exec(ctx, `UPDATE "ServerInstances" SET "LastSeenAt" = now() - interval '10 minutes' WHERE "Hostname" = 'b'`)
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, liveHosts(t, ctx, pool), "a server that stopped beating is not live")
	require.NoError(t, a.Beat(ctx))
	var retired bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT "IsDeleted" FROM "ServerInstances" WHERE "Id" = $1`, b.ID()).Scan(&retired))
	assert.True(t, retired, "a beat retires stale servers")

	old := b.ID()
	require.NoError(t, b.Beat(ctx))
	assert.Equal(t, old, b.ID(), "a server whose row was retired registers again, in the same row")
	assert.Equal(t, []string{"a", "b"}, liveHosts(t, ctx, pool))

	require.NoError(t, a.Retire(ctx))
	assert.Equal(t, []string{"b"}, liveHosts(t, ctx, pool))
}

func TestHeartbeatLoopStopsWithItsContext(t *testing.T) {
	pool := dbtest.NewPool(t)
	h := heartbeat(pool, "loop")
	require.NoError(t, h.Register(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		h.Loop(ctx, 10*time.Millisecond)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the loop did not stop")
	}
}

func TestARestartReusesTheRowOfItsInstance(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewPool(t)
	first := heartbeat(pool, "web-1")
	require.NoError(t, first.Register(ctx))
	require.NoError(t, first.Retire(ctx))

	second := serverinstances.New(sqlc.New(pool), serverinstances.Identity{Key: "web-1", Hostname: "web-1", Version: "1.1.0", SchemaVersion: 51},
		staleAfter, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, second.Register(ctx))
	assert.Equal(t, first.ID(), second.ID(), "a restart updates its row in place")
	var rows int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM "ServerInstances"`).Scan(&rows))
	assert.Equal(t, 1, rows, "restarts do not add rows")
	live, err := sqlc.New(pool).ListLiveServerInstances(ctx, staleAfter.Seconds())
	require.NoError(t, err)
	require.Len(t, live, 1)
	assert.Equal(t, "1.1.0", live[0].Version)
	assert.Equal(t, int64(51), live[0].SchemaVersion)
}
