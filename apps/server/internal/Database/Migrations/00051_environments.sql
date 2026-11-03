-- dhow:phase expand
-- +goose Up
CREATE TABLE "Environments" (
    "Id"                   uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"            timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"            timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"            boolean NOT NULL DEFAULT false,
    "OrganizationId"       uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "ProjectId"            uuid NOT NULL,
    "Name"                 TEXT NOT NULL CHECK ("Name" ~ '^[a-z0-9][a-z0-9_-]{0,63}$'),
    "Description"          TEXT NOT NULL DEFAULT '' CHECK (char_length("Description") <= 500),
    "Version"              bigint NOT NULL DEFAULT 1 CHECK ("Version" >= 1),
    "RuleVersion"          bigint NOT NULL DEFAULT 1 CHECK ("RuleVersion" >= 1),
    "AllowedRefs"          jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof("AllowedRefs") = 'array' AND jsonb_array_length("AllowedRefs") <= 50),
    "AllowPullRequests"    boolean NOT NULL DEFAULT false,
    "RequiredApprovals"    integer NOT NULL DEFAULT 1 CHECK ("RequiredApprovals" = 1),
    "AllowSelfApproval"    boolean NOT NULL DEFAULT false,
    "WaitMinutes"          integer NOT NULL DEFAULT 0 CHECK ("WaitMinutes" BETWEEN 0 AND 43200),
    "ApprovalTimeoutHours" integer NOT NULL DEFAULT 72 CHECK ("ApprovalTimeoutHours" BETWEEN 1 AND 720),
    "RequirePinnedImages"  boolean NOT NULL DEFAULT false,
    "AllowedPlugins"       TEXT[] NOT NULL DEFAULT '{}' CHECK (cardinality("AllowedPlugins") <= 50),
    "RequiredIsolation"    TEXT NOT NULL DEFAULT 'container' CHECK ("RequiredIsolation" IN ('container')),
    "AllowAdminBypass"     boolean NOT NULL DEFAULT false,
    "SupersedePending"     boolean NOT NULL DEFAULT true,
    "PausedAt"             timestamptz,
    "PausedByUserId"       uuid REFERENCES "Users" ("Id"),
    "PausedReason"         TEXT CHECK (char_length("PausedReason") BETWEEN 1 AND 500),
    "CreatedByUserId"      uuid NOT NULL REFERENCES "Users" ("Id"),
    FOREIGN KEY ("ProjectId", "OrganizationId") REFERENCES "Projects" ("Id", "OrganizationId"),
    CHECK (("PausedAt" IS NULL) = ("PausedByUserId" IS NULL) AND ("PausedAt" IS NULL) = ("PausedReason" IS NULL))
);

CREATE UNIQUE INDEX "UX_Environments_Project_Name" ON "Environments" ("ProjectId", "Name") WHERE NOT "IsDeleted";
CREATE UNIQUE INDEX "UX_Environments_Id_Organization" ON "Environments" ("Id", "OrganizationId");

CREATE TABLE "EnvironmentReviewers" (
    "Id"             uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"      timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"      timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"      boolean NOT NULL DEFAULT false,
    "OrganizationId" uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "EnvironmentId"  uuid NOT NULL,
    "UserId"         uuid REFERENCES "Users" ("Id"),
    "TeamId"         uuid REFERENCES "Teams" ("Id"),
    FOREIGN KEY ("EnvironmentId", "OrganizationId") REFERENCES "Environments" ("Id", "OrganizationId"),
    CHECK (("UserId" IS NULL) <> ("TeamId" IS NULL))
);

CREATE UNIQUE INDEX "UX_EnvironmentReviewers_User" ON "EnvironmentReviewers" ("EnvironmentId", "UserId")
    WHERE "UserId" IS NOT NULL AND NOT "IsDeleted";
CREATE UNIQUE INDEX "UX_EnvironmentReviewers_Team" ON "EnvironmentReviewers" ("EnvironmentId", "TeamId")
    WHERE "TeamId" IS NOT NULL AND NOT "IsDeleted";

-- +goose Down
DROP TABLE "EnvironmentReviewers";
DROP TABLE "Environments";
