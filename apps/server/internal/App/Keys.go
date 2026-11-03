package app

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	serverinstances "github.com/Khaymah/dhow/apps/server/internal/Core/ServerInstances"
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

func StartInstance(
	ctx context.Context, pool *pgxpool.Pool, provider keys.KeyProvider, ident serverinstances.Identity, log *slog.Logger,
) (*keys.Manager, func(), error) {
	manager, versions, stopKeys, err := StartKeys(ctx, pool, provider, log)
	if err != nil {
		return nil, nil, err
	}
	ident.KekVersions = versions
	stopHeartbeat, err := StartHeartbeat(ctx, pool, ident, log)
	if err != nil {
		stopKeys()
		return nil, nil, err
	}
	return manager, func() {
		stopHeartbeat()
		stopKeys()
	}, nil
}
