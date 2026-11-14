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

func TestTrustedTypesAreEnforcedInEveryMode(t *testing.T) {
	paths := []string{"/", "/dashboard", "/assets/index-abc123.js", "/api/v1/meta"}
	for _, dev := range []bool{true, false} {
		for _, scheme := range []string{"http", "https"} {
			for _, p := range paths {
				name := scheme + " " + p
				if dev {
					name = "dev " + name
				}
				t.Run(name, func(t *testing.T) {
					web := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
					router := app.NewRouter(app.Options{
						Config: config.Config{Dev: dev, PublicURL: &url.URL{Scheme: scheme, Host: "localhost"}},
						Log:    slog.New(slog.DiscardHandler),
						Web:    web,
					})
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, http.NoBody))
					csp := rec.Header().Get("Content-Security-Policy")
					assert.Contains(t, csp, "script-src 'self'")
					assert.Contains(t, csp, "require-trusted-types-for 'script'")
					assert.Contains(t, csp, "trusted-types 'none'")
					assert.NotContains(t, csp, "unsafe-inline")
					assert.Empty(t, rec.Header().Get("Content-Security-Policy-Report-Only"))
				})
			}
		}
	}
}
