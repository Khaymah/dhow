-- name: CountVariablesInScope :one
SELECT count(*)::bigint
FROM "Variables"
WHERE "OrganizationId" = @organization_id
  AND "ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
  AND "EnvironmentId" IS NOT DISTINCT FROM sqlc.narg('environment_id')::uuid
  AND NOT "IsDeleted";

-- name: ListVariablesInScope :many
SELECT *
FROM "Variables"
WHERE "OrganizationId" = @organization_id
  AND "ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
  AND "EnvironmentId" IS NOT DISTINCT FROM sqlc.narg('environment_id')::uuid
  AND NOT "IsDeleted"
ORDER BY lower("Name")
LIMIT @page_limit OFFSET @page_offset;

-- name: GetVariableForUpdate :one
SELECT *
FROM "Variables"
WHERE "OrganizationId" = @organization_id
  AND "ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
  AND "EnvironmentId" IS NOT DISTINCT FROM sqlc.narg('environment_id')::uuid
  AND lower("Name") = lower(@name::text)
  AND NOT "IsDeleted"
FOR UPDATE;

-- name: InsertVariable :one
INSERT INTO "Variables" (
    "OrganizationId", "ProjectId", "EnvironmentId", "Scope", "Name", "Value", "CreatedByUserId", "UpdatedByUserId", "CreatedAt", "UpdatedAt"
)
VALUES (
    @organization_id, sqlc.narg('project_id')::uuid, sqlc.narg('environment_id')::uuid, @scope, @name, @value, @user_id, @user_id, @now, @now
)
RETURNING *;

-- name: UpdateVariable :one
UPDATE "Variables"
SET "Value" = @value, "UpdatedByUserId" = @user_id, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: RemoveVariable :one
UPDATE "Variables"
SET "IsDeleted" = true, "UpdatedByUserId" = @user_id, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: VisibleVariableNames :many
SELECT v."Scope", lower(v."Name")::text AS "LowerName"
FROM "Variables" v
WHERE v."OrganizationId" = @organization_id AND NOT v."IsDeleted"
  AND (v."Scope" = 'organization' OR (v."Scope" = 'project' AND v."ProjectId" = sqlc.narg('project_id')::uuid));

-- name: ResolveVariables :many
SELECT v."Scope", v."Name", v."Value"
FROM "Variables" v
WHERE v."OrganizationId" = @organization_id AND NOT v."IsDeleted"
  AND (
    v."Scope" = 'organization'
    OR (v."Scope" = 'project' AND v."ProjectId" = @project_id)
    OR (v."Scope" = 'environment' AND v."ProjectId" = @project_id AND v."EnvironmentId" = sqlc.narg('environment_id')::uuid)
  );

-- name: RemoveEnvironmentVariables :execrows
UPDATE "Variables"
SET "IsDeleted" = true, "UpdatedAt" = @now
WHERE "EnvironmentId" = @environment_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";
