-- name: TryLockBackfill :one
SELECT pg_try_advisory_xact_lock(hashtextextended('dhow.backfill:' || @name::text, 0))::boolean;

-- name: EnsureBackfill :exec
INSERT INTO "Backfills" ("Name") VALUES (@name) ON CONFLICT DO NOTHING;

-- name: LockBackfillRow :one
SELECT "LastKey", "Processed", "Failed", "Passes", "DoneAt"
FROM "Backfills"
WHERE "Name" = @name AND NOT "IsDeleted"
FOR UPDATE;

-- name: AdvanceBackfill :exec
UPDATE "Backfills"
SET "LastKey" = @last_key, "Processed" = "Processed" + @processed, "Failed" = "Failed" + @failed, "UpdatedAt" = now()
WHERE "Name" = @name;

-- name: EndBackfillPass :exec
UPDATE "Backfills"
SET "LastKey" = '', "Passes" = "Passes" + 1, "Remaining" = @remaining::bigint,
    "DoneAt" = CASE WHEN @remaining::bigint = 0 THEN now() ELSE NULL END, "UpdatedAt" = now()
WHERE "Name" = @name;

-- name: ListBackfills :many
SELECT "Name", "LastKey", "Processed", "Failed", "Remaining", "Passes", "CreatedAt", "UpdatedAt", "DoneAt"
FROM "Backfills"
WHERE NOT "IsDeleted"
ORDER BY "Name";

-- name: ListDoneBackfills :many
SELECT "Name" FROM "Backfills"
WHERE NOT "IsDeleted" AND "DoneAt" IS NOT NULL AND "Name" = ANY(@names::text[])
ORDER BY "Name";
