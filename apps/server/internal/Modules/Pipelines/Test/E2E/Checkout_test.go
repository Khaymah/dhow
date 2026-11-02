package e2e_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	"github.com/Khaymah/dhow/internal/runnersig"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const outputsPipeline = `version: 1
on:
  push: {}
  pull_request: {}
jobs:
  build:
    image: IMAGE
    steps:
      - run: make
  ship:
    needs: [build]
    image: IMAGE
    env:
      VERSION: ${{ needs.build.outputs.version }}
    steps:
      - run: make ship
`

type gitWorld struct {
	env     *apptest.Env
	org     uuid.UUID
	project uuid.UUID
	repo    apptest.SSHRepository
	plain   *apptest.TestRunner
	runner  *apptest.TestRunner
}

func newGitWorld(t *testing.T) gitWorld {
	t.Helper()
	env := apptest.NewWithOptions(t, apptest.Options{CheckoutImage: apptest.TestImage})
	owner, d := env.SetupAndLogin()
	org, ownerID := env.OrganizationID(d.OrgSlug), uuid.MustParse(owner.UserID)
	project := env.SeedProject(org, ownerID)
	return gitWorld{
		env: env, org: org, project: project, repo: env.SeedSSHRepository(org, project, ownerID),
		plain:  env.SeedConnectedRunner(org, ownerID, apptest.SeedRunnerSpec{}),
		runner: env.SeedConnectedRunner(org, ownerID, apptest.SeedRunnerSpec{Capacity: 8, Features: checkoutFeatures()}),
	}
}

func (g gitWorld) trigger(event, ref, sha string) triggers.Trigger {
	return triggers.Trigger{
		OrganizationID: g.org, ProjectID: g.project, RepositoryID: g.repo.ID, DeliveryID: uuid.New(), Provider: "generic",
		Event: event, Ref: ref, Sha: sha,
	}
}

func (g gitWorld) spec(t *testing.T, job uuid.UUID) (map[string]any, []string) {
	t.Helper()
	var raw []byte
	var features []string
	require.NoError(t, g.env.Pool.QueryRow(context.Background(), `SELECT "Spec", "RequiredFeatures" FROM "Jobs" WHERE "Id" = $1`, job).Scan(&raw, &features))
	var s map[string]any
	require.NoError(t, json.Unmarshal(raw, &s))
	return s, features
}

func (g gitWorld) assertCheckout(t *testing.T, job uuid.UUID, ref, sha string) {
	t.Helper()
	s, features := g.spec(t, job)
	co, ok := s["checkout"].(map[string]any)
	require.True(t, ok, "%v", s)
	assert.Equal(t, g.repo.CloneURL, co["repository_url"])
	assert.Equal(t, ref, co["ref"])
	assert.Equal(t, sha, co["sha"])
	assert.Equal(t, "CHECKOUT_TRANSPORT_SSH", co["transport"])
	assert.Equal(t, g.repo.KnownHost, co["known_host"], "the pinned host key line")
	assert.EqualValues(t, 1, co["depth"])
	assert.Equal(t, "dhow.git.checkout", co["credential_secret"])
	assert.Contains(t, s["secrets"], map[string]any{"name": "dhow.git.checkout", "as_file": "git-credential"})
	assert.Contains(t, features, "checkout")
}

func (g gitWorld) assertNoKey(t *testing.T) {
	t.Helper()
	key := string(g.repo.PrivateKey)
	var body []string
	for _, line := range strings.Split(key, "\n") {
		if len(line) > 20 && !strings.HasPrefix(line, "-----") {
			body = append(body, line)
		}
	}
	require.NotEmpty(t, body)
	for _, form := range append(body, key) {
		for _, query := range []string{
			`SELECT count(*) FROM "Jobs" t WHERE position($1 in t::text) > 0`,
			`SELECT count(*) FROM "LogChunks" t WHERE position($1 in t::text) > 0`,
			`SELECT count(*) FROM "AuditEvents" t WHERE position($1 in t::text) > 0`,
			`SELECT count(*) FROM "PipelineRuns" t WHERE position($1 in t::text) > 0`,
			`SELECT count(*) FROM "RunJobs" t WHERE position($1 in t::text) > 0`,
		} {
			assert.Zero(t, g.env.CountRows(query, form), "the deploy key appears: %s", query)
		}
		assert.NotContains(t, g.env.Logs.String(), form, "the deploy key appears in the server log")
	}
}

func TestAGenericRunReadsItsFileOnARunnerAndChecksEveryJobOut(t *testing.T) {
	g := newGitWorld(t)
	run := g.env.Deliver(g.trigger(triggers.EventPush, "refs/heads/main", shaA)).RunID
	g.env.StartRuns()
	discovery := g.env.RunJobs(run)["dhow.discovery"]
	require.Equal(t, "active", discovery.State)
	g.assertCheckout(t, *discovery.Job, "refs/heads/main", shaA)
	grant, err := g.plain.Lease(context.Background(), 1, 0)
	require.NoError(t, err)
	assert.Nil(t, grant, "a runner without the checkout feature never takes a checkout job")

	_, ok := g.runner.Discover(pipeline(outputsPipeline))
	require.True(t, ok)
	g.env.StartRuns()
	state, _ := g.env.RunState(run)
	require.Equal(t, "running", state)
	build := g.env.RunJobs(run)["build"]
	require.Equal(t, "active", build.State)
	g.assertCheckout(t, *build.Job, "refs/heads/main", shaA)

	lease := g.runner.MustLease(0)
	start, err := g.runner.Start(lease)
	require.NoError(t, err)
	sealed := start.GetSecrets()
	require.NotNil(t, sealed, "the checkout key reaches the runner sealed to it at Start")
	aad := runnersig.SecretsAAD(lease.Grant.GetLeaseId(), lease.Grant.GetFencingToken(), lease.Grant.GetAttemptId(), g.runner.RunnerID)
	plain, err := runnersig.OpenSecrets(g.runner.Identity.Encryption, sealed.GetEnc(), aad, sealed.GetCiphertext())
	require.NoError(t, err)
	var bundle runnerv1.SecretBundle
	require.NoError(t, proto.Unmarshal(plain, &bundle))
	require.Len(t, bundle.GetSecrets(), 1)
	assert.Equal(t, "dhow.git.checkout", bundle.GetSecrets()[0].GetName())
	assert.Equal(t, g.repo.PrivateKey, bundle.GetSecrets()[0].GetValue(), "the deploy key, unsealed only for this attempt")
	require.NoError(t, g.runner.CompleteOutputs(lease, map[string]string{"version": "1.2.3"}))
	ship := g.env.RunJobs(run)["ship"]
	require.Equal(t, "active", ship.State)
	s, _ := g.spec(t, *ship.Job)
	assert.Equal(t, map[string]any{"VERSION": "1.2.3"}, s["env"], "an output reaches the job that needs it")
	key, done := g.runner.FinishNext(succeeded)
	require.True(t, done)
	assert.Equal(t, "ship", key)
	state, _ = g.env.RunState(run)
	assert.Equal(t, "succeeded", state)
	assert.Zero(t, g.env.CountRows(`SELECT count(*) FROM "CommitStatusReports"`), "a generic repository gets no commit status (decision 14)")
	g.assertNoKey(t)
}

func TestAForkRunReadsTheFileOfTheBase(t *testing.T) {
	g := newGitWorld(t)
	tr := g.trigger(triggers.EventPullRequest, "refs/pull/4/head", shaB)
	tr.BaseRef, tr.BaseSha, tr.PrNumber, tr.Origin = "refs/heads/main", shaA, 4, triggers.PullOriginFork
	run := g.env.Deliver(tr).RunID
	_, err := g.env.Pool.Exec(context.Background(), `UPDATE "PipelineRuns" SET "State" = 'pending' WHERE "Id" = $1`, run)
	require.NoError(t, err)
	g.env.StartRuns()
	discovery := g.env.RunJobs(run)["dhow.discovery"]
	require.Equal(t, "active", discovery.State)
	g.assertCheckout(t, *discovery.Job, "refs/heads/main", shaA)
}

func TestADiscoveryThatCannotFindTheCommitOnTheRefStopsTheRun(t *testing.T) {
	g := newGitWorld(t)
	run := g.env.Deliver(g.trigger(triggers.EventPush, "refs/heads/main", shaA)).RunID
	g.env.StartRuns()
	lease := g.runner.MustLease(0)
	_, err := g.runner.Start(lease)
	require.NoError(t, err)
	require.NoError(t, g.runner.Complete(lease, runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED, 0,
		runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_REF_MOVED))
	state, reason := g.env.RunState(run)
	assert.Equal(t, []string{"failed", "SHA_NOT_ON_REF"}, []string{state, reason})
	assert.Equal(t, 1, g.env.CountRows(`SELECT count(*) FROM "Jobs"`), "nothing runs after the discovery job")
}

func TestADiscoveryOutputThatDoesNotMatchItsDigestIsRefused(t *testing.T) {
	g := newGitWorld(t)
	run := g.env.Deliver(g.trigger(triggers.EventPush, "refs/heads/main", shaA)).RunID
	g.env.StartRuns()
	lease := g.runner.MustLease(0)
	_, err := g.runner.Start(lease)
	require.NoError(t, err)
	step := int32(0)
	g.runner.MustAppend(lease.Ref, &runnerv1.LogChunk{
		Seq: 1, Stream: runnerv1.LogStream_LOG_STREAM_STDOUT, StepIndex: &step, Data: []byte(apptest.DiscoveryLog("version: 1\n")),
	})
	require.NoError(t, g.runner.CompleteOutputs(lease, apptest.DiscoveryOutputs("something else")))
	state, reason := g.env.RunState(run)
	assert.Equal(t, []string{"failed", "DISCOVERY_FAILED"}, []string{state, reason})

	missing := g.env.Deliver(g.trigger(triggers.EventPush, "refs/heads/main", shaB)).RunID
	g.env.StartRuns()
	lease = g.runner.MustLease(0)
	_, err = g.runner.Start(lease)
	require.NoError(t, err)
	require.NoError(t, g.runner.CompleteOutputs(lease, map[string]string{"dhow_found": "false"}))
	state, reason = g.env.RunState(missing)
	assert.Equal(t, []string{"skipped", "DEFINITION_NOT_FOUND"}, []string{state, reason})
}
