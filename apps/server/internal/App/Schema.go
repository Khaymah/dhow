package app

import (
	"context"
	"log/slog"
	"os"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	serverinstances "github.com/Khaymah/dhow/apps/server/internal/Core/ServerInstances"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	"github.com/Khaymah/dhow/internal/buildinfo"
)

func PrepareSchema(ctx context.Context, m *migrations.Migrator, log *slog.Logger) error {
	st, err := m.CheckCompatible(ctx)
	if err != nil {
		return err
	}
	version, err := m.Up(ctx)
	if err != nil {
		return err
	}
	if st.State == migrations.SchemaAhead {
		log.Warn("the database schema is newer than this binary; running because its floor allows it",
			"database_version", st.DBVersion, "binary_latest", st.Latest, "floor", st.Floor, "binary", st.Binary)
		return nil
	}
	log.Info("database schema is current", "version", version)
	return nil
}

func StartHeartbeat(ctx context.Context, pool *pgxpool.Pool, build buildinfo.Info, schema int64, log *slog.Logger) (func(), error) {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	hb := serverinstances.New(sqlc.New(pool), serverinstances.Identity{
		Hostname: host, Version: build.Version, Commit: build.Commit, SchemaVersion: schema,
	}, config.ServerInstanceStaleAfter, log)
	if err := hb.Register(ctx); err != nil {
		return nil, err
	}
	bgCtx, cancel := context.WithCancel(ctx)
	var background sync.WaitGroup
	background.Go(func() { hb.Loop(bgCtx, config.ServerHeartbeatEvery) })
	return func() {
		cancel()
		background.Wait()
		retireCtx, done := context.WithTimeout(context.WithoutCancel(ctx), config.ServerRetireTimeout)
		defer done()
		if err := hb.Retire(retireCtx); err != nil {
			log.Warn("retire this server instance", "error", err)
		}
	}, nil
}

func schemaBinary(cfg config.Config, build buildinfo.Info) migrations.Binary {
	return migrations.Binary{Version: build.Version, Dev: cfg.Dev}
}
