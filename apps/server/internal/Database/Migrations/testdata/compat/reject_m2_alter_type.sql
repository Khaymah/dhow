-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" ALTER COLUMN "Attempts" TYPE bigint;

-- +goose Down
ALTER TABLE "Jobs" ALTER COLUMN "Attempts" TYPE integer;
