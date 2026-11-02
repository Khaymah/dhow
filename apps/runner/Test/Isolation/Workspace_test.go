//go:build isolation

package isolation_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const noShellImage = "localhost/dhow-isolation:noshell"

type attempt struct {
	id  string
	dir string
	ws  sandbox.Workspace
	seq int
}

func newAttempt(t *testing.T) *attempt {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	id := token()
	dir := filepath.Join(config.Config{StateDir: shared.stateDir}.AttemptsDir(), id)
	require.NoError(t, config.EnsurePrivateDir(dir))
	ws := workspaceFor(ctx, t, id, dir, defaultResources())
	require.NoError(t, ws.Check(ctx), "a fresh workspace passes its own check")
	return &attempt{id: id, dir: dir, ws: ws}
}

func (a *attempt) sandbox(t *testing.T, kind sandbox.Kind, img string) sandbox.Sandbox {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	box, err := shared.driver.Prepare(ctx, sandbox.PrepareSpec{
		RunnerID: runnerID, AttemptID: a.id, Seq: a.seq, Kind: kind, Image: img, Isolation: runnerv1.Isolation_ISOLATION_CONTAINER,
		Resources: defaultResources(), Dir: a.dir, DiskMode: config.DiskModeTmpfs, Workspace: a.ws,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = box.Destroy(context.Background()) })
	a.seq++
	require.NoError(t, box.Start(ctx))
	return box
}

func destroy(t *testing.T, box sandbox.Sandbox) {
	t.Helper()
	require.NoError(t, box.Destroy(context.Background()))
}

func TestSandboxesOfOneAttemptShareTheWorkspaceWithTheSameOwners(t *testing.T) {
	a := newAttempt(t)
	first := a.sandbox(t, sandbox.KindJob, image)
	one := run(t, first, busybox+" mkdir /workspace/src && echo one > /workspace/src/file && "+busybox+" cat /proc/self/uid_map\n", nil)
	require.Zero(t, one.result.ExitCode, one.stderr)
	destroy(t, first)

	second := a.sandbox(t, sandbox.KindPlugin, image)
	two := run(t, second, busybox+" stat -c '%u:%g' /workspace/src/file && echo two >> /workspace/src/file && "+
		busybox+" cat /workspace/src/file && "+busybox+" cat /proc/self/uid_map\n", nil)
	require.Zero(t, two.result.ExitCode, two.stderr)
	assert.Contains(t, two.stdout, "0:0\n")
	assert.Contains(t, two.stdout, "one\ntwo\n")
	mapOf := func(s string) string {
		for _, l := range strings.Split(s, "\n") {
			if f := strings.Fields(l); len(f) == 3 && f[0] == "0" {
				return strings.Join(f, " ")
			}
		}
		return ""
	}
	assert.NotEmpty(t, mapOf(one.stdout))
	assert.Equal(t, mapOf(one.stdout), mapOf(two.stdout), "every sandbox of the attempt has the same uid map")
	destroy(t, second)
}

func TestTheWorkspaceIsNosuidNodevAndItsLimitCountsOnce(t *testing.T) {
	a := newAttempt(t)
	first := a.sandbox(t, sandbox.KindJob, image)
	fill := run(t, first, busybox+" dd if=/dev/zero of=/workspace/a bs=1M count=40 && "+busybox+" cp /probe-setuid /workspace/p && "+
		busybox+" chmod 4755 /workspace/p && "+busybox+" mknod /workspace/null c 1 3 || true; "+busybox+" grep ' /workspace ' /proc/self/mountinfo\n", nil)
	require.Zero(t, fill.result.ExitCode, fill.stderr)
	assert.Contains(t, fill.stdout, "nosuid")
	assert.Contains(t, fill.stdout, "nodev")
	destroy(t, first)

	second := a.sandbox(t, sandbox.KindJob, image)
	more := run(t, second, busybox+" dd if=/dev/zero of=/workspace/b bs=1M count=40\n", nil)
	assert.NotZero(t, more.result.ExitCode)
	assert.Contains(t, more.stderr, "No space left on device", "the second sandbox shares the attempt's 64 MiB")
	suid := run(t, second, "/workspace/p capeff; "+busybox+" test ! -e /workspace/null\n", nil)
	assert.Zero(t, suid.result.ExitCode, suid.stdout)
	assert.Contains(t, suid.stdout, "blocked no capabilities")
	destroy(t, second)
}

func TestTheCheckoutCredentialNeverReachesAUserStep(t *testing.T) {
	secret := "dhow-checkout-" + token()
	a := newAttempt(t)
	co := a.sandbox(t, sandbox.KindCheckout, image)
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	got := runCtx(ctx, t, co, sandbox.StepSpec{
		Index: sandbox.CheckoutStepIndex, Argv: []string{busybox, "sh", "-c", busybox + " cat /run/dhow/secrets/git-credential > /dev/null && " +
			busybox + " test -x /run/dhow/bin/git-ssh && echo checkout > /workspace/README"},
		Files: map[string][]byte{"git-credential": []byte(secret)},
	})
	require.Zero(t, got.result.ExitCode, got.stderr)
	destroy(t, co)

	user := a.sandbox(t, sandbox.KindJob, image)
	look := run(t, user, "echo \"$REVERSED\" | "+busybox+" rev > /tmp/p; "+
		busybox+" grep -rlF -f /tmp/p /proc/[0-9]*/environ /proc/[0-9]*/cmdline /workspace /run /etc 2>/dev/null || true; "+
		busybox+" test ! -e /run/dhow/bin/git-ssh && "+busybox+" test ! -e /run/dhow/secrets/git-credential && echo clean; "+
		"/run/dhow/bin/connect git.example.com 22 </dev/null >/dev/null 2>&1 || echo no-route\n", map[string]string{"REVERSED": reverse(secret)})
	assert.NotContains(t, look.stdout, "/")
	assert.Contains(t, look.stdout, "clean")
	assert.Contains(t, look.stdout, "no-route", "a step without egress cannot reach the repository host")
	destroy(t, user)
}

func buildNoShellImage(ctx context.Context, t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join(os.Getenv(envBin), "escape-probe"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "escape-probe"), data, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Containerfile"), []byte("FROM scratch\nCOPY --chmod=0755 escape-probe /probe\n"), 0o644))
	_, err = shared.driver.Podman(ctx, "build", "--pull=never", "--network=none", "--tag", noShellImage,
		"--file", filepath.Join(dir, "Containerfile"), "--", dir)
	require.NoError(t, err)
}

func TestTheEscapeSuiteFailsAsAPluginWithoutAShell(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	buildNoShellImage(ctx, t)
	a := newAttempt(t)
	box := a.sandbox(t, sandbox.KindPlugin, noShellImage)
	hostPaths := strings.Join([]string{shared.stateDir, a.dir, filepath.Join(shared.stateDir, "identity-canary")}, ":")
	out := runCtx(ctx, t, box, sandbox.StepSpec{
		Index: 0, Argv: []string{probe, "escape"},
		Env: map[string]string{"DHOW_PROBE_HOST_PATHS": hostPaths, "DHOW_PROBE_CANARY": reverse(shared.canary)},
	})
	assert.Empty(t, escapeLines(out))
	assert.Zero(t, out.result.ExitCode, "every probe must report blocked")
	destroy(t, box)
}

func TestNothingIsLeftAfterTheAttempt(t *testing.T) {
	a := newAttempt(t)
	box := a.sandbox(t, sandbox.KindJob, image)
	out := run(t, box, "echo x > /workspace/x\n", nil)
	require.Zero(t, out.result.ExitCode)
	destroy(t, box)
	require.NoError(t, a.ws.Destroy(context.Background()))
	_, err := os.Lstat(filepath.Join(a.dir, "workspace"))
	assert.True(t, os.IsNotExist(err), "the workspace directory is gone")
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	mounts, err := shared.driver.Podman(ctx, "unshare", "cat", "/proc/self/mountinfo")
	require.NoError(t, err)
	assert.NotContains(t, string(mounts), a.id, "no workspace mount is left")
	vols, err := shared.driver.Podman(ctx, "volume", "ls", "--quiet")
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(vols)), "no volume is left")
}
