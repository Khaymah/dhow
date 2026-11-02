-- name: InsertRegistrationToken :one
INSERT INTO "RunnerRegistrationTokens" ("OrganizationId", "CreatedByUserId", "Description", "TokenHash", "Labels", "MaxUses",
                                        "ExpiresAt", "CreatedAt", "UpdatedAt")
VALUES (@organization_id, @created_by_user_id, @description, @token_hash, @labels::text[], @max_uses, @expires_at, @now, @now)
RETURNING *;

-- name: CountRegistrationTokens :one
SELECT count(*)
FROM "RunnerRegistrationTokens"
WHERE "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: ListRegistrationTokens :many
SELECT *
FROM "RunnerRegistrationTokens"
WHERE "OrganizationId" = @organization_id AND NOT "IsDeleted"
ORDER BY "CreatedAt" DESC, "Id" DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetRegistrationToken :one
SELECT *
FROM "RunnerRegistrationTokens"
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: RevokeRegistrationToken :one
UPDATE "RunnerRegistrationTokens"
SET "RevokedAt" = @now::timestamptz, "RevokedByUserId" = @user_id::uuid, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted" AND "RevokedAt" IS NULL
RETURNING *;

-- name: ConsumeRegistrationToken :one
UPDATE "RunnerRegistrationTokens" t
SET "Uses" = t."Uses" + 1, "LastUsedAt" = @now::timestamptz, "UpdatedAt" = @now
WHERE t."TokenHash" = @token_hash AND t."Uses" < t."MaxUses" AND t."ExpiresAt" > @now AND t."RevokedAt" IS NULL
  AND NOT t."IsDeleted"
  AND EXISTS (SELECT 1 FROM "Organizations" o WHERE o."Id" = t."OrganizationId" AND NOT o."IsDeleted")
RETURNING t."Id", t."OrganizationId", t."Labels";

-- name: InsertRunner :one
INSERT INTO "Runners" ("OrganizationId", "RegistrationTokenId", "Name", "Labels", "CapabilityLabels", "Os", "Arch", "Version",
                       "IsolationLevels", "Capacity", "SigningPublicKey", "EncryptionPublicKey", "KeysRotatedAt",
                       "LastSeenAt", "LastIpAddress", "Features", "CreatedAt", "UpdatedAt")
VALUES (@organization_id, @registration_token_id, @name, @labels::text[], @capability_labels::text[], @os, @arch, @version,
        @isolation_levels::text[], @capacity, @signing_public_key, @encryption_public_key, @now, @now, @ip_address,
        @features::text[], @now, @now)
RETURNING *;

-- name: CountRunners :one
SELECT count(*)
FROM "Runners"
WHERE "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: ListRunners :many
SELECT *
FROM "Runners"
WHERE "OrganizationId" = @organization_id AND NOT "IsDeleted"
ORDER BY "CreatedAt" DESC, "Id" DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetRunner :one
SELECT *
FROM "Runners"
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: UpdateRunner :one
UPDATE "Runners"
SET "Name"           = COALESCE(sqlc.narg(name)::text, "Name"),
    "Labels"         = COALESCE(sqlc.narg(labels)::text[], "Labels"),
    "State"          = CASE
                           WHEN sqlc.narg(disabled)::boolean IS NULL THEN "State"
                           WHEN sqlc.narg(disabled)::boolean THEN 'disabled'
                           ELSE 'active' END,
    "DisabledReason" = CASE
                           WHEN sqlc.narg(disabled)::boolean IS NULL THEN "DisabledReason"
                           WHEN sqlc.narg(disabled)::boolean AND "State" = 'disabled' THEN "DisabledReason"
                           WHEN sqlc.narg(disabled)::boolean THEN 'manual'
                           ELSE '' END,
    "UpdatedAt"      = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted" AND "State" <> 'revoked'
RETURNING *;

-- name: RevokeRunner :one
UPDATE "Runners"
SET "State" = 'revoked', "DisabledReason" = '', "RevokedAt" = @now::timestamptz, "UpdatedAt" = @now
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted" AND "State" <> 'revoked'
RETURNING *;

-- name: RevokeRunnerSessions :execrows
UPDATE "RunnerSessions"
SET "RevokedAt" = @now::timestamptz, "UpdatedAt" = @now
WHERE "RunnerId" = @runner_id AND "RevokedAt" IS NULL;

-- name: GetRunnerIdentity :one
SELECT "Id", "OrganizationId", "State", "DisabledReason", "SigningPublicKey", "KeyVersion"
FROM "Runners"
WHERE "Id" = @id AND NOT "IsDeleted";

-- name: LockRunnerIdentity :one
SELECT "Id", "OrganizationId", "State", "DisabledReason", "SigningPublicKey", "KeyVersion"
FROM "Runners"
WHERE "Id" = @id AND NOT "IsDeleted"
FOR UPDATE;

-- name: GetActiveRunnerSession :one
SELECT "Id", "ExpiresAt"
FROM "RunnerSessions"
WHERE "RunnerId" = @runner_id AND "RevokedAt" IS NULL AND NOT "IsDeleted";

-- name: RevokeRunnerSession :exec
UPDATE "RunnerSessions"
SET "RevokedAt" = @now::timestamptz, "UpdatedAt" = @now
WHERE "Id" = @id AND "RevokedAt" IS NULL;

-- name: InsertRunnerSession :one
INSERT INTO "RunnerSessions" ("RunnerId", "TokenHash", "ChallengeNonce", "KeyVersion", "ExpiresAt", "ReplacedSessionId",
                              "IpAddress", "CreatedAt", "UpdatedAt")
VALUES (@runner_id, @token_hash, @challenge_nonce, @key_version, @expires_at, sqlc.narg(replaced_session_id), @ip_address,
        @now, @now)
ON CONFLICT ("ChallengeNonce") DO NOTHING
RETURNING "Id";

-- name: CountRunnerSessionSwitches :one
SELECT count(*)
FROM "RunnerSessions"
WHERE "RunnerId" = @runner_id AND "ReplacedSessionId" IS NOT NULL AND "CreatedAt" > @since;

-- name: DisableRunnerAsClone :execrows
UPDATE "Runners"
SET "State" = 'disabled', "DisabledReason" = 'clone_suspected', "UpdatedAt" = @now
WHERE "Id" = @id AND "State" <> 'revoked' AND NOT "IsDeleted";

-- name: TouchRunner :exec
UPDATE "Runners"
SET "LastSeenAt" = @now::timestamptz, "LastIpAddress" = @ip_address, "UpdatedAt" = @now
WHERE "Id" = @id AND ("LastSeenAt" IS NULL OR "LastSeenAt" <= @touch_before::timestamptz);

-- name: ResolveRunnerSession :one
SELECT s."Id"          AS "SessionId",
       s."ExpiresAt"   AS "SessionExpiresAt",
       s."RevokedAt"   AS "SessionRevokedAt",
       s."KeyVersion"  AS "SessionKeyVersion",
       r."Id",
       r."OrganizationId",
       r."Name",
       r."Labels",
       r."CapabilityLabels",
       r."IsolationLevels",
       r."Capacity",
       r."SigningPublicKey",
       r."EncryptionPublicKey",
       r."KeyVersion",
       r."State",
       r."LastSeenAt",
       r."Features"
FROM "RunnerSessions" s
JOIN "Runners" r ON r."Id" = s."RunnerId" AND NOT r."IsDeleted"
WHERE s."TokenHash" = @token_hash AND NOT s."IsDeleted";

-- name: SetRunnerFeatures :exec
UPDATE "Runners"
SET "Features" = @features::text[], "UpdatedAt" = @now
WHERE "Id" = @id AND "Features" IS DISTINCT FROM @features::text[] AND NOT "IsDeleted";

-- name: RotateRunnerKeys :one
UPDATE "Runners"
SET "SigningPublicKey" = @signing_public_key, "EncryptionPublicKey" = @encryption_public_key, "KeyVersion" = "KeyVersion" + 1,
    "KeysRotatedAt" = @now, "UpdatedAt" = @now
WHERE "Id" = @id AND "KeyVersion" = @key_version AND "State" <> 'revoked' AND NOT "IsDeleted"
RETURNING "KeyVersion";

-- name: EnsureRunnerThrottle :exec
INSERT INTO "RunnerThrottles" ("Key", "Failures", "LastFailureAt", "CreatedAt", "UpdatedAt")
VALUES (@key, 0, @now, @now, @now)
ON CONFLICT ("Key") DO UPDATE SET "UpdatedAt" = GREATEST("RunnerThrottles"."UpdatedAt", EXCLUDED."UpdatedAt");

-- name: LockRunnerThrottle :one
SELECT "Failures", "LastFailureAt", "LockedUntil"
FROM "RunnerThrottles"
WHERE "Key" = @key
FOR UPDATE;

-- name: UpdateRunnerThrottle :exec
UPDATE "RunnerThrottles"
SET "Failures" = @failures, "LastFailureAt" = @now, "LockedUntil" = sqlc.narg(locked_until)::timestamptz, "UpdatedAt" = @now
WHERE "Key" = @key;

-- name: RefundRunnerThrottle :exec
UPDATE "RunnerThrottles"
SET "Failures" = GREATEST("Failures" - 1, 0),
    "LockedUntil" = CASE WHEN "Failures" - 1 < @free::integer THEN NULL ELSE "LockedUntil" END,
    "UpdatedAt" = @now
WHERE "Key" = @key;
