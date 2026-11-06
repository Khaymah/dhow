package policy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
)

func vol(entries ...string) string {
	out := "    volumes:\n"
	for _, e := range entries {
		out += "      - '" + e + "'\n"
	}
	return out
}

func TestPersistentDataIsInNamedVolumesAndHostBindsAreReadOnly(t *testing.T) {
	for name, tc := range map[string]struct{ extra, code string }{
		"a writable relative bind":           {vol("./data:/data"), "COMPOSE_UNSAFE_BIND_WRITABLE"},
		"an explicit rw bind":                {vol("./data:/data:rw"), "COMPOSE_UNSAFE_BIND_WRITABLE"},
		"a writable bind with z":             {vol("./data:/data:z"), "COMPOSE_UNSAFE_BIND_WRITABLE"},
		"a long syntax bind":                 {"    volumes:\n      - type: bind\n        source: ./data\n        target: /data\n", "COMPOSE_UNSAFE_BIND_WRITABLE"},
		"a long syntax bind read_only false": {"    volumes:\n      - type: bind\n        source: ./data\n        target: /data\n        read_only: false\n", "COMPOSE_UNSAFE_BIND_WRITABLE"},
		"a bind of shared":                   {vol(deployDir + "/shared/uploads:/u:ro"), "COMPOSE_UNSAFE_BIND_SHARED_DATA"},
		"an absolute bind in the release":    {vol(deployDir + "/releases/9-aaaaaaaa-1/conf:/c:ro"), "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"an absolute bind of another place":  {vol("/etc/ssl:/ssl:ro"), "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"a variable without a colon":         {vol("${X}"), "COMPOSE_UNSAFE_INTERPOLATION"},
		"a variable as a target":             {vol("data:/${X}"), "COMPOSE_UNSAFE_INTERPOLATION"},
		"a variable in the long syntax":      {"    volumes:\n      - type: volume\n        source: data\n        target: /${X}\n", "COMPOSE_UNSAFE_INTERPOLATION"},
	} {
		t.Run(name, func(t *testing.T) {
			_, code := check(t, compose(tc.extra), policy.Options{})
			assert.Equal(t, tc.code, code)
		})
	}
}

func TestReadOnlyBindsInsideTheReleaseAndNamedVolumesAreFine(t *testing.T) {
	extra := vol("./conf:/conf:ro", "./a/b:/b:ro,z", "data:/data", "/cache") + "    tmpfs: [/tmp]\n"
	extra += "volumes:\n  data: {}\n"
	res, code := check(t, compose(extra), policy.Options{})
	assert.Empty(t, code)
	assert.Equal(t, []string{"a/b", "conf"}, res.Binds)

	long := "    volumes:\n      - type: bind\n        source: ./conf\n        target: /conf\n        read_only: true\n"
	res, code = check(t, compose(long), policy.Options{})
	assert.Empty(t, code)
	assert.Equal(t, []string{"conf"}, res.Binds)
}

func TestTheFilesThatAreReadMustBeInTheReleaseOrTheSharedEnvFile(t *testing.T) {
	for name, tc := range map[string]struct{ extra, code string }{
		"an env file in shared":        {"    env_file: " + deployDir + "/shared/other.env\n", "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"an env file in another place": {"    env_file: /etc/passwd\n", "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"an env file in a release":     {"    env_file: " + deployDir + "/releases/9-aaaaaaaa-1/x.env\n", "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"an env file under a link":     {"    env_file: ./data/x.env\n", "COMPOSE_UNSAFE_BIND_SYMLINK"},
		"an env file in state":         {"    env_file: " + deployDir + "/.dhow/state.json\n", "COMPOSE_UNSAFE_BIND_STATE"},
		"a secret in shared":           {"secrets:\n  s:\n    file: " + deployDir + "/shared/key\n", "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"a config in state":            {"configs:\n  c:\n    file: " + deployDir + "/.dhow/x\n", "COMPOSE_UNSAFE_BIND_STATE"},
	} {
		t.Run(name, func(t *testing.T) {
			_, code := check(t, compose(tc.extra), policy.Options{Links: []string{"data"}})
			assert.Equal(t, tc.code, code)
		})
	}
	res, code := check(t, compose("    env_file:\n      - .env\n      - "+deployDir+"/shared/.env\n"), policy.Options{})
	assert.Empty(t, code)
	assert.Equal(t, []string{deployDir + "/shared/.env", ".env"}, append([]string{res.Files[1]}, res.Files[0]))
}

func TestOnlyANamedLocalVolumeWithoutOptionsIsAllowed(t *testing.T) {
	for name, tail := range map[string]string{
		"options":       "volumes:\n  v:\n    driver_opts:\n      type: none\n      o: bind\n      device: /\n",
		"empty options": "volumes:\n  v:\n    driver_opts: {}\n",
		"a driver":      "volumes:\n  v:\n    driver: nfs\n",
	} {
		t.Run(name, func(t *testing.T) {
			for _, allow := range []bool{false, true} {
				_, code := check(t, compose(tail), policy.Options{AllowUnsafe: allow})
				assert.Contains(t, []string{"COMPOSE_UNSAFE_VOLUME_OPTS", "COMPOSE_UNSAFE_VOLUME_DRIVER"}, code, "allow=%v", allow)
			}
		})
	}
	_, code := check(t, compose("volumes:\n  v:\n    driver: local\n"), policy.Options{})
	assert.Empty(t, code)
}

func TestTheAdministratorOptionReopensWritableBindsButNeverTheStateOrTheComposeFiles(t *testing.T) {
	allow := policy.Options{AllowUnsafe: true}
	for _, volume := range []string{"./data:/data", deployDir + "/shared/uploads:/u", deployDir + "/releases/9-aaaaaaaa-1/data:/d", "/etc:/etc"} {
		_, code := check(t, compose(vol(volume)), allow)
		assert.Empty(t, code, volume)
	}
	for volume, want := range map[string]string{
		deployDir + "/.dhow:/d":       "COMPOSE_UNSAFE_BIND_STATE",
		"./compose.yaml:/d":           "COMPOSE_UNSAFE_BIND_COMPOSE",
		".:/d":                        "COMPOSE_UNSAFE_BIND_RELEASE",
		deployDir + "/shared:/d":      "COMPOSE_UNSAFE_BIND_SHARED",
		deployDir + "/shared/.env:/d": "COMPOSE_UNSAFE_BIND_SHARED",
		deployDir + "/releases:/d":    "COMPOSE_UNSAFE_BIND_RELEASE",
		"${HOME}:/d":                  "COMPOSE_UNSAFE_INTERPOLATION",
		"./data:/d":                   "COMPOSE_UNSAFE_BIND_SYMLINK",
	} {
		_, code := check(t, compose(vol(volume)), policy.Options{AllowUnsafe: true, Links: []string{"data"}})
		assert.Equal(t, want, code, volume)
	}
}

func TestNetworksAndLoggingStayOnTheDefaultsUnlessUnsafeIsOpen(t *testing.T) {
	for name, tc := range map[string]struct{ extra, code string }{
		"a macvlan network":   {"networks:\n  n:\n    driver: macvlan\n", "COMPOSE_UNSAFE_NETWORK_DRIVER"},
		"network options":     {"networks:\n  n:\n    driver_opts: {a: b}\n", "COMPOSE_UNSAFE_NETWORK_OPTS"},
		"an ipam block":       {"networks:\n  n:\n    ipam: {driver: default}\n", "COMPOSE_UNSAFE_NETWORK_OPTS"},
		"a syslog driver":     {"    logging:\n      driver: syslog\n", "COMPOSE_UNSAFE_LOGGING"},
		"a logging option":    {"    logging:\n      driver: json-file\n      options: {tag: x}\n", "COMPOSE_UNSAFE_LOGGING"},
		"a logging extra key": {"    logging:\n      foo: bar\n", "COMPOSE_UNSAFE_LOGGING"},
	} {
		t.Run(name, func(t *testing.T) {
			_, code := check(t, compose(tc.extra), policy.Options{})
			assert.Equal(t, tc.code, code)
			_, code = check(t, compose(tc.extra), policy.Options{AllowUnsafe: true})
			assert.Empty(t, code)
		})
	}
	for _, ok := range []string{
		"networks:\n  n: {}\n", "networks:\n  n:\n    driver: bridge\n", "    logging:\n      driver: local\n",
		"    logging:\n      driver: json-file\n      options: {max-size: 10m, max-file: '3', compress: 'true'}\n",
	} {
		_, code := check(t, compose(ok), policy.Options{})
		assert.Empty(t, code, ok)
	}
}

func TestAVariableInANamespaceOptionIsRefusedEvenWhenUnsafeIsOpen(t *testing.T) {
	for _, key := range []string{"network_mode", "pid", "ipc", "uts"} {
		for _, allow := range []bool{false, true} {
			_, code := check(t, compose("    "+key+": ${X}\n"), policy.Options{AllowUnsafe: allow})
			assert.Equal(t, "COMPOSE_UNSAFE_INTERPOLATION", code, "%s allow=%v", key, allow)
		}
	}
}
