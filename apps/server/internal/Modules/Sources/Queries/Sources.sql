-- name: GetProjectState :one
SELECT "State"
FROM "Projects"
WHERE "Id" = @project_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
FOR SHARE;

-- name: GetRepositoryByProject :one
SELECT *
FROM "Repositories"
WHERE "ProjectId" = @project_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: LockRepositoryByProject :one
SELECT *
FROM "Repositories"
WHERE "ProjectId" = @project_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
FOR UPDATE;

-- name: InsertRepository :one
INSERT INTO "Repositories" (
    "Id", "OrganizationId", "ProjectId", "Provider", "FullName", "CloneUrl", "Transport", "DefaultBranch",
    "PendingHostKey", "PendingHostKeyFetchedAt", "DeployKeyPublic", "DeployKeySealed", "WebhookSecretSealed", "HookId",
    "CreatedByUserId", "CreatedAt", "UpdatedAt"
)
VALUES (
    @id, @organization_id, @project_id, @provider, @full_name, @clone_url, @transport, @default_branch,
    sqlc.narg(pending_host_key), sqlc.narg(pending_host_key_fetched_at), sqlc.narg(deploy_key_public), sqlc.narg(deploy_key_sealed),
    @webhook_secret_sealed, @hook_id, @created_by_user_id, @now, @now
)
RETURNING *;

-- name: UpdateRepository :one
UPDATE "Repositories"
SET "FullName"                   = @full_name,
    "CloneUrl"                   = @clone_url,
    "Transport"                  = @transport,
    "DefaultBranch"              = @default_branch,
    "HostKey"                    = sqlc.narg(host_key),
    "HostKeyConfirmedAt"         = sqlc.narg(host_key_confirmed_at),
    "HostKeyConfirmedByUserId"   = sqlc.narg(host_key_confirmed_by_user_id),
    "PendingHostKey"             = sqlc.narg(pending_host_key),
    "PendingHostKeyFetchedAt"    = sqlc.narg(pending_host_key_fetched_at),
    "DeployKeyPublic"            = sqlc.narg(deploy_key_public),
    "DeployKeySealed"            = sqlc.narg(deploy_key_sealed),
    "PreviousDeployKeyPublic"    = sqlc.narg(previous_deploy_key_public),
    "PreviousDeployKeySealed"    = sqlc.narg(previous_deploy_key_sealed),
    "PreviousDeployKeyExpiresAt" = sqlc.narg(previous_deploy_key_expires_at),
    "WebhookSecretSealed"        = @webhook_secret_sealed,
    "UpdatedAt"                  = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: UnlinkRepository :one
UPDATE "Repositories"
SET "IsDeleted" = true, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: ExpirePreviousDeployKeys :execrows
UPDATE "Repositories"
SET "PreviousDeployKeyPublic" = NULL, "PreviousDeployKeySealed" = NULL, "PreviousDeployKeyExpiresAt" = NULL, "UpdatedAt" = @now
WHERE "PreviousDeployKeyExpiresAt" <= @now AND NOT "IsDeleted";

-- name: GetRepositoryByHook :one
SELECT sqlc.embed(r), p."State" AS "ProjectState"
FROM "Repositories" r
JOIN "Projects" p ON p."Id" = r."ProjectId" AND p."OrganizationId" = r."OrganizationId"
WHERE r."HookId" = @hook_id AND r."Provider" = @provider AND NOT r."IsDeleted" AND NOT p."IsDeleted";

-- name: LockWebhookHook :exec
SELECT pg_advisory_xact_lock(hashtextextended('dhow.webhook:' || @provider::text || ':' || @hook_id::text, 0));

-- name: WebhookDeliverySeen :one
SELECT (
    EXISTS (
        SELECT 1
        FROM "WebhookDeliveries" d
        WHERE d."Provider" = @provider::text AND d."HookId" = @hook_id::text AND d."DeliveryId" = @delivery_id::text
          AND d."CreatedAt" >= @delivery_since::timestamptz
    )
    OR EXISTS (
        SELECT 1
        FROM "WebhookDeliveries" p
        WHERE p."Provider" = @provider::text AND p."HookId" = @hook_id::text AND p."PayloadDigest" = @payload_digest::bytea
          AND p."CreatedAt" >= @digest_since::timestamptz
    )
)::boolean;

-- name: InsertWebhookDelivery :one
INSERT INTO "WebhookDeliveries" (
    "Provider", "HookId", "DeliveryId", "PayloadDigest", "OrganizationId", "RepositoryId", "Event", "Ref", "Sha", "Outcome",
    "Reason", "CreatedAt", "UpdatedAt"
)
VALUES (
    @provider, @hook_id, @delivery_id, @payload_digest, sqlc.narg(organization_id), sqlc.narg(repository_id), @event,
    sqlc.narg(ref), sqlc.narg(sha), @outcome, sqlc.narg(reason), @now, @now
)
RETURNING "Id", "CreatedAt";

-- name: TouchRepositoryDelivery :exec
UPDATE "Repositories"
SET "LastDeliveryAt" = @now::timestamptz
WHERE "Id" = @id AND ("LastDeliveryAt" IS NULL OR "LastDeliveryAt" < @now::timestamptz);

-- name: TryLockWebhookPartitions :one
SELECT pg_try_advisory_xact_lock(hashtextextended('dhow.webhook-partitions', 0))::boolean;

-- name: EnsureWebhookPartitions :one
SELECT "EnsureWebhookPartitions"(@days_ahead::integer)::integer;

-- name: DropWebhookPartitions :one
SELECT "DropWebhookPartitions"(@retention_days::integer)::integer;

-- name: WebhookPartitionReady :one
SELECT "WebhookPartitionReady"()::boolean;

-- name: UpsertCommitStatus :one
INSERT INTO "CommitStatusReports" (
    "OrganizationId", "RepositoryId", "Sha", "RunId", "TargetUrl", "DesiredState", "DesiredAt", "NextAttemptAt", "CreatedAt", "UpdatedAt"
)
VALUES (@organization_id, @repository_id, @sha, @run_id, sqlc.narg(target_url), @desired_state, @now, @now, @now, @now)
ON CONFLICT ("RepositoryId", "RunId") WHERE NOT "IsDeleted" DO UPDATE
SET "DesiredState"  = EXCLUDED."DesiredState",
    "DesiredAt"     = EXCLUDED."DesiredAt",
    "TargetUrl"     = EXCLUDED."TargetUrl",
    "Attempts"      = 0,
    "LastError"     = NULL,
    "AbandonedAt"   = NULL,
    "NextAttemptAt" = LEAST("CommitStatusReports"."NextAttemptAt", EXCLUDED."NextAttemptAt"),
    "UpdatedAt"     = EXCLUDED."UpdatedAt"
WHERE "CommitStatusReports"."Sha" = EXCLUDED."Sha" AND "CommitStatusReports"."OrganizationId" = EXCLUDED."OrganizationId"
RETURNING *;

-- name: ClaimCommitStatuses :many
UPDATE "CommitStatusReports" c
SET "NextAttemptAt" = @lease_until, "UpdatedAt" = @now
FROM "Repositories" r
WHERE c."Id" IN (
    SELECT x."Id"
    FROM "CommitStatusReports" x
    WHERE NOT x."IsDeleted" AND x."AbandonedAt" IS NULL AND x."SentState" IS DISTINCT FROM x."DesiredState"
      AND x."NextAttemptAt" <= @now
    ORDER BY x."NextAttemptAt"
    LIMIT @batch
    FOR UPDATE SKIP LOCKED
)
  AND r."Id" = c."RepositoryId" AND r."OrganizationId" = c."OrganizationId"
RETURNING c."Id", c."OrganizationId", c."RepositoryId", c."Sha", c."RunId", c."TargetUrl", c."DesiredState", c."DesiredAt",
    c."Attempts", r."Provider", r."CloneUrl";

-- name: MarkCommitStatusSent :exec
UPDATE "CommitStatusReports"
SET "SentState" = @sent_state, "Attempts" = 0, "LastError" = NULL, "NextAttemptAt" = @now, "UpdatedAt" = @now
WHERE "Id" = @id;

-- name: MarkCommitStatusFailed :exec
UPDATE "CommitStatusReports"
SET "Attempts" = "Attempts" + 1, "LastError" = @last_error, "NextAttemptAt" = @next_attempt_at,
    "AbandonedAt" = sqlc.narg(abandoned_at), "UpdatedAt" = @now
WHERE "Id" = @id;
