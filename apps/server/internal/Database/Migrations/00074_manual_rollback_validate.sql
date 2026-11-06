-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
ALTER TABLE "PipelineRuns" VALIDATE CONSTRAINT "CK_PipelineRuns_Trigger";
ALTER TABLE "PipelineRuns" VALIDATE CONSTRAINT "CK_PipelineRuns_Source";
ALTER TABLE "DeploymentReleases" VALIDATE CONSTRAINT "CK_DeploymentReleases_Restored";
ALTER TABLE "DeploymentReleases" VALIDATE CONSTRAINT "CK_DeploymentReleases_HealthInputs";

-- +goose Down
