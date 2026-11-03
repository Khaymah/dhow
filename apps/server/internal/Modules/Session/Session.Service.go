package session

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	oidcauth "github.com/Khaymah/dhow/apps/server/internal/Core/Oidc"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Session/Models"
)

type DB interface {
	sqlc.DBTX
	dbtx.Beginner
}

type Publisher interface {
	Publish(ctx context.Context, event eventbus.Event) error
}

type Options struct {
	DB        DB
	Hasher    *auth.Hasher
	Passwords *auth.PasswordPolicy
	CSRF      *auth.CSRF
	Totp      *keys.Column
	Recovery  *auth.RecoveryHasher
	WebAuthn  *webauthn.WebAuthn
	Bus       Publisher
	Instance  func() policy.InstanceSettings
	Log       *slog.Logger
	Now       func() time.Time

	Oidc         *oidcauth.Registry
	OidcKey      []byte
	CallbackBase string
	EnvProviders []string
}

type Service struct {
	db        DB
	hasher    *auth.Hasher
	passwords *auth.PasswordPolicy
	csrf      *auth.CSRF
	totp      *keys.Column
	recovery  *auth.RecoveryHasher
	webauthn  *webauthn.WebAuthn
	bus       Publisher
	instance  func() policy.InstanceSettings
	log       *slog.Logger
	now       func() time.Time

	oidc         *oidcauth.Registry
	oidcKey      []byte
	callbackBase string
	envProviders []string
}

var (
	_ auth.Resolver        = (*Service)(nil)
	_ auth.FactorConfirmer = (*Service)(nil)
)

func NewService(o Options) *Service {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	instance := o.Instance
	if instance == nil {
		instance = func() policy.InstanceSettings { return policy.InstanceSettings{} }
	}
	return &Service{
		db: o.DB, hasher: o.Hasher, passwords: o.Passwords, csrf: o.CSRF, totp: o.Totp, recovery: o.Recovery,
		webauthn: o.WebAuthn, bus: o.Bus, instance: instance, log: o.Log, now: now,
		oidc: o.Oidc, oidcKey: o.OidcKey, callbackBase: o.CallbackBase, envProviders: append([]string{}, o.EnvProviders...),
	}
}

func (s *Service) queries() *sqlc.Queries { return sqlc.New(s.db) }

func (s *Service) Resolve(ctx context.Context, tokenHash []byte, now time.Time) (auth.Resolved, error) {
	return s.resolve(ctx, tokenHash, now, true)
}

func (s *Service) Peek(ctx context.Context, tokenHash []byte, now time.Time) (auth.Resolved, error) {
	return s.resolve(ctx, tokenHash, now, false)
}

func (s *Service) resolve(ctx context.Context, tokenHash []byte, now time.Time, touch bool) (auth.Resolved, error) {
	q := s.queries()
	row, err := q.ResolveSession(ctx, sqlc.ResolveSessionParams{
		TokenHash: tokenHash, Now: now, CreatedAfter: now.Add(-config.SessionAbsoluteTimeout),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Resolved{}, auth.ErrNoSession
	}
	if err != nil {
		return auth.Resolved{}, err
	}
	rows, err := q.ListUserMemberships(ctx, row.UserId)
	if err != nil {
		return auth.Resolved{}, err
	}
	memberships := policyMemberships(membershipsOf(rows, row.HasSecondFactor))
	idle := idleTimeout(row.IsInstanceOwner, memberships)
	if idleExpired(row.LastSeenAt, idle, now) {
		return auth.Resolved{}, auth.ErrNoSession
	}
	if touch && now.Sub(row.LastSeenAt) >= config.SessionTouchInterval {
		touchErr := q.TouchSession(ctx, sqlc.TouchSessionParams{
			ID: row.Id, Now: now, IdleExpiresAt: now.Add(idle), TouchBefore: now.Add(-config.SessionTouchInterval),
		})
		if touchErr != nil {
			s.log.WarnContext(ctx, "touch session failed", "error", touchErr)
		}
	}
	return auth.Resolved{
		SessionID: row.Id, UserID: row.UserId, InstanceOwner: row.IsInstanceOwner, CsrfHash: row.CsrfHash,
		ExpiresAt: absoluteExpiry(row.CreatedAt, row.ExpiresAt),
		Email:     row.Email, Name: row.Name, Locale: row.Locale, Memberships: memberships,
	}, nil
}

func (s *Service) TouchMembership(ctx context.Context, user, org uuid.UUID, now time.Time) error {
	return s.queries().TouchMembership(ctx, sqlc.TouchMembershipParams{
		OrganizationID: org, UserID: user, Now: now, TouchBefore: now.Add(-config.MembershipTouchInterval),
	})
}

func (s *Service) ConfirmFactor(ctx context.Context, p auth.Principal, f auth.Factor) error {
	if p.ViaToken() || p.SessionID == uuid.Nil {
		return apperrors.SessionRequired()
	}
	return s.confirmFactor(ctx, p, models.Factor(f))
}

func principalOf(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return auth.Principal{}, apperrors.Unauthenticated()
	}
	return p, nil
}

func (s *Service) Me(ctx context.Context) (models.Me, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Me{}, err
	}
	q := s.queries()
	rows, err := q.ListUserMemberships(ctx, p.UserID)
	if err != nil {
		return models.Me{}, err
	}
	flags, err := q.GetUserAccountFlags(ctx, p.UserID)
	if err != nil {
		return models.Me{}, err
	}
	factors, err := q.GetMfaFactors(ctx, p.UserID)
	if err != nil {
		return models.Me{}, err
	}
	resolved := membershipsOf(rows, hasMfa(factors))
	memberships := make([]models.Membership, 0, len(resolved))
	for _, m := range resolved {
		view := models.Membership{
			OrganizationID: m.OrganizationID, OrganizationName: m.row.OrganizationName,
			OrganizationSlug: m.row.OrganizationSlug, Role: string(m.Role),
		}
		if !m.SecondFactorDue.IsZero() {
			due := m.SecondFactorDue
			view.SecondFactorDue = &due
		}
		memberships = append(memberships, view)
	}
	return models.Me{
		User:        models.User{ID: p.UserID, Email: p.Email, Name: p.Name, Locale: p.Locale},
		Memberships: memberships, CSRFToken: p.CSRFToken, SessionID: p.SessionID, ExpiresAt: p.ExpiresAt,
		CanCreateOrganizations: policy.CanInstance(p.Subject, policy.OrgCreate, s.instance()),
		InstanceOwner:          p.InstanceOwner && !p.ViaToken(),
		MfaReenrollRequired:    flags.MfaResetAt != nil && !hasMfa(factors),
	}, nil
}

func (s *Service) List(ctx context.Context) ([]models.SessionInfo, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	rows, err := s.queries().ListActiveUserSessions(ctx, sqlc.ListActiveUserSessionsParams{
		UserID: p.UserID, Now: now, CreatedAfter: now.Add(-config.SessionAbsoluteTimeout),
	})
	if err != nil {
		return nil, err
	}
	idle := idleTimeout(p.InstanceOwner, p.Memberships)
	out := make([]models.SessionInfo, 0, len(rows))
	for _, r := range rows {
		if idleExpired(r.LastSeenAt, idle, now) {
			continue
		}
		out = append(out, models.SessionInfo{
			ID: r.Id, CreatedAt: r.CreatedAt, LastSeenAt: r.LastSeenAt, ExpiresAt: absoluteExpiry(r.CreatedAt, r.ExpiresAt),
			UserAgent: r.UserAgent, IPAddress: r.IpAddress, Current: r.Id == p.SessionID,
		})
	}
	return out, nil
}

func (s *Service) Logout(ctx context.Context) error {
	p, err := principalOf(ctx)
	if err != nil {
		return err
	}
	if _, err := s.queries().RevokeUserSession(ctx, sqlc.RevokeUserSessionParams{ID: p.SessionID, UserID: p.UserID, Now: s.now()}); err != nil {
		return err
	}
	return s.publish(ctx, eventbus.AuthLogout, p, p.SessionID.String(), nil)
}

func (s *Service) Revoke(ctx context.Context, id uuid.UUID) error {
	p, err := principalOf(ctx)
	if err != nil {
		return err
	}
	n, err := s.queries().RevokeUserSession(ctx, sqlc.RevokeUserSessionParams{ID: id, UserID: p.UserID, Now: s.now()})
	if err != nil {
		return err
	}
	if n == 0 {
		return apperrors.NotFound()
	}
	return s.publish(ctx, eventbus.AuthSessionRevoked, p, id.String(), nil)
}

func (s *Service) RevokeOthers(ctx context.Context) (int64, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return 0, err
	}
	n, err := s.queries().RevokeOtherUserSessions(ctx, sqlc.RevokeOtherUserSessionsParams{UserID: p.UserID, ExceptID: p.SessionID, Now: s.now()})
	if err != nil {
		return 0, err
	}
	return n, s.publish(ctx, eventbus.AuthSessionsRevokedOthers, p, p.UserID.String(), map[string]string{"Revoked": itoa(n)})
}

func (s *Service) publish(ctx context.Context, name string, p auth.Principal, targetID string, data map[string]string) error {
	return s.publishTarget(ctx, name, p, "session", targetID, data)
}

func (s *Service) publishTarget(ctx context.Context, name string, p auth.Principal, targetType, targetID string, data map[string]string) error {
	meta := auth.RequestMetaFrom(ctx)
	actor := p.UserID
	orgs := make([]uuid.UUID, 0, len(p.Memberships))
	for _, m := range p.Memberships {
		orgs = append(orgs, m.OrganizationID)
	}
	return s.bus.Publish(ctx, eventbus.Event{Name: name, Payload: eventbus.Audited{
		OrganizationIDs: orgs, ActorUserID: &actor, TargetType: targetType, TargetID: targetID, IP: meta.IPString(), Data: data,
	}})
}
