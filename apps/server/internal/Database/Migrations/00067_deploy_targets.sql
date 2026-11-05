-- dhow:phase expand
-- +goose Up
ALTER TABLE "Environments" ADD COLUMN "AllowExtraDeployEgress" boolean NOT NULL DEFAULT false;

CREATE TABLE "DeployTargets" (
    "Id"             uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"      timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"      timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"      boolean NOT NULL DEFAULT false,
    "OrganizationId" uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "ProjectId"      uuid NOT NULL,
    "EnvironmentId"  uuid NOT NULL,
    "Name"           TEXT NOT NULL CHECK ("Name" ~ '^[a-z0-9][a-z0-9_-]{0,63}$'),
    "Kind"           TEXT NOT NULL CHECK ("Kind" IN ('ssh', 'compose')),
    "Username"       TEXT NOT NULL CHECK ("Username" ~ '^[a-z_][a-z0-9_-]{0,31}$'),
    "KeySecretName"  TEXT CHECK ("KeySecretName" ~ '^[A-Za-z_][A-Za-z0-9_]{0,63}$'),
    "PublicKey"      TEXT CHECK (char_length("PublicKey") BETWEEN 1 AND 2048),
    "DeployPath"     TEXT NOT NULL CHECK ("DeployPath" ~ '^/[A-Za-z0-9._/-]{1,200}$'),
    "KeepReleases"   integer NOT NULL DEFAULT 5 CHECK ("KeepReleases" BETWEEN 2 AND 50),
    "Settings"       jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof("Settings") = 'object' AND octet_length("Settings"::text) <= 4096),
    "AllowedImages"  TEXT[] NOT NULL DEFAULT '{}' CHECK (cardinality("AllowedImages") <= 10),
    "Version"        bigint NOT NULL DEFAULT 1 CHECK ("Version" >= 1),
    "CreatedByUserId" uuid NOT NULL REFERENCES "Users" ("Id"),
    FOREIGN KEY ("ProjectId", "OrganizationId") REFERENCES "Projects" ("Id", "OrganizationId"),
    FOREIGN KEY ("EnvironmentId", "OrganizationId") REFERENCES "Environments" ("Id", "OrganizationId")
);

CREATE UNIQUE INDEX "UX_DeployTargets_Environment_Name" ON "DeployTargets" ("EnvironmentId", "Name") WHERE NOT "IsDeleted";
CREATE UNIQUE INDEX "UX_DeployTargets_Id_Organization" ON "DeployTargets" ("Id", "OrganizationId");
CREATE INDEX "IX_DeployTargets_Environment" ON "DeployTargets" ("EnvironmentId") WHERE NOT "IsDeleted";

CREATE TABLE "DeployTargetHosts" (
    "Id"                 uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"          timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"          timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"          boolean NOT NULL DEFAULT false,
    "OrganizationId"     uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "TargetId"           uuid NOT NULL,
    "Position"           integer NOT NULL CHECK ("Position" BETWEEN 1 AND 1000),
    "Host"               TEXT NOT NULL CHECK (char_length("Host") BETWEEN 1 AND 253),
    "Port"               integer NOT NULL DEFAULT 22 CHECK ("Port" BETWEEN 1 AND 65535),
    "DeployPath"         TEXT NOT NULL CHECK ("DeployPath" ~ '^/[A-Za-z0-9._/-]{1,200}$'),
    "HostKey"            TEXT CHECK (char_length("HostKey") BETWEEN 1 AND 2048),
    "HostKeyFingerprint" TEXT CHECK ("HostKeyFingerprint" ~ '^SHA256:[A-Za-z0-9+/]{43}$'),
    "HostKeyConfirmedAt" timestamptz,
    "HostKeyConfirmedBy" uuid REFERENCES "Users" ("Id"),
    "PendingHostKey"     TEXT CHECK (char_length("PendingHostKey") BETWEEN 1 AND 2048),
    "PendingFingerprint" TEXT CHECK ("PendingFingerprint" ~ '^SHA256:[A-Za-z0-9+/]{43}$'),
    FOREIGN KEY ("TargetId", "OrganizationId") REFERENCES "DeployTargets" ("Id", "OrganizationId"),
    CHECK (("HostKey" IS NULL) = ("HostKeyFingerprint" IS NULL)
           AND ("HostKey" IS NULL) = ("HostKeyConfirmedAt" IS NULL)
           AND ("HostKey" IS NULL) = ("HostKeyConfirmedBy" IS NULL)),
    CHECK (("PendingHostKey" IS NULL) = ("PendingFingerprint" IS NULL))
);

CREATE UNIQUE INDEX "UX_DeployTargetHosts_Target_Position" ON "DeployTargetHosts" ("TargetId", "Position") WHERE NOT "IsDeleted";
CREATE UNIQUE INDEX "UX_DeployTargetHosts_Target_Address" ON "DeployTargetHosts" ("TargetId", "Host", "Port") WHERE NOT "IsDeleted";
CREATE UNIQUE INDEX "UX_DeployTargetHosts_Address_Path" ON "DeployTargetHosts" ("OrganizationId", "Host", "Port", "DeployPath") WHERE NOT "IsDeleted";
CREATE INDEX "IX_DeployTargetHosts_Target" ON "DeployTargetHosts" ("TargetId", "Position") WHERE NOT "IsDeleted";

-- +goose Down
DROP TABLE "DeployTargetHosts";
DROP TABLE "DeployTargets";
ALTER TABLE "Environments" DROP COLUMN "AllowExtraDeployEgress";
