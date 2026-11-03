-- dhow:phase contract
-- dhow:floor 0.0.0
-- +goose Up
ALTER TABLE "Jobs" DROP COLUMN "Note";
DROP FUNCTION "EnsureLogPartitions"(date, integer);
ALTER TABLE "Jobs" ALTER COLUMN "Attempts" SET NOT NULL;

-- +goose Down
ALTER TABLE "Jobs" ADD COLUMN "Note" TEXT;
