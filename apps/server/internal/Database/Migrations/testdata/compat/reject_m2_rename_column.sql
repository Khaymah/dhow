-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" RENAME COLUMN "Note" TO "Notes";

-- +goose Down
ALTER TABLE "Jobs" RENAME COLUMN "Notes" TO "Note";
