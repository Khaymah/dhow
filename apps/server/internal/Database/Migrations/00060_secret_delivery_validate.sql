-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
ALTER TABLE "RunJobs" VALIDATE CONSTRAINT "CK_RunJobs_FailureDetail";
ALTER TABLE "Projects" VALIDATE CONSTRAINT "CK_Projects_ForkCacheRestore";

-- +goose Down
