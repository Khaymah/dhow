package deployrelease_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

func healthOf(url string) deployrelease.Health {
	return deployrelease.Health{URL: url, StatusMin: 200, StatusMax: 399, Timeout: time.Second, Retries: 1, Interval: time.Second, Stable: 1}
}

func check(t *testing.T, client *http.Client, url string) deployrelease.HealthResult {
	t.Helper()
	res, err := deployrelease.CheckHealth(context.Background(), client, "127.0.0.1", healthOf(url), fastClock())
	require.NoError(t, err)
	return res
}

func TestAHealthRedirectMustStayOnTheSameSchemeHostAndPort(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(ok.Close)
	var self string
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/same":
			http.Redirect(w, r, self+"/done", http.StatusFound)
		case "/other-port":
			http.Redirect(w, r, ok.URL+"/done", http.StatusFound)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(redirector.Close)
	self = redirector.URL
	client := deployrelease.NewHealthClient(http.DefaultTransport)

	assert.True(t, check(t, client, redirector.URL+"/same").Passed, "a redirect inside the same origin is followed")
	res := check(t, client, redirector.URL+"/other-port")
	assert.False(t, res.Passed, "the same host on another port is another origin")
	assert.Equal(t, deployrelease.ErrKindOther, res.LastError)
}

func TestAHealthRedirectFromHttpsToHttpIsRefused(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(plain.Close)
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	t.Cleanup(secure.Close)
	client := deployrelease.NewHealthClient(secure.Client().Transport)
	assert.False(t, check(t, client, secure.URL).Passed)
}
