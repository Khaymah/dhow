package app

import (
	"context"
	"log/slog"
	"sync"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	backfill "github.com/Khaymah/dhow/apps/server/internal/Core/Backfill"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	instancesettings "github.com/Khaymah/dhow/apps/server/internal/Core/Settings"
	session "github.com/Khaymah/dhow/apps/server/internal/Modules/Session"
	sources "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources"
)

func ValueRegistry() (*keys.Registry, error) {
	var all []keys.Location
	all = append(all, session.ValueLocations()...)
	all = append(all, sources.ValueLocations()...)
	all = append(all, instancesettings.ValueLocations()...)
	return keys.NewRegistry(all...)
}

func NewBackfills(db backfill.DB, values *keys.Values, log *slog.Logger) (*backfill.Runner, error) {
	reg, err := ValueRegistry()
	if err != nil {
		return nil, err
	}
	jobs, err := reg.BackfillJobs(values, log)
	if err != nil {
		return nil, err
	}
	return backfill.New(backfill.Options{
		DB: db, Jobs: jobs, BatchSize: config.BackfillBatch, Pause: config.BackfillPause, Every: config.BackfillEvery, Log: log,
	})
}

func StartBackfills(ctx context.Context, db backfill.DB, values *keys.Values, log *slog.Logger) (func(), error) {
	runner, err := NewBackfills(db, values, log)
	if err != nil {
		return nil, err
	}
	bgCtx, cancel := context.WithCancel(ctx)
	var background sync.WaitGroup
	background.Go(func() { runner.Loop(bgCtx) })
	return func() {
		cancel()
		background.Wait()
	}, nil
}
