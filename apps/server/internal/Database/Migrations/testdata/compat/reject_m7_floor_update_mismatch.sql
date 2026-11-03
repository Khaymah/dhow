-- dhow:phase contract
-- dhow:floor 1.0.0
-- +goose Up
ALTER TABLE "Jobs" DROP COLUMN "Note";
UPDATE "Instance" SET "SchemaFloor" = '0.9.0';

-- +goose Down
ALTER TABLE "Jobs" ADD COLUMN "Note" TEXT;
