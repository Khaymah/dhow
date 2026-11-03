-- dhow:phase expand
-- +goose Up
SAVEPOINT "BeforeNotes";
ALTER TABLE "Jobs" ADD COLUMN "Note" TEXT;
RELEASE SAVEPOINT "BeforeNotes";

-- +goose Down
ALTER TABLE "Jobs" DROP COLUMN "Note";
