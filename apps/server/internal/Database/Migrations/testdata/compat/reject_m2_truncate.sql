-- dhow:phase expand
-- +goose Up
TRUNCATE TABLE "JobLogs";

-- +goose Down
SELECT 1;
