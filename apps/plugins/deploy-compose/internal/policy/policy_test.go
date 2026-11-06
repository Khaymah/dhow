package policy_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
)

const (
	webDigest = "ghcr.io/acme/web@sha256:0000000000000000000000000000000000000000000000000000000000000001"
	dbDigest  = "registry.example.com:5000/db@sha256:0000000000000000000000000000000000000000000000000000000000000002"
	deployDir = "/srv/app"
)

func compose(extra string) []byte {
	return []byte("services:\n  web:\n    image: " + webDigest + "\n" + extra)
}

func check(t *testing.T, raw []byte, opt policy.Options) (policy.Result, string) {
	t.Helper()
	if opt.DeployPath == "" {
		opt.DeployPath = deployDir
	}
	res, err := policy.Check(raw, opt)
	if err == nil {
		return res, ""
	}
	var v *policy.Violation
	require.True(t, errors.As(err, &v), "every refusal is a Violation: %v", err)
	return res, v.Code
}

func TestAComposeFileWithPinnedImagesPasses(t *testing.T) {
	raw := []byte("services:\n  web:\n    image: " + webDigest + "\n    ports: ['8080:80']\n    volumes:\n      - data:/data\n      - ./conf:/conf:ro\n      - " + deployDir + "/shared/uploads:/uploads\n  db:\n    image: " + dbDigest + "\nvolumes:\n  data: {}\n")
	res, code := check(t, raw, policy.Options{})
	require.Empty(t, code)
	assert.Equal(t, map[string]string{"web": webDigest, "db": dbDigest}, res.Images)
	assert.Equal(t, []string{"db", "web"}, res.Services())
}

func TestEveryImageMustBePinnedByDigest(t *testing.T) {
	for name, image := range map[string]string{
		"a tag":             "ghcr.io/acme/web:1.2.3",
		"latest":            "ghcr.io/acme/web",
		"a short digest":    "ghcr.io/acme/web@sha256:abc",
		"another algorithm": "ghcr.io/acme/web@sha512:" + strings.Repeat("a", 64),
		"upper case digest": "ghcr.io/acme/web@sha256:" + strings.Repeat("A", 64),
		"a variable":        "${IMAGE}@sha256:" + strings.Repeat("a", 64),
		"a variable tag":    "ghcr.io/acme/web:${TAG}",
		"a space":           "ghcr.io/acme/web @sha256:" + strings.Repeat("a", 64),
		"empty":             "",
	} {
		t.Run(name, func(t *testing.T) {
			_, code := check(t, []byte("services:\n  web:\n    image: '"+image+"'\n"), policy.Options{})
			assert.Equal(t, policy.CodeNotPinned, code)
		})
	}
	_, code := check(t, []byte("services:\n  web:\n    command: run\n"), policy.Options{})
	assert.Equal(t, policy.CodeNotPinned, code, "a service without an image")
}

func TestAServiceThatBuildsIsRefusedEvenWhenUnsafeIsAllowed(t *testing.T) {
	raw := compose("    build: .\n")
	for _, allow := range []bool{false, true} {
		_, code := check(t, raw, policy.Options{AllowUnsafe: allow})
		assert.Equal(t, policy.CodeBuild, code)
	}
}

func TestAnImageOverrideReplacesTheImageOfTheFile(t *testing.T) {
	raw := []byte("services:\n  web:\n    image: ghcr.io/acme/web:latest\n")
	res, code := check(t, raw, policy.Options{Images: map[string]string{"web": webDigest}})
	require.Empty(t, code)
	assert.Equal(t, webDigest, res.Images["web"])

	_, code = check(t, raw, policy.Options{Images: map[string]string{"web": "ghcr.io/acme/web:1"}})
	assert.Equal(t, policy.CodeNotPinned, code)
	_, code = check(t, raw, policy.Options{Images: map[string]string{"worker": webDigest}})
	assert.Equal(t, policy.CodeUnknownService, code)
}

func TestTheUnsafeOptionsOfAServiceAreRefused(t *testing.T) {
	for name, tc := range map[string]struct{ extra, code string }{
		"privileged":              {"    privileged: true\n", "COMPOSE_UNSAFE_PRIVILEGED"},
		"privileged as text":      {"    privileged: 'yes'\n", "COMPOSE_UNSAFE_PRIVILEGED"},
		"privileged by variable":  {"    privileged: ${P}\n", "COMPOSE_UNSAFE_PRIVILEGED"},
		"pid host":                {"    pid: host\n", "COMPOSE_UNSAFE_PID_HOST"},
		"ipc host":                {"    ipc: host\n", "COMPOSE_UNSAFE_IPC_HOST"},
		"uts host":                {"    uts: host\n", "COMPOSE_UNSAFE_UTS_HOST"},
		"cgroup host":             {"    cgroup: host\n", "COMPOSE_UNSAFE_CGROUP_HOST"},
		"userns host":             {"    userns_mode: host\n", "COMPOSE_UNSAFE_USERNS_HOST"},
		"network host":            {"    network_mode: host\n", "COMPOSE_UNSAFE_NETWORK_HOST"},
		"cap_add":                 {"    cap_add: [NET_ADMIN]\n", "COMPOSE_UNSAFE_CAP_ADD"},
		"devices":                 {"    devices: ['/dev/sda:/dev/sda']\n", "COMPOSE_UNSAFE_DEVICES"},
		"gpu reservation":         {"    deploy:\n      resources:\n        reservations:\n          devices:\n            - driver: nvidia\n", "COMPOSE_UNSAFE_DEVICES"},
		"seccomp unconfined":      {"    security_opt: ['seccomp=unconfined']\n", "COMPOSE_UNSAFE_SECURITY_OPT"},
		"seccomp unconfined (:)":  {"    security_opt: ['seccomp:unconfined']\n", "COMPOSE_UNSAFE_SECURITY_OPT"},
		"apparmor unconfined":     {"    security_opt: ['apparmor=unconfined']\n", "COMPOSE_UNSAFE_SECURITY_OPT"},
		"the api socket":          {"    use_api_socket: true\n", "COMPOSE_UNSAFE_DOCKER_SOCKET"},
		"a privileged post_start": {"    post_start:\n      - command: id\n        privileged: true\n", "COMPOSE_UNSAFE_PRIVILEGED"},
	} {
		t.Run(name, func(t *testing.T) {
			_, code := check(t, compose(tc.extra), policy.Options{})
			assert.Equal(t, tc.code, code)
			_, code = check(t, compose(tc.extra), policy.Options{AllowUnsafe: true})
			assert.Empty(t, code, "the administrator can open the policy for a target")
		})
	}
}

func TestSafeOptionsAreNotRefused(t *testing.T) {
	for name, extra := range map[string]string{
		"privileged false":   "    privileged: false\n",
		"network bridge":     "    network_mode: bridge\n",
		"no new privileges":  "    security_opt: ['no-new-privileges:true']\n",
		"capabilities drop":  "    cap_drop: [ALL]\n",
		"a tmpfs":            "    volumes:\n      - type: tmpfs\n        target: /tmp\n",
		"a named volume":     "    volumes:\n      - type: volume\n        source: data\n        target: /data\n",
		"a bind in the path": "    volumes:\n      - type: bind\n        source: " + deployDir + "/shared/x\n        target: /x\n",
		"an env file":        "    env_file:\n      - .env\n      - path: ./other.env\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, code := check(t, compose(extra), policy.Options{})
			assert.Empty(t, code)
		})
	}
}

func TestBindMountsAreCheckedBeforeTheServerIsTouched(t *testing.T) {
	for name, tc := range map[string]struct{ volume, code string }{
		"the root":                  {"/:/host", "COMPOSE_UNSAFE_BIND_ROOT"},
		"the root with dots":        {"/a/..:/host", "COMPOSE_UNSAFE_BIND_ROOT"},
		"the docker socket":         {"/var/run/docker.sock:/var/run/docker.sock", "COMPOSE_UNSAFE_DOCKER_SOCKET"},
		"the other docker socket":   {"/run/docker.sock:/s", "COMPOSE_UNSAFE_DOCKER_SOCKET"},
		"a socket in the path":      {deployDir + "/shared/docker.sock:/s", "COMPOSE_UNSAFE_DOCKER_SOCKET"},
		"etc":                       {"/etc:/etc:ro", "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"home":                      {"~/keys:/keys", "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"the deploy path itself":    {deployDir + ":/d", "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"a sibling path":            {deployDir + "-other/x:/d", "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"the state of dhow":         {deployDir + "/.dhow:/d", "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"dots out of the deploy":    {deployDir + "/../etc:/d", "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"relative dots":             {"../../etc:/d", "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"relative dots after a dir": {"./a/../../etc:/d", "COMPOSE_UNSAFE_BIND_OUTSIDE"},
		"a variable":                {"${HOME}:/d", "COMPOSE_UNSAFE_INTERPOLATION"},
		"a variable in the path":    {"./${DIR}:/d", "COMPOSE_UNSAFE_INTERPOLATION"},
		"a link of the artifact":    {"./data/x:/d", "COMPOSE_UNSAFE_BIND_SYMLINK"},
	} {
		t.Run(name, func(t *testing.T) {
			raw := compose("    volumes:\n      - '" + tc.volume + "'\n")
			_, code := check(t, raw, policy.Options{Links: []string{"data"}})
			assert.Equal(t, tc.code, code)
		})
	}
}

func TestLongSyntaxBindMountsFollowTheSameRules(t *testing.T) {
	for name, extra := range map[string]string{
		"a bind of the root":  "    volumes:\n      - type: bind\n        source: /\n        target: /host\n",
		"a bind without type": "    volumes:\n      - source: /etc\n        target: /host\n",
		"a named pipe":        "    volumes:\n      - type: npipe\n        source: x\n        target: y\n",
		"volumes as a map":    "    volumes:\n      a: b\n",
		"an env file outside": "    env_file: /etc/shadow\n",
		"an env file list":    "    env_file:\n      - path: /etc/shadow\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, code := check(t, compose(extra), policy.Options{})
			assert.True(t, strings.HasPrefix(code, "COMPOSE_UNSAFE_"), code)
		})
	}
}

func TestTopLevelDefinitionsThatReachTheHostAreChecked(t *testing.T) {
	for name, tail := range map[string]string{
		"a volume bound to /":  "volumes:\n  v:\n    driver_opts:\n      type: none\n      o: bind\n      device: /\n",
		"a volume bound to ..": "volumes:\n  v:\n    driver_opts:\n      device: ../x\n",
		"a secret file":        "secrets:\n  s:\n    file: /etc/shadow\n",
		"a config file":        "configs:\n  c:\n    file: /root/.ssh/id_ed25519\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, code := check(t, compose(tail), policy.Options{})
			assert.True(t, strings.HasPrefix(code, "COMPOSE_UNSAFE_"), code)
			_, code = check(t, compose(tail), policy.Options{AllowUnsafe: true})
			assert.Empty(t, code)
		})
	}
	_, code := check(t, compose("secrets:\n  s:\n    file: ./secret.txt\n"), policy.Options{})
	assert.Empty(t, code)
}

func TestAFileThatIsNotAComposeFileIsInvalid(t *testing.T) {
	for name, raw := range map[string]string{
		"empty":                "",
		"a list":               "- a\n- b\n",
		"no services":          "version: '3'\n",
		"services is a list":   "services:\n  - web\n",
		"a service is a list":  "services:\n  web: [a]\n",
		"two documents":        "services:\n  web:\n    image: " + webDigest + "\n---\nservices: {}\n",
		"a duplicate key":      "services:\n  web:\n    image: " + webDigest + "\n    privileged: false\n    privileged: true\n",
		"broken yaml":          "services: [\n",
		"a bad service name":   "services:\n  'we b':\n    image: " + webDigest + "\n",
		"a number as the file": "42",
	} {
		t.Run(name, func(t *testing.T) {
			_, code := check(t, []byte(raw), policy.Options{})
			assert.Equal(t, policy.CodeInvalid, code)
		})
	}
	_, code := check(t, []byte(strings.Repeat("#", policy.MaxFileBytes+1)), policy.Options{})
	assert.Equal(t, policy.CodeInvalid, code)
}

func TestYamlAnchorsAndMergeKeysCannotHideAnOption(t *testing.T) {
	raw := "x-base: &base\n  privileged: true\nservices:\n  web:\n    <<: *base\n    image: " + webDigest + "\n"
	_, code := check(t, []byte(raw), policy.Options{})
	assert.Equal(t, "COMPOSE_UNSAFE_PRIVILEGED", code)
}

func TestImagesInputParsing(t *testing.T) {
	got, err := policy.ParseImages("web=" + webDigest + ",\n db=" + dbDigest)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"web": webDigest, "db": dbDigest}, got)

	empty, err := policy.ParseImages("")
	require.NoError(t, err)
	assert.Empty(t, empty)

	for _, bad := range []string{"web", "web=ghcr.io/acme/web:1", "=" + webDigest, "we b=" + webDigest, "web=" + webDigest + ",web=" + dbDigest} {
		_, err := policy.ParseImages(bad)
		require.ErrorIs(t, err, policy.ErrImages, bad)
	}
}

func TestTheOverrideFileHoldsExactlyTheImages(t *testing.T) {
	raw, err := policy.Override(map[string]string{"web": webDigest, "db": dbDigest})
	require.NoError(t, err)
	assert.Equal(t, "services:\n    db:\n        image: "+dbDigest+"\n    web:\n        image: "+webDigest+"\n", string(raw))

	_, err = policy.Override(map[string]string{"web": "ghcr.io/acme/web:1"})
	require.ErrorIs(t, err, policy.ErrImages)
	_, err = policy.Override(map[string]string{"we: b": webDigest})
	require.ErrorIs(t, err, policy.ErrImages)

	res, code := check(t, raw, policy.Options{})
	require.Empty(t, code)
	assert.Len(t, res.Images, 2)
}
