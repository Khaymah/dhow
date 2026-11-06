-- dhow:phase expand
-- +goose Up
ALTER TABLE "PipelineRuns" DROP CONSTRAINT IF EXISTS "PipelineRuns_Trigger_check";
ALTER TABLE "PipelineRuns" DROP CONSTRAINT IF EXISTS "CK_PipelineRuns_Trigger";
ALTER TABLE "PipelineRuns" ADD CONSTRAINT "CK_PipelineRuns_Trigger"
    CHECK ("Trigger" IN ('push', 'tag', 'pull_request', 'manual', 'api', 'check')) NOT VALID;
ALTER TABLE "PipelineRuns" DROP CONSTRAINT IF EXISTS "PipelineRuns_check3";
ALTER TABLE "PipelineRuns" DROP CONSTRAINT IF EXISTS "CK_PipelineRuns_Source";
ALTER TABLE "PipelineRuns" ADD CONSTRAINT "CK_PipelineRuns_Source"
    CHECK ("Trigger" IN ('manual', 'api', 'check') OR ("Sha" IS NOT NULL AND "RepositoryId" IS NOT NULL)) NOT VALID;

ALTER TABLE "RunJobs" DROP CONSTRAINT IF EXISTS "CK_RunJobs_Gate";
ALTER TABLE "RunJobs" ADD CONSTRAINT "CK_RunJobs_Gate"
    CHECK ("Gate" IS NULL OR "Gate" IN ('approval', 'timer', 'paused', 'environment_busy', 'target_busy')) NOT VALID;

CREATE TABLE IF NOT EXISTS "DeploymentHostChecks" (
    "Id"                uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"         timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"         timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"         boolean NOT NULL DEFAULT false,
    "OrganizationId"    uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "TargetId"          uuid NOT NULL,
    "HostId"            uuid NOT NULL,
    "DeploymentId"      uuid NOT NULL REFERENCES "Deployments" ("Id"),
    "RunJobId"          uuid NOT NULL,
    "Reason"            TEXT NOT NULL CHECK ("Reason" IN (
                            'ok', 'host_key_mismatch', 'unreachable', 'auth_failed', 'path_missing', 'current_not_symlink',
                            'state_corrupt', 'disk_low', 'locked', 'diverged')),
    "Message"           TEXT CHECK (char_length("Message") <= 256),
    "CurrentReleaseId"  TEXT CHECK ("CurrentReleaseId" ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
    "VerifiedReleaseId" TEXT CHECK ("VerifiedReleaseId" ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
    "FreeBytes"         bigint CHECK ("FreeBytes" >= 0),
    "Locked"            boolean NOT NULL DEFAULT false,
    "Repaired"          boolean NOT NULL DEFAULT false,
    "CheckedAt"         timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY ("TargetId", "OrganizationId") REFERENCES "DeployTargets" ("Id", "OrganizationId")
);

CREATE UNIQUE INDEX "UX_DeploymentHostChecks_RunJob_Host" ON "DeploymentHostChecks" ("RunJobId", "HostId") WHERE NOT "IsDeleted";
CREATE INDEX "IX_DeploymentHostChecks_Target" ON "DeploymentHostChecks" ("TargetId", "CheckedAt" DESC) WHERE NOT "IsDeleted";

-- +goose Down
DROP TABLE IF EXISTS "DeploymentHostChecks";
ALTER TABLE "RunJobs" DROP CONSTRAINT IF EXISTS "CK_RunJobs_Gate";
UPDATE "RunJobs" SET "Gate" = 'environment_busy' WHERE "Gate" = 'target_busy';
ALTER TABLE "RunJobs" ADD CONSTRAINT "CK_RunJobs_Gate"
    CHECK ("Gate" IS NULL OR "Gate" IN ('approval', 'timer', 'paused', 'environment_busy'));
