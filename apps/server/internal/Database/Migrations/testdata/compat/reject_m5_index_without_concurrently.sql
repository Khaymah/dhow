-- dhow:phase expand
-- +goose Up
CREATE INDEX "IX_Jobs_Note" ON "Jobs" ("Note");

-- +goose Down
DROP INDEX "IX_Jobs_Note";
