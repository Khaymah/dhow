-- name: TryLockKeyRekey :one
SELECT pg_try_advisory_lock(hashtextextended('dhow.keyring-rekey:' || current_schema(), 0))::boolean;

-- name: UnlockKeyRekey :one
SELECT pg_advisory_unlock(hashtextextended('dhow.keyring-rekey:' || current_schema(), 0))::boolean;

-- name: GetOpenKeyRekey :one
SELECT "Id", "Cutoff", "Superseded", "Resealed", "Destroyed"
FROM "KeyRekeys"
WHERE "DoneAt" IS NULL AND NOT "IsDeleted";

-- name: InsertKeyRekey :one
INSERT INTO "KeyRekeys" ("Cutoff") VALUES (now())
RETURNING "Id", "Cutoff", "Superseded", "Resealed", "Destroyed";

-- name: AddKeyRekeyProgress :exec
UPDATE "KeyRekeys"
SET "Superseded" = "Superseded" + @superseded, "Resealed" = "Resealed" + @resealed, "Destroyed" = "Destroyed" + @destroyed,
    "UpdatedAt" = now()
WHERE "Id" = @id;

-- name: FinishKeyRekey :exec
UPDATE "KeyRekeys" SET "DoneAt" = now(), "UpdatedAt" = now() WHERE "Id" = @id;

-- name: GetLastKeyRekey :one
SELECT "Id", "Cutoff", "Superseded", "Resealed", "Destroyed", "DoneAt"
FROM "KeyRekeys"
WHERE NOT "IsDeleted"
ORDER BY "Cutoff" DESC
LIMIT 1;

-- name: ListDataKeysToSupersede :many
SELECT "Id", "OwnerKind", "OwnerId"
FROM "DataKeys"
WHERE "State" = 'active' AND "CreatedAt" < @cutoff
ORDER BY "Id"
LIMIT @batch;

-- name: SupersedeDataKey :execrows
UPDATE "DataKeys"
SET "State" = 'superseded', "SupersededAt" = now(), "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = 'active';

-- name: ListSupersededDataKeyIds :many
SELECT "Id" FROM "DataKeys" WHERE "State" = 'superseded' ORDER BY "Id";

-- name: DestroySupersededDataKeys :execrows
UPDATE "DataKeys"
SET "State" = 'destroyed', "Wrapped" = NULL, "DestroyedAt" = now(), "UpdatedAt" = now()
WHERE "State" = 'superseded' AND "Id" = ANY(@ids::uuid[]);

-- name: LockSupersededDataKeys :many
SELECT "Id" FROM "DataKeys" WHERE "State" = 'superseded' ORDER BY "Id" FOR UPDATE;

-- name: CountDataKeysToSupersede :one
SELECT count(*)::bigint FROM "DataKeys" WHERE "State" = 'active' AND "CreatedAt" < @cutoff;

-- name: GetActiveKeyEncryptionKeyActivatedAt :one
SELECT "Version", "ActivatedAt" FROM "KeyEncryptionKeys" WHERE "State" = 'active';
