-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" ADD COLUMN "Note" TEXT;
PREPARE TRANSACTION 'notes';

-- +goose Down
ALTER TABLE "Jobs" DROP COLUMN "Note";
