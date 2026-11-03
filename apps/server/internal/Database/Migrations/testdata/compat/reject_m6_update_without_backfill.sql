-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" ADD COLUMN "Note" TEXT;
UPDATE "Jobs" j SET "Note" = '' WHERE j."Note" IS NULL;

-- +goose Down
ALTER TABLE "Jobs" DROP COLUMN "Note";
