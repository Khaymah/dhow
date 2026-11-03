-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
CREATE INDEX CONCURRENTLY "IX_Jobs_Note" ON ONLY "Jobs" ("Note");

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS "IX_Jobs_Note";
