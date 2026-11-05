package instancesettings

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
	"github.com/Khaymah/dhow/internal/logging"
)

func TestAFailedReloadRetriesWithABoundedBackoffAndFailsReadinessUntilItRecovers(t *testing.T) {
	pool := dbtest.NewPool(t)
	var waits []time.Duration
	var readyWhileFailing []error
	var s *Store
	s, err := New(Options{
		DB: pool, Values: testValues(t, pool), Log: logging.New(io.Discard, slog.LevelError),
		Wait: func(ctx context.Context, d time.Duration) bool {
			waits = append(waits, d)
			readyWhileFailing = append(readyWhileFailing, s.Ready(ctx))
			if len(waits) == 7 {
				_, err := pool.Exec(ctx, `UPDATE "InstanceSettings" SET "Value" = '{"RetentionDays": 9}'::jsonb WHERE "Group" = 'logs'`)
				require.NoError(t, err)
			}
			return true
		},
	})
	require.NoError(t, err)
	require.NoError(t, s.Load(t.Context()))
	require.NoError(t, s.Ready(t.Context()))

	_, err = pool.Exec(t.Context(), `INSERT INTO "InstanceSettings" ("Group", "Value") VALUES ('logs', '{"RetentionDays": 0}'::jsonb)`)
	require.NoError(t, err)
	s.Reload(t.Context())

	assert.Equal(t, []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second,
	}, waits, "the backoff doubles and stops growing at the cap")
	for _, e := range readyWhileFailing {
		require.ErrorIs(t, e, ErrReloadFailed)
	}
	require.NoError(t, s.Ready(t.Context()), "the next successful reload recovers")
	assert.Equal(t, 9, s.Current().Values.Logs.RetentionDays)
}

func TestAReloadStopsRetryingWhenTheServerStops(t *testing.T) {
	pool := dbtest.NewPool(t)
	_, err := pool.Exec(t.Context(), `INSERT INTO "InstanceSettings" ("Group", "Value") VALUES ('logs', '{"RetentionDays": 0}'::jsonb)`)
	require.NoError(t, err)
	calls := 0
	s, err := New(Options{
		DB: pool, Values: testValues(t, pool), Log: logging.New(io.Discard, slog.LevelError),
		Wait: func(context.Context, time.Duration) bool {
			calls++
			return false
		},
	})
	require.NoError(t, err)
	s.Reload(t.Context())
	assert.Equal(t, 1, calls)
	require.ErrorIs(t, s.Ready(t.Context()), ErrReloadFailed, "a stale instance stays not ready")
}
