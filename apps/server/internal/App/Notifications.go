package app

import (
	"context"
	"sync"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	mailer "github.com/Khaymah/dhow/apps/server/internal/Core/Mail"
	notifications "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications"
)

func notificationsFor(in Infra, values *keys.Values, bus *eventbus.Bus, factors auth.FactorConfirmer, mail mailer.Sender) (*notifications.Service, error) {
	service, err := notifications.NewService(notifications.Options{
		DB: in.Pool, Pool: in.Pool, Values: values, Factors: factors, Bus: bus, Mailer: mail, Config: in.Config.Notify,
		PublicURL: in.Config.PublicURL.String(), TLS: in.NotifyTLS, Lookup: in.NotifyLookup, Dial: in.NotifyDial,
		Log: in.Log, Now: in.Now,
	})
	if err != nil {
		return nil, err
	}
	notifications.Subscribe(bus, service)
	return service, nil
}

func StartNotifications(ctx context.Context, s *notifications.Service) func() {
	bgCtx, cancel := context.WithCancel(ctx)
	var background sync.WaitGroup
	background.Go(func() { s.Run(bgCtx) })
	return func() {
		cancel()
		background.Wait()
	}
}
