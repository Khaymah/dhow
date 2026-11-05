-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" DROP CONSTRAINT "CK_Jobs_RequiredFeatures";
ALTER TABLE "Jobs" ADD CONSTRAINT "CK_Jobs_RequiredFeatures" CHECK (
    cardinality("RequiredFeatures") <= 8
    AND "RequiredFeatures" <@ ARRAY['checkout', 'plugins', 'artifacts', 'cache', 'registry_auth', 'deploy_targets']::text[]
) NOT VALID;
ALTER TABLE "Runners" DROP CONSTRAINT "CK_Runners_Features";
ALTER TABLE "Runners" ADD CONSTRAINT "CK_Runners_Features" CHECK (
    cardinality("Features") <= 8
    AND "Features" <@ ARRAY['checkout', 'plugins', 'artifacts', 'cache', 'registry_auth', 'deploy_targets']::text[]
) NOT VALID;

-- +goose Down
UPDATE "Runners" SET "Features" = array_remove("Features", 'deploy_targets');
ALTER TABLE "Runners" DROP CONSTRAINT "CK_Runners_Features";
ALTER TABLE "Runners" ADD CONSTRAINT "CK_Runners_Features" CHECK (
    cardinality("Features") <= 8
    AND "Features" <@ ARRAY['checkout', 'plugins', 'artifacts', 'cache', 'registry_auth']::text[]
);
UPDATE "Jobs" SET "RequiredFeatures" = array_remove("RequiredFeatures", 'deploy_targets');
ALTER TABLE "Jobs" DROP CONSTRAINT "CK_Jobs_RequiredFeatures";
ALTER TABLE "Jobs" ADD CONSTRAINT "CK_Jobs_RequiredFeatures" CHECK (
    cardinality("RequiredFeatures") <= 8
    AND "RequiredFeatures" <@ ARRAY['checkout', 'plugins', 'artifacts', 'cache', 'registry_auth']::text[]
);
