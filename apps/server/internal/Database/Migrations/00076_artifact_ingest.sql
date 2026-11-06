-- dhow:phase expand
-- +goose Up
ALTER TABLE "PipelineRuns" DROP CONSTRAINT IF EXISTS "CK_PipelineRuns_Trigger";
ALTER TABLE "PipelineRuns" ADD CONSTRAINT "CK_PipelineRuns_Trigger"
    CHECK ("Trigger" IN ('push', 'tag', 'pull_request', 'manual', 'api', 'check', 'rollback', 'artifact')) NOT VALID;

CREATE TABLE "IngestTokens" (
    "Id"              uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"       timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"       timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"       boolean NOT NULL DEFAULT false,
    "OrganizationId"  uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "ProjectId"       uuid NOT NULL,
    "Name"            TEXT NOT NULL CHECK (char_length("Name") BETWEEN 1 AND 64),
    "TokenHash"       bytea NOT NULL CHECK (octet_length("TokenHash") = 32),
    "Prefix"          TEXT NOT NULL CHECK ("Prefix" ~ '^dhow_ing_[A-Za-z0-9_-]{8}$'),
    "RefPatterns"     jsonb NOT NULL CHECK (jsonb_typeof("RefPatterns") = 'array' AND jsonb_array_length("RefPatterns") BETWEEN 1 AND 20
                                            AND octet_length("RefPatterns"::text) <= 8192),
    "ExpiresAt"       timestamptz NOT NULL,
    "LastUsedAt"      timestamptz,
    "RevokedAt"       timestamptz,
    "RevokedByUserId" uuid REFERENCES "Users" ("Id"),
    "CreatedByUserId" uuid NOT NULL REFERENCES "Users" ("Id"),
    FOREIGN KEY ("ProjectId", "OrganizationId") REFERENCES "Projects" ("Id", "OrganizationId"),
    CHECK (("RevokedAt" IS NULL) = ("RevokedByUserId" IS NULL))
);

CREATE UNIQUE INDEX "UX_IngestTokens_Hash" ON "IngestTokens" ("TokenHash");
CREATE UNIQUE INDEX "UX_IngestTokens_Project_Name" ON "IngestTokens" ("ProjectId", "Name") WHERE NOT "IsDeleted" AND "RevokedAt" IS NULL;
CREATE INDEX "IX_IngestTokens_Project" ON "IngestTokens" ("ProjectId", "CreatedAt" DESC) WHERE NOT "IsDeleted";

CREATE TABLE "IngestedArtifacts" (
    "Id"                 uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"          timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"          timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"          boolean NOT NULL DEFAULT false,
    "OrganizationId"     uuid NOT NULL,
    "ProjectId"          uuid NOT NULL,
    "TokenId"            uuid NOT NULL REFERENCES "IngestTokens" ("Id"),
    "Name"               TEXT NOT NULL CHECK ("Name" ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
    "Digest"             bytea NOT NULL CHECK (octet_length("Digest") = 32),
    "SizeBytes"          bigint NOT NULL CHECK ("SizeBytes" >= 0),
    "Ref"                TEXT NOT NULL CHECK (char_length("Ref") BETWEEN 1 AND 255),
    "RefKind"            TEXT NOT NULL CHECK ("RefKind" IN ('branch', 'tag')),
    "Sha"                TEXT NOT NULL CHECK ("Sha" ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    "Provenance"         jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof("Provenance") = 'object' AND octet_length("Provenance"::text) <= 4096),
    "ProvenanceVerified" boolean NOT NULL DEFAULT false,
    "ExpiresAt"          timestamptz NOT NULL,
    "PinnedUntil"        timestamptz,
    FOREIGN KEY ("ProjectId", "OrganizationId") REFERENCES "Projects" ("Id", "OrganizationId")
);

CREATE UNIQUE INDEX "UX_IngestedArtifacts_Identity" ON "IngestedArtifacts" ("ProjectId", "Digest", "Name", "Ref", "Sha") WHERE NOT "IsDeleted";
CREATE UNIQUE INDEX "UX_IngestedArtifacts_Id_Organization" ON "IngestedArtifacts" ("Id", "OrganizationId");
CREATE INDEX "IX_IngestedArtifacts_Project" ON "IngestedArtifacts" ("ProjectId", "CreatedAt" DESC) WHERE NOT "IsDeleted";
CREATE INDEX "IX_IngestedArtifacts_Digest" ON "IngestedArtifacts" ("Digest") WHERE NOT "IsDeleted";
CREATE INDEX "IX_IngestedArtifacts_Expires" ON "IngestedArtifacts" ("ExpiresAt") WHERE NOT "IsDeleted";

CREATE TABLE "RunIngestedArtifacts" (
    "Id"                 uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"          timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"          timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"          boolean NOT NULL DEFAULT false,
    "OrganizationId"     uuid NOT NULL,
    "RunId"              uuid NOT NULL,
    "Name"               TEXT NOT NULL CHECK ("Name" ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
    "IngestedArtifactId" uuid NOT NULL,
    FOREIGN KEY ("RunId", "OrganizationId") REFERENCES "PipelineRuns" ("Id", "OrganizationId")
);

CREATE UNIQUE INDEX "UX_RunIngestedArtifacts_Run_Name" ON "RunIngestedArtifacts" ("RunId", "Name") WHERE NOT "IsDeleted";
CREATE INDEX "IX_RunIngestedArtifacts_Artifact" ON "RunIngestedArtifacts" ("IngestedArtifactId") WHERE NOT "IsDeleted";

-- +goose Down
DROP TABLE "RunIngestedArtifacts";
DROP TABLE "IngestedArtifacts";
DROP TABLE "IngestTokens";
ALTER TABLE "PipelineRuns" DROP CONSTRAINT IF EXISTS "CK_PipelineRuns_Trigger";
ALTER TABLE "PipelineRuns" ADD CONSTRAINT "CK_PipelineRuns_Trigger"
    CHECK ("Trigger" IN ('push', 'tag', 'pull_request', 'manual', 'api', 'check', 'rollback'));
