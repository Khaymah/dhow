package sources

import (
	"context"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const (
	tableRepositories         = "Repositories"
	deployKeyMaxPlaintext     = 4096
	deployKeyMaxSealed        = 8192
	webhookSecretMaxPlaintext = 256
	webhookSecretMaxSealed    = 1024
)

func DeployKeySpec() keys.Spec {
	return keys.Spec{
		Table: tableRepositories, Column: "DeployKeySealed", Label: config.DeployKeySealLabel, Owner: keys.OwnerProject,
		MaxPlaintext: deployKeyMaxPlaintext, MaxSealed: deployKeyMaxSealed,
	}
}

func PreviousDeployKeySpec() keys.Spec {
	spec := DeployKeySpec()
	spec.Column = "PreviousDeployKeySealed"
	return spec
}

func WebhookSecretSpec() keys.Spec {
	return keys.Spec{
		Table: tableRepositories, Column: "WebhookSecretSealed", Label: config.WebhookDeliveryKeyLabel, Owner: keys.OwnerProject,
		MaxPlaintext: webhookSecretMaxPlaintext, MaxSealed: webhookSecretMaxSealed,
	}
}

func deployOwner(project, repo uuid.UUID) (keys.Owner, []byte) {
	return keys.ProjectOwner(project), sealContext(contextDeploy, repo)
}

func webhookOwner(project, repo uuid.UUID) (keys.Owner, []byte) {
	return keys.ProjectOwner(project), sealContext(contextWebhook, repo)
}

func ValueLocations() []keys.Location {
	return []keys.Location{
		keys.RowLocation[sqlc.ScanRepositoryDeployKeysRow]{
			Of: DeployKeySpec(),
			ScanRows: func(ctx context.Context, q *sqlc.Queries, after uuid.UUID, limit int32, legacy bool) ([]sqlc.ScanRepositoryDeployKeysRow, error) {
				return q.ScanRepositoryDeployKeys(ctx, sqlc.ScanRepositoryDeployKeysParams{After: after, LegacyOnly: legacy, Batch: limit})
			},
			Read: func(r sqlc.ScanRepositoryDeployKeysRow) (uuid.UUID, keys.Owner, []byte, []byte) {
				owner, rowContext := deployOwner(r.ProjectId, r.Id)
				return r.Id, owner, rowContext, r.Sealed
			},
			ReplaceRow: func(ctx context.Context, q *sqlc.Queries, id uuid.UUID, old, sealed []byte) (int64, error) {
				return q.ReplaceRepositoryDeployKeys(ctx, sqlc.ReplaceRepositoryDeployKeysParams{Sealed: sealed, ID: id, Old: old})
			},
			CountOld: func(ctx context.Context, q *sqlc.Queries) (int64, error) {
				return q.CountLegacyRepositoryDeployKeys(ctx)
			},
			CountByKeys: func(ctx context.Context, q *sqlc.Queries, ids [][]byte) (int64, error) {
				return q.CountRepositoryDeployKeysUnder(ctx, ids)
			},
		},
		keys.RowLocation[sqlc.ScanRepositoryPreviousDeployKeysRow]{
			Of: PreviousDeployKeySpec(),
			ScanRows: func(ctx context.Context, q *sqlc.Queries, after uuid.UUID, limit int32, legacy bool) ([]sqlc.ScanRepositoryPreviousDeployKeysRow, error) {
				return q.ScanRepositoryPreviousDeployKeys(ctx, sqlc.ScanRepositoryPreviousDeployKeysParams{After: after, LegacyOnly: legacy, Batch: limit})
			},
			Read: func(r sqlc.ScanRepositoryPreviousDeployKeysRow) (uuid.UUID, keys.Owner, []byte, []byte) {
				owner, rowContext := deployOwner(r.ProjectId, r.Id)
				return r.Id, owner, rowContext, r.Sealed
			},
			ReplaceRow: func(ctx context.Context, q *sqlc.Queries, id uuid.UUID, old, sealed []byte) (int64, error) {
				return q.ReplaceRepositoryPreviousDeployKeys(ctx, sqlc.ReplaceRepositoryPreviousDeployKeysParams{Sealed: sealed, ID: id, Old: old})
			},
			CountOld: func(ctx context.Context, q *sqlc.Queries) (int64, error) {
				return q.CountLegacyRepositoryPreviousDeployKeys(ctx)
			},
			CountByKeys: func(ctx context.Context, q *sqlc.Queries, ids [][]byte) (int64, error) {
				return q.CountRepositoryPreviousDeployKeysUnder(ctx, ids)
			},
		},
		keys.RowLocation[sqlc.ScanRepositoryWebhookSecretsRow]{
			Of: WebhookSecretSpec(),
			ScanRows: func(ctx context.Context, q *sqlc.Queries, after uuid.UUID, limit int32, legacy bool) ([]sqlc.ScanRepositoryWebhookSecretsRow, error) {
				return q.ScanRepositoryWebhookSecrets(ctx, sqlc.ScanRepositoryWebhookSecretsParams{After: after, LegacyOnly: legacy, Batch: limit})
			},
			Read: func(r sqlc.ScanRepositoryWebhookSecretsRow) (uuid.UUID, keys.Owner, []byte, []byte) {
				owner, rowContext := webhookOwner(r.ProjectId, r.Id)
				return r.Id, owner, rowContext, r.Sealed
			},
			ReplaceRow: func(ctx context.Context, q *sqlc.Queries, id uuid.UUID, old, sealed []byte) (int64, error) {
				return q.ReplaceRepositoryWebhookSecrets(ctx, sqlc.ReplaceRepositoryWebhookSecretsParams{Sealed: sealed, ID: id, Old: old})
			},
			CountOld: func(ctx context.Context, q *sqlc.Queries) (int64, error) {
				return q.CountLegacyRepositoryWebhookSecrets(ctx)
			},
			CountByKeys: func(ctx context.Context, q *sqlc.Queries, ids [][]byte) (int64, error) {
				return q.CountRepositoryWebhookSecretsUnder(ctx, ids)
			},
		},
	}
}
