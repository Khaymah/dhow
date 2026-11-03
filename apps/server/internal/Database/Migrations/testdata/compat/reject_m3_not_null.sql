-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" ADD COLUMN "Priority" integer NOT NULL;

-- +goose Down
ALTER TABLE "Jobs" DROP COLUMN "Priority";
