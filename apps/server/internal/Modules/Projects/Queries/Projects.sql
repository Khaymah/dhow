-- name: InsertProject :one
INSERT INTO "Projects" ("OrganizationId", "Slug", "Name", "PipelinePath", "ForkPolicy", "CreatedByUserId", "CreatedAt", "UpdatedAt")
VALUES (@organization_id, @slug, @name, @pipeline_path, @fork_policy, @created_by_user_id, @now, @now)
RETURNING *;

-- name: CountProjects :one
SELECT count(*)
FROM "Projects"
WHERE "OrganizationId" = @organization_id AND NOT "IsDeleted"
  AND (@state::text = '' OR "State" = @state::text);

-- name: ListProjects :many
SELECT *
FROM "Projects"
WHERE "OrganizationId" = @organization_id AND NOT "IsDeleted"
  AND (@state::text = '' OR "State" = @state::text)
ORDER BY lower("Name"), "Id"
LIMIT @page_limit OFFSET @page_offset;

-- name: GetProject :one
SELECT *
FROM "Projects"
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: LockProject :one
SELECT *
FROM "Projects"
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
FOR UPDATE;

-- name: UpdateProject :one
UPDATE "Projects"
SET "Name" = @name, "PipelinePath" = @pipeline_path, "ForkPolicy" = @fork_policy, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "State" = 'active' AND NOT "IsDeleted"
RETURNING *;

-- name: ArchiveProject :one
UPDATE "Projects"
SET "State" = 'archived', "ArchivedAt" = @now::timestamptz, "UpdatedAt" = @now::timestamptz
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "State" = 'active' AND NOT "IsDeleted"
RETURNING *;

-- name: GetProjectBindings :one
SELECT p."Id",
       ARRAY(
           SELECT rb."Role"
           FROM "RoleBindings" rb
           WHERE rb."OrganizationId" = p."OrganizationId" AND rb."Scope" = 'project' AND rb."ProjectId" = p."Id"
             AND NOT rb."IsDeleted"
             AND (rb."UserId" = @user_id
                  OR EXISTS (SELECT 1
                             FROM "TeamMembers" tm
                             JOIN "Teams" t ON t."Id" = tm."TeamId" AND t."OrganizationId" = tm."OrganizationId" AND NOT t."IsDeleted"
                             WHERE tm."TeamId" = rb."TeamId" AND tm."OrganizationId" = rb."OrganizationId"
                               AND tm."UserId" = @user_id AND NOT tm."IsDeleted"))
           ORDER BY rb."Role"
       )::text[] AS "Roles"
FROM "Projects" p
WHERE p."Id" = @project_id AND p."OrganizationId" = @organization_id AND NOT p."IsDeleted";

-- name: LinkedProjects :many
SELECT "ProjectId"
FROM "Repositories"
WHERE "OrganizationId" = @organization_id AND "ProjectId" = ANY(@project_ids::uuid[]) AND NOT "IsDeleted";
