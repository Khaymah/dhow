package app_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

func TestTrustedTypesAreOnlyReportedInDevelopment(t *testing.T) {
	tests := []struct {
		name       string
		dev        bool
		wantReport string
	}{
		{"development reports", true, "require-trusted-types-for 'script'"},
		{"production does not", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			web := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
			router := app.NewRouter(app.Options{
				Config: config.Config{Dev: tt.dev, PublicURL: &url.URL{Scheme: "http", Host: "localhost"}},
				Log:    slog.New(slog.DiscardHandler),
				Web:    web,
			})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
			assert.Equal(t, tt.wantReport, rec.Header().Get("Content-Security-Policy-Report-Only"))
			assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "script-src 'self'")
			assert.NotContains(t, rec.Header().Get("Content-Security-Policy"), "trusted-types")
		})
	}
}
