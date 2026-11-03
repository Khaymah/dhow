package instancesettings

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/google/uuid"

	settingspec "github.com/Khaymah/dhow/apps/server/internal/Config/Settings"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

type OidcOperation string

const (
	OidcCreate OidcOperation = "create"
	OidcUpdate OidcOperation = "update"
	OidcRemove OidcOperation = "remove"

	fieldProviders = "Providers"
)

type OidcInput struct {
	ID             string
	Kind           string
	Name           string
	Issuer         string
	ClientID       string
	ClientSecret   string
	AllowedDomains []string
}

type OidcPatch struct {
	Kind           *string
	Name           *string
	Issuer         *string
	ClientID       *string
	ClientSecret   *string
	AllowedDomains *[]string
}

type OidcChange struct {
	Operation OidcOperation
	ID        string
	Version   int64
	Input     OidcInput
	Patch     OidcPatch
}

type oidcRow struct {
	rowID   uuid.UUID
	cfg     settingspec.OIDCProvider
	secret  []byte
	version int64
	sealed  bool
}

func (r oidcRow) enabled() bool { return r.cfg.ClientSecret != "" || r.sealed }

type oidcPlan struct {
	target  *oidcRow
	next    settingspec.OIDCProvider
	changed []string
}

type oidcPlanner struct {
	store  *Store
	q      *sqlc.Queries
	rows   []oidcRow
	locked bool
}

func invalidOidc(errs []settingspec.OIDCError) error {
	out := make([]apperrors.IErrorDetail, 0, len(errs))
	for _, e := range errs {
		out = append(out, detail(e.Field, invalidPrefix+e.Rule))
	}
	return apperrors.SettingInvalid(out...)
}

func (p *oidcPlanner) find(id string) *oidcRow {
	for i := range p.rows {
		if p.rows[i].cfg.ID == id {
			return &p.rows[i]
		}
	}
	return nil
}

func (p *oidcPlanner) others(id string) []settingspec.OIDCProvider {
	out := []settingspec.OIDCProvider{}
	for _, r := range p.rows {
		if r.cfg.ID != id {
			out = append(out, r.cfg)
		}
	}
	return out
}

func (p *oidcPlanner) enabledOthers(id string) []string {
	out := []string{}
	for _, r := range p.rows {
		if r.cfg.ID != id && r.enabled() {
			out = append(out, r.cfg.ID)
		}
	}
	return out
}

func (p *oidcPlanner) plan(ctx context.Context, c OidcChange) (oidcPlan, error) {
	if len(p.store.envOIDC) > 0 {
		return oidcPlan{}, apperrors.SettingLockedByEnv(detail(fieldProviders, apperrors.KeySettingLockedByEnv))
	}
	target := p.find(c.ID)
	if err := p.unreadable(c, target); err != nil {
		return oidcPlan{}, err
	}
	if c.Operation == OidcCreate {
		return p.planCreate(ctx, c.Input)
	}
	if target == nil {
		return oidcPlan{}, apperrors.NotFound()
	}
	if target.version != c.Version {
		return oidcPlan{}, apperrors.SettingChanged()
	}
	if c.Operation == OidcRemove {
		if target.enabled() {
			if err := p.strands(ctx, c.ID, nil); err != nil {
				return oidcPlan{}, err
			}
		}
		return oidcPlan{target: target, changed: []string{string(OidcRemove)}}, nil
	}
	return p.planUpdate(ctx, target, c.Patch)
}

func (p *oidcPlanner) unreadable(c OidcChange, target *oidcRow) error {
	for _, r := range p.rows {
		if !r.sealed {
			continue
		}
		fixes := target != nil && target.sealed && c.Operation != OidcCreate &&
			(c.Operation == OidcRemove || c.Patch.ClientSecret != nil)
		if !fixes {
			return fmt.Errorf("sign-in provider %s (from the database): client secret: %w; remove it or clear its secret first", r.cfg.ID, ErrUndecryptable)
		}
	}
	return nil
}

func (p *oidcPlanner) planCreate(ctx context.Context, in OidcInput) (oidcPlan, error) {
	next, errs := settingspec.ValidateOIDCProvider(settingspec.OIDCProvider{
		ID: in.ID, Kind: in.Kind, Name: in.Name, Issuer: in.Issuer, ClientID: in.ClientID, ClientSecret: in.ClientSecret,
		AllowedDomains: in.AllowedDomains,
	}, p.store.dev, true)
	if len(errs) > 0 {
		return oidcPlan{}, invalidOidc(errs)
	}
	if p.find(next.ID) != nil {
		return oidcPlan{}, apperrors.OidcProviderExists()
	}
	if len(p.rows) >= settingspec.OIDCMaxProviders {
		return oidcPlan{}, apperrors.OidcProviderLimit()
	}
	if err := p.issuerFree(next); err != nil {
		return oidcPlan{}, err
	}
	links, err := p.q.ProviderIdentityIssuers(ctx, sqlc.ProviderIdentityIssuersParams{Provider: next.ID, Issuer: next.Issuer})
	if err != nil {
		return oidcPlan{}, err
	}
	if links.OtherIssuer {
		return oidcPlan{}, apperrors.OidcProviderLinked(settingspec.OIDCFieldID)
	}
	return oidcPlan{next: next, changed: changedProviderFields(settingspec.OIDCProvider{}, next)}, nil
}

func (p *oidcPlanner) issuerFree(next settingspec.OIDCProvider) error {
	if _, _, found := settingspec.SharedIssuer(append(p.others(next.ID), next)); found {
		return apperrors.SettingInvalid(detail(settingspec.OIDCFieldIssuer, invalidPrefix+settingspec.RuleOIDCIssuerTaken))
	}
	return nil
}

func patched(old settingspec.OIDCProvider, patch OidcPatch) settingspec.OIDCProvider {
	next := old
	next.AllowedDomains = slices.Clone(old.AllowedDomains)
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	set(&next.Kind, patch.Kind)
	set(&next.Name, patch.Name)
	set(&next.Issuer, patch.Issuer)
	set(&next.ClientID, patch.ClientID)
	set(&next.ClientSecret, patch.ClientSecret)
	if patch.AllowedDomains != nil {
		next.AllowedDomains = *patch.AllowedDomains
	}
	return next
}

func (p *oidcPlanner) planUpdate(ctx context.Context, target *oidcRow, patch OidcPatch) (oidcPlan, error) {
	next, errs := settingspec.ValidateOIDCProvider(patched(target.cfg, patch), p.store.dev, false)
	if len(errs) > 0 {
		return oidcPlan{}, invalidOidc(errs)
	}
	var identity []string
	if next.Kind != target.cfg.Kind {
		identity = append(identity, settingspec.OIDCFieldType)
	}
	if next.Issuer != target.cfg.Issuer {
		identity = append(identity, settingspec.OIDCFieldIssuer)
	}
	if len(identity) > 0 {
		links, err := p.q.ProviderIdentityIssuers(ctx, sqlc.ProviderIdentityIssuersParams{Provider: next.ID, Issuer: target.cfg.Issuer})
		if err != nil {
			return oidcPlan{}, err
		}
		if links.Linked {
			return oidcPlan{}, apperrors.OidcProviderLinked(identity...)
		}
	}
	if len(identity) > 0 && target.cfg.ClientSecret != "" && patch.ClientSecret == nil {
		return oidcPlan{}, apperrors.SettingInvalid(detail(settingspec.OIDCFieldClientSecret, invalidPrefix+settingspec.RuleOIDCSecretAgain))
	}
	if err := p.issuerFree(next); err != nil {
		return oidcPlan{}, err
	}
	if target.enabled() && next.ClientSecret == "" {
		if err := p.strands(ctx, next.ID, nil); err != nil {
			return oidcPlan{}, err
		}
	}
	if next.ClientSecret != "" && len(next.AllowedDomains) > 0 && !slices.Equal(next.AllowedDomains, target.cfg.AllowedDomains) {
		if err := p.strands(ctx, next.ID, next.AllowedDomains); err != nil {
			return oidcPlan{}, err
		}
	}
	changed := changedProviderFields(target.cfg, next)
	if target.sealed && !slices.Contains(changed, settingspec.OIDCFieldClientSecret) {
		changed = append(changed, settingspec.OIDCFieldClientSecret)
		sort.Strings(changed)
	}
	return oidcPlan{target: target, next: next, changed: changed}, nil
}

func (p *oidcPlanner) strands(ctx context.Context, id string, domains []string) error {
	if p.locked {
		if _, err := p.q.LockUsersOfProvider(ctx, id); err != nil {
			return err
		}
	}
	var n int64
	var err error
	if domains == nil {
		n, err = p.q.CountUsersOnlyOnProvider(ctx, sqlc.CountUsersOnlyOnProviderParams{Provider: id, Others: p.enabledOthers(id)})
	} else {
		n, err = p.q.CountUsersOutsideDomains(ctx, sqlc.CountUsersOutsideDomainsParams{Provider: id, Domains: domains, Others: p.enabledOthers(id)})
	}
	if err != nil {
		return err
	}
	if n > 0 {
		return apperrors.OidcProviderOnlySignIn(n)
	}
	return nil
}

func changedProviderFields(before, after settingspec.OIDCProvider) []string {
	var out []string
	add := func(field string, differs bool) {
		if differs {
			out = append(out, field)
		}
	}
	add(settingspec.OIDCFieldType, before.Kind != after.Kind)
	add(settingspec.OIDCFieldName, before.Name != after.Name)
	add(settingspec.OIDCFieldIssuer, before.Issuer != after.Issuer)
	add(settingspec.OIDCFieldClientID, before.ClientID != after.ClientID)
	add(settingspec.OIDCFieldClientSecret, before.ClientSecret != after.ClientSecret)
	add(settingspec.OIDCFieldAllowedDomains, !slices.Equal(before.AllowedDomains, after.AllowedDomains))
	sort.Strings(out)
	return out
}

func (s *Store) oidcRows(ctx context.Context, q *sqlc.Queries, lock bool) ([]oidcRow, error) {
	if len(s.envOIDC) > 0 {
		return nil, nil
	}
	var rows []sqlc.ListOidcProvidersRow
	var err error
	if lock {
		var locked []sqlc.ListOidcProvidersForUpdateRow
		locked, err = q.ListOidcProvidersForUpdate(ctx)
		for _, r := range locked {
			rows = append(rows, sqlc.ListOidcProvidersRow(r))
		}
	} else {
		rows, err = q.ListOidcProviders(ctx)
	}
	if err != nil {
		return nil, err
	}
	out := make([]oidcRow, 0, len(rows))
	for _, r := range rows {
		p, err := s.decodeProvider(ctx, r.ProviderId, storedProvider{
			Type: r.Type, Name: r.Name, Issuer: r.Issuer, ClientID: r.ClientId, Domains: r.AllowedDomains, Secret: r.Secret,
		}, true)
		if err != nil {
			return nil, err
		}
		out = append(out, oidcRow{rowID: r.Id, cfg: p.Config, secret: r.Secret, version: r.Version, sealed: p.Sealed})
	}
	return out, nil
}

func (s *Store) CheckOidc(ctx context.Context, c OidcChange) error {
	if s.db == nil {
		return apperrors.ServiceUnavailable()
	}
	q := sqlc.New(s.db)
	rows, err := s.oidcRows(ctx, q, false)
	if err != nil {
		return err
	}
	_, err = (&oidcPlanner{store: s, q: q, rows: rows}).plan(ctx, c)
	return err
}
