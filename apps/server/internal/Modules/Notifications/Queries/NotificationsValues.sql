-- name: ScanNotificationConfigs :many
SELECT "Id", "ProjectId", "OrganizationId", "ConfigSealed" AS "Sealed"
FROM "NotificationChannels"
WHERE "ConfigSealed" IS NOT NULL AND "Id" > @after::uuid
ORDER BY "Id"
LIMIT @batch
FOR UPDATE SKIP LOCKED;

-- name: ReplaceNotificationConfigs :execrows
UPDATE "NotificationChannels" SET "ConfigSealed" = @sealed WHERE "Id" = @id AND "ConfigSealed" = @old;

-- name: CountNotificationConfigsUnder :one
SELECT count(*)::bigint FROM "NotificationChannels"
WHERE "ConfigSealed" IS NOT NULL AND get_byte("ConfigSealed", 0) = 3 AND substring("ConfigSealed" FROM 2 FOR 16) = ANY(@data_keys::bytea[]);
