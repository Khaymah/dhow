package deployrelease_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

type swapRemote struct {
	*localRemote
	onIncoming func()
}

func (s swapRemote) Mkdir(name string, mode fs.FileMode) error {
	if strings.Contains(name, ".incoming-") && s.onIncoming != nil {
		s.onIncoming()
	}
	return s.localRemote.Mkdir(name, mode)
}

func TestAnArchiveSwappedAfterTheScanIsRefusedBeforeItGoesLive(t *testing.T) {
	r := newLocalRemote(t)
	good, digest := makeArchive(t, map[string]string{"index.html": "good"})
	other, _ := makeArchive(t, map[string]string{"index.html": "evil"})
	rec := &recorder{}
	e := &deployrelease.Engine{
		Config: baseConfig(good, digest, "1-aaaaaaaa-1"), Doer: &probe{status: healthy}, Report: rec, Clock: fastClock(),
		Connect: func(context.Context) (deployrelease.Remote, error) {
			return swapRemote{localRemote: r, onIncoming: func() {
				raw, err := os.ReadFile(other)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(good, raw, 0o644))
			}}, nil
		},
	}
	f := e.Run(context.Background())
	require.NotNil(t, f)
	assert.Equal(t, deployrelease.ExitUnsafeArtifact, f.Exit)
	assert.ErrorIs(t, f, deployrelease.ErrDigest)
	assert.Empty(t, r.currentTarget())
	assert.NoDirExists(t, r.path("/srv/app/releases/1-aaaaaaaa-1"))
	assert.Empty(t, r.releaseNames(), "the upload directory is cleaned")
}

func TestAnArchiveWithoutFilesDeploysWithoutAChecksumList(t *testing.T) {
	r := newLocalRemote(t)
	path := filepath.Join(t.TempDir(), "empty.tgz")
	f, err := os.Create(path)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "assets/", Typeflag: tar.TypeDir, Mode: 0o755}))
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())
	out := deploy(t, r, baseConfig(path, digestFile(t, path), "1-aaaaaaaa-1"), &probe{status: healthy})
	require.Nil(t, out.failure)
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.currentTarget())
	for _, argv := range r.execs {
		assert.NotEqual(t, "sha256sum", argv[0])
	}
}

func digestFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func ownerOf(t *testing.T, r *localRemote) (string, time.Time) {
	t.Helper()
	var o struct {
		Release   string
		ExpiresAt time.Time
	}
	require.NoError(t, json.Unmarshal([]byte(r.read("/srv/app/.dhow/lock/owner")), &o))
	return o.Release, o.ExpiresAt
}

func TestALongDeployRenewsItsLockWhileItRuns(t *testing.T) {
	r := newLocalRemote(t)
	var first, second time.Time
	r.hook = func([]string) deployrelease.ExecResult {
		_, first = ownerOf(t, r)
		time.Sleep(150 * time.Millisecond)
		_, second = ownerOf(t, r)
		return deployrelease.ExecResult{}
	}
	path, digest := makeArchive(t, map[string]string{"index.html": "v1"})
	cfg := baseConfig(path, digest, "1-aaaaaaaa-1")
	cfg.PostSwitch = "./warm.sh"
	e := &deployrelease.Engine{
		Config: cfg, Doer: &probe{status: healthy}, Report: &recorder{}, Clock: fastClock(), LockRenew: 10 * time.Millisecond,
		Connect: func(context.Context) (deployrelease.Remote, error) { return r, nil },
	}
	require.Nil(t, e.Run(context.Background()))
	assert.True(t, second.After(first), "the expiry moved forward while the hook ran")
	assert.NoDirExists(t, r.path("/srv/app/.dhow/lock"))
}

func TestALockThatAnotherDeploymentTookOverIsNotRemovedByUs(t *testing.T) {
	r := newLocalRemote(t)
	r.hook = func([]string) deployrelease.ExecResult {
		other := fmt.Sprintf(`{"Release":"9-zzzzzzzz-1","ExpiresAt":%q}`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
		require.NoError(t, os.WriteFile(r.path("/srv/app/.dhow/lock/owner"), []byte(other), 0o600))
		return deployrelease.ExecResult{}
	}
	out := release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, func(c *deployrelease.Config) { c.PostSwitch = "./x.sh" })
	require.Nil(t, out.failure)
	owner, _ := ownerOf(t, r)
	assert.Equal(t, "9-zzzzzzzz-1", owner, "the lock and its owner are untouched")
}

func TestAHookIsLimitedOnTheServerWhenItHasTimeoutAndOnlyFromHereWhenNot(t *testing.T) {
	for name, noTimeout := range map[string]bool{"with timeout": false, "without timeout": true} {
		t.Run(name, func(t *testing.T) {
			r := newLocalRemote(t)
			r.noTimeout = noTimeout
			out := release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, func(c *deployrelease.Config) {
				c.PostSwitch = "./warm.sh"
				c.HookTimeout = 90 * time.Second
			})
			require.Nil(t, out.failure)
			argvs := r.hookArgvs()
			require.Len(t, argvs, 1)
			if noTimeout {
				assert.NotContains(t, argvs[0], "timeout")
				return
			}
			i := slices.Index(argvs[0], "timeout")
			require.GreaterOrEqual(t, i, 0)
			assert.Equal(t, []string{"timeout", "-s", "KILL", "90", "sh", "-c"}, argvs[0][i:i+6], "the server kills the hook at the limit")
		})
	}
}
