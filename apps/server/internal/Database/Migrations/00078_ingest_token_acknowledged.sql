-- dhow:phase expand
-- +goose Up
ALTER TABLE "IngestTokens" ADD COLUMN IF NOT EXISTS "AcknowledgedEnvironmentIds" uuid[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE "IngestTokens" DROP COLUMN IF EXISTS "AcknowledgedEnvironmentIds";
