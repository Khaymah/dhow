-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" DROP CONSTRAINT "Jobs_check1";
ALTER TABLE "Jobs" ALTER COLUMN "CreatedByUserId" DROP NOT NULL;
CREATE FUNCTION "Scratch"() RETURNS integer LANGUAGE sql AS 'SELECT 1';
DROP FUNCTION "Scratch"();

-- +goose Down
SELECT 1;
