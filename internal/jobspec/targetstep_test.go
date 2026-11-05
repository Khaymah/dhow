package jobspec_test

import (
	"encoding/base64"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func knownHostsFor(host string, port int) string {
	blob := append([]byte{0, 0, 0, 11}, []byte("ssh-ed25519")...)
	blob = append(blob, []byte("0123456789abcdef0123456789abcdef")...)
	return jobspec.KnownHostsLine(host, port, "ssh-ed25519 "+base64.StdEncoding.EncodeToString(blob))
}

func stepTarget() *runnerv1.StepTarget {
	return &runnerv1.StepTarget{
		Name: "prod", Kind: "ssh", User: "deploy", DeployPath: "/srv/app", ReleaseId: "12-abcdef12-1", KeepReleases: 5,
		Hosts: []*runnerv1.TargetHost{{HostId: "0190a1b2-0000-7000-8000-000000000001", Host: "web-1.example.com", Port: 22, KnownHostsLine: knownHostsFor("web-1.example.com", 22)}},
		Settings: &runnerv1.TargetSettings{
			BatchSize: 1, ReloadCommand: "systemctl reload app", PostSwitchCommand: "./warm.sh", Shared: []string{".env", "storage"},
		},
	}
}

func envOf(t *testing.T, edit func(map[string]string)) func(string) string {
	t.Helper()
	vars, err := jobspec.TargetEnv(stepTarget(), 0, "/run/dhow/secrets")
	require.NoError(t, err)
	if edit != nil {
		edit(vars)
	}
	return func(k string) string { return vars[k] }
}

func TestParseTargetStepReadsWhatTargetEnvWrites(t *testing.T) {
	got, err := jobspec.ParseTargetStep(envOf(t, nil))
	require.NoError(t, err)
	assert.Equal(t, "prod", got.Name)
	assert.Equal(t, "/srv/app", got.DeployPath)
	assert.Equal(t, "12-abcdef12-1", got.ReleaseID)
	assert.Equal(t, 5, got.KeepReleases)
	assert.Equal(t, "/run/dhow/secrets/target-key", got.KeyFile)
	require.Len(t, got.Hosts, 1)
	assert.Equal(t, "web-1.example.com", got.Hosts[0].Host)
	assert.Equal(t, []string{".env", "storage"}, got.Settings.Shared)
	assert.Equal(t, "systemctl reload app", got.Settings.ReloadCommand)
}

func TestParseTargetStepRefusesWhatTheServerWouldNeverSend(t *testing.T) {
	for name, edit := range map[string]func(map[string]string){
		"a user that starts with a dash": func(v map[string]string) { v[jobspec.EnvTargetUser] = "-oProxyCommand=x" },
		"a system path":                  func(v map[string]string) { v[jobspec.EnvTargetDeployPath] = "/etc/app" },
		"a path with a dot dot":          func(v map[string]string) { v[jobspec.EnvTargetDeployPath] = "/srv/../etc" },
		"a release id with a slash":      func(v map[string]string) { v[jobspec.EnvTargetReleaseID] = "a/b" },
		"keep below two":                 func(v map[string]string) { v[jobspec.EnvTargetKeepRelease] = "1" },
		"keep that is not a number":      func(v map[string]string) { v[jobspec.EnvTargetKeepRelease] = "x" },
		"no key file":                    func(v map[string]string) { v[jobspec.EnvTargetKeyFile] = "" },
		"a relative key file":            func(v map[string]string) { v[jobspec.EnvTargetKeyFile] = "target-key" },
		"no hosts":                       func(v map[string]string) { v[jobspec.EnvTargetHosts] = "[]" },
		"hosts that are not json":        func(v map[string]string) { v[jobspec.EnvTargetHosts] = "web-1" },
		"an unknown host field": func(v map[string]string) {
			v[jobspec.EnvTargetHosts] = strings.Replace(v[jobspec.EnvTargetHosts], "Host\"", "Hosts\"", 1)
		},
		"a host that starts with a dash": func(v map[string]string) {
			v[jobspec.EnvTargetHosts] = strings.ReplaceAll(v[jobspec.EnvTargetHosts], "web-1.example.com", "-oBad")
		},
		"a known hosts line of another host": func(v map[string]string) {
			v[jobspec.EnvTargetHosts] = strings.Replace(v[jobspec.EnvTargetHosts], `"KnownHostsLine":"web-1.example.com`, `"KnownHostsLine":"web-2.example.com`, 1)
		},
		"a hook with a newline": func(v map[string]string) {
			v[jobspec.EnvTargetSettings] = strings.Replace(v[jobspec.EnvTargetSettings], "./warm.sh", `./a\nb`, 1)
		},
		"a shared name with a dot dot": func(v map[string]string) {
			v[jobspec.EnvTargetSettings] = strings.Replace(v[jobspec.EnvTargetSettings], "storage", "..", 1)
		},
		"trailing data after the hosts": func(v map[string]string) { v[jobspec.EnvTargetHosts] += "[]" },
		"settings that are not json":    func(v map[string]string) { v[jobspec.EnvTargetSettings] = "{" },
		"a name that is not a name":     func(v map[string]string) { v[jobspec.EnvTargetName] = "Prod One" },
		"a kind that is not a kind":     func(v map[string]string) { v[jobspec.EnvTargetKind] = "ftp" },
	} {
		t.Run(name, func(t *testing.T) {
			_, err := jobspec.ParseTargetStep(envOf(t, edit))
			assert.ErrorIs(t, err, jobspec.ErrTargetStep)
		})
	}
}

func TestResolveHealthURLParsesExactlyAsTheServerDoes(t *testing.T) {
	for _, raw := range []string{
		"https://{host}/healthz", "http://{host}:8080/up?x=1", "https://lb.example.com/healthz", "https://lb.example.com:8443/h",
		"http://[2606:2800:220:1::1]/h", "https://{host}.evil.example.com/h", "https://user@{host}/h", "https://{host}:{host}/h",
		"ftp://{host}/h", "https://{host}/{host}", "https://exa mple.com/h", "https://{host}@x/h", "//{host}/h", "https://{host}:99999/h",
		"https://-oBad/h", "https://{host}\\@evil.com/h",
	} {
		for _, host := range []string{"web-1.example.com", "10.0.0.5", "2606:2800:220:1::1"} {
			endpoints, endpointErr := jobspec.HealthEndpoints(raw, []string{host})
			u, err := jobspec.ResolveHealthURL(raw, host)
			if endpointErr != nil {
				assert.Error(t, err, "%s for %s", raw, host)
				continue
			}
			require.NoError(t, err, "%s for %s", raw, host)
			port := strconv.Itoa(jobspec.DefaultHTTPPort)
			if u.Scheme == "https" {
				port = strconv.Itoa(jobspec.DefaultTLSPort)
			}
			if p := u.Port(); p != "" {
				port = p
			}
			n, err := strconv.Atoi(port)
			require.NoError(t, err)
			require.Len(t, endpoints, 1)
			assert.Equal(t, jobspec.Endpoint(strings.ToLower(u.Hostname()), n), endpoints[0], "%s for %s", raw, host)
		}
	}
}
