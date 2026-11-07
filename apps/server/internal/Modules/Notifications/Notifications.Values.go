package notifications

import (
	"context"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const (
	tableChannels      = "NotificationChannels"
	configMaxPlaintext = 4096
	configMaxSealed    = 8192
)

func ConfigSpec() keys.Spec {
	return keys.Spec{
		Table: tableChannels, Column: "ConfigSealed", Label: config.NotifyConfigSealLabel, Owner: keys.OwnerProject,
		AlsoOwners: []keys.OwnerKind{keys.OwnerOrganization}, MaxPlaintext: configMaxPlaintext, MaxSealed: configMaxSealed,
	}
}

func ownerOf(org uuid.UUID, project *uuid.UUID) keys.Owner {
	if project != nil {
		return keys.ProjectOwner(*project)
	}
	return keys.OrganizationOwner(org)
}

func ChannelSealContext(channel uuid.UUID) []byte {
	return []byte(config.NotifyConfigSealLabel + channel.String())
}

func ValueLocations() []keys.Location {
	return []keys.Location{
		keys.RowLocation[sqlc.ScanNotificationConfigsRow]{
			Of: ConfigSpec(),
			ScanRows: func(ctx context.Context, q *sqlc.Queries, after uuid.UUID, limit int32) ([]sqlc.ScanNotificationConfigsRow, error) {
				return q.ScanNotificationConfigs(ctx, sqlc.ScanNotificationConfigsParams{After: after, Batch: limit})
			},
			Read: func(r sqlc.ScanNotificationConfigsRow) (uuid.UUID, keys.Owner, []byte, []byte) {
				return r.Id, ownerOf(r.OrganizationId, r.ProjectId), ChannelSealContext(r.Id), r.Sealed
			},
			ReplaceRow: func(ctx context.Context, q *sqlc.Queries, id uuid.UUID, old, sealed []byte) (int64, error) {
				return q.ReplaceNotificationConfigs(ctx, sqlc.ReplaceNotificationConfigsParams{Sealed: sealed, ID: id, Old: old})
			},
			CountByKeys: func(ctx context.Context, q *sqlc.Queries, ids [][]byte) (int64, error) {
				return q.CountNotificationConfigsUnder(ctx, ids)
			},
		},
	}
}
