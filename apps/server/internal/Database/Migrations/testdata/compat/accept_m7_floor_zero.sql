-- dhow:phase contract
-- dhow:floor 0.0.0
-- +goose Up
ALTER TABLE "Jobs" DROP COLUMN "Note";

-- +goose Down
ALTER TABLE "Jobs" ADD COLUMN "Note" TEXT;
