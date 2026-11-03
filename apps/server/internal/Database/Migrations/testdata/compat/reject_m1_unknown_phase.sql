-- dhow:phase migrate
-- +goose Up
ALTER TABLE "Jobs" ADD COLUMN "Note" TEXT;

-- +goose Down
ALTER TABLE "Jobs" DROP COLUMN "Note";
