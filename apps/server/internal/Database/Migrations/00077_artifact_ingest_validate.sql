-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
ALTER TABLE "PipelineRuns" VALIDATE CONSTRAINT "CK_PipelineRuns_Trigger";
ALTER TABLE "RunIngestedArtifacts" VALIDATE CONSTRAINT "FK_RunIngestedArtifacts_Artifact";

-- +goose Down
