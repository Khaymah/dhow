-- dhow:phase expand
-- +goose Up
CREATE TABLE "DeploymentReleases" (
    "Id"                uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"         timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"         timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"         boolean NOT NULL DEFAULT false,
    "OrganizationId"    uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "DeploymentId"      uuid NOT NULL REFERENCES "Deployments" ("Id"),
    "RunJobId"          uuid NOT NULL,
    "JobId"             uuid NOT NULL,
    "LeaseId"           uuid NOT NULL,
    "TargetId"          uuid NOT NULL,
    "StepIndex"         integer NOT NULL CHECK ("StepIndex" >= 0),
    "Kind"              TEXT NOT NULL DEFAULT 'deploy' CHECK ("Kind" IN ('deploy', 'rollback')),
    "ReleaseId"         TEXT NOT NULL CHECK ("ReleaseId" ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
    "PreviousReleaseId" TEXT CHECK ("PreviousReleaseId" ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
    "ArtifactDigest"    bytea CHECK (octet_length("ArtifactDigest") = 32),
    "BatchSize"         integer NOT NULL DEFAULT 1 CHECK ("BatchSize" BETWEEN 1 AND 10),
    "State"             TEXT NOT NULL DEFAULT 'running'
                        CHECK ("State" IN ('running', 'succeeded', 'failed', 'rolled_back', 'rollback_failed', 'unknown')),
    "FailedHostId"      uuid,
    "RollbackReason"    TEXT CHECK ("RollbackReason" IN (
                            'health_check_failed', 'reload_failed', 'hook_failed', 'canceled', 'upload_failed', 'switch_failed', 'other')),
    "Message"           TEXT CHECK (char_length("Message") <= 256),
    "StartedAt"         timestamptz NOT NULL DEFAULT now(),
    "FinishedAt"        timestamptz,
    FOREIGN KEY ("TargetId", "OrganizationId") REFERENCES "DeployTargets" ("Id", "OrganizationId"),
    CHECK (("FinishedAt" IS NOT NULL) = ("State" <> 'running'))
);

CREATE UNIQUE INDEX "UX_DeploymentReleases_Target_Release" ON "DeploymentReleases" ("TargetId", "ReleaseId") WHERE NOT "IsDeleted";
CREATE UNIQUE INDEX "UX_DeploymentReleases_Target_Running" ON "DeploymentReleases" ("TargetId") WHERE "State" = 'running' AND NOT "IsDeleted";
CREATE UNIQUE INDEX "UX_DeploymentReleases_Id_Organization" ON "DeploymentReleases" ("Id", "OrganizationId");
CREATE INDEX "IX_DeploymentReleases_Target" ON "DeploymentReleases" ("TargetId", "CreatedAt" DESC) WHERE NOT "IsDeleted";
CREATE INDEX "IX_DeploymentReleases_RunJob" ON "DeploymentReleases" ("RunJobId") WHERE NOT "IsDeleted";

CREATE TABLE "DeploymentReleaseHosts" (
    "Id"                uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"         timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"         timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"         boolean NOT NULL DEFAULT false,
    "OrganizationId"    uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "ReleaseRowId"      uuid NOT NULL,
    "HostId"            uuid NOT NULL,
    "ReleaseId"         TEXT NOT NULL,
    "Position"          integer NOT NULL CHECK ("Position" >= 1),
    "Batch"             integer CHECK ("Batch" BETWEEN 1 AND 10),
    "State"             TEXT NOT NULL DEFAULT 'pending'
                        CHECK ("State" IN ('pending', 'uploaded', 'switched', 'verified', 'failed', 'rolled_back', 'rollback_failed', 'unknown', 'skipped')),
    "PreviousReleaseId" TEXT CHECK ("PreviousReleaseId" ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
    "Health"            jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof("Health") = 'object' AND octet_length("Health"::text) <= 1024),
    "RollbackReason"    TEXT CHECK ("RollbackReason" IN (
                            'health_check_failed', 'reload_failed', 'hook_failed', 'canceled', 'upload_failed', 'switch_failed', 'other')),
    "Message"           TEXT CHECK (char_length("Message") <= 256),
    "SwitchedAt"        timestamptz,
    "RolledBackAt"      timestamptz,
    "FinishedAt"        timestamptz,
    FOREIGN KEY ("ReleaseRowId", "OrganizationId") REFERENCES "DeploymentReleases" ("Id", "OrganizationId")
);

CREATE UNIQUE INDEX "UX_DeploymentReleaseHosts_Release_Host" ON "DeploymentReleaseHosts" ("ReleaseRowId", "HostId");
CREATE UNIQUE INDEX "UX_DeploymentReleaseHosts_Host_Release" ON "DeploymentReleaseHosts" ("HostId", "ReleaseId");

-- +goose Down
DROP TABLE "DeploymentReleaseHosts";
DROP TABLE "DeploymentReleases";
