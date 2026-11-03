package installer

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
)

const repoRoot = "../../../../../"

func TestEmbeddedAssetsMatchTheDeployFiles(t *testing.T) {
	for embedded, path := range map[string]string{
		string(UnitFile):    "deploy/systemd/dhow.service",
		string(ComposeFile): "deploy/compose/compose.yaml",
	} {
		want, err := os.ReadFile(repoRoot + path)
		require.NoError(t, err)
		assert.Equal(t, string(want), embedded, "Assets/ and %s drifted apart; copy the deploy file into Assets/", path)
	}
}

func TestUnitUsesTheInstallerPaths(t *testing.T) {
	unit := string(UnitFile)
	for _, want := range []string{
		"User=" + ServiceUser, "ExecStart=" + BinaryPath, "EnvironmentFile=" + EnvFilePath,
		"LoadCredential=dhow-secret-key:" + SecretKeyPath, "LoadCredential=dhow-database-url:" + DatabaseURLPath,
		"LoadCredential=dhow-keyring:" + KeyringPath,
		"CapabilityBoundingSet=\n", "NoNewPrivileges=yes", "ProtectSystem=strict", "UMask=0077",
		"RestartPreventExitStatus=" + strconv.Itoa(migrations.ExitSchemaTooNew) + " " + strconv.Itoa(keys.ExitKeyring) + "\n",
	} {
		assert.Contains(t, unit, want)
	}
	assert.NotContains(t, unit, "Environment=DHOW_SECRET_KEY")
	assert.NotContains(t, unit, "DHOW_KEYRING")
}

func TestComposeMountsTheKeyringAsASecret(t *testing.T) {
	compose := string(ComposeFile)
	for _, want := range []string{
		"DHOW_KEYRING_FILE: /run/secrets/dhow-keyring\n", "    secrets:\n      - dhow-keyring\n",
		"secrets:\n  dhow-keyring:\n    file: ./secrets/dhow-keyring\n",
	} {
		assert.Contains(t, compose, want)
	}
	assert.NotContains(t, compose, "DHOW_KEYRING:")
	assert.Equal(t, ComposeDir+"/secrets/dhow-keyring", ComposeKeyring)
}

func TestProxyFixturesMatchPhaseA(t *testing.T) {
	for embedded, path := range map[string]string{
		string(NginxFixture): "deploy/proxy-check/nginx/dhow.conf",
		string(CaddyFixture): "deploy/proxy-check/caddy/Caddyfile",
	} {
		want, err := os.ReadFile(repoRoot + path)
		if os.IsNotExist(err) {
			t.Skipf("%s is not on this branch yet", path)
		}
		require.NoError(t, err)
		assert.Equal(t, string(want), embedded, "Assets/proxy and %s drifted apart; copy the tested fixture into Assets/proxy", path)
	}
}

func TestTemplates(t *testing.T) {
	a := Answers{Domain: "ci.dhow.test", ListenAddr: "127.0.0.1:9000", Proxy: ProxyAnswers{Kind: ProxyNginx, Email: "ops@example.com"}}
	tls := string(NginxTLS(a))
	for _, want := range []string{
		"proxy_set_header Host $http_host;", "proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;",
		"proxy_set_header X-Forwarded-Proto $scheme;", "ssl_certificate /etc/letsencrypt/live/ci.dhow.test/fullchain.pem;",
		"location /.well-known/acme-challenge/ {", "return 301 https://$host$request_uri;", "proxy_pass http://127.0.0.1:9000;",
		"server_name ci.dhow.test;",
	} {
		assert.Contains(t, tls, want)
	}
	assert.NotContains(t, tls, fixtureDomain)
	assert.NotContains(t, tls, "http2 on;", "distro nginx before 1.25.1 rejects http2 on")
	assert.NotContains(t, tls, "Origin", "the proxy must never touch Origin")
	assert.NotContains(t, string(NginxHTTPOnly(a)), "443")
	caddy := string(CaddySite(a))
	assert.True(t, strings.HasPrefix(caddy, "ci.dhow.test {\n"))
	assert.Contains(t, caddy, "reverse_proxy 127.0.0.1:9000")
	assert.Equal(t, string(CaddyFixture), ProxyCaddyConfig(Answers{Domain: fixtureDomain, ListenAddr: fixtureUpstream}))
	assert.Equal(t, string(NginxFixture), ProxyNginxConfig(Answers{Domain: fixtureDomain, ListenAddr: "0.0.0.0:8080"}))
	snippets := ExistingProxySnippets(a)
	assert.Contains(t, snippets, "http2 on;", "the printed config is the tested fixture, unchanged except names")
	assert.Contains(t, snippets, "reverse_proxy 127.0.0.1:9000")
}

func TestCaddyImportRoundTrip(t *testing.T) {
	stock := ":80 {\n\troot * /usr/share/caddy\n\tfile_server\n}\n"
	with := WithCaddyImport([]byte(stock))
	assert.Equal(t, "import /etc/caddy/dhow.caddyfile\n\n"+stock, string(with))
	assert.Equal(t, string(with), string(WithCaddyImport(with)), "adding the import twice changes nothing")
	assert.Equal(t, stock, string(WithoutCaddyImport(with)))
}

func TestEnvFiles(t *testing.T) {
	existing := ParseEnv([]byte("# note\nDHOW_LOG_LEVEL=debug\nDHOW_PUBLIC_URL=\"https://old.example.com\"\nEXTRA=1\n"))
	a := Answers{Domain: "new.example.com", ListenAddr: DefaultListen, Proxy: ProxyAnswers{Kind: ProxyCaddy}}
	got := string(nativeEnv(a, existing))
	assert.Equal(t, "DHOW_PUBLIC_URL=https://new.example.com\nDHOW_LISTEN_ADDR=127.0.0.1:8080\nDHOW_TRUSTED_PROXIES=127.0.0.1/32,::1/128\nDHOW_LOG_LEVEL=debug\nEXTRA=1\n", got)
	off := false
	upgraded := ParseEnv([]byte("DHOW_BREACHED_PASSWORD_CHECK=true\n"))
	assert.NotContains(t, string(nativeEnv(a, upgraded)), "DHOW_BREACHED_PASSWORD_CHECK", "the default is left to the dashboard, even on an upgrade")
	a.BreachedPasswordCheck = &off
	assert.Contains(t, string(nativeEnv(a, upgraded)), "DHOW_BREACHED_PASSWORD_CHECK=false\n", "turning the check off at install time stays in env")
	a.BreachedPasswordCheck = nil
	compose := string(composeEnv(a, ParseEnv([]byte("DHOW_IMAGE=ghcr.io/khaymah/dhow@sha256:abc\n")), "k", "p", ComposeImageFor("v1.2.3"), ""))
	assert.Contains(t, compose, "DHOW_IMAGE=ghcr.io/khaymah/dhow@sha256:abc\n", "a pinned image is kept")
	assert.Equal(t, "ghcr.io/khaymah/dhow:1.2.3", ComposeImageFor("v1.2.3"))
	assert.Equal(t, "ghcr.io/khaymah/dhow:latest", ComposeImageFor("dev"))
}

func TestOnlyTheImageWaitsOnASchemaTooNew(t *testing.T) {
	dockerfile, err := os.ReadFile(repoRoot + "deploy/docker/Dockerfile")
	require.NoError(t, err)
	assert.Contains(t, string(dockerfile), "\nENV DHOW_ON_INCOMPATIBLE=wait\n", "the image waits instead of exiting into Compose's restart loop")
	assert.NotContains(t, string(UnitFile), "DHOW_ON_INCOMPATIBLE", "systemd keeps exit code 78")
	assert.NotContains(t, string(ComposeFile), "DHOW_ON_INCOMPATIBLE")
	assert.Contains(t, string(ComposeFile), "    hostname: dhow\n", "a stable host name keeps one ServerInstances row across container restarts")
}
