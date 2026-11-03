-- dhow:phase expand
-- +goose Up
CREATE TABLE "Labels" (
    "Id"        uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt" timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt" timestamptz NOT NULL DEFAULT now(),
    "IsDeleted" boolean NOT NULL DEFAULT false,
    "Name"      TEXT NOT NULL
);
CREATE UNIQUE INDEX "UX_Labels_Name" ON "Labels" ("Name") WHERE NOT "IsDeleted";

-- +goose Down
DROP TABLE "Labels";
