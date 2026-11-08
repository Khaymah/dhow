package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestTheServerNeedsAFileForItsCa(t *testing.T) {
	err := run(nil, quiet())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-ca-file")
}

func TestAnUnknownFlagIsRefused(t *testing.T) {
	require.Error(t, run([]string{"-nope"}, quiet()))
}

func TestACaFileThatCannotBeWrittenStopsTheStart(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-dir", "ca.pem")
	require.Error(t, run([]string{"-ca-file", missing, "-listen", "127.0.0.1:0"}, quiet()))
	_, err := os.Stat(missing)
	assert.Error(t, err)
}
