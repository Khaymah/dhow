-- name: CountRegistryCredentialsInScope :one
SELECT count(*)::bigint
FROM "RegistryCredentials"
WHERE "OrganizationId" = @organization_id AND "ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid AND NOT "IsDeleted";

-- name: ListRegistryCredentialsInScope :many
SELECT "Id", "CreatedAt", "UpdatedAt", "OrganizationId", "ProjectId", "Host", "Username", "UpdatedByUserId"
FROM "RegistryCredentials"
WHERE "OrganizationId" = @organization_id AND "ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid AND NOT "IsDeleted"
ORDER BY "Host"
LIMIT @page_limit OFFSET @page_offset;

-- name: GetRegistryCredentialForUpdate :one
SELECT *
FROM "RegistryCredentials"
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
  AND NOT "IsDeleted"
FOR UPDATE;

-- name: InsertRegistryCredential :one
INSERT INTO "RegistryCredentials" ("Id", "OrganizationId", "ProjectId", "Host", "Username", "PasswordSealed", "CreatedByUserId", "UpdatedByUserId", "CreatedAt", "UpdatedAt")
VALUES (@id, @organization_id, sqlc.narg('project_id')::uuid, @host, @username, @password_sealed, @user_id, @user_id, @now, @now)
RETURNING "Id", "CreatedAt", "UpdatedAt", "OrganizationId", "ProjectId", "Host", "Username", "UpdatedByUserId";

-- name: UpdateRegistryCredential :one
UPDATE "RegistryCredentials"
SET "Username" = @username, "PasswordSealed" = @password_sealed, "UpdatedByUserId" = @user_id, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING "Id", "CreatedAt", "UpdatedAt", "OrganizationId", "ProjectId", "Host", "Username", "UpdatedByUserId";

-- name: RemoveRegistryCredential :one
UPDATE "RegistryCredentials"
SET "IsDeleted" = true, "PasswordSealed" = NULL, "UpdatedByUserId" = @user_id, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING "Id", "CreatedAt", "UpdatedAt", "OrganizationId", "ProjectId", "Host", "Username", "UpdatedByUserId";

-- name: ResolveRegistryCredential :one
SELECT "Id", "OrganizationId", "ProjectId", "Host", "Username", "PasswordSealed"
FROM "RegistryCredentials"
WHERE "OrganizationId" = @organization_id AND "Host" = @host AND NOT "IsDeleted" AND "PasswordSealed" IS NOT NULL
  AND ("ProjectId" IS NULL OR "ProjectId" = @project_id)
ORDER BY "ProjectId" NULLS LAST
LIMIT 1;
