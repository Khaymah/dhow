-- dhow:phase expand
-- +goose Up
DROP TABLE IF EXISTS "Jobs" CASCADE;

-- +goose Down
SELECT 1;
