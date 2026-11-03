-- dhow:phase expand
-- +goose Up
CREATE TABLE "KeyEncryptionKeys" (
    "Id"             uuid        PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"      timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"      timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"      boolean     NOT NULL DEFAULT false,
    "Version"        integer     NOT NULL CHECK ("Version" >= 1),
    "CheckValue"     bytea       NOT NULL CHECK (octet_length("CheckValue") = 32),
    "Provider"       TEXT        NOT NULL DEFAULT 'file' CHECK ("Provider" ~ '^[a-z][a-z0-9-]{0,31}$'),
    "State"          TEXT        NOT NULL CHECK ("State" IN ('pending', 'active', 'retiring', 'retired')),
    "ActivatedAt"    timestamptz,
    "RetiredAt"      timestamptz,
    "LastExportedAt" timestamptz,
    CHECK (("State" = 'retired') = ("RetiredAt" IS NOT NULL)),
    CHECK ("State" = 'pending' OR "ActivatedAt" IS NOT NULL)
);

CREATE UNIQUE INDEX "UX_KeyEncryptionKeys_Version" ON "KeyEncryptionKeys" ("Version");
CREATE UNIQUE INDEX "UX_KeyEncryptionKeys_Active" ON "KeyEncryptionKeys" ((true)) WHERE "State" = 'active';

CREATE TABLE "DataKeys" (
    "Id"          uuid        PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"   timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"   timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"   boolean     NOT NULL DEFAULT false,
    "OwnerKind"   TEXT        NOT NULL CHECK ("OwnerKind" IN ('instance', 'organization', 'project', 'environment', 'user')),
    "OwnerId"     uuid,
    "KekVersion"  integer     NOT NULL REFERENCES "KeyEncryptionKeys" ("Version"),
    "Wrapped"     bytea       CHECK ("Wrapped" IS NULL OR octet_length("Wrapped") = 60),
    "State"       TEXT        NOT NULL DEFAULT 'active' CHECK ("State" IN ('active', 'destroyed')),
    "DestroyedAt" timestamptz,
    "UnwrapFailedAt" timestamptz,
    CHECK (("OwnerKind" = 'instance') = ("OwnerId" IS NULL)),
    CHECK (("State" = 'destroyed') = ("Wrapped" IS NULL)),
    CHECK (("State" = 'destroyed') = ("DestroyedAt" IS NOT NULL))
);

CREATE UNIQUE INDEX "UX_DataKeys_Owner" ON "DataKeys" ("OwnerKind", "OwnerId") NULLS NOT DISTINCT WHERE "State" = 'active';
CREATE INDEX "IX_DataKeys_KekVersion" ON "DataKeys" ("KekVersion") WHERE "State" = 'active';

ALTER TABLE "ServerInstances" ADD COLUMN "KekVersions" integer[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE "ServerInstances" DROP COLUMN "KekVersions";
DROP TABLE "DataKeys";
DROP TABLE "KeyEncryptionKeys";
