package deployrelease_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

func FuzzRollingPlan(f *testing.F) {
	f.Add(3, 1, 0, 5, 5, 3, 10)
	f.Add(10, 3, 600, 30, 20, 1, 10)
	f.Add(0, 0, -1, 0, 0, 0, 0)
	f.Add(10, 10, 5, 60, 60, 60, 60)
	f.Fuzz(func(t *testing.T, hosts, size, pauseSec, timeoutSec, intervalSec, stable, retries int) {
		pauseSec, timeoutSec, intervalSec = pauseSec%100_000, timeoutSec%100_000, intervalSec%100_000
		h := deployrelease.Health{
			URL: "http://{host}/healthz", StatusMin: 200, StatusMax: 399, Timeout: time.Duration(timeoutSec) * time.Second,
			Retries: retries, Interval: time.Duration(intervalSec) * time.Second, Stable: stable,
		}
		pause := time.Duration(pauseSec) * time.Second
		plan := deployrelease.PlanBatches(hosts, size)
		if err := deployrelease.ValidateRollout(hosts, size, pause, h); err != nil {
			return
		}
		require.NotEmpty(t, plan)
		next := 0
		for _, b := range plan {
			require.Equal(t, next, b[0], "the batches follow each other without a gap")
			require.Greater(t, b[1], b[0])
			require.LessOrEqual(t, b[1]-b[0], size)
			next = b[1]
		}
		require.Equal(t, hosts, next, "every server is in exactly one batch")
		require.Len(t, plan, (hosts+size-1)/size)
		require.LessOrEqual(t, time.Duration(len(plan))*h.Budget()+time.Duration(len(plan)-1)*pause, deployrelease.MaxRollingBudget)
	})
}
