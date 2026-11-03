package support

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing/fstest"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
)

type FakeDB struct{ Err error }

func (f FakeDB) Ping(context.Context) error { return f.Err }

type FakeMigrations struct {
	Pending bool
	Schema  migrations.SchemaState
	Err     error
}

func (f FakeMigrations) State(context.Context) (migrations.SchemaState, error) {
	switch {
	case f.Schema != "":
		return f.Schema, f.Err
	case f.Pending:
		return migrations.SchemaPending, f.Err
	}
	return migrations.SchemaCurrent, f.Err
}

type FakeQueries struct {
	Instance sqlc.Instance
	Err      error
}

func (f FakeQueries) GetInstance(context.Context) (sqlc.Instance, error) { return f.Instance, f.Err }

func DiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func Router(d system.Deps) http.Handler {
	return RouterWithWeb(d, app.NewWebHandler(fstest.MapFS{}))
}

func RouterWithWeb(d system.Deps, web http.Handler) http.Handler {
	if d.Log == nil {
		d.Log = DiscardLogger()
	}
	if d.DB == nil {
		d.DB = FakeDB{}
	}
	if d.Migrations == nil {
		d.Migrations = FakeMigrations{}
	}
	if d.Queries == nil {
		d.Queries = FakeQueries{}
	}
	return app.NewRouter(app.Options{
		Config: config.Config{
			PublicURL:  &url.URL{Scheme: "https", Host: "dhow.example.com"},
			ListenAddr: config.DefaultListenAddr,
		},
		Log:    d.Log,
		System: d,
		Web:    web,
	})
}

func Do(h http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, path, http.NoBody)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
