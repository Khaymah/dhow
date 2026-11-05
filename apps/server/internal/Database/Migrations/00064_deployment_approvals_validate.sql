-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
ALTER TABLE "PipelineRuns" VALIDATE CONSTRAINT "CK_PipelineRuns_Round";
ALTER TABLE "UserIdentities" VALIDATE CONSTRAINT "CK_UserIdentities_Login";

-- +goose Down
