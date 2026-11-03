-- dhow:phase expand
-- +goose Up
-- +goose StatementBegin
-- the body moved to the next file

-- +goose StatementEnd
ALTER TABLE "Jobs" ADD COLUMN "Note" TEXT;

-- +goose Down
ALTER TABLE "Jobs" DROP COLUMN "Note";
