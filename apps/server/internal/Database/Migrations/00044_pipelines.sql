-- +goose Up
CREATE TABLE "PipelineRunCounters" (
    "Id"        uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt" timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt" timestamptz NOT NULL DEFAULT now(),
    "IsDeleted" boolean NOT NULL DEFAULT false,
    "ProjectId" uuid NOT NULL REFERENCES "Projects" ("Id"),
    "Next"      bigint NOT NULL DEFAULT 1 CHECK ("Next" >= 1)
);

CREATE UNIQUE INDEX "UX_PipelineRunCounters_Project" ON "PipelineRunCounters" ("ProjectId");

CREATE TABLE "PipelineRuns" (
    "Id"                uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"         timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"         timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"         boolean NOT NULL DEFAULT false,
    "OrganizationId"    uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "ProjectId"         uuid NOT NULL,
    "RepositoryId"      uuid,
    "Provider"          TEXT CHECK ("Provider" IN ('github', 'gitlab', 'generic')),
    "Number"            bigint NOT NULL CHECK ("Number" >= 1),
    "Trigger"           TEXT NOT NULL CHECK ("Trigger" IN ('push', 'tag', 'pull_request', 'manual', 'api')),
    "Ref"               TEXT NOT NULL CHECK (char_length("Ref") BETWEEN 1 AND 255),
    "BaseRef"           TEXT CHECK (char_length("BaseRef") BETWEEN 1 AND 255),
    "Sha"               TEXT CHECK ("Sha" ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    "BaseSha"           TEXT CHECK ("BaseSha" ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    "IsFork"            boolean NOT NULL DEFAULT false,
    "PrNumber"          integer CHECK ("PrNumber" > 0),
    "Actor"             TEXT CHECK (char_length("Actor") BETWEEN 1 AND 255),
    "Inputs"            jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof("Inputs") = 'object' AND octet_length("Inputs"::text) <= 131072),
    "DefinitionPath"    TEXT NOT NULL CHECK (char_length("DefinitionPath") BETWEEN 1 AND 255),
    "DefinitionSha"     TEXT CHECK ("DefinitionSha" ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    "DefinitionDigest"  bytea CHECK (octet_length("DefinitionDigest") = 32),
    "DefinitionSource"  TEXT CHECK (octet_length("DefinitionSource") <= 262144),
    "ImagePins"         jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof("ImagePins") = 'object'),
    "Issues"            jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof("Issues") = 'array'),
    "State"             TEXT NOT NULL DEFAULT 'pending'
                        CHECK ("State" IN ('awaiting_approval', 'pending', 'queued', 'running', 'succeeded', 'failed', 'canceled', 'skipped')),
    "FailureReason"     TEXT NOT NULL DEFAULT '' CHECK (char_length("FailureReason") <= 64),
    "ConcurrencyGroup"  TEXT CHECK (char_length("ConcurrencyGroup") BETWEEN 1 AND 1024),
    "CancelInProgress"  boolean NOT NULL DEFAULT false,
    "DeliveryId"        uuid,
    "TriggeredByUserId" uuid REFERENCES "Users" ("Id"),
    "ApprovedByUserId"  uuid REFERENCES "Users" ("Id"),
    "ApprovedAt"        timestamptz,
    "CancelRequestedAt" timestamptz,
    "CancelRequestedBy" uuid REFERENCES "Users" ("Id"),
    "ClaimedUntil"      timestamptz,
    "StartedAt"         timestamptz,
    "FinishedAt"        timestamptz,
    FOREIGN KEY ("ProjectId", "OrganizationId") REFERENCES "Projects" ("Id", "OrganizationId"),
    FOREIGN KEY ("RepositoryId", "OrganizationId") REFERENCES "Repositories" ("Id", "OrganizationId"),
    CHECK (("FinishedAt" IS NOT NULL) = ("State" IN ('succeeded', 'failed', 'canceled', 'skipped'))),
    CHECK (("ApprovedAt" IS NULL) = ("ApprovedByUserId" IS NULL)),
    CHECK ("Trigger" <> 'pull_request' OR ("PrNumber" IS NOT NULL AND "BaseSha" IS NOT NULL)),
    CHECK ("Trigger" IN ('manual', 'api') OR ("Sha" IS NOT NULL AND "RepositoryId" IS NOT NULL)),
    CHECK ("IsFork" = false OR "Trigger" = 'pull_request'),
    CHECK (("DefinitionSource" IS NULL) = ("DefinitionDigest" IS NULL))
);

CREATE UNIQUE INDEX "UX_PipelineRuns_Project_Number" ON "PipelineRuns" ("ProjectId", "Number");
CREATE UNIQUE INDEX "UX_PipelineRuns_Delivery" ON "PipelineRuns" ("DeliveryId") WHERE "DeliveryId" IS NOT NULL;
CREATE UNIQUE INDEX "UX_PipelineRuns_Commit" ON "PipelineRuns" ("RepositoryId", "Ref", "Sha", "Trigger", "IsFork")
    WHERE "Trigger" IN ('push', 'tag', 'pull_request');
CREATE INDEX "IX_PipelineRuns_RefHead" ON "PipelineRuns" ("RepositoryId", "Ref", "CreatedAt" DESC)
    WHERE "Trigger" IN ('push', 'tag') AND NOT "IsFork";
CREATE UNIQUE INDEX "UX_PipelineRuns_Id_Organization" ON "PipelineRuns" ("Id", "OrganizationId");
CREATE INDEX "IX_PipelineRuns_Project" ON "PipelineRuns" ("ProjectId", "Number" DESC) WHERE NOT "IsDeleted";
CREATE INDEX "IX_PipelineRuns_Open" ON "PipelineRuns" ("State", "CreatedAt")
    WHERE "State" IN ('pending', 'queued', 'running') AND NOT "IsDeleted";
CREATE INDEX "IX_PipelineRuns_Group" ON "PipelineRuns" ("ProjectId", "ConcurrencyGroup", "Number")
    WHERE "ConcurrencyGroup" IS NOT NULL AND "State" IN ('queued', 'running') AND NOT "IsDeleted";

CREATE TABLE "RunJobs" (
    "Id"             uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"      timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"      timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"      boolean NOT NULL DEFAULT false,
    "OrganizationId" uuid NOT NULL,
    "RunId"          uuid NOT NULL,
    "Kind"           TEXT NOT NULL DEFAULT 'job' CHECK ("Kind" IN ('job', 'discovery')),
    "Key"            TEXT NOT NULL CHECK (char_length("Key") BETWEEN 1 AND 1024),
    "JobName"        TEXT NOT NULL CHECK (char_length("JobName") BETWEEN 1 AND 1024),
    "JobId"          TEXT NOT NULL CHECK (char_length("JobId") BETWEEN 1 AND 63),
    "Matrix"         jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof("Matrix") = 'array'),
    "Needs"          TEXT[] NOT NULL DEFAULT '{}' CHECK (cardinality("Needs") <= 256),
    "Position"       integer NOT NULL CHECK ("Position" >= 0),
    "Digest"         TEXT NOT NULL DEFAULT '' CHECK (char_length("Digest") <= 64),
    "State"          TEXT NOT NULL DEFAULT 'waiting'
                     CHECK ("State" IN ('waiting', 'active', 'succeeded', 'failed', 'canceled', 'skipped')),
    "FailureReason"  TEXT NOT NULL DEFAULT '' CHECK (char_length("FailureReason") <= 64),
    "CurrentJobId"   uuid,
    "Attempts"       integer NOT NULL DEFAULT 0 CHECK ("Attempts" >= 0),
    "Outputs"        jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof("Outputs") = 'object' AND octet_length("Outputs"::text) <= 65536),
    "StartedAt"      timestamptz,
    "FinishedAt"     timestamptz,
    FOREIGN KEY ("RunId", "OrganizationId") REFERENCES "PipelineRuns" ("Id", "OrganizationId"),
    CHECK ("State" <> 'active' OR ("CurrentJobId" IS NOT NULL AND "FinishedAt" IS NULL)),
    CHECK (("FinishedAt" IS NOT NULL) = ("State" IN ('succeeded', 'failed', 'canceled', 'skipped')))
);

CREATE UNIQUE INDEX "UX_RunJobs_Run_Key" ON "RunJobs" ("RunId", "Key");
CREATE UNIQUE INDEX "UX_RunJobs_CurrentJob" ON "RunJobs" ("CurrentJobId") WHERE "CurrentJobId" IS NOT NULL;
CREATE INDEX "IX_RunJobs_Run" ON "RunJobs" ("RunId", "Position") WHERE NOT "IsDeleted";
CREATE INDEX "IX_RunJobs_Active" ON "RunJobs" ("UpdatedAt") WHERE "State" = 'active' AND NOT "IsDeleted";

-- +goose StatementBegin
CREATE FUNCTION "GuardPipelineRunState"() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW."State" IN ('pending', 'awaiting_approval') THEN
            RETURN NEW;
        END IF;
        RAISE EXCEPTION 'a run starts pending or awaiting approval, not %', NEW."State" USING ERRCODE = 'check_violation';
    END IF;
    IF NEW."State" = OLD."State" OR (OLD."State", NEW."State") IN (
        ('awaiting_approval', 'pending'), ('awaiting_approval', 'canceled'),
        ('pending', 'queued'), ('pending', 'running'), ('pending', 'failed'), ('pending', 'canceled'), ('pending', 'skipped'),
        ('queued', 'running'), ('queued', 'canceled'), ('queued', 'failed'),
        ('running', 'succeeded'), ('running', 'failed'), ('running', 'canceled'),
        ('failed', 'running'), ('canceled', 'running')
    ) THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'a run cannot move from % to %', OLD."State", NEW."State" USING ERRCODE = 'check_violation';
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION "GuardRunJobState"() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW."State" = 'waiting' THEN
            RETURN NEW;
        END IF;
        RAISE EXCEPTION 'a run job starts waiting, not %', NEW."State" USING ERRCODE = 'check_violation';
    END IF;
    IF OLD."State" = 'succeeded' AND NEW."State" <> 'succeeded' THEN
        RAISE EXCEPTION 'a succeeded run job cannot change' USING ERRCODE = 'check_violation';
    END IF;
    IF NEW."State" = OLD."State" OR (OLD."State", NEW."State") IN (
        ('waiting', 'active'), ('waiting', 'skipped'), ('waiting', 'canceled'), ('waiting', 'failed'),
        ('active', 'succeeded'), ('active', 'failed'), ('active', 'canceled'),
        ('failed', 'waiting'), ('canceled', 'waiting'), ('skipped', 'waiting')
    ) THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'a run job cannot move from % to %', OLD."State", NEW."State" USING ERRCODE = 'check_violation';
END
$$;
-- +goose StatementEnd

CREATE TRIGGER "TR_PipelineRuns_State" BEFORE INSERT OR UPDATE ON "PipelineRuns"
    FOR EACH ROW EXECUTE FUNCTION "GuardPipelineRunState"();
CREATE TRIGGER "TR_RunJobs_State" BEFORE INSERT OR UPDATE ON "RunJobs"
    FOR EACH ROW EXECUTE FUNCTION "GuardRunJobState"();

ALTER TABLE "Jobs" ALTER COLUMN "CreatedByUserId" DROP NOT NULL;
ALTER TABLE "Jobs" DROP CONSTRAINT "Jobs_check1";
ALTER TABLE "Jobs" ADD CONSTRAINT "CK_Jobs_CancelRequestedBy" CHECK ("CancelRequestedBy" IS NULL OR "CancelRequestedAt" IS NOT NULL);

ALTER TABLE "ApiTokens" DROP CONSTRAINT "ApiTokens_Scopes_check";
ALTER TABLE "ApiTokens" ADD CONSTRAINT "ApiTokens_Scopes_check" CHECK (
    cardinality("Scopes") BETWEEN 1 AND 16
    AND "Scopes" <@ ARRAY['org.read', 'org.update', 'member.read', 'member.manage', 'audit.read', 'runner.read', 'runner.manage',
                          'job.read', 'job.cancel', 'project.read', 'project.manage', 'run.read', 'run.trigger', 'run.cancel']::text[]
);

-- +goose Down
UPDATE "ApiTokens" SET "Scopes" = array_remove(array_remove(array_remove("Scopes", 'run.read'), 'run.trigger'), 'run.cancel');
UPDATE "ApiTokens" SET "IsDeleted" = true, "Scopes" = ARRAY['org.read']::text[] WHERE cardinality("Scopes") = 0;
ALTER TABLE "ApiTokens" DROP CONSTRAINT "ApiTokens_Scopes_check";
ALTER TABLE "ApiTokens" ADD CONSTRAINT "ApiTokens_Scopes_check" CHECK (
    cardinality("Scopes") BETWEEN 1 AND 16
    AND "Scopes" <@ ARRAY['org.read', 'org.update', 'member.read', 'member.manage', 'audit.read', 'runner.read', 'runner.manage',
                          'job.read', 'job.cancel', 'project.read', 'project.manage']::text[]
);
ALTER TABLE "Jobs" DISABLE TRIGGER "TR_Jobs_State";
UPDATE "Jobs" j
SET "CancelRequestedBy" = (SELECT m."UserId"
                           FROM "Memberships" m
                           WHERE m."OrganizationId" = j."OrganizationId" AND m."Role" = 'owner'
                           ORDER BY m."CreatedAt"
                           LIMIT 1)
WHERE j."CancelRequestedAt" IS NOT NULL AND j."CancelRequestedBy" IS NULL;
UPDATE "Jobs" j
SET "CreatedByUserId" = (SELECT m."UserId"
                         FROM "Memberships" m
                         WHERE m."OrganizationId" = j."OrganizationId" AND m."Role" = 'owner'
                         ORDER BY m."CreatedAt"
                         LIMIT 1)
WHERE j."CreatedByUserId" IS NULL;
ALTER TABLE "Jobs" ENABLE TRIGGER "TR_Jobs_State";
ALTER TABLE "Jobs" ALTER COLUMN "CreatedByUserId" SET NOT NULL;
ALTER TABLE "Jobs" DROP CONSTRAINT "CK_Jobs_CancelRequestedBy";
ALTER TABLE "Jobs" ADD CONSTRAINT "Jobs_check1" CHECK (("CancelRequestedAt" IS NULL) = ("CancelRequestedBy" IS NULL));
DROP TRIGGER "TR_RunJobs_State" ON "RunJobs";
DROP TRIGGER "TR_PipelineRuns_State" ON "PipelineRuns";
DROP FUNCTION "GuardRunJobState"();
DROP FUNCTION "GuardPipelineRunState"();
DROP TABLE "RunJobs";
DROP TABLE "PipelineRuns";
DROP TABLE "PipelineRunCounters";
