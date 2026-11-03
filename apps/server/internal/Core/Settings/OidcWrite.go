package instancesettings

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

type OidcResult struct {
	Snapshot *Snapshot
	ID       string
	Changed  []string
	Version  int64
}

func (s *Store) sealSecret(ctx context.Context, q *sqlc.Queries, id, secret string) ([]byte, error) {
	if secret == "" {
		return nil, nil
	}
	return s.oidcKeys.SealIn(ctx, q, keys.InstanceOwner(), oidcSealContext(id), []byte(secret))
}

func (s *Store) ApplyOidc(ctx context.Context, c OidcChange, actor uuid.UUID) (OidcResult, error) {
	if s.db == nil {
		return OidcResult{}, apperrors.ServiceUnavailable()
	}
	res := OidcResult{ID: c.ID}
	err := dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		if err := q.LockOidcProviderList(ctx, config.OidcProvidersLockKey); err != nil {
			return err
		}
		rows, err := s.oidcRows(ctx, q, true)
		if err != nil {
			return err
		}
		p, err := (&oidcPlanner{store: s, q: q, rows: rows, locked: true}).plan(ctx, c)
		if err != nil {
			return err
		}
		res.Changed = p.changed
		if len(p.changed) == 0 {
			res.Version = p.target.version
			return nil
		}
		if res.Version, err = s.writeOidc(ctx, q, c.Operation, p, actor); err != nil {
			return err
		}
		if c.Operation == OidcCreate {
			res.ID = p.next.ID
		}
		return q.NotifySettingsChanged(ctx, GroupOidc)
	})
	if err != nil {
		return OidcResult{}, err
	}
	if len(res.Changed) > 0 {
		if err := s.Load(ctx); err != nil {
			return OidcResult{}, err
		}
	}
	res.Snapshot = s.Current()
	return res, nil
}

func (s *Store) writeOidc(ctx context.Context, q *sqlc.Queries, op OidcOperation, p oidcPlan, actor uuid.UUID) (int64, error) {
	var version int64
	var err error
	switch op {
	case OidcRemove:
		version, err = q.RemoveOidcProvider(ctx, sqlc.RemoveOidcProviderParams{Actor: &actor, ID: p.target.rowID, Version: p.target.version})
	case OidcCreate:
		var secret []byte
		if secret, err = s.sealSecret(ctx, q, p.next.ID, p.next.ClientSecret); err != nil {
			return 0, err
		}
		version, err = q.InsertOidcProvider(ctx, sqlc.InsertOidcProviderParams{
			ProviderID: p.next.ID, Type: p.next.Kind, Name: p.next.Name, Issuer: p.next.Issuer, ClientID: p.next.ClientID,
			AllowedDomains: p.next.AllowedDomains, Secret: secret, Actor: &actor,
		})
	default:
		secret := p.target.secret
		if p.target.sealed || p.next.ClientSecret != p.target.cfg.ClientSecret {
			if secret, err = s.sealSecret(ctx, q, p.next.ID, p.next.ClientSecret); err != nil {
				return 0, err
			}
		}
		version, err = q.UpdateOidcProvider(ctx, sqlc.UpdateOidcProviderParams{
			Type: p.next.Kind, Name: p.next.Name, Issuer: p.next.Issuer, ClientID: p.next.ClientID, AllowedDomains: p.next.AllowedDomains,
			Secret: secret, Actor: &actor, ID: p.target.rowID, Version: p.target.version,
		})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if op == OidcCreate {
			return 0, apperrors.OidcProviderExists()
		}
		return 0, apperrors.SettingChanged()
	}
	return version, err
}
