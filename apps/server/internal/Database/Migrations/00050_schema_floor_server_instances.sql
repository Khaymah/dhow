-- dhow:phase expand
-- +goose Up
ALTER TABLE "Instance" ADD COLUMN "SchemaFloor" TEXT NOT NULL DEFAULT '0.0.0';

CREATE TABLE "ServerInstances" (
    "Id"            uuid        PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"     timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"     timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"     boolean     NOT NULL DEFAULT false,
    "Hostname"      TEXT        NOT NULL,
    "Version"       TEXT        NOT NULL,
    "Commit"        TEXT        NOT NULL DEFAULT '',
    "SchemaVersion" bigint      NOT NULL,
    "StartedAt"     timestamptz NOT NULL DEFAULT now(),
    "LastSeenAt"    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX "IX_ServerInstances_LastSeenAt" ON "ServerInstances" ("LastSeenAt") WHERE NOT "IsDeleted";

-- +goose Down
DROP TABLE "ServerInstances";
ALTER TABLE "Instance" DROP COLUMN "SchemaFloor";
