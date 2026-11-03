-- dhow:phase expand
-- +goose Up
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS "UX_Jobs_Note" ON "Jobs" ("Note");

-- +goose Down
DROP INDEX "UX_Jobs_Note";
