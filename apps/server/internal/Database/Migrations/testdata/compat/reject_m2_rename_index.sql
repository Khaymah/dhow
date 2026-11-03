-- dhow:phase expand
-- +goose Up
ALTER INDEX "IX_Jobs_Note" RENAME TO "IX_Jobs_Notes";

-- +goose Down
ALTER INDEX "IX_Jobs_Notes" RENAME TO "IX_Jobs_Note";
