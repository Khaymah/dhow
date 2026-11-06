package e2e_test

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
	"github.com/Khaymah/dhow/internal/deployrelease/sshremote"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const hostCount = 3

var (
	fleetOnce sync.Once
	fleetSshd []*sshd
)

func sharedFleet(t *testing.T, image string) []*sshd {
	t.Helper()
	fleetOnce.Do(func() {
		private, authorized := newClientKey(t)
		for range hostCount {
			fleetSshd = append(fleetSshd, startSshdWith(t, image, private, authorized))
		}
	})
	require.Len(t, fleetSshd, hostCount, "the three test servers did not start")
	return fleetSshd
}

func groupHostID(i int) string { return fmt.Sprintf("0190a1b2-0000-7000-8000-00000000000%d", i+1) }

type group struct {
	t      *testing.T
	hosts  []*sshd
	plugin string
	name   string
	path   string
}

func newGroup(t *testing.T, name string) *group {
	image, plugin := requireE2E(t)
	g := &group{t: t, hosts: sharedFleet(t, image), plugin: plugin, name: name, path: "/srv/app/" + name}
	for i := range g.hosts {
		c := g.connect(i, g.hosts[i].knownHostsLine())
		require.NoError(t, c.MkdirAll(g.path, 0o755))
		require.NoError(t, c.Close())
	}
	return g
}

func (g *group) target(i int) *target {
	return &target{t: g.t, s: g.hosts[i], plugin: g.plugin, name: g.name, path: g.path}
}

func (g *group) connect(i int, known string) *sshremote.Client {
	return g.target(i).connect(known)
}

type rolling struct {
	release    string
	files      map[string]string
	batch      int
	pause      int
	reload     string
	shared     []string
	retries    int
	known      map[int]string
	port       map[int]int
	stopAtHost int
	stopAt     string
}

func (g *group) env(r rolling, key, archivePath, digest, report string) []string {
	var hosts []*runnerv1.TargetHost
	for i, h := range g.hosts {
		known, port := h.knownHostsLine(), sshPort
		if p := r.port[i]; p != 0 {
			port = p
			_, key, _ := strings.Cut(h.knownHostsLine(), " ")
			known = jobspec.KnownHostsLine(h.ip, p, key)
		}
		known = cmp.Or(r.known[i], known)
		hosts = append(hosts, &runnerv1.TargetHost{HostId: groupHostID(i), Host: h.ip, Port: int32(port), KnownHostsLine: known})
	}
	step := &runnerv1.StepTarget{
		Name: "prod", Kind: "ssh", User: "deploy", DeployPath: g.path, ReleaseId: r.release, KeepReleases: 5, Hosts: hosts,
		Settings: &runnerv1.TargetSettings{BatchSize: int32(max(r.batch, 1)), BatchPauseSeconds: int32(r.pause), ReloadCommand: r.reload, Shared: r.shared},
	}
	vars, err := jobspec.TargetEnv(step, 0, filepath.Dir(key))
	require.NoError(g.t, err)
	env := []string{"PATH=/usr/bin:/bin"}
	for k, v := range vars {
		env = append(env, k+"="+v)
	}
	return append(env,
		"DHOW_INPUT_TARGET=prod", "DHOW_INPUT_ARTIFACT_FILE="+archivePath, "DHOW_INPUT_ARTIFACT_DIGEST="+digest, "DHOW_DEPLOY_REPORT="+report,
		"DHOW_DEPLOY_ALLOW_PRIVATE_CIDRS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16",
		"DHOW_INPUT_HEALTH_CHECK_TIMEOUT=2s", "DHOW_INPUT_HEALTH_CHECK_INTERVAL=1s", fmt.Sprintf("DHOW_INPUT_HEALTH_CHECK_RETRIES=%d", cmp.Or(r.retries, 3)),
		"DHOW_INPUT_HEALTH_CHECK_STABLE=2", fmt.Sprintf("DHOW_INPUT_HEALTH_CHECK_URL=http://{host}:%d/%s/current/healthz", httpPort, g.name),
	)
}

func (g *group) deploy(scenario string, r rolling) result {
	g.t.Helper()
	archivePath, digest := packFiles(g.t, r.files)
	work := g.t.TempDir()
	key := filepath.Join(work, jobspec.TargetKeyFile(0))
	require.NoError(g.t, os.WriteFile(key, g.hosts[0].clientKey, 0o400))
	report := filepath.Join(work, "report")
	var watch func(*exec.Cmd)
	if r.stopAt != "" {
		watch = func(cmd *exec.Cmd) { go g.stopWhenCurrentIs(cmd, r.stopAtHost, r.stopAt) }
	}
	if scenario != "" {
		scenario = "rolling-" + scenario
	}
	return runPlugin(g.t, g.plugin, g.env(r, key, archivePath, digest, report), report, scenario, watch)
}

func (g *group) stopWhenCurrentIs(cmd *exec.Cmd, host int, want string) {
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if g.target(host).current() == want {
			_ = cmd.Process.Signal(syscall.SIGTERM)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
}

func (g *group) deployAll(release string, batch int) {
	g.t.Helper()
	out := g.deploy("", rolling{release: release, files: page("v"+release[:1], true), batch: batch})
	require.Equal(g.t, 0, out.exit, out.output)
}

func (g *group) provideShared(name string, hosts ...int) {
	g.t.Helper()
	for _, i := range hosts {
		c := g.connect(i, g.hosts[i].knownHostsLine())
		require.NoError(g.t, c.MkdirAll(g.path+"/shared", 0o755))
		w, err := c.Create(g.path+"/shared/"+name, 0o644)
		require.NoError(g.t, err)
		_, err = w.Write([]byte("ok"))
		require.NoError(g.t, err)
		require.NoError(g.t, w.Close())
		require.NoError(g.t, c.Close())
	}
}

func (g *group) putFile(i int, rel, body string) {
	g.t.Helper()
	c := g.connect(i, g.hosts[i].knownHostsLine())
	defer func() { _ = c.Close() }()
	w, err := c.Create(g.path+"/"+rel, 0o644)
	require.NoError(g.t, err)
	_, err = w.Write([]byte(body))
	require.NoError(g.t, err)
	require.NoError(g.t, w.Close())
}

func (g *group) current(i int) string { return g.target(i).current() }

func (g *group) served(i int, file string) string {
	_, body := g.hosts[i].get("/" + g.name + "/current/" + file)
	return body
}

func (g *group) exists(i int, rel string) bool { return g.target(i).exists(rel) }

func (g *group) releases(i int) []string {
	out := g.target(i).releases()
	slices.Sort(out)
	return out
}

func (r result) forHost(i int) []string {
	var out []string
	for _, e := range r.events {
		if e.HostID == groupHostID(i) {
			out = append(out, e.Event)
		}
	}
	return out
}

func (r result) groupKinds() []string {
	var out []string
	for _, e := range r.events {
		if e.HostID == "" {
			out = append(out, e.Event)
		}
	}
	return out
}

func (r result) indexOf(kind string, host int) int {
	return slices.IndexFunc(r.events, func(e deployrelease.Event) bool { return e.Event == kind && e.HostID == groupHostID(host) })
}
