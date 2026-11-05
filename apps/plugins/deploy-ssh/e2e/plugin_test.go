package e2e_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/archive"
	"github.com/Khaymah/dhow/internal/deployrelease"
	"github.com/Khaymah/dhow/internal/deployrelease/sshremote"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const hostID = "0190a1b2-0000-7000-8000-000000000001"

type target struct {
	t      *testing.T
	s      *sshd
	plugin string
	name   string
	path   string
}

func newTarget(t *testing.T, name string) *target {
	image, plugin := requireE2E(t)
	s := sharedSshd(t, image)
	d := &target{t: t, s: s, plugin: plugin, name: name, path: "/srv/app/" + name}
	c := d.connect(s.knownHostsLine())
	require.NoError(t, c.MkdirAll(d.path, 0o755))
	require.NoError(t, c.Close())
	return d
}

func (d *target) connect(knownHosts string) *sshremote.Client {
	d.t.Helper()
	dialer := net.Dialer{Timeout: 5 * time.Second}
	c, err := sshremote.Connect(context.Background(), sshremote.Options{
		Host: d.s.ip, Port: sshPort, User: "deploy", PrivateKey: d.s.clientKey, KnownHostsLine: knownHosts, Dial: dialer.DialContext,
	})
	require.NoError(d.t, err)
	return c
}

type spec struct {
	release    string
	files      map[string]string
	keep       int
	health     bool
	reload     string
	preSwitch  string
	postSwitch string
	shared     []string
	knownHosts string
	archive    string
}

type result struct {
	exit   int
	events []deployrelease.Event
	output string
}

func packFiles(t *testing.T, files map[string]string) (string, string) {
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
	return out, digestOf(t, out)
}

func digestOf(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func page(version string, healthy bool) map[string]string {
	files := map[string]string{"index.html": version}
	if healthy {
		files["healthz"] = "ok"
	}
	return files
}

func (d *target) env(s spec, key string, archivePath, digest, report string) []string {
	known := s.knownHosts
	if known == "" {
		known = d.s.knownHostsLine()
	}
	step := &runnerv1.StepTarget{
		Name: "prod", Kind: "ssh", User: "deploy", DeployPath: d.path, ReleaseId: s.release, KeepReleases: int32(max(s.keep, 5)),
		Hosts: []*runnerv1.TargetHost{{HostId: hostID, Host: d.s.ip, Port: sshPort, KnownHostsLine: known}},
		Settings: &runnerv1.TargetSettings{
			BatchSize: 1, ReloadCommand: s.reload, PreSwitchCommand: s.preSwitch, PostSwitchCommand: s.postSwitch, Shared: s.shared,
		},
	}
	vars, err := jobspec.TargetEnv(step, 0, filepath.Dir(key))
	require.NoError(d.t, err)
	env := []string{"PATH=/usr/bin:/bin"}
	for k, v := range vars {
		env = append(env, k+"="+v)
	}
	env = append(env,
		"DHOW_INPUT_TARGET=prod",
		"DHOW_INPUT_ARTIFACT_FILE="+archivePath, "DHOW_INPUT_ARTIFACT_DIGEST="+digest, "DHOW_DEPLOY_REPORT="+report,
		"DHOW_DEPLOY_ALLOW_PRIVATE_CIDRS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16",
		"DHOW_INPUT_HEALTH_CHECK_TIMEOUT=2s", "DHOW_INPUT_HEALTH_CHECK_INTERVAL=1s", "DHOW_INPUT_HEALTH_CHECK_RETRIES=3", "DHOW_INPUT_HEALTH_CHECK_STABLE=2",
	)
	if s.health {
		env = append(env, fmt.Sprintf("DHOW_INPUT_HEALTH_CHECK_URL=http://{host}:%d/%s/current/healthz", httpPort, d.name))
	}
	if s.keep > 0 {
		env = append(env, fmt.Sprintf("DHOW_INPUT_KEEP_RELEASES=%d", s.keep))
	}
	return env
}

func (d *target) deploy(scenario string, s spec) result {
	d.t.Helper()
	archivePath, digest := s.archive, ""
	if archivePath == "" {
		archivePath, digest = packFiles(d.t, s.files)
	} else {
		digest = digestOf(d.t, archivePath)
	}
	work := d.t.TempDir()
	key := filepath.Join(work, jobspec.TargetKeyFile(0))
	require.NoError(d.t, os.WriteFile(key, d.s.clientKey, 0o400))
	report := filepath.Join(work, "report")
	cmd := exec.CommandContext(context.Background(), d.plugin)
	cmd.Env = d.env(s, key, archivePath, digest, report)
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
	d.keepReport(scenario, raw, exit)
	return res
}

func (d *target) keepReport(scenario string, raw []byte, exit int) {
	dir := os.Getenv(envReport)
	if dir == "" {
		return
	}
	require.NoError(d.t, os.MkdirAll(dir, 0o755))
	require.NoError(d.t, os.WriteFile(filepath.Join(dir, scenario+".jsonl"), raw, 0o644))
	require.NoError(d.t, os.WriteFile(filepath.Join(dir, scenario+".exit"), []byte(fmt.Sprint(exit)), 0o644))
}

func (r result) kinds() []string {
	out := make([]string, 0, len(r.events))
	for _, e := range r.events {
		k := e.Event
		if e.HostID == "" && k != deployrelease.EventStarted {
			k += "/group"
		}
		out = append(out, k)
	}
	return out
}

func (d *target) current() string {
	d.t.Helper()
	c := d.connect(d.s.knownHostsLine())
	defer func() { _ = c.Close() }()
	link, err := c.Readlink(d.path + "/current")
	if err != nil {
		return ""
	}
	return link
}

func (d *target) releases() []string {
	d.t.Helper()
	c := d.connect(d.s.knownHostsLine())
	defer func() { _ = c.Close() }()
	list, err := c.ReadDir(d.path + "/releases")
	if err != nil {
		return nil
	}
	var out []string
	for _, f := range list {
		out = append(out, f.Name)
	}
	return out
}

func (d *target) served(file string) string {
	_, body := d.s.get("/" + d.name + "/current/" + file)
	return body
}

func (d *target) exists(rel string) bool {
	c := d.connect(d.s.knownHostsLine())
	defer func() { _ = c.Close() }()
	_, err := c.Lstat(d.path + "/" + rel)
	return err == nil
}

func (d *target) readRemote(rel string) string {
	c := d.connect(d.s.knownHostsLine())
	defer func() { _ = c.Close() }()
	raw, err := c.ReadFile(d.path+"/"+rel, 4096)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}
