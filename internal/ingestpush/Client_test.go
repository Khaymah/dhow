package ingestpush_test

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/ingestpush"
)

func pushError(t *testing.T, err error) *ingestpush.Error {
	t.Helper()
	var e *ingestpush.Error
	require.ErrorAs(t, err, &e)
	assert.NotContains(t, err.Error(), testToken, "an error must never carry the token")
	return e
}

func TestAPushSendsTheProtocolTheServerExpects(t *testing.T) {
	path, digest := writeArchive(t, map[string]string{"index.html": "<h1>hi</h1>"})
	srv := newFake(t, stored(digest, true))
	o := options(srv.URL + "/")
	o.Meta.RefKind = ingestpush.RefKindTag
	o.Meta.Ref = "v1.2.0"
	o.Meta.NoTrigger = true
	o.Meta.Provenance = map[string]string{"Provider": "github-actions", "RunId": "42"}
	o.Lang = "ar"

	res, err := ingestpush.Push(context.Background(), o, path)
	require.NoError(t, err)

	reqs := srv.requests()
	require.Len(t, reqs, 1)
	got := reqs[0]
	assert.Equal(t, http.MethodPut, got.Method)
	assert.Equal(t, "/ingest/v1/artifacts/"+digest, got.Path)
	assert.Equal(t, "Bearer "+testToken, got.Header.Get("Authorization"))
	assert.Equal(t, "application/gzip", got.Header.Get("Content-Type"))
	assert.Equal(t, "web", got.Header.Get(ingestpush.HeaderName))
	assert.Equal(t, "v1.2.0", got.Header.Get(ingestpush.HeaderRef))
	assert.Equal(t, "tag", got.Header.Get(ingestpush.HeaderRefKind))
	assert.Equal(t, testSha, got.Header.Get(ingestpush.HeaderSha))
	assert.Equal(t, "false", got.Header.Get(ingestpush.HeaderTrigger))
	assert.JSONEq(t, `{"Provider":"github-actions","RunId":"42"}`, got.Header.Get(ingestpush.HeaderProvenance))
	assert.Equal(t, "ar", got.Header.Get("Accept-Language"))
	assert.Positive(t, got.Length, "the length is sent, not a chunked body")
	assert.Empty(t, got.Header.Get("Transfer-Encoding"))
	assert.Equal(t, digest, got.Digest, "the bytes on the wire hash to the digest in the path")

	assert.Equal(t, digest, res.Digest)
	assert.True(t, res.Created)
	assert.True(t, res.Triggered)
	assert.Equal(t, "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b", res.RunID)
	assert.EqualValues(t, 7, res.RunNumber)
	assert.Equal(t, 1, res.Attempts)
}

func TestAFolderIsPackedAndSentWithItsOwnDigest(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hi</h1>"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("1"), 0o600))
	pkg, err := ingestpush.Prepare(dir, ingestpush.PrepareOptions{})
	require.NoError(t, err)
	defer pkg.Close()
	srv := newFake(t, stored(pkg.Digest, true))

	res, err := ingestpush.Push(context.Background(), options(srv.URL), dir)
	require.NoError(t, err)
	assert.Equal(t, pkg.Digest, res.Digest, "packing the same folder twice gives the same digest")
	assert.Equal(t, pkg.Digest, srv.requests()[0].Digest)
}

func TestADuplicateAnswers200AndIsNotACreation(t *testing.T) {
	path, digest := writeArchive(t, map[string]string{"a": "b"})
	srv := newFake(t, stored(digest, false))
	res, err := ingestpush.Push(context.Background(), options(srv.URL), path)
	require.NoError(t, err)
	assert.False(t, res.Created)
}

func TestEveryErrorCodeHasAClassAndTheRightRetryBehaviour(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		code     string
		class    ingestpush.Class
		attempts int
	}{
		{"invalid token", 401, ingestpush.CodeIngestTokenInvalid, ingestpush.ClassToken, 1},
		{"revoked token", 401, ingestpush.CodeIngestTokenRevoked, ingestpush.ClassToken, 1},
		{"expired token", 401, ingestpush.CodeIngestTokenExpired, ingestpush.ClassToken, 1},
		{"ref not allowed", 403, ingestpush.CodeRefNotAllowed, ingestpush.ClassToken, 1},
		{"digest mismatch", 422, ingestpush.CodeDigestMismatch, ingestpush.ClassRefused, 1},
		{"archive unsafe", 422, ingestpush.CodeArchiveUnsafe, ingestpush.ClassRefused, 1},
		{"validation", 422, ingestpush.CodeValidationFailed, ingestpush.ClassRefused, 1},
		{"too large", 413, ingestpush.CodeArtifactTooLarge, ingestpush.ClassRefused, 1},
		{"org quota", 413, ingestpush.CodeArtifactQuota, ingestpush.ClassRefused, 1},
		{"token quota", 413, ingestpush.CodeIngestTokenQuota, ingestpush.ClassRefused, 1},
		{"length required", 411, ingestpush.CodeLengthRequired, ingestpush.ClassRefused, 1},
		{"project archived", 409, ingestpush.CodeProjectArchived, ingestpush.ClassRefused, 1},
		{"too slow", 408, ingestpush.CodeUploadTooSlow, ingestpush.ClassTemporary, 3},
		{"rate limited", 429, ingestpush.CodeRateLimited, ingestpush.ClassTemporary, 3},
		{"unavailable", 503, ingestpush.CodeServiceUnavailable, ingestpush.ClassTemporary, 3},
		{"internal", 500, "INTERNAL_ERROR", ingestpush.ClassTemporary, 3},
		{"unknown 4xx", 418, "TEAPOT", ingestpush.ClassRefused, 1},
		{"unknown 401", 401, "SOMETHING_NEW", ingestpush.ClassToken, 1},
	}
	path, _ := writeArchive(t, map[string]string{"a": "b"})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newFake(t, failure(tc.status, tc.code, ""))
			o := options(srv.URL)
			o.Retries = 2
			_, err := ingestpush.Push(context.Background(), o, path)
			e := pushError(t, err)
			assert.Equal(t, tc.class, e.Class)
			assert.Equal(t, tc.code, e.Code)
			assert.Equal(t, tc.status, e.Status)
			assert.Equal(t, tc.attempts, e.Attempts)
			assert.Len(t, srv.requests(), tc.attempts)
		})
	}
}

func TestATemporaryFailureIsRetriedAndTheUploadThenSucceeds(t *testing.T) {
	path, digest := writeArchive(t, map[string]string{"a": "b"})
	ok := stored(digest, true)
	srv := newFake(t, func(n int, w http.ResponseWriter, r *http.Request) {
		if n < 3 {
			failure(503, ingestpush.CodeServiceUnavailable, "7")(n, w, r)
			return
		}
		ok(n, w, r)
	})
	var waits []time.Duration
	var retries []ingestpush.Retry
	o := options(srv.URL)
	o.Sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	o.OnRetry = func(r ingestpush.Retry) { retries = append(retries, r) }

	res, err := ingestpush.Push(context.Background(), o, path)
	require.NoError(t, err)
	assert.Equal(t, 3, res.Attempts)
	assert.Equal(t, []time.Duration{7 * time.Second, 7 * time.Second}, waits, "Retry-After is obeyed")
	require.Len(t, retries, 2)
	assert.Equal(t, ingestpush.Retry{Attempt: 1, Of: 4, Wait: 7 * time.Second, Code: ingestpush.CodeServiceUnavailable}, retries[0])
	for _, r := range srv.requests() {
		assert.Equal(t, digest, r.Digest, "every attempt sends the same bytes")
	}
}

func TestBackoffDoublesWithoutRetryAfter(t *testing.T) {
	path, _ := writeArchive(t, map[string]string{"a": "b"})
	srv := newFake(t, failure(500, "INTERNAL_ERROR", ""))
	var waits []time.Duration
	o := options(srv.URL)
	o.Retries = 4
	o.Sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	_, err := ingestpush.Push(context.Background(), o, path)
	e := pushError(t, err)
	assert.Equal(t, 5, e.Attempts)
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}, waits)
}

func TestALongRetryAfterStopsInsteadOfWaiting(t *testing.T) {
	path, _ := writeArchive(t, map[string]string{"a": "b"})
	srv := newFake(t, failure(429, ingestpush.CodeRateLimited, "3000"))
	_, err := ingestpush.Push(context.Background(), options(srv.URL), path)
	e := pushError(t, err)
	assert.Equal(t, ingestpush.ClassTemporary, e.Class)
	assert.Equal(t, 50*time.Minute, e.RetryAfter)
	assert.Len(t, srv.requests(), 1)
}

func TestZeroRetriesMeansOneAttempt(t *testing.T) {
	path, _ := writeArchive(t, map[string]string{"a": "b"})
	srv := newFake(t, failure(503, ingestpush.CodeServiceUnavailable, ""))
	o := options(srv.URL)
	o.Retries = -1
	_, err := ingestpush.Push(context.Background(), o, path)
	assert.Equal(t, ingestpush.CodeInput, pushError(t, err).Code)

	o.Retries = 0
	_, err = ingestpush.Push(context.Background(), o, path)
	assert.Equal(t, 1, pushError(t, err).Attempts)
}

func TestANetworkFailureIsRetriedThenReported(t *testing.T) {
	path, _ := writeArchive(t, map[string]string{"a": "b"})
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	o := options(url)
	o.Retries = 2
	var slept int
	o.Sleep = func(context.Context, time.Duration) error { slept++; return nil }
	_, err := ingestpush.Push(context.Background(), o, path)
	e := pushError(t, err)
	assert.Equal(t, ingestpush.ClassTemporary, e.Class)
	assert.Equal(t, ingestpush.CodeNetwork, e.Code)
	assert.Equal(t, 3, e.Attempts)
	assert.Equal(t, 2, slept)
}

func TestACancelledContextStopsTheRetries(t *testing.T) {
	path, _ := writeArchive(t, map[string]string{"a": "b"})
	srv := newFake(t, failure(503, ingestpush.CodeServiceUnavailable, ""))
	ctx, cancel := context.WithCancel(context.Background())
	o := options(srv.URL)
	o.Sleep = func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() }
	_, err := ingestpush.Push(ctx, o, path)
	e := pushError(t, err)
	assert.ErrorIs(t, e, context.Canceled)
	assert.Len(t, srv.requests(), 1)
}

func TestARedirectIsNeverFollowedSoTheTokenStaysHome(t *testing.T) {
	path, _ := writeArchive(t, map[string]string{"a": "b"})
	var hits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer other.Close()
	srv := newFake(t, func(_ int, w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
	})
	_, err := ingestpush.Push(context.Background(), options(srv.URL), path)
	e := pushError(t, err)
	assert.Equal(t, ingestpush.ClassUnexpected, e.Class)
	assert.Equal(t, ingestpush.CodeRedirect, e.Code)
	assert.Zero(t, hits.Load())
}

func TestAnAnswerThatIsNotAnIngestReplyIsUnexpected(t *testing.T) {
	path, digest := writeArchive(t, map[string]string{"a": "b"})
	cases := map[string]func(int, http.ResponseWriter, *http.Request){
		"html":           func(_ int, w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>welcome</html>")) },
		"success false":  func(_ int, w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"Success":false}`)) },
		"other digest":   stored("0000000000000000000000000000000000000000000000000000000000000000", true),
		"bad run id":     body(`{"Success":true,"Data":{"Digest":"` + digest + `","RunId":"x\ny"}}`),
		"bad run url":    body(`{"Success":true,"Data":{"Digest":"` + digest + `","RunUrl":"javascript:alert(1)"}}`),
		"control in url": body(`{"Success":true,"Data":{"Digest":"` + digest + `","RunUrl":"https://a.example/\u0007"}}`),
		"negative size":  body(`{"Success":true,"Data":{"Digest":"` + digest + `","SizeBytes":-1}}`),
	}
	for name, reply := range cases {
		t.Run(name, func(t *testing.T) {
			srv := newFake(t, reply)
			_, err := ingestpush.Push(context.Background(), options(srv.URL), path)
			e := pushError(t, err)
			assert.Equal(t, ingestpush.ClassUnexpected, e.Class)
			assert.Len(t, srv.requests(), 1, "an upload the server accepted is never repeated")
		})
	}
}

func body(s string) func(int, http.ResponseWriter, *http.Request) {
	return func(_ int, w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(s)) }
}

func TestAProxyPageForATooLargeBodyIsReadAsTooLarge(t *testing.T) {
	path, _ := writeArchive(t, map[string]string{"a": "b"})
	srv := newFake(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = w.Write([]byte("<html>413 Request Entity Too Large</html>"))
	})
	_, err := ingestpush.Push(context.Background(), options(srv.URL), path)
	e := pushError(t, err)
	assert.Equal(t, ingestpush.ClassRefused, e.Class)
	assert.Equal(t, ingestpush.CodeProxyLimit, e.Code)
}

func TestTLSNeedsATrustedCertificateOrTheCABundle(t *testing.T) {
	path, digest := writeArchive(t, map[string]string{"a": "b"})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stored(digest, true)(1, w, r)
	}))
	defer srv.Close()
	o := options(srv.URL)

	_, err := ingestpush.Push(context.Background(), o, path)
	e := pushError(t, err)
	assert.Equal(t, ingestpush.ClassInput, e.Class)
	assert.Equal(t, ingestpush.CodeTLS, e.Code)

	bundle := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(bundle, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600))
	o.CAFile = bundle
	res, err := ingestpush.Push(context.Background(), o, path)
	require.NoError(t, err)
	assert.Equal(t, digest, res.Digest)
}

func TestACABundleWithoutACertificateIsRefused(t *testing.T) {
	path, _ := writeArchive(t, map[string]string{"a": "b"})
	bundle := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(bundle, []byte("not a certificate"), 0o600))
	o := options("https://dhow.example.com")
	for _, file := range []string{bundle, filepath.Join(t.TempDir(), "missing.pem")} {
		o.CAFile = file
		_, err := ingestpush.Push(context.Background(), o, path)
		e := pushError(t, err)
		assert.Equal(t, ingestpush.ClassInput, e.Class)
		assert.ErrorIs(t, e, ingestpush.ErrCAFile)
	}
}

func TestTheProxyIsUsedForAnHTTPSServer(t *testing.T) {
	path, _ := writeArchive(t, map[string]string{"a": "b"})
	connects := make(chan string, 4)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			connects <- r.Host
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	o := options("https://ingest.example.test")
	o.Retries = 0
	o.Proxy = func(*http.Request) (*url.URL, error) { return proxyURL, nil }

	_, err = ingestpush.Push(context.Background(), o, path)
	e := pushError(t, err)
	assert.Equal(t, ingestpush.CodeNetwork, e.Code)
	select {
	case host := <-connects:
		assert.Equal(t, "ingest.example.test:443", host)
	default:
		t.Fatal("the request did not go through the proxy")
	}
}

func TestTheDefaultProxyReadsTheEnvironment(t *testing.T) {
	path, _ := writeArchive(t, map[string]string{"a": "b"})
	connects := make(chan string, 4)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			connects <- r.Host
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("https_proxy", proxy.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	o := options("https://ingest.example.test")
	o.Retries = 0
	_, err := ingestpush.Push(context.Background(), o, path)
	require.Error(t, err)
	select {
	case host := <-connects:
		assert.Equal(t, "ingest.example.test:443", host)
	default:
		t.Skip("the process cached its proxy settings before this test")
	}
}

func TestBadInputFailsBeforeAnyRequest(t *testing.T) {
	path, _ := writeArchive(t, map[string]string{"a": "b"})
	srv := newFake(t, failure(500, "X_Y_Z", ""))
	cases := map[string]func(*ingestpush.Options){
		"no token":      func(o *ingestpush.Options) { o.Token = "" },
		"short token":   func(o *ingestpush.Options) { o.Token = ingestpush.TokenPrefix + "short" },
		"wrong prefix":  func(o *ingestpush.Options) { o.Token = "ghp_" + testToken[len(ingestpush.TokenPrefix):] },
		"bad name":      func(o *ingestpush.Options) { o.Meta.Name = "../x" },
		"bad ref":       func(o *ingestpush.Options) { o.Meta.Ref = "a..b" },
		"tag mismatch":  func(o *ingestpush.Options) { o.Meta.RefKind = "tag"; o.Meta.Ref = "refs/heads/main" },
		"bad kind":      func(o *ingestpush.Options) { o.Meta.RefKind = "pull" },
		"bad sha":       func(o *ingestpush.Options) { o.Meta.Sha = "ABC" },
		"zero sha":      func(o *ingestpush.Options) { o.Meta.Sha = "0000000000000000000000000000000000000000" },
		"bad server":    func(o *ingestpush.Options) { o.Server = "ftp://x" },
		"http not here": func(o *ingestpush.Options) { o.Server = "http://dhow.example.com" },
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			o := options(srv.URL)
			edit(&o)
			_, err := ingestpush.Push(context.Background(), o, path)
			assert.Equal(t, ingestpush.ClassInput, pushError(t, err).Class)
			assert.Empty(t, srv.requests())
		})
	}
}
