-- dhow:phase expand
-- +goose Up
CREATE TABLE "BlobOwners" (
    "Id"             uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"      timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"      timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"      boolean NOT NULL DEFAULT false,
    "OrganizationId" uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "Digest"         bytea NOT NULL CHECK (octet_length("Digest") = 32)
);

CREATE UNIQUE INDEX "UX_BlobOwners_Organization_Digest" ON "BlobOwners" ("OrganizationId", "Digest") WHERE NOT "IsDeleted";
CREATE INDEX "IX_BlobOwners_Digest" ON "BlobOwners" ("Digest") WHERE NOT "IsDeleted";

CREATE TABLE "BlobUploads" (
    "Id"             uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"      timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"      timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"      boolean NOT NULL DEFAULT false,
    "OrganizationId" uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "LeaseId"        uuid NOT NULL,
    "Kind"           TEXT NOT NULL CHECK ("Kind" IN ('artifact', 'cache')),
    "Name"           TEXT NOT NULL CHECK (char_length("Name") <= 64),
    "State"          TEXT NOT NULL DEFAULT 'uploading' CHECK ("State" IN ('uploading', 'failed', 'stored')),
    "Digest"         bytea CHECK (octet_length("Digest") = 32)
);

CREATE UNIQUE INDEX "UX_BlobUploads_Lease_Kind_Name" ON "BlobUploads" ("LeaseId", "Kind", "Name");

-- +goose Down
DROP TABLE "BlobUploads";
DROP TABLE "BlobOwners";
