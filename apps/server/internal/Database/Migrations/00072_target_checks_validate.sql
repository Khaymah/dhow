-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
ALTER TABLE "PipelineRuns" VALIDATE CONSTRAINT "CK_PipelineRuns_Trigger";
ALTER TABLE "PipelineRuns" VALIDATE CONSTRAINT "CK_PipelineRuns_Source";
ALTER TABLE "RunJobs" VALIDATE CONSTRAINT "CK_RunJobs_Gate";

-- +goose Down
