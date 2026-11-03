-- name: GetSchemaFloor :one
SELECT "SchemaFloor" FROM "Instance" WHERE "Id" = 1;

-- name: SetPreflightTimeouts :exec
SELECT set_config('lock_timeout', @lock_timeout::text, true), set_config('statement_timeout', @statement_timeout::text, true);

-- name: ListInvalidIndexDrops :many
SELECT format('DROP INDEX CONCURRENTLY IF EXISTS %I.%I', n.nspname, c.relname)::text AS "Statement"
FROM pg_catalog.pg_index i
JOIN pg_catalog.pg_class c ON c.oid = i.indexrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE NOT i.indisvalid
  AND n.nspname = current_schema()
  AND c.relname = ANY(@names::text[]);
