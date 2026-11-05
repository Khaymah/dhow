package session

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Session/Models"
)

const targetIdentity = "identity"

func (s *Service) providerName(id string) string {
	if p, err := s.oidc.Get(id); err == nil {
		return p.Info().Name
	}
	return id
}

func (s *Service) ListIdentities(ctx context.Context) ([]models.LinkedIdentity, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries().ListUserIdentities(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]models.LinkedIdentity, 0, len(rows))
	for _, r := range rows {
		out = append(out, models.LinkedIdentity{
			ID: r.Id, Provider: r.Provider, ProviderName: s.providerName(r.Provider), Email: r.Email,
			CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt,
		})
	}
	return out, nil
}

func (s *Service) BeginOidcLink(ctx context.Context, provider string, factor models.Factor) (models.OidcAuthorization, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.OidcAuthorization{}, err
	}
	if _, err := s.oidcProvider(provider); err != nil {
		return models.OidcAuthorization{}, err
	}
	linked, err := s.queries().HasIdentityForProvider(ctx, sqlc.HasIdentityForProviderParams{UserID: p.UserID, Provider: provider})
	if err != nil {
		return models.OidcAuthorization{}, err
	}
	if linked {
		return models.OidcAuthorization{}, apperrors.OidcAlreadyLinked()
	}
	if err := s.confirmFactor(ctx, p, factor); err != nil {
		return models.OidcAuthorization{}, err
	}
	userID, sessionID := p.UserID, p.SessionID
	return s.startFlow(ctx, purposeLink, provider, &userID, &sessionID)
}

func (s *Service) CompleteOidcLink(ctx context.Context, in models.OidcCallback) (models.IdentityChange, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.IdentityChange{}, err
	}
	now := s.now()
	t, err := s.takeFlow(ctx, purposeLink, in, now)
	if err != nil {
		return models.IdentityChange{}, err
	}
	if t.row.UserId == nil || t.row.SessionId == nil || *t.row.UserId != p.UserID || *t.row.SessionId != p.SessionID {
		return models.IdentityChange{}, apperrors.OidcFlowInvalid()
	}
	got, _, err := s.exchange(ctx, t, in.Code)
	if err != nil {
		return models.IdentityChange{}, err
	}
	var change models.IdentityChange
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		if err := s.currentIssuer(ctx, q, t, got.identity.Issuer); err != nil {
			return err
		}
		row, err := q.InsertIdentity(ctx, sqlc.InsertIdentityParams{
			UserID: p.UserID, Provider: t.row.Provider, Issuer: got.identity.Issuer, Subject: got.identity.Subject,
			Email: got.email, Login: loginOf(got.identity), Now: now,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return s.linkConflict(ctx, q, p.UserID, t.row.Provider, got)
		}
		if err != nil {
			return err
		}
		change.Identity = models.LinkedIdentity{
			ID: row.Id, Provider: row.Provider, ProviderName: s.providerName(row.Provider), Email: row.Email,
			CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt,
		}
		change.Revoked, err = q.RevokeOtherUserSessions(ctx, sqlc.RevokeOtherUserSessionsParams{UserID: p.UserID, ExceptID: p.SessionID, Now: now})
		return err
	})
	if err != nil {
		return models.IdentityChange{}, err
	}
	return change, s.publishTarget(ctx, eventbus.AuthIdentityLinked, p, targetIdentity, change.Identity.ID.String(), map[string]string{
		"Provider": t.row.Provider, "RevokedSessions": itoa(change.Revoked),
	})
}

func (s *Service) linkConflict(ctx context.Context, q *sqlc.Queries, userID uuid.UUID, provider string, got exchanged) error {
	owner, err := q.FindIdentityOwner(ctx, sqlc.FindIdentityOwnerParams{
		Issuer: got.identity.Issuer, Subject: got.identity.Subject, UserID: userID, Provider: provider,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.Conflict()
	}
	if err != nil {
		return err
	}
	if owner.UserId == userID {
		return apperrors.OidcAlreadyLinked()
	}
	return apperrors.OidcIdentityInUse()
}

func (s *Service) lockSignInMethods(ctx context.Context, q *sqlc.Queries, userID uuid.UUID) (sqlc.LockUserSignInMethodsRow, error) {
	if err := q.LockUserRow(ctx, userID); err != nil {
		return sqlc.LockUserSignInMethodsRow{}, err
	}
	return q.LockUserSignInMethods(ctx, sqlc.LockUserSignInMethodsParams{ID: userID, Env: s.envProviders})
}

func (s *Service) currentIssuer(ctx context.Context, q *sqlc.Queries, t takenFlow, issuer string) error {
	if err := q.LockOidcProvidersShared(ctx, config.OidcProvidersLockKey); err != nil {
		return err
	}
	current := t.provider.Issuer()
	if len(s.envProviders) == 0 {
		stored, err := q.CurrentProviderIssuer(ctx, t.row.Provider)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.OidcFlowInvalid()
		}
		if err != nil {
			return err
		}
		current = stored
	}
	if current != issuer {
		return apperrors.OidcFlowInvalid()
	}
	return nil
}

func signInMethods(m sqlc.LockUserSignInMethodsRow) int64 {
	n := m.Identities + m.Passkeys
	if m.HasPassword {
		n++
	}
	return n
}

func (s *Service) RemoveIdentity(ctx context.Context, id uuid.UUID, factor models.Factor) (int64, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return 0, err
	}
	owned, err := s.ListIdentities(ctx)
	if err != nil {
		return 0, err
	}
	target, ok := findIdentity(owned, id)
	if !ok {
		return 0, apperrors.NotFound()
	}
	if err := s.confirmFactor(ctx, p, factor); err != nil {
		return 0, err
	}
	now := s.now()
	var revoked int64
	var provider string
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		methods, err := s.lockSignInMethods(ctx, q, p.UserID)
		if err != nil {
			return err
		}
		active, err := q.SignInProviderActive(ctx, sqlc.SignInProviderActiveParams{Provider: target.Provider, Env: s.envProviders})
		if err != nil {
			return err
		}
		if active && signInMethods(methods) <= 1 {
			return apperrors.LastSignInMethod()
		}
		provider, err = q.RemoveUserIdentity(ctx, sqlc.RemoveUserIdentityParams{ID: id, UserID: p.UserID, Now: now})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound()
		}
		if err != nil {
			return err
		}
		revoked, err = q.RevokeOtherUserSessions(ctx, sqlc.RevokeOtherUserSessionsParams{UserID: p.UserID, ExceptID: p.SessionID, Now: now})
		return err
	})
	if err != nil {
		return 0, err
	}
	return revoked, s.publishTarget(ctx, eventbus.AuthIdentityUnlinked, p, targetIdentity, id.String(), map[string]string{
		"Provider": provider, "RevokedSessions": itoa(revoked),
	})
}

func findIdentity(list []models.LinkedIdentity, id uuid.UUID) (models.LinkedIdentity, bool) {
	for _, i := range list {
		if i.ID == id {
			return i, true
		}
	}
	return models.LinkedIdentity{}, false
}
