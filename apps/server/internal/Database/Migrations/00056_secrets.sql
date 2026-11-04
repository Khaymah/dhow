-- dhow:phase expand
-- +goose Up
CREATE TABLE "Secrets" (
    "Id"                      uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"               timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"               timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"               boolean NOT NULL DEFAULT false,
    "OrganizationId"          uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "ProjectId"               uuid,
    "EnvironmentId"           uuid,
    "Scope"                   TEXT NOT NULL CHECK ("Scope" IN ('organization', 'project', 'environment')),
    "Name"                    TEXT NOT NULL CHECK ("Name" ~ '^[A-Za-z_][A-Za-z0-9_]{0,63}$' AND upper("Name") NOT LIKE 'DHOW\_%'),
    "Description"             TEXT NOT NULL DEFAULT '' CHECK (char_length("Description") <= 256),
    "CurrentVersion"          integer NOT NULL DEFAULT 1 CHECK ("CurrentVersion" >= 1),
    "Visibility"              TEXT NOT NULL DEFAULT 'all_projects' CHECK ("Visibility" IN ('all_projects', 'selected_projects')),
    "AvailableToPullRequests" boolean NOT NULL DEFAULT false,
    "CreatedByUserId"         uuid NOT NULL REFERENCES "Users" ("Id"),
    "UpdatedByUserId"         uuid NOT NULL REFERENCES "Users" ("Id"),
    "LastUsedAt"              timestamptz,
    FOREIGN KEY ("ProjectId", "OrganizationId") REFERENCES "Projects" ("Id", "OrganizationId"),
    FOREIGN KEY ("EnvironmentId", "OrganizationId") REFERENCES "Environments" ("Id", "OrganizationId"),
    CHECK (
        ("Scope" = 'organization' AND "ProjectId" IS NULL AND "EnvironmentId" IS NULL)
        OR ("Scope" = 'project' AND "ProjectId" IS NOT NULL AND "EnvironmentId" IS NULL)
        OR ("Scope" = 'environment' AND "ProjectId" IS NOT NULL AND "EnvironmentId" IS NOT NULL)
    ),
    CHECK ("Scope" = 'organization' OR "Visibility" = 'all_projects'),
    CHECK ("Scope" <> 'environment' OR NOT "AvailableToPullRequests")
);

CREATE UNIQUE INDEX "UX_Secrets_Scope_Name" ON "Secrets" ("OrganizationId", "ProjectId", "EnvironmentId", lower("Name"))
    NULLS NOT DISTINCT WHERE NOT "IsDeleted";
CREATE UNIQUE INDEX "UX_Secrets_Id_Organization" ON "Secrets" ("Id", "OrganizationId");
CREATE INDEX "IX_Secrets_Project" ON "Secrets" ("ProjectId", lower("Name")) WHERE NOT "IsDeleted" AND "ProjectId" IS NOT NULL;
CREATE INDEX "IX_Secrets_Environment" ON "Secrets" ("EnvironmentId") WHERE NOT "IsDeleted" AND "EnvironmentId" IS NOT NULL;

CREATE TABLE "SecretVersions" (
    "Id"              uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"       timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"       timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"       boolean NOT NULL DEFAULT false,
    "OrganizationId"  uuid NOT NULL,
    "SecretId"        uuid NOT NULL,
    "Version"         integer NOT NULL CHECK ("Version" >= 1),
    "ValueSealed"     bytea CHECK (octet_length("ValueSealed") BETWEEN 1 AND 33024),
    "SizeBytes"       integer NOT NULL CHECK ("SizeBytes" BETWEEN 8 AND 32768),
    "CreatedByUserId" uuid NOT NULL REFERENCES "Users" ("Id"),
    "SupersededAt"    timestamptz,
    "ErasedAt"        timestamptz,
    FOREIGN KEY ("SecretId", "OrganizationId") REFERENCES "Secrets" ("Id", "OrganizationId"),
    CHECK (("ValueSealed" IS NULL) = ("ErasedAt" IS NOT NULL))
);

CREATE UNIQUE INDEX "UX_SecretVersions_Secret_Version" ON "SecretVersions" ("SecretId", "Version");
CREATE INDEX "IX_SecretVersions_Superseded" ON "SecretVersions" ("SupersededAt") WHERE "ValueSealed" IS NOT NULL AND "SupersededAt" IS NOT NULL;

CREATE TABLE "SecretProjectAccess" (
    "Id"             uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"      timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"      timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"      boolean NOT NULL DEFAULT false,
    "OrganizationId" uuid NOT NULL,
    "SecretId"       uuid NOT NULL,
    "ProjectId"      uuid NOT NULL,
    FOREIGN KEY ("SecretId", "OrganizationId") REFERENCES "Secrets" ("Id", "OrganizationId"),
    FOREIGN KEY ("ProjectId", "OrganizationId") REFERENCES "Projects" ("Id", "OrganizationId")
);

CREATE UNIQUE INDEX "UX_SecretProjectAccess_Secret_Project" ON "SecretProjectAccess" ("SecretId", "ProjectId") WHERE NOT "IsDeleted";
CREATE INDEX "IX_SecretProjectAccess_Project" ON "SecretProjectAccess" ("ProjectId") WHERE NOT "IsDeleted";

CREATE TABLE "Variables" (
    "Id"              uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"       timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"       timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"       boolean NOT NULL DEFAULT false,
    "OrganizationId"  uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "ProjectId"       uuid,
    "EnvironmentId"   uuid,
    "Scope"           TEXT NOT NULL CHECK ("Scope" IN ('organization', 'project', 'environment')),
    "Name"            TEXT NOT NULL CHECK ("Name" ~ '^[A-Za-z_][A-Za-z0-9_]{0,63}$' AND upper("Name") NOT LIKE 'DHOW\_%'),
    "Value"           TEXT NOT NULL DEFAULT '' CHECK (octet_length("Value") <= 4096),
    "CreatedByUserId" uuid NOT NULL REFERENCES "Users" ("Id"),
    "UpdatedByUserId" uuid NOT NULL REFERENCES "Users" ("Id"),
    FOREIGN KEY ("ProjectId", "OrganizationId") REFERENCES "Projects" ("Id", "OrganizationId"),
    FOREIGN KEY ("EnvironmentId", "OrganizationId") REFERENCES "Environments" ("Id", "OrganizationId"),
    CHECK (
        ("Scope" = 'organization' AND "ProjectId" IS NULL AND "EnvironmentId" IS NULL)
        OR ("Scope" = 'project' AND "ProjectId" IS NOT NULL AND "EnvironmentId" IS NULL)
        OR ("Scope" = 'environment' AND "ProjectId" IS NOT NULL AND "EnvironmentId" IS NOT NULL)
    )
);

CREATE UNIQUE INDEX "UX_Variables_Scope_Name" ON "Variables" ("OrganizationId", "ProjectId", "EnvironmentId", lower("Name"))
    NULLS NOT DISTINCT WHERE NOT "IsDeleted";
CREATE INDEX "IX_Variables_Project" ON "Variables" ("ProjectId") WHERE NOT "IsDeleted" AND "ProjectId" IS NOT NULL;

CREATE TABLE "RegistryCredentials" (
    "Id"              uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"       timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"       timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"       boolean NOT NULL DEFAULT false,
    "OrganizationId"  uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "ProjectId"       uuid,
    "Host"            TEXT NOT NULL CHECK ("Host" ~ '^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?(:[0-9]{1,5})?$'),
    "Username"        TEXT NOT NULL CHECK (char_length("Username") BETWEEN 1 AND 256),
    "PasswordSealed"  bytea CHECK (octet_length("PasswordSealed") BETWEEN 1 AND 4352),
    "CreatedByUserId" uuid NOT NULL REFERENCES "Users" ("Id"),
    "UpdatedByUserId" uuid NOT NULL REFERENCES "Users" ("Id"),
    FOREIGN KEY ("ProjectId", "OrganizationId") REFERENCES "Projects" ("Id", "OrganizationId"),
    CHECK ("IsDeleted" OR "PasswordSealed" IS NOT NULL)
);

CREATE UNIQUE INDEX "UX_RegistryCredentials_Scope_Host" ON "RegistryCredentials" ("OrganizationId", "ProjectId", "Host")
    NULLS NOT DISTINCT WHERE NOT "IsDeleted";

-- +goose Down
DROP TABLE "RegistryCredentials";
DROP TABLE "Variables";
DROP TABLE "SecretProjectAccess";
DROP TABLE "SecretVersions";
DROP TABLE "Secrets";
