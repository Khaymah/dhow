package secrets

import (
	"context"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
)

func SecretValueSpec() keys.Spec {
	return keys.Spec{
		Table: "SecretVersions", Column: "ValueSealed", Label: config.SecretSealLabel, Owner: keys.OwnerOrganization,
		AlsoOwners: []keys.OwnerKind{keys.OwnerProject, keys.OwnerEnvironment}, MaxPlaintext: config.SecretMaxBytes, MaxSealed: config.SecretSealedMax,
	}
}

func RegistryPasswordSpec() keys.Spec {
	return keys.Spec{
		Table: "RegistryCredentials", Column: "PasswordSealed", Label: config.RegistryCredentialLabel, Owner: keys.OwnerOrganization,
		AlsoOwners: []keys.OwnerKind{keys.OwnerProject}, MaxPlaintext: config.RegistryPasswordMax, MaxSealed: config.RegistryPasswordSealedMax,
	}
}

func ValueLocations() []keys.Location {
	return []keys.Location{
		keys.RowLocation[sqlc.ScanSecretValuesRow]{
			Of: SecretValueSpec(),
			ScanRows: func(ctx context.Context, q *sqlc.Queries, after uuid.UUID, limit int32, legacy bool) ([]sqlc.ScanSecretValuesRow, error) {
				return q.ScanSecretValues(ctx, sqlc.ScanSecretValuesParams{After: after, LegacyOnly: legacy, Batch: limit})
			},
			Read: func(r sqlc.ScanSecretValuesRow) (uuid.UUID, keys.Owner, []byte, []byte) {
				return r.Id, ownerOf(r.Scope, r.OrganizationId, r.ProjectId, r.EnvironmentId), sealContext(r.SecretId, r.Version), r.Sealed
			},
			ReplaceRow: func(ctx context.Context, q *sqlc.Queries, id uuid.UUID, old, sealed []byte) (int64, error) {
				return q.ReplaceSecretValue(ctx, sqlc.ReplaceSecretValueParams{Sealed: sealed, ID: id, Old: old})
			},
			CountOld: func(ctx context.Context, q *sqlc.Queries) (int64, error) { return q.CountLegacySecretValues(ctx) },
			CountByKeys: func(ctx context.Context, q *sqlc.Queries, ids [][]byte) (int64, error) {
				return q.CountSecretValuesUnder(ctx, ids)
			},
		},
		keys.RowLocation[sqlc.ScanRegistryPasswordsRow]{
			Of: RegistryPasswordSpec(),
			ScanRows: func(ctx context.Context, q *sqlc.Queries, after uuid.UUID, limit int32, legacy bool) ([]sqlc.ScanRegistryPasswordsRow, error) {
				return q.ScanRegistryPasswords(ctx, sqlc.ScanRegistryPasswordsParams{After: after, LegacyOnly: legacy, Batch: limit})
			},
			Read: func(r sqlc.ScanRegistryPasswordsRow) (uuid.UUID, keys.Owner, []byte, []byte) {
				scope := models.ScopeOrganization
				if r.ProjectId != nil {
					scope = models.ScopeProject
				}
				return r.Id, ownerOf(scope, r.OrganizationId, r.ProjectId, nil), registryContext(r.Id), r.Sealed
			},
			ReplaceRow: func(ctx context.Context, q *sqlc.Queries, id uuid.UUID, old, sealed []byte) (int64, error) {
				return q.ReplaceRegistryPassword(ctx, sqlc.ReplaceRegistryPasswordParams{Sealed: sealed, ID: id, Old: old})
			},
			CountOld: func(ctx context.Context, q *sqlc.Queries) (int64, error) { return q.CountLegacyRegistryPasswords(ctx) },
			CountByKeys: func(ctx context.Context, q *sqlc.Queries, ids [][]byte) (int64, error) {
				return q.CountRegistryPasswordsUnder(ctx, ids)
			},
		},
	}
}
