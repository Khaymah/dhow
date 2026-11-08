package events

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Events/Models"
)

const closeWait = 2 * time.Second

type Listener func(ctx context.Context) (*pgx.Conn, error)

func (s *Service) Run(ctx context.Context) {
	if s.listen == nil {
		return
	}
	s.hub.SetHealthy(false)
	backoff := config.OrgEventsListenMin
	for ctx.Err() == nil {
		err := s.serve(ctx)
		s.hub.SetHealthy(false)
		if ctx.Err() != nil {
			return
		}
		s.log.WarnContext(ctx, "the live event listener stopped, retrying", "error", err, "retry_in", backoff.String())
		timer := time.NewTimer(backoff/2 + rand.N(backoff/2+1)) //nolint:gosec // Retry jitter needs no cryptographic randomness.
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		backoff = min(backoff*2, config.OrgEventsListenMax)
	}
}

func (s *Service) serve(ctx context.Context) error {
	conn, err := s.listen(ctx)
	if err != nil {
		return err
	}
	defer func() {
		closing, stop := context.WithTimeout(context.WithoutCancel(ctx), closeWait)
		defer stop()
		if cerr := conn.Close(closing); cerr != nil {
			s.log.WarnContext(ctx, "close the live event listener", "error", cerr)
		}
	}()
	if err := sqlc.New(conn).ListenOrgEvents(ctx); err != nil {
		return err
	}
	s.hub.Restart()
	s.hub.SetHealthy(true)
	stopCanary := s.startCanary(ctx)
	defer stopCanary()
	for {
		wait, cancel := context.WithTimeout(ctx, s.silence)
		n, err := conn.WaitForNotification(wait)
		cancel()
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return errListenerSilent
		}
		if err != nil {
			return err
		}
		s.receive(ctx, n.Payload)
	}
}

var errListenerSilent = errors.New("the live event listener heard nothing, not even its own canary")

func (s *Service) receive(ctx context.Context, payload string) {
	var notice models.Notice
	if err := json.Unmarshal([]byte(payload), &notice); err != nil {
		s.log.WarnContext(ctx, "a live event could not be read", "error", err)
		return
	}
	if notice.Type == models.TypeCanary {
		return
	}
	if notice.Organization == uuid.Nil || !s.known[notice.Type] {
		s.log.WarnContext(ctx, "a live event of an unknown kind was dropped")
		return
	}
	s.hub.Append(notice)
}

func (s *Service) startCanary(ctx context.Context) func() {
	if s.db == nil {
		return func() {}
	}
	inner, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(s.canary)
		defer ticker.Stop()
		raw, err := json.Marshal(models.Notice{Type: models.TypeCanary, At: s.now().UTC()})
		if err != nil {
			return
		}
		for {
			select {
			case <-inner.Done():
				return
			case <-ticker.C:
				if err := sqlc.New(s.db).NotifyOrgEvent(inner, string(raw)); err != nil && inner.Err() == nil {
					s.log.WarnContext(inner, "the live event canary was not sent", "error", err)
				}
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}
