-- dhow:phase expand
-- +goose Up
CREATE TABLE "NotificationInbox" (
    "Id"             uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"      timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"      timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"      boolean NOT NULL DEFAULT false,
    "UserId"         uuid NOT NULL REFERENCES "Users" ("Id"),
    "OrganizationId" uuid REFERENCES "Organizations" ("Id"),
    "EventId"        uuid NOT NULL,
    "EventName"      TEXT NOT NULL CHECK (char_length("EventName") BETWEEN 1 AND 128),
    "TitleKey"       TEXT NOT NULL CHECK ("TitleKey" ~ '^[a-z][a-zA-Z0-9_.]{0,127}$'),
    "Params"         jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof("Params") = 'object' AND octet_length("Params"::text) <= 2048),
    "Link"           TEXT NOT NULL DEFAULT '' CHECK (char_length("Link") <= 512 AND ("Link" = '' OR ("Link" LIKE '/%' AND "Link" NOT LIKE '//%'))),
    "IsAccount"      boolean NOT NULL DEFAULT false,
    "IsRead"         boolean NOT NULL DEFAULT false,
    "ReadAt"         timestamptz,
    CHECK ("IsRead" = ("ReadAt" IS NOT NULL))
);

CREATE UNIQUE INDEX "UX_NotificationInbox_User_Event" ON "NotificationInbox" ("UserId", "EventId");
CREATE INDEX "IX_NotificationInbox_User" ON "NotificationInbox" ("UserId", "CreatedAt" DESC, "Id" DESC) WHERE NOT "IsDeleted";
CREATE INDEX "IX_NotificationInbox_Unread" ON "NotificationInbox" ("UserId") WHERE NOT "IsDeleted" AND NOT "IsRead";
CREATE INDEX "IX_NotificationInbox_Created" ON "NotificationInbox" ("CreatedAt") WHERE NOT "IsDeleted";

-- +goose Down
DROP TABLE "NotificationInbox";
