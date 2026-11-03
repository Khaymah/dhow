-- dhow:phase expand
-- dhow:backfill small
-- +goose Up
ALTER TABLE "InstanceSettings" ADD COLUMN "Note" TEXT;
UPDATE "InstanceSettings" SET "Note" = '';

-- +goose Down
ALTER TABLE "InstanceSettings" DROP COLUMN "Note";
