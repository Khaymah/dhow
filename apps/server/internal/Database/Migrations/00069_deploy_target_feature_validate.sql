-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" VALIDATE CONSTRAINT "CK_Jobs_RequiredFeatures";
ALTER TABLE "Runners" VALIDATE CONSTRAINT "CK_Runners_Features";

-- +goose Down
