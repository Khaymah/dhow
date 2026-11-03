-- dhow:phase contract
-- dhow:floor 0.0.0
-- +goose Up
ALTER TABLE "Jobs"
    ADD FOREIGN KEY ("ProjectId") REFERENCES "Projects" ("Id");

-- +goose Down
SELECT 1;
