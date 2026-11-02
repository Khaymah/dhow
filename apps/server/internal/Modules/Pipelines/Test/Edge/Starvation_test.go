package edge_test

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	"github.com/Khaymah/dhow/internal/pipelinespec"
)

const blocked = 101

func source() (string, []byte) {
	src := strings.ReplaceAll(simple(), "IMAGE", apptest.TestImage)
	sum := sha256.Sum256([]byte(src))
	return src, sum[:]
}

func seedRuns(t *testing.T, env *apptest.Env, tn tenant, state string, from, n int) {
	t.Helper()
	src, digest := source()
	_, err := env.Pool.Exec(context.Background(),
		`INSERT INTO "PipelineRuns" ("OrganizationId", "ProjectId", "RepositoryId", "Provider", "Number", "Trigger", "Ref", "Sha",
		 "DefinitionPath", "DefinitionSha", "DefinitionSource", "DefinitionDigest", "ConcurrencyGroup")
		 SELECT $1, $2, $3, 'generic', g, 'manual', 'refs/heads/main', $4, '.dhow.yml', $4, $5, $6, 'deploy'
		 FROM generate_series($7::int, $7::int + $8::int - 1) g`,
		tn.org, tn.project, tn.repo, sha, src, digest, from, n)
	require.NoError(t, err)
	if state == "pending" {
		return
	}
	_, err = env.Pool.Exec(context.Background(), `UPDATE "PipelineRuns" SET "State" = $2 WHERE "ProjectId" = $1 AND "State" = 'pending'`,
		tn.project, state)
	require.NoError(t, err)
}

func TestOnceBlockedQueuedRunsNeverStarveAnotherGroup(t *testing.T) {
	env := apptest.New(t)
	busy, free := newTenant(t, env, "busy"), newTenant(t, env, "free")
	seedRuns(t, env, busy, "running", 1, 1)
	seedRuns(t, env, busy, "queued", 2, blocked)
	seedRuns(t, env, free, "queued", 500, 1)
	started, err := env.Wired.Pipelines.StartQueued(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, started)
	var state string
	require.NoError(t, env.Pool.QueryRow(context.Background(), `SELECT "State" FROM "PipelineRuns" WHERE "ProjectId" = $1`, free.project).Scan(&state))
	assert.Equal(t, "running", state, "only the head of each group is a candidate, so a long queue elsewhere never fills the batch")
}

func waitingRunJob(t *testing.T, env *apptest.Env, tn tenant) {
	t.Helper()
	src, _ := source()
	spec, issues := pipelinespec.Check([]byte(src))
	require.False(t, pipelinespec.HasErrors(issues))
	plan, err := pipelinespec.Compile(spec)
	require.NoError(t, err)
	_, err = env.Pool.Exec(context.Background(),
		`INSERT INTO "RunJobs" ("OrganizationId", "RunId", "Key", "JobName", "JobId", "Position", "Digest")
		 SELECT r."OrganizationId", r."Id", 'build', 'build', 'build', 0, $2 FROM "PipelineRuns" r WHERE r."ProjectId" = $1`,
		tn.project, plan.Jobs[0].Digest)
	require.NoError(t, err)
}

func fillQueue(t *testing.T, env *apptest.Env, tn tenant) {
	t.Helper()
	job := env.SeedJob(tn.org, tn.owner)
	_, err := env.Pool.Exec(context.Background(),
		`INSERT INTO "Jobs" ("OrganizationId", "Name", "Spec", "SpecDigest", "Image", "Isolation", "QueueDeadline", "CreatedByUserId")
		 SELECT "OrganizationId", "Name", "Spec", "SpecDigest", "Image", "Isolation", "QueueDeadline", "CreatedByUserId"
		 FROM "Jobs", generate_series(1, 999) WHERE "Id" = $1`, job)
	require.NoError(t, err)
}

func TestOnceRunsThatCannotMoveNeverStarveOneThatCan(t *testing.T) {
	env := apptest.New(t)
	stuck, free := newTenant(t, env, "stuck"), newTenant(t, env, "free")
	fillQueue(t, env, stuck)
	seedRuns(t, env, stuck, "running", 1, blocked)
	waitingRunJob(t, env, stuck)
	_, err := env.Pool.Exec(context.Background(), `UPDATE "PipelineRuns" SET "UpdatedAt" = now() - interval '1 hour' WHERE "ProjectId" = $1`, stuck.project)
	require.NoError(t, err)
	seedRuns(t, env, free, "running", 1, 1)
	waitingRunJob(t, env, free)
	env.Clock.Advance(time.Minute)
	for range 2 {
		_, err := env.Wired.Pipelines.Reconcile(context.Background())
		require.NoError(t, err)
	}
	var state string
	require.NoError(t, env.Pool.QueryRow(context.Background(),
		`SELECT rj."State" FROM "RunJobs" rj JOIN "PipelineRuns" r ON r."Id" = rj."RunId" WHERE r."ProjectId" = $1`, free.project).Scan(&state))
	assert.Equal(t, "active", state, "a run that tried and could not move goes to the back of the line")
}
