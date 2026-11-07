package notifications

import (
	"context"
	"errors"
	"regexp"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Models"
)

const (
	fieldRules        = "Rules"
	fieldEvents       = "Events"
	fieldStatuses     = "Statuses"
	fieldEnvironments = "Environments"
	fieldRoles        = "Roles"
	environmentsMax   = 50
	ruleChannelsMax   = 20
)

var environmentName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func ruleOf(r sqlc.NotificationRule) models.Rule {
	return models.Rule{
		ChannelID: r.ChannelId, Events: r.Events, Environments: r.Environments, Statuses: r.Statuses, Roles: r.Roles, Version: r.Version,
	}
}

func dedupe(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func validRule(kind string, r models.Rule) []apperrors.IErrorDetail {
	var details []apperrors.IErrorDetail
	if len(r.Events) == 0 || len(r.Events) > len(Events()) || slices.ContainsFunc(r.Events, func(e string) bool { return !ValidEvent(e) }) {
		details = append(details, invalid(fieldEvents))
	}
	if len(r.Statuses) == 0 || len(r.Statuses) > len(Statuses()) || slices.ContainsFunc(r.Statuses, func(e string) bool { return !ValidStatus(e) }) {
		details = append(details, invalid(fieldStatuses))
	}
	if len(r.Environments) > environmentsMax || slices.ContainsFunc(r.Environments, func(e string) bool { return !environmentName.MatchString(e) }) {
		details = append(details, invalid(fieldEnvironments))
	}
	badRole := slices.ContainsFunc(r.Roles, func(v string) bool { _, ok := policy.ParseRole(v); return !ok })
	if len(r.Roles) > config.NotifyRolesMax || badRole || (kind == models.KindEmail) == (len(r.Roles) == 0) {
		details = append(details, invalid(fieldRoles))
	}
	return details
}

func (s *Service) GetRules(ctx context.Context, scope models.Scope) ([]models.Rule, error) {
	rows, err := s.queries().ListNotificationRules(ctx, sqlc.ListNotificationRulesParams{OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID})
	if err != nil {
		return nil, err
	}
	out := make([]models.Rule, 0, len(rows))
	for _, r := range rows {
		out = append(out, ruleOf(r))
	}
	return out, nil
}

func (s *Service) PutRules(ctx context.Context, scope models.Scope, in []models.Rule) ([]models.Rule, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return nil, err
	}
	if p.ViaToken() {
		return nil, apperrors.SessionRequired()
	}
	if len(in) == 0 || len(in) > ruleChannelsMax {
		return nil, apperrors.ValidationFailed(invalid(fieldRules))
	}
	seen := map[uuid.UUID]bool{}
	for _, r := range in {
		if seen[r.ChannelID] {
			return nil, apperrors.ValidationFailed(invalid(fieldRules))
		}
		seen[r.ChannelID] = true
	}
	var out []models.Rule
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		out = out[:0]
		for _, r := range in {
			saved, err := s.putRule(ctx, q, scope, p.UserID, r)
			if err != nil {
				return err
			}
			out = append(out, saved)
		}
		return nil
	})
	return out, err
}

func (s *Service) putRule(ctx context.Context, q *sqlc.Queries, scope models.Scope, actor uuid.UUID, r models.Rule) (models.Rule, error) {
	ch, err := q.GetNotificationChannelForUpdate(ctx, sqlc.GetNotificationChannelForUpdateParams{ID: r.ChannelID, OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID})
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Rule{}, apperrors.ValidationFailed(invalid(fieldRules))
	}
	if err != nil {
		return models.Rule{}, err
	}
	r.Events, r.Statuses, r.Environments, r.Roles = dedupe(r.Events), dedupe(r.Statuses), dedupe(r.Environments), dedupe(r.Roles)
	if details := validRule(ch.Kind, r); len(details) > 0 {
		return models.Rule{}, apperrors.ValidationFailed(details...)
	}
	params := sqlc.UpdateNotificationRuleParams{ChannelID: ch.Id, Events: r.Events, Environments: nonNil(r.Environments), Statuses: r.Statuses, Roles: nonNil(r.Roles)}
	row, err := q.UpdateNotificationRule(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		row, err = q.InsertNotificationRule(ctx, sqlc.InsertNotificationRuleParams{
			OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID, ChannelID: ch.Id, Events: params.Events,
			Environments: params.Environments, Statuses: params.Statuses, Roles: params.Roles,
		})
	}
	if err != nil {
		return models.Rule{}, err
	}
	return ruleOf(row), s.audit(ctx, auditEntry{
		name: eventbus.NotificationRuleUpdated, org: scope.OrganizationID, actor: &actor, ip: auth.RequestMetaFrom(ctx).IPString(),
		target: targetRule, targetID: ch.Id, tx: q, data: s.channelAudit(ch, scope),
	})
}

func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
