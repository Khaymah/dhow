-- name: InsertIngestToken :one
INSERT INTO "IngestTokens" ("OrganizationId", "ProjectId", "Name", "TokenHash", "Prefix", "RefPatterns", "ExpiresAt", "CreatedByUserId",
                            "AcknowledgedEnvironments")
VALUES (@organization_id, @project_id, @name, @token_hash, @prefix, @ref_patterns, @expires_at, @created_by_user_id,
        @acknowledged_environments::text[])
RETURNING *;

-- name: ListIngestTokens :many
SELECT *
FROM "IngestTokens"
WHERE "OrganizationId" = @organization_id AND "ProjectId" = @project_id AND NOT "IsDeleted"
ORDER BY "CreatedAt" DESC, "Id" DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountIngestTokens :one
SELECT count(*)::bigint AS "Total"
FROM "IngestTokens"
WHERE "OrganizationId" = @organization_id AND "ProjectId" = @project_id AND NOT "IsDeleted";

-- name: CountActiveIngestTokens :one
SELECT count(*)::bigint AS "Active"
FROM "IngestTokens"
WHERE "OrganizationId" = @organization_id AND "ProjectId" = @project_id AND NOT "IsDeleted" AND "RevokedAt" IS NULL
  AND "ExpiresAt" > @now;

-- name: LockIngestTokens :exec
SELECT pg_advisory_xact_lock(hashtextextended('dhow-ingest-tokens:' || @project_id::text, 0));

-- name: IngestTokenNameTaken :one
SELECT EXISTS (SELECT 1
               FROM "IngestTokens"
               WHERE "ProjectId" = @project_id AND "OrganizationId" = @organization_id AND "Name" = @name
                 AND NOT "IsDeleted" AND "RevokedAt" IS NULL)::boolean AS "Taken";

-- name: RevokeIngestToken :one
UPDATE "IngestTokens"
SET "RevokedAt" = @now, "RevokedByUserId" = @revoked_by_user_id, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "ProjectId" = @project_id AND NOT "IsDeleted" AND "RevokedAt" IS NULL
RETURNING *;

-- name: GetIngestToken :one
SELECT *
FROM "IngestTokens"
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "ProjectId" = @project_id AND NOT "IsDeleted";

-- name: FindIngestTokenByHash :one
SELECT t.*, p."State" AS "ProjectState"
FROM "IngestTokens" t
JOIN "Projects" p ON p."Id" = t."ProjectId" AND p."OrganizationId" = t."OrganizationId" AND NOT p."IsDeleted"
WHERE t."TokenHash" = @token_hash AND NOT t."IsDeleted";

-- name: TouchIngestToken :exec
UPDATE "IngestTokens"
SET "LastUsedAt" = @now
WHERE "Id" = @id AND ("LastUsedAt" IS NULL OR "LastUsedAt" < @stale_before);

-- name: LockIngestProject :exec
SELECT pg_advisory_xact_lock(hashtextextended('dhow-ingest:' || @project_id::text, 0));

-- name: FindIngestedArtifact :one
SELECT *
FROM "IngestedArtifacts"
WHERE "ProjectId" = @project_id AND "OrganizationId" = @organization_id AND "Digest" = @digest AND "Name" = @name
  AND "Ref" = @ref AND "Sha" = @sha AND NOT "IsDeleted";

-- name: InsertIngestedArtifact :one
INSERT INTO "IngestedArtifacts" ("OrganizationId", "ProjectId", "TokenId", "Name", "Digest", "SizeBytes", "Ref", "RefKind", "Sha",
                                 "Provenance", "ExpiresAt")
VALUES (@organization_id, @project_id, @token_id, @name, @digest, @size_bytes, @ref, @ref_kind, @sha, @provenance, @expires_at)
RETURNING *;

-- name: IngestedArtifactRun :one
SELECT r."Id" AS "RunId", r."Number", r."State"
FROM "RunIngestedArtifacts" ria
JOIN "PipelineRuns" r ON r."Id" = ria."RunId" AND r."OrganizationId" = ria."OrganizationId" AND NOT r."IsDeleted"
WHERE ria."IngestedArtifactId" = @ingested_artifact_id AND ria."OrganizationId" = @organization_id AND NOT ria."IsDeleted"
ORDER BY r."Number" DESC
LIMIT 1;

-- name: ListIngestedArtifacts :many
SELECT a.*, t."Name" AS "TokenName"
FROM "IngestedArtifacts" a
JOIN "IngestTokens" t ON t."Id" = a."TokenId"
WHERE a."OrganizationId" = @organization_id AND a."ProjectId" = @project_id AND NOT a."IsDeleted"
ORDER BY a."CreatedAt" DESC, a."Id" DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListIngestedRuns :many
SELECT ria."IngestedArtifactId", r."Id" AS "RunId", r."Number", r."State"
FROM "RunIngestedArtifacts" ria
JOIN "PipelineRuns" r ON r."Id" = ria."RunId" AND r."OrganizationId" = ria."OrganizationId" AND NOT r."IsDeleted"
WHERE ria."OrganizationId" = @organization_id AND ria."IngestedArtifactId" = ANY (@ids::uuid[]) AND NOT ria."IsDeleted"
ORDER BY r."Number";

-- name: CountIngestedArtifacts :one
SELECT count(*)::bigint AS "Total"
FROM "IngestedArtifacts"
WHERE "OrganizationId" = @organization_id AND "ProjectId" = @project_id AND NOT "IsDeleted";

-- name: IngestTokenBytes :one
SELECT COALESCE(sum(u."SizeBytes"), 0)::bigint AS "Bytes"
FROM (SELECT DISTINCT ON ("Digest") "SizeBytes"
      FROM "IngestedArtifacts"
      WHERE "TokenId" = @token_id AND NOT "IsDeleted"
      ORDER BY "Digest") u;

-- name: RecheckIngestToken :one
SELECT t."Id", t."RevokedAt", t."ExpiresAt", p."State" AS "ProjectState"
FROM "IngestTokens" t
JOIN "Projects" p ON p."Id" = t."ProjectId" AND p."OrganizationId" = t."OrganizationId" AND NOT p."IsDeleted"
WHERE t."Id" = @id AND NOT t."IsDeleted"
FOR SHARE OF t;

-- name: ListEnvironmentReach :many
SELECT e."Name", e."AllowedRefs",
       EXISTS (SELECT 1 FROM "DeployTargets" d WHERE d."EnvironmentId" = e."Id" AND NOT d."IsDeleted")::boolean AS "HasTargets",
       EXISTS (SELECT 1 FROM "EnvironmentReviewers" r WHERE r."EnvironmentId" = e."Id" AND NOT r."IsDeleted")::boolean AS "HasReviewers"
FROM "Environments" e
WHERE e."OrganizationId" = @organization_id AND e."ProjectId" = @project_id AND NOT e."IsDeleted"
ORDER BY e."Name";

-- name: IngestTargetProject :one
SELECT p."Id", p."State"
FROM "Projects" p
WHERE p."Id" = @project_id AND p."OrganizationId" = @organization_id AND NOT p."IsDeleted";

-- name: ListIngestedByDigest :many
SELECT *
FROM "IngestedArtifacts"
WHERE "ProjectId" = @project_id AND "OrganizationId" = @organization_id AND "Digest" = @digest AND NOT "IsDeleted"
ORDER BY "CreatedAt" DESC, "Id" DESC
LIMIT @max_rows;

-- name: ListIngestCreatorAccess :many
SELECT u."Id" AS "UserId", m."Role",
       ARRAY(SELECT r."Role" FROM "OrganizationTeamRoles" r
             WHERE r."UserId" = u."Id" AND r."OrganizationId" = m."OrganizationId")::text[] AS "TeamRoles",
       ARRAY(SELECT rb."Role"
             FROM "RoleBindings" rb
             WHERE rb."OrganizationId" = m."OrganizationId" AND rb."Scope" = 'project' AND rb."ProjectId" = @project_id::uuid
               AND NOT rb."IsDeleted"
               AND (rb."UserId" = u."Id"
                    OR EXISTS (SELECT 1
                               FROM "TeamMembers" tm
                               JOIN "Teams" t ON t."Id" = tm."TeamId" AND t."OrganizationId" = tm."OrganizationId" AND NOT t."IsDeleted"
                               WHERE tm."TeamId" = rb."TeamId" AND tm."OrganizationId" = rb."OrganizationId"
                                 AND tm."UserId" = u."Id" AND NOT tm."IsDeleted")))::text[] AS "Bindings"
FROM "Users" u
JOIN "Memberships" m ON m."UserId" = u."Id" AND m."OrganizationId" = @organization_id AND NOT m."IsDeleted"
WHERE u."Id" = ANY (@user_ids::uuid[]) AND NOT u."IsDeleted" AND NOT u."Disabled";

-- name: RevokeIngestTokensOfCreator :many
UPDATE "IngestTokens"
SET "RevokedAt" = @now::timestamptz, "RevokedByUserId" = @actor_id::uuid, "UpdatedAt" = @now::timestamptz
WHERE "OrganizationId" = @organization_id AND "CreatedByUserId" = @user_id AND NOT "IsDeleted" AND "RevokedAt" IS NULL
RETURNING "Id", "ProjectId", "Name";

-- name: AcknowledgeIngestTokenReach :one
UPDATE "IngestTokens"
SET "AcknowledgedEnvironments" = @environments::text[], "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "ProjectId" = @project_id AND NOT "IsDeleted" AND "RevokedAt" IS NULL
  AND "ExpiresAt" > @now
RETURNING *;
