-- dhow:phase expand
-- +goose Up
ALTER TABLE "ApiTokens" DROP CONSTRAINT "ApiTokens_Scopes_check";
ALTER TABLE "ApiTokens" ADD CONSTRAINT "ApiTokens_Scopes_check" CHECK (
    cardinality("Scopes") BETWEEN 1 AND 16
    AND "Scopes" <@ ARRAY['org.read', 'org.update', 'member.read', 'member.manage', 'audit.read', 'runner.read', 'runner.manage',
                          'job.read', 'job.cancel', 'project.read', 'project.manage', 'run.read', 'run.trigger', 'run.cancel',
                          'secret.read']::text[]
) NOT VALID;

-- +goose Down
UPDATE "ApiTokens" SET "Scopes" = array_remove("Scopes", 'secret.read');
UPDATE "ApiTokens" SET "IsDeleted" = true, "Scopes" = ARRAY['org.read']::text[] WHERE cardinality("Scopes") = 0;
ALTER TABLE "ApiTokens" DROP CONSTRAINT "ApiTokens_Scopes_check";
ALTER TABLE "ApiTokens" ADD CONSTRAINT "ApiTokens_Scopes_check" CHECK (
    cardinality("Scopes") BETWEEN 1 AND 16
    AND "Scopes" <@ ARRAY['org.read', 'org.update', 'member.read', 'member.manage', 'audit.read', 'runner.read', 'runner.manage',
                          'job.read', 'job.cancel', 'project.read', 'project.manage', 'run.read', 'run.trigger', 'run.cancel']::text[]
);
