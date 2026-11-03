package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	artifacts "github.com/Khaymah/dhow/apps/server/internal/Modules/Artifacts"
)

func artifactsFor(in Infra) (*artifacts.Service, error) {
	cfg := in.Config.Artifacts
	if cfg.BlobDir == "" {
		cfg = config.DefaultArtifacts()
	}
	store, err := blob.NewDisk(blob.DiskOptions{
		Dir: cfg.BlobDir, Create: cfg.BlobDirIsDev, Reserve: config.BlobFreeSpaceReserve, CheckEvery: config.BlobSpaceCheckEvery,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", config.EnvBlobDir, err)
	}
	return artifacts.NewService(artifacts.Options{
		DB: in.Pool, Store: store, Config: cfg, SecretKey: in.Config.SecretKey, Log: in.Log, Now: in.Now,
	})
}

func runnerBodyLimit(next http.Handler) http.Handler {
	limited := middleware.BodyLimit(config.RunnerBodyMaxBytes)(next)
	blobs := config.RunnerAPIPrefix + config.BlobsPath
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, blobs) {
			next.ServeHTTP(w, r)
			return
		}
		limited.ServeHTTP(w, r)
	})
}

func StartArtifacts(ctx context.Context, a *artifacts.Service, log *slog.Logger) func() {
	if a == nil {
		return func() {}
	}
	bgCtx, cancel := context.WithCancel(ctx)
	var background sync.WaitGroup
	background.Go(func() { a.RunRetention(bgCtx) })
	log.Info("artifact retention started", "every", config.BlobSweepEvery.String())
	return func() {
		cancel()
		background.Wait()
	}
}
