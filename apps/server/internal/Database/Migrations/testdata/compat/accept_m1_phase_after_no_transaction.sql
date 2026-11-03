-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS "IX_Jobs_Note" ON "Jobs" ("Note");

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS "IX_Jobs_Note";
