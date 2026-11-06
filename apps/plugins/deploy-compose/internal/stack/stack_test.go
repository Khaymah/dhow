package stack_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/stack"
	"github.com/Khaymah/dhow/internal/archive"
	"github.com/Khaymah/dhow/internal/deployrelease"
)

const project = "shop"

func digest(n int) string {
	return fmt.Sprintf("registry.example.com/shop/web@sha256:%064x", n)
}

func composeFile(n int, extra string) string {
	return "services:\n  web:\n    image: " + digest(n) + "\n" + extra
}

type events struct {
	mu   sync.Mutex
	list []deployrelease.Event
}

func (e *events) Emit(ev deployrelease.Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.list = append(e.list, ev)
}

func (e *events) has(kind string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, ev := range e.list {
		if ev.Event == kind {
			return true
		}
	}
	return false
}

func (e *events) group(kind string) (deployrelease.Event, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, ev := range e.list {
		if ev.Event == kind && ev.HostID == "" {
			return ev, true
		}
	}
	return deployrelease.Event{}, false
}

type rig struct {
	t       *testing.T
	log     *execLog
	hosts   []*fakeHost
	connect map[int]int
	policy  policy.Options
}

func newRig(t *testing.T, names ...string) *rig {
	t.Helper()
	r := &rig{t: t, log: &execLog{}, connect: map[int]int{}}
	for _, n := range names {
		r.hosts = append(r.hosts, newFakeHost(t, n, r.log))
	}
	return r
}

func pack(t *testing.T, files map[string]string, links map[string]string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	var top []string
	for name, body := range files {
		full := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
		top = append(top, strings.SplitN(name, "/", 2)[0])
	}
	for name, target := range links {
		require.NoError(t, os.Symlink(target, filepath.Join(dir, name)))
		top = append(top, name)
	}
	out := filepath.Join(t.TempDir(), "artifact.tgz")
	f, err := os.Create(out)
	require.NoError(t, err)
	require.NoError(t, archive.Pack(dir, top, f))
	require.NoError(t, f.Close())
	raw, err := os.ReadFile(out)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	return out, hex.EncodeToString(sum[:])
}

type rollout struct {
	files map[string]string
	links map[string]string
	size  int
}

func (r *rig) deploy(id string, o rollout) (*deployrelease.Failure, *events) {
	r.t.Helper()
	path, sum := pack(r.t, o.files, o.links)
	act := &stack.Activator{Project: project, File: "compose.yaml", WaitTimeout: 30 * time.Second, Policy: r.policy}
	cfg := deployrelease.Config{
		DeployPath: "/srv/app", ReleaseID: id, Keep: 5, HookTimeout: time.Minute, FirstFailure: deployrelease.FirstFailureKeep,
		ArchivePath: path, ArchiveDigest: sum, Activator: act,
	}
	rec := &events{}
	f := &deployrelease.Fleet{Config: cfg, BatchSize: max(o.size, 1), Report: rec, Clock: deployrelease.RealClock(), Log: discard{}}
	for i, h := range r.hosts {
		f.Hosts = append(f.Hosts, deployrelease.Host{
			ID: fmt.Sprintf("0190a1b2-0000-7000-8000-00000000000%d", i+1), Name: h.name,
			Connect: func(context.Context) (deployrelease.Remote, error) { r.connect[i]++; return h, nil },
		})
	}
	return f.Run(context.Background()), rec
}

type discard struct{}

func (discard) Info(string, ...any) {}
func (discard) Warn(string, ...any) {}

func release(n int) string { return fmt.Sprintf("r%d", n) }

func args(id string, tail ...string) string {
	base := "/srv/app/releases/" + id
	return strings.Join(append([]string{
		"env", "-i", "PATH=/usr/local/bin:/usr/bin:/bin", "DOCKER_CONFIG=/home/deploy/.docker",
		"docker", "compose", "--project-name=" + project, "--project-directory=" + base,
		"--file=" + base + "/compose.yaml", "--file=" + base + "/compose.dhow.yaml", "--env-file=/dev/null",
	}, tail...), " ")
}

func up(id string) string {
	return args(id, "up", "--detach", "--wait", "--no-build", "--wait-timeout=30")
}

func pull(id string) string { return args(id, "pull", "--quiet") }

func onlyDocker(lines []string) []string {
	var out []string
	for _, l := range lines {
		if strings.HasPrefix(l, "env -i ") && !strings.HasSuffix(l, " config --format json") {
			out = append(out, l)
		}
	}
	return out
}

func okDeploy(t *testing.T, r *rig, n int) {
	t.Helper()
	f, _ := r.deploy(release(n), rollout{files: map[string]string{"compose.yaml": composeFile(n, "")}})
	require.Nil(t, f, "deploy %d", n)
}

func TestADeployPullsThenStartsTheServicesWithOneQuotedLinePerCommand(t *testing.T) {
	r := newRig(t, "a")
	f, rec := r.deploy("r1", rollout{files: map[string]string{"compose.yaml": composeFile(1, "")}})
	require.Nil(t, f)
	assert.Equal(t, []string{pull("r1"), up("r1")}, onlyDocker(r.log.forHost("a")))
	assert.Equal(t, "releases/r1", r.hosts[0].current())
	assert.True(t, rec.has(deployrelease.EventVerified))
	assert.Equal(t, "services:\n    web:\n        image: "+digest(1)+"\n", r.hosts[0].read("/srv/app/releases/r1/compose.dhow.yaml"))
}

func TestTheImagesInputPinsAServiceWithoutTouchingTheFile(t *testing.T) {
	r := newRig(t, "a")
	r.policy = policy.Options{Images: map[string]string{"web": digest(7)}}
	f, _ := r.deploy("r1", rollout{files: map[string]string{"compose.yaml": "services:\n  web:\n    image: registry.example.com/shop/web:latest\n"}})
	require.Nil(t, f)
	assert.Contains(t, r.hosts[0].read("/srv/app/releases/r1/compose.dhow.yaml"), digest(7))
	assert.Contains(t, r.hosts[0].read("/srv/app/releases/r1/compose.yaml"), ":latest")
}

func TestAnUnsafeOrUnpinnedFileIsRefusedBeforeAnyServerIsTouched(t *testing.T) {
	for name, tc := range map[string]struct {
		file  string
		links map[string]string
		code  string
	}{
		"a tag":         {"services:\n  web:\n    image: shop/web:1\n", nil, policy.CodeNotPinned},
		"privileged":    {composeFile(1, "    privileged: true\n"), nil, "COMPOSE_UNSAFE_PRIVILEGED"},
		"the root":      {composeFile(1, "    volumes: ['/:/host']\n"), nil, "COMPOSE_UNSAFE_BIND_ROOT"},
		"docker.sock":   {composeFile(1, "    volumes: ['/var/run/docker.sock:/s']\n"), nil, "COMPOSE_UNSAFE_DOCKER_SOCKET"},
		"build":         {composeFile(1, "    build: .\n"), nil, policy.CodeBuild},
		"a link source": {composeFile(1, "    volumes: ['./data:/d']\n"), map[string]string{"data": "sub"}, "COMPOSE_UNSAFE_BIND_SYMLINK"},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, "a", "b")
			f, rec := r.deploy("r1", rollout{files: map[string]string{"compose.yaml": tc.file, "sub/x": "x"}, links: tc.links})
			require.NotNil(t, f)
			assert.Equal(t, stack.ExitPolicy, f.Exit)
			var v *policy.Violation
			require.True(t, errors.As(f, &v))
			assert.Equal(t, tc.code, v.Code)
			assert.Empty(t, r.log.all(), "no command ran on any server")
			assert.Zero(t, r.connect[0]+r.connect[1], "no server was even connected")
			assert.True(t, rec.has(deployrelease.EventFailed))
		})
	}
}

func TestAnArtifactWithoutTheComposeFileIsRefused(t *testing.T) {
	r := newRig(t, "a")
	f, _ := r.deploy("r1", rollout{files: map[string]string{"other.yaml": composeFile(1, "")}})
	require.NotNil(t, f)
	assert.Equal(t, stack.ExitPolicy, f.Exit)
	assert.Empty(t, r.log.all())
}

func TestTheFileOnTheServerIsCheckedAgainAndMustBeARegularFile(t *testing.T) {
	r := newRig(t, "a")
	h := r.hosts[0]
	rel := deployrelease.Release{DeployPath: "/srv/app", Path: "/srv/app/releases/r9", ID: "r9"}
	require.NoError(t, h.MkdirAll(rel.Path, 0o755))
	act := &stack.Activator{Project: project, File: "compose.yaml"}

	require.NoError(t, os.WriteFile(h.path(rel.Path+"/compose.yaml"), []byte(composeFile(1, "    privileged: true\n")), 0o644))
	err := act.Prepare(context.Background(), h, rel)
	var f *deployrelease.Failure
	require.ErrorAs(t, err, &f)
	assert.Equal(t, stack.ExitPolicy, f.Exit)
	assert.Empty(t, r.log.all())

	require.NoError(t, os.Remove(h.path(rel.Path+"/compose.yaml")))
	require.NoError(t, os.Symlink("/etc/passwd", h.path(rel.Path+"/compose.yaml")))
	err = act.Prepare(context.Background(), h, rel)
	require.ErrorAs(t, err, &f)
	assert.ErrorIs(t, err, stack.ErrFile)
	assert.Empty(t, r.log.all())
}

func TestAMissingImageFailsThePullAndLeavesTheOldServicesAlone(t *testing.T) {
	r := newRig(t, "a")
	okDeploy(t, r, 1)
	r.hosts[0].docker = func(argv []string) deployrelease.ExecResult {
		if argv[len(argv)-2] == "pull" {
			return deployrelease.ExecResult{ExitCode: 1, Output: []byte("manifest unknown")}
		}
		return deployrelease.ExecResult{}
	}
	before := len(r.log.forHost("a"))
	f, rec := r.deploy("r2", rollout{files: map[string]string{"compose.yaml": composeFile(2, "")}})
	require.NotNil(t, f)
	assert.Equal(t, deployrelease.ExitFailed, f.Exit)
	assert.Equal(t, deployrelease.CauseHook, f.Reason)
	assert.Equal(t, "releases/r1", r.hosts[0].current())
	for _, line := range r.log.forHost("a")[before:] {
		assert.NotContains(t, line, " up ", "the services were not touched")
	}
	assert.NoDirExists(t, r.hosts[0].path("/srv/app/releases/r2"))
	assert.True(t, rec.has(deployrelease.EventFailed))
	assert.False(t, rec.has(deployrelease.EventRolledBack), "nothing was switched, so nothing is rolled back")
}

func TestAServiceThatDoesNotComeUpHealthyRollsBackToThePreviousImages(t *testing.T) {
	r := newRig(t, "a")
	okDeploy(t, r, 1)
	r.hosts[0].docker = func(argv []string) deployrelease.ExecResult {
		if strings.Contains(strings.Join(argv, " "), "releases/r2") && argv[len(argv)-5] == "up" {
			return deployrelease.ExecResult{ExitCode: 1, Output: []byte("container web is unhealthy")}
		}
		return deployrelease.ExecResult{}
	}
	before := len(r.log.forHost("a"))
	f, rec := r.deploy("r2", rollout{files: map[string]string{"compose.yaml": composeFile(2, "")}})
	require.NotNil(t, f)
	assert.Equal(t, deployrelease.ExitRolledBack, f.Exit)
	assert.Equal(t, deployrelease.CauseReload, f.Reason)
	assert.Equal(t, "releases/r1", r.hosts[0].current())
	assert.Equal(t, []string{pull("r2"), up("r2"), up("r1")}, onlyDocker(r.log.forHost("a")[before:]))
	assert.True(t, rec.has(deployrelease.EventRolledBack))
	_, group := rec.group(deployrelease.EventRolledBack)
	assert.True(t, group)
}

func TestTheFirstReleaseThatDoesNotComeUpIsKeptForTheAdministrator(t *testing.T) {
	r := newRig(t, "a")
	r.hosts[0].docker = func(argv []string) deployrelease.ExecResult {
		if argv[len(argv)-5] == "up" {
			return deployrelease.ExecResult{ExitCode: 1}
		}
		return deployrelease.ExecResult{}
	}
	f, _ := r.deploy("r1", rollout{files: map[string]string{"compose.yaml": composeFile(1, "")}})
	require.NotNil(t, f)
	assert.Equal(t, deployrelease.ExitFailed, f.Exit)
	assert.Equal(t, "releases/r1", r.hosts[0].current())
}

func TestTheSecondServerFailingInTheSecondBatchRollsBackTheFirst(t *testing.T) {
	r := newRig(t, "a", "b")
	okDeploy(t, r, 1)
	r.hosts[1].docker = func(argv []string) deployrelease.ExecResult {
		if strings.Contains(strings.Join(argv, " "), "releases/r2") && argv[len(argv)-5] == "up" {
			return deployrelease.ExecResult{ExitCode: 1}
		}
		return deployrelease.ExecResult{}
	}
	mark := len(r.log.all())
	f, rec := r.deploy("r2", rollout{files: map[string]string{"compose.yaml": composeFile(2, "")}, size: 1})
	require.NotNil(t, f)
	assert.Equal(t, deployrelease.ExitRolledBack, f.Exit)
	for _, h := range r.hosts {
		assert.Equal(t, "releases/r1", h.current(), h.name)
	}
	assert.Equal(t, []string{pull("r2"), up("r2"), up("r1")}, onlyDocker(r.log.forHost("a")[mark/2:]))
	assert.Equal(t, []string{pull("r2"), up("r2"), up("r1")}, onlyDocker(r.log.forHost("b")[mark/2:]))
	_, ok := rec.group(deployrelease.EventRolledBack)
	assert.True(t, ok, "the group is rolled_back")
	assert.False(t, rec.has(deployrelease.EventVerified))
}

func TestTheEnvFileOfTheServerIsUsedWhenItIsThere(t *testing.T) {
	r := newRig(t, "a")
	require.NoError(t, r.hosts[0].MkdirAll("/srv/app/shared", 0o755))
	require.NoError(t, os.WriteFile(r.hosts[0].path("/srv/app/shared/.env"), []byte("A=1\n"), 0o600))
	f, _ := r.deploy("r1", rollout{files: map[string]string{"compose.yaml": composeFile(1, "")}})
	require.Nil(t, f)
	for _, line := range onlyDocker(r.log.forHost("a")) {
		assert.Contains(t, line, "--env-file=/srv/app/shared/.env")
	}
}

func TestEveryArgumentIsOneWordOrOneQuotedWord(t *testing.T) {
	act := &stack.Activator{Project: project, File: "deploy/my app.yaml"}
	argv := act.Argv(deployrelease.Release{DeployPath: "/srv/app", Path: "/srv/app/releases/r1"}, "/dev/null", "up")
	line, err := deployrelease.Command(argv...)
	require.NoError(t, err)
	assert.Contains(t, line, "'--file=/srv/app/releases/r1/deploy/my app.yaml'")
	assert.NotContains(t, line, ";")
	assert.True(t, strings.HasPrefix(line, "docker compose '--project-name=shop' "))
}
