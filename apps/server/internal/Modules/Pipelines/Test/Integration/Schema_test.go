package integration_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

const (
	checkViolation  = "23514"
	uniqueViolation = "23505"
	sha             = "5555555555555555555555555555555555555555"
)

func TestThePipelinesSchemaEnforcesItsStateMachines(t *testing.T) {
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	project := env.SeedProject(org, uuid.MustParse(owner.UserID))
	ctx := context.Background()
	running := env.SeedRun(project, "pending")
	_, err := env.Pool.Exec(ctx, `UPDATE "PipelineRuns" SET "State" = 'running' WHERE "Id" = $1`, running)
	require.NoError(t, err)
	done := env.SeedRun(project, "pending")
	for _, step := range []string{
		`UPDATE "PipelineRuns" SET "State" = 'running' WHERE "Id" = $1`,
		`UPDATE "PipelineRuns" SET "State" = 'succeeded', "FinishedAt" = now() WHERE "Id" = $1`,
	} {
		_, err = env.Pool.Exec(ctx, step, done)
		require.NoError(t, err)
	}
	var runJob uuid.UUID
	require.NoError(t, env.Pool.QueryRow(ctx, `INSERT INTO "RunJobs" ("OrganizationId", "RunId", "Key", "JobName", "JobId", "Position")
		VALUES ($1, $2, 'a', 'a', 'a', 0) RETURNING "Id"`, org, running).Scan(&runJob))
	_, err = env.Pool.Exec(ctx, `UPDATE "RunJobs" SET "State" = 'active', "CurrentJobId" = uuidv7(), "StartedAt" = now() WHERE "Id" = $1`, runJob)
	require.NoError(t, err)
	_, err = env.Pool.Exec(ctx, `UPDATE "RunJobs" SET "State" = 'succeeded', "FinishedAt" = now() WHERE "Id" = $1`, runJob)
	require.NoError(t, err)
	tests := []struct {
		name string
		sql  string
		args []any
		code string
	}{
		{"a run that starts running", `INSERT INTO "PipelineRuns" ("OrganizationId", "ProjectId", "Number", "Trigger", "Ref", "DefinitionPath", "State")
			VALUES ($1, $2, 999, 'manual', 'refs/heads/main', '.dhow.yml', 'running')`, []any{org, project}, checkViolation},
		{"a running run back to pending", `UPDATE "PipelineRuns" SET "State" = 'pending' WHERE "Id" = $1`, []any{running}, checkViolation},
		{"a succeeded run running again", `UPDATE "PipelineRuns" SET "State" = 'running', "FinishedAt" = NULL WHERE "Id" = $1`, []any{done}, checkViolation},
		{"a finished run without a time", `UPDATE "PipelineRuns" SET "State" = 'failed' WHERE "Id" = $1`, []any{running}, checkViolation},
		{
			"a run number used twice", `UPDATE "PipelineRuns" SET "Number" = (SELECT "Number" FROM "PipelineRuns" WHERE "Id" = $2) WHERE "Id" = $1`,
			[]any{running, done},
			uniqueViolation,
		},
		{"a webhook run without a sha", `INSERT INTO "PipelineRuns" ("OrganizationId", "ProjectId", "Number", "Trigger", "Ref", "DefinitionPath")
			VALUES ($1, $2, 998, 'push', 'refs/heads/main', '.dhow.yml')`, []any{org, project}, checkViolation},
		{"a fork run that is not a pull request", `INSERT INTO "PipelineRuns" ("OrganizationId", "ProjectId", "Number", "Trigger", "Ref",
			"DefinitionPath", "IsFork") VALUES ($1, $2, 997, 'manual', 'refs/heads/main', '.dhow.yml', true)`, []any{org, project}, checkViolation},
		{"a bad sha", `UPDATE "PipelineRuns" SET "Sha" = 'HEAD' WHERE "Id" = $1`, []any{running}, checkViolation},
		{"a succeeded run job that fails later", `UPDATE "RunJobs" SET "State" = 'failed' WHERE "Id" = $1`, []any{runJob}, checkViolation},
		{"an active run job without a job", `INSERT INTO "RunJobs" ("OrganizationId", "RunId", "Key", "JobName", "JobId", "Position", "State")
			VALUES ($1, $2, 'b', 'b', 'b', 1, 'active')`, []any{org, running}, checkViolation},
		{"the same key twice in a run", `INSERT INTO "RunJobs" ("OrganizationId", "RunId", "Key", "JobName", "JobId", "Position")
			VALUES ($1, $2, 'a', 'a', 'a', 1)`, []any{org, running}, uniqueViolation},
		{"a run job of another organization's run", `INSERT INTO "RunJobs" ("OrganizationId", "RunId", "Key", "JobName", "JobId", "Position")
			VALUES ($1, $2, 'c', 'c', 'c', 2)`, []any{env.SeedOrganization("Other", "other"), running}, "23503"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := env.Pool.Exec(ctx, tt.sql, tt.args...)
			var pgErr *pgconn.PgError
			require.ErrorAs(t, err, &pgErr)
			assert.Equal(t, tt.code, pgErr.Code, err.Error())
		})
	}
}

func TestTheSameCommitOnTheSameRefIsOneRowEvenWithoutTheService(t *testing.T) {
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org, ownerID := env.OrganizationID(d.OrgSlug), uuid.MustParse(owner.UserID)
	project := env.SeedProject(org, ownerID)
	repo := env.SeedRepository(org, project, ownerID)
	insert := `INSERT INTO "PipelineRuns" ("OrganizationId", "ProjectId", "RepositoryId", "Number", "Trigger", "Ref", "Sha", "DefinitionPath")
		VALUES ($1, $2, $3, $4, 'push', 'refs/heads/main', $5, '.dhow.yml')`
	_, err := env.Pool.Exec(context.Background(), insert, org, project, repo, 1, sha)
	require.NoError(t, err)
	_, err = env.Pool.Exec(context.Background(), insert, org, project, repo, 2, sha)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	assert.Equal(t, uniqueViolation, pgErr.Code)
}

func TestThePipelinesMigrationRollsBackCompletely(t *testing.T) {
	pool := dbtest.NewEmptyPool(t)
	ctx := context.Background()
	m, err := migrations.NewMigrator(pool, migrations.Binary{})
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()
	_, err = m.Up(ctx)
	require.NoError(t, err)
	tables := func() int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname = current_schema()
			AND tablename IN ('PipelineRuns', 'PipelineRunCounters', 'RunJobs')`).Scan(&n))
		return n
	}
	nullable := func() string {
		var v string
		require.NoError(t, pool.QueryRow(ctx, `SELECT is_nullable FROM information_schema.columns WHERE table_schema = current_schema()
			AND table_name = 'Jobs' AND column_name = 'CreatedByUserId'`).Scan(&v))
		return v
	}
	assert.Equal(t, 3, tables())
	assert.Equal(t, "YES", nullable(), "a job a webhook's run makes has no user")
	require.NoError(t, m.DownTo(ctx, 43))
	assert.Zero(t, tables())
	assert.Equal(t, "NO", nullable())
	_, err = m.Up(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, tables())
}
