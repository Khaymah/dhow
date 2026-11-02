//go:build isolation

package isolation_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	podman "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Podman"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const annotatedImage = "localhost/dhow-isolation:annotated"

func TestALeftoverProcessDiesBeforeTheNextStepGetsItsSecret(t *testing.T) {
	box, _ := start(t, defaultResources())
	secret := "dhow-secret-" + token()
	thief := busybox + " sh -c 'while :; do " + busybox + " cat /run/dhow/secrets/* /proc/[0-9]*/environ >> /workspace/.loot 2>/dev/null; " + busybox + " sleep 0.05; done'"
	first := run(t, box, "( "+busybox+" setsid "+thief+" </dev/null >/dev/null 2>&1 & )\n", nil)
	require.Zero(t, first.result.ExitCode)

	ctx := context.Background()
	second := runCtx(ctx, t, box, sandbox.StepSpec{
		Index:  1,
		Script: probe + " alone\n" + busybox + " sleep 1\n",
		Env:    map[string]string{"DEPLOY_TOKEN": secret},
		Files:  map[string][]byte{"deploy-key": []byte(secret)},
	})
	assert.Empty(t, escapeLines(second), "no process from the first step is alive when the second starts")
	assert.Zero(t, second.result.ExitCode)

	third := runCtx(ctx, t, box, sandbox.StepSpec{
		Index:  2,
		Script: busybox + " touch /workspace/.loot\n! " + busybox + " grep -q -F -- \"$WANT\" /workspace/.loot\n",
		Env:    map[string]string{"WANT": secret},
	})
	assert.Zero(t, third.result.ExitCode, "the leftover loop never saw the second step's secret")
}

func TestNothingUnderTheStateDirIsOpenToOtherUsers(t *testing.T) {
	start(t, defaultResources())
	storage := filepath.Join(shared.stateDir, "storage")
	var open []string
	err := filepath.WalkDir(shared.stateDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				return fs.SkipDir
			}
			return err
		}
		if path == storage {
			return fs.SkipDir
		}
		info, err := d.Info()
		if err != nil || info.Mode()&fs.ModeSymlink != 0 {
			return nil
		}
		perm := info.Mode().Perm()
		if perm&0o006 != 0 && othersReach(shared.stateDir, path) {
			open = append(open, path+" "+info.Mode().String())
		}
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, open, "other host users can read or write these")
}

func TestImageAnnotationsCannotReachTheRuntime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Containerfile"), []byte("FROM "+image+"\n"), 0o644))
	_, err := shared.driver.Podman(ctx, "build", "--pull=never", "--network=none",
		"--annotation", "run.oci.keep_original_groups=1", "--tag", annotatedImage,
		"--file", filepath.Join(dir, "Containerfile"), "--", dir)
	require.NoError(t, err)

	id := token()
	attempt := filepath.Join(config.Config{StateDir: shared.stateDir}.AttemptsDir(), id)
	require.NoError(t, config.EnsurePrivateDir(attempt))
	box, err := shared.driver.Prepare(ctx, sandbox.PrepareSpec{
		RunnerID: runnerID, AttemptID: id, Image: annotatedImage, Isolation: runnerv1.Isolation_ISOLATION_CONTAINER,
		Resources: defaultResources(), Dir: attempt, DiskMode: config.DiskModeTmpfs,
		Workspace: workspaceFor(ctx, t, id, attempt, defaultResources()),
	})
	if err != nil {
		assert.ErrorIs(t, err, podman.ErrPolicyMismatch, "a refused annotated image is refused by verify")
		return
	}
	t.Cleanup(func() { _ = box.Destroy(context.Background()) })
	out, err := shared.driver.Podman(ctx, "inspect", "--type", "container", "--format", "{{json .Config.Annotations}}", "--", "dhow-"+id)
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(out), "run.oci.keep_original_groups"), "the image annotation reached the container")
}

func othersReach(root, path string) bool {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil || info.Mode().Perm()&0o001 == 0 {
			return false
		}
		if dir == root || dir == filepath.Dir(dir) {
			return true
		}
	}
}

func TestSignalsFromTheJobDoNotStopTheAgent(t *testing.T) {
	box, _ := start(t, defaultResources())
	first := run(t, box, busybox+" kill -TERM 1; "+busybox+" kill -QUIT 1; "+busybox+" kill -USR1 1; "+busybox+" kill -INT 1; "+busybox+" sleep 1\n", nil)
	require.Zero(t, first.result.ExitCode)
	second := run(t, box, busybox+" true\n", nil)
	assert.Zero(t, second.result.ExitCode, "the agent still runs steps after the job signalled it")
}
