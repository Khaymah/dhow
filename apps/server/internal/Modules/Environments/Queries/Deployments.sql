-- name: LockGateEnvironment :one
SELECT e."Id", e."Name", e."RuleVersion", e."AllowedRefs", e."AllowPullRequests", e."WaitMinutes", e."RequirePinnedImages",
       e."AllowedPlugins", (e."PausedAt" IS NOT NULL)::boolean AS "Paused", e."LockedByRunId", e."ApprovalTimeoutHours",
       e."SupersedePending",
       EXISTS (SELECT 1
               FROM "EnvironmentReviewers" r
               WHERE r."EnvironmentId" = e."Id" AND r."OrganizationId" = e."OrganizationId" AND NOT r."IsDeleted")::boolean
           AS "HasReviewers"
FROM "Environments" e
WHERE e."ProjectId" = @project_id AND e."OrganizationId" = @organization_id AND e."Name" = @name AND NOT e."IsDeleted"
FOR UPDATE OF e;

-- name: HoldEnvironment :exec
UPDATE "Environments"
SET "LockedByRunId" = @run_id, "LockedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND ("LockedByRunId" IS NULL OR "LockedByRunId" = @run_id);

-- name: InsertDeployment :exec
INSERT INTO "Deployments" (
    "OrganizationId", "EnvironmentId", "RunId", "RunNumber", "RunJobId", "JobId", "JobKey", "Ref", "Sha", "RuleVersion",
    "ApprovalId", "CreatedAt", "UpdatedAt"
)
VALUES (
    @organization_id, @environment_id, @run_id, @run_number, @run_job_id, @job_id, @job_key, @ref, @sha, @rule_version,
    sqlc.narg('approval_id')::uuid, @now, @now
);

-- name: SettleDeployments :many
UPDATE "Deployments" d
SET "State" = CASE rj."State" WHEN 'succeeded' THEN 'succeeded' WHEN 'failed' THEN 'failed' ELSE 'canceled' END,
    "FinishedAt" = @now, "UpdatedAt" = @now
FROM "RunJobs" rj
WHERE d."RunId" = @run_id AND d."OrganizationId" = @organization_id AND d."State" IN ('queued', 'running')
  AND rj."Id" = d."RunJobId" AND rj."OrganizationId" = d."OrganizationId"
  AND rj."State" IN ('succeeded', 'failed', 'canceled', 'skipped')
RETURNING d."Id", d."EnvironmentId", d."RunJobId", d."State";

-- name: ReleaseEnvironments :exec
WITH held AS (
    SELECT h."Id"
    FROM "Environments" h
    WHERE h."LockedByRunId" = @run_id AND h."OrganizationId" = @organization_id AND NOT (h."Name" = ANY(@keep::text[]))
    ORDER BY h."Id"
    FOR UPDATE OF h
)
UPDATE "Environments" e
SET "LockedByRunId" = NULL, "LockedAt" = NULL
FROM held
WHERE e."Id" = held."Id";

-- name: ReleaseStaleEnvironments :execrows
WITH stale AS (
    SELECT e."Id"
    FROM "Environments" e
    WHERE e."LockedByRunId" IS NOT NULL
      AND NOT EXISTS (SELECT 1
                      FROM "PipelineRuns" r
                      WHERE r."Id" = e."LockedByRunId" AND r."OrganizationId" = e."OrganizationId"
                        AND r."State" IN ('pending', 'queued', 'running') AND NOT r."IsDeleted")
    ORDER BY e."Id"
    FOR UPDATE OF e SKIP LOCKED
)
UPDATE "Environments" e
SET "LockedByRunId" = NULL, "LockedAt" = NULL
FROM stale
WHERE e."Id" = stale."Id";

-- name: ListDeployments :many
SELECT d.*
FROM "Deployments" d
WHERE d."EnvironmentId" = @environment_id AND d."OrganizationId" = @organization_id AND NOT d."IsDeleted"
ORDER BY d."CreatedAt" DESC, d."Id" DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountDeployments :one
SELECT count(*)
FROM "Deployments"
WHERE "EnvironmentId" = @environment_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: GetStartEnvironment :one
SELECT e."Id", e."Name", e."RuleVersion", e."AllowedRefs", e."AllowPullRequests", (e."PausedAt" IS NOT NULL)::boolean AS "Paused",
       d."RuleVersion" AS "DeployedRuleVersion"
FROM "Deployments" d
JOIN "Environments" e ON e."Id" = d."EnvironmentId" AND e."OrganizationId" = d."OrganizationId" AND NOT e."IsDeleted"
WHERE d."RunJobId" = @run_job_id AND d."JobId" = @job_id AND d."OrganizationId" = @organization_id AND NOT d."IsDeleted"
FOR SHARE OF e;
