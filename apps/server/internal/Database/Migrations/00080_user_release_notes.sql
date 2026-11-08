-- dhow:phase expand
-- +goose Up
CREATE TABLE "UserReleaseNotes" (
    "Id"          uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"   timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"   timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"   boolean NOT NULL DEFAULT false,
    "UserId"      uuid NOT NULL REFERENCES "Users" ("Id"),
    "SeenVersion" TEXT NOT NULL CHECK ("SeenVersion" ~ '^[0-9]{1,6}\.[0-9]{1,6}\.[0-9]{1,6}(-rc\.[0-9]{1,6})?$')
);

CREATE UNIQUE INDEX "UX_UserReleaseNotes_User" ON "UserReleaseNotes" ("UserId") WHERE NOT "IsDeleted";

-- +goose Down
DROP TABLE "UserReleaseNotes";
