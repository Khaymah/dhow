package e2e_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commitstatus "github.com/Khaymah/dhow/apps/server/internal/Core/CommitStatus"
	runevents "github.com/Khaymah/dhow/apps/server/internal/Core/RunEvents"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var errCrash = errors.New("the server died before the commit")

type crashAfter struct {
	next runevents.Sink
}

func (c crashAfter) JobFinished(ctx context.Context, tx *sqlc.Queries, f runevents.Finished) error {
	if err := c.next.JobFinished(ctx, tx, f); err != nil {
		return err
	}
	return errCrash
}

func TestAServerThatDiesBeforeTheCommitLosesNothing(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline("version: 1\non:\n  push: {}\njobs:\n  build:\n    image: IMAGE\n    steps:\n      - run: make\n"+
		"  ship:\n    needs: [build]\n    image: IMAGE\n    steps:\n      - run: make ship\n"))
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	r := w.runners[0]
	g := r.MustLease(0)
	_, err := r.Start(g)
	require.NoError(t, err)

	w.env.Wired.Jobs.UseRunEvents(crashAfter{next: w.env.Wired.Pipelines})
	require.Error(t, r.Complete(g, succeeded, 0, runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED))
	job := uuid.MustParse(g.Grant.GetJobId())
	state, _ := w.env.JobState(job)
	assert.Equal(t, "running", state, "the job's end and the run's step roll back together")
	jobs := w.env.RunJobs(run)
	assert.Equal(t, "active", jobs["build"].State)
	assert.Equal(t, "waiting", jobs["ship"].State)
	assert.Equal(t, 1, w.env.CountRows(`SELECT count(*) FROM "Jobs"`))

	w.env.Wired.Jobs.UseRunEvents(w.env.Wired.Pipelines)
	require.NoError(t, r.Complete(g, succeeded, 0, runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED), "after the restart the runner sends its result again")
	jobs = w.env.RunJobs(run)
	assert.Equal(t, "succeeded", jobs["build"].State)
	assert.Equal(t, "active", jobs["ship"].State, "the run goes on from where it was")
	assert.Equal(t, []string{"ship"}, w.finishN(t, 1, succeeded))
	runState, _ := w.env.RunState(run)
	assert.Equal(t, "succeeded", runState)
}

func TestTheReconcilerRepairsARunJobThatMissedItsEvent(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline("version: 1\non:\n  push: {}\njobs:\n  build:\n    image: IMAGE\n    steps:\n      - run: make\n"))
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	w.env.Wired.Jobs.UseRunEvents(nil)
	assert.Equal(t, []string{"build"}, w.finishN(t, 1, succeeded))
	assert.Equal(t, "active", w.env.RunJobs(run)["build"].State)
	w.env.Wired.Jobs.UseRunEvents(w.env.Wired.Pipelines)
	w.env.Clock.Advance(time.Minute)
	res, err := w.env.Wired.Pipelines.Reconcile(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Drifted)
	state, _ := w.env.RunState(run)
	assert.Equal(t, "succeeded", state)
}

type recorded struct {
	states []string
}

func (r *recorded) Record(_ context.Context, _ *sqlc.Queries, d commitstatus.Desired) error {
	r.states = append(r.states, d.State)
	return nil
}

func TestRunsReportTheirStateToTheProviderInTheSameTransaction(t *testing.T) {
	w := newWorld(t)
	statuses := &recorded{}
	w.env.Wired.Pipelines.UseStatuses(statuses)
	w.defs.Put(shaA, pipeline("version: 1\non:\n  push: {}\njobs:\n  build:\n    image: IMAGE\n    steps:\n      - run: make\n"))
	tr := w.trigger("push", "refs/heads/main", shaA)
	tr.Provider = "gitlab"
	w.env.Deliver(tr)
	w.env.StartRuns()
	w.drain(t, succeeded)
	assert.Equal(t, []string{"pending", "running", "success"}, statuses.states)
}
