-- dhow:phase contract
-- dhow:floor 1.0.1
-- +goose Up
ALTER TABLE "Jobs" DROP COLUMN "Note";
UPDATE "Instance" SET "SchemaFloor" = '1.0.1';

-- +goose Down
ALTER TABLE "Jobs" ADD COLUMN "Note" TEXT;
