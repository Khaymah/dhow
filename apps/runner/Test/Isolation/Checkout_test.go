//go:build isolation

package isolation_test

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	checkout "github.com/Khaymah/dhow/apps/runner/internal/Checkout"
	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	egress "github.com/Khaymah/dhow/apps/runner/internal/Egress"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	envGitArchive = "DHOW_ISOLATION_GIT_ARCHIVE"
	envSSHD       = "DHOW_ISOLATION_SSHD"
	gitHost       = "git.example.com"
)

type gitServer struct {
	root      string
	repo      string
	sha       string
	port      int
	clientKey []byte
	hostLine  string
	otherLine string
	url       string
}

func command(ctx context.Context, t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.com",
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s %v: %s", name, args, out)
	return strings.TrimSpace(string(out))
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	return port
}

func startGitServer(ctx context.Context, t *testing.T, sshd string) *gitServer {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0o755))
	s := &gitServer{root: root, repo: filepath.Join(root, "srv", "app.git"), port: freePort(t)}
	seed := filepath.Join(root, "seed")
	command(ctx, t, root, "git", "init", "--quiet", "--bare", "-b", "main", s.repo)
	command(ctx, t, root, "git", "init", "--quiet", "-b", "main", seed)
	require.NoError(t, os.WriteFile(filepath.Join(seed, "README"), []byte("checked out\n"), 0o644))
	command(ctx, t, seed, "git", "add", "--", "README")
	command(ctx, t, seed, "git", "commit", "--quiet", "-m", "first")
	command(ctx, t, seed, "git", "push", "--quiet", s.repo, "main")
	s.sha = command(ctx, t, seed, "git", "rev-parse", "HEAD")
	for _, k := range []string{"client", "host", "other"} {
		command(ctx, t, root, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", k, "-f", filepath.Join(root, k))
	}
	var err error
	s.clientKey, err = os.ReadFile(filepath.Join(root, "client"))
	require.NoError(t, err)
	pub := func(name string) string {
		raw, err := os.ReadFile(filepath.Join(root, name+".pub"))
		require.NoError(t, err)
		f := strings.Fields(string(raw))
		return fmt.Sprintf("[%s]:%d %s %s", gitHost, s.port, f[0], f[1])
	}
	s.hostLine, s.otherLine = pub("host"), pub("other")
	authorized, err := os.ReadFile(filepath.Join(root, "client.pub"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "authorized_keys"), authorized, 0o600))
	cfg := strings.Join([]string{
		"ListenAddress 127.0.0.1:" + strconv.Itoa(s.port), "HostKey " + filepath.Join(root, "host"),
		"AuthorizedKeysFile " + filepath.Join(root, "authorized_keys"), "PidFile none", "StrictModes no", "UsePAM no",
		"PasswordAuthentication no", "KbdInteractiveAuthentication no", "",
	}, "\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, "sshd_config"), []byte(cfg), 0o600))
	daemon := exec.CommandContext(ctx, sshd, "-D", "-e", "-f", filepath.Join(root, "sshd_config"))
	daemon.Env = []string{"PATH=" + os.Getenv("PATH")}
	require.NoError(t, daemon.Start())
	t.Cleanup(func() { _ = daemon.Process.Kill(); _ = daemon.Wait() })
	deadline := time.Now().Add(10 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(s.port), time.Second)
		if err == nil {
			_ = c.Close()
			break
		}
		require.True(t, time.Now().Before(deadline), "sshd did not listen: %v", err)
		time.Sleep(50 * time.Millisecond)
	}
	u, err := user.Current()
	require.NoError(t, err)
	s.url = fmt.Sprintf("ssh://%s@%s:%d%s", u.Username, gitHost, s.port, s.repo)
	return s
}

func loadGitImage(ctx context.Context, t *testing.T, archive string) string {
	t.Helper()
	out, err := shared.driver.Podman(ctx, "load", "--quiet", "--input", archive)
	require.NoError(t, err)
	for _, l := range strings.Split(string(out), "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(l), "Loaded image: "); ok {
			return name
		}
	}
	t.Fatalf("podman load named no image: %s", out)
	return ""
}

func (a *attempt) checkoutSandbox(ctx context.Context, t *testing.T, img string, port int) sandbox.Sandbox {
	t.Helper()
	allow := []string{gitHost + ":" + strconv.Itoa(port)}
	box, err := shared.driver.Prepare(ctx, sandbox.PrepareSpec{
		RunnerID: runnerID, AttemptID: a.id, Seq: a.seq, Kind: sandbox.KindCheckout, Image: img,
		Isolation: runnerv1.Isolation_ISOLATION_CONTAINER, Resources: defaultResources(), Dir: a.dir,
		DiskMode: config.DiskModeTmpfs, Egress: allow, Workspace: a.ws,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = box.Destroy(context.Background()) })
	a.seq++
	eb, ok := box.(sandbox.Egress)
	require.True(t, ok)
	p, err := egress.New(egress.Options{
		Allow: allow, AllowPrivate: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")},
		Resolver: &testResolver{answers: map[string][]string{gitHost: {"127.0.0.1"}}},
	})
	require.NoError(t, err)
	sctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = p.Serve(sctx, eb.EgressListener()) }()
	require.NoError(t, box.Start(ctx))
	return box
}

func checkoutStep(s *gitServer, knownHost string) sandbox.StepSpec {
	return sandbox.StepSpec{
		Index: sandbox.CheckoutStepIndex, Argv: []string{sandbox.AgentPath, checkout.Command},
		Env: map[string]string{
			checkout.EnvURL: s.url, checkout.EnvRef: "refs/heads/main", checkout.EnvSha: s.sha, checkout.EnvDepth: "1",
			checkout.EnvCredential: "1",
		},
		Files: map[string][]byte{checkout.CredentialFile: s.clientKey, checkout.KnownHostsFile: []byte(knownHost + "\n")},
	}
}

func TestARealCheckoutOverSSHInTheGitImage(t *testing.T) {
	archive, sshd := os.Getenv(envGitArchive), os.Getenv(envSSHD)
	if archive == "" || sshd == "" {
		t.Skipf("set %s to a docker save of deploy/dhow-git and %s to an sshd binary to run the checkout end to end", envGitArchive, envSSHD)
	}
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	img := loadGitImage(ctx, t, archive)
	s := startGitServer(ctx, t, sshd)

	a := newAttempt(t)
	co := a.checkoutSandbox(ctx, t, img, s.port)
	out := runCtx(ctx, t, co, checkoutStep(s, s.hostLine))
	require.Equal(t, int32(checkout.ExitOK), out.result.ExitCode, out.stderr)
	assert.NotContains(t, out.stdout+out.stderr, "BEGIN OPENSSH PRIVATE KEY")
	destroy(t, co)
	user := a.sandbox(t, sandbox.KindJob, image)
	got := run(t, user, busybox+" cat /workspace/README && "+busybox+" test -d /workspace/.git && echo has-git\n", nil)
	assert.Contains(t, got.stdout, "checked out")
	assert.Contains(t, got.stdout, "has-git")
	destroy(t, user)

	b := newAttempt(t)
	mismatch := b.checkoutSandbox(ctx, t, img, s.port)
	bad := runCtx(ctx, t, mismatch, checkoutStep(s, s.otherLine))
	assert.Equal(t, int32(checkout.ExitHostKeyMismatch), bad.result.ExitCode, bad.stderr)
	assert.Contains(t, bad.stderr, "HOST_KEY_MISMATCH")
	destroy(t, mismatch)
}
