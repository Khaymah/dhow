package instancesettings

import (
	"context"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	settingspec "github.com/Khaymah/dhow/apps/server/internal/Config/Settings"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const (
	secretsMaxPlaintext = 16384
	secretsMaxSealed    = 65536
	oidcMaxSealed       = 4096
)

func SecretsSpec() keys.Spec {
	return keys.Spec{
		Table: "InstanceSettings", Column: "Secrets", Label: config.InstanceSettingsSealLabel, Owner: keys.OwnerInstance,
		MaxPlaintext: secretsMaxPlaintext, MaxSealed: secretsMaxSealed,
	}
}

func OidcSecretSpec() keys.Spec {
	return keys.Spec{
		Table: "OidcProviders", Column: "Secret", Label: config.InstanceSettingsSealLabel, Owner: keys.OwnerInstance,
		MaxPlaintext: settingspec.OIDCMaxSecretBytes, MaxSealed: oidcMaxSealed,
	}
}

func groupContext(group string) []byte { return []byte(contextPrefix + group) }

func ValueLocations() []keys.Location {
	return []keys.Location{
		keys.RowLocation[sqlc.ScanInstanceSettingSecretsRow]{
			Of: SecretsSpec(),
			ScanRows: func(ctx context.Context, q *sqlc.Queries, after uuid.UUID, limit int32) ([]sqlc.ScanInstanceSettingSecretsRow, error) {
				return q.ScanInstanceSettingSecrets(ctx, sqlc.ScanInstanceSettingSecretsParams{After: after, Batch: limit})
			},
			Read: func(r sqlc.ScanInstanceSettingSecretsRow) (uuid.UUID, keys.Owner, []byte, []byte) {
				return r.Id, keys.InstanceOwner(), groupContext(r.Group), r.Sealed
			},
			ReplaceRow: func(ctx context.Context, q *sqlc.Queries, id uuid.UUID, old, sealed []byte) (int64, error) {
				return q.ReplaceInstanceSettingSecrets(ctx, sqlc.ReplaceInstanceSettingSecretsParams{Sealed: sealed, ID: id, Old: old})
			},
			CountByKeys: func(ctx context.Context, q *sqlc.Queries, ids [][]byte) (int64, error) {
				return q.CountInstanceSettingSecretsUnder(ctx, ids)
			},
		},
		keys.RowLocation[sqlc.ScanOidcProviderSecretsRow]{
			Of: OidcSecretSpec(),
			ScanRows: func(ctx context.Context, q *sqlc.Queries, after uuid.UUID, limit int32) ([]sqlc.ScanOidcProviderSecretsRow, error) {
				return q.ScanOidcProviderSecrets(ctx, sqlc.ScanOidcProviderSecretsParams{After: after, Batch: limit})
			},
			Read: func(r sqlc.ScanOidcProviderSecretsRow) (uuid.UUID, keys.Owner, []byte, []byte) {
				return r.Id, keys.InstanceOwner(), oidcSealContext(r.ProviderId), r.Sealed
			},
			ReplaceRow: func(ctx context.Context, q *sqlc.Queries, id uuid.UUID, old, sealed []byte) (int64, error) {
				return q.ReplaceOidcProviderSecrets(ctx, sqlc.ReplaceOidcProviderSecretsParams{Sealed: sealed, ID: id, Old: old})
			},
			CountByKeys: func(ctx context.Context, q *sqlc.Queries, ids [][]byte) (int64, error) {
				return q.CountOidcProviderSecretsUnder(ctx, ids)
			},
		},
	}
}
