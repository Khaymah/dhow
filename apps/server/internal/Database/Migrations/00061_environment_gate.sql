-- dhow:phase expand
-- +goose Up
ALTER TABLE "RunJobs" ADD COLUMN "EnvironmentId" uuid;
ALTER TABLE "RunJobs" ADD COLUMN "Gate" TEXT;
ALTER TABLE "RunJobs" ADD COLUMN "GateOpensAt" timestamptz;
ALTER TABLE "RunJobs" ADD CONSTRAINT "CK_RunJobs_Gate"
    CHECK ("Gate" IS NULL OR "Gate" IN ('approval', 'timer', 'paused', 'environment_busy')) NOT VALID;

ALTER TABLE "Environments" ADD COLUMN "LockedByRunId" uuid;
ALTER TABLE "Environments" ADD COLUMN "LockedAt" timestamptz;
ALTER TABLE "Environments" ADD CONSTRAINT "CK_Environments_Lock"
    CHECK (("LockedByRunId" IS NULL) = ("LockedAt" IS NULL)) NOT VALID;

CREATE TABLE "Deployments" (
    "Id"             uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"      timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"      timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"      boolean NOT NULL DEFAULT false,
    "OrganizationId" uuid NOT NULL,
    "EnvironmentId"  uuid NOT NULL,
    "RunId"          uuid NOT NULL,
    "RunNumber"      bigint NOT NULL CHECK ("RunNumber" >= 1),
    "RunJobId"       uuid NOT NULL,
    "JobId"          uuid NOT NULL,
    "JobKey"         TEXT NOT NULL CHECK (char_length("JobKey") BETWEEN 1 AND 1024),
    "Ref"            TEXT NOT NULL CHECK (char_length("Ref") BETWEEN 1 AND 255),
    "Sha"            TEXT CHECK ("Sha" ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    "RuleVersion"    bigint NOT NULL CHECK ("RuleVersion" >= 1),
    "State"          TEXT NOT NULL DEFAULT 'queued'
                     CHECK ("State" IN ('queued', 'running', 'succeeded', 'failed', 'canceled')),
    "FinishedAt"     timestamptz,
    FOREIGN KEY ("EnvironmentId", "OrganizationId") REFERENCES "Environments" ("Id", "OrganizationId"),
    CHECK (("FinishedAt" IS NOT NULL) = ("State" IN ('succeeded', 'failed', 'canceled')))
);

CREATE UNIQUE INDEX "UX_Deployments_RunJob_Job" ON "Deployments" ("RunJobId", "JobId");
CREATE INDEX "IX_Deployments_Environment" ON "Deployments" ("EnvironmentId", "CreatedAt" DESC) WHERE NOT "IsDeleted";

-- +goose Down
DROP TABLE "Deployments";
ALTER TABLE "Environments" DROP CONSTRAINT "CK_Environments_Lock";
ALTER TABLE "Environments" DROP COLUMN "LockedAt";
ALTER TABLE "Environments" DROP COLUMN "LockedByRunId";
ALTER TABLE "RunJobs" DROP CONSTRAINT "CK_RunJobs_Gate";
ALTER TABLE "RunJobs" DROP COLUMN "GateOpensAt";
ALTER TABLE "RunJobs" DROP COLUMN "Gate";
ALTER TABLE "RunJobs" DROP COLUMN "EnvironmentId";
