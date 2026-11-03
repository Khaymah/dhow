package instancesettings

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	settingspec "github.com/Khaymah/dhow/apps/server/internal/Config/Settings"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const (
	GroupOidc    = "oidc"
	oidcSealPart = "oidc:"
)

type OidcProvider struct {
	Config    settingspec.OIDCProvider
	SecretSet bool
	Sealed    bool
	Version   int64
	CreatedAt *time.Time
	UpdatedAt *time.Time
	UpdatedBy *uuid.UUID
}

func (p OidcProvider) Enabled() bool { return p.SecretSet }

type Oidc struct {
	Source    Source
	Providers []OidcProvider
}

func (o Oidc) Locked() bool { return o.Source == SourceEnv }

func (o Oidc) Active() []settingspec.OIDCProvider {
	out := make([]settingspec.OIDCProvider, 0, len(o.Providers))
	for _, p := range o.Providers {
		if p.Enabled() {
			out = append(out, p.Config)
		}
	}
	return out
}

func (o Oidc) Find(id string) (OidcProvider, bool) {
	for _, p := range o.Providers {
		if p.Config.ID == id {
			return p, true
		}
	}
	return OidcProvider{}, false
}

func oidcSealContext(id string) []byte { return []byte(contextPrefix + oidcSealPart + id) }

func (s *Store) envOidc() Oidc {
	out := Oidc{Source: SourceEnv, Providers: make([]OidcProvider, 0, len(s.envOIDC))}
	for _, c := range s.envOIDC {
		out.Providers = append(out.Providers, OidcProvider{Config: c, SecretSet: c.ClientSecret != ""})
	}
	return out
}

func (s *Store) emptyOidc() Oidc {
	if len(s.envOIDC) > 0 {
		return s.envOidc()
	}
	return Oidc{Source: SourceDatabase, Providers: []OidcProvider{}}
}

func (s *Store) readOidc(ctx context.Context, q *sqlc.Queries) (Oidc, error) {
	if len(s.envOIDC) > 0 {
		return s.envOidc(), nil
	}
	rows, err := q.ListOidcProviders(ctx)
	if err != nil {
		return Oidc{}, fmt.Errorf("read sign-in providers: %w", err)
	}
	out := Oidc{Source: SourceDatabase, Providers: make([]OidcProvider, 0, len(rows))}
	var errs []error
	for _, r := range rows {
		p, err := s.decodeProvider(ctx, r.ProviderId, storedProvider{
			Type: r.Type, Name: r.Name, Issuer: r.Issuer, ClientID: r.ClientId, Domains: r.AllowedDomains, Secret: r.Secret,
		}, false)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		created, updated := r.CreatedAt, r.UpdatedAt
		p.Version, p.CreatedAt, p.UpdatedAt, p.UpdatedBy = r.Version, &created, &updated, r.UpdatedBy
		out.Providers = append(out.Providers, p)
	}
	if len(rows) > settingspec.OIDCMaxProviders {
		errs = append(errs, fmt.Errorf("the database lists %d sign-in providers, more than %d", len(rows), settingspec.OIDCMaxProviders))
	}
	if first, second, found := settingspec.SharedIssuer(out.Active()); found {
		errs = append(errs, fmt.Errorf("sign-in providers %s and %s (from the database) share the issuer %s", first.ID, second.ID, second.Issuer))
	}
	if len(errs) > 0 {
		return Oidc{}, errors.Join(errs...)
	}
	return out, nil
}

type storedProvider struct {
	Type     string
	Name     string
	Issuer   string
	ClientID string
	Domains  []string
	Secret   []byte
}

func (s *Store) decodeProvider(ctx context.Context, id string, r storedProvider, tolerant bool) (OidcProvider, error) {
	cfg := settingspec.OIDCProvider{
		ID: id, Kind: r.Type, Name: r.Name, Issuer: r.Issuer, ClientID: r.ClientID, AllowedDomains: r.Domains,
	}
	sealed := false
	if len(r.Secret) > 0 {
		plain, err := s.oidcKeys.Open(ctx, keys.InstanceOwner(), oidcSealContext(id), r.Secret)
		switch {
		case err != nil && !tolerant:
			return OidcProvider{}, fmt.Errorf("sign-in provider %s (from the database): client secret: %w", id, ErrUndecryptable)
		case err != nil:
			sealed = true
		default:
			cfg.ClientSecret = string(plain)
			clear(plain)
		}
	}
	valid, errs := settingspec.ValidateOIDCProvider(cfg, s.dev, false)
	if len(errs) > 0 {
		return OidcProvider{}, fmt.Errorf("sign-in provider %s (from the database): %s %s", id, errs[0].Field, errs[0].Message)
	}
	return OidcProvider{Config: valid, SecretSet: valid.ClientSecret != "" || sealed, Sealed: sealed}, nil
}

func (s *Store) Watch(fn func(*Snapshot)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchers = append(s.watchers, fn)
}

func (s *Store) publish(snap *Snapshot) {
	s.current.Store(snap)
	for _, fn := range s.watchers {
		fn(snap)
	}
}
