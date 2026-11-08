package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Models"
	"github.com/Khaymah/dhow/internal/netguard"
)

const (
	messageInvalid = "errors.validationFailed"
	fieldName      = "Name"
	fieldKind      = "Kind"
	fieldLocale    = "Locale"
	fieldURL       = "Url"
	projectActive  = "active"
	resolveTimeout = 5 * time.Second
)

func invalid(field string) apperrors.IErrorDetail {
	return apperrors.IErrorDetail{Field: field, Message: messageInvalid}
}

func defaultRoles() []string {
	return []string{string(policy.RoleOwner), string(policy.RoleAdmin), string(policy.RoleMaintainer)}
}

func (s *Service) policy() netguard.Policy { return netguard.Policy{Allowed: s.cfg.AllowPrivate} }

func (s *Service) dialer() netguard.Dialer {
	return netguard.Dialer{Policy: s.policy(), Lookup: s.lookup, Timeout: config.NotifyRequestTimeout}
}

func validateCreate(in models.CreateChannelInput) (string, *apperrors.AppError) {
	var details []apperrors.IErrorDetail
	if !slices.Contains(models.Kinds(), in.Kind) {
		details = append(details, invalid(fieldKind))
	}
	if !auth.ValidDisplayName(in.Name, config.NotifyChannelNameMax) {
		details = append(details, invalid(fieldName))
	}
	locale := in.Locale
	if locale == "" {
		locale = string(i18n.DefaultLocale)
	}
	if locale != string(i18n.Ar) && locale != string(i18n.En) {
		details = append(details, invalid(fieldLocale))
	}
	if slices.Contains(models.Kinds(), in.Kind) {
		details = append(details, validInputs(in.Kind, in.URL, in.BotToken, in.ChatID)...)
	}
	if len(details) > 0 {
		return "", apperrors.ValidationFailed(details...)
	}
	return locale, nil
}

func (s *Service) requireActiveProject(ctx context.Context, q *sqlc.Queries, scope models.Scope) error {
	if scope.ProjectID == nil {
		return nil
	}
	state, err := q.NotificationScopeProject(ctx, sqlc.NotificationScopeProjectParams{ProjectID: *scope.ProjectID, OrganizationID: scope.OrganizationID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return apperrors.NotFound()
	case err != nil:
		return err
	case state != projectActive:
		return apperrors.ProjectArchived()
	}
	return nil
}

func scopeKey(scope models.Scope) string {
	if scope.ProjectID != nil {
		return "project:" + scope.ProjectID.String()
	}
	return "org:" + scope.OrganizationID.String()
}

func channelOf(row sqlc.NotificationChannel) models.Channel {
	reason := row.DisabledReason.String
	return models.Channel{
		ID: row.Id, OrganizationID: row.OrganizationId, ProjectID: row.ProjectId, Kind: row.Kind, Name: row.Name, Locale: row.Locale,
		Enabled: row.Enabled, DisabledReason: reason, ConsecutiveFailures: int(row.ConsecutiveFailures), Host: row.Host,
		HasSecret: len(row.ConfigSealed) > 0, Version: row.Version, CreatedByUserID: row.CreatedByUserId,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func isUnique(err error) bool {
	var pe interface{ SQLState() string }
	return errors.As(err, &pe) && pe.SQLState() == "23505"
}

func (s *Service) sealConfig(ctx context.Context, q *sqlc.Queries, scope models.Scope, id uuid.UUID, cfg channelConfig) ([]byte, error) {
	raw, err := json.Marshal(cfg) //nolint:gosec // The settings are sealed with the keyring right after, and never leave the server.
	if err != nil {
		return nil, fmt.Errorf("encode the settings of a notification channel: %w", err)
	}
	return s.column.SealIn(ctx, q, ownerOf(scope.OrganizationID, scope.ProjectID), ChannelSealContext(id), raw)
}

func (s *Service) openConfig(ctx context.Context, row sqlc.NotificationChannel) (channelConfig, error) {
	var cfg channelConfig
	if len(row.ConfigSealed) == 0 {
		return cfg, apperrors.ChannelSecretInvalid()
	}
	raw, err := s.column.Open(ctx, ownerOf(row.OrganizationId, row.ProjectId), ChannelSealContext(row.Id), row.ConfigSealed)
	if err != nil {
		return cfg, fmt.Errorf("open the settings of a notification channel: %w", err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil || !validConfig(row.Kind, cfg) {
		return channelConfig{}, apperrors.ChannelSecretInvalid()
	}
	return cfg, nil
}

func (s *Service) confirmed(ctx context.Context, factor auth.Factor) (auth.Principal, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return auth.Principal{}, err
	}
	if p.ViaToken() {
		return auth.Principal{}, apperrors.SessionRequired()
	}
	if err := s.factors.ConfirmFactor(ctx, p, factor); err != nil {
		return auth.Principal{}, err
	}
	return p, nil
}

func (s *Service) CreateChannel(
	ctx context.Context, scope models.Scope, in models.CreateChannelInput, factor auth.Factor,
) (models.CreatedChannel, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.CreatedChannel{}, err
	}
	locale, ae := validateCreate(in)
	if ae != nil {
		return models.CreatedChannel{}, ae
	}
	change, err := s.newConfig(ctx, in)
	if err != nil {
		return models.CreatedChannel{}, err
	}
	if p, err = s.confirmed(ctx, factor); err != nil {
		return models.CreatedChannel{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return models.CreatedChannel{}, fmt.Errorf("make a channel id: %w", err)
	}
	var row sqlc.NotificationChannel
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		row, err = s.insertChannel(ctx, q, scope, id, in, locale, change, p.UserID)
		if err != nil {
			return err
		}
		actor := p.UserID
		return s.audit(ctx, auditEntry{
			name: eventbus.NotificationChannelCreated, org: scope.OrganizationID, actor: &actor, ip: auth.RequestMetaFrom(ctx).IPString(),
			target: targetChannel, targetID: row.Id, tx: q, data: s.channelAudit(row, scope),
		})
	})
	if err != nil {
		return models.CreatedChannel{}, err
	}
	return models.CreatedChannel{Channel: channelOf(row), Secret: change.secret}, nil
}

func (s *Service) channelAudit(row sqlc.NotificationChannel, scope models.Scope) map[string]string {
	data := map[string]string{"Name": row.Name, "Kind": row.Kind}
	if row.Host != "" {
		data["Host"] = row.Host
	}
	if scope.ProjectID != nil {
		data["ProjectId"] = scope.ProjectID.String()
	}
	return data
}

func (s *Service) insertChannel(
	ctx context.Context, q *sqlc.Queries, scope models.Scope, id uuid.UUID, in models.CreateChannelInput,
	locale string, change configChange, creator uuid.UUID,
) (sqlc.NotificationChannel, error) {
	if err := q.LockNotificationScope(ctx, scopeKey(scope)); err != nil {
		return sqlc.NotificationChannel{}, err
	}
	if err := s.requireActiveProject(ctx, q, scope); err != nil {
		return sqlc.NotificationChannel{}, err
	}
	total, err := q.CountNotificationChannels(ctx, sqlc.CountNotificationChannelsParams{OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID})
	if err != nil {
		return sqlc.NotificationChannel{}, err
	}
	if total >= config.NotifyChannelsPerScope {
		return sqlc.NotificationChannel{}, apperrors.ValidationFailed(invalid(fieldKind))
	}
	var sealed []byte
	if change.changed {
		if sealed, err = s.sealConfig(ctx, q, scope, id, change.cfg); err != nil {
			return sqlc.NotificationChannel{}, err
		}
	}
	row, err := q.InsertNotificationChannel(ctx, sqlc.InsertNotificationChannelParams{
		ID: id, OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID, Kind: in.Kind, Name: in.Name, Locale: locale, Host: change.host,
		ConfigSealed: sealed, CreatedByUserID: creator,
	})
	if isUnique(err) {
		return sqlc.NotificationChannel{}, apperrors.ChannelExists()
	}
	if err != nil {
		return sqlc.NotificationChannel{}, err
	}
	roles := []string{}
	if models.ByRole(in.Kind) {
		roles = defaultRoles()
	}
	_, err = q.InsertNotificationRule(ctx, sqlc.InsertNotificationRuleParams{
		OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID, ChannelID: row.Id, Events: DefaultEvents(),
		Environments: []string{}, Statuses: DefaultStatuses(), Roles: roles,
	})
	return row, err
}

func (s *Service) ListChannels(ctx context.Context, scope models.Scope, page, limit int) (models.ChannelPage, error) {
	q := s.queries()
	total, err := q.CountNotificationChannels(ctx, sqlc.CountNotificationChannelsParams{OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID})
	if err != nil {
		return models.ChannelPage{}, err
	}
	rows, err := q.ListNotificationChannels(ctx, sqlc.ListNotificationChannelsParams{
		OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID, PageLimit: int32(limit), PageOffset: int32((page - 1) * limit), //nolint:gosec // Bounded by the page parameters.
	})
	if err != nil {
		return models.ChannelPage{}, err
	}
	manage := s.canManage(ctx, scope)
	items := make([]models.Channel, 0, len(rows))
	for _, r := range rows {
		items = append(items, hideFromReaders(channelOf(r), manage))
	}
	return models.ChannelPage{Items: items, Total: total}, nil
}

func (s *Service) GetChannel(ctx context.Context, scope models.Scope, id uuid.UUID) (models.Channel, error) {
	row, err := s.queries().GetNotificationChannel(ctx, sqlc.GetNotificationChannelParams{ID: id, OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID})
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Channel{}, apperrors.NotFound()
	}
	if err != nil {
		return models.Channel{}, err
	}
	return hideFromReaders(channelOf(row), s.canManage(ctx, scope)), nil
}

func (s *Service) canManage(ctx context.Context, scope models.Scope) bool {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return false
	}
	if scope.ProjectID != nil {
		return policy.Allows(p.ProjectRole, policy.NotificationManage)
	}
	role, member := p.RoleIn(scope.OrganizationID)
	return member && policy.Allows(role, policy.OrgNotificationManage)
}

func hideFromReaders(c models.Channel, manage bool) models.Channel {
	if !manage {
		c.Host, c.CreatedByUserID = "", uuid.Nil
	}
	return c
}
