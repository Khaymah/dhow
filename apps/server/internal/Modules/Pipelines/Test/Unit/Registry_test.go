package unit_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pipelines "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines"
	"github.com/Khaymah/dhow/internal/netguard"
)

var indexDigest = "sha256:" + strings.Repeat("c", 64)

func registryHandler(manifest []byte) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("scope") != "repository:library/alpine:pull" {
			http.Error(w, "bad scope", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"token":"pull-token"}`))
	})
	mux.HandleFunc("/v2/library/alpine/manifests/3", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer pull-token" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="https://auth.example.com/token",service="registry",scope="repository:library/alpine:pull"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", indexDigest)
	})
	mux.HandleFunc("/v2/team/app/manifests/v1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write(manifest)
		}
	})
	mux.HandleFunc("/v2/team/huge/manifests/v1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(strings.Repeat("x", 5<<20)))
		}
	})
	mux.HandleFunc("/v2/team/slow/manifests/v1", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	})
	mux.HandleFunc("/v2/team/moved/manifests/v1", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.example.com/v2/team/moved/manifests/v1", http.StatusFound)
	})
	mux.HandleFunc("/v2/team/insecure/manifests/v1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="http://auth.example.com/token",service="registry"`)
		w.WriteHeader(http.StatusUnauthorized)
	})
	return mux
}

type fakeRegistry struct {
	resolver pipelines.Registry
}

func newRegistry(t *testing.T, allow bool, address string) fakeRegistry {
	t.Helper()
	manifest := []byte(`{"schemaVersion":2}`)
	srv := httptest.NewTLSServer(registryHandler(manifest))
	t.Cleanup(srv.Close)
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	transport, ok := srv.Client().Transport.(*http.Transport)
	require.True(t, ok)
	policy := netguard.Policy{}
	if allow {
		allowed, err := netguard.ParseCIDRs("10.255.0.0/16")
		require.NoError(t, err)
		policy.Allowed = allowed
	}
	dialer := netguard.Dialer{
		Policy: policy,
		Lookup: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr(address)}, nil
		},
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
		},
	}
	return fakeRegistry{resolver: pipelines.Registry{
		Dialer: dialer, TLS: &tls.Config{RootCAs: transport.TLSClientConfig.RootCAs, MinVersion: tls.VersionTLS12}, Timeout: 10 * time.Second,
	}}
}

func TestTheRegistryPinsATagThroughTheGuard(t *testing.T) {
	r := newRegistry(t, true, "10.255.0.20")
	got, err := r.resolver.Resolve(context.Background(), "registry.example.com/library/alpine:3")
	require.NoError(t, err)
	assert.Equal(t, "registry.example.com/library/alpine:3@"+indexDigest, got, "an anonymous pull token is fetched and used")

	got, err = r.resolver.Resolve(context.Background(), "registry.example.com/team/app:v1")
	require.NoError(t, err)
	sum := sha256.Sum256([]byte(`{"schemaVersion":2}`))
	assert.Equal(t, "registry.example.com/team/app:v1@sha256:"+hex.EncodeToString(sum[:]), got, "without a digest header the manifest is hashed")
}

func TestTheRegistryRefusesWhatItShouldNotReach(t *testing.T) {
	tests := []struct {
		name    string
		allow   bool
		address string
		image   string
		want    error
	}{
		{"a private address that is not allowed", false, "10.255.0.20", "registry.example.com/library/alpine:3", netguard.ErrBlocked},
		{"loopback", true, "127.0.0.1", "registry.example.com/library/alpine:3", netguard.ErrBlocked},
		{"cloud metadata", true, "169.254.169.254", "registry.example.com/library/alpine:3", netguard.ErrBlocked},
		{"a manifest over the size limit", true, "10.255.0.20", "registry.example.com/team/huge:v1", netguard.ErrTooLarge},
		{"a redirect", true, "10.255.0.20", "registry.example.com/team/moved:v1", netguard.ErrRedirect},
		{"a token address over http", true, "10.255.0.20", "registry.example.com/team/insecure:v1", pipelines.ErrRegistry},
		{"a missing tag", true, "10.255.0.20", "registry.example.com/team/none:v1", pipelines.ErrImageNotFound},
		{"an image already pinned", true, "10.255.0.20", "registry.example.com/team/app:v1@" + indexDigest, pipelines.ErrRegistry},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRegistry(t, tt.allow, tt.address)
			_, err := r.resolver.Resolve(context.Background(), tt.image)
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestTheRegistryCallHasATimeLimit(t *testing.T) {
	r := newRegistry(t, true, "10.255.0.20")
	r.resolver.Timeout = 300 * time.Millisecond
	start := time.Now()
	_, err := r.resolver.Resolve(context.Background(), "registry.example.com/team/slow:v1")
	require.Error(t, err)
	assert.Less(t, time.Since(start), 1500*time.Millisecond)
}
