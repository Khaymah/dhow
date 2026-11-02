package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

const derivedPipeline = `version: 1
on:
  push: {}
jobs:
  build:
    image: IMAGE
    steps:
      - run: make
  ship:
    needs: [build]
    image: IMAGE
    env:
      VERSION: ${{ needs.build.outputs.token }}
      MODE: fast
      BRANCH: ${{ git.branch }}
    steps:
      - run: echo skipped
        if: ${{ git.branch == 'nope' }}
      - run: make ship
        env:
          TAG: ${{ needs.build.outputs.token }}
          PLAIN: "yes"
`

const derivedToken = "registry-login-derived-from-a-secret"

type jobDetail struct {
	Spec struct {
		Env   map[string]string
		Steps []struct {
			Run string
			Env map[string]string
		}
	}
}

func (w world) completeWithOutputs(t *testing.T, outputs map[string]string) {
	t.Helper()
	for _, r := range w.runners {
		g, err := r.Lease(context.Background(), 1, 0)
		require.NoError(t, err)
		if g == nil {
			continue
		}
		_, err = r.Start(g)
		require.NoError(t, err)
		require.NoError(t, r.CompleteOutputs(g, outputs))
		return
	}
	t.Fatal("no runner had a job to lease")
}

func TestAViewerAndADeveloperSeeEveryDerivedValueRedacted(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline(derivedPipeline))
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	state, reason := w.env.RunState(run)
	require.Equal(t, "running", state, "%s %v", reason, w.view(t, run).Issues)
	w.completeWithOutputs(t, map[string]string{"token": derivedToken})
	ship := w.env.RunJobs(run)["ship"]
	require.Equal(t, "active", ship.State)

	var stored []byte
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "Spec"::text FROM "Jobs" WHERE "Id" = $1`, *ship.Job).Scan(&stored))
	assert.Contains(t, string(stored), derivedToken, "the runner still gets the value")

	for _, role := range []string{"viewer", "developer", "owner"} {
		t.Run(role, func(t *testing.T) {
			client := w.owner
			if role != "owner" {
				email := role + "@example.com"
				w.env.SeedUser(w.org, email, "a long password for "+role, role)
				c, res := w.env.Login(email, "a long password for "+role)
				require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
				client = c
			}
			res := client.Get(apptest.OrgPath(w.org, "/jobs/"+ship.Job.String()))
			require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
			assert.NotContains(t, string(res.Raw), derivedToken)
			var d jobDetail
			res.Decode(t, &d)
			assert.Equal(t, map[string]string{"VERSION": "[REDACTED]", "MODE": "fast", "BRANCH": "[REDACTED]"}, d.Spec.Env)
			require.Len(t, d.Spec.Steps, 1)
			assert.Equal(t, "make ship", d.Spec.Steps[0].Run)
			assert.Equal(t, map[string]string{"TAG": "[REDACTED]", "PLAIN": "yes"}, d.Spec.Steps[0].Env)
		})
	}
}
