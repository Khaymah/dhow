package e2e_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Gen"
	support "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Test/Support"
	"github.com/Khaymah/dhow/internal/buildinfo"
)

func TestLiveness(t *testing.T) {
	h := support.Router(system.Deps{DB: support.FakeDB{Err: errors.New("down")}})
	rec := support.Do(h, http.MethodGet, "/api/v1/health/live", nil)
	require.Equal(t, http.StatusOK, rec.Code, "liveness must not depend on the database")
	var body gen.HealthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	require.NotNil(t, body.Message)
	assert.Equal(t, "system.health.live", *body.Message)
}

func TestReadiness(t *testing.T) {
	tests := []struct {
		name       string
		deps       system.Deps
		wantStatus int
		wantChecks map[gen.HealthCheckName]bool
	}{
		{"ready", system.Deps{}, http.StatusOK, map[gen.HealthCheckName]bool{"database": true, "migrations": true}},
		{
			"database down",
			system.Deps{DB: support.FakeDB{Err: errors.New("dial tcp 10.0.0.5:5432: password=hunter2")}},
			http.StatusServiceUnavailable,
			map[gen.HealthCheckName]bool{"database": false, "migrations": false},
		},
		{
			"pending migrations",
			system.Deps{Migrations: support.FakeMigrations{Pending: true}},
			http.StatusServiceUnavailable,
			map[gen.HealthCheckName]bool{"database": true, "migrations": false},
		},
		{
			"migration status error",
			system.Deps{Migrations: support.FakeMigrations{Err: errors.New("boom")}},
			http.StatusServiceUnavailable,
			map[gen.HealthCheckName]bool{"database": true, "migrations": false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := support.Do(support.Router(tt.deps), http.MethodGet, "/api/v1/health/ready", nil)
			require.Equal(t, tt.wantStatus, rec.Code)
			assert.NotContains(t, rec.Body.String(), "hunter2")
			assert.NotContains(t, rec.Body.String(), "10.0.0.5")
			var body gen.HealthResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.NotNil(t, body.Data)
			assert.Equal(t, tt.wantStatus == http.StatusOK, body.Success)
			if tt.wantStatus == http.StatusOK {
				assert.Nil(t, body.Error)
			} else {
				require.NotNil(t, body.Error)
				assert.Equal(t, "SERVICE_UNAVAILABLE", body.Error.Code)
			}
			got := map[gen.HealthCheckName]bool{}
			for _, c := range body.Data.Checks {
				got[c.Name] = c.Ok
			}
			assert.Equal(t, tt.wantChecks, got)
		})
	}
}

func TestMeta(t *testing.T) {
	build := buildinfo.Info{Version: "v0.1.0", Commit: "abc123"}
	completedAt := time.Now()
	done := sqlc.Instance{SetupCompletedAt: &completedAt}
	tests := []struct {
		name          string
		queries       support.FakeQueries
		wantSetupNeed bool
	}{
		{"setup required", support.FakeQueries{}, true},
		{"setup done", support.FakeQueries{Instance: done}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := support.Do(support.Router(system.Deps{Build: build, Queries: tt.queries}), http.MethodGet, "/api/v1/meta", nil)
			require.Equal(t, http.StatusOK, rec.Code)
			var body gen.MetaResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.True(t, body.Success)
			assert.Nil(t, body.Error)
			require.NotNil(t, body.Data)
			assert.Equal(t, gen.Meta{Version: "v0.1.0", Commit: "abc123", ServerBuild: &gen.ServerBuild{Version: "v0.1.0", Commit: "abc123"}, SetupRequired: tt.wantSetupNeed}, *body.Data)
			require.NotNil(t, body.Message)
			assert.Equal(t, "system.meta.loaded", *body.Message)
		})
	}
}

func TestMetaErrorFollowsAcceptLanguage(t *testing.T) {
	deps := system.Deps{Queries: support.FakeQueries{Err: errors.New("boom")}}
	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"arabic by default", nil, "حدث خطأ غير متوقع. حاول مرة أخرى بعد قليل."},
		{"english", map[string]string{"Accept-Language": "en-US,en;q=0.9"}, "Something went wrong. Try again later."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := support.Do(support.Router(deps), http.MethodGet, "/api/v1/meta", tt.headers)
			require.Equal(t, http.StatusInternalServerError, rec.Code)
			var body gen.ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "INTERNAL", body.Error.Code)
			assert.Equal(t, tt.want, body.Error.Message)
		})
	}
}

func TestRoutingAndHeaders(t *testing.T) {
	h := support.Router(system.Deps{})
	tests := []struct {
		name, method, path string
		wantStatus         int
		wantCode           string
	}{
		{"unknown api route", http.MethodGet, "/api/v1/nope", http.StatusNotFound, "NOT_FOUND"},
		{"unknown api version", http.MethodGet, "/api/v9/meta", http.StatusNotFound, "NOT_FOUND"},
		{"web root until the UI is embedded", http.MethodGet, "/", http.StatusNotFound, "NOT_FOUND"},
		{"wrong method", http.MethodPost, "/api/v1/meta", http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := support.Do(h, tt.method, tt.path, nil)
			require.Equal(t, tt.wantStatus, rec.Code)
			var body gen.ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.False(t, body.Success)
			assert.Equal(t, tt.wantCode, body.Error.Code)
			assert.NotEmpty(t, rec.Header().Get("Content-Security-Policy"))
			assert.NotEmpty(t, rec.Header().Get("Strict-Transport-Security"))
			assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
		})
	}
	rec := support.Do(h, http.MethodGet, "/api/v1/meta", nil)
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("DHOW_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("DHOW_TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("DHOW_TEST_DATABASE_URL not set")
	}
	return dsn
}

const migrationLockWait = 3 * time.Minute

func TestFullStackWithPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), migrationLockWait)
	defer cancel()
	pool, err := config.OpenDatabase(ctx, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	migrator, err := migrations.NewMigrator(pool, migrations.Binary{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, migrator.Close()) })

	require.NoError(t, migrator.DownTo(ctx, 0))
	h := support.Router(system.Deps{DB: pool, Migrations: migrator, Queries: sqlc.New(pool)})

	rec := support.Do(h, http.MethodGet, "/api/v1/health/ready", nil)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "ready before migrations")

	_, err = migrator.Up(ctx)
	require.NoError(t, err)
	rec = support.Do(h, http.MethodGet, "/api/v1/health/ready", nil)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = support.Do(h, http.MethodGet, "/api/v1/meta", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var meta gen.MetaResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &meta))
	require.NotNil(t, meta.Data)
	assert.True(t, meta.Success)
	assert.True(t, meta.Data.SetupRequired, "a fresh instance needs setup")
}
