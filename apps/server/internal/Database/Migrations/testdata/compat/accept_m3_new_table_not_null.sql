-- dhow:phase expand
-- +goose Up
CREATE TABLE "Labels" (
    "Id"        uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt" timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt" timestamptz NOT NULL DEFAULT now(),
    "IsDeleted" boolean NOT NULL DEFAULT false
);
ALTER TABLE "Labels" ADD COLUMN "Name" TEXT NOT NULL;

-- +goose Down
DROP TABLE "Labels";
