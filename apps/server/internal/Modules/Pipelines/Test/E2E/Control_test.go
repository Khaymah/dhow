package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func (w world) auditCount(action string) int {
	return w.env.CountRows(`SELECT count(*) FROM "AuditEvents" WHERE "Action" = $1`, action)
}

func TestCancelingARunReachesTheRunnerAndCancelsWhatWaits(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline(dagPipeline))
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	assert.Equal(t, 1, w.auditCount("project.run_started"), "the run that queued jobs is in the audit log")
	r := w.runners[0]
	g := r.MustLease(0)
	_, err := r.Start(g)
	require.NoError(t, err)
	leased := w.env.RunJobKey(uuid.MustParse(g.Grant.GetJobId()))

	res := w.post(t, w.owner, run, "cancel")
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	jobs := w.env.RunJobs(run)
	for _, b := range append(builds(), "publish") {
		assert.Equal(t, "canceled", jobs[b].State, b)
	}
	queued := "unit"
	if leased == "unit" {
		queued = "lint"
	}
	assert.Equal(t, "canceled", jobs[queued].State, "a job no runner took yet is canceled at once")
	assert.Equal(t, "active", jobs[leased].State)
	ctrl, err := r.Heartbeat(g.Ref)
	require.NoError(t, err)
	assert.True(t, ctrl.GetCancelRequested(), "the runner sees the cancel on its next heartbeat")
	assert.Equal(t, 1, w.auditCount("project.run_cancel_requested"))

	require.NoError(t, r.Complete(g, runnerv1.AttemptResult_ATTEMPT_RESULT_CANCELED, 0, runnerv1.FailureReason_FAILURE_REASON_CANCELED))
	state, reason := w.env.RunState(run)
	assert.Equal(t, []string{"canceled", "CANCELED"}, []string{state, reason})
	again := w.post(t, w.owner, run, "cancel")
	assert.Equal(t, http.StatusOK, again.Status, "cancel is idempotent")
	assert.Equal(t, 1, w.auditCount("project.run_cancel_requested"))
}

const groupPipeline = `version: 1
on:
  push: {}
concurrency:
  group: ${{ "deploy-" + git.branch }}
  cancel_in_progress: CANCEL
jobs:
  deploy:
    image: IMAGE
    steps:
      - run: make deploy
`

func groupFile(cancel string) string {
	return pipeline(replace(groupPipeline, "CANCEL", cancel))
}

func TestAConcurrencyGroupMakesTheNextRunWait(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, groupFile("false"))
	w.defs.Put(shaB, groupFile("false"))
	first := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	second := w.push("refs/heads/main", shaB)
	w.env.StartRuns()
	state, _ := w.env.RunState(second)
	assert.Equal(t, "queued", state, "the newer run waits for the older one in its group")
	assert.Equal(t, []string{"deploy"}, w.finishN(t, 1, succeeded))
	state, _ = w.env.RunState(first)
	assert.Equal(t, "succeeded", state)
	w.env.StartRuns()
	state, _ = w.env.RunState(second)
	assert.Equal(t, "running", state)
	v := w.view(t, second)
	require.NotNil(t, v.Run.ConcurrencyGroup)
	assert.Equal(t, "deploy-main", *v.Run.ConcurrencyGroup)
}

func TestCancelInProgressCancelsTheOlderRun(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, groupFile("true"))
	w.defs.Put(shaB, groupFile("true"))
	first := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	second := w.push("refs/heads/main", shaB)
	w.env.StartRuns()
	state, reason := w.env.RunState(first)
	assert.Equal(t, []string{"canceled", "CANCELED"}, []string{state, reason})
	state, _ = w.env.RunState(second)
	assert.Equal(t, "running", state)
	jobs := w.env.RunJobs(first)
	assert.Equal(t, "canceled", jobs["deploy"].State)
	jobState, _ := w.env.JobState(*jobs["deploy"].Job)
	assert.Equal(t, "canceled", jobState)
}

const forkBase = `version: 1
on:
  pull_request:
    branches: [main]
jobs:
  test:
    image: IMAGE
    steps:
      - run: make test
  deploy:
    image: IMAGE
    environment: production
    steps:
      - run: make deploy
  publish:
    image: IMAGE
    env:
      TOKEN: ${{ secrets.TOKEN }}
    steps:
      - run: make publish
`

const forkHead = `version: 1
on:
  pull_request: {}
jobs:
  mine:
    image: IMAGE
    network:
      egress: [evil.example.com:443]
    steps:
      - run: curl https://evil.example.com
`

func (w world) forkPR() uuid.UUID {
	w.env.T.Helper()
	tr := w.trigger("pull_request", "refs/pull/7/head", shaB)
	tr.BaseRef, tr.BaseSha, tr.PrNumber, tr.Origin = "refs/heads/main", shaA, 7, triggers.PullOriginFork
	got := w.env.Deliver(tr)
	require.True(w.env.T, got.Created)
	return got.RunID
}

func TestAForkPullRequestWaitsForApprovalAndRunsTheBaseFile(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline(forkBase))
	w.defs.Put(shaB, pipeline(forkHead))
	run := w.forkPR()
	w.env.StartRuns()
	state, _ := w.env.RunState(run)
	require.Equal(t, "awaiting_approval", state, "a fork's run takes no runner before someone approves it")
	assert.Zero(t, w.defs.Calls())

	developer, _ := w.env.SignedIn(w.org, "dev@example.com", "developer")
	assert.Equal(t, http.StatusForbidden, w.post(t, developer, run, "approve").Status)
	maintainer, _ := w.env.SignedIn(w.org, "maintainer@example.com", "maintainer")
	res := w.post(t, maintainer, run, "approve")
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	assert.Equal(t, http.StatusConflict, w.post(t, maintainer, run, "approve").Status)
	w.env.StartRuns()

	v := w.view(t, run)
	assert.Equal(t, shaA, v.Run.DefinitionSha, "the file comes from the base, not from the fork")
	require.NotNil(t, v.Run.Sha)
	assert.Equal(t, shaB, *v.Run.Sha, "the code that runs is the fork's head")
	assert.Equal(t, sha256Hex(pipeline(forkBase)), v.Run.DefinitionDigest)
	jobs := w.env.RunJobs(run)
	_, mine := jobs["mine"]
	assert.False(t, mine, "a job the fork added never runs")
	assert.Equal(t, "active", jobs["test"].State)
	assert.Equal(t, []string{"skipped", "FORK_PR_NO_ENVIRONMENT"}, []string{jobs["deploy"].State, jobs["deploy"].Reason})
	assert.Equal(t, []string{"failed", "FORK_PR_NO_SECRETS"}, []string{jobs["publish"].State, jobs["publish"].Reason})
	assert.Equal(t, 1, w.auditCount("project.run_approved"))
}

func TestAProjectThatTrustsForksRunsThemAtOnce(t *testing.T) {
	w := newWorld(t)
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "Projects" SET "ForkPolicy" = 'automatic' WHERE "Id" = $1`, w.project)
	require.NoError(t, err)
	w.defs.Put(shaA, pipeline(forkBase))
	run := w.forkPR()
	w.env.StartRuns()
	state, _ := w.env.RunState(run)
	assert.Equal(t, "running", state)
}

func TestATagIsPinnedOnceAndEveryRerunPullsTheSameDigest(t *testing.T) {
	w := newWorld(t)
	first := "docker.io/library/alpine:3@sha256:" + repeat("a", 64)
	w.images.move("docker.io/library/alpine:3", first)
	w.defs.Put(shaA, "version: 1\non:\n  push: {}\njobs:\n  build:\n    image: alpine:3\n    steps:\n      - run: make\n")
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	v := w.view(t, run)
	require.Len(t, v.ImagePins, 1)
	assert.Equal(t, "alpine:3", v.ImagePins[0].Image)
	assert.Equal(t, first, v.ImagePins[0].Pinned)
	assert.Equal(t, first, w.jobImage(t, *w.env.RunJobs(run)["build"].Job))

	w.images.move("docker.io/library/alpine:3", "docker.io/library/alpine:3@sha256:"+repeat("b", 64))
	assert.Equal(t, []string{"build"}, w.finishN(t, 1, failed))
	state, _ := w.env.RunState(run)
	require.Equal(t, "failed", state)
	res := w.post(t, w.owner, run, "rerun-failed")
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	again := w.env.RunJobs(run)["build"]
	assert.Equal(t, "active", again.State)
	assert.Equal(t, first, w.jobImage(t, *again.Job), "a rerun pulls the digest the run started with, though the tag moved")
	assert.Equal(t, 1, w.images.calls, "the registry is asked once per run")
	assert.Equal(t, 1, w.auditCount("project.run_rerun_requested"))
	assert.Equal(t, []string{"build"}, w.finishN(t, 1, succeeded))
	assert.Equal(t, http.StatusConflict, w.post(t, w.owner, run, "rerun-failed").Status, "a run that succeeded has nothing to run again")

	w.defs.Put(shaB, "version: 1\non:\n  push: {}\njobs:\n  build:\n    image: ghost:1\n    steps:\n      - run: make\n")
	unpinned := w.push("refs/heads/main", shaB)
	w.env.StartRuns()
	u := w.view(t, unpinned)
	assert.Equal(t, "IMAGE_PIN_FAILED", u.Run.FailureReason)
	require.NotEmpty(t, u.Issues)
	assert.Equal(t, "jobs.build.image", u.Issues[len(u.Issues)-1].Path)
}

func (w world) jobImage(t *testing.T, job uuid.UUID) string {
	t.Helper()
	var image string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "Spec"->>'image' FROM "Jobs" WHERE "Id" = $1`, job).Scan(&image))
	return image
}

func TestTheRoleOnTheProjectDecidesWhoControlsRuns(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline(dagPipeline))
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	maintainer, id := w.env.SignedIn(w.org, "lowered@example.com", "maintainer")
	w.env.SeedUserProjectBinding(w.org, w.project, id, "viewer")
	assert.Equal(t, http.StatusOK, maintainer.Get(w.runPath(run, "")).Status)
	assert.Equal(t, http.StatusForbidden, w.post(t, maintainer, run, "cancel").Status)
	res := maintainer.Do(apptest.Request{
		Method: http.MethodPost, Path: apptest.OrgPath(w.org, "/jobs/"+w.env.RunJobs(run)["lint"].Job.String()+"/cancel"),
	})
	assert.Equal(t, http.StatusForbidden, res.Status, "the organization route uses the role on the job's project too")
}
