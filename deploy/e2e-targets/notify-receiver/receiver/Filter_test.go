package receiver_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func count(t *testing.T, srv *httptest.Server, query string) string {
	t.Helper()
	res, err := http.Get(srv.URL + "/_count?" + query)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode, string(raw))
	return strings.TrimSpace(string(raw))
}

func TestTheCountFiltersMessagesByServiceTextKeySignatureAndMentions(t *testing.T) {
	_, srv := newServer(t)
	post(t, srv, "/_secret", hookSecret)
	post(t, srv, slackPath, `{"text":"فشل النشر &lt;!channel&gt;"}`)
	post(t, srv, discordPath, `{"content":"Deployment failed @everyone","allowed_mentions":{"parse":[]}}`)
	post(t, srv, discordPath, `{"content":"Deployment failed"}`)
	stamp, sig := sign(hookSecret, time.Now(), "{}")
	hook(t, srv, stamp, sig, "{}")
	hook(t, srv, stamp, "v1=wrong", "{}")

	assert.Equal(t, "5", count(t, srv, ""))
	assert.Equal(t, "1", count(t, srv, "service=slack"))
	assert.Equal(t, "1", count(t, srv, "service=slack&contains="+url.QueryEscape("فشل النشر")))
	assert.Equal(t, "0", count(t, srv, "service=slack&absent="+url.QueryEscape("&lt;!channel&gt;")))
	assert.Equal(t, "2", count(t, srv, "service=discord&contains=failed"))
	assert.Equal(t, "1", count(t, srv, "service=discord&mentions=none"))
	assert.Equal(t, "1", count(t, srv, "service=discord&contains=%40everyone&mentions=none"))
	assert.Equal(t, "1", count(t, srv, "service=webhook&signed=1"))
	assert.Equal(t, "2", count(t, srv, "service=webhook"))
	assert.Equal(t, "2", count(t, srv, "key=event-1"))
	assert.Equal(t, "0", count(t, srv, "key=event-2"))

	res, err := http.Get(srv.URL + "/_count?service=nope")
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}
