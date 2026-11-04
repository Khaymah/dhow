-- dhow:phase expand
-- +goose Up
ALTER TABLE "RunJobs" ADD COLUMN "FailureDetail" jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE "RunJobs" ADD CONSTRAINT "CK_RunJobs_FailureDetail"
    CHECK (jsonb_typeof("FailureDetail") = 'object' AND octet_length("FailureDetail"::text) <= 1024) NOT VALID;

ALTER TABLE "Projects" ADD COLUMN "ForkCacheRestore" TEXT NOT NULL DEFAULT 'none';
ALTER TABLE "Projects" ADD CONSTRAINT "CK_Projects_ForkCacheRestore"
    CHECK ("ForkCacheRestore" IN ('none', 'default_branch')) NOT VALID;

ALTER TABLE "CacheEntries" ADD COLUMN "HadSecrets" boolean NOT NULL DEFAULT false;

ALTER TABLE "Jobs" DROP CONSTRAINT "CK_Jobs_RequiredFeatures";
ALTER TABLE "Jobs" ADD CONSTRAINT "CK_Jobs_RequiredFeatures" CHECK (
    cardinality("RequiredFeatures") <= 8
    AND "RequiredFeatures" <@ ARRAY['checkout', 'plugins', 'artifacts', 'cache', 'registry_auth']::text[]
) NOT VALID;
ALTER TABLE "Runners" DROP CONSTRAINT "CK_Runners_Features";
ALTER TABLE "Runners" ADD CONSTRAINT "CK_Runners_Features" CHECK (
    cardinality("Features") <= 8
    AND "Features" <@ ARRAY['checkout', 'plugins', 'artifacts', 'cache', 'registry_auth']::text[]
) NOT VALID;

-- +goose Down
UPDATE "Runners" SET "Features" = array_remove("Features", 'registry_auth');
ALTER TABLE "Runners" DROP CONSTRAINT "CK_Runners_Features";
ALTER TABLE "Runners" ADD CONSTRAINT "CK_Runners_Features" CHECK (
    cardinality("Features") <= 8
    AND "Features" <@ ARRAY['checkout', 'plugins', 'artifacts', 'cache']::text[]
);
UPDATE "Jobs" SET "RequiredFeatures" = array_remove("RequiredFeatures", 'registry_auth');
ALTER TABLE "Jobs" DROP CONSTRAINT "CK_Jobs_RequiredFeatures";
ALTER TABLE "Jobs" ADD CONSTRAINT "CK_Jobs_RequiredFeatures" CHECK (
    cardinality("RequiredFeatures") <= 8
    AND "RequiredFeatures" <@ ARRAY['checkout', 'plugins', 'artifacts', 'cache']::text[]
);
ALTER TABLE "CacheEntries" DROP COLUMN "HadSecrets";
ALTER TABLE "Projects" DROP CONSTRAINT "CK_Projects_ForkCacheRestore";
ALTER TABLE "Projects" DROP COLUMN "ForkCacheRestore";
ALTER TABLE "RunJobs" DROP CONSTRAINT "CK_RunJobs_FailureDetail";
ALTER TABLE "RunJobs" DROP COLUMN "FailureDetail";
