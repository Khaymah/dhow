package ingestpush_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/ingestpush"
)

const (
	testSha   = "1111111111111111111111111111111111111111"
	testToken = ingestpush.TokenPrefix + "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789_-abcde"
)

func options(server string) ingestpush.Options {
	return ingestpush.Options{
		Server:  server,
		Token:   testToken,
		Meta:    ingestpush.Meta{Name: "web", Ref: "main", Sha: testSha},
		Retries: ingestpush.DefaultRetries,
		Sleep:   func(context.Context, time.Duration) error { return nil },
	}
}

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}))
		_, err := tw.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func writeArchive(t *testing.T, files map[string]string) (string, string) {
	t.Helper()
	data := tarGz(t, files)
	path := filepath.Join(t.TempDir(), "build.tar.gz")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	sum := sha256.Sum256(data)
	return path, hex.EncodeToString(sum[:])
}

type seen struct {
	Method  string
	Path    string
	Header  http.Header
	Length  int64
	Digest  string
	Request int
}

type fake struct {
	*httptest.Server
	mu    sync.Mutex
	seen  []seen
	reply func(n int, w http.ResponseWriter, r *http.Request)
}

func newFake(t *testing.T, reply func(n int, w http.ResponseWriter, r *http.Request)) *fake {
	t.Helper()
	f := &fake{reply: reply}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.Close)
	return f
}

func (f *fake) handle(w http.ResponseWriter, r *http.Request) {
	h := sha256.New()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r.Body)
	h.Write(buf.Bytes())
	f.mu.Lock()
	n := len(f.seen) + 1
	f.seen = append(f.seen, seen{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Length: r.ContentLength, Digest: hex.EncodeToString(h.Sum(nil)), Request: n})
	f.mu.Unlock()
	f.reply(n, w, r)
}

func (f *fake) requests() []seen {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]seen(nil), f.seen...)
}

func stored(digest string, created bool) func(int, http.ResponseWriter, *http.Request) {
	return func(_ int, w http.ResponseWriter, _ *http.Request) {
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"Success":true,"Data":{"Name":"web","Digest":"` + digest + `","SizeBytes":10,"Created":` + boolText(created) +
			`,"Triggered":true,"RunId":"0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b","RunNumber":7,"RunUrl":"https://dhow.example.com/runs/7"}}`))
	}
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func failure(status int, code string, retryAfter string) func(int, http.ResponseWriter, *http.Request) {
	return func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"Success":false,"Error":{"Code":"` + code + `","Message":"` + strings.Repeat("x", 3) + `"}}`))
	}
}
