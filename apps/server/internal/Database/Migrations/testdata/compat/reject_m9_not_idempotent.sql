-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" VALIDATE CONSTRAINT "CK_Jobs_Attempts";
ALTER TABLE "Jobs" ADD COLUMN "Note" TEXT;

-- +goose Down
ALTER TABLE "Jobs" DROP COLUMN IF EXISTS "Note";
