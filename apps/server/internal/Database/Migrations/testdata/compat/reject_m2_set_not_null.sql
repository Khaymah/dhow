-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" ALTER COLUMN "Note" SET NOT NULL;

-- +goose Down
ALTER TABLE "Jobs" ALTER COLUMN "Note" DROP NOT NULL;
