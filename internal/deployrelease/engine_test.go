package deployrelease_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

func release(t *testing.T, r *localRemote, id, body string, doer deployrelease.Doer, edit func(*deployrelease.Config)) outcome {
	t.Helper()
	path, digest := makeArchive(t, map[string]string{"index.html": body, "assets/app.js": "js " + body})
	cfg := baseConfig(path, digest, id)
	if edit != nil {
		edit(&cfg)
	}
	return deploy(t, r, cfg, doer)
}

func TestAHealthyDeployPutsTheReleaseUnderCurrentAndKeepsTheOldOne(t *testing.T) {
	r := newLocalRemote(t)
	first := release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, nil)
	require.Nil(t, first.failure)
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.currentTarget())
	assert.Equal(t, []string{"release_started", "uploaded", "switched", "health", "verified", "verified/group", "pruned"}, first.events.kinds())
	assert.Equal(t, "v1", r.read("/srv/app/releases/1-aaaaaaaa-1/index.html"))
	assert.Equal(t, "js v1", r.read("/srv/app/current/assets/app.js"))

	second := release(t, r, "2-bbbbbbbb-1", "v2", &probe{status: healthy}, nil)
	require.Nil(t, second.failure)
	assert.Equal(t, "releases/2-bbbbbbbb-1", r.currentTarget())
	assert.Equal(t, "v2", r.read("/srv/app/current/index.html"))
	assert.Equal(t, "v1", r.read("/srv/app/releases/1-aaaaaaaa-1/index.html"), "the old release stays")
	assert.Equal(t, "2-bbbbbbbb-1\n", r.read("/srv/app/.dhow/verified"))
	switched, ok := second.events.find(deployrelease.EventSwitched)
	require.True(t, ok)
	assert.Equal(t, "1-aaaaaaaa-1", switched.PreviousReleaseID, "the first release was verified by Dhow, so the server knows it")
	assert.NoDirExists(t, r.path("/srv/app/.dhow/lock"), "the lock is released")
}

func TestAFailedHealthCheckPutsTheLastGoodReleaseBack(t *testing.T) {
	r := newLocalRemote(t)
	require.Nil(t, release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, nil).failure)
	require.Nil(t, release(t, r, "2-bbbbbbbb-1", "v2", &probe{status: healthy}, nil).failure)

	bad := &probe{status: unhealthy}
	out := release(t, r, "3-cccccccc-1", "v3", bad, nil)
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitRolledBack, out.failure.Exit)
	assert.Equal(t, deployrelease.CauseHealth, out.failure.Reason)
	assert.Equal(t, "releases/2-bbbbbbbb-1", r.currentTarget())
	assert.Equal(t, "v2", r.read("/srv/app/current/index.html"))
	assert.Equal(t, "2-bbbbbbbb-1\n", r.read("/srv/app/.dhow/verified"))
	assert.Equal(t, 5, bad.calls, "the check uses all its retries before it gives up")
	assert.Equal(t, []string{"release_started", "uploaded", "switched", "health", "rolled_back", "rolled_back/group"}, out.events.kinds())
	health, _ := out.events.find(deployrelease.EventHealth)
	require.NotNil(t, health.Health)
	assert.False(t, health.Health.Passed)
	assert.Equal(t, 503, health.Health.LastStatus)
	assert.Equal(t, deployrelease.ErrKindStatus, health.Health.LastError)
	rolled, _ := out.events.find(deployrelease.EventRolledBack)
	assert.Equal(t, deployrelease.CauseHealth, rolled.Reason)
	assert.NoDirExists(t, r.path("/srv/app/.dhow/lock"))
}

func TestTheHealthCheckNeedsConsecutiveSuccesses(t *testing.T) {
	r := newLocalRemote(t)
	flaky := &probe{status: func(call int, _ string) int {
		if call == 2 {
			return 500
		}
		return 200
	}}
	out := release(t, r, "1-aaaaaaaa-1", "v1", flaky, nil)
	require.Nil(t, out.failure)
	assert.Equal(t, 5, flaky.calls, "one success, a failure that resets the count, then three in a row")
	health, _ := out.events.find(deployrelease.EventHealth)
	assert.True(t, health.Health.Passed)
	assert.Equal(t, 3, health.Health.StableChecks)
	assert.Equal(t, "web-1.example.com", mustHost(t, flaky.urls[0]))
}

func mustHost(t *testing.T, raw string) string {
	t.Helper()
	var host string
	_, err := fmt.Sscanf(raw, "https://%s", &host)
	require.NoError(t, err)
	for i, c := range host {
		if c == '/' {
			return host[:i]
		}
	}
	return host
}

func TestAFirstDeployThatFailsItsCheckLeavesCurrentAloneAndFails(t *testing.T) {
	r := newLocalRemote(t)
	out := release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: unhealthy}, nil)
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitHealthFailed, out.failure.Exit)
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.currentTarget(), "there is nothing to go back to, so current stays")
	assert.Equal(t, []string{"release_started", "uploaded", "switched", "health", "failed", "failed/group"}, out.events.kinds())
	assert.Empty(t, r.read("/srv/app/.dhow/verified"))
}

func TestKeepReleasesLeavesTheCurrentAndThePreviousAndTheNewestOthers(t *testing.T) {
	r := newLocalRemote(t)
	for i := 1; i <= 6; i++ {
		id := fmt.Sprintf("%d-aaaaaaaa-1", i)
		out := release(t, r, id, fmt.Sprintf("v%d", i), &probe{status: healthy}, func(c *deployrelease.Config) { c.Keep = 3 })
		require.Nil(t, out.failure, id)
	}
	assert.Equal(t, []string{"4-aaaaaaaa-1", "5-aaaaaaaa-1", "6-aaaaaaaa-1"}, r.releaseNames())
	assert.Equal(t, "releases/6-aaaaaaaa-1", r.currentTarget())
}

func TestPruningNeverTouchesThePreviousReleaseEvenWhenItIsTheOldest(t *testing.T) {
	r := newLocalRemote(t)
	for _, id := range []string{"1-aaaaaaaa-1", "2-aaaaaaaa-1", "3-aaaaaaaa-1"} {
		require.Nil(t, release(t, r, id, id, &probe{status: healthy}, func(c *deployrelease.Config) { c.Keep = 2 }).failure)
	}
	assert.Equal(t, []string{"2-aaaaaaaa-1", "3-aaaaaaaa-1"}, r.releaseNames())
}

func TestPruningOrdersByTheDeployHistoryAndNotByNameOrTime(t *testing.T) {
	r := newLocalRemote(t)
	for _, id := range []string{"99-aaaaaaaa-1", "100-aaaaaaaa-1", "101-aaaaaaaa-1", "102-aaaaaaaa-1"} {
		require.Nil(t, release(t, r, id, id, &probe{status: healthy}, func(c *deployrelease.Config) { c.Keep = 3 }).failure)
	}
	assert.Equal(t, []string{"100-aaaaaaaa-1", "101-aaaaaaaa-1", "102-aaaaaaaa-1"}, r.releaseNames(), "100 is newer than 99 although it sorts before it")
}

func TestAReleaseThatFailedIsPrunedBeforeOnesThatWentLive(t *testing.T) {
	r := newLocalRemote(t)
	require.Nil(t, release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, nil).failure)
	require.Nil(t, release(t, r, "2-aaaaaaaa-1", "v2", &probe{status: healthy}, nil).failure)
	bad := release(t, r, "3-aaaaaaaa-1", "v3", &probe{status: unhealthy}, nil)
	require.NotNil(t, bad.failure)
	require.Nil(t, release(t, r, "4-aaaaaaaa-1", "v4", &probe{status: healthy}, func(c *deployrelease.Config) { c.Keep = 3 }).failure)
	assert.NotContains(t, r.releaseNames(), "3-aaaaaaaa-1")
	assert.ElementsMatch(t, []string{"1-aaaaaaaa-1", "2-aaaaaaaa-1", "4-aaaaaaaa-1"}, r.releaseNames())
}

func TestAnUnsafeArchiveChangesNothingOnTheServer(t *testing.T) {
	r := newLocalRemote(t)
	path := unsafeArchive(t)
	out := deploy(t, r, baseConfig(path, "", "1-aaaaaaaa-1"), &probe{status: healthy})
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitUnsafeArtifact, out.failure.Exit)
	assert.NoDirExists(t, r.path("/srv/app/releases"))
	assert.NoDirExists(t, r.path("/srv/app/.dhow"))
	assert.Empty(t, r.currentTarget())
	assert.Equal(t, []string{"release_started", "failed", "failed/group"}, out.events.kinds())
}

func TestAWrongDigestIsRefusedBeforeAnythingIsSent(t *testing.T) {
	r := newLocalRemote(t)
	path, _ := makeArchive(t, map[string]string{"index.html": "x"})
	out := deploy(t, r, baseConfig(path, strings.Repeat("0", 64), "1-aaaaaaaa-1"), &probe{status: healthy})
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitUnsafeArtifact, out.failure.Exit)
	assert.NoDirExists(t, r.path("/srv/app/releases"))
}

func TestAHostKeyMismatchStopsBeforeAnyRemoteCall(t *testing.T) {
	path, digest := makeArchive(t, map[string]string{"index.html": "x"})
	rec := &recorder{}
	e := &deployrelease.Engine{
		Config: baseConfig(path, digest, "1-aaaaaaaa-1"), Report: rec, Clock: fastClock(), Doer: &probe{status: healthy},
		Connect: func(context.Context) (deployrelease.Remote, error) {
			return nil, fmt.Errorf("dial: %w", deployrelease.ErrHostKeyMismatch)
		},
	}
	f := e.Run(context.Background())
	require.NotNil(t, f)
	assert.Equal(t, deployrelease.ExitHostKeyMismatch, f.Exit)
	assert.Equal(t, []string{"release_started", "failed", "failed/group"}, rec.kinds())
}

func TestAPlainConnectionFailureHasItsOwnExitCode(t *testing.T) {
	path, digest := makeArchive(t, map[string]string{"index.html": "x"})
	e := &deployrelease.Engine{
		Config: baseConfig(path, digest, "1-aaaaaaaa-1"), Clock: fastClock(), Doer: &probe{status: healthy},
		Connect: func(context.Context) (deployrelease.Remote, error) { return nil, errConnect },
	}
	f := e.Run(context.Background())
	require.NotNil(t, f)
	assert.Equal(t, deployrelease.ExitConnect, f.Exit)
}

func TestCurrentThatIsARealDirectoryIsNeverTouched(t *testing.T) {
	r := newLocalRemote(t)
	require.NoError(t, os.MkdirAll(r.path("/srv/app/current"), 0o755))
	require.NoError(t, os.WriteFile(r.path("/srv/app/current/old.txt"), []byte("old"), 0o644))
	out := release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, nil)
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitCurrentNotLink, out.failure.Exit)
	assert.Equal(t, "old", r.read("/srv/app/current/old.txt"))
	assert.NoDirExists(t, r.path("/srv/app/releases/1-aaaaaaaa-1"))
}

func TestALockHeldByAnotherDeploymentBlocksAndAnExpiredOneIsTaken(t *testing.T) {
	r := newLocalRemote(t)
	require.NoError(t, os.MkdirAll(r.path("/srv/app/.dhow/lock"), 0o700))
	live := fmt.Sprintf(`{"Release":"9-zzzzzzzz-1","ExpiresAt":%q}`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	require.NoError(t, os.WriteFile(r.path("/srv/app/.dhow/lock/owner"), []byte(live), 0o600))
	blocked := release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, func(c *deployrelease.Config) {})
	require.NotNil(t, blocked.failure)
	assert.Equal(t, deployrelease.ExitTargetLocked, blocked.failure.Exit)
	assert.DirExists(t, r.path("/srv/app/.dhow/lock"), "somebody else's lock is not removed")
	assert.Empty(t, r.currentTarget())

	old := fmt.Sprintf(`{"Release":"9-zzzzzzzz-1","ExpiresAt":%q}`, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339))
	require.NoError(t, os.WriteFile(r.path("/srv/app/.dhow/lock/owner"), []byte(old), 0o600))
	taken := release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, nil)
	require.Nil(t, taken.failure)
}

func TestTheServerNeedsTwiceTheSpaceOfTheUnpackedRelease(t *testing.T) {
	r := newLocalRemote(t)
	r.free = 10
	out := release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, nil)
	require.NotNil(t, out.failure)
	assert.ErrorIs(t, out.failure, deployrelease.ErrNoSpace)
	assert.NoDirExists(t, r.path("/srv/app/releases/1-aaaaaaaa-1"))
}

func TestSharedPathsAreLinkedFromTheReleaseAndAConflictFails(t *testing.T) {
	r := newLocalRemote(t)
	out := release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, func(c *deployrelease.Config) { c.Shared = []string{".env", "uploads"} })
	require.Nil(t, out.failure)
	target, err := os.Readlink(r.path("/srv/app/releases/1-aaaaaaaa-1/uploads"))
	require.NoError(t, err)
	assert.Equal(t, "../../shared/uploads", target)

	clash := release(t, r, "2-bbbbbbbb-1", "v2", &probe{status: healthy}, func(c *deployrelease.Config) { c.Shared = []string{"index.html"} })
	require.NotNil(t, clash.failure)
	assert.ErrorIs(t, clash.failure, deployrelease.ErrSharedConflict)
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.currentTarget())
	assert.NoDirExists(t, r.path("/srv/app/releases/2-bbbbbbbb-1"))
}

func TestAFailedReloadAfterTheSwitchRollsBack(t *testing.T) {
	r := newLocalRemote(t)
	require.Nil(t, release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, nil).failure)
	reloads := 0
	r.hook = func(argv []string) deployrelease.ExecResult {
		reloads++
		if reloads == 1 {
			return deployrelease.ExecResult{ExitCode: 3, Output: []byte("boom")}
		}
		return deployrelease.ExecResult{}
	}
	out := release(t, r, "2-bbbbbbbb-1", "v2", &probe{status: healthy}, func(c *deployrelease.Config) { c.ReloadCommand = "systemctl reload app" })
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitRolledBack, out.failure.Exit)
	assert.Equal(t, deployrelease.CauseReload, out.failure.Reason)
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.currentTarget())
	assert.Equal(t, 2, reloads, "the reload runs again for the old release")
	assert.NotContains(t, out.events.kinds(), "health", "no health check runs after a failed reload")
}

func TestAFailedPreSwitchHookNeverSwitches(t *testing.T) {
	r := newLocalRemote(t)
	require.Nil(t, release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, nil).failure)
	r.hook = func([]string) deployrelease.ExecResult { return deployrelease.ExecResult{ExitCode: 1} }
	out := release(t, r, "2-bbbbbbbb-1", "v2", &probe{status: healthy}, func(c *deployrelease.Config) { c.PreSwitch = "./migrate.sh" })
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.CauseHook, out.failure.Reason)
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.currentTarget())
	assert.NoDirExists(t, r.path("/srv/app/releases/2-bbbbbbbb-1"))
	assert.Equal(t, []string{"release_started", "uploaded", "failed", "failed/group"}, out.events.kinds())
}

func TestHooksTravelAsOneQuotedArgumentAndNeverGetValuesGluedIn(t *testing.T) {
	r := newLocalRemote(t)
	hook := `echo "a b" ; rm -rf / # '$(touch x)'`
	out := release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, func(c *deployrelease.Config) { c.PostSwitch = hook })
	require.Nil(t, out.failure)
	argvs := r.hookArgvs()
	require.Len(t, argvs, 1)
	argv := argvs[0]
	assert.Equal(t, hook, argv[len(argv)-1], "the hook is exactly what the admin wrote, as its own argument")
	assert.Contains(t, argv, "DHOW_RELEASE_PATH=/srv/app/releases/1-aaaaaaaa-1")
	assert.Contains(t, argv, "DHOW_RELEASE_ID=1-aaaaaaaa-1")
	line, err := deployrelease.Command(argv...)
	require.NoError(t, err)
	assert.NotContains(t, line[:len(line)-len(deployrelease.Quote(hook))], hook)
}

func TestCancelingAfterTheSwitchRollsBackWithAFreshContext(t *testing.T) {
	r := newLocalRemote(t)
	require.Nil(t, release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, nil).failure)
	path, digest := makeArchive(t, map[string]string{"index.html": "v2"})
	ctx, cancel := context.WithCancel(context.Background())
	rec := &recorder{}
	doer := &probe{status: func(int, string) int {
		cancel()
		return 200
	}}
	e := &deployrelease.Engine{
		Config: baseConfig(path, digest, "2-bbbbbbbb-1"), Doer: doer, Report: rec, Clock: fastClock(),
		Connect: func(context.Context) (deployrelease.Remote, error) { return r, nil },
	}
	f := e.Run(ctx)
	require.NotNil(t, f)
	assert.Equal(t, deployrelease.ExitRolledBack, f.Exit)
	assert.Equal(t, deployrelease.CauseCanceled, f.Reason)
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.currentTarget())
}

func TestARetryOfTheSameReleaseAfterTheSwitchResumesAtTheCheck(t *testing.T) {
	r := newLocalRemote(t)
	require.Nil(t, release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, nil).failure)
	first := release(t, r, "2-bbbbbbbb-1", "v2", &probe{status: healthy}, func(c *deployrelease.Config) { c.Health.URL = "" })
	require.Nil(t, first.failure)
	require.NoError(t, os.WriteFile(r.path("/srv/app/.dhow/verified"), []byte("1-aaaaaaaa-1\n"), 0o644))

	again := release(t, r, "2-bbbbbbbb-1", "v2", &probe{status: healthy}, nil)
	require.Nil(t, again.failure)
	assert.Equal(t, "2-bbbbbbbb-1\n", r.read("/srv/app/.dhow/verified"))
	assert.Equal(t, "releases/2-bbbbbbbb-1", r.currentTarget())
	assert.Equal(t, "v2", r.read("/srv/app/current/index.html"))
}

func TestWhenCurrentIsNotTheVerifiedReleaseTheRollbackGoesToTheVerifiedOne(t *testing.T) {
	r := newLocalRemote(t)
	require.Nil(t, release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, nil).failure)
	broken := release(t, r, "2-bbbbbbbb-1", "v2", &probe{status: healthy}, func(c *deployrelease.Config) { c.Health.URL = "" })
	require.Nil(t, broken.failure)
	require.NoError(t, os.WriteFile(r.path("/srv/app/.dhow/verified"), []byte("1-aaaaaaaa-1\n"), 0o644))

	out := release(t, r, "3-cccccccc-1", "v3", &probe{status: unhealthy}, nil)
	require.NotNil(t, out.failure)
	assert.Equal(t, deployrelease.ExitRolledBack, out.failure.Exit)
	assert.Equal(t, "releases/1-aaaaaaaa-1", r.currentTarget(), "the unknown release 2 is not trusted as a rollback target")
}

func TestAnInvalidConfigurationIsRefusedBeforeAnyWork(t *testing.T) {
	r := newLocalRemote(t)
	for name, edit := range map[string]func(*deployrelease.Config){
		"a path":       func(c *deployrelease.Config) { c.DeployPath = "/etc" },
		"a release":    func(c *deployrelease.Config) { c.ReleaseID = "../x" },
		"a health":     func(c *deployrelease.Config) { c.Health.URL = "ftp://x" },
		"a budget":     func(c *deployrelease.Config) { c.Health.Retries = 60; c.Health.Timeout = 30 * time.Second },
		"a hook":       func(c *deployrelease.Config) { c.PostSwitch = "a\nb" },
		"a shared":     func(c *deployrelease.Config) { c.Shared = []string{"../x"} },
		"a keep":       func(c *deployrelease.Config) { c.Keep = 1 },
		"stable>tries": func(c *deployrelease.Config) { c.Health.Stable = 9 },
	} {
		t.Run(name, func(t *testing.T) {
			out := release(t, r, "1-aaaaaaaa-1", "v1", &probe{status: healthy}, edit)
			require.NotNil(t, out.failure)
			assert.Equal(t, deployrelease.ExitInput, out.failure.Exit)
		})
	}
	assert.NoDirExists(t, r.path("/srv/app/releases"))
}
