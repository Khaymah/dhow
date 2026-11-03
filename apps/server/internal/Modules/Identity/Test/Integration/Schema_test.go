package integration_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

var identityTables = []string{"Organizations", "Users", "Memberships", "Sessions", "LoginThrottles", "SetupTokens", "AuditEvents"}

func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1)`, name).Scan(&exists))
	return exists
}

func TestMigrationsGoUpAndDownCleanly(t *testing.T) {
	pool := dbtest.NewEmptyPool(t)
	ctx := context.Background()
	m, err := migrations.NewMigrator(pool, migrations.Binary{})
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()

	_, err = m.Up(ctx)
	require.NoError(t, err)
	for _, name := range identityTables {
		assert.True(t, tableExists(t, pool, name), name)
	}

	require.NoError(t, m.DownTo(ctx, 1))
	for _, name := range identityTables {
		assert.False(t, tableExists(t, pool, name), "%s must be gone after rolling back", name)
	}
	assert.True(t, tableExists(t, pool, "Instance"), "migration 00001 is untouched")
	var functions int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname = current_schema() AND p.proname = 'AuditEvents_RejectChange'`).Scan(&functions))
	assert.Zero(t, functions, "the trigger function goes with the table")

	_, err = m.Up(ctx)
	require.NoError(t, err)
	require.NoError(t, m.DownTo(ctx, 0))
	_, err = m.Up(ctx)
	require.NoError(t, err)
	state, err := m.State(ctx)
	require.NoError(t, err)
	assert.Equal(t, migrations.SchemaCurrent, state)
}

func TestEveryTableFollowsTheConventions(t *testing.T) {
	pool := dbtest.NewPool(t)
	ctx := context.Background()
	for _, table := range identityTables {
		t.Run(table, func(t *testing.T) {
			rows, err := pool.Query(ctx, `SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1`, table)
			require.NoError(t, err)
			defer rows.Close()
			type column struct {
				dataType, nullable string
				def                *string
			}
			cols := map[string]column{}
			for rows.Next() {
				var name string
				var c column
				require.NoError(t, rows.Scan(&name, &c.dataType, &c.nullable, &c.def))
				cols[name] = c
				assert.NotEqual(t, "timestamp without time zone", c.dataType, "%s.%s", table, name)
				assert.NotEqual(t, "character varying", c.dataType, "%s.%s", table, name)
			}
			require.NoError(t, rows.Err())
			assert.Equal(t, "uuid", cols["Id"].dataType)
			assert.Contains(t, *cols["Id"].def, "uuidv7()")
			assert.Equal(t, "timestamp with time zone", cols["CreatedAt"].dataType)
			assert.Equal(t, "timestamp with time zone", cols["UpdatedAt"].dataType)
			assert.Equal(t, "boolean", cols["IsDeleted"].dataType)
			assert.Equal(t, "NO", cols["IsDeleted"].nullable)
			for name := range cols {
				assert.Regexp(t, `^[A-Z][A-Za-z0-9]*$`, name, "columns are PascalCase")
			}
		})
	}
}

func TestConstraints(t *testing.T) {
	pool := dbtest.NewPool(t)
	ctx := context.Background()
	org := func(slug string) string {
		var id string
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO "Organizations" ("Name", "Slug") VALUES ('Org', $1) RETURNING "Id"::text`, slug).Scan(&id))
		return id
	}
	user := func(email string) string {
		var id string
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO "Users" ("Email", "Name", "PasswordHash") VALUES ($1, 'U', 'x') RETURNING "Id"::text`, email).Scan(&id))
		return id
	}
	orgID, userID := org("acme"), user("a@example.com")

	rejected := []struct {
		name      string
		statement string
		args      []any
	}{
		{"an unknown role", `INSERT INTO "Memberships" ("UserId", "OrganizationId", "Role") VALUES ($1, $2, 'root')`, []any{userID, orgID}},
		{"an upper-case role", `INSERT INTO "Memberships" ("UserId", "OrganizationId", "Role") VALUES ($1, $2, 'Owner')`, []any{userID, orgID}},
		{"a membership without a user", `INSERT INTO "Memberships" ("UserId", "OrganizationId", "Role") VALUES (uuidv7(), $1, 'viewer')`, []any{orgID}},
		{"a mixed-case email", `INSERT INTO "Users" ("Email", "Name", "PasswordHash") VALUES ('Mixed@Example.com', 'U', 'x')`, nil},
		{"a duplicate email", `INSERT INTO "Users" ("Email", "Name", "PasswordHash") VALUES ('a@example.com', 'U', 'x')`, nil},
		{"an unknown locale", `INSERT INTO "Users" ("Email", "Name", "PasswordHash", "Locale") VALUES ('l@example.com', 'U', 'x', 'fr')`, nil},
		{"an empty name", `INSERT INTO "Users" ("Email", "Name", "PasswordHash") VALUES ('n@example.com', '', 'x')`, nil},
		{"a duplicate slug", `INSERT INTO "Organizations" ("Name", "Slug") VALUES ('Again', 'acme')`, nil},
		{"a slug with capitals", `INSERT INTO "Organizations" ("Name", "Slug") VALUES ('Bad', 'Bad')`, nil},
		{"a slug with a double hyphen", `INSERT INTO "Organizations" ("Name", "Slug") VALUES ('Bad', 'a--b')`, nil},
		{"a slug that is too long", `INSERT INTO "Organizations" ("Name", "Slug") VALUES ('Bad', repeat('a', 64))`, nil},
		{"a session token hash of the wrong size", `INSERT INTO "Sessions" ("UserId", "TokenHash", "CsrfHash", "ExpiresAt", "IdleExpiresAt") VALUES ($1, decode('00', 'hex'), sha256('x'), now(), now())`, []any{userID}},
		{"a session without a user", `INSERT INTO "Sessions" ("UserId", "TokenHash", "CsrfHash", "ExpiresAt", "IdleExpiresAt") VALUES (uuidv7(), sha256('t'), sha256('x'), now(), now())`, nil},
		{"a setup token hash of the wrong size", `INSERT INTO "SetupTokens" ("TokenHash", "ExpiresAt") VALUES (decode('00', 'hex'), now())`, nil},
		{"an audit hash of the wrong size", `INSERT INTO "AuditEvents" ("Action", "PrevHash", "Hash") VALUES ('x', decode('00', 'hex'), sha256('x'))`, nil},
		{"an audit event without an action", `INSERT INTO "AuditEvents" ("Action", "PrevHash", "Hash") VALUES ('', sha256('x'), sha256('x'))`, nil},
		{"a negative failure count", `INSERT INTO "LoginThrottles" ("Key", "Failures") VALUES ('k', -1)`, nil},
	}
	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pool.Exec(ctx, tt.statement, tt.args...)
			assert.Error(t, err)
		})
	}

	t.Run("a user cannot hold two memberships in one organization", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO "Memberships" ("UserId", "OrganizationId", "Role") VALUES ($1, $2, 'viewer')`, userID, orgID)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO "Memberships" ("UserId", "OrganizationId", "Role") VALUES ($1, $2, 'admin')`, userID, orgID)
		assert.Error(t, err)
	})

	t.Run("unique values are free again once the row is soft-deleted", func(t *testing.T) {
		_, err := pool.Exec(ctx, `UPDATE "Users" SET "IsDeleted" = true WHERE "Email" = 'a@example.com'`)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO "Users" ("Email", "Name", "PasswordHash") VALUES ('a@example.com', 'U', 'x')`)
		assert.NoError(t, err)
		_, err = pool.Exec(ctx, `UPDATE "Organizations" SET "IsDeleted" = true WHERE "Slug" = 'acme'`)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO "Organizations" ("Name", "Slug") VALUES ('Again', 'acme')`)
		assert.NoError(t, err)
	})

	t.Run("token hashes are unique even after revocation", func(t *testing.T) {
		stmt := `INSERT INTO "Sessions" ("UserId", "TokenHash", "CsrfHash", "ExpiresAt", "IdleExpiresAt", "RevokedAt") VALUES ($1, sha256('same'), sha256('c'), now(), now(), now())`
		_, err := pool.Exec(ctx, stmt, userID)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, stmt, userID)
		assert.Error(t, err)
	})
}

func TestIndexesExist(t *testing.T) {
	pool := dbtest.NewPool(t)
	rows, err := pool.Query(context.Background(), `SELECT indexname FROM pg_indexes WHERE schemaname = current_schema()`)
	require.NoError(t, err)
	defer rows.Close()
	have := map[string]bool{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		have[name] = true
	}
	require.NoError(t, rows.Err())
	for _, want := range []string{
		"UX_Organizations_Slug", "UX_Users_Email", "UX_Memberships_User_Organization", "IX_Memberships_Organization",
		"UX_Sessions_TokenHash", "IX_Sessions_User_Active", "UX_LoginThrottles_Key", "UX_SetupTokens_TokenHash",
		"UX_AuditEvents_Seq", "IX_AuditEvents_Organization_Seq", "IX_AuditEvents_Actor",
	} {
		assert.True(t, have[want], want)
	}
}
