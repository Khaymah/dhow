-- name: LockReleaseLease :one
SELECT l."Id" AS "LeaseId", l."JobId", l."OrganizationId", j."Spec", rj."Id" AS "RunJobId", d."Id" AS "DeploymentId"
FROM "Leases" l
JOIN "Jobs" j ON j."Id" = l."JobId"
LEFT JOIN "RunJobs" rj ON rj."Id" = j."OriginId" AND rj."OrganizationId" = j."OrganizationId" AND j."OriginKind" = 'pipeline'
LEFT JOIN "Deployments" d ON d."RunJobId" = rj."Id" AND d."JobId" = j."Id" AND d."OrganizationId" = j."OrganizationId"
WHERE l."Id" = @lease_id AND l."FencingToken" = @fencing_token AND l."RunnerId" = @runner_id
  AND l."State" = 'active' AND l."ExpiresAt" > now() AND NOT l."IsDeleted"
FOR SHARE OF l;

-- name: GetReleaseForUpdate :one
SELECT *
FROM "DeploymentReleases"
WHERE "TargetId" = @target_id AND "ReleaseId" = @release_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
FOR UPDATE;

-- name: GetRunningReleaseOfTarget :one
SELECT r.*
FROM "DeploymentReleases" r
WHERE r."TargetId" = @target_id AND r."OrganizationId" = @organization_id AND r."State" = 'running' AND NOT r."IsDeleted"
FOR UPDATE OF r;

-- name: IsLeaseActive :one
SELECT EXISTS (SELECT 1
               FROM "Leases" l
               WHERE l."Id" = @lease_id AND l."State" = 'active' AND l."ExpiresAt" > now() AND NOT l."IsDeleted")::boolean AS "Active";

-- name: InsertRelease :one
INSERT INTO "DeploymentReleases" (
    "OrganizationId", "DeploymentId", "RunJobId", "JobId", "LeaseId", "TargetId", "StepIndex", "Kind", "ReleaseId",
    "PreviousReleaseId", "ArtifactDigest", "BatchSize", "StartedAt", "CreatedAt", "UpdatedAt"
)
VALUES (
    @organization_id, @deployment_id, @run_job_id, @job_id, @lease_id, @target_id, @step_index, @kind, @release_id,
    sqlc.narg('previous_release_id')::text, sqlc.narg('artifact_digest')::bytea, @batch_size, @now, @now, @now
)
RETURNING *;

-- name: InsertReleaseHost :exec
INSERT INTO "DeploymentReleaseHosts" ("OrganizationId", "ReleaseRowId", "HostId", "ReleaseId", "Position", "CreatedAt", "UpdatedAt")
VALUES (@organization_id, @release_row_id, @host_id, @release_id, @position, @now, @now);

-- name: ListReleaseHostsForUpdate :many
SELECT *
FROM "DeploymentReleaseHosts"
WHERE "ReleaseRowId" = @release_row_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
ORDER BY "Position"
FOR UPDATE;

-- name: UpdateReleaseHost :exec
UPDATE "DeploymentReleaseHosts"
SET "State" = @state, "Batch" = sqlc.narg('batch')::integer, "PreviousReleaseId" = sqlc.narg('previous_release_id')::text,
    "Health" = @health, "RollbackReason" = sqlc.narg('rollback_reason')::text, "Message" = sqlc.narg('message')::text,
    "SwitchedAt" = sqlc.narg('switched_at')::timestamptz, "RolledBackAt" = sqlc.narg('rolled_back_at')::timestamptz,
    "FinishedAt" = sqlc.narg('finished_at')::timestamptz, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id;

-- name: FinishRelease :exec
UPDATE "DeploymentReleases"
SET "State" = @state, "FailedHostId" = sqlc.narg('failed_host_id')::uuid, "RollbackReason" = sqlc.narg('rollback_reason')::text,
    "Message" = sqlc.narg('message')::text, "FinishedAt" = @now, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "State" = 'running';

-- name: SetReleaseFailedHost :exec
UPDATE "DeploymentReleases"
SET "FailedHostId" = @failed_host_id, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "FailedHostId" IS NULL;

-- name: MarkReleaseDeploymentRunning :exec
UPDATE "Deployments"
SET "State" = 'running', "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "State" = 'queued';

-- name: LockStaleRunningReleases :many
SELECT r.*
FROM "DeploymentReleases" r
WHERE r."State" = 'running' AND NOT r."IsDeleted"
  AND NOT EXISTS (SELECT 1
                  FROM "Leases" l
                  WHERE l."Id" = r."LeaseId" AND l."State" = 'active' AND l."ExpiresAt" > now() AND NOT l."IsDeleted")
ORDER BY r."CreatedAt", r."Id"
LIMIT 100
FOR UPDATE OF r SKIP LOCKED;

-- name: LockRunningReleasesOfRun :many
SELECT r.*
FROM "DeploymentReleases" r
JOIN "Deployments" d ON d."Id" = r."DeploymentId"
JOIN "RunJobs" rj ON rj."Id" = r."RunJobId"
WHERE d."RunId" = @run_id AND r."OrganizationId" = @organization_id AND r."State" = 'running' AND NOT r."IsDeleted"
  AND rj."State" IN ('succeeded', 'failed', 'canceled', 'skipped')
ORDER BY r."CreatedAt", r."Id"
FOR UPDATE OF r;

-- name: MarkReleaseHostsUnknown :execrows
UPDATE "DeploymentReleaseHosts"
SET "State" = 'unknown', "FinishedAt" = @now, "UpdatedAt" = @now
WHERE "ReleaseRowId" = @release_row_id AND "OrganizationId" = @organization_id AND "State" IN ('uploaded', 'switched');

-- name: ListDeploymentReleases :many
SELECT *
FROM "DeploymentReleases"
WHERE "DeploymentId" = @deployment_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
ORDER BY "CreatedAt", "Id";
