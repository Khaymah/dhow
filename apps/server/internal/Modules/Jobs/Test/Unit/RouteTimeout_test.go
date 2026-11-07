package unit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	jobs "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs"
)

func TestEveryJobRouteButTheLogStreamHasTheAPITimeout(t *testing.T) {
	for _, tt := range []struct {
		pattern  string
		deadline bool
	}{
		{"/organizations/{id}/jobs", true},
		{"/organizations/{id}/jobs/{jobId}", true},
		{"/organizations/{id}/jobs/{jobId}/logs", true},
		{jobs.LogStreamPath, false},
		{jobs.LogRawPath, false},
	} {
		t.Run(tt.pattern, func(t *testing.T) {
			var left time.Duration
			var has bool
			h := jobs.RouteTimeout(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				var d time.Time
				d, has = r.Context().Deadline()
				left = time.Until(d)
			}))
			rc := chi.NewRouteContext()
			rc.RoutePatterns = []string{config.APIPrefix + tt.pattern}
			req := httptest.NewRequestWithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rc), http.MethodGet, "/x", http.NoBody)
			h.ServeHTTP(httptest.NewRecorder(), req)
			assert.Equal(t, tt.deadline, has)
			if tt.deadline {
				assert.LessOrEqual(t, left, config.APITimeout)
			}
		})
	}
}
