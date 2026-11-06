package e2e_test

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

func TestADeployThenAnotherSwitchesCurrentAndKeepsTheOldRelease(t *testing.T) {
	d := newTarget(t, "basic")
	first := d.deploy("deploy-ok", spec{release: "1-aaaaaaaa-1", files: page("v1", true), health: true})
	require.Equal(t, 0, first.exit, first.output)
	assert.Equal(t, []string{"release_started", "uploaded", "switched", "health", "verified", "verified/group", "pruned"}, first.kinds())
	assert.Equal(t, "v1", d.served("index.html"))

	second := d.deploy("deploy-second", spec{release: "2-bbbbbbbb-1", files: page("v2", true), health: true})
	require.Equal(t, 0, second.exit, second.output)
	assert.Equal(t, "releases/2-bbbbbbbb-1", d.current())
	assert.Equal(t, "v2", d.served("index.html"), "the service answers with the new release")
	assert.ElementsMatch(t, []string{"1-aaaaaaaa-1", "2-bbbbbbbb-1"}, d.releases(), "the old release stays")
	assert.Equal(t, "2-bbbbbbbb-1", d.readRemote(".dhow/verified"))
	assert.False(t, d.exists(".dhow/lock"))
	var switched deployrelease.Event
	for _, e := range second.events {
		if e.Event == deployrelease.EventSwitched {
			switched = e
		}
	}
	assert.Equal(t, "1-aaaaaaaa-1", switched.PreviousReleaseID)
}

func TestAFailedHealthCheckRollsBackToTheLastGoodRelease(t *testing.T) {
	d := newTarget(t, "rollback")
	require.Equal(t, 0, d.deploy("rb-1", spec{release: "1-aaaaaaaa-1", files: page("v1", true), health: true}).exit)
	require.Equal(t, 0, d.deploy("rb-2", spec{release: "2-bbbbbbbb-1", files: page("v2", true), health: true}).exit)

	bad := d.deploy("rollback", spec{release: "3-cccccccc-1", files: page("v3", false), health: true})
	assert.Equal(t, deployrelease.ExitRolledBack, bad.exit, bad.output)
	assert.Contains(t, bad.output, "DEPLOY_ROLLED_BACK")
	assert.Equal(t, "releases/2-bbbbbbbb-1", d.current())
	assert.Equal(t, "v2", d.served("index.html"), "the service answers with the old release again")
	assert.Equal(t, "2-bbbbbbbb-1", d.readRemote(".dhow/verified"))
	assert.Equal(t, []string{"release_started", "uploaded", "switched", "health", "rolled_back", "rolled_back/group"}, bad.kinds())
	for _, e := range bad.events {
		if e.Event == deployrelease.EventHealth {
			require.NotNil(t, e.Health)
			assert.False(t, e.Health.Passed)
			assert.Equal(t, 404, e.Health.LastStatus)
			assert.Equal(t, deployrelease.ErrKindStatus, e.Health.LastError)
		}
	}
	assert.False(t, d.exists(".dhow/lock"))
}

func TestAFirstDeployWhoseCheckFailsLeavesCurrentAloneAndFails(t *testing.T) {
	d := newTarget(t, "firstfail")
	out := d.deploy("first-failure", spec{release: "1-aaaaaaaa-1", files: page("v1", false), health: true})
	assert.Equal(t, deployrelease.ExitHealthFailed, out.exit, out.output)
	assert.Equal(t, "releases/1-aaaaaaaa-1", d.current(), "there is no earlier release to go back to")
	assert.Equal(t, []string{"release_started", "uploaded", "switched", "health", "failed", "failed/group"}, out.kinds())
}

func TestKeepReleasesLeavesExactlyThatManyAfterSixDeploys(t *testing.T) {
	d := newTarget(t, "keep")
	for i := 1; i <= 6; i++ {
		out := d.deploy(fmt.Sprintf("keep-%d", i), spec{release: fmt.Sprintf("%d-aaaaaaaa-1", i), files: page(fmt.Sprintf("v%d", i), true), health: true, keep: 3})
		require.Equal(t, 0, out.exit, out.output)
	}
	assert.ElementsMatch(t, []string{"4-aaaaaaaa-1", "5-aaaaaaaa-1", "6-aaaaaaaa-1"}, d.releases())
	assert.Equal(t, "releases/6-aaaaaaaa-1", d.current())
	assert.Equal(t, "v6", d.served("index.html"))
}

func TestAWrongHostKeyIsRefusedAndNothingIsWritten(t *testing.T) {
	d := newTarget(t, "hostkey")
	out := d.deploy("host-key-mismatch", spec{release: "1-aaaaaaaa-1", files: page("v1", true), health: true, knownHosts: otherHostKey(t, d.s)})
	assert.Equal(t, deployrelease.ExitHostKeyMismatch, out.exit, out.output)
	assert.Contains(t, out.output, "HOST_KEY_MISMATCH")
	assert.False(t, d.exists("releases"), "no directory was made")
	assert.False(t, d.exists(".dhow"))
	assert.Empty(t, d.current())
	assert.Equal(t, []string{"release_started", "failed", "failed/group"}, out.kinds())
}

func TestAnUnsafeArchiveIsRefusedBeforeTheServerIsTouched(t *testing.T) {
	d := newTarget(t, "unsafe")
	path := filepath.Join(t.TempDir(), "evil.tgz")
	f, err := os.Create(path)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "../../escape.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1}))
	_, err = tw.Write([]byte("x"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())

	out := d.deploy("unsafe-archive", spec{release: "1-aaaaaaaa-1", archive: path, health: true})
	assert.Equal(t, deployrelease.ExitUnsafeArtifact, out.exit, out.output)
	assert.Contains(t, out.output, "ARTIFACT_UNSAFE")
	assert.False(t, d.exists("releases"))
	assert.False(t, d.exists("escape.txt"))
}

func TestHooksRunInsideTheReleaseAndAFailedReloadRollsBack(t *testing.T) {
	d := newTarget(t, "hooks")
	require.Equal(t, 0, d.deploy("hooks-1", spec{release: "1-aaaaaaaa-1", files: page("v1", true), health: true}).exit)

	ok := d.deploy("hooks-ok", spec{
		release: "2-bbbbbbbb-1", files: page("v2", true), health: true,
		preSwitch: `echo "$DHOW_RELEASE_ID before" > pre.txt`, postSwitch: `echo "$DHOW_PREVIOUS_RELEASE after" > post.txt`,
	})
	require.Equal(t, 0, ok.exit, ok.output)
	assert.Equal(t, "2-bbbbbbbb-1 before", d.readRemote("releases/2-bbbbbbbb-1/pre.txt"))
	assert.Equal(t, "1-aaaaaaaa-1 after", d.readRemote("releases/2-bbbbbbbb-1/post.txt"))

	failed := d.deploy("hooks-reload-failed", spec{
		release: "3-cccccccc-1", files: page("v3", true), health: true, reload: `test "$DHOW_RELEASE_ID" != 3-cccccccc-1`,
	})
	assert.Equal(t, deployrelease.ExitRolledBack, failed.exit, failed.output)
	assert.Equal(t, "releases/2-bbbbbbbb-1", d.current())
	assert.Equal(t, "v2", d.served("index.html"))
	rolled := failed.events[len(failed.events)-1]
	assert.Equal(t, deployrelease.EventRolledBack, rolled.Event)
	assert.Equal(t, deployrelease.CauseReload, rolled.Reason)

	pre := d.deploy("hooks-pre-failed", spec{release: "4-dddddddd-1", files: page("v4", true), health: true, preSwitch: "exit 4"})
	assert.NotEqual(t, 0, pre.exit)
	assert.Equal(t, "releases/2-bbbbbbbb-1", d.current(), "a failed pre-switch hook never switches")
	assert.False(t, d.exists("releases/4-dddddddd-1"))
}

func TestAHookTextIsRunExactlyAsWrittenWhateverItContains(t *testing.T) {
	d := newTarget(t, "quote")
	hook := `printf '%s' "it's \"quoted\" $(echo sub) ; end" > out.txt`
	out := d.deploy("quote", spec{release: "1-aaaaaaaa-1", files: page("v1", true), health: true, postSwitch: hook})
	require.Equal(t, 0, out.exit, out.output)
	assert.Equal(t, `it's "quoted" sub ; end`, d.readRemote("releases/1-aaaaaaaa-1/out.txt"))
}

func TestSharedPathsAreLinkedIntoEachRelease(t *testing.T) {
	d := newTarget(t, "shared")
	out := d.deploy("shared", spec{release: "1-aaaaaaaa-1", files: page("v1", true), health: true, shared: []string{"uploads", ".env"}})
	require.Equal(t, 0, out.exit, out.output)
	c := d.connect(d.s.knownHostsLine())
	defer func() { _ = c.Close() }()
	link, err := c.Readlink(d.path + "/releases/1-aaaaaaaa-1/uploads")
	require.NoError(t, err)
	assert.Equal(t, "../../shared/uploads", link)
	assert.True(t, d.exists("shared"))
}

func TestACurrentThatIsARealDirectoryIsLeftAlone(t *testing.T) {
	d := newTarget(t, "realdir")
	c := d.connect(d.s.knownHostsLine())
	require.NoError(t, c.MkdirAll(d.path+"/current", 0o755))
	w, err := c.Create(d.path+"/current/old.txt", 0o644)
	require.NoError(t, err)
	_, err = w.Write([]byte("old"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, c.Close())

	out := d.deploy("current-dir", spec{release: "1-aaaaaaaa-1", files: page("v1", true), health: true})
	assert.Equal(t, deployrelease.ExitCurrentNotLink, out.exit, out.output)
	assert.Contains(t, out.output, "CURRENT_NOT_A_SYMLINK")
	assert.Equal(t, "old", d.readRemote("current/old.txt"))
	assert.False(t, d.exists("releases/1-aaaaaaaa-1"))
}

func TestATermSignalAfterTheSwitchRollsBackToTheLastGoodRelease(t *testing.T) {
	d := newTarget(t, "term")
	require.Equal(t, 0, d.deploy("term-1", spec{release: "1-aaaaaaaa-1", files: page("v1", true), health: true}).exit)

	out := d.deploy("term-canceled", spec{release: "2-bbbbbbbb-1", files: page("v2", false), health: true, stopAt: "releases/2-bbbbbbbb-1", retries: 30})
	assert.Equal(t, deployrelease.ExitRolledBack, out.exit, out.output)
	assert.Equal(t, "releases/1-aaaaaaaa-1", d.current())
	assert.Equal(t, "v1", d.served("index.html"))
	last := out.events[len(out.events)-1]
	assert.Equal(t, deployrelease.EventRolledBack, last.Event)
	assert.Equal(t, deployrelease.CauseCanceled, last.Reason)
	assert.False(t, d.exists(".dhow/lock"))
}
