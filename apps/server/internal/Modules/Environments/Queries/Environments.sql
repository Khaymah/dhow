-- name: GetEnvironmentProject :one
SELECT p."State",
       COALESCE((SELECT r."DefaultBranch"
                 FROM "Repositories" r
                 WHERE r."ProjectId" = p."Id" AND r."OrganizationId" = p."OrganizationId" AND NOT r."IsDeleted"
                 LIMIT 1), '')::text AS "DefaultBranch"
FROM "Projects" p
WHERE p."Id" = @project_id AND p."OrganizationId" = @organization_id AND NOT p."IsDeleted";

-- name: InsertEnvironment :one
INSERT INTO "Environments" (
    "OrganizationId", "ProjectId", "Name", "Description", "AllowedRefs", "AllowPullRequests", "RequiredApprovals",
    "AllowSelfApproval", "WaitMinutes", "ApprovalTimeoutHours", "RequirePinnedImages", "AllowedPlugins", "RequiredIsolation",
    "AllowAdminBypass", "SupersedePending", "AllowExtraDeployEgress", "CreatedByUserId", "CreatedAt", "UpdatedAt"
)
VALUES (
    @organization_id, @project_id, @name, @description, @allowed_refs, @allow_pull_requests, @required_approvals,
    @allow_self_approval, @wait_minutes, @approval_timeout_hours, @require_pinned_images, @allowed_plugins::text[], @required_isolation,
    @allow_admin_bypass, @supersede_pending, @allow_extra_deploy_egress, @created_by_user_id, @now, @now
)
RETURNING *;

-- name: CountEnvironments :one
SELECT count(*)
FROM "Environments"
WHERE "ProjectId" = @project_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: ListEnvironments :many
SELECT *
FROM "Environments"
WHERE "ProjectId" = @project_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
ORDER BY "Name"
LIMIT @page_limit OFFSET @page_offset;

-- name: GetEnvironment :one
SELECT *
FROM "Environments"
WHERE "ProjectId" = @project_id AND "OrganizationId" = @organization_id AND "Name" = @name AND NOT "IsDeleted";

-- name: LockEnvironment :one
SELECT *
FROM "Environments"
WHERE "ProjectId" = @project_id AND "OrganizationId" = @organization_id AND "Name" = @name AND NOT "IsDeleted"
FOR UPDATE;

-- name: UpdateEnvironmentRules :one
UPDATE "Environments"
SET "Description" = @description, "AllowedRefs" = @allowed_refs, "AllowPullRequests" = @allow_pull_requests,
    "RequiredApprovals" = @required_approvals, "AllowSelfApproval" = @allow_self_approval, "WaitMinutes" = @wait_minutes,
    "ApprovalTimeoutHours" = @approval_timeout_hours, "RequirePinnedImages" = @require_pinned_images,
    "AllowedPlugins" = @allowed_plugins::text[], "RequiredIsolation" = @required_isolation, "AllowAdminBypass" = @allow_admin_bypass,
    "SupersedePending" = @supersede_pending, "AllowExtraDeployEgress" = @allow_extra_deploy_egress,
    "RuleVersion" = "RuleVersion" + CASE WHEN @rules_changed::boolean THEN 1 ELSE 0 END,
    "Version" = "Version" + 1, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: PauseEnvironment :one
UPDATE "Environments"
SET "PausedAt" = @now::timestamptz, "PausedByUserId" = @paused_by_user_id, "PausedReason" = @paused_reason,
    "RuleVersion" = "RuleVersion" + 1, "Version" = "Version" + 1, "UpdatedAt" = @now::timestamptz
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "PausedAt" IS NULL AND NOT "IsDeleted"
RETURNING *;

-- name: ResumeEnvironment :one
UPDATE "Environments"
SET "PausedAt" = NULL, "PausedByUserId" = NULL, "PausedReason" = NULL,
    "RuleVersion" = "RuleVersion" + 1, "Version" = "Version" + 1, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "PausedAt" IS NOT NULL AND NOT "IsDeleted"
RETURNING *;

-- name: RemoveEnvironment :one
UPDATE "Environments"
SET "IsDeleted" = true, "Version" = "Version" + 1, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: ListEnvironmentReviewers :many
SELECT "EnvironmentId", "UserId", "TeamId"
FROM "EnvironmentReviewers"
WHERE "OrganizationId" = @organization_id AND "EnvironmentId" = ANY(@environment_ids::uuid[]) AND NOT "IsDeleted"
ORDER BY "EnvironmentId", "Id";

-- name: InsertEnvironmentReviewer :exec
INSERT INTO "EnvironmentReviewers" ("OrganizationId", "EnvironmentId", "UserId", "TeamId", "CreatedAt", "UpdatedAt")
VALUES (@organization_id, @environment_id, @user_id, @team_id, @now, @now);

-- name: RemoveEnvironmentReviewers :exec
UPDATE "EnvironmentReviewers"
SET "IsDeleted" = true, "UpdatedAt" = @now
WHERE "OrganizationId" = @organization_id AND "EnvironmentId" = @environment_id AND NOT "IsDeleted";

-- name: ReviewerUserRoles :many
SELECT m."UserId", m."Role",
       ARRAY(
           SELECT rb."Role"
           FROM "RoleBindings" rb
           WHERE rb."OrganizationId" = m."OrganizationId" AND rb."Scope" = 'project' AND rb."ProjectId" = @project_id
             AND NOT rb."IsDeleted"
             AND (rb."UserId" = m."UserId"
                  OR EXISTS (SELECT 1
                             FROM "TeamMembers" tm
                             JOIN "Teams" t ON t."Id" = tm."TeamId" AND t."OrganizationId" = tm."OrganizationId" AND NOT t."IsDeleted"
                             WHERE tm."TeamId" = rb."TeamId" AND tm."OrganizationId" = rb."OrganizationId"
                               AND tm."UserId" = m."UserId" AND NOT tm."IsDeleted"))
           ORDER BY rb."Role"
       )::text[] AS "Bindings"
FROM "Memberships" m
JOIN "Users" u ON u."Id" = m."UserId" AND NOT u."IsDeleted"
WHERE m."OrganizationId" = @organization_id AND m."UserId" = ANY(@user_ids::uuid[]) AND NOT m."IsDeleted";

-- name: ReviewerTeams :many
SELECT "Id"
FROM "Teams"
WHERE "OrganizationId" = @organization_id AND "Id" = ANY(@team_ids::uuid[]) AND NOT "IsDeleted";
