-- name: ListBoardEnvironments :many
SELECT e."Id", e."Name", (e."PausedAt" IS NOT NULL)::boolean AS "Paused"
FROM "Environments" e
WHERE e."OrganizationId" = @organization_id AND e."ProjectId" = @project_id AND NOT e."IsDeleted"
ORDER BY e."Name", e."Id"
LIMIT @max_rows;

-- name: ListBoardTargets :many
SELECT t."Id", t."EnvironmentId", t."Name", t."Kind",
       (SELECT count(*)
        FROM "DeployTargetHosts" h
        WHERE h."TargetId" = t."Id" AND h."OrganizationId" = t."OrganizationId" AND NOT h."IsDeleted")::integer AS "Hosts"
FROM "DeployTargets" t
JOIN "Environments" e ON e."Id" = t."EnvironmentId" AND e."OrganizationId" = t."OrganizationId"
WHERE t."OrganizationId" = @organization_id AND t."ProjectId" = @project_id AND NOT t."IsDeleted" AND NOT e."IsDeleted"
ORDER BY e."Name", t."Name", t."Id"
LIMIT @max_rows;

-- name: ListBoardReleases :many
SELECT s."Slot"::text AS "Slot", s."Id", s."TargetId", s."ReleaseId", s."Kind", s."State", s."RestoredReleaseId", s."RollbackReason",
       s."BatchSize", s."ArtifactDigest", s."StartedAt", s."FinishedAt"
FROM (
    SELECT 'current' AS "Slot", r."Id", r."TargetId", r."ReleaseId", r."Kind", r."State", r."RestoredReleaseId", r."RollbackReason",
           r."BatchSize", r."ArtifactDigest", r."StartedAt", r."FinishedAt"
    FROM "DeployTargets" t
    CROSS JOIN LATERAL (
        SELECT * FROM "DeploymentReleases" x
        WHERE x."TargetId" = t."Id" AND x."OrganizationId" = t."OrganizationId" AND x."State" = 'succeeded' AND NOT x."IsDeleted"
        ORDER BY x."StartedAt" DESC, x."Id" DESC LIMIT 1
    ) r
    WHERE t."Id" = ANY (@target_ids::uuid[]) AND t."OrganizationId" = @organization_id AND t."ProjectId" = @project_id AND NOT t."IsDeleted"
    UNION ALL
    SELECT 'last', r."Id", r."TargetId", r."ReleaseId", r."Kind", r."State", r."RestoredReleaseId", r."RollbackReason",
           r."BatchSize", r."ArtifactDigest", r."StartedAt", r."FinishedAt"
    FROM "DeployTargets" t
    CROSS JOIN LATERAL (
        SELECT * FROM "DeploymentReleases" x
        WHERE x."TargetId" = t."Id" AND x."OrganizationId" = t."OrganizationId" AND x."State" <> 'running' AND NOT x."IsDeleted"
        ORDER BY x."StartedAt" DESC, x."Id" DESC LIMIT 1
    ) r
    WHERE t."Id" = ANY (@target_ids::uuid[]) AND t."OrganizationId" = @organization_id AND t."ProjectId" = @project_id AND NOT t."IsDeleted"
    UNION ALL
    SELECT 'running', r."Id", r."TargetId", r."ReleaseId", r."Kind", r."State", r."RestoredReleaseId", r."RollbackReason",
           r."BatchSize", r."ArtifactDigest", r."StartedAt", r."FinishedAt"
    FROM "DeployTargets" t
    CROSS JOIN LATERAL (
        SELECT * FROM "DeploymentReleases" x
        WHERE x."TargetId" = t."Id" AND x."OrganizationId" = t."OrganizationId" AND x."State" = 'running' AND NOT x."IsDeleted"
        ORDER BY x."StartedAt" DESC, x."Id" DESC LIMIT 1
    ) r
    WHERE t."Id" = ANY (@target_ids::uuid[]) AND t."OrganizationId" = @organization_id AND t."ProjectId" = @project_id AND NOT t."IsDeleted"
) s;

-- name: ListBoardRunningHosts :many
SELECT h."ReleaseRowId",
       count(*)::integer AS "Hosts",
       count(*) FILTER (WHERE h."State" NOT IN ('pending', 'uploaded'))::integer AS "Done",
       COALESCE(max(h."Batch"), 0)::integer AS "Batch"
FROM "DeploymentReleaseHosts" h
WHERE h."ReleaseRowId" = ANY (@release_row_ids::uuid[]) AND h."OrganizationId" = @organization_id AND NOT h."IsDeleted"
GROUP BY h."ReleaseRowId";

-- name: ListBoardHostChecks :many
SELECT c."TargetId", c."HostId", c."CurrentReleaseId", c."CheckedAt"
FROM "DeployTargetHosts" h
CROSS JOIN LATERAL (
    SELECT x."TargetId", x."HostId", x."CurrentReleaseId", x."CheckedAt"
    FROM "DeploymentHostChecks" x
    WHERE x."TargetId" = h."TargetId" AND x."HostId" = h."Id" AND x."OrganizationId" = h."OrganizationId" AND NOT x."IsDeleted"
    ORDER BY x."CheckedAt" DESC, x."Id" DESC
    LIMIT 1
) c
WHERE h."TargetId" = ANY (@target_ids::uuid[]) AND h."OrganizationId" = @organization_id AND NOT h."IsDeleted";

-- name: CountBoardApprovals :many
SELECT a."EnvironmentId", count(*)::integer AS "Pending"
FROM "DeploymentApprovals" a
JOIN "Environments" e ON e."Id" = a."EnvironmentId" AND e."OrganizationId" = a."OrganizationId"
WHERE a."OrganizationId" = @organization_id AND e."ProjectId" = @project_id AND a."State" = 'pending'
  AND a."ExpiresAt" > @now::timestamptz AND NOT a."IsDeleted" AND NOT e."IsDeleted"
GROUP BY a."EnvironmentId";
