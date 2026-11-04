-- name: CountSecretsInScope :one
SELECT count(*)::bigint
FROM "Secrets"
WHERE "OrganizationId" = @organization_id
  AND "ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
  AND "EnvironmentId" IS NOT DISTINCT FROM sqlc.narg('environment_id')::uuid
  AND NOT "IsDeleted";

-- name: ListSecretsInScope :many
SELECT sqlc.embed(s), v."SizeBytes"
FROM "Secrets" s
JOIN "SecretVersions" v ON v."SecretId" = s."Id" AND v."Version" = s."CurrentVersion"
WHERE s."OrganizationId" = @organization_id
  AND s."ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
  AND s."EnvironmentId" IS NOT DISTINCT FROM sqlc.narg('environment_id')::uuid
  AND NOT s."IsDeleted"
ORDER BY lower(s."Name")
LIMIT @page_limit OFFSET @page_offset;

-- name: GetSecretForUpdate :one
SELECT *
FROM "Secrets"
WHERE "OrganizationId" = @organization_id
  AND "ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
  AND "EnvironmentId" IS NOT DISTINCT FROM sqlc.narg('environment_id')::uuid
  AND lower("Name") = lower(@name::text)
  AND NOT "IsDeleted"
FOR UPDATE;

-- name: InsertSecret :one
INSERT INTO "Secrets" (
    "OrganizationId", "ProjectId", "EnvironmentId", "Scope", "Name", "Description", "Visibility", "AvailableToPullRequests",
    "CreatedByUserId", "UpdatedByUserId", "CreatedAt", "UpdatedAt"
)
VALUES (
    @organization_id, sqlc.narg('project_id')::uuid, sqlc.narg('environment_id')::uuid, @scope, @name, @description, @visibility,
    @available_to_pull_requests, @user_id, @user_id, @now, @now
)
RETURNING *;

-- name: UpdateSecret :one
UPDATE "Secrets"
SET "Description" = @description, "Visibility" = @visibility, "AvailableToPullRequests" = @available_to_pull_requests,
    "CurrentVersion" = @current_version, "UpdatedByUserId" = @user_id, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: RemoveSecret :one
UPDATE "Secrets"
SET "IsDeleted" = true, "UpdatedByUserId" = @user_id, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: InsertSecretVersion :exec
INSERT INTO "SecretVersions" ("OrganizationId", "SecretId", "Version", "ValueSealed", "SizeBytes", "CreatedByUserId", "CreatedAt", "UpdatedAt")
VALUES (@organization_id, @secret_id, @version, @value_sealed, @size_bytes, @user_id, @now, @now);

-- name: SupersedeSecretVersions :exec
UPDATE "SecretVersions"
SET "SupersededAt" = @now::timestamptz, "UpdatedAt" = @now::timestamptz
WHERE "SecretId" = @secret_id AND "Version" < @version AND "SupersededAt" IS NULL;

-- name: EraseSecretVersionsBelow :execrows
UPDATE "SecretVersions"
SET "ValueSealed" = NULL, "ErasedAt" = @now::timestamptz, "UpdatedAt" = @now::timestamptz
WHERE "SecretId" = @secret_id AND "Version" <= @version AND "ValueSealed" IS NOT NULL;

-- name: EraseAllSecretVersions :execrows
UPDATE "SecretVersions"
SET "ValueSealed" = NULL, "ErasedAt" = @now::timestamptz, "UpdatedAt" = @now::timestamptz
WHERE "SecretId" = @secret_id AND "ValueSealed" IS NOT NULL;

-- name: GetSecretVersion :one
SELECT "Version", "ValueSealed", "SizeBytes"
FROM "SecretVersions"
WHERE "SecretId" = @secret_id AND "Version" = @version;

-- name: ListSecretProjectAccess :many
SELECT "SecretId", "ProjectId"
FROM "SecretProjectAccess"
WHERE "SecretId" = ANY(@secret_ids::uuid[]) AND NOT "IsDeleted"
ORDER BY "SecretId", "ProjectId";

-- name: ClearSecretProjectAccess :exec
UPDATE "SecretProjectAccess"
SET "IsDeleted" = true, "UpdatedAt" = @now
WHERE "SecretId" = @secret_id AND NOT "IsDeleted";

-- name: InsertSecretProjectAccess :exec
INSERT INTO "SecretProjectAccess" ("OrganizationId", "SecretId", "ProjectId", "CreatedAt", "UpdatedAt")
SELECT @organization_id, @secret_id, p, @now, @now
FROM unnest(@project_ids::uuid[]) AS p;

-- name: VisibleSecretNames :many
SELECT s."Scope", lower(s."Name")::text AS "LowerName"
FROM "Secrets" s
WHERE s."OrganizationId" = @organization_id AND NOT s."IsDeleted"
  AND (
    (s."Scope" = 'organization' AND (s."Visibility" = 'all_projects' OR EXISTS (
        SELECT 1 FROM "SecretProjectAccess" a
        WHERE a."SecretId" = s."Id" AND a."ProjectId" = sqlc.narg('project_id')::uuid AND NOT a."IsDeleted")))
    OR (s."Scope" = 'project' AND s."ProjectId" = sqlc.narg('project_id')::uuid)
  );

-- name: ResolveSecrets :many
SELECT s."Id", s."Scope", s."Name", s."CurrentVersion", s."AvailableToPullRequests", s."OrganizationId", s."ProjectId", s."EnvironmentId",
       v."ValueSealed", v."SizeBytes"
FROM "Secrets" s
JOIN "SecretVersions" v ON v."SecretId" = s."Id" AND v."Version" = s."CurrentVersion"
WHERE s."OrganizationId" = @organization_id AND NOT s."IsDeleted" AND lower(s."Name") = ANY(@lower_names::text[])
  AND (
    (s."Scope" = 'organization' AND (s."Visibility" = 'all_projects' OR EXISTS (
        SELECT 1 FROM "SecretProjectAccess" a
        WHERE a."SecretId" = s."Id" AND a."ProjectId" = @project_id AND NOT a."IsDeleted")))
    OR (s."Scope" = 'project' AND s."ProjectId" = @project_id)
    OR (s."Scope" = 'environment' AND s."ProjectId" = @project_id AND s."EnvironmentId" = sqlc.narg('environment_id')::uuid)
  );

-- name: TouchSecretsUsed :exec
UPDATE "Secrets"
SET "LastUsedAt" = @now::timestamptz
WHERE "Id" = ANY(@ids::uuid[]) AND NOT "IsDeleted";

-- name: RemoveEnvironmentSecrets :many
UPDATE "Secrets"
SET "IsDeleted" = true, "UpdatedAt" = @now
WHERE "EnvironmentId" = @environment_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING "Id";

-- name: EraseSecretVersionsOf :execrows
UPDATE "SecretVersions"
SET "ValueSealed" = NULL, "ErasedAt" = @now::timestamptz, "UpdatedAt" = @now::timestamptz
WHERE "SecretId" = ANY(@secret_ids::uuid[]) AND "ValueSealed" IS NOT NULL;
