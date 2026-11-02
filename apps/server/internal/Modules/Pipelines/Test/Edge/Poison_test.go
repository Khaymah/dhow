package edge_test

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

type tenant struct {
	org, owner, project, repo uuid.UUID
}

func newTenant(t *testing.T, env *apptest.Env, slug string) tenant {
	t.Helper()
	org := env.SeedOrganization("Org "+slug, slug)
	owner := env.SeedUser(org, slug+"@example.com", "a long password for "+slug, "owner")
	project := env.SeedProject(org, owner)
	return tenant{org: org, owner: owner, project: project, repo: env.SeedRepository(org, project, owner)}
}

func seedQueued(t *testing.T, env *apptest.Env, tn tenant, number int) uuid.UUID {
	t.Helper()
	source := strings.ReplaceAll(simple(), "IMAGE", apptest.TestImage)
	digest := sha256.Sum256([]byte(source))
	ctx := context.Background()
	var id uuid.UUID
	require.NoError(t, env.Pool.QueryRow(ctx,
		`INSERT INTO "PipelineRuns" ("OrganizationId", "ProjectId", "RepositoryId", "Provider", "Number", "Trigger", "Ref", "Sha",
		 "DefinitionPath", "DefinitionSha", "DefinitionSource", "DefinitionDigest", "ConcurrencyGroup")
		 VALUES ($1, $2, $3, 'generic', $4, 'push', 'refs/heads/main', $5, '.dhow.yml', $5, $6, $7, 'deploy') RETURNING "Id"`,
		tn.org, tn.project, tn.repo, number, sha, source, digest[:]).Scan(&id))
	_, err := env.Pool.Exec(ctx, `UPDATE "PipelineRuns" SET "State" = 'queued' WHERE "Id" = $1`, id)
	require.NoError(t, err)
	return id
}

func TestOnceAPoisonRunNeverStopsTheRunsOfOtherOrganizations(t *testing.T) {
	env := apptest.New(t)
	poisonTenant, healthyTenant := newTenant(t, env, "poison"), newTenant(t, env, "healthy")
	poison := seedQueued(t, env, poisonTenant, 1)
	_, err := env.Pool.Exec(context.Background(), `INSERT INTO "RunJobs" ("OrganizationId", "RunId", "Key", "JobName", "JobId", "Position")
		VALUES ($1, $2, 'build', 'build', 'build', 0)`, poisonTenant.org, poison)
	require.NoError(t, err, "a row that makes the expansion break a constraint")
	healthy := seedQueued(t, env, healthyTenant, 2)

	res, err := env.Wired.Pipelines.Reconcile(context.Background())
	require.NoError(t, err, "one bad run never aborts the pass")
	assert.Equal(t, 1, res.Failed)
	assert.Equal(t, 1, res.Unqueued)
	state, reason := env.RunState(poison)
	assert.Equal(t, []string{"failed", "RUN_EXPANSION_FAILED"}, []string{state, reason})
	state, _ = env.RunState(healthy)
	assert.Equal(t, "running", state, "the healthy run of the other organization still starts")
	assert.Equal(t, "active", env.RunJobs(healthy)["build"].State)
}
