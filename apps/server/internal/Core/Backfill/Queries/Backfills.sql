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
SET "LastKey" = @last_key, "Processed" = "Processed" + @processed,
    "Failed" = CASE WHEN "LastKey" = '' THEN @failed::bigint ELSE "Failed" + @failed::bigint END, "UpdatedAt" = now()
WHERE "Name" = @name;

-- name: EndBackfillPass :exec
UPDATE "Backfills"
SET "LastKey" = '', "Passes" = "Passes" + 1, "Remaining" = @remaining::bigint,
    "Failed" = CASE WHEN "LastKey" = '' THEN 0 ELSE "Failed" END,
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
