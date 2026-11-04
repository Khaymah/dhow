-- name: GetSecretsProject :one
SELECT "State"
FROM "Projects"
WHERE "Id" = @project_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: GetSecretsEnvironment :one
SELECT "Id"
FROM "Environments"
WHERE "ProjectId" = @project_id AND "OrganizationId" = @organization_id AND "Name" = @name AND NOT "IsDeleted";

-- name: LockSecretsScope :exec
SELECT pg_advisory_xact_lock(hashtextextended(@scope_key::text, 0));

-- name: CountOrganizationProjects :one
SELECT count(*)::bigint
FROM "Projects"
WHERE "OrganizationId" = @organization_id AND "Id" = ANY(@project_ids::uuid[]) AND NOT "IsDeleted";

-- name: LockSecretsProject :one
SELECT "State"
FROM "Projects"
WHERE "Id" = @project_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
FOR SHARE;

-- name: LockSecretsEnvironment :one
SELECT "Id"
FROM "Environments"
WHERE "ProjectId" = @project_id AND "OrganizationId" = @organization_id AND "Name" = @name AND NOT "IsDeleted"
FOR SHARE;
