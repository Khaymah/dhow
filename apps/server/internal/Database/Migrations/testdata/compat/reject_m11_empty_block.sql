-- dhow:phase expand
-- +goose Up
-- +goose StatementBegin
-- +goose StatementEnd
ALTER TABLE "Jobs" ADD COLUMN "Note" TEXT;

-- +goose Down
ALTER TABLE "Jobs" DROP COLUMN "Note";
