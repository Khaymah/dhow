package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	outbox "github.com/Khaymah/dhow/apps/server/internal/Core/Outbox"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Models"
)

func rolesOf(raw []string) []policy.Role {
	out := make([]policy.Role, 0, len(raw))
	for _, r := range raw {
		if role, ok := policy.ParseRole(r); ok {
			out = append(out, role)
		}
	}
	return out
}

func (s *Service) creatorAllowed(ctx context.Context, ch sqlc.NotificationChannel) (bool, error) {
	rows, err := s.queries().ListNotificationCreatorAccess(ctx, sqlc.ListNotificationCreatorAccessParams{
		OrganizationID: ch.OrganizationId, ProjectID: ch.ProjectId, UserIds: []uuid.UUID{ch.CreatedByUserId},
	})
	if err != nil {
		return false, err
	}
	for _, r := range rows {
		role, ok := policy.EffectiveOf(r.Role, r.TeamRoles)
		if !ok {
			continue
		}
		if ch.ProjectId == nil {
			return policy.Allows(role, policy.OrgNotificationManage), nil
		}
		return policy.Allows(policy.ProjectRole(role, rolesOf(r.Bindings)), policy.NotificationManage), nil
	}
	return false, nil
}

func (s *Service) liveChannel(ctx context.Context, item outbox.Item) (sqlc.NotificationChannel, outbox.Result, bool) {
	if item.ChannelID == nil {
		return sqlc.NotificationChannel{}, outbox.Result{Code: CodeChannelGone, Permanent: true}, false
	}
	ch, err := s.queries().GetNotificationChannelByID(ctx, *item.ChannelID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ch, outbox.Result{Code: CodeChannelGone, Permanent: true}, false
	case err != nil:
		return ch, outbox.Result{Code: CodeConfig}, false
	case !ch.Enabled:
		return ch, outbox.Result{Code: CodeChannelOff, Permanent: true}, false
	}
	allowed, err := s.creatorAllowed(ctx, ch)
	if err != nil {
		return ch, outbox.Result{Code: CodeConfig}, false
	}
	if !allowed {
		if err := s.disableChannel(ctx, ch, models.ReasonCreatorRemoved, 0); err != nil {
			s.log.ErrorContext(ctx, "notification channel could not be stopped", "channel_id", ch.Id.String(), "error", err)
		}
		return ch, outbox.Result{Code: CodeChannelOff, Permanent: true}, false
	}
	return ch, outbox.Result{}, true
}

func (s *Service) disableChannel(ctx context.Context, ch sqlc.NotificationChannel, reason string, failures int) error {
	return dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		row, err := q.DisableNotificationChannel(ctx, sqlc.DisableNotificationChannelParams{ID: ch.Id, Reason: reason})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		scope := models.Scope{OrganizationID: row.OrganizationId, ProjectID: row.ProjectId}
		data := s.channelAudit(row, scope)
		data["Reason"] = reason
		if err := s.audit(ctx, auditEntry{
			name: eventbus.NotificationChannelDisabled, org: row.OrganizationId, target: targetChannel, targetID: row.Id, tx: q, data: data,
		}); err != nil {
			return err
		}
		if reason == models.ReasonFailures {
			return s.mailCreatorDisabled(ctx, q, row, failures)
		}
		return nil
	})
}

func (s *Service) Settled(ctx context.Context, item outbox.Item, res outbox.Result, _ bool) {
	if item.ChannelID == nil || item.EventName == EventTest && res.OK() {
		return
	}
	q := s.queries()
	if res.OK() {
		if err := q.RecordNotificationSuccess(ctx, *item.ChannelID); err != nil {
			s.log.WarnContext(ctx, "notification channel health could not be saved", "channel_id", item.ChannelID.String(), "error", err)
		}
		return
	}
	if res.Code == CodeChannelOff || res.Code == CodeChannelGone || res.Code == CodeMailOff {
		return
	}
	row, err := q.RecordNotificationFailure(ctx, *item.ChannelID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.log.WarnContext(ctx, "notification channel health could not be saved", "channel_id", item.ChannelID.String(), "error", err)
		}
		return
	}
	if row.ConsecutiveFailures < config.NotifyBreakerFailures || !row.Enabled {
		return
	}
	ch, err := q.GetNotificationChannelByID(ctx, *item.ChannelID)
	if err != nil {
		return
	}
	if err := s.disableChannel(ctx, ch, models.ReasonFailures, int(row.ConsecutiveFailures)); err != nil {
		s.log.ErrorContext(ctx, "notification channel could not be stopped", "channel_id", ch.Id.String(), "error", err)
	}
}

func (s *Service) Deliverers() map[outbox.Kind]outbox.Deliverer {
	return map[outbox.Kind]outbox.Deliverer{
		outbox.KindEmail:         emailDeliverer{s: s},
		outbox.KindWebhook:       webhookDeliverer{s: s},
		outbox.KindInternalEmail: internalMailDeliverer{s: s},
	}
}

func (s *Service) Worker() (*outbox.Worker, error) {
	if s.worker != nil {
		return s.worker, nil
	}
	if s.pool == nil {
		return nil, ErrNoPool
	}
	w, err := outbox.NewWorker(outbox.Options{
		Pool: s.pool, Log: s.log, Now: s.now, Deliverers: s.Deliverers(), Observer: s,
		Global: config.NotifyGlobalConcurrency, PerChannel: config.NotifyChannelConcurrency,
	})
	if err != nil {
		return nil, fmt.Errorf("build the notification worker: %w", err)
	}
	s.worker = w
	return w, nil
}

func (s *Service) Drain(ctx context.Context) error {
	w, err := s.Worker()
	if err != nil {
		return err
	}
	if _, err := w.Drain(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Service) Run(ctx context.Context) {
	w, err := s.Worker()
	if err != nil {
		s.log.ErrorContext(ctx, "notification worker cannot start", "error", err)
		return
	}
	go s.scanLoop(ctx)
	w.Run(ctx)
}

func (s *Service) scanLoop(ctx context.Context) {
	ticker := time.NewTicker(config.NotifyOfflineScanEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.ScanRunners(ctx); err != nil {
				s.log.WarnContext(ctx, "runner outage scan failed", "error", err)
			}
		}
	}
}
