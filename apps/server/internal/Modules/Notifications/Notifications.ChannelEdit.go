package notifications

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Models"
)

type configChange struct {
	cfg     channelConfig
	host    string
	secret  string
	changed bool
}

func validateUpdate(in models.UpdateChannelInput) *apperrors.AppError {
	var details []apperrors.IErrorDetail
	if !auth.ValidDisplayName(in.Name, config.NotifyChannelNameMax) {
		details = append(details, invalid(fieldName))
	}
	if in.Locale != string(i18n.Ar) && in.Locale != string(i18n.En) {
		details = append(details, invalid(fieldLocale))
	}
	if in.Version < 1 {
		details = append(details, invalid("Version"))
	}
	if len(details) > 0 {
		return apperrors.ValidationFailed(details...)
	}
	return nil
}

func (s *Service) planConfig(ctx context.Context, row sqlc.NotificationChannel, in models.UpdateChannelInput) (configChange, error) {
	if row.Kind != models.KindWebhook || (in.URL == nil && !in.RotateSecret) {
		if in.URL != nil || in.RotateSecret {
			return configChange{}, apperrors.ValidationFailed(invalid(fieldURL))
		}
		return configChange{}, nil
	}
	cfg, err := s.openConfig(ctx, row)
	if err != nil && in.URL == nil {
		return configChange{}, err
	}
	out := configChange{cfg: cfg, host: row.Host, changed: true}
	if in.URL != nil {
		target, err := s.checkTarget(ctx, *in.URL)
		if err != nil {
			return configChange{}, err
		}
		out.cfg.URL, out.host = target.URL.String(), target.Display
	}
	if in.RotateSecret || !ValidSecret(out.cfg.Secret) {
		if out.cfg.Secret, err = NewSecret(); err != nil {
			return configChange{}, err
		}
		out.secret = out.cfg.Secret
	}
	return out, nil
}

func (s *Service) UpdateChannel(
	ctx context.Context, scope models.Scope, id uuid.UUID, in models.UpdateChannelInput, factor auth.Factor,
) (models.CreatedChannel, error) {
	if ae := validateUpdate(in); ae != nil {
		return models.CreatedChannel{}, ae
	}
	p, err := s.confirmed(ctx, factor)
	if err != nil {
		return models.CreatedChannel{}, err
	}
	var row sqlc.NotificationChannel
	var change configChange
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		current, err := q.GetNotificationChannelForUpdate(ctx, sqlc.GetNotificationChannelForUpdateParams{ID: id, OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound()
		}
		if err != nil {
			return err
		}
		if current.Version != in.Version {
			return apperrors.ChannelVersionConflict()
		}
		if change, err = s.planConfig(ctx, current, in); err != nil {
			return err
		}
		if row, err = s.applyUpdate(ctx, q, scope, current, in, change, p.UserID); err != nil {
			return err
		}
		actor := p.UserID
		return s.audit(ctx, auditEntry{
			name: eventbus.NotificationChannelUpdated, org: scope.OrganizationID, actor: &actor, ip: auth.RequestMetaFrom(ctx).IPString(),
			target: targetChannel, targetID: row.Id, tx: q, data: s.channelAudit(row, scope),
		})
	})
	if err != nil {
		return models.CreatedChannel{}, err
	}
	return models.CreatedChannel{Channel: channelOf(row), Secret: change.secret}, nil
}

func (s *Service) applyUpdate(
	ctx context.Context, q *sqlc.Queries, scope models.Scope, current sqlc.NotificationChannel, in models.UpdateChannelInput,
	change configChange, actor uuid.UUID,
) (sqlc.NotificationChannel, error) {
	taken, err := q.NotificationChannelNameTaken(ctx, sqlc.NotificationChannelNameTakenParams{
		OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID, Name: in.Name, ExceptID: current.Id,
	})
	if err != nil {
		return sqlc.NotificationChannel{}, err
	}
	if taken {
		return sqlc.NotificationChannel{}, apperrors.ChannelExists()
	}
	row, err := q.UpdateNotificationChannel(ctx, sqlc.UpdateNotificationChannelParams{
		ID: current.Id, OrganizationID: scope.OrganizationID, Name: in.Name, Locale: in.Locale, Version: in.Version,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.NotificationChannel{}, apperrors.ChannelVersionConflict()
	}
	if err != nil {
		return sqlc.NotificationChannel{}, err
	}
	if change.changed {
		sealed, err := s.sealConfig(ctx, q, scope, current.Id, change.cfg)
		if err != nil {
			return sqlc.NotificationChannel{}, err
		}
		if row, err = q.ReplaceNotificationChannelConfig(ctx, sqlc.ReplaceNotificationChannelConfigParams{
			ID: current.Id, OrganizationID: scope.OrganizationID, ConfigSealed: sealed, Host: change.host,
		}); err != nil {
			return sqlc.NotificationChannel{}, err
		}
	}
	if in.Enable && !row.Enabled {
		row, err = q.EnableNotificationChannel(ctx, sqlc.EnableNotificationChannelParams{ID: current.Id, OrganizationID: scope.OrganizationID, ActorID: actor})
	}
	return row, err
}

func (s *Service) RemoveChannel(ctx context.Context, scope models.Scope, id uuid.UUID, factor auth.Factor) (models.Channel, error) {
	p, err := s.confirmed(ctx, factor)
	if err != nil {
		return models.Channel{}, err
	}
	var row sqlc.NotificationChannel
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		if _, err := q.GetNotificationChannelForUpdate(ctx, sqlc.GetNotificationChannelForUpdateParams{ID: id, OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.NotFound()
			}
			return err
		}
		if row, err = q.RemoveNotificationChannel(ctx, sqlc.RemoveNotificationChannelParams{ID: id, OrganizationID: scope.OrganizationID}); err != nil {
			return err
		}
		if err := q.RemoveNotificationRulesOfChannel(ctx, id); err != nil {
			return err
		}
		actor := p.UserID
		return s.audit(ctx, auditEntry{
			name: eventbus.NotificationChannelRemoved, org: scope.OrganizationID, actor: &actor, ip: auth.RequestMetaFrom(ctx).IPString(),
			target: targetChannel, targetID: row.Id, tx: q, data: s.channelAudit(row, scope),
		})
	})
	if err != nil {
		return models.Channel{}, err
	}
	return channelOf(row), nil
}
