package events

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Events/Models"
)

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
		if cerr := conn.Close(context.WithoutCancel(ctx)); cerr != nil {
			s.log.WarnContext(ctx, "close the live event listener", "error", cerr)
		}
	}()
	if err := sqlc.New(conn).ListenOrgEvents(ctx); err != nil {
		return err
	}
	s.hub.Restart()
	s.hub.SetHealthy(true)
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var notice models.Notice
		if err := json.Unmarshal([]byte(n.Payload), &notice); err != nil || notice.Organization.String() == "" {
			s.log.WarnContext(ctx, "a live event could not be read", "error", err)
			continue
		}
		s.hub.Append(notice)
	}
}
