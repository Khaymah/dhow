-- dhow:phase expand
-- +goose Up
CREATE TABLE "NotificationChannels" (
    "Id"                  uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"           timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"           timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"           boolean NOT NULL DEFAULT false,
    "OrganizationId"      uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "ProjectId"           uuid,
    "Kind"                TEXT NOT NULL CHECK ("Kind" IN ('email', 'webhook', 'slack', 'telegram', 'discord', 'inapp')),
    "Name"                TEXT NOT NULL CHECK (char_length("Name") BETWEEN 1 AND 64),
    "Locale"              TEXT NOT NULL DEFAULT 'ar' CHECK ("Locale" IN ('ar', 'en')),
    "Enabled"             boolean NOT NULL DEFAULT true,
    "DisabledReason"      TEXT CHECK ("DisabledReason" IN ('creator-removed', 'failures')),
    "ConsecutiveFailures" integer NOT NULL DEFAULT 0 CHECK ("ConsecutiveFailures" >= 0),
    "Host"                TEXT NOT NULL DEFAULT '' CHECK (char_length("Host") <= 255),
    "ConfigSealed"        bytea CHECK (octet_length("ConfigSealed") BETWEEN 1 AND 8192 AND get_byte("ConfigSealed", 0) = 3),
    "Version"             bigint NOT NULL DEFAULT 1 CHECK ("Version" >= 1),
    "CreatedByUserId"     uuid NOT NULL REFERENCES "Users" ("Id"),
    FOREIGN KEY ("ProjectId", "OrganizationId") REFERENCES "Projects" ("Id", "OrganizationId"),
    CHECK ("Enabled" = ("DisabledReason" IS NULL)),
    CHECK ("Kind" IN ('email', 'inapp') OR "ConfigSealed" IS NOT NULL)
);

CREATE UNIQUE INDEX "UX_NotificationChannels_Scope_Name" ON "NotificationChannels" ("OrganizationId", "ProjectId", lower("Name"))
    NULLS NOT DISTINCT WHERE NOT "IsDeleted";
CREATE UNIQUE INDEX "UX_NotificationChannels_Id_Organization" ON "NotificationChannels" ("Id", "OrganizationId");
CREATE INDEX "IX_NotificationChannels_Project" ON "NotificationChannels" ("ProjectId") WHERE NOT "IsDeleted" AND "ProjectId" IS NOT NULL;
CREATE INDEX "IX_NotificationChannels_Creator" ON "NotificationChannels" ("CreatedByUserId") WHERE NOT "IsDeleted";

CREATE TABLE "NotificationRules" (
    "Id"             uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"      timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"      timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"      boolean NOT NULL DEFAULT false,
    "OrganizationId" uuid NOT NULL,
    "ProjectId"      uuid,
    "ChannelId"      uuid NOT NULL,
    "Events"         TEXT[] NOT NULL CHECK (cardinality("Events") BETWEEN 1 AND 16),
    "Environments"   TEXT[] NOT NULL DEFAULT '{}' CHECK (cardinality("Environments") <= 50),
    "Statuses"       TEXT[] NOT NULL CHECK (cardinality("Statuses") BETWEEN 1 AND 16),
    "Roles"          TEXT[] NOT NULL DEFAULT '{}' CHECK (cardinality("Roles") <= 5),
    "Version"        bigint NOT NULL DEFAULT 1 CHECK ("Version" >= 1),
    FOREIGN KEY ("ChannelId", "OrganizationId") REFERENCES "NotificationChannels" ("Id", "OrganizationId")
);

CREATE UNIQUE INDEX "UX_NotificationRules_Channel" ON "NotificationRules" ("ChannelId") WHERE NOT "IsDeleted";
CREATE INDEX "IX_NotificationRules_Scope" ON "NotificationRules" ("OrganizationId", "ProjectId") WHERE NOT "IsDeleted";

CREATE TABLE "NotificationOutbox" (
    "Id"              uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"       timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"       timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"       boolean NOT NULL DEFAULT false,
    "OrganizationId"  uuid REFERENCES "Organizations" ("Id"),
    "ProjectId"       uuid,
    "ChannelId"       uuid,
    "RecipientUserId" uuid REFERENCES "Users" ("Id"),
    "Kind"            TEXT NOT NULL CHECK ("Kind" IN ('email', 'webhook', 'slack', 'telegram', 'discord', 'inapp', 'internal-email')),
    "EventId"         uuid NOT NULL,
    "EventName"       TEXT NOT NULL CHECK (char_length("EventName") BETWEEN 1 AND 128),
    "Payload"         jsonb NOT NULL CHECK (jsonb_typeof("Payload") = 'object' AND octet_length("Payload"::text) <= 16384),
    "State"           TEXT NOT NULL DEFAULT 'pending' CHECK ("State" IN ('pending', 'sending', 'sent', 'dead')),
    "Attempts"        integer NOT NULL DEFAULT 0 CHECK ("Attempts" >= 0),
    "NextAttemptAt"   timestamptz NOT NULL DEFAULT now(),
    "ClaimedUntil"    timestamptz,
    "LastAttemptAt"   timestamptz,
    "LastError"       TEXT NOT NULL DEFAULT '' CHECK (char_length("LastError") <= 64),
    "LastStatus"      integer CHECK ("LastStatus" BETWEEN 100 AND 599),
    "SentAt"          timestamptz,
    FOREIGN KEY ("ChannelId", "OrganizationId") REFERENCES "NotificationChannels" ("Id", "OrganizationId"),
    CHECK ("OrganizationId" IS NOT NULL OR "Kind" = 'internal-email'),
    CHECK ("ChannelId" IS NOT NULL OR "Kind" = 'internal-email'),
    CHECK ("Kind" <> 'internal-email' OR "RecipientUserId" IS NOT NULL)
);

CREATE UNIQUE INDEX "UX_NotificationOutbox_Event_Channel_Recipient" ON "NotificationOutbox" ("EventId", "ChannelId", "RecipientUserId") NULLS NOT DISTINCT;
CREATE INDEX "IX_NotificationOutbox_Due" ON "NotificationOutbox" ("NextAttemptAt") WHERE "State" = 'pending' AND NOT "IsDeleted";
CREATE INDEX "IX_NotificationOutbox_Claimed" ON "NotificationOutbox" ("ClaimedUntil") WHERE "State" = 'sending' AND NOT "IsDeleted";
CREATE INDEX "IX_NotificationOutbox_Channel" ON "NotificationOutbox" ("ChannelId", "CreatedAt" DESC) WHERE NOT "IsDeleted";
CREATE INDEX "IX_NotificationOutbox_Project" ON "NotificationOutbox" ("ProjectId", "CreatedAt" DESC) WHERE NOT "IsDeleted";
CREATE INDEX "IX_NotificationOutbox_Recipient" ON "NotificationOutbox" ("RecipientUserId") WHERE "RecipientUserId" IS NOT NULL;
CREATE INDEX "IX_NotificationOutbox_Finished" ON "NotificationOutbox" ("UpdatedAt") WHERE "State" IN ('sent', 'dead');

CREATE TABLE "NotificationMutes" (
    "Id"        uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt" timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt" timestamptz NOT NULL DEFAULT now(),
    "IsDeleted" boolean NOT NULL DEFAULT false,
    "UserId"    uuid NOT NULL REFERENCES "Users" ("Id"),
    "Muted"     boolean NOT NULL DEFAULT true
);

CREATE UNIQUE INDEX "UX_NotificationMutes_User" ON "NotificationMutes" ("UserId");

CREATE TABLE "NotificationDevices" (
    "Id"          uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"   timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"   timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"   boolean NOT NULL DEFAULT false,
    "UserId"      uuid NOT NULL REFERENCES "Users" ("Id"),
    "Fingerprint" bytea NOT NULL CHECK (octet_length("Fingerprint") = 32),
    "LastSeenAt"  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX "UX_NotificationDevices_User_Fingerprint" ON "NotificationDevices" ("UserId", "Fingerprint");

CREATE TABLE "NotificationRunnerOutages" (
    "Id"             uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"      timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"      timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"      boolean NOT NULL DEFAULT false,
    "OrganizationId" uuid NOT NULL REFERENCES "Organizations" ("Id"),
    "RunnerId"       uuid NOT NULL REFERENCES "Runners" ("Id"),
    "ResolvedAt"     timestamptz
);

CREATE UNIQUE INDEX "UX_NotificationRunnerOutages_Open" ON "NotificationRunnerOutages" ("RunnerId") WHERE "ResolvedAt" IS NULL;
CREATE INDEX "IX_NotificationRunnerOutages_Organization" ON "NotificationRunnerOutages" ("OrganizationId");

-- +goose Down
DROP TABLE "NotificationRunnerOutages";
DROP TABLE "NotificationDevices";
DROP TABLE "NotificationMutes";
DROP TABLE "NotificationOutbox";
DROP TABLE "NotificationRules";
DROP TABLE "NotificationChannels";
