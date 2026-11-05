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

const goodFingerprint = "SHA256:AbCdEfGhIjKlMnOpQrStUvWxYz0123456789AbCdEfG"

func TestTheTargetsSchemaEnforcesItsInvariants(t *testing.T) {
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	project := env.SeedProject(org, uuid.MustParse(owner.UserID))
	environment := env.SeedEnvironment(project, "production")
	target, host := env.SeedDeployTarget(project, "production", "web")
	otherOrg := env.SeedOrganization("Other", "other")
	ctx := context.Background()
	tests := []struct {
		name string
		code string
		sql  string
		args []any
	}{
		{"an upper-case name", checkViolation, `UPDATE "DeployTargets" SET "Name" = 'Web' WHERE "Id" = $1`, []any{target}},
		{"an unknown kind", checkViolation, `UPDATE "DeployTargets" SET "Kind" = 'ftp' WHERE "Id" = $1`, []any{target}},
		{"a username with a space", checkViolation, `UPDATE "DeployTargets" SET "Username" = 'a b' WHERE "Id" = $1`, []any{target}},
		{"a relative path", checkViolation, `UPDATE "DeployTargets" SET "DeployPath" = 'srv/app' WHERE "Id" = $1`, []any{target}},
		{"a path with a shell character", checkViolation, `UPDATE "DeployTargets" SET "DeployPath" = '/srv/a;b' WHERE "Id" = $1`, []any{target}},
		{"one release kept", checkViolation, `UPDATE "DeployTargets" SET "KeepReleases" = 1 WHERE "Id" = $1`, []any{target}},
		{"fifty-one releases kept", checkViolation, `UPDATE "DeployTargets" SET "KeepReleases" = 51 WHERE "Id" = $1`, []any{target}},
		{"settings that are not an object", checkViolation, `UPDATE "DeployTargets" SET "Settings" = '[]' WHERE "Id" = $1`, []any{target}},
		{
			"settings over four kibibytes", checkViolation,
			`UPDATE "DeployTargets" SET "Settings" = jsonb_build_object('A', repeat('x', 5000)) WHERE "Id" = $1`,
			[]any{target},
		},
		{"eleven allowed images", checkViolation, `UPDATE "DeployTargets" SET "AllowedImages" = array_fill('a'::text, ARRAY[11]) WHERE "Id" = $1`, []any{target}},
		{"a secret name with a dash", checkViolation, `UPDATE "DeployTargets" SET "KeySecretName" = 'A-B' WHERE "Id" = $1`, []any{target}},
		{"a version of zero", checkViolation, `UPDATE "DeployTargets" SET "Version" = 0 WHERE "Id" = $1`, []any{target}},
		{
			"the same name twice in an environment", uniqueViolation,
			`INSERT INTO "DeployTargets" ("OrganizationId", "ProjectId", "EnvironmentId", "Name", "Kind", "Username", "DeployPath", "CreatedByUserId")
			 SELECT "OrganizationId", "ProjectId", "EnvironmentId", "Name", "Kind", "Username", '/srv/other', "CreatedByUserId" FROM "DeployTargets" WHERE "Id" = $1`,
			[]any{target},
		},
		{
			"an environment of another organization", foreignKey,
			`INSERT INTO "DeployTargets" ("OrganizationId", "ProjectId", "EnvironmentId", "Name", "Kind", "Username", "DeployPath", "CreatedByUserId")
			 VALUES ($1, $2, $3, 'x', 'ssh', 'deploy', '/srv/x', $4)`,
			[]any{otherOrg, project, environment, owner.UserID},
		},
		{"a position of zero", checkViolation, `UPDATE "DeployTargetHosts" SET "Position" = 0 WHERE "Id" = $1`, []any{host}},
		{"a port of zero", checkViolation, `UPDATE "DeployTargetHosts" SET "Port" = 0 WHERE "Id" = $1`, []any{host}},
		{"a port over 65535", checkViolation, `UPDATE "DeployTargetHosts" SET "Port" = 65536 WHERE "Id" = $1`, []any{host}},
		{"an empty host", checkViolation, `UPDATE "DeployTargetHosts" SET "Host" = '' WHERE "Id" = $1`, []any{host}},
		{
			"a pinned key without a fingerprint", checkViolation,
			`UPDATE "DeployTargetHosts" SET "HostKey" = 'ssh-ed25519 AAAA' WHERE "Id" = $1`,
			[]any{host},
		},
		{
			"a pinned key without who confirmed it", checkViolation,
			`UPDATE "DeployTargetHosts" SET "HostKey" = 'ssh-ed25519 AAAA', "HostKeyFingerprint" = $2, "HostKeyConfirmedAt" = now() WHERE "Id" = $1`,
			[]any{host, goodFingerprint},
		},
		{
			"a fingerprint that is not SHA256", checkViolation,
			`UPDATE "DeployTargetHosts" SET "PendingHostKey" = 'ssh-ed25519 AAAA', "PendingFingerprint" = 'MD5:aa:bb' WHERE "Id" = $1`,
			[]any{host},
		},
		{"a pending key without a fingerprint", checkViolation, `UPDATE "DeployTargetHosts" SET "PendingHostKey" = 'ssh-ed25519 AAAA' WHERE "Id" = $1`, []any{host}},
		{
			"the same position twice", uniqueViolation,
			`INSERT INTO "DeployTargetHosts" ("OrganizationId", "TargetId", "Position", "Host", "Port", "DeployPath")
			 SELECT "OrganizationId", "TargetId", "Position", 'other.example.com', 22, "DeployPath" FROM "DeployTargetHosts" WHERE "Id" = $1`,
			[]any{host},
		},
		{
			"the same address twice in a target", uniqueViolation,
			`INSERT INTO "DeployTargetHosts" ("OrganizationId", "TargetId", "Position", "Host", "Port", "DeployPath")
			 SELECT "OrganizationId", "TargetId", 2, "Host", "Port", "DeployPath" FROM "DeployTargetHosts" WHERE "Id" = $1`,
			[]any{host},
		},
		{
			"a host row of another organization", foreignKey,
			`INSERT INTO "DeployTargetHosts" ("OrganizationId", "TargetId", "Position", "Host", "Port", "DeployPath") VALUES ($1, $2, 3, 'x.example.com', 22, '/srv/x')`,
			[]any{otherOrg, target},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := env.Pool.Exec(ctx, tt.sql, tt.args...)
			var pgErr *pgconn.PgError
			require.ErrorAs(t, err, &pgErr, tt.sql)
			assert.Equal(t, tt.code, pgErr.Code, err.Error())
		})
	}

	_, err := env.Pool.Exec(ctx,
		`UPDATE "DeployTargetHosts" SET "HostKey" = 'ssh-ed25519 AAAA', "HostKeyFingerprint" = $2, "HostKeyConfirmedAt" = now(), "HostKeyConfirmedBy" = $3 WHERE "Id" = $1`,
		host, goodFingerprint, owner.UserID)
	require.NoError(t, err, "a key with its fingerprint, time and confirmer is accepted")
	_, err = env.Pool.Exec(ctx, `UPDATE "DeployTargets" SET "IsDeleted" = true WHERE "Id" = $1`, target)
	require.NoError(t, err)
	_, err = env.Pool.Exec(ctx, `UPDATE "DeployTargetHosts" SET "IsDeleted" = true WHERE "TargetId" = $1`, target)
	require.NoError(t, err)
	freed, _ := env.SeedDeployTarget(project, "production", "web")
	assert.NotEqual(t, target, freed, "a removed target frees its name and its address")
}

func TestTheTargetsMigrationRollsBackCompletely(t *testing.T) {
	pool := dbtest.NewEmptyPool(t)
	ctx := context.Background()
	m, err := migrations.NewMigrator(pool, migrations.Binary{})
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()
	_, err = m.Up(ctx)
	require.NoError(t, err)
	present := func() (int, int) {
		var tables, columns int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname = current_schema()
			AND tablename IN ('DeployTargets', 'DeployTargetHosts')`).Scan(&tables))
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema()
			AND table_name = 'Environments' AND column_name = 'AllowExtraDeployEgress'`).Scan(&columns))
		return tables, columns
	}
	tables, columns := present()
	assert.Equal(t, 2, tables)
	assert.Equal(t, 1, columns)
	require.NoError(t, m.DownTo(ctx, 66))
	tables, columns = present()
	assert.Zero(t, tables)
	assert.Zero(t, columns)
	_, err = m.Up(ctx)
	require.NoError(t, err)
	tables, columns = present()
	assert.Equal(t, 2, tables)
	assert.Equal(t, 1, columns)
}
