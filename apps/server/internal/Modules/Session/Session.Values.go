package session

import (
	"context"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const totpSecretMaxSealed = 128

func TotpSecretSpec() keys.Spec {
	return keys.Spec{
		Table: "TotpFactors", Column: "SecretSealed", Label: config.TotpSealLabel, Owner: keys.OwnerUser,
		MaxPlaintext: auth.TotpSecretBytes, MaxSealed: totpSecretMaxSealed,
	}
}

func ValueLocations() []keys.Location {
	return []keys.Location{keys.RowLocation[sqlc.ScanTotpSecretsRow]{
		Of: TotpSecretSpec(),
		ScanRows: func(ctx context.Context, q *sqlc.Queries, after uuid.UUID, limit int32) ([]sqlc.ScanTotpSecretsRow, error) {
			return q.ScanTotpSecrets(ctx, sqlc.ScanTotpSecretsParams{After: after, Batch: limit})
		},
		Read: func(r sqlc.ScanTotpSecretsRow) (uuid.UUID, keys.Owner, []byte, []byte) {
			return r.Id, keys.UserOwner(r.UserId), totpContext(r.UserId), r.Sealed
		},
		ReplaceRow: func(ctx context.Context, q *sqlc.Queries, id uuid.UUID, old, sealed []byte) (int64, error) {
			return q.ReplaceTotpSecrets(ctx, sqlc.ReplaceTotpSecretsParams{Sealed: sealed, ID: id, Old: old})
		},
		CountByKeys: func(ctx context.Context, q *sqlc.Queries, ids [][]byte) (int64, error) {
			return q.CountTotpSecretsUnder(ctx, ids)
		},
	}}
}
