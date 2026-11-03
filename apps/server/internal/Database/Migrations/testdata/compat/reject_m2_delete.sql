-- dhow:phase expand
-- +goose Up
DELETE FROM "Sessions" WHERE "IsDeleted";

-- +goose Down
SELECT 1;
