package app

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
)

func OpenKeyring(cfg config.Keyring) (*keys.FileProvider, error) {
	ring, err := keys.ReadKeyringFile(cfg.Path, keys.FileRules{AllowGroupRead: cfg.Credential})
	if err != nil {
		return nil, err
	}
	return keys.NewFileProvider(ring)
}

func StartKeys(ctx context.Context, pool *pgxpool.Pool, provider keys.KeyProvider, log *slog.Logger) (*keys.Manager, []int32, func(), error) {
	manager := keys.NewManager(pool, provider, log)
	reg, err := manager.Register(ctx)
	if err != nil {
		log.Error("the keyring does not fit this database; stopping", "code", keys.CodeOf(err), "error", err.Error())
		return nil, nil, nil, err
	}
	log.Info("keyring loaded", "provider", provider.Name(), "kek_versions", reg.Versions, "active_kek_version", reg.Active,
		"registered", reg.Registered)
	if reg.Activated {
		log.Info("activated the first KEK version of this database", "kek_version", reg.Active)
	}
	bgCtx, cancel := context.WithCancel(ctx)
	var background sync.WaitGroup
	background.Go(func() { manager.RewrapLoop(bgCtx, config.KeyRewrapEvery) })
	return manager, reg.Versions, func() {
		cancel()
		background.Wait()
	}, nil
}
