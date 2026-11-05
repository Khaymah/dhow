-- name: LockKeyring :exec
SELECT pg_advisory_xact_lock(hashtextextended(CASE current_schema() WHEN 'public' THEN 'dhow.keyring' ELSE 'dhow.keyring:' || current_schema() END, 0));

-- name: TryLockKeyRewrap :one
SELECT pg_try_advisory_xact_lock(hashtextextended(CASE current_schema() WHEN 'public' THEN 'dhow.keyring-rewrap' ELSE 'dhow.keyring-rewrap:' || current_schema() END, 0))::boolean;

-- name: ListKeyEncryptionKeys :many
SELECT "Version", "CheckValue", "Provider", "State", "ActivatedAt", "RetiredAt", "LastExportedAt"
FROM "KeyEncryptionKeys"
ORDER BY "Version";

-- name: InsertKeyEncryptionKey :exec
INSERT INTO "KeyEncryptionKeys" ("Version", "CheckValue", "Provider", "State", "ActivatedAt")
VALUES (@version, @check_value, @provider, @state, CASE WHEN @state::text = 'pending' THEN NULL ELSE now() END);

-- name: GetActiveKeyEncryptionKey :one
SELECT "Version" FROM "KeyEncryptionKeys" WHERE "State" = 'active';

-- name: DemoteActiveKeyEncryptionKey :execrows
UPDATE "KeyEncryptionKeys"
SET "State" = 'retiring', "UpdatedAt" = now()
WHERE "State" = 'active';

-- name: PromoteKeyEncryptionKey :execrows
UPDATE "KeyEncryptionKeys"
SET "State" = 'active', "ActivatedAt" = now(), "RetiredAt" = NULL, "UpdatedAt" = now()
WHERE "Version" = @version AND "State" IN ('pending', 'retiring');

-- name: RetireDrainedKeyEncryptionKeys :many
UPDATE "KeyEncryptionKeys" AS k
SET "State" = 'retired', "RetiredAt" = now(), "UpdatedAt" = now()
WHERE k."State" = 'retiring'
  AND NOT EXISTS (SELECT 1 FROM "DataKeys" AS d WHERE d."KekVersion" = k."Version" AND d."State" <> 'destroyed')
RETURNING k."Version";

-- name: RetireKeyEncryptionKey :execrows
UPDATE "KeyEncryptionKeys"
SET "State" = 'retired', "RetiredAt" = now(), "UpdatedAt" = now()
WHERE "Version" = @version AND "State" <> 'retired';

-- name: ReplaceKeyEncryptionKeyCheck :execrows
UPDATE "KeyEncryptionKeys"
SET "CheckValue" = @check_value, "State" = 'pending', "ActivatedAt" = NULL, "RetiredAt" = NULL, "UpdatedAt" = now()
WHERE "Version" = @version;

-- name: MarkKeyringExported :execrows
UPDATE "KeyEncryptionKeys"
SET "LastExportedAt" = now(), "UpdatedAt" = now()
WHERE "Version" = ANY(@versions::integer[]);

-- name: GetDataKey :one
SELECT "Id", "OwnerKind", "OwnerId", "KekVersion", "Wrapped", "State"
FROM "DataKeys"
WHERE "Id" = @id;

-- name: GetActiveDataKeyForOwner :one
SELECT "Id", "OwnerKind", "OwnerId", "KekVersion", "Wrapped", "State"
FROM "DataKeys"
WHERE "OwnerKind" = @owner_kind AND "OwnerId" IS NOT DISTINCT FROM sqlc.narg('owner_id')::uuid AND "State" = 'active';

-- name: InsertDataKey :execrows
INSERT INTO "DataKeys" ("Id", "OwnerKind", "OwnerId", "KekVersion", "Wrapped")
SELECT @id, @owner_kind, sqlc.narg('owner_id')::uuid, k."Version", @wrapped
FROM "KeyEncryptionKeys" AS k
WHERE k."Version" = @kek_version AND k."State" = 'active'
FOR SHARE OF k
ON CONFLICT DO NOTHING;

-- name: DestroyDataKeysOfOwner :execrows
UPDATE "DataKeys"
SET "State" = 'destroyed', "Wrapped" = NULL, "DestroyedAt" = now(), "UpdatedAt" = now()
WHERE "OwnerKind" = @owner_kind AND "OwnerId" IS NOT DISTINCT FROM sqlc.narg('owner_id')::uuid AND "State" <> 'destroyed';

-- name: DestroyDataKeysOnVersions :execrows
UPDATE "DataKeys"
SET "State" = 'destroyed', "Wrapped" = NULL, "DestroyedAt" = now(), "UpdatedAt" = now()
WHERE "KekVersion" = ANY(@versions::integer[]) AND "State" <> 'destroyed';

-- name: ListDataKeysToRewrap :many
SELECT "Id", "OwnerKind", "OwnerId", "KekVersion", "Wrapped"
FROM "DataKeys"
WHERE "State" IN ('active', 'superseded') AND "KekVersion" <> @active_version AND "UnwrapFailedAt" IS NULL
ORDER BY "Id"
LIMIT @batch
FOR UPDATE SKIP LOCKED;

-- name: RewrapDataKey :execrows
UPDATE "DataKeys"
SET "KekVersion" = @kek_version, "Wrapped" = @wrapped, "UpdatedAt" = now()
WHERE "Id" = @id AND "State" IN ('active', 'superseded') AND "KekVersion" = @from_version;

-- name: CountDataKeysByVersion :many
SELECT "KekVersion", count(*)::bigint AS "Count", (count(*) FILTER (WHERE "UnwrapFailedAt" IS NOT NULL))::bigint AS "Unreadable"
FROM "DataKeys"
WHERE "State" <> 'destroyed'
GROUP BY "KekVersion"
ORDER BY "KekVersion";

-- name: CountDataKeysOnVersions :one
SELECT count(*)::bigint FROM "DataKeys" WHERE "State" <> 'destroyed' AND "KekVersion" = ANY(@versions::integer[]);

-- name: ListLiveServerInstanceKeks :many
SELECT "InstanceKey", "Hostname", "KekVersions"
FROM "ServerInstances"
WHERE NOT "IsDeleted" AND "LastSeenAt" >= now() - make_interval(secs => @stale_after_seconds::double precision)
ORDER BY "InstanceKey";

-- name: MarkDataKeyUnreadable :execrows
UPDATE "DataKeys"
SET "UnwrapFailedAt" = now(), "UpdatedAt" = now()
WHERE "Id" = @id AND "State" <> 'destroyed' AND "UnwrapFailedAt" IS NULL;

-- name: ClearDataKeyUnreadable :execrows
UPDATE "DataKeys"
SET "UnwrapFailedAt" = NULL, "UpdatedAt" = now()
WHERE "UnwrapFailedAt" IS NOT NULL AND "State" <> 'destroyed';

-- name: LockActiveDataKeyForOwner :one
SELECT "Id", "OwnerKind", "OwnerId", "KekVersion", "Wrapped", "State"
FROM "DataKeys"
WHERE "OwnerKind" = @owner_kind AND "OwnerId" IS NOT DISTINCT FROM sqlc.narg('owner_id')::uuid AND "State" = 'active'
FOR SHARE;

-- name: GetOwnerDataKeyStates :one
SELECT (count(*) FILTER (WHERE "State" = 'destroyed'))::bigint AS "Destroyed",
       (count(*) FILTER (WHERE "State" <> 'destroyed'))::bigint AS "Live"
FROM "DataKeys"
WHERE "OwnerKind" = @owner_kind AND "OwnerId" IS NOT DISTINCT FROM sqlc.narg('owner_id')::uuid;
