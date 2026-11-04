package e2e_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	"github.com/Khaymah/dhow/internal/runnersig"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	deployValue = "a deploy token value that must stay sealed"

	secretPipeline = `version: 1
on:
  push: {}
  pull_request: {}
jobs:
  deploy:
    image: IMAGE
    steps:
      - run: ./deploy.sh
        env:
          TOKEN: ${{ secrets.DEPLOY_TOKEN }}
  missing:
    image: IMAGE
    steps:
      - run: ./deploy.sh
        env:
          TOKEN: ${{ secrets.NOT_THERE }}
`
)

func (w world) leaseKey(t *testing.T, key string) (*apptest.TestRunner, *apptest.Grant) {
	t.Helper()
	for _, r := range w.runners {
		g, err := r.Lease(context.Background(), 1, 0)
		require.NoError(t, err)
		if g == nil {
			continue
		}
		if w.env.RunJobKey(uuid.MustParse(g.Grant.GetJobId())) == key {
			return r, g
		}
	}
	t.Fatalf("no runner got the job %q", key)
	return nil, nil
}

func openStart(t *testing.T, r *apptest.TestRunner, g *apptest.Grant, start *runnerv1.StartResponse) map[string]string {
	t.Helper()
	sealed := start.GetSecrets()
	require.NotNil(t, sealed)
	aad := runnersig.SecretsAAD(g.Ref.GetLeaseId(), g.Ref.GetFencingToken(), g.Grant.GetAttemptId(), r.RunnerID)
	plain, err := runnersig.OpenSecrets(r.Identity.Encryption, sealed.GetEnc(), aad, sealed.GetCiphertext())
	require.NoError(t, err)
	var b runnerv1.SecretBundle
	require.NoError(t, proto.Unmarshal(plain, &b))
	out := map[string]string{}
	for _, s := range b.GetSecrets() {
		out[s.GetName()] = string(s.GetValue())
	}
	return out
}

func failureNames(t *testing.T, w world, run, key string) string {
	t.Helper()
	var detail string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT "FailureDetail"->>'Names' FROM "RunJobs" WHERE "RunId" = $1 AND "Key" = $2`, run, key).Scan(&detail))
	return detail
}

func TestProjectSecretsReachTheJobThatNamesThem(t *testing.T) {
	w := newWorld(t)
	w.env.SeedSecret(apptest.SecretRef(w.org, &w.project, ""), "DEPLOY_TOKEN", deployValue)
	w.defs.Put(shaA, pipeline(secretPipeline))
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()

	jobs := w.env.RunJobs(run)
	assert.Equal(t, []string{"failed", "SECRET_NOT_FOUND"}, []string{jobs["missing"].State, jobs["missing"].Reason})
	assert.Equal(t, `["NOT_THERE"]`, failureNames(t, w, run.String(), "missing"), "the detail holds names only")
	require.Equal(t, "active", jobs["deploy"].State)

	r, g := w.leaseKey(t, "deploy")
	start, err := r.Start(g)
	require.NoError(t, err)
	assert.Equal(t, deployValue, openStart(t, r, g, start)["DEPLOY_TOKEN"])
	assert.Equal(t, 1, w.env.AuditCount("job.secrets_delivered"))
	var data string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT "Data"::text FROM "AuditEvents" WHERE "Action" = 'job.secrets_delivered'`).Scan(&data))
	assert.Contains(t, data, "DEPLOY_TOKEN:project:v1")
	assert.NotContains(t, data, deployValue)
	assert.NotContains(t, w.env.Logs.String(), deployValue)
}

func TestAPullRequestOnlyGetsSecretsMarkedForIt(t *testing.T) {
	w := newWorld(t)
	w.env.SeedSecret(apptest.SecretRef(w.org, &w.project, ""), "DEPLOY_TOKEN", deployValue)
	w.defs.Put(shaB, pipeline(secretPipeline))
	tr := w.trigger("pull_request", "refs/pull/3/head", shaB)
	tr.BaseRef, tr.BaseSha, tr.PrNumber, tr.Origin = "refs/heads/main", shaA, 3, triggers.PullOriginSameRepository
	got := w.env.Deliver(tr)
	require.True(t, got.Created)
	w.env.StartRuns()
	jobs := w.env.RunJobs(got.RunID)
	assert.Equal(t, []string{"failed", "SECRET_NOT_AVAILABLE_TO_PULL_REQUESTS"}, []string{jobs["deploy"].State, jobs["deploy"].Reason})
	assert.Equal(t, `["DEPLOY_TOKEN"]`, failureNames(t, w, got.RunID.String(), "deploy"))
}

func TestStartRechecksTheRunBeforeOpeningAnySecret(t *testing.T) {
	w := newWorld(t)
	w.env.SeedSecret(apptest.SecretRef(w.org, &w.project, ""), "DEPLOY_TOKEN", deployValue)
	w.defs.Put(shaA, pipeline(secretPipeline))
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	require.Equal(t, "active", w.env.RunJobs(run)["deploy"].State)
	_, err := w.env.Pool.Exec(context.Background(),
		`UPDATE "PipelineRuns" SET "IsFork" = true, "Trigger" = 'pull_request', "PrNumber" = 1, "BaseSha" = $2 WHERE "Id" = $1`, run, shaA)
	require.NoError(t, err)

	r, g := w.leaseKey(t, "deploy")
	_, err = r.Start(g)
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "SECRETS_UNAVAILABLE", ce.Message(), "a run that turned into a fork gets nothing")
	assert.Zero(t, w.env.AuditCount("job.secrets_delivered"))
	var used *string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "LastUsedAt"::text FROM "Secrets" WHERE "Name" = 'DEPLOY_TOKEN'`).Scan(&used))
	assert.Nil(t, used, "the secret was never opened")
}

const varsPipeline = `version: 1
on:
  push: {}
jobs:
  build:
    image: IMAGE
    if: ${{ vars.REGION == "us" }}
    env:
      REGION: ${{ vars.REGION }}
      BUCKET: ${{ vars.BUCKET }}
    steps:
      - run: ./build.sh
`

func TestVariablesFillTheContextWithTheClosestScope(t *testing.T) {
	w := newWorld(t)
	w.env.SeedVariable(apptest.SecretRef(w.org, nil, ""), "REGION", "eu")
	w.env.SeedVariable(apptest.SecretRef(w.org, nil, ""), "BUCKET", "artifacts-eu")
	w.env.SeedVariable(apptest.SecretRef(w.org, &w.project, ""), "region", "us")
	w.defs.Put(shaA, pipeline(varsPipeline))
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	state, reason := w.env.RunState(run)
	require.Equal(t, "active", w.env.RunJobs(run)["build"].State, "the project value wins in if: (run %s %s, job %s)", state, reason, w.env.RunJobs(run)["build"].Reason)
	_, g := w.leaseKey(t, "build")
	env := g.Grant.GetSpec().GetEnv()
	assert.Equal(t, "us", env["REGION"])
	assert.Equal(t, "artifacts-eu", env["BUCKET"])
}

const registryPipeline = `version: 1
on:
  push: {}
jobs:
  build:
    image: IMAGE
    steps:
      - run: ./build.sh
`

func TestARegistryLoginReachesTheRunnerAtStart(t *testing.T) {
	w := newWorld(t)
	w.env.SeedRegistryCredential(apptest.SecretRef(w.org, &w.project, ""), "registry.example.com", "robot", deployValue)
	r := w.env.SeedConnectedRunner(w.org, w.ownerID, apptest.SeedRunnerSpec{Capacity: 4, Features: []string{"checkout", "plugins", "registry_auth"}})
	w.defs.Put(shaA, pipeline(registryPipeline))
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	require.Equal(t, "active", w.env.RunJobs(run)["build"].State)

	g, err := r.Lease(context.Background(), 1, 0)
	require.NoError(t, err)
	require.NotNil(t, g, "only a runner with registry_auth gets the job")
	auth := g.Grant.GetSpec().GetRegistryAuth()
	require.Len(t, auth, 1)
	assert.Equal(t, "registry.example.com", auth[0].GetHost())
	assert.Equal(t, "dhow.registry.0", auth[0].GetSecretName())
	start, err := r.Start(g)
	require.NoError(t, err)
	values := openStart(t, r, g, start)
	assert.JSONEq(t, `{"Username":"robot","Password":"`+deployValue+`"}`, values["dhow.registry.0"])
	var data string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT "Data"::text FROM "AuditEvents" WHERE "Action" = 'job.secrets_delivered'`).Scan(&data))
	assert.Contains(t, data, "registry.example.com")
	assert.NotContains(t, data, deployValue)
}

func TestAForkNeverGetsARegistryLogin(t *testing.T) {
	w := newWorld(t)
	w.env.SeedRegistryCredential(apptest.SecretRef(w.org, &w.project, ""), "registry.example.com", "robot", deployValue)
	w.defs.Put(shaA, pipeline(`version: 1
on:
  pull_request: {}
jobs:
  build:
    image: IMAGE
    steps:
      - run: ./build.sh
`))
	run := w.forkPR()
	maintainer, _ := w.env.SignedIn(w.org, "maintainer@example.com", "maintainer")
	w.env.StartRuns()
	require.Equal(t, 200, w.post(t, maintainer, run, "approve").Status)
	w.env.StartRuns()
	require.Equal(t, "active", w.env.RunJobs(run)["build"].State)
	_, g := w.leaseKey(t, "build")
	assert.Empty(t, g.Grant.GetSpec().GetRegistryAuth())
}

func TestAPullRequestNeverGetsARegistryLogin(t *testing.T) {
	w := newWorld(t)
	w.env.SeedRegistryCredential(apptest.SecretRef(w.org, &w.project, ""), "registry.example.com", "robot", deployValue)
	r := w.env.SeedConnectedRunner(w.org, w.ownerID, apptest.SeedRunnerSpec{Capacity: 4, Features: []string{"checkout", "plugins", "registry_auth"}})
	w.defs.Put(shaB, pipeline(`version: 1
on:
  pull_request: {}
jobs:
  build:
    image: IMAGE
    steps:
      - run: ./build.sh
`))
	tr := w.trigger("pull_request", "refs/pull/4/head", shaB)
	tr.BaseRef, tr.BaseSha, tr.PrNumber, tr.Origin = "refs/heads/main", shaA, 4, triggers.PullOriginSameRepository
	got := w.env.Deliver(tr)
	require.True(t, got.Created)
	w.env.StartRuns()
	require.Equal(t, "active", w.env.RunJobs(got.RunID)["build"].State)
	g, err := r.Lease(context.Background(), 1, 0)
	require.NoError(t, err)
	if g == nil {
		_, g = w.leaseKey(t, "build")
	}
	assert.Empty(t, g.Grant.GetSpec().GetRegistryAuth(), "a pull request run never gets a registry login")
}
