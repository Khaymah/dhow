package config_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/apps/plugins/deploy-ssh/internal/config"
	"github.com/Khaymah/dhow/internal/deployrelease"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func line(host string) string {
	blob := append([]byte{0, 0, 0, 11}, []byte("ssh-ed25519")...)
	blob = append(blob, []byte("0123456789abcdef0123456789abcdef")...)
	return jobspec.KnownHostsLine(host, 22, "ssh-ed25519 "+base64.StdEncoding.EncodeToString(blob))
}

func host(name string) *runnerv1.TargetHost {
	return &runnerv1.TargetHost{HostId: "0190a1b2-0000-7000-8000-000000000001", Host: name, Port: 22, KnownHostsLine: line(name)}
}

func env(t *testing.T, hosts []*runnerv1.TargetHost, extra map[string]string) func(string) string {
	t.Helper()
	vars, err := jobspec.TargetEnv(&runnerv1.StepTarget{
		Name: "prod", Kind: "ssh", User: "deploy", DeployPath: "/srv/app", ReleaseId: "12-abcdef12-1", KeepReleases: 5, Hosts: hosts,
		Settings: &runnerv1.TargetSettings{BatchSize: 1, ReloadCommand: "systemctl reload app", Shared: []string{".env"}},
	}, 0, "/run/dhow/secrets")
	require.NoError(t, err)
	vars["DHOW_INPUT_ARTIFACT_FILE"] = "/run/dhow/inputs/site.tgz"
	vars["DHOW_INPUT_ARTIFACT_DIGEST"] = strings.Repeat("a", 64)
	vars["DHOW_DEPLOY_REPORT"] = "/tmp/.dhow-report"
	vars["HTTPS_PROXY"] = "http://127.0.0.1:3128"
	for k, v := range extra {
		vars[k] = v
	}
	return func(k string) string { return vars[k] }
}

func TestTheDefaultsMatchThePluginManifest(t *testing.T) {
	s, err := config.Parse(env(t, []*runnerv1.TargetHost{host("web-1.example.com")}, nil))
	require.NoError(t, err)
	assert.Equal(t, "/run/dhow/secrets/target-key", s.KeyFile)
	assert.Equal(t, "/tmp/.dhow-report", s.ReportPath)
	assert.Equal(t, "http://127.0.0.1:3128", s.ProxyURL)
	d := s.Deploy
	assert.Equal(t, deployrelease.FirstFailureKeep, d.FirstFailure)
	assert.Equal(t, 5, d.Keep)
	assert.Equal(t, "web-1.example.com", d.HostName)
	assert.Equal(t, "systemctl reload app", d.ReloadCommand)
	assert.Equal(t, []string{".env"}, d.Shared)
	assert.False(t, d.Health.Enabled(), "without a health_check_url nothing is checked")
}

func TestTheHealthInputsAreReadWithTheirDefaults(t *testing.T) {
	s, err := config.Parse(env(t, []*runnerv1.TargetHost{host("web-1.example.com")}, map[string]string{"DHOW_INPUT_HEALTH_CHECK_URL": "https://{host}/healthz"}))
	require.NoError(t, err)
	h := s.Deploy.Health
	assert.Equal(t, deployrelease.Health{
		URL: "https://{host}/healthz", StatusMin: 200, StatusMax: 399, Timeout: 5 * time.Second, Retries: 10, Interval: 5 * time.Second, Stable: 3,
	}, h)

	s, err = config.Parse(env(t, []*runnerv1.TargetHost{host("web-1.example.com")}, map[string]string{
		"DHOW_INPUT_HEALTH_CHECK_URL": "https://lb.example.com/up", "DHOW_INPUT_HEALTH_CHECK_STATUS": "204", "DHOW_INPUT_HEALTH_CHECK_TIMEOUT": "2s",
		"DHOW_INPUT_HEALTH_CHECK_RETRIES": "4", "DHOW_INPUT_HEALTH_CHECK_INTERVAL": "1s", "DHOW_INPUT_HEALTH_CHECK_STABLE": "2",
		"DHOW_INPUT_HEALTH_CHECK_START_DELAY": "3s", "DHOW_INPUT_KEEP_RELEASES": "3", "DHOW_INPUT_ON_FIRST_FAILURE": "fail",
	}))
	require.NoError(t, err)
	assert.Equal(t, 204, s.Deploy.Health.StatusMin)
	assert.Equal(t, 204, s.Deploy.Health.StatusMax)
	assert.Equal(t, 3*time.Second, s.Deploy.Health.StartDelay)
	assert.Equal(t, 3, s.Deploy.Keep, "the input can keep fewer than the target does")
	assert.Equal(t, deployrelease.FirstFailureFail, s.Deploy.FirstFailure)
}

func TestKeepReleasesFromTheStepCannotKeepMoreThanTheTarget(t *testing.T) {
	s, err := config.Parse(env(t, []*runnerv1.TargetHost{host("web-1.example.com")}, map[string]string{"DHOW_INPUT_KEEP_RELEASES": "30"}))
	require.NoError(t, err)
	assert.Equal(t, 5, s.Deploy.Keep)
}

func TestWhatThePluginCannotDoYetFailsClearly(t *testing.T) {
	one := []*runnerv1.TargetHost{host("web-1.example.com")}
	for name, c := range map[string]struct {
		hosts []*runnerv1.TargetHost
		extra map[string]string
	}{
		"two servers":           {[]*runnerv1.TargetHost{host("web-1.example.com"), host("web-2.example.com")}, nil},
		"another strategy":      {one, map[string]string{"DHOW_INPUT_STRATEGY": "blue-green"}},
		"a compose target":      {one, map[string]string{"DHOW_TARGET_KIND": "compose"}},
		"a health url on ftp":   {one, map[string]string{"DHOW_INPUT_HEALTH_CHECK_URL": "ftp://x/h"}},
		"a budget over 10 min":  {one, map[string]string{"DHOW_INPUT_HEALTH_CHECK_URL": "https://x.example.com/h", "DHOW_INPUT_HEALTH_CHECK_RETRIES": "60", "DHOW_INPUT_HEALTH_CHECK_TIMEOUT": "30s"}},
		"stable over retries":   {one, map[string]string{"DHOW_INPUT_HEALTH_CHECK_URL": "https://x.example.com/h", "DHOW_INPUT_HEALTH_CHECK_STABLE": "11"}},
		"a status of words":     {one, map[string]string{"DHOW_INPUT_HEALTH_CHECK_URL": "https://x.example.com/h", "DHOW_INPUT_HEALTH_CHECK_STATUS": "ok"}},
		"a timeout of words":    {one, map[string]string{"DHOW_INPUT_HEALTH_CHECK_URL": "https://x.example.com/h", "DHOW_INPUT_HEALTH_CHECK_TIMEOUT": "soon"}},
		"no artifact digest":    {one, map[string]string{"DHOW_INPUT_ARTIFACT_DIGEST": ""}},
		"no artifact":           {one, map[string]string{"DHOW_INPUT_ARTIFACT_FILE": ""}},
		"keep that is a word":   {one, map[string]string{"DHOW_INPUT_KEEP_RELEASES": "many"}},
		"a private range of /4": {one, map[string]string{"DHOW_DEPLOY_ALLOW_PRIVATE_CIDRS": "0.0.0.0/4"}},
		"loopback as allowed":   {one, map[string]string{"DHOW_DEPLOY_ALLOW_PRIVATE_CIDRS": "127.0.0.0/8"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := config.Parse(env(t, c.hosts, c.extra))
			assert.Error(t, err)
		})
	}
}
