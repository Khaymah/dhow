-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" ADD COLUMN "Notes" TEXT, ADD CONSTRAINT "CK_Jobs_Notes" CHECK ("Notes" IN ('a', 'b')) NOT VALID, DROP "Note";

-- +goose Down
ALTER TABLE "Jobs" DROP COLUMN "Notes";
