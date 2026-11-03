-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS "IX_PipelineRuns_WebhookRecent" ON "PipelineRuns" ("ProjectId", "CreatedAt")
    WHERE "DeliveryId" IS NOT NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS "IX_PipelineRuns_WebhookRecent";
