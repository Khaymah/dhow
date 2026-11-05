//go:build isolation

package isolation_test

import (
	"context"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	egress "github.com/Khaymah/dhow/apps/runner/internal/Egress"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	keyLine1 = "b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW"
	keyLine2 = "QyNTUxOQAAACD3d5eD0W6rQmJXm5nQ0m5rWQ7s5oX6lWQ0v2q2QJ0m3AAAAJgq1ZZ"
	keyText  = "-----BEGIN OPENSSH " + "PRIVATE KEY-----\n" + keyLine1 + "\n" + keyLine2 + "\n-----END OPENSSH " + "PRIVATE KEY-----\n"
	keyPath  = sandbox.SecretsPath + "/target-key"
)

func prepare(t *testing.T, dir, id string, seq int, kind sandbox.Kind, mutate func(*sandbox.PrepareSpec)) sandbox.Sandbox {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	spec := sandbox.PrepareSpec{
		RunnerID: runnerID, AttemptID: id, Seq: seq, Kind: kind, Image: image, Isolation: runnerv1.Isolation_ISOLATION_CONTAINER,
		Resources: defaultResources(), Dir: dir, DiskMode: config.DiskModeTmpfs,
	}
	spec.Workspace = workspaceForShared(ctx, t, id, dir)
	if mutate != nil {
		mutate(&spec)
	}
	box, err := shared.driver.Prepare(ctx, spec)
	require.NoError(t, err)
	t.Cleanup(func() { _ = box.Destroy(context.Background()) })
	return box
}

var workspaces sync.Map

func workspaceForShared(ctx context.Context, t *testing.T, id, dir string) sandbox.Workspace {
	t.Helper()
	if ws, ok := workspaces.Load(id); ok {
		return ws.(sandbox.Workspace)
	}
	ws := workspaceFor(ctx, t, id, dir, defaultResources())
	workspaces.Store(id, ws)
	return ws
}

func newTargetAttempt(t *testing.T) (string, string) {
	t.Helper()
	id := token()
	dir := filepath.Join(config.Config{StateDir: shared.stateDir}.AttemptsDir(), id)
	require.NoError(t, config.EnsurePrivateDir(dir))
	return id, dir
}

func TestTheKeyFileStaysInTheDeploySandboxAwayFromALiveNeighbourAndTheNextStep(t *testing.T) {
	id, dir := newTargetAttempt(t)
	otherID, otherDir := newTargetAttempt(t)
	deploy := prepare(t, dir, id, 1, sandbox.KindPlugin, nil)
	other := prepare(t, otherDir, otherID, 1, sandbox.KindJob, nil)
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	require.NoError(t, deploy.Start(ctx))
	require.NoError(t, other.Start(ctx))

	held := make(chan output, 1)
	go func() {
		held <- runCtx(ctx, t, deploy, sandbox.StepSpec{
			Index: 1, Argv: []string{busybox, "sleep", "20"}, Files: map[string][]byte{"target-key": []byte(keyText)},
			Env: map[string]string{"DHOW_TARGET_KEY_FILE": keyPath},
		})
	}()
	time.Sleep(2 * time.Second)

	hunt := strings.Join([]string{
		"for f in /proc/[0-9]*/environ /proc/[0-9]*/cmdline /proc/[0-9]*/mountinfo; do cat \"$f\" 2>/dev/null; done > /tmp/proc-dump",
		"grep -c " + keyLine1 + " /tmp/proc-dump || true",
		"grep -c DHOW_TARGET_KEY_FILE /tmp/proc-dump || true",
		"ls /proc | grep -c '^[0-9]'",
		"cat " + keyPath + " 2>&1 || true",
		"ls /run/dhow/secrets 2>&1 || true",
		"find / -xdev -name 'target-key*' 2>/dev/null || true",
		"echo done",
		"",
	}, "\n")
	out := run(t, other, hunt, nil)
	assert.NotContains(t, out.stdout, keyLine1, "another sandbox reads the key neither as a file nor through /proc")
	assert.NotContains(t, out.stdout, "BEGIN OPENSSH")
	assert.NotContains(t, out.stdout, "DHOW_TARGET_KEY_FILE", "the environment of the deploy step is not visible from the other sandbox")
	assert.NotContains(t, out.stdout, "target-key")
	assert.Contains(t, out.stdout, "done")

	cancel()
	<-held
	require.NoError(t, deploy.Destroy(context.Background()))

	next := prepare(t, dir, id, 2, sandbox.KindJob, nil)
	nctx, ncancel := context.WithTimeout(context.Background(), stepTimeout)
	defer ncancel()
	require.NoError(t, next.Start(nctx))
	after := run(t, next, hunt, nil)
	assert.NotContains(t, after.stdout, keyLine1, "the next step of the same job finds no key in the workspace or under /run/dhow")
	assert.NotContains(t, after.stdout, "target-key")
	assert.Contains(t, after.stdout, "done")
}

func TestTheDeployStepReadsTheKeyFromItsFileAndTheFileIsGoneWhenItEnds(t *testing.T) {
	id, dir := newTargetAttempt(t)
	deploy := prepare(t, dir, id, 1, sandbox.KindPlugin, nil)
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	require.NoError(t, deploy.Start(ctx))
	out := runCtx(ctx, t, deploy, sandbox.StepSpec{
		Index: 1, Argv: []string{busybox, "cat", keyPath}, Files: map[string][]byte{"target-key": []byte(keyText)},
	})
	assert.Zero(t, out.result.ExitCode)
	assert.Equal(t, keyText, out.stdout)
	after := run(t, deploy, "ls /run/dhow/secrets | wc -l\n", nil)
	assert.Equal(t, "0", strings.TrimSpace(after.stdout), "the secrets directory is wiped after the step")
}

func TestOnlyTheStepThatWasGivenAnArchiveSeesItAndItCannotChangeIt(t *testing.T) {
	id, dir := newTargetAttempt(t)
	archive := filepath.Join(dir, "archive-site.tgz")
	require.NoError(t, os.WriteFile(archive, []byte("not really a tarball, only bytes\n"), config.PrivateFileMode))
	plugin := prepare(t, dir, id, 1, sandbox.KindPlugin, func(s *sandbox.PrepareSpec) {
		s.Inputs = []sandbox.InputFile{{Name: "site.tgz", Path: archive, Digest: strings.Repeat("a", 64)}}
	})
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	require.NoError(t, plugin.Start(ctx))

	got := runCtx(ctx, t, plugin, sandbox.StepSpec{Index: 1, Argv: []string{busybox, "cat", sandbox.InputsPath + "/site.tgz"}})
	assert.Zero(t, got.result.ExitCode)
	assert.Equal(t, "not really a tarball, only bytes\n", got.stdout)

	write := run(t, plugin, "echo changed > "+sandbox.InputsPath+"/site.tgz 2>&1; echo \"write=$?\"\ntouch "+sandbox.InputsPath+"/new 2>&1; echo \"touch=$?\"\n", nil)
	assert.NotContains(t, write.stdout, "write=0")
	assert.NotContains(t, write.stdout, "touch=0")
	assert.NotContains(t, write.stdout, "changed")
	require.NoError(t, plugin.Destroy(context.Background()))

	other := prepare(t, dir, id, 2, sandbox.KindJob, nil)
	require.NoError(t, other.Start(ctx))
	absent := run(t, other, "ls "+sandbox.InputsPath+" 2>&1; echo \"ls=$?\"\n", nil)
	assert.NotContains(t, absent.stdout, "site.tgz")
	assert.NotContains(t, absent.stdout, "ls=0")
}

func startWithExact(t *testing.T, exact []string, answers map[string][]string) sandbox.Sandbox {
	t.Helper()
	id, dir := newTargetAttempt(t)
	box := prepare(t, dir, id, 1, sandbox.KindPlugin, func(s *sandbox.PrepareSpec) { s.Exact = exact })
	eb, ok := box.(sandbox.Egress)
	require.True(t, ok)
	require.NotNil(t, eb.EgressListener(), "a job with only exact endpoints still gets the egress channel")
	p, err := egress.New(egress.Options{
		Exact:        exact,
		AllowPrivate: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")},
		Resolver:     &testResolver{answers: answers},
	})
	require.NoError(t, err)
	sctx, scancel := context.WithCancel(context.Background())
	t.Cleanup(scancel)
	go func() { _ = p.Serve(sctx, eb.EgressListener()) }()
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	require.NoError(t, box.Start(ctx))
	return box
}

func TestADeploySandboxReachesItsExactEndpointsAndNothingElse(t *testing.T) {
	port := echoOnLoopback(t)
	exact := []string{"127.0.0.1:" + port, "web-1.dhow.test:" + port}
	box := startWithExact(t, exact, map[string][]string{"web-1.dhow.test": {"127.0.0.1"}, "yaml.dhow.test": {"127.0.0.1"}})
	tests := []struct {
		target string
		ok     bool
		status string
	}{
		{"127.0.0.1:" + port, true, "status 200"},
		{"web-1.dhow.test:" + port, true, "status 200"},
		{"127.0.0.1:1", false, "status 403"},
		{"web-1.dhow.test:1", false, "status 403"},
		{"yaml.dhow.test:" + port, false, "status 403"},
		{"127.0.0.2:" + port, false, "status 403"},
		{"registry.npmjs.org:443", false, "status 403"},
	}
	for _, tt := range tests {
		out := run(t, box, probe+" tunnel "+tt.target+"\n", nil)
		assert.Contains(t, out.stdout, tt.status, tt.target)
		if tt.ok {
			assert.Zero(t, out.result.ExitCode, tt.target)
			assert.Contains(t, out.stdout, "echo ping", tt.target)
		} else {
			assert.NotZero(t, out.result.ExitCode, tt.target)
		}
	}
}
