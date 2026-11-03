-- dhow:phase expand
-- +goose Up
-- +goose StatementBegin
ALTER TABLE "Jobs" ADD COLUMN "Note" TEXT; ROLLBACK; ALTER TABLE "Jobs" ADD COLUMN "MoreNote" TEXT;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE "Jobs" DROP COLUMN "Note";
