-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
CREATE INDEX CONCURRENTLY ON "Jobs" ("Attempts");

-- +goose Down
SELECT 1;
