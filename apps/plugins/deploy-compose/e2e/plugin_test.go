package e2e_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/archive"
	"github.com/Khaymah/dhow/internal/deployrelease"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

type deployment struct {
	t       *testing.T
	plugin  string
	hosts   []*composeHost
	name    string
	project string
	path    string
	port    int
}

func newDeployment(t *testing.T, name string, port int, hostCount int) *deployment {
	t.Helper()
	image, plugin := requireE2E(t)
	all := sharedHosts(t, image)
	d := &deployment{t: t, plugin: plugin, hosts: all[:hostCount], name: name, project: name, path: "/srv/app/" + name, port: port}
	for _, h := range d.hosts {
		h.prepare(t, d.path, d.project)
	}
	return d
}

func healthyCheck() string {
	return `["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/"]`
}

func failingCheck() string { return `["CMD", "false"]` }

func stateCheck() string {
	return `["CMD-SHELL", "v=$$(wget -q -O - http://127.0.0.1:8080/) && test -n \"$$v\" && test \"$$v\" != \"$$(cat /state/bad 2>/dev/null)\""]`
}

func (d *deployment) compose(image, check, extra string) string {
	return d.render(image, check, "", extra)
}

func (d *deployment) composeMount(image, mount string) string {
	return d.render(image, healthyCheck(), "      - '"+mount+"'\n", "")
}

func (d *deployment) render(image, check, mount, extra string) string {
	return fmt.Sprintf(`services:
  web:
    image: %s
    ports:
      - "%d:8080"
    healthcheck:
      test: %s
      interval: 1s
      timeout: 2s
      retries: 2
      start_period: 1s
    volumes:
      - state:/state
%s%s
volumes:
  state: {}
`, image, d.port, check, mount, extra)
}

type spec struct {
	release  string
	compose  string
	extra    map[string]string
	batch    int
	unsafe   bool
	images   string
	health   bool
	knownBad map[int]string
}

type result struct {
	exit   int
	events []deployrelease.Event
	output string
}

func pack(t *testing.T, files map[string]string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	var top []string
	for name, body := range files {
		full := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
		top = append(top, strings.SplitN(name, "/", 2)[0])
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

func hostID(i int) string { return fmt.Sprintf("0190a1b2-0000-7000-8000-00000000000%d", i+1) }

func (d *deployment) env(s spec, key, archivePath, sum, report string) []string {
	var targets []*runnerv1.TargetHost
	for i, h := range d.hosts {
		known := h.knownHostsLine()
		if bad := s.knownBad[i]; bad != "" {
			known = bad
		}
		targets = append(targets, &runnerv1.TargetHost{HostId: hostID(i), Host: h.ip, Port: sshPort, KnownHostsLine: known})
	}
	step := &runnerv1.StepTarget{
		Name: "prod", Kind: "compose", User: "deploy", DeployPath: d.path, ReleaseId: s.release, KeepReleases: 5, Hosts: targets,
		Settings: &runnerv1.TargetSettings{BatchSize: int32(max(s.batch, 1)), ComposeProject: d.project, ComposeFile: "compose.yaml", AllowUnsafeCompose: s.unsafe},
	}
	vars, err := jobspec.TargetEnv(step, 0, filepath.Dir(key))
	require.NoError(d.t, err)
	env := []string{"PATH=/usr/bin:/bin"}
	for k, v := range vars {
		env = append(env, k+"="+v)
	}
	env = append(env,
		"DHOW_INPUT_TARGET=prod", "DHOW_INPUT_ARTIFACT_FILE="+archivePath, "DHOW_INPUT_ARTIFACT_DIGEST="+sum, "DHOW_DEPLOY_REPORT="+report,
		"DHOW_DEPLOY_ALLOW_PRIVATE_CIDRS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16",
		"DHOW_INPUT_WAIT_TIMEOUT=30s", "DHOW_INPUT_PULL_TIMEOUT=2m",
		"DHOW_INPUT_HEALTH_CHECK_TIMEOUT=2s", "DHOW_INPUT_HEALTH_CHECK_INTERVAL=1s", "DHOW_INPUT_HEALTH_CHECK_RETRIES=4", "DHOW_INPUT_HEALTH_CHECK_STABLE=2",
	)
	if s.images != "" {
		env = append(env, "DHOW_INPUT_IMAGES="+s.images)
	}
	if s.health {
		env = append(env, fmt.Sprintf("DHOW_INPUT_HEALTH_CHECK_URL=http://{host}:%d/", d.port))
	}
	return env
}

func (d *deployment) deploy(scenario string, s spec) result {
	d.t.Helper()
	files := map[string]string{"compose.yaml": s.compose}
	for name, body := range s.extra {
		files[name] = body
	}
	archivePath, sum := pack(d.t, files)
	work := d.t.TempDir()
	key := filepath.Join(work, jobspec.TargetKeyFile(0))
	require.NoError(d.t, os.WriteFile(key, d.hosts[0].clientKey, 0o400))
	report := filepath.Join(work, "report")
	cmd := exec.CommandContext(context.Background(), d.plugin)
	cmd.Env = d.env(s, key, archivePath, sum, report)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	exit := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		require.ErrorAs(d.t, err, &ee)
		exit = ee.ExitCode()
	}
	raw, _ := os.ReadFile(report)
	res := result{exit: exit, output: out.String()}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var e deployrelease.Event
		require.NoError(d.t, json.Unmarshal([]byte(line), &e), line)
		res.events = append(res.events, e)
	}
	keepReport(d.t, scenario, raw, exit)
	return res
}

func keepReport(t *testing.T, scenario string, raw []byte, exit int) {
	t.Helper()
	dir := os.Getenv(envReport)
	if dir == "" || scenario == "" {
		return
	}
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, scenario+".jsonl"), raw, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, scenario+".exit"), []byte(fmt.Sprint(exit)), 0o644))
}

func (r result) has(kind string) bool {
	for _, e := range r.events {
		if e.Event == kind {
			return true
		}
	}
	return false
}

func (r result) groupEvent(kind string) (deployrelease.Event, bool) {
	for _, e := range r.events {
		if e.Event == kind && e.HostID == "" {
			return e, true
		}
	}
	return deployrelease.Event{}, false
}
