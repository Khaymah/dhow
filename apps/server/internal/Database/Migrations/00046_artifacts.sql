-- dhow:phase expand
-- +goose Up
CREATE TABLE "Blobs" (
    "Id"         uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"  timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"  timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"  boolean NOT NULL DEFAULT false,
    "Digest"     bytea NOT NULL CHECK (octet_length("Digest") = 32),
    "SizeBytes"  bigint NOT NULL CHECK ("SizeBytes" >= 0),
    "LastUsedAt" timestamptz NOT NULL DEFAULT now(),
    "StorageKey" TEXT NOT NULL CHECK ("StorageKey" ~ '^[0-9a-f]{2}/[0-9a-f]{64}$')
);

CREATE UNIQUE INDEX "UX_Blobs_Digest" ON "Blobs" ("Digest") WHERE NOT "IsDeleted";
CREATE INDEX "IX_Blobs_LastUsed" ON "Blobs" ("LastUsedAt") WHERE NOT "IsDeleted";

CREATE TABLE "Artifacts" (
    "Id"             uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"      timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"      timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"      boolean NOT NULL DEFAULT false,
    "OrganizationId" uuid NOT NULL,
    "ProjectId"      uuid NOT NULL,
    "RunId"          uuid NOT NULL,
    "RunJobId"       uuid NOT NULL,
    "RunJobKey"      TEXT NOT NULL CHECK (char_length("RunJobKey") BETWEEN 1 AND 1024),
    "JobId"          uuid NOT NULL,
    "Name"           TEXT NOT NULL CHECK ("Name" ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
    "Digest"         bytea NOT NULL CHECK (octet_length("Digest") = 32),
    "SizeBytes"      bigint NOT NULL CHECK ("SizeBytes" >= 0),
    "Untrusted"      boolean NOT NULL,
    "ExpiresAt"      timestamptz NOT NULL,
    FOREIGN KEY ("RunId", "OrganizationId") REFERENCES "PipelineRuns" ("Id", "OrganizationId"),
    FOREIGN KEY ("ProjectId", "OrganizationId") REFERENCES "Projects" ("Id", "OrganizationId")
);

CREATE UNIQUE INDEX "UX_Artifacts_Run_Name" ON "Artifacts" ("RunId", "Name") WHERE NOT "IsDeleted";
CREATE INDEX "IX_Artifacts_Digest" ON "Artifacts" ("Digest") WHERE NOT "IsDeleted";
CREATE INDEX "IX_Artifacts_Expires" ON "Artifacts" ("ExpiresAt") WHERE NOT "IsDeleted";
CREATE INDEX "IX_Artifacts_RunJob" ON "Artifacts" ("RunId", "RunJobKey") WHERE NOT "IsDeleted";

CREATE TABLE "CacheEntries" (
    "Id"             uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"      timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"      timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"      boolean NOT NULL DEFAULT false,
    "OrganizationId" uuid NOT NULL,
    "ProjectId"      uuid NOT NULL,
    "Scope"          TEXT NOT NULL CHECK (char_length("Scope") BETWEEN 1 AND 300),
    "Key"            TEXT NOT NULL CHECK (char_length("Key") BETWEEN 1 AND 512),
    "Digest"         bytea NOT NULL CHECK (octet_length("Digest") = 32),
    "SizeBytes"      bigint NOT NULL CHECK ("SizeBytes" >= 0),
    "LastUsedAt"     timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY ("ProjectId", "OrganizationId") REFERENCES "Projects" ("Id", "OrganizationId")
);

CREATE UNIQUE INDEX "UX_CacheEntries_Project_Scope_Key" ON "CacheEntries" ("ProjectId", "Scope", "Key") WHERE NOT "IsDeleted";
CREATE INDEX "IX_CacheEntries_Project_LastUsed" ON "CacheEntries" ("ProjectId", "LastUsedAt") WHERE NOT "IsDeleted";
CREATE INDEX "IX_CacheEntries_Digest" ON "CacheEntries" ("Digest") WHERE NOT "IsDeleted";

ALTER TABLE "Organizations" ADD COLUMN "LogRetentionDays" integer CHECK ("LogRetentionDays" BETWEEN 1 AND 3650);

-- +goose Down
ALTER TABLE "Organizations" DROP COLUMN "LogRetentionDays";
DROP TABLE "CacheEntries";
DROP TABLE "Artifacts";
DROP TABLE "Blobs";
