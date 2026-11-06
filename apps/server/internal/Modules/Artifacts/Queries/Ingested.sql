-- name: ListRunIngestedArtifacts :many
SELECT ia."Name", ia."Digest", ia."SizeBytes"
FROM "RunIngestedArtifacts" ria
JOIN "IngestedArtifacts" ia ON ia."Id" = ria."IngestedArtifactId" AND ia."OrganizationId" = ria."OrganizationId"
WHERE ria."OrganizationId" = @organization_id AND ria."RunId" = @run_id AND ria."Name" = ANY (@names::text[])
  AND NOT ria."IsDeleted" AND NOT ia."IsDeleted"
  AND (ia."ExpiresAt" > now() OR ia."PinnedUntil" > now())
ORDER BY ria."Name";

-- name: PinIngestedArtifacts :execrows
UPDATE "IngestedArtifacts" a
SET "PinnedUntil" = now() + make_interval(secs => @pin_seconds::float8), "UpdatedAt" = now()
WHERE a."PinnedUntil" IS NULL AND NOT a."IsDeleted"
  AND EXISTS (SELECT 1
              FROM "DeploymentReleases" r
              JOIN "DeployTargets" t ON t."Id" = r."TargetId" AND t."OrganizationId" = r."OrganizationId"
              WHERE r."ArtifactDigest" = a."Digest" AND r."OrganizationId" = a."OrganizationId" AND t."ProjectId" = a."ProjectId"
                AND r."Kind" = 'deploy' AND r."State" = 'succeeded' AND NOT r."IsDeleted" AND NOT t."IsDeleted"
                AND (SELECT count(*)
                     FROM "DeploymentReleases" n
                     WHERE n."TargetId" = r."TargetId" AND n."OrganizationId" = r."OrganizationId"
                       AND n."Kind" = 'deploy' AND n."State" = 'succeeded' AND NOT n."IsDeleted"
                       AND (n."StartedAt", n."Id") > (r."StartedAt", r."Id")) < t."KeepReleases");

-- name: ExpireIngestedArtifacts :execrows
UPDATE "IngestedArtifacts"
SET "IsDeleted" = true, "UpdatedAt" = now()
WHERE "Id" IN (SELECT a."Id"
               FROM "IngestedArtifacts" a
               WHERE a."ExpiresAt" <= now() AND NOT a."IsDeleted"
                 AND NOT (a."PinnedUntil" > now()
                          AND EXISTS (SELECT 1
                                      FROM "DeploymentReleases" r
                                      JOIN "DeployTargets" t ON t."Id" = r."TargetId" AND t."OrganizationId" = r."OrganizationId"
                                      WHERE r."ArtifactDigest" = a."Digest" AND r."OrganizationId" = a."OrganizationId"
                                        AND t."ProjectId" = a."ProjectId" AND r."Kind" = 'deploy' AND r."State" = 'succeeded'
                                        AND NOT r."IsDeleted" AND NOT t."IsDeleted"
                                        AND (SELECT count(*)
                                             FROM "DeploymentReleases" n
                                             WHERE n."TargetId" = r."TargetId" AND n."OrganizationId" = r."OrganizationId"
                                               AND n."Kind" = 'deploy' AND n."State" = 'succeeded' AND NOT n."IsDeleted"
                                               AND (n."StartedAt", n."Id") > (r."StartedAt", r."Id")) < t."KeepReleases"))
               LIMIT @batch
               FOR UPDATE SKIP LOCKED);
