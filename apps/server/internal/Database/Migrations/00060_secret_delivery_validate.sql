-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
ALTER TABLE "RunJobs" VALIDATE CONSTRAINT "CK_RunJobs_FailureDetail";
ALTER TABLE "Projects" VALIDATE CONSTRAINT "CK_Projects_ForkCacheRestore";
ALTER TABLE "Jobs" VALIDATE CONSTRAINT "CK_Jobs_RequiredFeatures";
ALTER TABLE "Runners" VALIDATE CONSTRAINT "CK_Runners_Features";

-- +goose Down
