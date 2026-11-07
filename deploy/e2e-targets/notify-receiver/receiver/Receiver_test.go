package receiver_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/deploy/e2e-targets/notify-receiver/receiver"
)

const (
	slackPath    = "/services/T0123/B4567/abcdEFGH1234"
	discordPath  = "/api/webhooks/123456789012345678/abc_DEF-123"
	telegramPath = "/bot123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw/sendMessage"
)

func post(t *testing.T, srv *httptest.Server, path, body string) (int, string) {
	t.Helper()
	res, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(raw)
}

func newServer(t *testing.T) (*receiver.Receiver, *httptest.Server) {
	t.Helper()
	r := receiver.New()
	srv := httptest.NewServer(r.Handler())
	t.Cleanup(srv.Close)
	return r, srv
}

func TestSlackAcceptsATextAndRefusesTheRest(t *testing.T) {
	r, srv := newServer(t)
	status, body := post(t, srv, slackPath, `{"text":"hello"}`)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "ok", body)

	for name, tc := range map[string]struct {
		path, body, want string
		status           int
	}{
		"unknown hook":  {"/services/x/y", `{"text":"a"}`, "no_service", http.StatusNotFound},
		"no text":       {slackPath, `{}`, "no_text", http.StatusBadRequest},
		"not json":      {slackPath, `text=a`, "invalid_payload", http.StatusBadRequest},
		"too long text": {slackPath, `{"text":"` + strings.Repeat("a", 40001) + `"}`, "msg_too_long", http.StatusBadRequest},
	} {
		status, body := post(t, srv, tc.path, tc.body)
		assert.Equal(t, tc.status, status, name)
		assert.Equal(t, tc.want, body, name)
	}
	require.Len(t, r.Messages(), 1)
	assert.Equal(t, receiver.ServiceSlack, r.Messages()[0].Service)
	assert.Equal(t, "hello", r.Messages()[0].Text)
}

func TestDiscordKeepsTheMentionPolicyAndRefusesAnEmptyOrLongMessage(t *testing.T) {
	r, srv := newServer(t)
	status, _ := post(t, srv, discordPath, `{"content":"@everyone hi","allowed_mentions":{"parse":[]}}`)
	assert.Equal(t, http.StatusNoContent, status)
	status, body := post(t, srv, discordPath+"?wait=true", `{"content":"x"}`)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"id"`)

	status, body = post(t, srv, discordPath, `{"content":""}`)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, body, "50006")
	status, _ = post(t, srv, discordPath, `{"content":"`+strings.Repeat("a", 2001)+`"}`)
	assert.Equal(t, http.StatusBadRequest, status)
	status, _ = post(t, srv, discordPath, `{"content":"a","allowed_mentions":{"parse":["nobody"]}}`)
	assert.Equal(t, http.StatusBadRequest, status)
	status, body = post(t, srv, "/api/webhooks/abc/def", `{"content":"a"}`)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Contains(t, body, "Unknown Webhook")

	msgs := r.Messages()
	require.Len(t, msgs, 2)
	assert.True(t, msgs[0].HasMentions)
	assert.Empty(t, msgs[0].MentionsParse, "nothing is parsed into a mention, so @everyone stays text")
	assert.False(t, msgs[1].HasMentions)
}

func TestTelegramParsesHTMLLikeTheRealApiAndRefusesAnUnescapedMessage(t *testing.T) {
	r, srv := newServer(t)
	ok := `{"chat_id":"-100123","text":"<b>Failed</b>\nBranch: a &lt;!channel&gt; &amp; b","parse_mode":"HTML"}`
	status, body := post(t, srv, telegramPath, ok)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"ok":true`)

	numeric := `{"chat_id":42,"text":"plain <not html>"}`
	status, _ = post(t, srv, telegramPath, numeric)
	assert.Equal(t, http.StatusOK, status, "without a parse mode the text is plain")

	for name, tc := range map[string]struct {
		path, body string
		status     int
	}{
		"raw angle bracket": {telegramPath, `{"chat_id":"1","text":"a <!channel> b","parse_mode":"HTML"}`, http.StatusBadRequest},
		"raw ampersand":     {telegramPath, `{"chat_id":"1","text":"a & b","parse_mode":"HTML"}`, http.StatusBadRequest},
		"unknown tag":       {telegramPath, `{"chat_id":"1","text":"<script>x</script>","parse_mode":"HTML"}`, http.StatusBadRequest},
		"markdown mode":     {telegramPath, `{"chat_id":"1","text":"a","parse_mode":"Markdown"}`, http.StatusBadRequest},
		"no chat":           {telegramPath, `{"text":"a"}`, http.StatusBadRequest},
		"no text":           {telegramPath, `{"chat_id":"1"}`, http.StatusBadRequest},
		"too long":          {telegramPath, `{"chat_id":"1","text":"` + strings.Repeat("a", 4097) + `"}`, http.StatusBadRequest},
		"bad token":         {"/botnot-a-token/sendMessage", `{"chat_id":"1","text":"a"}`, http.StatusNotFound},
		"other method":      {"/bot123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw/getMe", `{}`, http.StatusNotFound},
	} {
		status, body := post(t, srv, tc.path, tc.body)
		assert.Equal(t, tc.status, status, name)
		assert.Contains(t, body, `"ok":false`, name)
	}
	msgs := r.Messages()
	require.Len(t, msgs, 2)
	assert.Equal(t, "-100123", msgs[0].ChatID)
	assert.Equal(t, "HTML", msgs[0].ParseMode)
	assert.Equal(t, "42", msgs[1].ChatID)
}

func TestAScriptedStatusAnswersBeforeTheMessageIsKept(t *testing.T) {
	r, srv := newServer(t)
	r.Script(receiver.ServiceSlack, http.StatusInternalServerError, http.StatusTooManyRequests)
	for _, want := range []int{http.StatusInternalServerError, http.StatusTooManyRequests, http.StatusOK} {
		status, _ := post(t, srv, slackPath, `{"text":"a"}`)
		assert.Equal(t, want, status)
	}
	assert.Equal(t, 3, r.Attempts(receiver.ServiceSlack))
	assert.Len(t, r.Messages(), 1, "only the accepted message is kept")

	r.Reset()
	assert.Empty(t, r.Messages())
	assert.Zero(t, r.Attempts(receiver.ServiceSlack))
}

func TestTheControlEndpointsListAndResetAndScript(t *testing.T) {
	_, srv := newServer(t)
	post(t, srv, slackPath, `{"text":"one"}`)

	res, err := http.Get(srv.URL + "/_messages")
	require.NoError(t, err)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&list))
	require.NoError(t, res.Body.Close())
	require.Len(t, list, 1)
	assert.Equal(t, "one", list[0]["Text"])
	for key := range list[0] {
		assert.NotContains(t, strings.ToLower(key), "path", "the listing never carries a webhook path or a token")
		assert.NotContains(t, strings.ToLower(key), "token")
	}

	status, _ := post(t, srv, "/_script?service=slack&status=503", "")
	assert.Equal(t, http.StatusNoContent, status)
	status, _ = post(t, srv, slackPath, `{"text":"two"}`)
	assert.Equal(t, http.StatusServiceUnavailable, status)
	status, _ = post(t, srv, "/_script?service=nope&status=503", "")
	assert.Equal(t, http.StatusBadRequest, status)
	status, _ = post(t, srv, "/_script?service=slack&status=abc", "")
	assert.Equal(t, http.StatusBadRequest, status)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/_messages", nil)
	require.NoError(t, err)
	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	assert.Equal(t, http.StatusNoContent, res.StatusCode)

	res, err = http.Get(srv.URL + "/slack-not-here")
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	assert.Equal(t, http.StatusNotFound, res.StatusCode)

	res, err = http.Get(srv.URL + slackPath)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	assert.Equal(t, http.StatusMethodNotAllowed, res.StatusCode)
}

func TestTheCertificateCoversTheThreeServiceHostsAndOnlyTheIssuedCaTrustsIt(t *testing.T) {
	r := receiver.New()
	cfg, caPEM, err := receiver.NewTLS(time.Now())
	require.NoError(t, err)
	srv := httptest.NewUnstartedServer(r.Handler())
	srv.TLS = cfg
	srv.StartTLS()
	t.Cleanup(srv.Close)

	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(caPEM))
	target := srv.Listener.Addr().String()
	for _, host := range []string{"hooks.slack.com", "discord.com", "discordapp.com", "api.telegram.org"} {
		client := &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, target)
			},
		}}
		res, err := client.Get("https://" + host + "/_health")
		require.NoError(t, err, host)
		require.NoError(t, res.Body.Close())
		assert.Equal(t, http.StatusNoContent, res.StatusCode)
	}

	strict := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: x509.NewCertPool(), MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, target)
		},
	}}
	refused, err := strict.Get("https://hooks.slack.com/_health")
	if refused != nil {
		require.NoError(t, refused.Body.Close())
	}
	require.Error(t, err, "a client that does not trust the generated CA refuses the receiver")
}
