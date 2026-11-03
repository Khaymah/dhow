-- name: RegisterServerInstance :one
INSERT INTO "ServerInstances" ("Hostname", "Version", "Commit", "SchemaVersion")
VALUES (@hostname, @version, @commit, @schema_version)
RETURNING "Id";

-- name: TouchServerInstance :execrows
UPDATE "ServerInstances"
SET "LastSeenAt" = now(), "UpdatedAt" = now()
WHERE "Id" = @id AND NOT "IsDeleted";

-- name: RetireServerInstance :exec
UPDATE "ServerInstances"
SET "IsDeleted" = true, "UpdatedAt" = now()
WHERE "Id" = @id AND NOT "IsDeleted";

-- name: RetireStaleServerInstances :execrows
UPDATE "ServerInstances"
SET "IsDeleted" = true, "UpdatedAt" = now()
WHERE NOT "IsDeleted" AND "LastSeenAt" < now() - make_interval(secs => @stale_after_seconds::double precision);

-- name: ListLiveServerInstances :many
SELECT "Id", "Hostname", "Version", "Commit", "SchemaVersion", "StartedAt", "LastSeenAt"
FROM "ServerInstances"
WHERE NOT "IsDeleted" AND "LastSeenAt" >= now() - make_interval(secs => @stale_after_seconds::double precision)
ORDER BY "StartedAt", "Id";
