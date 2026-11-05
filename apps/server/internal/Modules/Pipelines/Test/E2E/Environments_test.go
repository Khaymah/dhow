package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	envValue    = "a production key that only main may open"
	taggedImage = "registry.example.com/dhow/test:1"

	envPipeline = `version: 1
on:
  push: {}
  tag: {}
  pull_request: {}
jobs:
  build:
    image: IMAGE
    steps:
      - run: make
  deploy:
    image: IMAGE
    needs: [build]
    environment: production
    if: ${{ vars.TARGET == 'prod-1' }}
    steps:
      - run: ./deploy.sh
        env:
          KEY: ${{ secrets.PROD_KEY }}
`

	plainDeployPipeline = `version: 1
on:
  push: {}
jobs:
  deploy:
    image: IMAGE
    environment: production
    steps:
      - run: ./deploy.sh
`

	missingEnvPipeline = `version: 1
on:
  push: {}
jobs:
  deploy:
    image: IMAGE
    environment: staging
    steps:
      - run: ./deploy.sh
`
)

func (w world) production(t *testing.T) uuid.UUID {
	t.Helper()
	id := w.env.SeedEnvironment(w.project, "production")
	w.env.SeedSecret(apptest.SecretRef(w.org, &w.project, "production"), "PROD_KEY", envValue)
	w.env.SeedVariable(apptest.SecretRef(w.org, &w.project, "production"), "TARGET", "prod-1")
	return id
}

func (w world) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	_, err := w.env.Pool.Exec(context.Background(), sql, args...)
	require.NoError(t, err)
}

func (w world) complete(t *testing.T, key string) {
	t.Helper()
	r, g := w.leaseKey(t, key)
	_, err := r.Start(g)
	require.NoError(t, err)
	require.NoError(t, r.Complete(g, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, 0, runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED))
}

func (w world) reopen(t *testing.T) {
	t.Helper()
	w.exec(t, `UPDATE "PipelineRuns" SET "UpdatedAt" = now() - interval '1 minute' WHERE "State" = 'running'`)
	_, err := w.env.Wired.Pipelines.Reconcile(context.Background())
	require.NoError(t, err)
}

func (w world) gate(t *testing.T, run uuid.UUID, key string) (string, *time.Time) {
	t.Helper()
	var gate *string
	var opens *time.Time
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT "Gate", "GateOpensAt" FROM "RunJobs" WHERE "RunId" = $1 AND "Key" = $2`, run, key).Scan(&gate, &opens))
	if gate == nil {
		return "", opens
	}
	return *gate, opens
}

func (w world) secretUsed(t *testing.T) bool {
	t.Helper()
	var used *time.Time
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT "LastUsedAt" FROM "Secrets" WHERE "Name" = 'PROD_KEY'`).Scan(&used))
	return used != nil
}

func (w world) deployState(t *testing.T, env uuid.UUID) (string, *uuid.UUID) {
	t.Helper()
	var state string
	var holder *uuid.UUID
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT COALESCE((SELECT d."State" FROM "Deployments" d WHERE d."EnvironmentId" = e."Id" ORDER BY d."CreatedAt" DESC LIMIT 1), ''),
		        e."LockedByRunId"
		 FROM "Environments" e WHERE e."Id" = $1`, env).Scan(&state, &holder))
	return state, holder
}

func (w world) toDeploy(t *testing.T, sha string) uuid.UUID {
	t.Helper()
	run := w.push("refs/heads/main", sha)
	w.env.StartRuns()
	w.complete(t, "build")
	return run
}

func TestAJobOnAnEnvironmentThatDoesNotExistFails(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline(missingEnvPipeline))
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	jobs := w.env.RunJobs(run)
	assert.Equal(t, []string{"failed", "ENVIRONMENT_NOT_FOUND"}, []string{jobs["deploy"].State, jobs["deploy"].Reason})
}

func TestEnvironmentSecretsReachTheJobOnAnAllowedBranch(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	w.defs.Put(shaA, pipeline(envPipeline))
	run := w.toDeploy(t, shaA)
	require.Equal(t, "active", w.env.RunJobs(run)["deploy"].State, "vars.TARGET from the environment made if true")
	state, holder := w.deployState(t, env)
	assert.Equal(t, "queued", state)
	require.NotNil(t, holder)
	assert.Equal(t, run, *holder, "the run holds the environment")

	r, g := w.leaseKey(t, "deploy")
	start, err := r.Start(g)
	require.NoError(t, err)
	assert.Equal(t, envValue, openStart(t, r, g, start)["PROD_KEY"])
	require.NoError(t, r.Complete(g, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, 0, runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED))

	state, holder = w.deployState(t, env)
	assert.Equal(t, "succeeded", state)
	assert.Nil(t, holder, "the lock is released when no job of the run needs the environment")
	runState, _ := w.env.RunState(run)
	assert.Equal(t, "succeeded", runState)

	res := w.owner.Get(apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/environments/production/deployments"))
	require.Equal(t, http.StatusOK, res.Status)
	var list []struct{ RunId, State, Ref string }
	require.NoError(t, json.Unmarshal(res.Data, &list))
	require.Len(t, list, 1)
	assert.Equal(t, []string{run.String(), "succeeded", "refs/heads/main"}, []string{list[0].RunId, list[0].State, list[0].Ref})
}

func TestABranchTheEnvironmentDoesNotAllowIsSkippedWithoutOpeningASecret(t *testing.T) {
	for _, c := range []struct{ event, ref string }{{triggers.EventPush, "refs/heads/feature/login"}, {triggers.EventTag, "refs/tags/main"}} {
		t.Run(c.ref, func(t *testing.T) {
			w := newWorld(t)
			w.production(t)
			w.defs.Put(shaA, pipeline(envPipeline))
			got := w.env.Deliver(w.trigger(c.event, c.ref, shaA))
			require.True(t, got.Created)
			run := got.RunID
			w.env.StartRuns()
			w.complete(t, "build")
			jobs := w.env.RunJobs(run)
			assert.Equal(t, []string{"skipped", "ENVIRONMENT_REF_NOT_ALLOWED"}, []string{jobs["deploy"].State, jobs["deploy"].Reason})
			var detail string
			require.NoError(t, w.env.Pool.QueryRow(context.Background(),
				`SELECT "FailureDetail"->>'Environment' FROM "RunJobs" WHERE "RunId" = $1 AND "Key" = 'deploy'`, run).Scan(&detail))
			assert.Equal(t, "production", detail)
			assert.False(t, w.secretUsed(t))
			assert.Empty(t, w.approvals(t, run), "no approval request exists, so there is nothing to bypass")
			assert.Equal(t, 0, w.env.AuditCount("deployment.approval_requested"))
			assert.Equal(t, 0, w.env.AuditCount("job.secrets_delivered"))
			state, _ := w.env.RunState(run)
			assert.Equal(t, "succeeded", state, "a skipped deploy does not fail the run")
		})
	}
}

func TestAPullRequestReachesTheEnvironmentOnlyWhenTheEnvironmentAllowsIt(t *testing.T) {
	for _, allow := range []bool{false, true} {
		w := newWorld(t)
		env := w.production(t)
		w.exec(t, `UPDATE "Environments" SET "AllowPullRequests" = $2 WHERE "Id" = $1`, env, allow)
		w.defs.Put(shaB, pipeline(envPipeline))
		tr := w.trigger("pull_request", "refs/pull/3/head", shaB)
		tr.BaseRef, tr.BaseSha, tr.PrNumber, tr.Origin = "refs/heads/main", shaA, 3, triggers.PullOriginSameRepository
		got := w.env.Deliver(tr)
		require.True(t, got.Created)
		w.env.StartRuns()
		w.complete(t, "build")
		deploy := w.env.RunJobs(got.RunID)["deploy"]
		if allow {
			assert.Equal(t, "active", deploy.State, "a PR into main may deploy to an environment that allows PRs on main")
			continue
		}
		assert.Equal(t, []string{"skipped", "ENVIRONMENT_REF_NOT_ALLOWED"}, []string{deploy.State, deploy.Reason})
	}
}

func TestPinnedImagesAndPluginRulesFailTheJob(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	w.exec(t, `UPDATE "Environments" SET "RequirePinnedImages" = true WHERE "Id" = $1`, env)
	w.images.pinned[taggedImage] = apptest.TestImage
	w.defs.Put(shaA, strings.ReplaceAll(envPipeline, "IMAGE", taggedImage))
	run := w.toDeploy(t, shaA)
	deploy := w.env.RunJobs(run)["deploy"]
	assert.Equal(t, []string{"failed", "ENVIRONMENT_REQUIRES_PINNED_IMAGES"}, []string{deploy.State, deploy.Reason})
	assert.False(t, w.secretUsed(t))
}

func TestAPausedEnvironmentHoldsTheJobUntilItResumes(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	w.exec(t, `UPDATE "Environments" SET "PausedAt" = now(), "PausedByUserId" = $2, "PausedReason" = 'freeze' WHERE "Id" = $1`, env, w.ownerID)
	w.defs.Put(shaA, pipeline(envPipeline))
	run := w.toDeploy(t, shaA)
	require.Equal(t, "waiting", w.env.RunJobs(run)["deploy"].State)
	gate, _ := w.gate(t, run, "deploy")
	assert.Equal(t, "paused", gate)
	res := w.owner.Get(w.runPath(run, ""))
	require.Equal(t, http.StatusOK, res.Status)
	assert.Contains(t, string(res.Data), `"Environment":"production"`)
	assert.Contains(t, string(res.Data), `"Gate":"paused"`)
	w.reopen(t)
	gate, _ = w.gate(t, run, "deploy")
	assert.Equal(t, "paused", gate, "the reconciler keeps it waiting")

	w.exec(t, `UPDATE "Environments" SET "PausedAt" = NULL, "PausedByUserId" = NULL, "PausedReason" = NULL WHERE "Id" = $1`, env)
	w.reopen(t)
	assert.Equal(t, "active", w.env.RunJobs(run)["deploy"].State)
	gate, _ = w.gate(t, run, "deploy")
	assert.Empty(t, gate)
}

func TestAnEnvironmentWithReviewersWaitsForApproval(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	w.exec(t, `INSERT INTO "EnvironmentReviewers" ("OrganizationId", "EnvironmentId", "UserId") VALUES ($1, $2, $3)`, w.org, env, w.ownerID)
	w.defs.Put(shaA, pipeline(envPipeline))
	run := w.toDeploy(t, shaA)
	w.reopen(t)
	assert.Equal(t, "waiting", w.env.RunJobs(run)["deploy"].State)
	gate, _ := w.gate(t, run, "deploy")
	assert.Equal(t, "approval", gate)
	assert.False(t, w.secretUsed(t))
}

func TestTheWaitTimerOpensTheGateWhenItsTimeComes(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	w.exec(t, `UPDATE "Environments" SET "WaitMinutes" = 10 WHERE "Id" = $1`, env)
	w.defs.Put(shaA, pipeline(envPipeline))
	before := w.env.Clock.Now()
	run := w.toDeploy(t, shaA)
	gate, opens := w.gate(t, run, "deploy")
	assert.Equal(t, "timer", gate)
	require.NotNil(t, opens)
	assert.WithinDuration(t, before.Add(10*time.Minute), *opens, time.Second)
	w.reopen(t)
	_, again := w.gate(t, run, "deploy")
	assert.True(t, opens.Equal(*again), "waiting again does not move the time")

	w.env.Clock.Advance(11 * time.Minute)
	w.reopen(t)
	assert.Equal(t, "active", w.env.RunJobs(run)["deploy"].State)
}

func TestOneRunHoldsTheEnvironmentAtATime(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	w.defs.Put(shaA, pipeline(envPipeline))
	w.defs.Put(shaB, pipeline(envPipeline))
	first := w.push("refs/heads/main", shaA)
	second := w.push("refs/heads/main", shaB)
	w.env.StartRuns()
	w.complete(t, "build")
	w.complete(t, "build")
	require.Equal(t, "active", w.env.RunJobs(first)["deploy"].State)
	assert.Equal(t, "waiting", w.env.RunJobs(second)["deploy"].State)
	gate, _ := w.gate(t, second, "deploy")
	assert.Equal(t, "environment_busy", gate)

	w.complete(t, "deploy")
	_, holder := w.deployState(t, env)
	assert.Nil(t, holder)
	w.reopen(t)
	assert.Equal(t, "active", w.env.RunJobs(second)["deploy"].State, "the next run takes the free environment")
	_, holder = w.deployState(t, env)
	require.NotNil(t, holder)
	assert.Equal(t, second, *holder)
}

func TestAStaleLockIsReleasedByTheReconciler(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	gone := uuid.New()
	w.exec(t, `UPDATE "Environments" SET "LockedByRunId" = $2, "LockedAt" = now() WHERE "Id" = $1`, env, gone)
	w.reopen(t)
	_, holder := w.deployState(t, env)
	assert.Nil(t, holder)
}

func TestARuleChangeAfterTheGateGivesTheJobNoSecret(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	w.defs.Put(shaA, pipeline(envPipeline))
	run := w.toDeploy(t, shaA)
	require.Equal(t, "active", w.env.RunJobs(run)["deploy"].State)
	w.exec(t, `UPDATE "Environments" SET "RuleVersion" = "RuleVersion" + 1 WHERE "Id" = $1`, env)

	r, g := w.leaseKey(t, "deploy")
	_, err := r.Start(g)
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.False(t, w.secretUsed(t), "no environment secret was opened")
	w.reopen(t)
	deploy := w.env.RunJobs(run)["deploy"]
	assert.Equal(t, "ENVIRONMENT_RULE_CHANGED", deploy.Reason)
}

func TestAPauseAfterTheGateStopsAJobThatAsksForNoSecret(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	w.defs.Put(shaA, pipeline(plainDeployPipeline))
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	require.Equal(t, "active", w.env.RunJobs(run)["deploy"].State)
	w.exec(t, `UPDATE "Environments" SET "PausedAt" = now(), "PausedByUserId" = $2, "PausedReason" = 'freeze', "RuleVersion" = "RuleVersion" + 1 WHERE "Id" = $1`, env, w.ownerID)

	r, g := w.leaseKey(t, "deploy")
	_, err := r.Start(g)
	require.Error(t, err, "Start checks the environment even when the job asks for no secret")
	w.reopen(t)
	assert.Equal(t, "ENVIRONMENT_RULE_CHANGED", w.env.RunJobs(run)["deploy"].Reason)
}
