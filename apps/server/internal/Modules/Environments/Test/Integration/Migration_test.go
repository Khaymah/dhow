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
	foreignKey      = "23503"
)

func TestTheEnvironmentsSchemaEnforcesItsInvariants(t *testing.T) {
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	project := env.SeedProject(org, uuid.MustParse(owner.UserID))
	environment := env.SeedEnvironment(project, "production")
	otherOrg := env.SeedOrganization("Other", "other")
	ctx := context.Background()
	tests := []struct {
		name string
		code string
		sql  string
		args []any
	}{
		{"two required approvals", checkViolation, `UPDATE "Environments" SET "RequiredApprovals" = 2 WHERE "Id" = $1`, []any{environment}},
		{"an upper-case name", checkViolation, `UPDATE "Environments" SET "Name" = 'Production' WHERE "Id" = $1`, []any{environment}},
		{"a wait over thirty days", checkViolation, `UPDATE "Environments" SET "WaitMinutes" = 43201 WHERE "Id" = $1`, []any{environment}},
		{"a timeout of zero hours", checkViolation, `UPDATE "Environments" SET "ApprovalTimeoutHours" = 0 WHERE "Id" = $1`, []any{environment}},
		{"another isolation", checkViolation, `UPDATE "Environments" SET "RequiredIsolation" = 'gvisor' WHERE "Id" = $1`, []any{environment}},
		{"refs that are not a list", checkViolation, `UPDATE "Environments" SET "AllowedRefs" = '{}' WHERE "Id" = $1`, []any{environment}},
		{
			"fifty-one refs", checkViolation,
			`UPDATE "Environments" SET "AllowedRefs" = (SELECT jsonb_agg(jsonb_build_object('Kind', 'branch', 'Pattern', 'r' || i))
			 FROM generate_series(1, 51) i) WHERE "Id" = $1`,
			[]any{environment},
		},
		{
			"a pause without a reason", checkViolation, `UPDATE "Environments" SET "PausedAt" = now(), "PausedByUserId" = $2 WHERE "Id" = $1`,
			[]any{environment, owner.UserID},
		},
		{
			"a reviewer who is a user and a team", checkViolation,
			`INSERT INTO "EnvironmentReviewers" ("OrganizationId", "EnvironmentId", "UserId", "TeamId")
			 VALUES ($1, $2, $3, (SELECT "Id" FROM "Teams" LIMIT 1))`,
			[]any{org, environment, owner.UserID},
		},
		{
			"a reviewer who is nobody", checkViolation,
			`INSERT INTO "EnvironmentReviewers" ("OrganizationId", "EnvironmentId") VALUES ($1, $2)`,
			[]any{org, environment},
		},
		{
			"the same name twice in a project", uniqueViolation,
			`INSERT INTO "Environments" ("OrganizationId", "ProjectId", "Name", "CreatedByUserId") VALUES ($1, $2, 'production', $3)`,
			[]any{org, project, owner.UserID},
		},
		{
			"a project of another organization", foreignKey,
			`INSERT INTO "Environments" ("OrganizationId", "ProjectId", "Name", "CreatedByUserId") VALUES ($1, $2, 'staging', $3)`,
			[]any{otherOrg, project, owner.UserID},
		},
		{
			"a reviewer row of another organization", foreignKey,
			`INSERT INTO "EnvironmentReviewers" ("OrganizationId", "EnvironmentId", "UserId") VALUES ($1, $2, $3)`,
			[]any{otherOrg, environment, owner.UserID},
		},
	}
	_, err := env.Pool.Exec(ctx, `INSERT INTO "Teams" ("OrganizationId", "Name") VALUES ($1, 'Ops')`, org)
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := env.Pool.Exec(ctx, tt.sql, tt.args...)
			var pgErr *pgconn.PgError
			require.ErrorAs(t, err, &pgErr)
			assert.Equal(t, tt.code, pgErr.Code, err.Error())
		})
	}
	_, err = env.Pool.Exec(ctx, `UPDATE "Environments" SET "IsDeleted" = true WHERE "Id" = $1`, environment)
	require.NoError(t, err)
	assert.NotEqual(t, environment, env.SeedEnvironment(project, "production"), "a removed environment frees its name")
}

func TestTheEnvironmentsMigrationRollsBackCompletely(t *testing.T) {
	pool := dbtest.NewEmptyPool(t)
	ctx := context.Background()
	m, err := migrations.NewMigrator(pool)
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()
	_, err = m.Up(ctx)
	require.NoError(t, err)
	tables := func() int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname = current_schema()
			AND tablename IN ('Environments', 'EnvironmentReviewers')`).Scan(&n))
		return n
	}
	assert.Equal(t, 2, tables())
	require.NoError(t, m.DownTo(ctx, 49))
	assert.Zero(t, tables())
	_, err = m.Up(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, tables())
}
