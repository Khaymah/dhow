package session

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Session/Models"
)

func (s *Service) BeginOidcLogin(ctx context.Context, provider string) (models.OidcAuthorization, error) {
	return s.startFlow(ctx, purposeLogin, provider, nil, nil)
}

func (s *Service) CompleteOidcLogin(ctx context.Context, in models.OidcCallback) (models.LoginResult, error) {
	meta := auth.RequestMetaFrom(ctx)
	now := s.now()
	t, err := s.takeFlow(ctx, purposeLogin, in, now)
	if err != nil {
		return models.LoginResult{}, err
	}
	got, reason, err := s.exchange(ctx, t, in.Code)
	if err != nil {
		return models.LoginResult{}, s.failOidcLogin(ctx, t.row.Provider, reason, nil, got.identity, err)
	}
	user, err := s.queries().FindUserByIdentity(ctx, sqlc.FindUserByIdentityParams{Issuer: got.identity.Issuer, Subject: got.identity.Subject})
	if errors.Is(err, pgx.ErrNoRows) {
		return models.LoginResult{}, s.failOidcLogin(ctx, t.row.Provider, reasonNotLinked, nil, got.identity, apperrors.OidcNotLinked())
	}
	if err != nil {
		return models.LoginResult{}, err
	}
	if user.Disabled {
		id := user.Id
		return models.LoginResult{}, s.failOidcLogin(ctx, t.row.Provider, reasonDisabled, &id, got.identity, apperrors.OidcSignInFailed())
	}
	if err := s.queries().TouchIdentity(ctx, sqlc.TouchIdentityParams{ID: user.IdentityId, Now: now, Email: got.email, Login: loginOf(got.identity)}); err != nil {
		return models.LoginResult{}, err
	}
	who := sessionUser{ID: user.Id, Email: user.Email, Name: user.Name, Locale: user.Locale}
	factors, err := s.queries().GetMfaFactors(ctx, user.Id)
	if err != nil {
		return models.LoginResult{}, err
	}
	method := models.MethodOidc + ":" + t.row.Provider
	if hasMfa(factors) {
		return s.issuePending(ctx, now, who, factors, meta, method)
	}
	return s.openSession(ctx, now, who, meta, method, "")
}
