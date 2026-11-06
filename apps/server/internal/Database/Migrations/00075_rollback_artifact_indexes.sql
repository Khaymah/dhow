-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS "IX_DeploymentReleases_Digest_Kept" ON "DeploymentReleases" ("ArtifactDigest")
    WHERE NOT "IsDeleted" AND "Kind" = 'deploy' AND "State" = 'succeeded';
CREATE INDEX CONCURRENTLY IF NOT EXISTS "IX_DeploymentReleases_Target_Kept" ON "DeploymentReleases" ("TargetId", "StartedAt" DESC, "Id")
    WHERE NOT "IsDeleted" AND "Kind" = 'deploy' AND "State" = 'succeeded';

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS "IX_DeploymentReleases_Target_Kept";
DROP INDEX CONCURRENTLY IF EXISTS "IX_DeploymentReleases_Digest_Kept";
