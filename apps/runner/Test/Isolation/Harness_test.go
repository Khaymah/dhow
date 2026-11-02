//go:build isolation

package isolation_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	podman "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Podman"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	envBin      = "DHOW_ISOLATION_BIN"
	envBusybox  = "DHOW_ISOLATION_BUSYBOX"
	image       = "localhost/dhow-isolation:test"
	runnerID    = "isolation"
	probe       = "/probe"
	busybox     = "/bin/busybox"
	stepTimeout = 3 * time.Minute
)

type suite struct {
	driver   *podman.Driver
	stateDir string
	canary   string
}

var shared suite

func TestMain(m *testing.M) {
	code, err := setUp()
	if err != nil {
		fmt.Fprintln(os.Stderr, "isolation suite:", err)
		os.Exit(1)
	}
	os.Exit(code(m))
}

func setUp() (func(*testing.M) int, error) {
	bin := os.Getenv(envBin)
	if bin == "" {
		return nil, fmt.Errorf("%s must name the directory with dhow-runner and escape-probe (run make runner-isolation)", envBin)
	}
	box := os.Getenv(envBusybox)
	if box == "" {
		box = "/bin/busybox"
	}
	dir, err := os.MkdirTemp("", "dhow-isolation-")
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	d, err := podman.New(ctx, podman.Options{
		Config: config.Config{StateDir: dir}, AgentPath: filepath.Join(bin, "dhow-runner"), SkipPull: true,
	})
	if err != nil {
		return nil, err
	}
	canary := token()
	if err := os.WriteFile(filepath.Join(dir, "identity-canary"), []byte(canary), config.PrivateFileMode); err != nil {
		return nil, err
	}
	if err := os.Setenv("DHOW_RUNNER_CANARY", canary); err != nil {
		return nil, err
	}
	if err := buildImage(ctx, d, bin, box); err != nil {
		return nil, err
	}
	shared = suite{driver: d, stateDir: dir, canary: canary}
	return func(m *testing.M) int {
		code := m.Run()
		_ = d.Cleanup(ctx, runnerID)
		_, _ = d.Podman(ctx, "unshare", "rm", "-rf", "--", dir)
		_ = os.RemoveAll(dir)
		return code
	}, nil
}

func buildImage(ctx context.Context, d *podman.Driver, bin, box string) error {
	dir, err := os.MkdirTemp("", "dhow-isolation-image-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	for name, src := range map[string]string{"busybox": box, "escape-probe": filepath.Join(bin, "escape-probe")} {
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o755); err != nil {
			return err
		}
	}
	containerfile := strings.Join([]string{
		"FROM scratch",
		"COPY --chmod=0755 busybox /bin/busybox",
		"COPY --chmod=0755 busybox /bin/sh",
		"COPY --chmod=0755 escape-probe /probe",
		"COPY --chmod=4755 escape-probe /probe-setuid",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "Containerfile"), []byte(containerfile), 0o644); err != nil {
		return err
	}
	_, err = d.Podman(ctx, "build", "--pull=never", "--network=none", "--tag", image, "--file", filepath.Join(dir, "Containerfile"), "--", dir)
	return err
}

func token() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

func defaultResources() sandbox.Resources {
	return sandbox.Resources{CPUMillis: 1000, MemoryBytes: 256 << 20, Pids: 128, DiskBytes: 64 << 20}
}

func start(t *testing.T, r sandbox.Resources) (sandbox.Sandbox, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	id := token()
	dir := filepath.Join(config.Config{StateDir: shared.stateDir}.AttemptsDir(), id)
	require.NoError(t, config.EnsurePrivateDir(dir))
	box, err := shared.driver.Prepare(ctx, sandbox.PrepareSpec{
		RunnerID: runnerID, AttemptID: id, Image: image, Isolation: runnerv1.Isolation_ISOLATION_CONTAINER,
		Resources: r, Dir: dir, DiskMode: config.DiskModeTmpfs, Workspace: workspaceFor(ctx, t, id, dir, r),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = box.Destroy(context.Background()) })
	require.NoError(t, box.Start(ctx))
	return box, dir
}

func workspaceFor(ctx context.Context, t *testing.T, id, dir string, r sandbox.Resources) sandbox.Workspace {
	t.Helper()
	ws, err := shared.driver.Workspace(ctx, sandbox.WorkspaceSpec{
		RunnerID: runnerID, AttemptID: id, Dir: dir, DiskMode: config.DiskModeTmpfs, DiskBytes: r.DiskBytes,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Destroy(context.Background()) })
	return ws
}

type output struct {
	result sandbox.StepResult
	stdout string
	stderr string
}

func run(t *testing.T, box sandbox.Sandbox, script string, env map[string]string) output {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	return runCtx(ctx, t, box, sandbox.StepSpec{Script: script, Env: env})
}

func runCtx(ctx context.Context, t *testing.T, box sandbox.Sandbox, step sandbox.StepSpec) output {
	t.Helper()
	var stdout, stderr bytes.Buffer
	step.Stdout, step.Stderr = &stdout, &stderr
	res, err := box.Exec(ctx, step)
	require.NoError(t, err)
	t.Logf("exit %d signal %d\nstdout:\n%s\nstderr:\n%s", res.ExitCode, res.Signal, stdout.String(), stderr.String())
	return output{result: res, stdout: stdout.String(), stderr: stderr.String()}
}
