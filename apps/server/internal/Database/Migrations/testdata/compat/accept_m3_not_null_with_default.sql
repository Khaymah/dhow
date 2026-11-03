-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" ADD COLUMN "Priority" integer NOT NULL DEFAULT 0 CHECK ("Priority" IS NOT NULL);
ALTER TABLE "Jobs" ADD COLUMN IF NOT EXISTS "Tags" TEXT[] CHECK ("Tags" IS NOT NULL OR true);

-- +goose Down
ALTER TABLE "Jobs" DROP COLUMN "Tags";
ALTER TABLE "Jobs" DROP COLUMN "Priority";
