package app

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	serverinstances "github.com/Khaymah/dhow/apps/server/internal/Core/ServerInstances"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	"github.com/Khaymah/dhow/internal/buildinfo"
)

const unknownHost = "unknown"

func PrepareSchema(ctx context.Context, m *migrations.Migrator, log *slog.Logger) error {
	if _, err := m.CheckCompatible(ctx); err != nil {
		return err
	}
	st, err := m.Apply(ctx)
	if err != nil {
		return err
	}
	if st.State == migrations.SchemaAhead {
		log.Warn("the database schema is newer than this binary; running because its floor allows it",
			"database_version", st.DBVersion, "binary_latest", st.Latest, "floor", st.Floor, "binary", st.Binary)
		return nil
	}
	log.Info("database schema is current", "version", st.DBVersion)
	return nil
}

func HoldIncompatible(ctx context.Context, err error, mode string, log *slog.Logger) error {
	var incompatible *migrations.IncompatibleError
	if mode != config.OnIncompatibleWait || !errors.As(err, &incompatible) {
		return err
	}
	log.Error("the database schema is too new for this binary; waiting for a stop signal instead of exiting, "+
		"so the container does not restart in a loop", "code", migrations.CodeSchemaTooNew, "error", err.Error(),
		config.EnvOnIncompatible, mode)
	<-ctx.Done()
	return nil
}

func ServerIdentity(build buildinfo.Info, schema int64, key string, hostname func() (string, error), log *slog.Logger) serverinstances.Identity {
	host, err := hostname()
	if err != nil || host == "" {
		log.Warn("could not read the host name; set "+config.EnvInstanceID+" so this server keeps one row in ServerInstances",
			"error", err)
		host = unknownHost
	}
	if key == "" {
		key = host
	}
	return serverinstances.Identity{Key: key, Hostname: host, Version: build.Version, Commit: build.Commit, SchemaVersion: schema}
}

func StartHeartbeat(ctx context.Context, pool *pgxpool.Pool, ident serverinstances.Identity, log *slog.Logger) (func(), error) {
	hb := serverinstances.New(sqlc.New(pool), ident, config.ServerInstanceStaleAfter, log)
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
