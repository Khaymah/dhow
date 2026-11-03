package migrations

import (
	"errors"
	"strings"
	"testing"
)

func TestEvaluate(t *testing.T) {
	release := Binary{Version: "1.4.2"}
	tests := []struct {
		name   string
		db     int64
		floor  string
		bin    Binary
		want   SchemaState
		reason string
	}{
		{"the database matches the binary", 50, "0.0.0", release, SchemaCurrent, ""},
		{"the database is behind", 40, "0.0.0", release, SchemaPending, ""},
		{"a newer expand-only database", 52, "0.0.0", release, SchemaAhead, ""},
		{"the floor equals the binary", 52, "1.4.2", release, SchemaAhead, ""},
		{"the floor is below the binary", 52, "1.3.9", release, SchemaAhead, ""},
		{"a v-prefixed binary", 52, "1.4.0", Binary{Version: "v1.4.2"}, SchemaAhead, ""},
		{"the floor is above the binary", 52, "1.5.0", release, SchemaIncompatible, "the database needs dhow 1.5.0 or newer"},
		{"a patch above the binary", 52, "1.4.3", release, SchemaIncompatible, "1.4.3"},
		{"a release candidate is below its release", 52, "1.5.0", Binary{Version: "1.5.0-rc.1"}, SchemaIncompatible, "1.5.0"},
		{"a release candidate above the floor", 52, "1.5.0-rc.1", Binary{Version: "1.5.0-rc.2"}, SchemaAhead, ""},
		{"numeric pre-release identifiers compare as numbers", 52, "1.5.0-rc.2", Binary{Version: "1.5.0-rc.10"}, SchemaAhead, ""},
		{"a dev build on a newer database", 52, "0.0.0", Binary{Version: "dev"}, SchemaIncompatible, "DHOW_DEV=true"},
		{"a dev build with DHOW_DEV", 52, "9.9.9", Binary{Version: "dev", Dev: true}, SchemaAhead, ""},
		{"a git describe build is a version", 52, "0.1.0", Binary{Version: "0.1.0-3-gabc1234"}, SchemaIncompatible, "0.1.0"},
		{"a dev build on a current database", 50, "0.0.0", Binary{Version: "dev"}, SchemaCurrent, ""},
		{"a floor that is not a version", 52, "latest", release, SchemaIncompatible, "not a version"},
		{"an empty floor", 52, "", Binary{Version: "dev", Dev: true}, SchemaIncompatible, "not a version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.db, 50, tt.floor, tt.bin)
			if got.State != tt.want {
				t.Fatalf("state %q, want %q (reason %q)", got.State, tt.want, got.Reason)
			}
			if tt.reason != "" && !strings.Contains(got.Reason, tt.reason) {
				t.Errorf("reason %q does not mention %q", got.Reason, tt.reason)
			}
			if got.State.Ready() != (tt.want == SchemaCurrent || tt.want == SchemaAhead) {
				t.Errorf("Ready() is %v for %q", got.State.Ready(), got.State)
			}
		})
	}
}

func TestIncompatibleErrorNamesTheVersionAndExitCode(t *testing.T) {
	st := Evaluate(53, 50, "2.0.0", Binary{Version: "1.9.0"})
	var err error = &IncompatibleError{Status: st}
	wrapped := errors.Join(errors.New("start"), err)
	var coded interface{ ExitCode() int }
	if !errors.As(wrapped, &coded) || coded.ExitCode() != 78 {
		t.Fatalf("the error must carry exit code 78, got %v", coded)
	}
	if !errors.Is(wrapped, ErrSchemaTooNew) {
		t.Error("the error must match ErrSchemaTooNew")
	}
	for _, want := range []string{CodeSchemaTooNew, "2.0.0", "1.9.0", "upgrades.md", "migration 53", "up to 50"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q does not mention %q", err.Error(), want)
		}
	}
}
