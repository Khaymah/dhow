-- name: GetInstance :one
SELECT "Id", "CreatedAt", "UpdatedAt", "SetupCompletedAt", "SchemaFloor"
FROM "Instance"
WHERE "Id" = 1;

-- name: GetDatabaseTime :one
SELECT now()::timestamptz AS "Now";
