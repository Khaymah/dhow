-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
ALTER TABLE "RunJobs" VALIDATE CONSTRAINT "CK_RunJobs_Gate";
ALTER TABLE "Environments" VALIDATE CONSTRAINT "CK_Environments_Lock";
CREATE INDEX CONCURRENTLY IF NOT EXISTS "IX_RunJobs_Gated" ON "RunJobs" ("RunId") WHERE "Gate" IS NOT NULL AND "State" = 'waiting' AND NOT "IsDeleted";
CREATE INDEX CONCURRENTLY IF NOT EXISTS "IX_RunJobs_EnvironmentQueue" ON "RunJobs" ("EnvironmentId")
    WHERE "Gate" = 'environment_busy' AND "State" = 'waiting' AND NOT "IsDeleted";
CREATE INDEX CONCURRENTLY IF NOT EXISTS "IX_Environments_LockedBy" ON "Environments" ("LockedByRunId")
    WHERE "LockedByRunId" IS NOT NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS "IX_Environments_LockedBy";
DROP INDEX CONCURRENTLY IF EXISTS "IX_RunJobs_EnvironmentQueue";
DROP INDEX CONCURRENTLY IF EXISTS "IX_RunJobs_Gated";
