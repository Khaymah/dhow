-- name: UpsertHostCheck :one
INSERT INTO "DeploymentHostChecks" (
    "OrganizationId", "TargetId", "HostId", "DeploymentId", "RunJobId", "Reason", "Message", "CurrentReleaseId", "VerifiedReleaseId",
    "FreeBytes", "Locked", "CheckedAt", "CreatedAt", "UpdatedAt"
)
VALUES (
    @organization_id, @target_id, @host_id, @deployment_id, @run_job_id, @reason, sqlc.narg('message')::text,
    sqlc.narg('current_release_id')::text, sqlc.narg('verified_release_id')::text, sqlc.narg('free_bytes')::bigint, @locked, @now, @now, @now
)
ON CONFLICT ("RunJobId", "HostId") WHERE NOT "IsDeleted" DO UPDATE
SET "Reason" = EXCLUDED."Reason", "Message" = EXCLUDED."Message", "CurrentReleaseId" = EXCLUDED."CurrentReleaseId",
    "VerifiedReleaseId" = EXCLUDED."VerifiedReleaseId", "FreeBytes" = EXCLUDED."FreeBytes", "Locked" = EXCLUDED."Locked",
    "CheckedAt" = EXCLUDED."CheckedAt", "UpdatedAt" = EXCLUDED."UpdatedAt"
RETURNING "Id";

-- name: MarkHostCheckRepaired :exec
UPDATE "DeploymentHostChecks"
SET "Repaired" = true, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id;

-- name: ListLatestHostChecks :many
SELECT DISTINCT ON (c."HostId") c.*
FROM "DeploymentHostChecks" c
WHERE c."TargetId" = @target_id AND c."OrganizationId" = @organization_id AND NOT c."IsDeleted"
ORDER BY c."HostId", c."CheckedAt" DESC, c."Id" DESC;

-- name: LockUnknownReleaseHosts :many
SELECT h."Id", h."ReleaseRowId", h."ReleaseId", h."State", r."State" AS "GroupState", r."TargetId"
FROM "DeploymentReleaseHosts" h
JOIN "DeploymentReleases" r ON r."Id" = h."ReleaseRowId" AND r."OrganizationId" = h."OrganizationId"
WHERE h."HostId" = @host_id AND h."OrganizationId" = @organization_id AND r."TargetId" = @target_id
  AND h."State" = 'unknown' AND NOT h."IsDeleted" AND NOT r."IsDeleted"
ORDER BY r."CreatedAt", r."Id"
FOR UPDATE OF h, r;

-- name: RepairReleaseHost :exec
UPDATE "DeploymentReleaseHosts"
SET "State" = @state, "Message" = @message, "RolledBackAt" = sqlc.narg('rolled_back_at')::timestamptz, "FinishedAt" = @now, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "State" = 'unknown';

-- name: ListReleaseHostStates :many
SELECT "State"
FROM "DeploymentReleaseHosts"
WHERE "ReleaseRowId" = @release_row_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: RepairRelease :exec
UPDATE "DeploymentReleases"
SET "State" = @state, "Message" = @message, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "State" = 'unknown';

-- name: SupersedeOlderHostChecks :exec
UPDATE "DeploymentHostChecks"
SET "IsDeleted" = true, "UpdatedAt" = @now
WHERE "TargetId" = @target_id AND "HostId" = @host_id AND "OrganizationId" = @organization_id AND "Id" <> @id AND NOT "IsDeleted";
