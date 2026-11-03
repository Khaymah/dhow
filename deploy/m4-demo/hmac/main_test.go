package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTheHMACMatchesAKnownVector(t *testing.T) {
	key := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.WriteFile(key, []byte("key\n"), 0o600))
	var out bytes.Buffer
	code := run([]string{key}, strings.NewReader("The quick brown fox jumps over the lazy dog"), &out)
	require.Equal(t, 0, code)
	assert.Equal(t, "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8\n", out.String())
}

func TestAnEmptyOrMissingKeyIsRefused(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.WriteFile(empty, []byte("\n"), 0o600))
	assert.Equal(t, 1, run([]string{empty}, strings.NewReader("x"), &bytes.Buffer{}))
	assert.Equal(t, 1, run([]string{filepath.Join(t.TempDir(), "none")}, strings.NewReader("x"), &bytes.Buffer{}))
	assert.Equal(t, 2, run(nil, strings.NewReader("x"), &bytes.Buffer{}))
}
