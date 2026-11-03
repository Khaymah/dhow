-- dhow:phase contract
-- +goose Up
ALTER TABLE "Jobs" DROP COLUMN "Note";

-- +goose Down
ALTER TABLE "Jobs" ADD COLUMN "Note" TEXT;
