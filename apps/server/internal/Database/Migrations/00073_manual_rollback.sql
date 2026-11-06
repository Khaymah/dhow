-- dhow:phase expand
-- +goose Up
ALTER TABLE "PipelineRuns" DROP CONSTRAINT IF EXISTS "CK_PipelineRuns_Trigger";
ALTER TABLE "PipelineRuns" ADD CONSTRAINT "CK_PipelineRuns_Trigger"
    CHECK ("Trigger" IN ('push', 'tag', 'pull_request', 'manual', 'api', 'check', 'rollback')) NOT VALID;
ALTER TABLE "PipelineRuns" DROP CONSTRAINT IF EXISTS "CK_PipelineRuns_Source";
ALTER TABLE "PipelineRuns" ADD CONSTRAINT "CK_PipelineRuns_Source"
    CHECK ("Trigger" IN ('manual', 'api', 'check', 'rollback') OR ("Sha" IS NOT NULL AND "RepositoryId" IS NOT NULL)) NOT VALID;

ALTER TABLE "DeploymentReleases" ADD COLUMN IF NOT EXISTS "RestoredReleaseId" TEXT;
ALTER TABLE "DeploymentReleases" ADD COLUMN IF NOT EXISTS "HealthInputs" jsonb NOT NULL DEFAULT '{}';
ALTER TABLE "DeploymentReleases" DROP CONSTRAINT IF EXISTS "CK_DeploymentReleases_Restored";
ALTER TABLE "DeploymentReleases" ADD CONSTRAINT "CK_DeploymentReleases_Restored"
    CHECK ("RestoredReleaseId" IS NULL OR ("Kind" = 'rollback' AND "RestoredReleaseId" ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$')) NOT VALID;
ALTER TABLE "DeploymentReleases" DROP CONSTRAINT IF EXISTS "CK_DeploymentReleases_HealthInputs";
ALTER TABLE "DeploymentReleases" ADD CONSTRAINT "CK_DeploymentReleases_HealthInputs"
    CHECK (jsonb_typeof("HealthInputs") = 'object' AND octet_length("HealthInputs"::text) <= 2048) NOT VALID;
ALTER TABLE "DeploymentReleases" DROP CONSTRAINT IF EXISTS "CK_DeploymentReleases_RollbackRestores";
ALTER TABLE "DeploymentReleases" ADD CONSTRAINT "CK_DeploymentReleases_RollbackRestores"
    CHECK ("Kind" <> 'rollback' OR "RestoredReleaseId" IS NOT NULL) NOT VALID;

-- +goose Down
ALTER TABLE "DeploymentReleases" DROP CONSTRAINT IF EXISTS "CK_DeploymentReleases_RollbackRestores";
ALTER TABLE "DeploymentReleases" DROP CONSTRAINT IF EXISTS "CK_DeploymentReleases_HealthInputs";
ALTER TABLE "DeploymentReleases" DROP CONSTRAINT IF EXISTS "CK_DeploymentReleases_Restored";
ALTER TABLE "DeploymentReleases" DROP COLUMN IF EXISTS "HealthInputs";
ALTER TABLE "DeploymentReleases" DROP COLUMN IF EXISTS "RestoredReleaseId";
