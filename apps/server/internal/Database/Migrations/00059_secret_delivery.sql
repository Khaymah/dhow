-- dhow:phase expand
-- +goose Up
ALTER TABLE "RunJobs" ADD COLUMN "FailureDetail" jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE "RunJobs" ADD CONSTRAINT "CK_RunJobs_FailureDetail"
    CHECK (jsonb_typeof("FailureDetail") = 'object' AND octet_length("FailureDetail"::text) <= 1024) NOT VALID;

ALTER TABLE "Projects" ADD COLUMN "ForkCacheRestore" TEXT NOT NULL DEFAULT 'none';
ALTER TABLE "Projects" ADD CONSTRAINT "CK_Projects_ForkCacheRestore"
    CHECK ("ForkCacheRestore" IN ('none', 'default_branch')) NOT VALID;

ALTER TABLE "CacheEntries" ADD COLUMN "HadSecrets" boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE "CacheEntries" DROP COLUMN "HadSecrets";
ALTER TABLE "Projects" DROP CONSTRAINT "CK_Projects_ForkCacheRestore";
ALTER TABLE "Projects" DROP COLUMN "ForkCacheRestore";
ALTER TABLE "RunJobs" DROP CONSTRAINT "CK_RunJobs_FailureDetail";
ALTER TABLE "RunJobs" DROP COLUMN "FailureDetail";
