package migrations_test

import (
	"context"
	"io/fs"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

var rePgDumpMajor = regexp.MustCompile(`\(PostgreSQL\) (\d+)`)

func pgDump(t *testing.T, ctx context.Context, server int) []string {
	t.Helper()
	argv := strings.Fields(config.TestPgDump())
	version := append(append([]string{}, argv[1:]...), "--version")
	out, err := exec.CommandContext(ctx, argv[0], version...).Output()
	major := 0
	if m := rePgDumpMajor.FindSubmatch(out); err == nil && m != nil {
		major, _ = strconv.Atoi(string(m[1]))
	}
	if major < server {
		msg := "a pg_dump for PostgreSQL " + strconv.Itoa(server) + " is needed; set " + config.EnvTestPgDump +
			", e.g. \"docker run --rm --network host postgres:18 pg_dump\""
		if config.RunningInCI() {
			t.Fatal(msg)
		}
		t.Skip(msg)
	}
	return argv
}

func schemaOf(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var schema string
	require.NoError(t, pool.QueryRow(ctx, "SELECT current_schema()").Scan(&schema))
	return schema
}

func dumpSchema(t *testing.T, ctx context.Context, argv []string, schema string) string {
	t.Helper()
	args := append(append([]string{}, argv[1:]...), "--schema-only", "--no-owner", "--no-privileges", "--schema="+schema,
		"--exclude-table="+schema+".goose_db_version", config.TestDatabaseURL())
	out, err := exec.CommandContext(ctx, argv[0], args...).CombinedOutput()
	require.NoError(t, err, string(out))
	var keep []string
	for _, line := range strings.Split(strings.ReplaceAll(string(out), schema, "SCHEMA"), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "", strings.HasPrefix(trimmed, "--"), strings.HasPrefix(trimmed, `\`),
			strings.HasPrefix(trimmed, "SET "), strings.HasPrefix(trimmed, "SELECT pg_catalog.set_config"):
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "\n")
}

func applyWithSplitter(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	names, err := fs.Glob(migrations.FS, "*.sql")
	require.NoError(t, err)
	for _, name := range names {
		body, err := fs.ReadFile(migrations.FS, name)
		require.NoError(t, err)
		parsed, err := migrations.ParseUp(string(body))
		require.NoError(t, err, name)
		if !parsed.UseTx {
			for _, stmt := range parsed.Up {
				_, err := pool.Exec(ctx, stmt)
				require.NoError(t, err, "%s: %s", name, stmt)
			}
			continue
		}
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		for _, stmt := range parsed.Up {
			_, err := tx.Exec(ctx, stmt)
			require.NoError(t, err, "%s: %s", name, stmt)
		}
		require.NoError(t, tx.Commit(ctx))
	}
}

func TestSplitterBuildsTheSameSchemaAsGoose(t *testing.T) {
	ctx := context.Background()
	byGoose := dbtest.NewPool(t)
	bySplitter := dbtest.NewEmptyPool(t)
	var server int
	require.NoError(t, byGoose.QueryRow(ctx, "SELECT current_setting('server_version_num')::int / 10000").Scan(&server))
	argv := pgDump(t, ctx, server)

	applyWithSplitter(t, ctx, bySplitter)

	want := dumpSchema(t, ctx, argv, schemaOf(t, ctx, byGoose))
	got := dumpSchema(t, ctx, argv, schemaOf(t, ctx, bySplitter))
	require.Contains(t, want, `CREATE TABLE SCHEMA."Instance"`, "the dump must hold the schema")
	require.Contains(t, want, `"SchemaFloor" text DEFAULT '0.0.0'::text NOT NULL`)
	assert.Equal(t, want, got, "the splitter and goose must build the same schema from every migration")
}
