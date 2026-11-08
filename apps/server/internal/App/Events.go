package app

import (
	"context"
	"sync"

	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	events "github.com/Khaymah/dhow/apps/server/internal/Modules/Events"
)

func eventsFor(in Infra, bus *eventbus.Bus) *events.Service {
	service := events.NewService(events.Options{
		DB: in.Pool, Listen: events.Listener(listener(in.Pool)), Limits: in.Config.Logs, WriteWait: in.LogStreamWriteWait, Log: in.Log, Now: in.Now,
	})
	events.Subscribe(bus, service)
	return service
}

func StartEvents(ctx context.Context, s *events.Service) func() {
	bgCtx, cancel := context.WithCancel(ctx)
	var background sync.WaitGroup
	background.Go(func() { s.Run(bgCtx) })
	return func() {
		cancel()
		background.Wait()
	}
}
