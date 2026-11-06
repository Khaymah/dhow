package policy_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
)

func TestWhatPullsInOtherFilesOrSharesNamespacesIsRefusedEvenWhenUnsafeIsAllowed(t *testing.T) {
	for name, tc := range map[string]struct{ extra, code string }{
		"extends":              {"    extends:\n      file: other.yaml\n      service: web\n", "COMPOSE_UNSAFE_EXTENDS"},
		"a provider":           {"    provider:\n      type: tool\n", "COMPOSE_UNSAFE_PROVIDER"},
		"include":              {"include:\n  - other.yaml\n", "COMPOSE_UNSAFE_INCLUDE"},
		"volumes_from":         {"    volumes_from: [db]\n", "COMPOSE_UNSAFE_VOLUMES_FROM"},
		"pid of a container":   {"    pid: 'container:abc'\n", "COMPOSE_UNSAFE_NAMESPACE_SHARED"},
		"pid of a service":     {"    pid: 'service:db'\n", "COMPOSE_UNSAFE_NAMESPACE_SHARED"},
		"network of a service": {"    network_mode: 'service:db'\n", "COMPOSE_UNSAFE_NAMESPACE_SHARED"},
		"network of a box":     {"    network_mode: 'container:abc'\n", "COMPOSE_UNSAFE_NAMESPACE_SHARED"},
		"ipc of a service":     {"    ipc: 'service:db'\n", "COMPOSE_UNSAFE_NAMESPACE_SHARED"},
		"ipc shareable":        {"    ipc: shareable\n", "COMPOSE_UNSAFE_NAMESPACE_SHARED"},
		"an external network":  {"networks:\n  n:\n    external: true\n", "COMPOSE_UNSAFE_EXTERNAL"},
		"a named network":      {"networks:\n  n:\n    name: other\n", "COMPOSE_UNSAFE_EXTERNAL"},
		"an external volume":   {"volumes:\n  v:\n    external: true\n", "COMPOSE_UNSAFE_EXTERNAL"},
		"a named volume":       {"volumes:\n  v:\n    name: other\n", "COMPOSE_UNSAFE_EXTERNAL"},
		"a volume driver":      {"volumes:\n  v:\n    driver: nfs\n", "COMPOSE_UNSAFE_VOLUME_DRIVER"},
		"a secret from env":    {"secrets:\n  s:\n    environment: HOME\n", "COMPOSE_UNSAFE_SECRET_SOURCE"},
		"a config from env":    {"configs:\n  c:\n    environment: HOME\n", "COMPOSE_UNSAFE_SECRET_SOURCE"},
		"an external secret":   {"secrets:\n  s:\n    external: true\n", "COMPOSE_UNSAFE_SECRET_SOURCE"},
		"inline content":       {"configs:\n  c:\n    content: x\n", "COMPOSE_UNSAFE_SECRET_SOURCE"},
	} {
		t.Run(name, func(t *testing.T) {
			for _, allow := range []bool{false, true} {
				_, code := check(t, compose(tc.extra), policy.Options{AllowUnsafe: allow})
				assert.Equal(t, tc.code, code, "allow=%v", allow)
			}
		})
	}
}

func TestAKeyThatIsNotOnTheListIsRefusedAndNewKeysFailClosed(t *testing.T) {
	for name, extra := range map[string]string{
		"a new service key":    "    brand_new_key: true\n",
		"container_name":       "    container_name: shop\n",
		"dns":                  "    dns: [1.1.1.1]\n",
		"develop":              "    develop:\n      watch: []\n",
		"a deploy key":         "    deploy:\n      mode: global\n",
		"a top level key":      "brand_new: {}\n",
		"a top level services": "x-ok: 1\nservice: {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			for _, allow := range []bool{false, true} {
				_, code := check(t, compose(extra), policy.Options{AllowUnsafe: allow})
				assert.Equal(t, "COMPOSE_UNSAFE_UNKNOWN_KEY", code, "allow=%v", allow)
			}
		})
	}
}

func TestTheNormalKeysOfAnApplicationAreAllowed(t *testing.T) {
	extra := strings.Join([]string{
		"    command: run", "    entrypoint: ['/bin/app']", "    environment: ['A=1']", "    env_file: .env",
		"    ports: ['80:80', '443:443']", "    expose: ['9000']", "    networks: [back]", "    depends_on: [db]", "    restart: always",
		"    labels: {a: b}", "    logging: {driver: json-file}", "    user: '1000'", "    working_dir: /app", "    read_only: true",
		"    tmpfs: [/tmp]", "    stop_signal: SIGTERM", "    stop_grace_period: 10s", "    init: true", "    hostname: web", "    domainname: example.com",
		"    mem_limit: 512m", "    cpus: 1", "    ulimits: {nofile: 1024}", "    shm_size: 64m", "    profiles: [prod]", "    x-note: 1",
		"    deploy:", "      resources: {limits: {memory: 1g}}", "      restart_policy: {condition: any}",
		"    secrets: [s]", "    configs: [c]",
		"networks:", "  back: {}", "secrets:", "  s:", "    file: ./s.txt", "configs:", "  c:", "    file: ./c.txt", "x-top: 1", "",
	}, "\n")
	_, code := check(t, compose(extra), policy.Options{})
	assert.Empty(t, code)
}

func TestTheOptionsThatAreUnsafeAreOpenOnlyWithTheAdministratorOption(t *testing.T) {
	for key, extra := range map[string]string{
		"CGROUP_PARENT":  "    cgroup_parent: x\n",
		"RUNTIME":        "    runtime: runc\n",
		"SYSCTLS":        "    sysctls: {net.core.somaxconn: 1024}\n",
		"GROUP_ADD":      "    group_add: [audio]\n",
		"EXTRA_HOSTS":    "    extra_hosts: ['a:1.1.1.1']\n",
		"EXTERNAL_LINKS": "    external_links: [db]\n",
		"MODELS":         "    models: {m: {}}\n",
	} {
		t.Run(key, func(t *testing.T) {
			_, code := check(t, compose(extra), policy.Options{})
			assert.Equal(t, "COMPOSE_UNSAFE_"+key, code)
			_, code = check(t, compose(extra), policy.Options{AllowUnsafe: true})
			assert.Empty(t, code)
		})
	}
	_, code := check(t, compose("models:\n  m: {}\n"), policy.Options{})
	assert.Equal(t, "COMPOSE_UNSAFE_MODELS", code)
}

func TestABindCannotReachTheReleaseItsFilesOrItsAncestors(t *testing.T) {
	const base = deployDir
	for name, tc := range map[string]struct{ volume, code string }{
		"the release as dot":         {".:/d", "COMPOSE_UNSAFE_BIND_RELEASE"},
		"the release as dot slash":   {"./:/d", "COMPOSE_UNSAFE_BIND_RELEASE"},
		"a dot with dots":            {"./a/..:/d", "COMPOSE_UNSAFE_BIND_RELEASE"},
		"the releases folder":        {base + "/releases:/d", "COMPOSE_UNSAFE_BIND_RELEASE"},
		"a release folder":           {base + "/releases/2-bbbbbbbb-1:/d", "COMPOSE_UNSAFE_BIND_RELEASE"},
		"a release folder with dots": {base + "/releases/2-bbbbbbbb-1/.:/d", "COMPOSE_UNSAFE_BIND_RELEASE"},
		"the deploy base":            {base + ":/d", "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"the parent of the base":     {"/srv:/d", "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"current":                    {base + "/current:/d", "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"the compose file":           {"./compose.yaml:/d", "COMPOSE_UNSAFE_BIND_COMPOSE"},
		"the override file":          {"./compose.dhow.yaml:/d", "COMPOSE_UNSAFE_BIND_COMPOSE"},
		"the compose file by path":   {base + "/releases/9-aaaaaaaa-1/compose.yaml:/d", "COMPOSE_UNSAFE_BIND_COMPOSE"},
		"the override by path":       {base + "/releases/9-aaaaaaaa-1/compose.dhow.yaml:/d", "COMPOSE_UNSAFE_BIND_COMPOSE"},
		"shared itself":              {base + "/shared:/d", "COMPOSE_UNSAFE_BIND_SHARED"},
		"shared with a slash":        {base + "/shared/:/d", "COMPOSE_UNSAFE_BIND_SHARED"},
		"the env file":               {base + "/shared/.env:/d", "COMPOSE_UNSAFE_BIND_SHARED"},
		"a link in another release":  {base + "/releases/9-aaaaaaaa-1/data/x:/d", "COMPOSE_UNSAFE_BIND_SYMLINK"},
		"a link by relative path":    {"./data:/d", "COMPOSE_UNSAFE_BIND_SYMLINK"},
	} {
		t.Run(name, func(t *testing.T) {
			_, code := check(t, compose("    volumes:\n      - '"+tc.volume+"'\n"), policy.Options{Links: []string{"data"}})
			assert.Equal(t, tc.code, code)
		})
	}
}

func TestAComposeFileInASubfolderIsProtectedWithItsFolder(t *testing.T) {
	raw := compose("    volumes: ['./deploy:/d']\n")
	_, code := check(t, raw, policy.Options{File: "deploy/prod.yaml"})
	assert.Equal(t, "COMPOSE_UNSAFE_BIND_COMPOSE", code)
	_, code = check(t, compose("    volumes: ['./deploy/data:/d']\n"), policy.Options{File: "deploy/prod.yaml"})
	assert.Empty(t, code)
}

func TestSubfoldersOfTheReleaseAndOfSharedStillWork(t *testing.T) {
	for _, volume := range []string{
		"./conf:/c", "./a/b:/c", deployDir + "/shared/uploads:/u", deployDir + "/shared/uploads/2026:/u", deployDir + "/shared/.cache:/u",
		deployDir + "/releases/9-aaaaaaaa-1/conf:/c",
	} {
		_, code := check(t, compose("    volumes: ['"+volume+"']\n"), policy.Options{})
		assert.Empty(t, code, volume)
	}
}

func TestTheEnvFileOfTheServerCanBeReadEvenThoughItCannotBeMounted(t *testing.T) {
	_, code := check(t, compose("    env_file: "+deployDir+"/shared/.env\n"), policy.Options{})
	assert.Empty(t, code)
}

func TestTheOverrideFileIsReadBackStrictly(t *testing.T) {
	raw, err := policy.Override(map[string]string{"web": webDigest})
	assert.NoError(t, err)
	got, err := policy.ParseOverride(raw)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"web": webDigest}, got)
	for _, bad := range []string{"", "services: {}\n", "services:\n  web:\n    image: x:1\n", "services:\n  web:\n    image: " + webDigest + "\n    privileged: true\n"} {
		_, err := policy.ParseOverride([]byte(bad))
		assert.Error(t, err, bad)
	}
}
