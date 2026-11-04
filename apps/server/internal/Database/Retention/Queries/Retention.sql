-- name: TryRetentionLock :one
SELECT pg_try_advisory_lock(@lock_key::bigint) AS "Locked";

-- name: ReleaseRetentionLock :one
SELECT pg_advisory_unlock(@lock_key::bigint) AS "Released";

-- name: RetentionClock :one
SELECT now()::timestamptz AS "Now";

-- name: PurgeLoginThrottles :execrows
DELETE FROM "LoginThrottles"
WHERE "Id" IN (
    SELECT t."Id" FROM "LoginThrottles" t
    WHERE t."LastFailureAt" < @before AND t."UpdatedAt" < @before AND (t."LockedUntil" IS NULL OR t."LockedUntil" < @before)
    LIMIT @batch
    FOR UPDATE SKIP LOCKED
);

-- name: PurgeRunnerThrottles :execrows
DELETE FROM "RunnerThrottles"
WHERE "Id" IN (
    SELECT t."Id" FROM "RunnerThrottles" t
    WHERE t."LastFailureAt" < @before AND t."UpdatedAt" < @before AND (t."LockedUntil" IS NULL OR t."LockedUntil" < @before)
    LIMIT @batch
    FOR UPDATE SKIP LOCKED
);

-- name: PurgeAuthChallenges :execrows
DELETE FROM "AuthChallenges"
WHERE "Id" IN (
    SELECT c."Id" FROM "AuthChallenges" c
    WHERE c."ExpiresAt" < @before
    LIMIT @batch
    FOR UPDATE SKIP LOCKED
);

-- name: PurgeOidcFlows :execrows
DELETE FROM "OidcFlows"
WHERE "Id" IN (
    SELECT f."Id" FROM "OidcFlows" f
    WHERE f."ExpiresAt" < @before
    LIMIT @batch
    FOR UPDATE SKIP LOCKED
);

-- name: PurgeInvitations :execrows
DELETE FROM "Invitations"
WHERE "Id" IN (
    SELECT i."Id" FROM "Invitations" i
    WHERE i."AcceptedAt" IS NULL AND LEAST(i."ExpiresAt", i."RevokedAt") < @before
    LIMIT @batch
    FOR UPDATE SKIP LOCKED
);

-- name: PurgeSetupTokens :execrows
DELETE FROM "SetupTokens"
WHERE "Id" IN (
    SELECT s."Id" FROM "SetupTokens" s
    WHERE s."ExpiresAt" < @before
    LIMIT @batch
    FOR UPDATE SKIP LOCKED
);

-- name: PurgeRateLimits :execrows
DELETE FROM "RateLimits"
WHERE "Id" IN (
    SELECT r."Id" FROM "RateLimits" r
    WHERE r."ExpiresAt" < @before
    LIMIT @batch
    FOR UPDATE SKIP LOCKED
);

-- name: EraseExpiredSecretVersions :execrows
UPDATE "SecretVersions"
SET "ValueSealed" = NULL, "ErasedAt" = now(), "UpdatedAt" = now()
WHERE "Id" IN (
    SELECT v."Id" FROM "SecretVersions" v
    WHERE v."ValueSealed" IS NOT NULL AND v."SupersededAt" IS NOT NULL AND v."SupersededAt" < @before::timestamptz
    ORDER BY v."SupersededAt"
    LIMIT @batch
    FOR UPDATE SKIP LOCKED
);
