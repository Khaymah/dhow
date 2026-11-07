package edge_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
	support "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Test/Support"
)

const (
	leakyDBError    = `dial tcp 10.0.0.5:5432: password authentication failed for user "dhow" password=hunter2`
	leakyQueryError = `ERROR: relation "Instance" does not exist (SQLSTATE 42P01)`
)

var leakMarkers = []string{"hunter2", "10.0.0.5", "5432", "SQLSTATE", "relation", "dial tcp", "goroutine", "panic", "password"}

type panicDB struct{}

func (panicDB) Ping(context.Context) error { panic(leakyDBError) }

func keysOf(t *testing.T, body []byte) map[string]json.RawMessage {
	t.Helper()
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &raw), "body is not a JSON object: %q", body)
	return raw
}

func TestEnvelopeShape(t *testing.T) {
	tests := []struct {
		name        string
		deps        system.Deps
		method      string
		path        string
		wantStatus  int
		wantSuccess bool
		wantKeys    []string
		wantCode    string
	}{
		{"liveness", system.Deps{}, http.MethodGet, "/api/v1/health/live", http.StatusOK, true, []string{"Success", "Data", "Message"}, ""},
		{"readiness ok", system.Deps{}, http.MethodGet, "/api/v1/health/ready", http.StatusOK, true, []string{"Success", "Data", "Message"}, ""},
		{
			"readiness unavailable",
			system.Deps{DB: support.FakeDB{Err: errors.New(leakyDBError)}},
			http.MethodGet, "/api/v1/health/ready", http.StatusServiceUnavailable, false,
			[]string{"Success", "Data", "Error"},
			"SERVICE_UNAVAILABLE",
		},
		{"meta", system.Deps{}, http.MethodGet, "/api/v1/meta", http.StatusOK, true, []string{"Success", "Data", "Message"}, ""},
		{
			"meta internal error",
			system.Deps{Queries: support.FakeQueries{Err: errors.New(leakyQueryError)}},
			http.MethodGet, "/api/v1/meta", http.StatusInternalServerError, false,
			[]string{"Success", "Error"},
			"INTERNAL",
		},
		{
			"recovered panic",
			system.Deps{DB: panicDB{}},
			http.MethodGet, "/api/v1/health/ready", http.StatusInternalServerError, false,
			[]string{"Success", "Error"},
			"INTERNAL",
		},
		{"api not found", system.Deps{}, http.MethodGet, "/api/v1/nope", http.StatusNotFound, false, []string{"Success", "Error"}, "NOT_FOUND"},
		{"api version not found", system.Deps{}, http.MethodGet, "/api/v2/meta", http.StatusNotFound, false, []string{"Success", "Error"}, "NOT_FOUND"},
		{"web not found", system.Deps{}, http.MethodGet, "/dashboard", http.StatusNotFound, false, []string{"Success", "Error"}, "NOT_FOUND"},
		{"method not allowed", system.Deps{}, http.MethodDelete, "/api/v1/meta", http.StatusMethodNotAllowed, false, []string{"Success", "Error"}, "METHOD_NOT_ALLOWED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := support.Do(support.Router(tt.deps), tt.method, tt.path, nil)
			require.Equal(t, tt.wantStatus, rec.Code)
			assert.True(t, strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json"))

			raw := keysOf(t, rec.Body.Bytes())
			keys := make([]string, 0, len(raw))
			for k := range raw {
				keys = append(keys, k)
			}
			assert.ElementsMatch(t, tt.wantKeys, keys, "absent fields must be omitted, never null")
			for k, v := range raw {
				assert.NotEqual(t, "null", string(v), k)
			}
			var success bool
			require.NoError(t, json.Unmarshal(raw["Success"], &success))
			assert.Equal(t, tt.wantSuccess, success)

			if tt.wantCode == "" {
				return
			}
			var detail map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw["Error"], &detail))
			var code, message string
			require.NoError(t, json.Unmarshal(detail["Code"], &code))
			require.NoError(t, json.Unmarshal(detail["Message"], &message))
			assert.Equal(t, tt.wantCode, code)
			assert.NotEmpty(t, message)
			assert.NotContains(t, detail, "Details", "Details is omitted when empty")
			if code != "METHOD_NOT_ALLOWED" {
				assert.Contains(t, detail, "Fix", "a code with a fix returns it")
			}
		})
	}
}

func TestErrorBodiesLeakNothing(t *testing.T) {
	tests := []struct {
		name string
		deps system.Deps
		path string
	}{
		{"readiness database error", system.Deps{DB: support.FakeDB{Err: errors.New(leakyDBError)}}, "/api/v1/health/ready"},
		{"readiness migration error", system.Deps{Migrations: support.FakeMigrations{Err: errors.New(leakyDBError)}}, "/api/v1/health/ready"},
		{"meta query error", system.Deps{Queries: support.FakeQueries{Err: errors.New(leakyQueryError)}}, "/api/v1/meta"},
		{"recovered panic", system.Deps{DB: panicDB{}}, "/api/v1/health/ready"},
		{"not found", system.Deps{}, "/api/v1/nope"},
	}
	for _, tt := range tests {
		for _, lang := range []string{"ar", "en"} {
			t.Run(tt.name+" "+lang, func(t *testing.T) {
				rec := support.Do(support.Router(tt.deps), http.MethodGet, tt.path, map[string]string{"Accept-Language": lang})
				body := strings.ToLower(rec.Body.String())
				for _, m := range leakMarkers {
					assert.NotContains(t, body, strings.ToLower(m))
				}
			})
		}
	}
}

func TestBodyLimitApplies(t *testing.T) {
	h := support.Router(system.Deps{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/meta", strings.NewReader(strings.Repeat("a", config.MaxBodyBytes+1)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	raw := keysOf(t, rec.Body.Bytes())
	assert.JSONEq(t, `{"Code":"PAYLOAD_TOO_LARGE","Message":"حجم الطلب أكبر من المسموح.","Fix":"أرسل بيانات أقل في الطلب الواحد، مثل ملف أصغر أو عناصر أقل."}`, string(raw["Error"]))
}

func TestPanicIsRecoveredAndRequestIDIsReturned(t *testing.T) {
	rec := support.Do(support.Router(system.Deps{DB: panicDB{}}), http.MethodGet, "/api/v1/health/ready", nil)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
}

func TestMalformedRequestsNeverReachTheController(t *testing.T) {
	tests := []struct {
		name string
		path string
		want int
	}{
		{"trailing slash is a different route", "/api/v1/meta/", http.StatusNotFound},
		{"encoded traversal", "/api/v1/%2e%2e/meta", http.StatusNotFound},
		{"camelCase segment", "/api/v1/healthLive", http.StatusNotFound},
	}
	h := support.Router(system.Deps{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := support.Do(h, http.MethodGet, tt.path, nil)
			assert.Equal(t, tt.want, rec.Code)
			assert.Contains(t, rec.Body.String(), "NOT_FOUND")
		})
	}
}
