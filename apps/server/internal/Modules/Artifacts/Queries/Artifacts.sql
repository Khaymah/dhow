-- name: LockArtifactLease :one
SELECT l."Id" AS "LeaseId", l."JobId", l."OrganizationId", j."Spec", j."OriginKind", j."OriginId",
       rj."Id" AS "RunJobId", rj."Key" AS "RunJobKey", r."Id" AS "RunId", r."ProjectId", r."Trigger", r."Ref", r."IsFork",
       r."PrNumber", r."Sha", r."RepositoryId", COALESCE(repo."DefaultBranch", '')::text AS "DefaultBranch",
       COALESCE((SELECT p."ForkCacheRestore" FROM "Projects" p WHERE p."Id" = r."ProjectId"), 'none')::text AS "ForkCacheRestore",
       COALESCE((SELECT h."Sha"
                 FROM "PipelineRuns" h
                 WHERE h."RepositoryId" = r."RepositoryId" AND h."Ref" = r."Ref" AND h."Trigger" IN ('push', 'tag')
                   AND NOT h."IsFork" AND NOT h."IsDeleted" AND h."Sha" IS NOT NULL
                 ORDER BY h."CreatedAt" DESC, h."Id" DESC
                 LIMIT 1), '')::text AS "RefHead"
FROM "Leases" l
JOIN "Jobs" j ON j."Id" = l."JobId"
LEFT JOIN "RunJobs" rj ON rj."Id" = j."OriginId" AND rj."OrganizationId" = j."OrganizationId" AND j."OriginKind" = 'pipeline'
LEFT JOIN "PipelineRuns" r ON r."Id" = rj."RunId" AND r."OrganizationId" = rj."OrganizationId"
LEFT JOIN "Repositories" repo ON repo."Id" = r."RepositoryId" AND NOT repo."IsDeleted"
WHERE l."Id" = @lease_id AND l."FencingToken" = @fencing_token AND l."RunnerId" = @runner_id
  AND l."State" = 'active' AND l."ExpiresAt" > now() AND NOT l."IsDeleted"
FOR SHARE OF l;

-- name: LockBlobsShared :exec
SELECT pg_advisory_xact_lock_shared(@lock_key::bigint);

-- name: TryBlobSweepLock :one
SELECT pg_try_advisory_lock(@lock_key::bigint) AS "Locked";

-- name: ReleaseBlobSweepLock :one
SELECT pg_advisory_unlock(@lock_key::bigint) AS "Released";

-- name: UpsertBlob :one
INSERT INTO "Blobs" ("Digest", "SizeBytes", "StorageKey")
VALUES (@digest, @size_bytes, @storage_key)
ON CONFLICT ("Digest") WHERE NOT "IsDeleted"
DO UPDATE SET "LastUsedAt" = now(), "UpdatedAt" = now()
RETURNING *;

-- name: ClaimUpload :one
INSERT INTO "BlobUploads" ("OrganizationId", "LeaseId", "Kind", "Name")
VALUES (@organization_id, @lease_id, @kind, @name)
ON CONFLICT ("LeaseId", "Kind", "Name")
DO UPDATE SET "State" = 'uploading', "UpdatedAt" = now()
WHERE "BlobUploads"."State" = 'failed'
RETURNING "Id";

-- name: FinishUpload :exec
UPDATE "BlobUploads"
SET "State" = @state, "Digest" = sqlc.narg(digest), "UpdatedAt" = now()
WHERE "Id" = @id;

-- name: UpsertBlobOwner :exec
INSERT INTO "BlobOwners" ("OrganizationId", "Digest")
VALUES (@organization_id, @digest)
ON CONFLICT ("OrganizationId", "Digest") WHERE NOT "IsDeleted" DO NOTHING;

-- name: OwnedBlobSize :one
SELECT b."SizeBytes"
FROM "Blobs" b
JOIN "BlobOwners" o ON o."Digest" = b."Digest" AND o."OrganizationId" = @organization_id AND NOT o."IsDeleted"
WHERE b."Digest" = @digest AND NOT b."IsDeleted";

-- name: RetireBlobOwners :exec
UPDATE "BlobOwners"
SET "IsDeleted" = true, "UpdatedAt" = now()
WHERE "Digest" = ANY (@digests::bytea[]) AND NOT "IsDeleted";

-- name: KnownBlobs :many
SELECT "Digest"
FROM "Blobs"
WHERE "Digest" = ANY (@digests::bytea[]) AND NOT "IsDeleted";

-- name: LockOrganizationArtifacts :exec
SELECT pg_advisory_xact_lock(hashtextextended('dhow-artifacts:' || @organization_id::text, 0));

-- name: OrganizationArtifactBytes :one
SELECT COALESCE(sum(a."SizeBytes"), 0)::bigint AS "Bytes"
FROM "Artifacts" a
WHERE a."OrganizationId" = @organization_id AND NOT a."IsDeleted"
  AND (a."ExpiresAt" > now() OR EXISTS (SELECT 1
          FROM "DeploymentReleases" r
          JOIN "DeployTargets" t ON t."Id" = r."TargetId" AND t."OrganizationId" = r."OrganizationId"
          WHERE r."ArtifactDigest" = a."Digest" AND r."OrganizationId" = a."OrganizationId" AND t."ProjectId" = a."ProjectId"
            AND NOT a."Untrusted" AND a."ExpiresAt" > now() - make_interval(secs => @rollback_grace_seconds::float8)
            AND r."Kind" = 'deploy' AND r."State" = 'succeeded' AND NOT r."IsDeleted" AND NOT t."IsDeleted"
            AND (SELECT count(*)
                 FROM "DeploymentReleases" n
                 WHERE n."TargetId" = r."TargetId" AND n."OrganizationId" = r."OrganizationId"
                   AND n."Kind" = 'deploy' AND n."State" = 'succeeded' AND NOT n."IsDeleted"
                   AND (n."StartedAt", n."Id") > (r."StartedAt", r."Id")) < t."KeepReleases"));

-- name: ProjectUntrustedArtifactBytes :one
SELECT COALESCE(sum("SizeBytes"), 0)::bigint AS "Bytes"
FROM "Artifacts"
WHERE "ProjectId" = @project_id AND "Untrusted" AND NOT "IsDeleted" AND "ExpiresAt" > now();

-- name: TouchBlob :one
UPDATE "Blobs"
SET "LastUsedAt" = now(), "UpdatedAt" = now()
WHERE "Digest" = @digest AND NOT "IsDeleted"
RETURNING "SizeBytes";

-- name: BlobSize :one
SELECT "SizeBytes"
FROM "Blobs"
WHERE "Digest" = @digest AND NOT "IsDeleted";

-- name: RetireArtifact :exec
UPDATE "Artifacts"
SET "IsDeleted" = true, "UpdatedAt" = now()
WHERE "RunId" = @run_id AND "Name" = @name AND NOT "IsDeleted";

-- name: InsertArtifact :one
INSERT INTO "Artifacts" ("OrganizationId", "ProjectId", "RunId", "RunJobId", "RunJobKey", "JobId", "Name", "Digest", "SizeBytes",
                         "Untrusted", "ExpiresAt")
VALUES (@organization_id, @project_id, @run_id, @run_job_id, @run_job_key, @job_id, @name, @digest, @size_bytes, @untrusted,
        now() + make_interval(secs => @retention_seconds::float8))
RETURNING *;

-- name: ArtifactOwner :one
SELECT "RunJobKey"
FROM "Artifacts"
WHERE "RunId" = @run_id AND "Name" = @name AND NOT "IsDeleted";

-- name: ListRunJobArtifacts :many
SELECT *
FROM "Artifacts"
WHERE "OrganizationId" = @organization_id AND "RunId" = @run_id AND "RunJobKey" = ANY (@run_job_keys::text[])
  AND NOT "IsDeleted" AND "ExpiresAt" > now()
ORDER BY "Name";

-- name: ListRunArtifacts :many
SELECT a.*
FROM "Artifacts" a
JOIN "PipelineRuns" r ON r."Id" = a."RunId" AND r."OrganizationId" = a."OrganizationId"
WHERE a."OrganizationId" = @organization_id AND a."ProjectId" = @project_id AND a."RunId" = @run_id
  AND NOT a."IsDeleted" AND a."ExpiresAt" > now() AND NOT r."IsDeleted"
ORDER BY a."Name"
LIMIT @max_rows;

-- name: RunVisible :one
SELECT EXISTS (SELECT 1
               FROM "PipelineRuns"
               WHERE "Id" = @run_id AND "OrganizationId" = @organization_id AND "ProjectId" = @project_id
                 AND NOT "IsDeleted")::boolean AS "Visible";

-- name: GetArtifact :one
SELECT a.*
FROM "Artifacts" a
JOIN "PipelineRuns" r ON r."Id" = a."RunId" AND r."OrganizationId" = a."OrganizationId"
WHERE a."Id" = @id AND a."OrganizationId" = @organization_id AND a."ProjectId" = @project_id
  AND NOT a."IsDeleted" AND a."ExpiresAt" > now() AND NOT r."IsDeleted";

-- name: FindCacheEntry :one
UPDATE "CacheEntries"
SET "LastUsedAt" = now(), "UpdatedAt" = now()
WHERE "Id" = (SELECT c."Id"
              FROM "CacheEntries" c
              WHERE c."ProjectId" = @project_id AND c."Key" = @key AND c."Scope" = ANY (@scopes::text[]) AND NOT c."IsDeleted"
                AND (NOT @without_secrets::boolean OR NOT c."HadSecrets")
              ORDER BY array_position(@scopes::text[], c."Scope")
              LIMIT 1)
RETURNING *;

-- name: LockProjectCache :exec
SELECT pg_advisory_xact_lock(hashtextextended('dhow-cache:' || @project_id::text, 0));

-- name: InsertCacheEntry :one
INSERT INTO "CacheEntries" ("OrganizationId", "ProjectId", "Scope", "Key", "Digest", "SizeBytes", "HadSecrets")
VALUES (@organization_id, @project_id, @scope, @key, @digest, @size_bytes, @had_secrets)
ON CONFLICT ("ProjectId", "Scope", "Key") WHERE NOT "IsDeleted" DO NOTHING
RETURNING "Id";

-- name: ProjectCacheBytes :one
SELECT COALESCE(sum("SizeBytes"), 0)::bigint AS "Bytes"
FROM "CacheEntries"
WHERE "ProjectId" = @project_id AND NOT "IsDeleted";

-- name: OldestCacheEntries :many
SELECT "Id", "SizeBytes"
FROM "CacheEntries"
WHERE "ProjectId" = @project_id AND NOT "IsDeleted" AND (@include_protected::boolean OR "Scope" <> @protected_scope::text)
ORDER BY ("Scope" = @protected_scope::text), "LastUsedAt", "Id"
LIMIT @max_rows;

-- name: EvictCacheEntries :execrows
UPDATE "CacheEntries"
SET "IsDeleted" = true, "UpdatedAt" = now()
WHERE "Id" = ANY (@ids::uuid[]) AND NOT "IsDeleted";

-- name: ExpireArtifacts :execrows
UPDATE "Artifacts"
SET "IsDeleted" = true, "UpdatedAt" = now()
WHERE "Id" IN (SELECT a."Id"
               FROM "Artifacts" a
               WHERE a."ExpiresAt" <= now() AND NOT a."IsDeleted"
                 AND NOT EXISTS (SELECT 1
                         FROM "DeploymentReleases" r
                         JOIN "DeployTargets" t ON t."Id" = r."TargetId" AND t."OrganizationId" = r."OrganizationId"
                         WHERE r."ArtifactDigest" = a."Digest" AND r."OrganizationId" = a."OrganizationId" AND t."ProjectId" = a."ProjectId"
                           AND NOT a."Untrusted" AND a."ExpiresAt" > now() - make_interval(secs => @rollback_grace_seconds::float8)
                           AND r."Kind" = 'deploy' AND r."State" = 'succeeded' AND NOT r."IsDeleted" AND NOT t."IsDeleted"
                           AND (SELECT count(*)
                                FROM "DeploymentReleases" n
                                WHERE n."TargetId" = r."TargetId" AND n."OrganizationId" = r."OrganizationId"
                                  AND n."Kind" = 'deploy' AND n."State" = 'succeeded' AND NOT n."IsDeleted"
                                  AND (n."StartedAt", n."Id") > (r."StartedAt", r."Id")) < t."KeepReleases")
               LIMIT @batch
               FOR UPDATE SKIP LOCKED);

-- name: MarkUnusedBlobs :many
UPDATE "Blobs"
SET "IsDeleted" = true, "UpdatedAt" = now()
WHERE "Id" IN (SELECT b."Id"
               FROM "Blobs" b
               WHERE NOT b."IsDeleted" AND b."LastUsedAt" < @before
                 AND NOT EXISTS (SELECT 1 FROM "Artifacts" a WHERE a."Digest" = b."Digest" AND NOT a."IsDeleted")
                 AND NOT EXISTS (SELECT 1 FROM "CacheEntries" c WHERE c."Digest" = b."Digest" AND NOT c."IsDeleted")
                 AND NOT EXISTS (SELECT 1 FROM "IngestedArtifacts" i WHERE i."Digest" = b."Digest" AND NOT i."IsDeleted")
               ORDER BY b."LastUsedAt"
               LIMIT @batch
               FOR UPDATE SKIP LOCKED)
RETURNING "Digest";

-- name: ArtifactSize :one
SELECT "SizeBytes"
FROM "Artifacts"
WHERE "RunId" = @run_id AND "Name" = @name AND NOT "IsDeleted" AND "ExpiresAt" > now();

-- name: FindRollbackArtifact :one
SELECT a."Name", a."Digest", a."SizeBytes"
FROM "Artifacts" a
JOIN "Blobs" b ON b."Digest" = a."Digest" AND NOT b."IsDeleted"
WHERE a."OrganizationId" = @organization_id AND a."ProjectId" = @project_id AND a."Digest" = @digest
  AND NOT a."Untrusted" AND NOT a."IsDeleted" AND a."ExpiresAt" > now() - make_interval(secs => @rollback_grace_seconds::float8)
ORDER BY a."CreatedAt" DESC, a."Id" DESC
LIMIT 1;
