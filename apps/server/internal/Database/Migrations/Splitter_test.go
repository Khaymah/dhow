package migrations

import (
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"
)

func TestParseUp(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		want  []string
		useTx bool
		phase string
		floor string
	}{
		{
			name:  "statements end at a semicolon on their last line",
			in:    "-- +goose Up\nCREATE TABLE \"A\" (\n    \"Id\" int\n);\n\nINSERT INTO \"A\" VALUES (1);\n-- +goose Down\nDROP TABLE \"A\";\n",
			want:  []string{"CREATE TABLE \"A\" (\n    \"Id\" int\n);", "INSERT INTO \"A\" VALUES (1);"},
			useTx: true,
		},
		{
			name:  "a block keeps its inner semicolons",
			in:    "-- +goose Up\n-- +goose StatementBegin\nCREATE FUNCTION f() RETURNS int AS $$\nBEGIN\n    RETURN 1;\nEND;\n$$ LANGUAGE plpgsql;\n-- +goose StatementEnd\nSELECT 1;\n",
			want:  []string{"CREATE FUNCTION f() RETURNS int AS $$\nBEGIN\n    RETURN 1;\nEND;\n$$ LANGUAGE plpgsql;", "SELECT 1;"},
			useTx: true,
		},
		{
			name:  "directives and NO TRANSACTION",
			in:    "-- +goose NO TRANSACTION\n-- dhow:phase Contract\n-- dhow:floor 1.2.0\n-- +goose Up\nDROP INDEX CONCURRENTLY IF EXISTS \"X\";\n",
			want:  []string{"DROP INDEX CONCURRENTLY IF EXISTS \"X\";"},
			phase: "contract",
			floor: "1.2.0",
		},
		{
			name:  "leading comments are dropped and inner ones kept",
			in:    "-- +goose Up\n-- about A\nCREATE TABLE \"A\" (\n    \"Id\" int -- the key; not the end\n);\n",
			want:  []string{"CREATE TABLE \"A\" (\n    \"Id\" int -- the key; not the end\n);"},
			useTx: true,
		},
		{
			name:  "a semicolon before a trailing comment ends the statement",
			in:    "-- +goose Up\nSELECT 1; -- done\nSELECT 2;\n",
			want:  []string{"SELECT 1; -- done", "SELECT 2;"},
			useTx: true,
		},
		{
			name: "NO TRANSACTION after the Down section, as goose reads it",
			in:   "-- dhow:phase expand\n-- +goose Up\nSELECT 1;\n-- +goose Down\nSELECT 2;\n-- +goose NO TRANSACTION\n",
			want: []string{"SELECT 1;"}, phase: "expand",
		},
		{
			name: "NO TRANSACTION inside the Up section",
			in:   "-- +goose Up\nSELECT 1;\n-- +goose NO TRANSACTION\nSELECT 2;\n",
			want: []string{"SELECT 1;", "SELECT 2;"},
		},
		{
			name:  "an Up section with no statements",
			in:    "-- +goose Up\n-- nothing\n-- +goose Down\n",
			useTx: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseUp(tt.in)
			if err != nil {
				t.Fatalf("ParseUp: %v", err)
			}
			if !reflect.DeepEqual(got.Up, tt.want) {
				t.Errorf("statements\n got %q\nwant %q", got.Up, tt.want)
			}
			if got.UseTx != tt.useTx || got.Phase != tt.phase || got.Floor != tt.floor {
				t.Errorf("got tx=%v phase=%q floor=%q, want tx=%v phase=%q floor=%q",
					got.UseTx, got.Phase, got.Floor, tt.useTx, tt.phase, tt.floor)
			}
		})
	}
}

func TestParseUpRejects(t *testing.T) {
	for name, in := range map[string]string{
		"no Up":                       "SELECT 1;\n",
		"a statement before Up":       "SELECT 1;\n-- +goose Up\nSELECT 2;\n",
		"a missing semicolon":         "-- +goose Up\nSELECT 1\n",
		"a missing semicolon at Down": "-- +goose Up\nSELECT 1\n-- +goose Down\nSELECT 2;\n",
		"an open block":               "-- +goose Up\n-- +goose StatementBegin\nSELECT 1;\n",
		"an End without Begin":        "-- +goose Up\n-- +goose StatementEnd\n",
		"two Up sections":             "-- +goose Up\n-- +goose Up\n",
		"envsub":                      "-- +goose ENVSUB ON\n-- +goose Up\nSELECT 1;\n",
		"an unknown annotation":       "-- +goose Sideways\n-- +goose Up\nSELECT 1;\n",
		"an indented annotation":      "  -- +goose Up\nSELECT 1;\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseUp(in); !errors.Is(err, ErrSplit) {
				t.Errorf("ParseUp accepted %q (err %v)", in, err)
			}
		})
	}
}

func TestParseUpReadsEveryEmbeddedMigration(t *testing.T) {
	names, err := fs.Glob(FS, "*.sql")
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	for _, name := range names {
		body, err := fs.ReadFile(FS, name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		parsed, err := ParseUp(string(body))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		for k, stmt := range parsed.Up {
			if TransactionControl(stmt) {
				t.Errorf("%s: statement %d controls the transaction", name, k+1)
			}
		}
		if len(parsed.Up) == 0 {
			t.Errorf("%s: no statements", name)
		}
		if parsed.UseTx == strings.Contains(string(body), "+goose NO TRANSACTION") {
			t.Errorf("%s: UseTx %v does not match the annotation", name, parsed.UseTx)
		}
	}
}

func TestConcurrentIndexNames(t *testing.T) {
	got := ConcurrentIndexNames([]string{
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS "IX_A" ON "T" ("X");`,
		"create unique index concurrently ix_b on t (x);",
		`CREATE INDEX CONCURRENTLY "IX ""q""" ON "T" ("X");`,
		`CREATE INDEX "IX_NotConcurrent" ON "T" ("X");`,
		`ALTER TABLE "T" VALIDATE CONSTRAINT "C";`,
		`CREATE INDEX CONCURRENTLY ON "T" ("X");`,
		`CREATE UNIQUE INDEX CONCURRENTLY ON ONLY "T" ("X");`,
	})
	want := []string{"IX_A", "ix_b", `IX "q"`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTransactionControl(t *testing.T) {
	for stmt, want := range map[string]bool{
		"COMMIT;":                  true,
		"commit work;":             true,
		"END;":                     true,
		"ROLLBACK;":                true,
		"ABORT;":                   true,
		"BEGIN;":                   true,
		"START TRANSACTION;":       true,
		"SAVEPOINT s;":             true,
		"RELEASE SAVEPOINT s;":     true,
		"PREPARE TRANSACTION 'x';": true,
		"COMMIT PREPARED 'x';":     true,
		"CREATE TABLE \"A\" (\"Id\" int); COMMIT;": true,
		"-- a note\nCOMMIT;":                       true,
		"SELECT 'COMMIT; END;';":                   false,
		"CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END $$ LANGUAGE plpgsql;": false,
		"DO $body$ BEGIN PERFORM 1; END $body$;":                                         false,
		"SELECT 1; -- COMMIT;":                                                           false,
		"/* ROLLBACK; */ SELECT 1;":                                                      false,
		"CREATE TABLE \"Commit\" (\"End\" int);":                                         false,
	} {
		if got := TransactionControl(stmt); got != want {
			t.Errorf("TransactionControl(%q) = %v, want %v", stmt, got, want)
		}
	}
}
