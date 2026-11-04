package unit_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secretstore "github.com/Khaymah/dhow/apps/server/internal/Core/SecretStore"
	pipelines "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines"
	"github.com/Khaymah/dhow/internal/netguard"
)

const (
	loginUser     = "robot"
	loginPassword = "a registry password that must stay home"
)

type seenAuth struct {
	mu   sync.Mutex
	seen map[string]string
}

func (s *seenAuth) record(path, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[path] = value
}

func (s *seenAuth) get(path string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen[path]
}

func basicOf(user, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))
}

func loginHandler(seen *seenAuth) http.Handler {
	mux := http.NewServeMux()
	want := basicOf(loginUser, loginPassword)
	mux.HandleFunc("/v2/private/basic/manifests/v1", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != want {
			w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", indexDigest)
	})
	mux.HandleFunc("/own-token", func(w http.ResponseWriter, r *http.Request) {
		seen.record("/own-token", r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") != want {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"token":"private-token"}`))
	})
	mux.HandleFunc("/foreign-token", func(w http.ResponseWriter, r *http.Request) {
		seen.record("/foreign-token", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"token":"private-token"}`))
	})
	for path, realm := range map[string]string{
		"/v2/private/own/manifests/v1":     "https://registry.example.com/own-token",
		"/v2/private/foreign/manifests/v1": "https://auth.docker.io/foreign-token",
	} {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer private-token" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="`+realm+`",service="registry"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Docker-Content-Digest", indexDigest)
		})
	}
	return mux
}

func testCertificate(t *testing.T, names ...string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: names[0]}, DNSNames: names,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

func loginRegistry(t *testing.T, seen *seenAuth) pipelines.Registry {
	t.Helper()
	cert, pool := testCertificate(t, "registry.example.com", "auth.docker.io")
	srv := httptest.NewUnstartedServer(loginHandler(seen))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	dialer := netguard.Dialer{
		Lookup: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("203.0.113.30")}, nil
		},
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
		},
	}
	return pipelines.Registry{Dialer: dialer, TLS: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, Timeout: 10 * time.Second}
}

func login(host string) *secretstore.RegistryLogin {
	return &secretstore.RegistryLogin{Host: host, Username: loginUser, Password: secretstore.Value(loginPassword)}
}

func TestARegistryLoginReachesTheRegistryHostOnly(t *testing.T) {
	seen := &seenAuth{seen: map[string]string{}}
	r := loginRegistry(t, seen)
	ctx := context.Background()

	got, err := r.ResolveWith(ctx, "registry.example.com/private/basic:v1", login("registry.example.com"))
	require.NoError(t, err)
	assert.Equal(t, "registry.example.com/private/basic:v1@"+indexDigest, got, "a Basic challenge from the registry gets the login")

	_, err = r.Resolve(ctx, "registry.example.com/private/basic:v1")
	require.Error(t, err, "without a login a Basic challenge fails")

	_, err = r.ResolveWith(ctx, "registry.example.com/private/basic:v1", login("other.example.com"))
	require.Error(t, err, "a login for another host is never sent")
	assert.NotContains(t, err.Error(), loginPassword)

	got, err = r.ResolveWith(ctx, "registry.example.com/private/own:v1", login("registry.example.com"))
	require.NoError(t, err)
	assert.Equal(t, "registry.example.com/private/own:v1@"+indexDigest, got)
	assert.Equal(t, basicOf(loginUser, loginPassword), seen.get("/own-token"), "a token realm on the registry host gets the login")

	got, err = r.ResolveWith(ctx, "registry.example.com/private/foreign:v1", login("registry.example.com"))
	require.NoError(t, err, "a known token service still answers anonymously")
	assert.Equal(t, "registry.example.com/private/foreign:v1@"+indexDigest, got)
	assert.Empty(t, seen.get("/foreign-token"), "a token realm on another host never gets the login")
}

func TestTheRegistryHostOfAnImage(t *testing.T) {
	for image, want := range map[string]string{
		"alpine:3":                      "registry-1.docker.io",
		"docker.io/library/alpine:3":    "registry-1.docker.io",
		"ghcr.io/team/app:v1":           "ghcr.io",
		"registry.example.com:5000/a:b": "registry.example.com:5000",
	} {
		got, ok := pipelines.RegistryHost(image)
		assert.True(t, ok, image)
		assert.Equal(t, want, got, image)
	}
	_, ok := pipelines.RegistryHost("alpine@sha256:" + indexDigest[7:])
	assert.False(t, ok, "an image by digest needs no pin")
}
