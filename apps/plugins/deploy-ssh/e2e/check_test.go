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

const checkRelease = "99-cccccccc-1"

func (g *group) check(scenario string, r rolling) result {
	g.t.Helper()
	work := g.t.TempDir()
	key := filepath.Join(work, jobspec.TargetKeyFile(0))
	require.NoError(g.t, os.WriteFile(key, g.hosts[0].clientKey, 0o400))
	report := filepath.Join(work, "report")
	env := append(g.env(r, key, "", "", report), "DHOW_INPUT_MODE=check")
	if scenario != "" {
		scenario = "check-" + scenario
	}
	return runPlugin(g.t, g.plugin, env, report, scenario, nil)
}

func (r result) checks() map[int]deployrelease.Event {
	out := map[int]deployrelease.Event{}
	for i := range hostCount {
		for _, e := range r.events {
			if e.Event == deployrelease.EventChecked && e.HostID == groupHostID(i) {
				out[i] = e
			}
		}
	}
	return out
}

func (g *group) tree(i int) []string {
	return append(g.releases(i), g.current(i), g.target(i).readRemote(".dhow/verified"), g.target(i).readRemote(".dhow/state.json"))
}

func TestACheckOfThreeServersTellsTheWrongKeyAndTheFullDiskApartAndChangesNothing(t *testing.T) {
	g := newGroup(t, lowDiskName)
	g.deployAll(v1, 1)
	before := [][]string{g.tree(0), g.tree(1), g.tree(2)}

	out := g.check("three-servers", rolling{release: checkRelease, known: map[int]string{1: otherHostKey(t, g.hosts[1])}})

	assert.Equal(t, deployrelease.ExitCheckFailed, out.exit, out.output)
	assert.Contains(t, out.output, "TARGET_CHECK_FAILED")
	got := out.checks()
	require.Len(t, got, 3)
	assert.Equal(t, deployrelease.CheckOK, got[0].Reason)
	assert.Equal(t, v1, got[0].CurrentReleaseID)
	assert.Equal(t, v1, got[0].VerifiedReleaseID)
	assert.Equal(t, deployrelease.CheckHostKey, got[1].Reason)
	assert.Empty(t, got[1].CurrentReleaseID, "nothing was read from a server whose key does not match")
	assert.Equal(t, deployrelease.CheckDiskLow, got[2].Reason)
	assert.Positive(t, got[2].FreeBytes)
	assert.Less(t, got[2].FreeBytes, int64(deployrelease.CheckMinFree))
	assert.Equal(t, v1, got[2].CurrentReleaseID, "the facts of a server with a full disk are still read")
	assert.Equal(t, before, [][]string{g.tree(0), g.tree(1), g.tree(2)}, "a check changes nothing on any server")
	g.requireNoLocks()
	assert.Empty(t, out.groupKinds(), "a check sends no group event")
}

func TestACheckSeesAServerThatIsBetweenTwoReleasesAfterAKilledDeploy(t *testing.T) {
	g := newGroup(t, "checkkill")
	g.deployAll(v1, 1)
	g.provideShared("healthz", 0, 1, 2)
	killed := g.deploy("", rolling{
		release: v2, files: map[string]string{"index.html": "v2"}, shared: []string{"healthz"}, retries: 40, stopAtHost: 0,
		stopAt: "releases/" + v2, hard: true,
	})
	require.NotEqual(t, 0, killed.exit, "the plugin was killed after it switched the first server")

	held := g.check("killed-locked", rolling{release: checkRelease})
	assert.Equal(t, deployrelease.ExitCheckFailed, held.exit, held.output)
	for i, e := range held.checks() {
		assert.Equal(t, deployrelease.CheckLocked, e.Reason, "the dead plugin left the lock of host %d", i+1)
		assert.True(t, e.Locked)
	}
	assert.Equal(t, v2, held.checks()[0].CurrentReleaseID, "the facts are read even when the lock is held")
	assert.Equal(t, v1, held.checks()[0].VerifiedReleaseID)

	for i := range 3 {
		c := g.connect(i, g.hosts[i].knownHostsLine())
		require.NoError(t, c.RemoveAll(g.path+"/.dhow/lock"))
		require.NoError(t, c.Close())
	}
	out := g.check("killed", rolling{release: checkRelease})

	got := out.checks()
	assert.Equal(t, deployrelease.CheckDiverged, got[0].Reason, "current is the new release but only the old one passed its health check")
	assert.Equal(t, v2, got[0].CurrentReleaseID)
	assert.Equal(t, v1, got[0].VerifiedReleaseID)
	assert.Equal(t, deployrelease.CheckOK, got[1].Reason)
	assert.Equal(t, v1, got[1].CurrentReleaseID)
	assert.Equal(t, 0, out.exit, out.output, "a diverged server is not a failure of the check: a deployment copes with it")
}

func TestADeployRefusesADamagedStateFileOnARealServer(t *testing.T) {
	g := newGroup(t, "checkcorrupt")
	g.deployAll(v1, 1)
	c := g.connect(1, g.hosts[1].knownHostsLine())
	require.NoError(t, deployrelease.WriteFile(c, g.path+"/.dhow/state.json", []byte("{broken"), 0o644))
	require.NoError(t, c.Close())

	checked := g.check("corrupt", rolling{release: checkRelease})
	assert.Equal(t, deployrelease.CheckStateCorrupt, checked.checks()[1].Reason)

	out := g.deploy("", rolling{release: v2, files: page("v2", true)})
	assert.Equal(t, deployrelease.ExitStateCorrupt, out.exit, out.output)
	assert.Contains(t, out.output, "TARGET_STATE_CORRUPT")
	g.requireOn(v1, 0, 1, 2)
	for i := range 3 {
		assert.ElementsMatch(t, []string{v1}, g.releases(i), "nothing was uploaded to host %d", i+1)
	}
	g.requireNoLocks()
}
