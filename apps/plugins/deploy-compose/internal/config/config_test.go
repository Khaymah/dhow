package config_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/config"
	"github.com/Khaymah/dhow/internal/deployplugin"
	"github.com/Khaymah/dhow/internal/deployrelease"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const image = "registry.example.com/shop/web@sha256:0000000000000000000000000000000000000000000000000000000000000001"

func line(host string) string {
	blob := append([]byte{0, 0, 0, 11}, []byte("ssh-ed25519")...)
	blob = append(blob, []byte("0123456789abcdef0123456789abcdef")...)
	return jobspec.KnownHostsLine(host, 22, "ssh-ed25519 "+base64.StdEncoding.EncodeToString(blob))
}

func host(n string) *runnerv1.TargetHost {
	name := "web-" + n + ".example.com"
	return &runnerv1.TargetHost{HostId: "0190a1b2-0000-7000-8000-00000000000" + n, Host: name, Port: 22, KnownHostsLine: line(name)}
}

type setup struct {
	kind     string
	settings *runnerv1.TargetSettings
	extra    map[string]string
}

func env(t *testing.T, s setup) func(string) string {
	t.Helper()
	if s.kind == "" {
		s.kind = "compose"
	}
	if s.settings == nil {
		s.settings = &runnerv1.TargetSettings{BatchSize: 1, ComposeProject: "shop", ComposeFile: "compose.yaml"}
	}
	vars, err := jobspec.TargetEnv(&runnerv1.StepTarget{
		Name: "prod", Kind: s.kind, User: "deploy", DeployPath: "/srv/shop", ReleaseId: "12-abcdef12-1", KeepReleases: 5,
		Hosts: []*runnerv1.TargetHost{host("1"), host("2")}, Settings: s.settings,
	}, 0, "/run/dhow/secrets")
	require.NoError(t, err)
	vars["DHOW_INPUT_ARTIFACT_FILE"] = "/run/dhow/inputs/shop.tgz"
	vars["DHOW_INPUT_ARTIFACT_DIGEST"] = strings.Repeat("a", 64)
	for k, v := range s.extra {
		vars[k] = v
	}
	return func(k string) string { return vars[k] }
}

func TestTheDefaultsMatchThePluginManifest(t *testing.T) {
	s, err := config.Parse(env(t, setup{}))
	require.NoError(t, err)
	assert.Equal(t, "shop", s.Project)
	assert.Equal(t, "compose.yaml", s.File)
	assert.False(t, s.AllowUnsafe)
	assert.Empty(t, s.Images)
	assert.Equal(t, 2*time.Minute, s.WaitTimeout)
	assert.Equal(t, 10*time.Minute, s.PullTimeout)
	assert.Len(t, s.Step.Hosts, 2)
	assert.Equal(t, deployrelease.FirstFailureKeep, s.Deploy.FirstFailure)
}

func TestTheInputsAreRead(t *testing.T) {
	s, err := config.Parse(env(t, setup{
		settings: &runnerv1.TargetSettings{BatchSize: 2, ComposeProject: "shop-2", ComposeFile: "deploy/prod.yaml", AllowUnsafeCompose: true},
		extra:    map[string]string{"DHOW_INPUT_IMAGES": "web=" + image, "DHOW_INPUT_WAIT_TIMEOUT": "45s", "DHOW_INPUT_PULL_TIMEOUT": "2m"},
	}))
	require.NoError(t, err)
	assert.Equal(t, "shop-2", s.Project)
	assert.Equal(t, "deploy/prod.yaml", s.File)
	assert.True(t, s.AllowUnsafe)
	assert.Equal(t, map[string]string{"web": image}, s.Images)
	assert.Equal(t, 45*time.Second, s.WaitTimeout)
	assert.Equal(t, 2*time.Minute, s.PullTimeout)
	assert.Equal(t, 2, s.BatchSize)
}

func TestWhatThePluginDoesNotAcceptIsRefused(t *testing.T) {
	good := &runnerv1.TargetSettings{BatchSize: 1, ComposeProject: "shop", ComposeFile: "compose.yaml"}
	with := func(edit func(*runnerv1.TargetSettings)) *runnerv1.TargetSettings {
		c := &runnerv1.TargetSettings{BatchSize: good.BatchSize, ComposeProject: good.ComposeProject, ComposeFile: good.ComposeFile}
		edit(c)
		return c
	}
	for name, s := range map[string]setup{
		"an ssh target":             {kind: "ssh", settings: &runnerv1.TargetSettings{BatchSize: 1}},
		"a project with a slash":    {settings: with(func(c *runnerv1.TargetSettings) { c.ComposeProject = "a/b" })},
		"an upper case project":     {settings: with(func(c *runnerv1.TargetSettings) { c.ComposeProject = "Shop" })},
		"a file that climbs up":     {settings: with(func(c *runnerv1.TargetSettings) { c.ComposeFile = "../x.yaml" })},
		"an absolute file":          {settings: with(func(c *runnerv1.TargetSettings) { c.ComposeFile = "/etc/x.yaml" })},
		"the override file":         {settings: with(func(c *runnerv1.TargetSettings) { c.ComposeFile = "compose.dhow.yaml" })},
		"a reload command":          {settings: with(func(c *runnerv1.TargetSettings) { c.ReloadCommand = "true" })},
		"a shared path":             {settings: with(func(c *runnerv1.TargetSettings) { c.Shared = []string{".env"} })},
		"an image by tag":           {extra: map[string]string{"DHOW_INPUT_IMAGES": "web=ghcr.io/acme/web:1"}},
		"a wait that is too long":   {extra: map[string]string{"DHOW_INPUT_WAIT_TIMEOUT": "1h"}},
		"a wait that is too short":  {extra: map[string]string{"DHOW_INPUT_WAIT_TIMEOUT": "1s"}},
		"a pull that is not a time": {extra: map[string]string{"DHOW_INPUT_PULL_TIMEOUT": "soon"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := config.Parse(env(t, s))
			require.Error(t, err)
		})
	}
}

func TestAnInputErrorIsAnInputErrorOfThePlugin(t *testing.T) {
	_, err := config.Parse(env(t, setup{extra: map[string]string{"DHOW_INPUT_WAIT_TIMEOUT": "soon"}}))
	require.ErrorIs(t, err, deployplugin.ErrInput)
}
