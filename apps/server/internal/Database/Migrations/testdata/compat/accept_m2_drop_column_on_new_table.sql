-- dhow:phase expand
-- +goose Up
CREATE TABLE "Notes" (
    "Id"        uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt" timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt" timestamptz NOT NULL DEFAULT now(),
    "IsDeleted" boolean NOT NULL DEFAULT false,
    "Body"      TEXT,
    "Draft"     TEXT
);

ALTER TABLE "Notes" DROP COLUMN "Draft";
ALTER TABLE "Notes" RENAME COLUMN "Body" TO "Text";
ALTER TABLE "Notes" ALTER COLUMN "Text" SET NOT NULL;
CREATE INDEX "IX_Notes_Text" ON "Notes" ("Text");
UPDATE "Notes" SET "Text" = '';
DELETE FROM "Notes";
DROP TABLE "Notes";

-- +goose Down
SELECT 1;
