package receiver_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/deploy/e2e-targets/notify-receiver/receiver"
)

const (
	hookPath   = "/hooks/demo"
	hookSecret = "dhow_whsec_0123456789abcdef0123456789abcdef0123456" //nolint:gosec // A made-up secret for a test. allow-secret
)

func sign(secret string, at time.Time, body string) (string, string) {
	stamp := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stamp + "." + body))
	return stamp, "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func hook(t *testing.T, srv *httptest.Server, stamp, signature, body string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+hookPath, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Dhow-Event", "run.finished")
	req.Header.Set("Dhow-Delivery", "delivery-1")
	req.Header.Set("Idempotency-Key", "event-1")
	req.Header.Set("Dhow-Timestamp", stamp)
	req.Header.Set("Dhow-Signature", signature)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	return res.StatusCode
}

func TestAWebhookIsKeptWithItsHeadersAndItsSignatureIsChecked(t *testing.T) {
	r, srv := newServer(t)
	status, _ := post(t, srv, "/_secret", hookSecret)
	assert.Equal(t, http.StatusNoContent, status)
	body := `{"Event":"run.finished"}`
	stamp, good := sign(hookSecret, time.Now(), body)
	assert.Equal(t, http.StatusOK, hook(t, srv, stamp, good, body))
	_, wrong := sign("dhow_whsec_other", time.Now(), body)
	assert.Equal(t, http.StatusOK, hook(t, srv, stamp, wrong, body))
	oldStamp, old := sign(hookSecret, time.Now().Add(-time.Hour), body)
	assert.Equal(t, http.StatusOK, hook(t, srv, oldStamp, old, body))

	msgs := r.Messages()
	require.Len(t, msgs, 3)
	assert.Equal(t, receiver.ServiceWebhook, msgs[0].Service)
	assert.Equal(t, body, msgs[0].Text)
	assert.Equal(t, "run.finished", msgs[0].Event)
	assert.Equal(t, "event-1", msgs[0].Idempotency)
	assert.True(t, msgs[0].Signed)
	assert.False(t, msgs[1].Signed, "a signature made with another secret is not valid")
	assert.False(t, msgs[2].Signed, "a signature older than the replay window is not valid")
}

func TestAWebhookWithoutASecretIsNeverSigned(t *testing.T) {
	r, srv := newServer(t)
	body := `{}`
	stamp, sig := sign("", time.Now(), body)
	assert.Equal(t, http.StatusOK, hook(t, srv, stamp, sig, body))
	require.Len(t, r.Messages(), 1)
	assert.False(t, r.Messages()[0].Signed)
}

func TestAHeldRequestIsCountedAtOnceAndKeptWhenItEnds(t *testing.T) {
	r, srv := newServer(t)
	status, _ := post(t, srv, "/_hold?service=webhook&ms=300", "")
	assert.Equal(t, http.StatusNoContent, status)
	for _, bad := range []string{"/_hold?service=nope&ms=1", "/_hold?service=webhook&ms=-1", "/_hold?service=webhook&ms=x", "/_hold?service=webhook&ms=3600000"} {
		status, _ = post(t, srv, bad, "")
		assert.Equal(t, http.StatusBadRequest, status, bad)
	}
	done := make(chan int, 1)
	go func() {
		res, err := http.Post(srv.URL+hookPath, "application/json", strings.NewReader("{}"))
		if err != nil {
			done <- 0
			return
		}
		_ = res.Body.Close()
		done <- res.StatusCode
	}()
	require.Eventually(t, func() bool { return r.Attempts(receiver.ServiceWebhook) == 1 }, 5*time.Second, 10*time.Millisecond)
	assert.Empty(t, r.Messages(), "a request that is still held is not accepted yet")
	assert.Equal(t, http.StatusOK, <-done)
	assert.Len(t, r.Messages(), 1)
}

func TestTheStatsCountAttemptsAndAcceptedMessages(t *testing.T) {
	_, srv := newServer(t)
	post(t, srv, "/_script?service=webhook&status=503", "")
	hook(t, srv, "1", "v1=x", "{}")
	hook(t, srv, "1", "v1=x", "{}")
	res, err := http.Get(srv.URL + "/_stats")
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	var stats struct {
		Attempts map[string]int
		Accepted map[string]int
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&stats))
	assert.Equal(t, 2, stats.Attempts["webhook"])
	assert.Equal(t, 1, stats.Accepted["webhook"])
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/_stats", nil)
	require.NoError(t, err)
	other, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, other.Body.Close())
	assert.Equal(t, http.StatusMethodNotAllowed, other.StatusCode)
}
