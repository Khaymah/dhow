-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
ALTER TABLE "PipelineRuns" VALIDATE CONSTRAINT "CK_PipelineRuns_Trigger";

-- +goose Down
