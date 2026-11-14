package middleware_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
)

type errorBody struct {
	Success bool            `json:"Success"`
	Data    json.RawMessage `json:"Data"`
	Error   *struct {
		Code     string `json:"Code"`
		Message  string `json:"Message"`
		Fix      string `json:"Fix"`
		DocsPath string `json:"DocsPath"`
	} `json:"Error"`
}

func assertErrorBody(t *testing.T, rec *httptest.ResponseRecorder, code string) {
	t.Helper()
	var body errorBody
	dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
	dec.DisallowUnknownFields()
	require.NoError(t, dec.Decode(&body))
	assert.False(t, body.Success)
	assert.Empty(t, body.Data)
	require.NotNil(t, body.Error)
	assert.Equal(t, code, body.Error.Code)
	assert.NotEmpty(t, body.Error.Message)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSecurityHeaders(t *testing.T) {
	tests := []struct {
		name     string
		options  middleware.SecurityOptions
		wantHSTS bool
	}{
		{"https", middleware.SecurityOptions{HSTS: true}, true},
		{"http", middleware.SecurityOptions{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := middleware.SecurityHeaders(tt.options)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			rec := serve(h, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
			want := map[string]string{
				"Content-Security-Policy":      middleware.ContentSecurityPolicy,
				"X-Content-Type-Options":       "nosniff",
				"Referrer-Policy":              "no-referrer",
				"Cross-Origin-Opener-Policy":   "same-origin",
				"Cross-Origin-Resource-Policy": "same-origin",
				"X-Frame-Options":              "DENY",
			}
			for k, v := range want {
				assert.Equal(t, v, rec.Header().Get(k), k)
			}
			for _, d := range []string{"frame-ancestors 'none'", "base-uri 'none'", "object-src 'none'", "default-src 'self'"} {
				assert.Contains(t, rec.Header().Get("Content-Security-Policy"), d)
			}
			assert.Equal(t, tt.wantHSTS, rec.Header().Get("Strict-Transport-Security") != "")
			assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "require-trusted-types-for 'script'")
			assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "trusted-types 'none'")
			assert.Empty(t, rec.Header().Get("Content-Security-Policy-Report-Only"))
			assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"), "CORS header must never be set")
		})
	}
}

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	tests := []struct {
		name    string
		trusted []netip.Prefix
		remote  string
		xff     []string
		want    string
	}{
		{"direct client ignores spoofed header", trusted, "203.0.113.9:4000", []string{"1.2.3.4"}, "203.0.113.9"},
		{"trusted proxy forwards client", trusted, "10.0.0.2:4000", []string{"198.51.100.7"}, "198.51.100.7"},
		{"spoof left of real client is ignored", trusted, "10.0.0.2:4000", []string{"1.2.3.4, 198.51.100.7"}, "198.51.100.7"},
		{"chain of trusted proxies", trusted, "10.0.0.2:4000", []string{"198.51.100.7, 10.1.1.1"}, "198.51.100.7"},
		{"multiple header lines", trusted, "10.0.0.2:4000", []string{"1.2.3.4", "198.51.100.7"}, "198.51.100.7"},
		{"garbage hop stops the chain", trusted, "10.0.0.2:4000", []string{"198.51.100.7, not-an-ip"}, "10.0.0.2"},
		{"trusted proxy without header", trusted, "10.0.0.2:4000", nil, "10.0.0.2"},
		{"ipv6 peer", trusted, "[2001:db8::1]:443", []string{"1.2.3.4"}, "2001:db8::1"},
		{"no trusted proxies", nil, "10.0.0.2:1", []string{"1.2.3.4"}, "10.0.0.2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got netip.Addr
			h := middleware.ClientIP(tt.trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = middleware.ClientIPFrom(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			req.RemoteAddr = tt.remote
			for _, v := range tt.xff {
				req.Header.Add("X-Forwarded-For", v)
			}
			serve(h, req)
			assert.Equal(t, tt.want, got.String())
		})
	}
}

func TestBodyLimit(t *testing.T) {
	var readErr error
	h := middleware.BodyLimit(8)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))

	t.Run("declared too large", func(t *testing.T) {
		rec := serve(h, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("0123456789")))
		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
		assertErrorBody(t, rec, "PAYLOAD_TOO_LARGE")
	})
	t.Run("undeclared too large", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", io.NopCloser(strings.NewReader("0123456789")))
		req.ContentLength = -1
		serve(h, req)
		var mbe *http.MaxBytesError
		assert.ErrorAs(t, readErr, &mbe)
	})
	t.Run("within limit", func(t *testing.T) {
		readErr = nil
		rec := serve(h, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("small")))
		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.NoError(t, readErr)
	})
}

func TestRecover(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	h := middleware.RequestID(middleware.Recover(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("db password is hunter2")
	})))
	rec := serve(h, httptest.NewRequest(http.MethodGet, "/x", http.NoBody))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assertErrorBody(t, rec, "INTERNAL")
	assert.NotContains(t, rec.Body.String(), "hunter2")
	assert.Contains(t, logs.String(), rec.Header().Get("X-Request-ID"))
}

func TestRecoverAbortHandler(t *testing.T) {
	var logs bytes.Buffer
	h := middleware.Recover(slog.New(slog.NewJSONHandler(&logs, nil)))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	rec := serve(h, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	assert.Zero(t, rec.Body.Len())
	assert.NotContains(t, logs.String(), "stack")
}

func TestRequestIDIgnoresClientValue(t *testing.T) {
	var seen string
	h := middleware.RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = middleware.RequestIDFrom(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("X-Request-ID", "attacker-chosen\nvalue")
	rec := serve(h, req)
	assert.NotContains(t, seen, "attacker")
	assert.Equal(t, seen, rec.Header().Get("X-Request-ID"))
	assert.Len(t, seen, 26)
}

func TestTimeoutSetsDeadline(t *testing.T) {
	var ok bool
	h := middleware.Timeout(time.Second)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, ok = r.Context().Deadline()
	}))
	serve(h, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	assert.True(t, ok, "no deadline on request context")
}

func TestNoStore(t *testing.T) {
	h := middleware.NoStore(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := serve(h, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}

func TestAccessLogOmitsQuery(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	h := middleware.AccessLog(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	serve(h, httptest.NewRequest(http.MethodGet, "/api/v1/meta?token=abc123", http.NoBody))
	assert.NotContains(t, logs.String(), "abc123")
	assert.Contains(t, logs.String(), `"status":418`)
}

func TestLang(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   i18n.Locale
	}{
		{"default is arabic", "", i18n.Ar},
		{"english", "en", i18n.En},
		{"english region", "en-US,en;q=0.9", i18n.En},
		{"quality wins", "ar;q=0.4, en;q=0.8", i18n.En},
		{"arabic first", "ar-EG,ar;q=0.9,en;q=0.5", i18n.Ar},
		{"unsupported falls back to arabic", "fr-FR,de;q=0.8", i18n.Ar},
		{"wildcard falls back to arabic", "*", i18n.Ar},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got i18n.Locale
			h := middleware.Lang(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = i18n.LocaleFrom(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			if tt.header != "" {
				req.Header.Set("Accept-Language", tt.header)
			}
			rec := serve(h, req)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, string(tt.want), rec.Header().Get("Content-Language"))
		})
	}
}

func TestStrictErrors(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	t.Run("request error is a bad request", func(t *testing.T) {
		rec := httptest.NewRecorder()
		middleware.StrictRequestError(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody), io.ErrUnexpectedEOF)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assertErrorBody(t, rec, "BAD_REQUEST")
		assert.NotContains(t, rec.Body.String(), "EOF")
	})
	t.Run("an oversized body is payload too large", func(t *testing.T) {
		rec := httptest.NewRecorder()
		middleware.StrictRequestError(rec, httptest.NewRequest(http.MethodPost, "/", http.NoBody), fmt.Errorf("decode: %w", &http.MaxBytesError{Limit: 10}))
		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
		assertErrorBody(t, rec, "PAYLOAD_TOO_LARGE")
	})
	t.Run("unknown error is internal and logged", func(t *testing.T) {
		rec := httptest.NewRecorder()
		middleware.StrictResponseError(log)(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody), io.ErrClosedPipe)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assertErrorBody(t, rec, "INTERNAL")
		assert.NotContains(t, rec.Body.String(), "pipe")
		assert.Contains(t, logs.String(), "closed pipe")
	})
}
