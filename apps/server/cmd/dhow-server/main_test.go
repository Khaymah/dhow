package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
)

func TestRunRejectsUnknownCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unknown", []string{"serve"}},
		{"healthcheck with extra arguments", []string{"healthcheck", "--now"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(context.Background(), tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown command")
		})
	}
}

func TestRunHealthcheckFailsWhenNothingListens(t *testing.T) {
	t.Setenv("DHOW_LISTEN_ADDR", "127.0.0.1:1")
	require.Error(t, run(context.Background(), []string{"healthcheck"}))
}

func TestRunDispatchesInstallerCommands(t *testing.T) {
	for _, cmd := range []string{"install", "doctor", "uninstall"} {
		t.Run(cmd, func(t *testing.T) {
			require.NoError(t, run(context.Background(), []string{cmd, "--help"}))
		})
	}
	err := run(context.Background(), []string{"install", "--bogus"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flag provided but not defined")
}

func TestExitCodeOfASchemaTooNew(t *testing.T) {
	tooNew := &migrations.IncompatibleError{Status: migrations.Evaluate(60, 50, "2.0.0", migrations.Binary{Version: "1.0.0"})}
	assert.Equal(t, 78, exitCode(tooNew))
	assert.Equal(t, 78, exitCode(fmt.Errorf("start: %w", tooNew)))
	assert.Equal(t, 1, exitCode(errors.New("anything else")))
	assert.Equal(t, 1, exitCode(otherExit{}), "only a schema too new for the binary exits with 78")
}

type otherExit struct{}

func (otherExit) Error() string { return "other" }

func (otherExit) ExitCode() int { return 78 }

func TestExitCodeOfAKeyringThatDoesNotFit(t *testing.T) {
	missing := &keys.Error{Code: keys.CodeKeyringMissingVersion, Version: 2}
	assert.Equal(t, keys.ExitKeyring, exitCode(missing))
	assert.Equal(t, 79, exitCode(fmt.Errorf("start: %w", missing)))
	assert.Equal(t, 1, exitCode(&keys.NotEverywhereError{Version: 2}), "a refused activation is not a boot failure")
}

func TestRunDispatchesKeys(t *testing.T) {
	require.ErrorIs(t, run(context.Background(), []string{"keys"}), app.ErrKeysUsage)
	require.ErrorIs(t, run(context.Background(), []string{"keys", "rotate"}), app.ErrKeysUsage)
	t.Setenv("DHOW_KEYRING_FILE", "")
	t.Setenv("CREDENTIALS_DIRECTORY", "")
	err := run(context.Background(), []string{"keys", "generate"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DHOW_KEYRING_FILE")
}

func TestRunDispatchesMigrate(t *testing.T) {
	require.ErrorIs(t, run(context.Background(), []string{"migrate"}), app.ErrMigrateUsage)
	require.ErrorIs(t, run(context.Background(), []string{"migrate", "up"}), app.ErrMigrateUsage)
	t.Setenv("DHOW_DATABASE_URL", "")
	t.Setenv("DHOW_DATABASE_URL_FILE", "")
	t.Setenv("CREDENTIALS_DIRECTORY", "")
	err := run(context.Background(), []string{"migrate", "status"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DHOW_DATABASE_URL")
}
