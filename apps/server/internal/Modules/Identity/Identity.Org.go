package identity

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Identity/Models"
)

func (s *Service) GetOrganization(ctx context.Context, id uuid.UUID) (models.Organization, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return models.Organization{}, apperrors.Unauthenticated()
	}
	row, err := s.queries().GetOrganizationForMember(ctx, sqlc.GetOrganizationForMemberParams{ID: id, UserID: p.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Organization{}, apperrors.NotFound()
	}
	if err != nil {
		return models.Organization{}, err
	}
	org := organizationOf(row.Id, row.Name, row.Slug, row.CreatedAt, row.RequireSecondFactor, row.RequireSecondFactorSince)
	org.LogRetentionDays = daysOf(row.LogRetentionDays)
	return org, nil
}

func (s *Service) UpdateLogRetention(ctx context.Context, id uuid.UUID, days *int, factor auth.Factor) (models.Organization, error) {
	p, _, err := actorIn(ctx, id)
	if err != nil {
		return models.Organization{}, err
	}
	var value pgtype.Int4
	if days != nil {
		if *days < config.OrgLogRetentionMin || *days > config.LogRetentionMax {
			return models.Organization{}, apperrors.ValidationFailed(invalid("LogRetentionDays"))
		}
		value = pgtype.Int4{Int32: int32(*days), Valid: true}
	}
	state, err := s.queries().GetUserMfaState(ctx, p.UserID)
	if err != nil {
		return models.Organization{}, err
	}
	if !state.HasTotp && !state.HasPasskey {
		return models.Organization{}, apperrors.SecondFactorRequired()
	}
	if err := s.factors.ConfirmFactor(ctx, p, factor); err != nil {
		return models.Organization{}, err
	}
	shown := "server"
	if value.Valid {
		shown = strconv.Itoa(int(value.Int32))
	}
	var org models.Organization
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		if _, err := q.LockOrganization(ctx, id); errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound()
		} else if err != nil {
			return err
		}
		self, err := loadMember(ctx, q, id, p.UserID)
		if err != nil {
			return err
		}
		if self.Direct != policy.RoleOwner {
			return apperrors.Forbidden()
		}
		row, err := q.SetOrganizationLogRetention(ctx, sqlc.SetOrganizationLogRetentionParams{ID: id, Days: value, Now: s.now()})
		if err != nil {
			return err
		}
		org = organizationOf(row.Id, row.Name, row.Slug, row.CreatedAt, row.RequireSecondFactor, row.RequireSecondFactorSince)
		org.LogRetentionDays = daysOf(row.LogRetentionDays)
		actor := p.UserID
		return s.bus.Publish(ctx, eventbus.Event{Name: eventbus.IdentityOrganizationUpdated, Payload: eventbus.Audited{
			OrganizationIDs: []uuid.UUID{id}, ActorUserID: &actor, TargetType: "organization", TargetID: id.String(),
			IP: auth.RequestMetaFrom(ctx).IPString(), Data: map[string]string{"LogRetentionDays": shown}, Tx: q,
		}})
	})
	if err != nil {
		return models.Organization{}, err
	}
	return org, nil
}

func (s *Service) UpdateOrganization(ctx context.Context, id uuid.UUID, name string) (models.Organization, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return models.Organization{}, apperrors.Unauthenticated()
	}
	if !auth.ValidDisplayName(name, maxNameRunes) {
		return models.Organization{}, apperrors.ValidationFailed(invalid(fieldName))
	}
	row, err := s.queries().UpdateOrganizationName(ctx, sqlc.UpdateOrganizationNameParams{ID: id, UserID: p.UserID, Name: name, Now: s.now()})
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Organization{}, apperrors.NotFound()
	}
	if err != nil {
		return models.Organization{}, err
	}
	actor := p.UserID
	err = s.bus.Publish(ctx, eventbus.Event{Name: eventbus.IdentityOrganizationUpdated, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{id}, ActorUserID: &actor, TargetType: "organization", TargetID: id.String(),
		IP: auth.RequestMetaFrom(ctx).IPString(), Data: map[string]string{"Name": name},
	}})
	if err != nil {
		return models.Organization{}, err
	}
	return organizationOf(row.Id, row.Name, row.Slug, row.CreatedAt, row.RequireSecondFactor, row.RequireSecondFactorSince), nil
}

func mfaOf(totp, passkey bool) string {
	switch {
	case passkey:
		return "passkey"
	case totp:
		return "totp"
	default:
		return "none"
	}
}

func memberTeams(ids []uuid.UUID, names []string) []models.MemberTeam {
	out := make([]models.MemberTeam, 0, len(ids))
	for i, id := range ids {
		if i >= len(names) {
			break
		}
		out = append(out, models.MemberTeam{ID: id, Name: names[i]})
	}
	return out
}

func (s *Service) ListMembers(ctx context.Context, id uuid.UUID, page, limit int) ([]models.Member, int64, error) {
	_, actor, err := actorIn(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	manager := policy.Allows(actor, policy.MemberManage)
	q := s.queries()
	total, err := q.CountMembers(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListMembers(ctx, sqlc.ListMembersParams{
		OrganizationID: id, PageLimit: int32(limit), PageOffset: int32((page - 1) * limit), //nolint:gosec // limit and page are bounded by response.PageParams.
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]models.Member, 0, len(rows))
	for _, r := range rows {
		effective, ok := policy.EffectiveOf(r.Role, r.TeamRoles)
		if !ok {
			continue
		}
		m := models.Member{
			UserID: r.UserId, Name: r.Name, Email: r.Email, Role: r.Role, EffectiveRole: string(effective), JoinedAt: r.JoinedAt,
			Teams: memberTeams(r.TeamIds, r.TeamNames),
		}
		if manager {
			m.Mfa = mfaOf(r.HasTotp, r.HasPasskey)
			m.MfaResetPending = r.MfaResetAt != nil && m.Mfa == "none"
			m.LastActiveAt = r.LastActiveAt
		}
		out = append(out, m)
	}
	return out, total, nil
}

func daysOf(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	d := v.Int32
	return &d
}
