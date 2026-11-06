package deployrelease_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

type checkRig struct {
	remotes []*localRemote
	refuse  map[int]error
	rec     *recorder
}

func newCheckRig(t *testing.T, n int) *checkRig {
	t.Helper()
	c := &checkRig{refuse: map[int]error{}, rec: &recorder{}}
	for range n {
		c.remotes = append(c.remotes, newLocalRemote(t))
	}
	return c
}

func (c *checkRig) run() *deployrelease.Failure {
	checker := &deployrelease.Checker{DeployPath: "/srv/app", Report: c.rec, Log: quiet{}, Clock: fastClock()}
	for i, r := range c.remotes {
		checker.Hosts = append(checker.Hosts, deployrelease.Host{
			ID: fmt.Sprintf("0190a1b2-0000-7000-8000-00000000000%d", i+1), Name: fmt.Sprintf("web-%d.example.com", i+1),
			Connect: func(context.Context) (deployrelease.Remote, error) {
				if err := c.refuse[i]; err != nil {
					return nil, err
				}
				return r, nil
			},
		})
	}
	return checker.Run(context.Background())
}

func (c *checkRig) reasons() []string {
	var out []string
	for _, e := range c.rec.events {
		out = append(out, e.Reason)
	}
	return out
}

type quiet struct{}

func (quiet) Info(string, ...any) {}
func (quiet) Warn(string, ...any) {}

func settle(t *testing.T, r *localRemote, current, verified string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(r.path("/srv/app/releases/"+current), 0o755))
	require.NoError(t, os.MkdirAll(r.path("/srv/app/.dhow"), 0o755))
	require.NoError(t, os.Symlink("releases/"+current, r.path("/srv/app/current")))
	if verified != "" {
		require.NoError(t, os.WriteFile(r.path("/srv/app/.dhow/verified"), []byte(verified+"\n"), 0o644))
	}
}

func TestACheckOfThreeServersGivesEachItsOwnReason(t *testing.T) {
	c := newCheckRig(t, 3)
	settle(t, c.remotes[0], "10-aaaaaaaa-0", "10-aaaaaaaa-0")
	c.refuse[1] = fmt.Errorf("log in to the server: %w", deployrelease.ErrAuthFailed)
	settle(t, c.remotes[2], "10-aaaaaaaa-0", "10-aaaaaaaa-0")
	c.remotes[2].free = 1 << 20

	f := c.run()
	require.NotNil(t, f)
	assert.Equal(t, deployrelease.ExitCheckFailed, f.Exit)
	assert.Equal(t, []string{deployrelease.CheckOK, deployrelease.CheckAuthFailed, deployrelease.CheckDiskLow}, c.reasons())
	assert.Equal(t, "10-aaaaaaaa-0", c.rec.events[0].CurrentReleaseID)
	assert.Equal(t, "10-aaaaaaaa-0", c.rec.events[0].VerifiedReleaseID)
}

func TestACheckSaysWhyAServerIsNotReadyAndChangesNothing(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(r *localRemote)
		refuse error
		want   string
	}{
		{"a wrong host key", nil, deployrelease.ErrHostKeyMismatch, deployrelease.CheckHostKey},
		{"a server that cannot be reached", nil, errors.New("dial tcp: refused"), deployrelease.CheckUnreachable},
		{"a deploy path that is gone", func(r *localRemote) { require.NoError(t, os.RemoveAll(r.path("/srv/app"))) }, nil, deployrelease.CheckPathMissing},
		{"a current that is a directory", func(r *localRemote) {
			require.NoError(t, os.MkdirAll(r.path("/srv/app/current"), 0o755))
		}, nil, deployrelease.CheckCurrentNotLink},
		{"a damaged state file", func(r *localRemote) {
			settle(t, r, "10-aaaaaaaa-0", "10-aaaaaaaa-0")
			require.NoError(t, os.WriteFile(r.path("/srv/app/.dhow/state.json"), []byte("{not json"), 0o644))
		}, nil, deployrelease.CheckStateCorrupt},
		{"a verified file that is not a release", func(r *localRemote) {
			settle(t, r, "10-aaaaaaaa-0", "")
			require.NoError(t, os.WriteFile(r.path("/srv/app/.dhow/verified"), []byte("../../etc\n"), 0o644))
		}, nil, deployrelease.CheckStateCorrupt},
		{"a lock held by a live deployment", func(r *localRemote) {
			settle(t, r, "10-aaaaaaaa-0", "10-aaaaaaaa-0")
			require.NoError(t, os.MkdirAll(r.path("/srv/app/.dhow/lock"), 0o700))
			require.NoError(t, os.WriteFile(r.path("/srv/app/.dhow/lock/owner"), []byte(`{"Release":"11-bbbbbbbb-0","ExpiresAt":"2999-01-01T00:00:00Z"}`), 0o600))
		}, nil, deployrelease.CheckLocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newCheckRig(t, 1)
			if tc.setup != nil {
				tc.setup(c.remotes[0])
			}
			c.refuse[0] = tc.refuse
			before := snapshot(t, c.remotes[0].root)
			f := c.run()
			require.NotNil(t, f)
			assert.Equal(t, []string{tc.want}, c.reasons())
			assert.Equal(t, before, snapshot(t, c.remotes[0].root), "a check writes nothing on the server")
		})
	}
}

func TestACheckLetsADivergedServerPassAndSaysSo(t *testing.T) {
	c := newCheckRig(t, 1)
	settle(t, c.remotes[0], "11-bbbbbbbb-0", "10-aaaaaaaa-0")
	require.NoError(t, os.WriteFile(c.remotes[0].path("/srv/app/.dhow/state.json"),
		[]byte(`{"State":"switching","Release":"11-bbbbbbbb-0","Previous":"10-aaaaaaaa-0","At":"2026-11-05T12:00:00Z"}`), 0o644))

	assert.Nil(t, c.run(), "a deployment copes with a server that is not on its verified release")
	assert.Equal(t, []string{deployrelease.CheckDiverged}, c.reasons())
	assert.Equal(t, "11-bbbbbbbb-0", c.rec.events[0].CurrentReleaseID)
	assert.Equal(t, "10-aaaaaaaa-0", c.rec.events[0].VerifiedReleaseID)
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		switch {
		case d.Type()&os.ModeSymlink != 0:
			target, _ := os.Readlink(p)
			out[rel] = "-> " + target
		case d.IsDir():
			out[rel] = "dir"
		default:
			raw, _ := os.ReadFile(p)
			out[rel] = string(raw)
		}
		return nil
	}))
	return out
}

func TestADeployRefusesADamagedStateFileBeforeItChangesAnything(t *testing.T) {
	files := map[string]string{
		"state.json": "{\"State\":\"flying\",\"Release\":\"x\"}",
		"verified":   "not a release id!\n",
		"history":    "10-aaaaaaaa-0\n../bad\n",
	}
	for name, body := range files {
		t.Run(name, func(t *testing.T) {
			r := newLocalRemote(t)
			path, digest := makeArchive(t, map[string]string{"index.html": "v2"})
			require.NoError(t, os.MkdirAll(r.path("/srv/app/.dhow"), 0o755))
			require.NoError(t, os.WriteFile(r.path("/srv/app/.dhow/"+name), []byte(body), 0o644))
			before := snapshot(t, r.root)

			out := deploy(t, r, baseConfig(path, digest, "11-bbbbbbbb-0"), &probe{status: healthy})

			require.NotNil(t, out.failure)
			assert.Equal(t, deployrelease.ExitStateCorrupt, out.failure.Exit)
			assert.ErrorIs(t, out.failure, deployrelease.ErrStateCorrupt)
			assert.Contains(t, out.failure.Err.Error(), name, "the reason names the damaged file")
			assert.Equal(t, before, snapshot(t, r.root), "nothing changed on the server")
		})
	}
}

func TestADeployAcceptsTheStateFilesItWrites(t *testing.T) {
	r := newLocalRemote(t)
	first, d1 := makeArchive(t, map[string]string{"index.html": "v1"})
	second, d2 := makeArchive(t, map[string]string{"index.html": "v2"})
	require.Nil(t, deploy(t, r, baseConfig(first, d1, "10-aaaaaaaa-0"), &probe{status: healthy}).failure)
	require.Nil(t, deploy(t, r, baseConfig(second, d2, "11-bbbbbbbb-0"), &probe{status: healthy}).failure)
}
