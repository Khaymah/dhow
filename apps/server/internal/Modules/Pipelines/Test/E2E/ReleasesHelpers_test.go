package e2e_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func (j releaseJob) sweepReleases(t *testing.T) { j.reopen(t) }

func (j releaseJob) runningReleases(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, j.env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "DeploymentReleases" WHERE "State" = 'running'`).Scan(&n))
	return n
}
