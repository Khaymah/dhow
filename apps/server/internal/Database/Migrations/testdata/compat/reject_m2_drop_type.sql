-- dhow:phase expand
-- +goose Up
DROP TYPE "JobState";

-- +goose Down
SELECT 1;
