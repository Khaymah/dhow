-- dhow:phase expand
-- +goose Up
DROP FUNCTION IF EXISTS "EnsureLogPartitions"(date, integer);

-- +goose Down
SELECT 1;
