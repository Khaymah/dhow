-- name: LiveApproval :one
SELECT *
FROM "DeploymentApprovals"
WHERE "RunId" = @run_id AND "EnvironmentId" = @environment_id AND "Round" = @round AND "OrganizationId" = @organization_id
  AND "State" IN ('pending', 'approved', 'bypassed') AND NOT "IsDeleted"
FOR UPDATE;

-- name: LatestApproval :one
SELECT *
FROM "DeploymentApprovals"
WHERE "RunId" = @run_id AND "EnvironmentId" = @environment_id AND "Round" = @round AND "OrganizationId" = @organization_id
  AND NOT "IsDeleted"
ORDER BY "CreatedAt" DESC, "Id" DESC
LIMIT 1;

-- name: InsertApproval :one
INSERT INTO "DeploymentApprovals" (
    "OrganizationId", "EnvironmentId", "RunId", "RunNumber", "Ref", "Round", "RuleVersion", "ExpiresAt", "CreatedAt", "UpdatedAt"
)
VALUES (@organization_id, @environment_id, @run_id, @run_number, @ref, @round, @rule_version, @expires_at, @now, @now)
RETURNING *;

-- name: CloseApproval :execrows
UPDATE "DeploymentApprovals"
SET "State" = @state, "DecidedAt" = @now::timestamptz, "SupersededByRunId" = sqlc.narg('superseded_by_run_id')::uuid,
    "UpdatedAt" = @now::timestamptz
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "State" = 'pending';

-- name: SupersedeDecidedApproval :execrows
UPDATE "DeploymentApprovals"
SET "State" = 'superseded', "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "State" IN ('approved', 'bypassed');

-- name: SupersedeOlderPending :many
UPDATE "DeploymentApprovals"
SET "State" = 'superseded', "DecidedAt" = @now::timestamptz, "SupersededByRunId" = @run_id::uuid, "UpdatedAt" = @now::timestamptz
WHERE "EnvironmentId" = @environment_id AND "OrganizationId" = @organization_id AND "Ref" = @ref AND "State" = 'pending'
  AND "RunNumber" < @run_number AND "RunId" <> @run_id AND NOT "IsDeleted"
RETURNING "Id", "RunId";

-- name: CancelRunApprovals :exec
UPDATE "DeploymentApprovals"
SET "State" = 'canceled', "DecidedAt" = @now::timestamptz, "UpdatedAt" = @now::timestamptz
WHERE "RunId" = @run_id AND "OrganizationId" = @organization_id AND "State" = 'pending' AND NOT "IsDeleted";

-- name: IsEnvironmentReviewer :one
SELECT EXISTS (SELECT 1
               FROM "EnvironmentReviewers" r
               WHERE r."EnvironmentId" = @environment_id AND r."OrganizationId" = @organization_id AND NOT r."IsDeleted"
                 AND (r."UserId" = @user_id
                      OR EXISTS (SELECT 1
                                 FROM "TeamMembers" tm
                                 JOIN "Teams" t ON t."Id" = tm."TeamId" AND t."OrganizationId" = tm."OrganizationId" AND NOT t."IsDeleted"
                                 WHERE tm."TeamId" = r."TeamId" AND tm."OrganizationId" = r."OrganizationId"
                                   AND tm."UserId" = @user_id AND NOT tm."IsDeleted")))::boolean;

-- name: LockApproval :one
SELECT a.*, e."ProjectId", e."Name" AS "EnvironmentName", e."RuleVersion" AS "CurrentRuleVersion",
       e."AllowSelfApproval", e."AllowAdminBypass", (e."PausedAt" IS NOT NULL)::boolean AS "Paused"
FROM "DeploymentApprovals" a
JOIN "Environments" e ON e."Id" = a."EnvironmentId" AND e."OrganizationId" = a."OrganizationId" AND NOT e."IsDeleted"
WHERE a."Id" = @id AND a."RunId" = @run_id AND a."OrganizationId" = @organization_id AND e."ProjectId" = @project_id
  AND NOT a."IsDeleted"
FOR UPDATE OF a;

-- name: DecideApproval :execrows
UPDATE "DeploymentApprovals"
SET "State" = @state, "DecidedByUserId" = @user_id::uuid, "DecidedAt" = @now::timestamptz, "FactorKind" = @factor_kind::text,
    "Comment" = sqlc.narg('comment')::text, "BypassReason" = sqlc.narg('bypass_reason')::text, "UpdatedAt" = @now::timestamptz
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "State" = 'pending';

-- name: ApprovalRun :one
SELECT r."TriggeredByUserId", r."Actor", r."Trigger", r."State", r."ProjectId",
       COALESCE((SELECT repo."CloneUrl"
                 FROM "Repositories" repo
                 WHERE repo."Id" = r."RepositoryId" AND repo."OrganizationId" = r."OrganizationId"), '')::text AS "CloneUrl"
FROM "PipelineRuns" r
WHERE r."Id" = @run_id AND r."OrganizationId" = @organization_id AND NOT r."IsDeleted";

-- name: UserLogins :many
SELECT "Issuer", "Login"
FROM "UserIdentities"
WHERE "UserId" = @user_id AND "Login" IS NOT NULL AND NOT "IsDeleted";

-- name: ListRunApprovals :many
SELECT a.*, e."Name" AS "EnvironmentName"
FROM "DeploymentApprovals" a
JOIN "Environments" e ON e."Id" = a."EnvironmentId" AND e."OrganizationId" = a."OrganizationId"
WHERE a."RunId" = @run_id AND a."OrganizationId" = @organization_id AND e."ProjectId" = @project_id AND NOT a."IsDeleted"
ORDER BY a."CreatedAt", a."Id";

-- name: ListPendingApprovals :many
SELECT a.*, e."Name" AS "EnvironmentName", e."ProjectId"
FROM "DeploymentApprovals" a
JOIN "Environments" e ON e."Id" = a."EnvironmentId" AND e."OrganizationId" = a."OrganizationId" AND NOT e."IsDeleted"
WHERE a."OrganizationId" = @organization_id AND a."State" = 'pending' AND a."ExpiresAt" > @now AND NOT a."IsDeleted"
  AND e."ProjectId" = ANY(@project_ids::uuid[])
ORDER BY a."CreatedAt", a."Id"
LIMIT @page_limit;

-- name: EnvironmentAllowsSelfApproval :one
SELECT "AllowSelfApproval"
FROM "Environments"
WHERE "Id" = @id AND "OrganizationId" = @organization_id;

-- name: OrganizationProjectIDs :many
SELECT "Id"
FROM "Projects"
WHERE "OrganizationId" = @organization_id AND NOT "IsDeleted";
