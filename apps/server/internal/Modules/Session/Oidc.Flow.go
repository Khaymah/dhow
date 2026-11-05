package session

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	oidcauth "github.com/Khaymah/dhow/apps/server/internal/Core/Oidc"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Session/Models"
	"github.com/Khaymah/dhow/internal/logging"
)

const (
	purposeLogin = "login"
	purposeLink  = "link"
	labelPKCE    = "pkce"
	labelNonce   = "nonce"

	reasonRejected   = "rejected"
	reasonDomain     = "domain"
	reasonNotLinked  = "not_linked"
	reasonDisabled   = "disabled"
	reasonProviderNA = "unavailable"
)

type takenFlow struct {
	row      sqlc.TakeOidcFlowRow
	provider oidcauth.Provider
	flow     oidcauth.Flow
}

func loginOf(id oidcauth.Identity) pgtype.Text {
	return pgtype.Text{String: id.Login, Valid: id.Login != ""}
}

type exchanged struct {
	identity oidcauth.Identity
	email    string
}

func (s *Service) derive(label, cookie string) string {
	mac := hmac.New(sha256.New, s.oidcKey)
	mac.Write([]byte(label))
	mac.Write([]byte{0})
	mac.Write([]byte(cookie))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Service) flowFor(cookie, state, provider string) oidcauth.Flow {
	return oidcauth.Flow{
		State: state, Nonce: s.derive(labelNonce, cookie), Verifier: s.derive(labelPKCE, cookie),
		RedirectURL: s.callbackBase + provider,
	}
}

func (s *Service) oidcProvider(id string) (oidcauth.Provider, error) {
	p, err := s.oidc.Get(id)
	if err != nil {
		return nil, apperrors.NotFound()
	}
	return p, nil
}

func (s *Service) providerUnavailable(ctx context.Context, provider string, err error) error {
	s.log.WarnContext(ctx, "identity provider is unavailable", "provider", provider, "error", logging.UserValue(err.Error()))
	return apperrors.OidcProviderUnavailable()
}

func (s *Service) startFlow(ctx context.Context, purpose, providerID string, userID, sessionID *uuid.UUID) (models.OidcAuthorization, error) {
	p, err := s.oidcProvider(providerID)
	if err != nil {
		return models.OidcAuthorization{}, err
	}
	cookie, cookieHash, err := auth.NewSessionToken(config.SessionTokenBytes)
	if err != nil {
		return models.OidcAuthorization{}, err
	}
	state, stateHash, err := auth.NewSessionToken(config.SessionTokenBytes)
	if err != nil {
		return models.OidcAuthorization{}, err
	}
	url, err := p.AuthorizeURL(ctx, s.flowFor(cookie, state, providerID))
	if err != nil {
		return models.OidcAuthorization{}, s.providerUnavailable(ctx, providerID, err)
	}
	now := s.now()
	expires := now.Add(config.OidcFlowTTL)
	_, err = s.queries().InsertOidcFlow(ctx, sqlc.InsertOidcFlowParams{
		Purpose: purpose, Provider: providerID, Issuer: p.Issuer(), TokenHash: cookieHash, StateHash: stateHash,
		DeviceHash: auth.DeviceHash(auth.RequestMetaFrom(ctx).UserAgent), UserID: userID, SessionID: sessionID,
		ExpiresAt: expires, Now: now,
	})
	if err != nil {
		return models.OidcAuthorization{}, err
	}
	return models.OidcAuthorization{URL: url, ExpiresAt: expires, CookieValue: cookie}, nil
}

func (s *Service) takeFlow(ctx context.Context, purpose string, in models.OidcCallback, now time.Time) (takenFlow, error) {
	meta := auth.RequestMetaFrom(ctx)
	cookieHash, ok := auth.HashSessionToken(meta.OidcToken, config.SessionTokenBytes)
	if !ok {
		return takenFlow{}, apperrors.OidcFlowInvalid()
	}
	row, err := s.queries().TakeOidcFlow(ctx, sqlc.TakeOidcFlowParams{TokenHash: cookieHash, Purpose: purpose, Now: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return takenFlow{}, apperrors.OidcFlowInvalid()
	}
	if err != nil {
		return takenFlow{}, err
	}
	stateHash, ok := auth.HashSessionToken(in.State, config.SessionTokenBytes)
	if !ok || subtle.ConstantTimeCompare(stateHash, row.StateHash) != 1 || row.Provider != in.Provider ||
		subtle.ConstantTimeCompare(row.DeviceHash, auth.DeviceHash(meta.UserAgent)) != 1 {
		return takenFlow{}, apperrors.OidcFlowInvalid()
	}
	p, err := s.oidcProvider(row.Provider)
	if err != nil || row.Issuer != p.Issuer() {
		return takenFlow{}, apperrors.OidcFlowInvalid()
	}
	return takenFlow{row: row, provider: p, flow: s.flowFor(meta.OidcToken, in.State, row.Provider)}, nil
}

func (s *Service) exchange(ctx context.Context, t takenFlow, code string) (exchanged, string, error) {
	id, err := t.provider.Exchange(ctx, code, t.flow)
	if errors.Is(err, oidcauth.ErrUnavailable) {
		return exchanged{}, reasonProviderNA, s.providerUnavailable(ctx, t.row.Provider, err)
	}
	if err != nil {
		s.log.WarnContext(ctx, "identity provider response rejected", "provider", t.row.Provider, "error", logging.UserValue(err.Error()))
		return exchanged{}, reasonRejected, apperrors.OidcSignInFailed()
	}
	email, ok := oidcauth.AcceptedEmail(id, t.provider.AllowedDomains())
	if !ok {
		return exchanged{}, reasonDomain, apperrors.OidcDomainNotAllowed()
	}
	return exchanged{identity: id, email: email}, "", nil
}

func subjectDigest(issuer, subject string) string {
	sum := sha256.Sum256([]byte(issuer + "\x00" + subject))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:emailDigestLength]
}

func (s *Service) failOidcLogin(ctx context.Context, provider, reason string, userID *uuid.UUID, id oidcauth.Identity, ae error) error {
	data := map[string]string{"Provider": provider, "Reason": reason}
	if id.Subject != "" {
		data["SubjectDigest"] = subjectDigest(id.Issuer, id.Subject)
	}
	var err error
	if userID != nil {
		err = s.publishFor(ctx, eventbus.AuthOidcLoginFailed, *userID, "user", userID.String(), data)
	} else {
		err = s.bus.Publish(ctx, eventbus.Event{Name: eventbus.AuthOidcLoginFailed, Payload: eventbus.Audited{
			TargetType: "identity", IP: auth.RequestMetaFrom(ctx).IPString(), Data: data,
		}})
	}
	if err != nil {
		return err
	}
	return ae
}

func (s *Service) ListOidcProviders() []models.OidcProvider {
	infos := s.oidc.List()
	out := make([]models.OidcProvider, 0, len(infos))
	for _, i := range infos {
		out = append(out, models.OidcProvider{ID: i.ID, Name: i.Name, Kind: i.Kind})
	}
	return out
}
