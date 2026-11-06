package policy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
)

func TestNetworkModeIsAbsentOrNoneAndNetworksAreAttachedWithNetworks(t *testing.T) {
	for _, mode := range []string{"bridge", "default", "other_default", "myproject_net", "''"} {
		for _, allow := range []bool{false, true} {
			_, code := check(t, compose("    network_mode: "+mode+"\n"), policy.Options{AllowUnsafe: allow})
			assert.Equal(t, "COMPOSE_UNSAFE_NETWORK_MODE", code, "%s allow=%v", mode, allow)
		}
	}
	_, code := check(t, compose("    network_mode: none\n"), policy.Options{})
	assert.Empty(t, code)
	_, code = check(t, compose("    networks: [back]\nnetworks:\n  back: {}\n"), policy.Options{})
	assert.Empty(t, code)
}

func TestATagThatIsNotACoreYamlTagIsRefused(t *testing.T) {
	for name, raw := range map[string]string{
		"reset":         "services:\n  web:\n    image: " + webDigest + "\n    ports: !reset []\n",
		"override":      "services:\n  web:\n    image: " + webDigest + "\n    ports: !override ['1:1']\n",
		"a custom tag":  "services:\n  web:\n    image: " + webDigest + "\n    command: !x run\n",
		"a tagged key":  "services:\n  web:\n    image: " + webDigest + "\n    !k labels: {}\n",
		"a top level":   "!top\nservices:\n  web:\n    image: " + webDigest + "\n",
		"in a sequence": "services:\n  web:\n    image: " + webDigest + "\n    expose: [!y '80']\n",
	} {
		t.Run(name, func(t *testing.T) {
			for _, allow := range []bool{false, true} {
				_, code := check(t, []byte(raw), policy.Options{AllowUnsafe: allow})
				assert.Equal(t, policy.CodeInvalid, code, "allow=%v", allow)
			}
		})
	}
	_, code := check(t, []byte("services:\n  web:\n    image: "+webDigest+"\n    expose: [!!str '80']\n    init: !!bool true\n"), policy.Options{})
	assert.Empty(t, code, "core tags are fine")
}

func TestATopLevelNameIsAbsentOrTheProjectName(t *testing.T) {
	opt := policy.Options{Project: "shop"}
	_, code := check(t, compose("name: shop\n"), opt)
	assert.Empty(t, code)
	for _, name := range []string{"other", "shop-2", "''", "${P}", "Shop"} {
		for _, allow := range []bool{false, true} {
			o := opt
			o.AllowUnsafe = allow
			_, code = check(t, compose("name: "+name+"\n"), o)
			assert.Contains(t, []string{"COMPOSE_UNSAFE_PROJECT_NAME", "COMPOSE_UNSAFE_INTERPOLATION"}, code, "%s allow=%v", name, allow)
		}
	}
	_, code = check(t, compose("name: shop\n"), policy.Options{})
	assert.Equal(t, "COMPOSE_UNSAFE_PROJECT_NAME", code, "without a project name nothing is equal")
}

func TestLabelsMayNotUseTheDockerPrefix(t *testing.T) {
	for name, extra := range map[string]string{
		"a service map":    "    labels:\n      com.docker.compose.project: other\n",
		"a service list":   "    labels:\n      - com.docker.compose.project=other\n",
		"a capital letter": "    labels:\n      Com.Docker.compose.service: x\n",
		"a network":        "networks:\n  n:\n    labels:\n      com.docker.x: y\n",
		"a volume":         "volumes:\n  v:\n    labels:\n      com.docker.compose.volume: y\n",
	} {
		t.Run(name, func(t *testing.T) {
			for _, allow := range []bool{false, true} {
				_, code := check(t, compose(extra), policy.Options{AllowUnsafe: allow})
				assert.Equal(t, "COMPOSE_UNSAFE_LABEL", code, "allow=%v", allow)
			}
		})
	}
	_, code := check(t, compose("    labels:\n      com.example.a: b\n      traefik.enable: 'true'\n"), policy.Options{})
	assert.Empty(t, code)
}

func TestBindOptionsOfTheLongFormNeedTheAdministratorOption(t *testing.T) {
	long := func(opts string) string {
		return "    volumes:\n      - type: bind\n        source: ./c\n        target: /c\n        read_only: true\n        bind:\n" + opts
	}
	for name, opts := range map[string]string{
		"propagation":      "          propagation: rshared\n",
		"selinux":          "          selinux: z\n",
		"recursive":        "          recursive: writable\n",
		"create_host_path": "          create_host_path: true\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, code := check(t, compose(long(opts)), policy.Options{})
			assert.Equal(t, "COMPOSE_UNSAFE_BIND_OPTIONS", code)
			_, code = check(t, compose(long(opts)), policy.Options{AllowUnsafe: true})
			assert.Empty(t, code)
		})
	}
	_, code := check(t, compose(long("          create_host_path: false\n")), policy.Options{})
	assert.Empty(t, code)
	_, code = check(t, compose(long("          create_host_path: 'false'\n")), policy.Options{})
	assert.Equal(t, policy.CodeInvalid, code)
}
