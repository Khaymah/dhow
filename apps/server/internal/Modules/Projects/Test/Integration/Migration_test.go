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

const checkViolation = "23514"

func TestTheProjectsSchemaEnforcesItsInvariants(t *testing.T) {
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	ownerID := uuid.MustParse(owner.UserID)
	project := env.SeedProject(org, ownerID)
	job := env.SeedJob(org, ownerID)
	ctx := context.Background()
	tests := []struct {
		name string
		sql  string
		args []any
	}{
		{"an archived project without a time", `UPDATE "Projects" SET "State" = 'archived' WHERE "Id" = $1`, []any{project}},
		{"an active project with an archive time", `UPDATE "Projects" SET "ArchivedAt" = now() WHERE "Id" = $1`, []any{project}},
		{"an unknown fork policy", `UPDATE "Projects" SET "ForkPolicy" = 'always' WHERE "Id" = $1`, []any{project}},
		{"an upper-case slug", `UPDATE "Projects" SET "Slug" = 'Web' WHERE "Id" = $1`, []any{project}},
		{"an empty pipeline path", `UPDATE "Projects" SET "PipelinePath" = '' WHERE "Id" = $1`, []any{project}},
		{"a pipeline job without an origin", `UPDATE "Jobs" SET "OriginKind" = 'pipeline' WHERE "Id" = $1`, []any{job}},
		{"a manual job with an origin", `UPDATE "Jobs" SET "OriginId" = uuidv7() WHERE "Id" = $1`, []any{job}},
		{"an unknown origin", `UPDATE "Jobs" SET "OriginKind" = 'webhook', "OriginId" = uuidv7() WHERE "Id" = $1`, []any{job}},
		{"an unknown feature", `UPDATE "Jobs" SET "RequiredFeatures" = ARRAY['docker'] WHERE "Id" = $1`, []any{job}},
		{
			"a token scope that does not exist", `UPDATE "ApiTokens" SET "Scopes" = ARRAY['run.approve'] WHERE "Id" = $1`,
			[]any{env.SeedAPIToken(ownerID, org, "project.read", "project.manage").ID},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := env.Pool.Exec(ctx, tt.sql, tt.args...)
			var pgErr *pgconn.PgError
			require.ErrorAs(t, err, &pgErr)
			assert.Equal(t, checkViolation, pgErr.Code, err.Error())
		})
	}
}

func TestTheProjectsMigrationRollsBackCompletely(t *testing.T) {
	pool := dbtest.NewEmptyPool(t)
	ctx := context.Background()
	m, err := migrations.NewMigrator(pool, migrations.Binary{})
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()
	_, err = m.Up(ctx)
	require.NoError(t, err)
	objects := func() int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM pg_tables WHERE schemaname = current_schema() AND tablename = 'Projects') +
			(SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'Jobs'
			   AND column_name IN ('OriginKind', 'OriginId', 'RequiredFeatures'))`).Scan(&n))
		return n
	}
	assert.Equal(t, 4, objects())
	var unvalidated int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint c JOIN pg_namespace n ON n.oid = c.connamespace
		WHERE n.nspname = current_schema() AND NOT c.convalidated`).Scan(&unvalidated))
	assert.Zero(t, unvalidated, "00042 validates every constraint 00041 adds as NOT VALID")
	var indexes int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'IX_Jobs_Origin'`).Scan(&indexes))
	assert.Equal(t, 1, indexes)
	require.NoError(t, m.DownTo(ctx, 40))
	assert.Zero(t, objects())
	_, err = m.Up(ctx)
	require.NoError(t, err)
	assert.Equal(t, 4, objects())
}
