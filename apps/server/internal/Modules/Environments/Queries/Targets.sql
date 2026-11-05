-- name: InsertDeployTarget :one
INSERT INTO "DeployTargets" (
    "OrganizationId", "ProjectId", "EnvironmentId", "Name", "Kind", "Username", "KeySecretName", "DeployPath", "KeepReleases",
    "Settings", "AllowedImages", "CreatedByUserId", "CreatedAt", "UpdatedAt"
)
VALUES (
    @organization_id, @project_id, @environment_id, @name, @kind, @username, sqlc.narg('key_secret_name')::text, @deploy_path,
    @keep_releases, @settings, @allowed_images::text[], @created_by_user_id, @now, @now
)
RETURNING *;

-- name: ListDeployTargets :many
SELECT *
FROM "DeployTargets"
WHERE "EnvironmentId" = @environment_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
ORDER BY "Name";

-- name: GetDeployTarget :one
SELECT *
FROM "DeployTargets"
WHERE "EnvironmentId" = @environment_id AND "OrganizationId" = @organization_id AND "Name" = @name AND NOT "IsDeleted";

-- name: LockDeployTarget :one
SELECT *
FROM "DeployTargets"
WHERE "EnvironmentId" = @environment_id AND "OrganizationId" = @organization_id AND "Name" = @name AND NOT "IsDeleted"
FOR UPDATE;

-- name: UpdateDeployTarget :one
UPDATE "DeployTargets"
SET "Username" = @username,
    "PublicKey" = CASE WHEN "KeySecretName" IS DISTINCT FROM sqlc.narg('key_secret_name')::text THEN NULL ELSE "PublicKey" END,
    "KeySecretName" = sqlc.narg('key_secret_name')::text, "DeployPath" = @deploy_path,
    "KeepReleases" = @keep_releases, "Settings" = @settings, "AllowedImages" = @allowed_images::text[],
    "Version" = "Version" + 1, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: SetDeployTargetKey :one
UPDATE "DeployTargets"
SET "KeySecretName" = @key_secret_name, "PublicKey" = @public_key, "Version" = "Version" + 1, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: TouchDeployTarget :one
UPDATE "DeployTargets"
SET "Version" = "Version" + 1, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: RemoveDeployTarget :one
UPDATE "DeployTargets"
SET "IsDeleted" = true, "Version" = "Version" + 1, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: RemoveEnvironmentDeployTargets :exec
UPDATE "DeployTargets"
SET "IsDeleted" = true, "Version" = "Version" + 1, "UpdatedAt" = @now
WHERE "EnvironmentId" = @environment_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: RemoveEnvironmentDeployTargetHosts :exec
UPDATE "DeployTargetHosts" h
SET "IsDeleted" = true, "UpdatedAt" = @now
FROM "DeployTargets" t
WHERE h."TargetId" = t."Id" AND h."OrganizationId" = t."OrganizationId"
  AND t."EnvironmentId" = @environment_id AND t."OrganizationId" = @organization_id AND NOT h."IsDeleted";

-- name: RemoveDeployTargetHosts :exec
UPDATE "DeployTargetHosts"
SET "IsDeleted" = true, "UpdatedAt" = @now
WHERE "TargetId" = @target_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: ListDeployTargetHosts :many
SELECT *
FROM "DeployTargetHosts"
WHERE "OrganizationId" = @organization_id AND "TargetId" = ANY(@target_ids::uuid[]) AND NOT "IsDeleted"
ORDER BY "TargetId", "Position";

-- name: InsertDeployTargetHost :one
INSERT INTO "DeployTargetHosts" ("OrganizationId", "TargetId", "Position", "Host", "Port", "DeployPath", "CreatedAt", "UpdatedAt")
VALUES (@organization_id, @target_id, @position, @host, @port, @deploy_path, @now, @now)
RETURNING *;

-- name: LockDeployTargetHost :one
SELECT *
FROM "DeployTargetHosts"
WHERE "Id" = @id AND "TargetId" = @target_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
FOR UPDATE;

-- name: UpdateDeployTargetHostAddress :one
UPDATE "DeployTargetHosts"
SET "Host" = @host, "Port" = @port, "HostKey" = NULL, "HostKeyFingerprint" = NULL, "HostKeyConfirmedAt" = NULL,
    "HostKeyConfirmedBy" = NULL, "PendingHostKey" = NULL, "PendingFingerprint" = NULL, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: RemoveDeployTargetHost :execrows
UPDATE "DeployTargetHosts"
SET "IsDeleted" = true, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: SyncDeployTargetHostPaths :exec
UPDATE "DeployTargetHosts"
SET "DeployPath" = @deploy_path, "UpdatedAt" = @now
WHERE "TargetId" = @target_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted" AND "DeployPath" <> @deploy_path;

-- name: ShiftDeployTargetHostPositions :exec
UPDATE "DeployTargetHosts"
SET "Position" = "Position" + 100
WHERE "TargetId" = @target_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: SetDeployTargetHostPositions :exec
UPDATE "DeployTargetHosts" h
SET "Position" = o.pos::integer
FROM unnest(@host_ids::uuid[]) WITH ORDINALITY AS o(id, pos)
WHERE h."Id" = o.id AND h."TargetId" = @target_id AND h."OrganizationId" = @organization_id AND NOT h."IsDeleted";

-- name: SetDeployTargetHostPendingKey :one
UPDATE "DeployTargetHosts"
SET "PendingHostKey" = @pending_host_key, "PendingFingerprint" = @pending_fingerprint, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: ConfirmDeployTargetHostKey :one
UPDATE "DeployTargetHosts"
SET "HostKey" = "PendingHostKey", "HostKeyFingerprint" = "PendingFingerprint", "HostKeyConfirmedAt" = @now::timestamptz,
    "HostKeyConfirmedBy" = @confirmed_by, "PendingHostKey" = NULL, "PendingFingerprint" = NULL, "UpdatedAt" = @now::timestamptz
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "PendingHostKey" IS NOT NULL AND NOT "IsDeleted"
RETURNING *;

-- name: EnvironmentSecretExists :one
SELECT EXISTS (
    SELECT 1
    FROM "Secrets"
    WHERE "OrganizationId" = @organization_id AND "EnvironmentId" = @environment_id AND "Scope" = 'environment'
      AND lower("Name") = lower(@name::text) AND NOT "IsDeleted"
)::boolean;
