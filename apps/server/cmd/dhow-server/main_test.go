package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
}
