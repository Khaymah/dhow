-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" ADD COLUMN "Note" TEXT;
end;

-- +goose Down
ALTER TABLE "Jobs" DROP COLUMN "Note";
