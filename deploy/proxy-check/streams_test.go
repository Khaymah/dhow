package proxycheck_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	streamPattern = `^/api/v1/organizations/[^/]+/(jobs/[^/]+/logs/stream|events)$`
	streamPath    = "/api/v1/organizations/01a1806c-0d24-7b1e-a5d9-8cae4bbd8563/jobs/01a1806c-0d3f-7378-9416-e36ec6297cc0/logs/stream"
)

const eventsPath = "/api/v1/organizations/01a1806c-0d24-7b1e-a5d9-8cae4bbd8563/events"

func fixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	require.NoError(t, err)
	return string(raw)
}

func braced(t *testing.T, config, start string) string {
	t.Helper()
	i := strings.Index(config, start)
	require.GreaterOrEqual(t, i, 0, "missing %q", start)
	rest := config[i:]
	depth := 0
	for j, r := range rest {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return rest[:j+1]
			}
		}
	}
	t.Fatalf("unterminated block %q", start)
	return ""
}

func TestTheStreamPatternMatchesOnlyTheLiveStreams(t *testing.T) {
	re := regexp.MustCompile(streamPattern)
	assert.True(t, re.MatchString(streamPath))
	assert.True(t, re.MatchString(eventsPath))
	for _, other := range []string{
		strings.TrimSuffix(streamPath, "/stream"),
		streamPath + "/x",
		"/api/v1/organizations/a/b/jobs/c/logs/stream",
		"/runner-api/dhow.runner.v1.LogService/Append",
		eventsPath + "/x",
		"/api/v1/organizations/a/b/events",
		"/api/v1/organizations/01a1806c-0d24-7b1e-a5d9-8cae4bbd8563/events-feed",
	} {
		assert.False(t, re.MatchString(other), other)
	}
}

func TestNginxStreamsLogsUnbufferedAndGivesRunnersTime(t *testing.T) {
	conf := fixture(t, "nginx/dhow.conf")
	stream := braced(t, conf, "location ~ "+streamPattern+" {")
	for _, want := range []string{"proxy_buffering off;", "proxy_cache off;", "proxy_read_timeout 1h;", "proxy_send_timeout 1h;"} {
		assert.Contains(t, stream, want)
	}
	runner := braced(t, conf, "location /runner-api/ {")
	assert.Contains(t, runner, "proxy_read_timeout 90s;")
	assert.NotContains(t, braced(t, conf, "location / {"), "proxy_buffering off", "only the stream path skips buffering")
}

func TestCaddyFlushesTheLogStreamAndGivesRunnersTime(t *testing.T) {
	conf := fixture(t, "caddy/Caddyfile")
	assert.Contains(t, conf, "@logstream path_regexp "+streamPattern+"\n")
	stream := braced(t, conf, "reverse_proxy @logstream")
	assert.Contains(t, stream, "flush_interval -1")
	assert.Contains(t, stream, "read_timeout 1h")
	rest := conf[strings.Index(conf, stream)+len(stream):]
	assert.Contains(t, braced(t, rest, "reverse_proxy 127.0.0.1:8080 {"), "read_timeout 90s")
}

func TestTraefikNeverCutsAStream(t *testing.T) {
	conf := fixture(t, "traefik/compose.traefik.yaml")
	assert.Contains(t, conf, "respondingTimeouts.writeTimeout=0s")
	assert.NotContains(t, conf, "buffering", "the buffering middleware would hold the log stream back")
}
