-- name: CountOverviewJobs :one
SELECT count(*) FILTER (WHERE j."State" IN ('leased', 'running'))::bigint AS running,
       count(*) FILTER (WHERE j."State" = 'queued')::bigint AS queued,
       count(*) FILTER (
           WHERE j."State" = 'queued'
             AND NOT EXISTS (SELECT 1
                             FROM "Runners" r
                             WHERE r."OrganizationId" = j."OrganizationId" AND r."State" = 'active' AND NOT r."IsDeleted"
                               AND j."Labels" <@ (r."Labels" || r."CapabilityLabels")
                               AND j."Isolation" = ANY (r."IsolationLevels")))::bigint AS queued_no_runner,
       count(*) FILTER (
           WHERE j."State" IN ('failed', 'timed_out') AND j."FinishedAt" > @failed_since::timestamptz)::bigint AS failed_last_24h
FROM "Jobs" j
WHERE j."OrganizationId" = @organization_id AND NOT j."IsDeleted"
  AND (j."State" IN ('queued', 'leased', 'running')
    OR (j."State" IN ('failed', 'timed_out') AND j."FinishedAt" > @failed_since::timestamptz));

-- name: CountOverviewRunners :one
SELECT count(*) FILTER (WHERE "State" = 'active' AND "LastSeenAt" > @online_since::timestamptz)::bigint AS online,
       count(*) FILTER (
           WHERE "State" = 'active' AND ("LastSeenAt" IS NULL OR "LastSeenAt" <= @online_since::timestamptz))::bigint AS offline,
       count(*) FILTER (WHERE "State" = 'disabled')::bigint AS paused
FROM "Runners"
WHERE "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: CountOverviewApprovals :one
SELECT count(*)::bigint
FROM "DeploymentApprovals" a
WHERE a."OrganizationId" = @organization_id AND a."State" = 'pending' AND a."ExpiresAt" > @now::timestamptz AND NOT a."IsDeleted"
  AND EXISTS (SELECT 1
              FROM "EnvironmentReviewers" r
              WHERE r."EnvironmentId" = a."EnvironmentId" AND r."OrganizationId" = a."OrganizationId" AND NOT r."IsDeleted"
                AND (r."UserId" = @user_id
                     OR EXISTS (SELECT 1
                                FROM "TeamMembers" tm
                                JOIN "Teams" t ON t."Id" = tm."TeamId" AND t."OrganizationId" = tm."OrganizationId" AND NOT t."IsDeleted"
                                WHERE tm."TeamId" = r."TeamId" AND tm."OrganizationId" = r."OrganizationId"
                                  AND tm."UserId" = @user_id AND NOT tm."IsDeleted")));

-- name: CountOverviewInvitations :one
SELECT count(*) FILTER (WHERE "ExpiresAt" > @now::timestamptz)::bigint AS pending
FROM "Invitations"
WHERE "OrganizationId" = @organization_id AND "AcceptedAt" IS NULL AND "RevokedAt" IS NULL AND NOT "IsDeleted";
