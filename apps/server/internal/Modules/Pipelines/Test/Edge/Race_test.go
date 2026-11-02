package edge_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	runevents "github.com/Khaymah/dhow/apps/server/internal/Core/RunEvents"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	sha       = "4444444444444444444444444444444444444444"
	fanOut    = 100
	runners   = 30
	succeeded = runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED
)

type world struct {
	env     *apptest.Env
	org     uuid.UUID
	project uuid.UUID
	repo    uuid.UUID
	defs    *apptest.FakeDefinitions
	runners []*apptest.TestRunner
}

func newWorld(t *testing.T, source string) world {
	t.Helper()
	defs := apptest.NewFakeDefinitions()
	defs.Put(sha, strings.ReplaceAll(source, "IMAGE", apptest.TestImage))
	env := apptest.NewWithOptions(t, apptest.Options{Definitions: defs})
	owner, d := env.SetupAndLogin()
	org, ownerID := env.OrganizationID(d.OrgSlug), uuid.MustParse(owner.UserID)
	project := env.SeedProject(org, ownerID)
	w := world{env: env, org: org, project: project, repo: env.SeedRepository(org, project, ownerID), defs: defs}
	for range runners {
		w.runners = append(w.runners, env.SeedConnectedRunner(org, ownerID, apptest.SeedRunnerSpec{Capacity: 8}))
	}
	return w
}

func (w world) trigger() triggers.Trigger {
	return triggers.Trigger{
		OrganizationID: w.org, ProjectID: w.project, RepositoryID: w.repo, DeliveryID: uuid.New(), Provider: "generic",
		Event: triggers.EventPush, Ref: "refs/heads/main", Sha: sha,
	}
}

func parallel(n int, fn func(i int) error) []error {
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Go(func() {
			<-start
			errs[i] = fn(i)
		})
	}
	close(start)
	wg.Wait()
	return errs
}

func simple() string {
	return "version: 1\non:\n  push: {}\njobs:\n  build:\n    image: IMAGE\n    steps:\n      - run: make\n"
}

func TestOnceTheSameTriggerFromManyDeliveriesMakesOneRun(t *testing.T) {
	w := newWorld(t, simple())
	created := make([]bool, 20)
	errs := parallel(len(created), func(i int) error {
		return dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
			got, err := w.env.Wired.Pipelines.Deliver(context.Background(), q, w.trigger())
			created[i] = got.Created
			return err
		})
	})
	for _, err := range errs {
		require.NoError(t, err)
	}
	n := 0
	for _, c := range created {
		if c {
			n++
		}
	}
	assert.Equal(t, 1, n, "exactly one delivery made the run")
	assert.Equal(t, 1, w.env.CountRows(`SELECT count(*) FROM "PipelineRuns"`))
}

func fanPipeline() string {
	values := make([]string, fanOut)
	for i := range values {
		values[i] = fmt.Sprint(i + 1)
	}
	return "version: 1\non:\n  push: {}\njobs:\n  fan:\n    image: IMAGE\n    strategy:\n      max_parallel: 50\n      matrix:\n        n: [" +
		strings.Join(values, ", ") + "]\n    steps:\n      - run: make\n  final:\n    needs: [fan]\n    image: IMAGE\n    steps:\n      - run: make final\n"
}

func (w world) leaseAll(t *testing.T) []*apptest.Grant {
	t.Helper()
	var out []*apptest.Grant
	for _, r := range w.runners {
		for range 4 {
			g, err := r.Lease(context.Background(), 1, 0)
			if err != nil && strings.Contains(err.Error(), "LEASE_RATE_LIMITED") {
				break
			}
			require.NoError(t, err)
			if g == nil {
				return out
			}
			_, err = r.Start(g)
			require.NoError(t, err)
			out = append(out, g)
		}
	}
	return out
}

func (w world) completeAll(t *testing.T, grants []*apptest.Grant) {
	t.Helper()
	owner := map[*apptest.Grant]*apptest.TestRunner{}
	for _, g := range grants {
		for _, r := range w.runners {
			if g.Grant.GetRunnerId() == r.RunnerID {
				owner[g] = r
			}
		}
	}
	errs := parallel(len(grants), func(i int) error {
		g := grants[i]
		return owner[g].Complete(g, succeeded, 0, runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED)
	})
	for _, err := range errs {
		require.NoError(t, err)
	}
}

func TestRaceManyJobsFinishingTogetherQueueEachRunJobExactlyOnce(t *testing.T) {
	w := newWorld(t, fanPipeline())
	run := w.env.Deliver(w.trigger()).RunID
	w.env.StartRuns()
	first := w.leaseAll(t)
	require.Len(t, first, 50, "max_parallel holds the matrix to fifty at a time")
	w.completeAll(t, first)
	second := w.leaseAll(t)
	require.Len(t, second, 50, "every finish queued exactly one waiting job")
	w.completeAll(t, second)

	jobs := w.env.RunJobs(run)
	require.Len(t, jobs, fanOut+1)
	assert.Equal(t, "active", jobs["final"].State)
	per := map[string]int{}
	rows, err := w.env.Pool.Query(context.Background(),
		`SELECT rj."Key", count(j."Id") FROM "RunJobs" rj LEFT JOIN "Jobs" j ON j."OriginId" = rj."Id" WHERE rj."RunId" = $1 GROUP BY rj."Key"`, run)
	require.NoError(t, err)
	for rows.Next() {
		var key string
		var n int
		require.NoError(t, rows.Scan(&key, &n))
		per[key] = n
	}
	rows.Close()
	require.Len(t, per, fanOut+1)
	for key, n := range per {
		assert.Equal(t, 1, n, "%s was queued %d times", key, n)
	}
	w.completeAll(t, w.leaseAll(t))
	state, _ := w.env.RunState(run)
	assert.Equal(t, "succeeded", state)
}

func TestOnceAHundredCopiesOfTheSameEventMoveTheRunOnce(t *testing.T) {
	w := newWorld(t, "version: 1\non:\n  push: {}\njobs:\n  build:\n    image: IMAGE\n    steps:\n      - run: make\n"+
		"  ship:\n    needs: [build]\n    image: IMAGE\n    steps:\n      - run: make ship\n")
	run := w.env.Deliver(w.trigger()).RunID
	w.env.StartRuns()
	w.env.Wired.Jobs.UseRunEvents(nil)
	grants := w.leaseAll(t)
	require.Len(t, grants, 1)
	w.completeAll(t, grants)
	w.env.Wired.Jobs.UseRunEvents(w.env.Wired.Pipelines)
	build := w.env.RunJobs(run)["build"]
	require.Equal(t, "active", build.State)
	var origin uuid.UUID
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "OriginId" FROM "Jobs" WHERE "Id" = $1`, build.Job).Scan(&origin))
	errs := parallel(fanOut, func(int) error {
		return dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
			return w.env.Wired.Pipelines.JobFinished(context.Background(), q, runevents.Finished{
				OrganizationID: w.org, JobID: *build.Job, OriginID: origin, State: "succeeded",
			})
		})
	})
	for _, err := range errs {
		require.NoError(t, err)
	}
	jobs := w.env.RunJobs(run)
	assert.Equal(t, "succeeded", jobs["build"].State)
	assert.Equal(t, "active", jobs["ship"].State)
	assert.Equal(t, 2, w.env.CountRows(`SELECT count(*) FROM "Jobs"`), "ship was queued once, not once per copy")
}
