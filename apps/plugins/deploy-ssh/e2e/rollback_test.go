package e2e_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	rollbackRelease = "99-nosha-1"
	failMarker      = ".fail-rollback"
)

type backTo struct {
	restore string
	files   map[string]string
	digest  string
	reload  string
}

func (g *group) rollBack(scenario string, r rolling, b backTo) result {
	g.t.Helper()
	r.release = rollbackRelease
	r.reload = b.reload
	var archivePath, digest string
	if b.files != nil {
		archivePath, digest = packFiles(g.t, b.files)
	}
	if b.digest != "" {
		digest = b.digest
	}
	work := g.t.TempDir()
	key := filepath.Join(work, jobspec.TargetKeyFile(0))
	require.NoError(g.t, os.WriteFile(key, g.hosts[0].clientKey, 0o400))
	report := filepath.Join(work, "report")
	env := append(g.env(r, key, archivePath, digest, report), "DHOW_INPUT_MODE=rollback", "DHOW_INPUT_RELEASE="+b.restore)
	if scenario != "" {
		scenario = "rollback-" + scenario
	}
	return runPlugin(g.t, g.plugin, env, report, scenario, nil)
}

func (g *group) prune(host int, release string) {
	g.t.Helper()
	c := g.connect(host, g.hosts[host].knownHostsLine())
	require.NoError(g.t, c.RemoveAll(g.path+"/releases/"+release))
	require.NoError(g.t, c.Close())
}

func (g *group) threeReleases() {
	g.t.Helper()
	g.deployAll(v1, 1)
	g.deployAll(v2, 1)
	g.deployAll(v3, 1)
	g.requireOn(v3, 0, 1, 2)
}

func TestAManualRollbackOfThreeServersSwitchesTwoAndPutsTheThirdBackFromTheArtifact(t *testing.T) {
	g := newGroup(t, "backok")
	g.threeReleases()
	g.prune(1, v1)
	require.False(t, g.exists(1, "releases/"+v1))

	out := g.rollBack("ok", rolling{}, backTo{restore: v1, files: page("v1", true)})

	require.Equal(t, 0, out.exit, out.output)
	g.requireOn(v1, 0, 1, 2)
	for i := range 3 {
		assert.Equal(t, []string{"uploaded", "switched", "health", "verified", "pruned"}, out.forHost(i), "host %d", i+1)
		assert.Equal(t, v1, g.target(i).readRemote(".dhow/verified"))
		assert.Equal(t, "ok", g.served(i, "healthz"))
		assert.Equal(t, rollbackRelease, out.events[out.indexOf("switched", i)].ReleaseID, "the report is under the release of the rollback run")
	}
	assert.True(t, g.exists(1, "releases/"+v1), "the server that pruned the release got it back")
	assert.Equal(t, v3, out.events[out.indexOf("switched", 0)].PreviousReleaseID, "the release on the server is what the report calls the previous one")
	assert.Equal(t, []string{"release_started", "batch_passed", "batch_passed", "batch_passed", "verified"}, out.groupKinds())
	assert.Equal(t, deployrelease.ModeRollback, out.events[0].Mode)
	g.requireNoLocks()
}

func TestAManualRollbackWithoutAnArtifactIsRefusedWhenAServerLostTheReleaseAndChangesNothing(t *testing.T) {
	g := newGroup(t, "backgone")
	g.threeReleases()
	g.prune(2, v1)

	out := g.rollBack("refused-pruned", rolling{}, backTo{restore: v1})

	assert.Equal(t, deployrelease.ExitFailed, out.exit, out.output)
	g.requireOn(v3, 0, 1, 2)
	for i := range 3 {
		assert.Equal(t, v3, g.target(i).readRemote(".dhow/verified"), "host %d", i+1)
	}
	assert.True(t, g.exists(0, "releases/"+v1), "a release a server still has is not removed by the refusal")
	assert.Equal(t, []string{"release_started", "failed"}, out.groupKinds())
	g.requireNoLocks()
}

func TestAManualRollbackChecksTheDigestOfTheArtifactBeforeItTouchesAnyServer(t *testing.T) {
	g := newGroup(t, "backdigest")
	g.threeReleases()
	g.prune(1, v1)

	out := g.rollBack("wrong-digest", rolling{}, backTo{
		restore: v1, files: page("v1", true), digest: "0000000000000000000000000000000000000000000000000000000000000000",
	})

	assert.Equal(t, deployrelease.ExitUnsafeArtifact, out.exit, out.output)
	g.requireOn(v3, 0, 1, 2)
	assert.False(t, g.exists(1, "releases/"+v1), "nothing of the artifact reached the server")
	g.requireNoLocks()
}

func TestAManualRollbackThatFailsOnTheSecondServerLeavesTheGroupRollbackFailedAndTriesNothingAgain(t *testing.T) {
	g := newGroup(t, "backfail")
	g.threeReleases()
	g.prune(1, v1)
	g.putFile(1, failMarker, "x")
	reload := `if [ -e "$DHOW_DEPLOY_PATH/` + failMarker + `" ]; then exit 1; fi`

	out := g.rollBack("fails-on-one-server", rolling{}, backTo{restore: v1, files: page("v1", true), reload: reload})

	assert.Equal(t, deployrelease.ExitRollbackFailed, out.exit, out.output)
	assert.Contains(t, out.output, "DEPLOY_ROLLBACK_FAILED")
	assert.Equal(t, "releases/"+v1, g.current(0), "the first server stays on the release it was rolled back to")
	assert.Equal(t, "releases/"+v1, g.current(1), "the server that failed is not put back by itself")
	assert.Equal(t, "releases/"+v3, g.current(2), "the third server was never touched")
	for i := range 3 {
		assert.Equal(t, v3, g.target(i).readRemote(".dhow/verified"), "nothing was recorded as verified on host %d", i+1)
	}
	assert.Equal(t, []string{"uploaded", "switched", "health"}, out.forHost(0))
	assert.Equal(t, []string{"uploaded", "switched", "rollback_failed"}, out.forHost(1))
	assert.Equal(t, []string{"uploaded", "skipped"}, out.forHost(2))
	assert.Equal(t, []string{"release_started", "batch_passed", "rollback_failed"}, out.groupKinds())
	assert.Equal(t, deployrelease.CauseReload, out.events[len(out.events)-1].Reason)
	g.requireNoLocks()

	c := g.connect(1, g.hosts[1].knownHostsLine())
	require.NoError(t, c.Remove(g.path+"/"+failMarker))
	require.NoError(t, c.Close())
	fixed := g.deploy("after-manual-rollback-failed", rolling{release: v4, files: page("v4", true)})
	require.Equal(t, 0, fixed.exit, fixed.output)
	g.requireOn(v4, 0, 1, 2)
}
