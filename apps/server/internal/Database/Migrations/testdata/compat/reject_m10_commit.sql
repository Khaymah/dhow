-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" ADD COLUMN "Note" TEXT;
COMMIT;
ALTER TABLE "Jobs" ADD COLUMN "MoreNote" TEXT;

-- +goose Down
ALTER TABLE "Jobs" DROP COLUMN "MoreNote";
ALTER TABLE "Jobs" DROP COLUMN "Note";
