-- dhow:phase contract
-- dhow:floor 1.0.0
-- +goose Up
DROP TABLE "Notes";
UPDATE "Instance" SET "SchemaFloor" = '1.0.0';

-- +goose Down
SELECT 1;
