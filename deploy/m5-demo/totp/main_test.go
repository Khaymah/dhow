package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestTheCodeMatchesTheRFC6238Vector(t *testing.T) {
	secret, err := encoding.DecodeString(rfcSecret)
	require.NoError(t, err)
	assert.Equal(t, "287082", code(secret, 1))
	assert.Equal(t, "081804", code(secret, 37037036))
}

func TestEachCallMovesToALaterStep(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	now := time.Unix(30*1000+5, 0)
	var slept time.Duration
	call := func() string {
		var out bytes.Buffer
		require.Equal(t, 0, run([]string{state}, strings.NewReader(rfcSecret+"\n"), &out, func() time.Time { return now }, func(d time.Duration) {
			slept += d
			now = now.Add(d)
		}))
		return strings.TrimSpace(out.String())
	}
	secret, err := encoding.DecodeString(rfcSecret)
	require.NoError(t, err)
	assert.Equal(t, code(secret, 1000), call())
	assert.Equal(t, code(secret, 1001), call())
	assert.Zero(t, slept)
	assert.Equal(t, code(secret, 1002), call())
	assert.Positive(t, slept)
	raw, err := os.ReadFile(state)
	require.NoError(t, err)
	assert.Equal(t, "1002\n", string(raw))
}

func TestABadSecretOrUsageIsRefused(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	now := func() time.Time { return time.Unix(0, 0) }
	assert.Equal(t, 1, run([]string{state}, strings.NewReader("not base32 !"), &bytes.Buffer{}, now, func(time.Duration) {}))
	assert.Equal(t, 2, run(nil, strings.NewReader(rfcSecret), &bytes.Buffer{}, now, func(time.Duration) {}))
	require.NoError(t, os.WriteFile(state, []byte("x\n"), 0o600))
	assert.Equal(t, 1, run([]string{state}, strings.NewReader(rfcSecret), &bytes.Buffer{}, now, func(time.Duration) {}))
	require.NoError(t, os.WriteFile(state, []byte("999999999999\n"), 0o600))
	assert.Equal(t, 1, run([]string{state}, strings.NewReader(rfcSecret), &bytes.Buffer{}, now, func(time.Duration) {}))
}

func TestTheSecretIsNeverTakenFromTheArguments(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	now := func() time.Time { return time.Unix(0, 0) }
	var out bytes.Buffer
	assert.Equal(t, 2, run([]string{state, rfcSecret}, strings.NewReader(rfcSecret), &out, now, func(time.Duration) {}))
	assert.Empty(t, out.String())
	assert.NoFileExists(t, state)
}
