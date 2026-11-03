-- dhow:phase expand
-- +goose Up
CREATE TABLE IF NOT EXISTS "Labels" (
    "Id"        uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt" timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt" timestamptz NOT NULL DEFAULT now(),
    "IsDeleted" boolean NOT NULL DEFAULT false,
    "Name"      TEXT NOT NULL
);
ALTER TABLE "Labels" ADD CONSTRAINT "CK_Labels_Name" CHECK (char_length("Name") <= 64);

-- +goose Down
DROP TABLE "Labels";
