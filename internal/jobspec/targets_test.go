package jobspec_test

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func hostKey(keyType string) string {
	blob := binary.BigEndian.AppendUint32(nil, uint32(len(keyType)))
	blob = append(blob, keyType...)
	blob = append(blob, "key-material"...)
	return keyType + " " + base64.StdEncoding.EncodeToString(blob)
}

func targetSpec() *runnerv1.JobSpec {
	hosts := []*runnerv1.TargetHost{}
	for _, h := range []string{"web-1.example.com", "web-2.example.com", "203.0.113.9"} {
		hosts = append(hosts, &runnerv1.TargetHost{
			HostId: uuid.NewString(), Host: h, Port: 22, KnownHostsLine: jobspec.KnownHostsLine(h, 22, hostKey("ssh-ed25519")),
		})
	}
	return &runnerv1.JobSpec{
		Image: image,
		Steps: []*runnerv1.Step{
			{Name: "prepare", Run: "./prepare.sh"},
			{Name: "deploy", Plugin: &runnerv1.Plugin{Image: image, Inputs: map[string]string{"target": "web"}}, Mutating: true},
		},
		Secrets: []*runnerv1.SecretRef{
			{Name: jobspec.TargetKeySecret(0), Target: &runnerv1.SecretRef_AsFile{AsFile: jobspec.TargetKeyFile(0)}, StepIndexes: []int32{1}},
		},
		Network: &runnerv1.Network{ExactEndpoints: []string{
			"web-1.example.com:22", "web-2.example.com:22", "203.0.113.9:22", "web-1.example.com:443",
		}},
		Targets: []*runnerv1.StepTarget{{
			StepIndex: 1, TargetId: uuid.NewString(), Version: 3, Name: "web", Kind: "ssh", User: "deploy", DeployPath: "/srv/web",
			ReleaseId: "12-abcdef01-0", KeepReleases: 5, Hosts: hosts,
			Settings: &runnerv1.TargetSettings{
				BatchSize: 2, BatchPauseSeconds: 10, ReloadCommand: "sudo systemctl reload web", Shared: []string{".env", "storage"},
			},
		}},
	}
}

func TestATargetSpecIsValid(t *testing.T) {
	out, err := jobspec.Prepare(targetSpec())
	require.NoError(t, err)
	assert.Equal(t, []string{jobspec.FeaturePlugins, jobspec.FeatureDeployTargets}, jobspec.Features(out))
}

func TestATargetSpecIsRefusedWhenItBreaksARule(t *testing.T) {
	cases := []struct {
		name   string
		change func(*runnerv1.JobSpec)
		field  string
		reason string
	}{
		{"a step that is not a plugin", func(s *runnerv1.JobSpec) { s.Targets[0].StepIndex = 0 }, "Targets[0].StepIndex", jobspec.ReasonInvalid},
		{"a step out of range", func(s *runnerv1.JobSpec) { s.Targets[0].StepIndex = 9 }, "Targets[0].StepIndex", jobspec.ReasonOutOfRange},
		{"a step that is not mutating", func(s *runnerv1.JobSpec) { s.Steps[1].Mutating = false }, "Targets[0].StepIndex", jobspec.ReasonInvalid},
		{"a name the step does not name", func(s *runnerv1.JobSpec) { s.Targets[0].Name = "other" }, "Targets[0].StepIndex", jobspec.ReasonInvalid},
		{"two targets on one step", func(s *runnerv1.JobSpec) { s.Targets = append(s.Targets, s.Targets[0]) }, "Targets[1].StepIndex", jobspec.ReasonDuplicate},
		{"a target id that is not an id", func(s *runnerv1.JobSpec) { s.Targets[0].TargetId = "web" }, "Targets[0].TargetId", jobspec.ReasonInvalid},
		{"a version of zero", func(s *runnerv1.JobSpec) { s.Targets[0].Version = 0 }, "Targets[0].Version", jobspec.ReasonOutOfRange},
		{"an unknown kind", func(s *runnerv1.JobSpec) { s.Targets[0].Kind = "ftp" }, "Targets[0].Kind", jobspec.ReasonInvalid},
		{"a user that starts with a dash", func(s *runnerv1.JobSpec) { s.Targets[0].User = "-oProxyCommand=x" }, "Targets[0].User", jobspec.ReasonInvalid},
		{"a user with a space", func(s *runnerv1.JobSpec) { s.Targets[0].User = "dep loy" }, "Targets[0].User", jobspec.ReasonInvalid},
		{"a path with dots", func(s *runnerv1.JobSpec) { s.Targets[0].DeployPath = "/srv/../etc" }, "Targets[0].DeployPath", jobspec.ReasonInvalid},
		{"a system path", func(s *runnerv1.JobSpec) { s.Targets[0].DeployPath = "/etc/web" }, "Targets[0].DeployPath", jobspec.ReasonInvalid},
		{"a relative path", func(s *runnerv1.JobSpec) { s.Targets[0].DeployPath = "srv/web" }, "Targets[0].DeployPath", jobspec.ReasonInvalid},
		{"a release id with a slash", func(s *runnerv1.JobSpec) { s.Targets[0].ReleaseId = "a/b" }, "Targets[0].ReleaseId", jobspec.ReasonInvalid},
		{"one release kept", func(s *runnerv1.JobSpec) { s.Targets[0].KeepReleases = 1 }, "Targets[0].KeepReleases", jobspec.ReasonOutOfRange},
		{"no host", func(s *runnerv1.JobSpec) { s.Targets[0].Hosts = nil }, "Targets[0].Hosts", jobspec.ReasonRequired},
		{"eleven hosts", func(s *runnerv1.JobSpec) {
			for range 8 {
				s.Targets[0].Hosts = append(s.Targets[0].Hosts, s.Targets[0].Hosts[0])
			}
		}, "Targets[0].Hosts", jobspec.ReasonTooMany},
		{"a host that starts with a dash", func(s *runnerv1.JobSpec) { s.Targets[0].Hosts[0].Host = "-oProxyCommand=x" }, "Targets[0].Hosts[0].Host", jobspec.ReasonInvalid},
		{"a host with a zone", func(s *runnerv1.JobSpec) { s.Targets[0].Hosts[2].Host = "fe80::1%eth0" }, "Targets[0].Hosts[2].Host", jobspec.ReasonInvalid},
		{"a port of zero", func(s *runnerv1.JobSpec) { s.Targets[0].Hosts[0].Port = 0 }, "Targets[0].Hosts[0].Port", jobspec.ReasonOutOfRange},
		{"a host id that repeats", func(s *runnerv1.JobSpec) { s.Targets[0].Hosts[1].HostId = s.Targets[0].Hosts[0].HostId }, "Targets[0].Hosts[1].HostId", jobspec.ReasonInvalid},
		{"a key line for another host", func(s *runnerv1.JobSpec) {
			s.Targets[0].Hosts[0].KnownHostsLine = jobspec.KnownHostsLine("other.example.com", 22, hostKey("ssh-ed25519"))
		}, "Targets[0].Hosts[0].KnownHostsLine", jobspec.ReasonInvalid},
		{"a key line with a wrong key blob", func(s *runnerv1.JobSpec) { s.Targets[0].Hosts[0].KnownHostsLine = "web-1.example.com ssh-ed25519 AAAA" }, "Targets[0].Hosts[0].KnownHostsLine", jobspec.ReasonInvalid},
		{"a key line with a certificate type", func(s *runnerv1.JobSpec) {
			s.Targets[0].Hosts[0].KnownHostsLine = jobspec.KnownHostsLine("web-1.example.com", 22, hostKey("ssh-ed25519-cert-v01@openssh.com"))
		}, "Targets[0].Hosts[0].KnownHostsLine", jobspec.ReasonInvalid},
		{"a batch bigger than the hosts", func(s *runnerv1.JobSpec) { s.Targets[0].Settings.BatchSize = 4 }, "Targets[0].Settings.BatchSize", jobspec.ReasonOutOfRange},
		{"a pause over ten minutes", func(s *runnerv1.JobSpec) { s.Targets[0].Settings.BatchPauseSeconds = 601 }, "Targets[0].Settings.BatchPauseSeconds", jobspec.ReasonOutOfRange},
		{"a hook with two lines", func(s *runnerv1.JobSpec) { s.Targets[0].Settings.PreSwitchCommand = "a\nb" }, "Targets[0].Settings.PreSwitchCommand", jobspec.ReasonInvalid},
		{"a shared name with dots", func(s *runnerv1.JobSpec) { s.Targets[0].Settings.Shared = []string{"../x"} }, "Targets[0].Settings.Shared[0]", jobspec.ReasonInvalid},
		{"compose settings on an ssh target", func(s *runnerv1.JobSpec) { s.Targets[0].Settings.ComposeProject = "web" }, "Targets[0].Settings.ComposeProject", jobspec.ReasonInvalid},
		{"no settings", func(s *runnerv1.JobSpec) { s.Targets[0].Settings = nil }, "Targets[0].Settings", jobspec.ReasonRequired},
		{"no key secret", func(s *runnerv1.JobSpec) { s.Secrets = nil }, "Targets[0].Key", jobspec.ReasonRequired},
		{"a key for every step", func(s *runnerv1.JobSpec) { s.Secrets[0].StepIndexes = nil }, "Secrets[0]", jobspec.ReasonReserved},
		{"a key for the run step", func(s *runnerv1.JobSpec) { s.Secrets[0].StepIndexes = []int32{0, 1} }, "Secrets[0]", jobspec.ReasonReserved},
		{"a key as an environment variable", func(s *runnerv1.JobSpec) {
			s.Secrets[0].Target = &runnerv1.SecretRef_AsEnv{AsEnv: "KEY"}
		}, "Secrets[0]", jobspec.ReasonReserved},
		{"a key file with another name", func(s *runnerv1.JobSpec) { s.Secrets[0].Target = &runnerv1.SecretRef_AsFile{AsFile: "id"} }, "Secrets[0]", jobspec.ReasonReserved},
		{"a key secret of a target that does not exist", func(s *runnerv1.JobSpec) {
			s.Secrets = append(s.Secrets, &runnerv1.SecretRef{Name: jobspec.TargetKeySecret(1), Target: &runnerv1.SecretRef_AsFile{AsFile: jobspec.TargetKeyFile(1)}, StepIndexes: []int32{1}})
		}, "Secrets[1].Name", jobspec.ReasonReserved},
		{"a reserved name that is not a key", func(s *runnerv1.JobSpec) {
			s.Secrets = append(s.Secrets, &runnerv1.SecretRef{Name: "dhow.target.x", Target: &runnerv1.SecretRef_AsEnv{AsEnv: "X"}})
		}, "Secrets[1].Name", jobspec.ReasonReserved},
		{"a host missing from the exact endpoints", func(s *runnerv1.JobSpec) {
			s.Network.ExactEndpoints = []string{"web-1.example.com:22"}
		}, "Targets[0].Hosts", jobspec.ReasonInvalid},
		{"an exact endpoint that is a wildcard", func(s *runnerv1.JobSpec) {
			s.Network.ExactEndpoints = append(s.Network.ExactEndpoints, "*.example.com:443")
		}, "Network.ExactEndpoints[4]", jobspec.ReasonInvalid},
		{"an exact endpoint without a port", func(s *runnerv1.JobSpec) {
			s.Network.ExactEndpoints = append(s.Network.ExactEndpoints, "example.com")
		}, "Network.ExactEndpoints[4]", jobspec.ReasonInvalid},
		{"an exact endpoint twice", func(s *runnerv1.JobSpec) {
			s.Network.ExactEndpoints = append(s.Network.ExactEndpoints, "web-1.example.com:22")
		}, "Network.ExactEndpoints[4]", jobspec.ReasonDuplicate},
		{"too many targets", func(s *runnerv1.JobSpec) {
			for range 4 {
				s.Targets = append(s.Targets, s.Targets[0])
			}
		}, "Targets", jobspec.ReasonTooMany},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spec := targetSpec()
			c.change(spec)
			_, err := jobspec.Prepare(spec)
			require.Error(t, err)
			assert.Contains(t, violations(t, err), jobspec.Violation{Field: c.field, Reason: c.reason})
		})
	}
}

func TestTheNamesAndEndpointsOfTargetsAreReservedWithoutATarget(t *testing.T) {
	spec := valid()
	spec.Secrets = append(spec.Secrets, &runnerv1.SecretRef{Name: jobspec.TargetKeySecret(0), Target: &runnerv1.SecretRef_AsFile{AsFile: jobspec.TargetKeyFile(0)}})
	spec.Network.ExactEndpoints = []string{"example.com:22"}
	_, err := jobspec.Prepare(spec)
	require.Error(t, err)
	got := violations(t, err)
	assert.Contains(t, got, jobspec.Violation{Field: "Secrets[2].Name", Reason: jobspec.ReasonReserved})
	assert.Contains(t, got, jobspec.Violation{Field: "Network.ExactEndpoints", Reason: jobspec.ReasonReserved})
}

func TestTargetKeySecretNames(t *testing.T) {
	for i := range jobspec.MaxTargets {
		got, ok := jobspec.ParseTargetKeySecret(jobspec.TargetKeySecret(i))
		assert.True(t, ok)
		assert.Equal(t, i, got)
	}
	for _, bad := range []string{"", "dhow.target.", "dhow.target.0", "dhow.target.00.key", "dhow.target.-1.key", "dhow.target.4.key", "dhow.target.1.keys", "dhow.git.checkout"} {
		_, ok := jobspec.ParseTargetKeySecret(bad)
		assert.False(t, ok, bad)
	}
	assert.Equal(t, "target-key", jobspec.TargetKeyFile(0))
	assert.Equal(t, "target-key-2", jobspec.TargetKeyFile(2))
}

func TestValidDeployPath(t *testing.T) {
	for p, want := range map[string]bool{
		"/srv/web": true, "/var/www/shop.example": true, "/srv": false, "/": false, "/srv/web/": false, "/srv//web": false,
		"/srv/../web": false, "/srv/./web": false, "/etc/web": false, "/usr/local/web": false, "/var/lib/docker/x": false,
		"/srv/-rf": false, "/srv/we b": false, "": false, "/srv/web\n": false,
	} {
		assert.Equal(t, want, jobspec.ValidDeployPath(p), p)
	}
}

func TestKnownHostsPattern(t *testing.T) {
	assert.Equal(t, "web.example.com", jobspec.KnownHostsPattern("web.example.com", 22))
	assert.Equal(t, "[web.example.com]:2222", jobspec.KnownHostsPattern("web.example.com", 2222))
	assert.Equal(t, "[::1]:2222", jobspec.KnownHostsPattern("::1", 2222))
}

func TestParseEndpoint(t *testing.T) {
	for entry, want := range map[string]bool{
		"web.example.com:22": true, "203.0.113.9:2222": true, "[2001:db8::1]:22": true, "web.example.com": false, "web.example.com:0": false,
		"web.example.com:022": false, "WEB.example.com:22": false, "*.example.com:22": false, "[fe80::1%eth0]:22": false, "-x.example.com:22": false,
		"": false, ":22": false, "a b:22": false,
	} {
		_, _, ok := jobspec.ParseEndpoint(entry)
		assert.Equal(t, want, ok, entry)
	}
}

func TestHealthEndpoints(t *testing.T) {
	hosts := []string{"web-1.example.com", "203.0.113.9", "2001:db8::1"}
	cases := []struct {
		name string
		raw  string
		want []string
		bad  bool
	}{
		{"nothing to check", "", nil, false},
		{"one address for the whole group", "https://app.example.com/healthz", []string{"app.example.com:443"}, false},
		{"plain http", "http://app.example.com/healthz", []string{"app.example.com:80"}, false},
		{"a port", "https://app.example.com:8443/", []string{"app.example.com:8443"}, false},
		{"upper case host", "https://APP.example.com/", []string{"app.example.com:443"}, false},
		{"every host", "https://{host}/healthz", []string{"web-1.example.com:443", "203.0.113.9:443", "[2001:db8::1]:443"}, false},
		{"every host and a port", "http://{host}:8080/up", []string{"web-1.example.com:8080", "203.0.113.9:8080", "[2001:db8::1]:8080"}, false},
		{"a user name", "https://root@app.example.com/", nil, true},
		{"a password", "https://root:x@app.example.com/", nil, true},
		{"ftp", "ftp://app.example.com/", nil, true},
		{"no scheme", "app.example.com/healthz", nil, true},
		{"a space", "https://app.example.com/a b", nil, true},
		{"a newline", "https://app.example.com/\n", nil, true},
		{"the placeholder in the path", "https://app.example.com/{host}", nil, true},
		{"the placeholder next to text", "https://x{host}/", nil, true},
		{"the placeholder in the query", "https://{host}/?h={host}", nil, true},
		{"the placeholder in the user", "https://{host}@app.example.com/", nil, true},
		{"a port that is zero", "https://app.example.com:0/", nil, true},
		{"an empty host", "https:///x", nil, true},
		{"an address too long", "https://app.example.com/" + string(make([]byte, jobspec.MaxHealthURLLength)), nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := jobspec.HealthEndpoints(c.raw, hosts)
			if c.bad {
				assert.ErrorIs(t, err, jobspec.ErrHealthURL)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestTheEnvironmentOfATargetStep(t *testing.T) {
	spec := targetSpec()
	env, err := jobspec.TargetEnv(spec.GetTargets()[0], 0, "/run/dhow/secrets/")
	require.NoError(t, err)
	assert.Equal(t, "web", env[jobspec.EnvTargetName])
	assert.Equal(t, "ssh", env[jobspec.EnvTargetKind])
	assert.Equal(t, "deploy", env[jobspec.EnvTargetUser])
	assert.Equal(t, "/srv/web", env[jobspec.EnvTargetDeployPath])
	assert.Equal(t, "12-abcdef01-0", env[jobspec.EnvTargetReleaseID])
	assert.Equal(t, "5", env[jobspec.EnvTargetKeepRelease])
	assert.Equal(t, "/run/dhow/secrets/target-key", env[jobspec.EnvTargetKeyFile])

	var hosts []struct {
		HostId, Host, KnownHostsLine string
		Port                         int
	}
	require.NoError(t, json.Unmarshal([]byte(env[jobspec.EnvTargetHosts]), &hosts))
	require.Len(t, hosts, 3)
	for i, h := range spec.GetTargets()[0].GetHosts() {
		assert.Equal(t, []string{h.GetHostId(), h.GetHost(), h.GetKnownHostsLine()}, []string{hosts[i].HostId, hosts[i].Host, hosts[i].KnownHostsLine}, "in order")
	}
	var settings struct {
		BatchSize, BatchPauseSeconds int
		ReloadCommand                string
		Shared                       []string
	}
	require.NoError(t, json.Unmarshal([]byte(env[jobspec.EnvTargetSettings]), &settings))
	assert.Equal(t, 2, settings.BatchSize)
	assert.Equal(t, "sudo systemctl reload web", settings.ReloadCommand)
	assert.Equal(t, []string{".env", "storage"}, settings.Shared)
	for name := range env {
		assert.Contains(t, name, "DHOW_TARGET_")
	}
	second, err := jobspec.TargetEnv(spec.GetTargets()[0], 1, "/run/dhow/secrets")
	require.NoError(t, err)
	assert.Equal(t, "/run/dhow/secrets/target-key-1", second[jobspec.EnvTargetKeyFile])
}

func FuzzHealthEndpoints(f *testing.F) {
	for _, seed := range []string{
		"", "https://app.example.com/healthz", "http://{host}:8080/up", "https://{host}/", "https://root@x.example.com/", "https://x{host}/",
		"https://{host}/?{host}", "ftp://x", "https://[::1]:443/", "https://{host}@x.example.com", "{host}", "https://",
	} {
		f.Add(seed)
	}
	hosts := []string{"web-1.example.com", "203.0.113.9", "2001:db8::1"}
	f.Fuzz(func(t *testing.T, raw string) {
		got, err := jobspec.HealthEndpoints(raw, hosts)
		if err != nil {
			if got != nil {
				t.Fatalf("an error came with endpoints: %v", got)
			}
			return
		}
		again, err := jobspec.HealthEndpoints(raw, hosts)
		if err != nil || len(again) != len(got) {
			t.Fatalf("the same address gave two answers: %v and %v", got, again)
		}
		for _, e := range got {
			if _, _, ok := jobspec.ParseEndpoint(e); !ok {
				t.Fatalf("endpoint %q is not a valid endpoint", e)
			}
		}
		if len(got) > len(hosts) {
			t.Fatalf("%d endpoints for %d hosts", len(got), len(hosts))
		}
	})
}

func FuzzParseEndpoint(f *testing.F) {
	for _, seed := range []string{"web.example.com:22", "[2001:db8::1]:22", "203.0.113.9:2222", "", ":", "a:b:c", "[::1", "*.x.com:1", "x.com:99999"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, entry string) {
		host, port, ok := jobspec.ParseEndpoint(entry)
		if !ok {
			return
		}
		if jobspec.Endpoint(host, port) != entry {
			t.Fatalf("%q does not round trip: %q", entry, jobspec.Endpoint(host, port))
		}
		if port < 1 || port > 65535 || host == "" || host[0] == '-' {
			t.Fatalf("%q gave host %q and port %d", entry, host, port)
		}
	})
}
