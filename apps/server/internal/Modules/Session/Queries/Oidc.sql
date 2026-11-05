-- name: InsertOidcFlow :one
INSERT INTO "OidcFlows" ("Purpose", "Provider", "Issuer", "TokenHash", "StateHash", "DeviceHash", "UserId", "SessionId", "ExpiresAt",
                         "CreatedAt", "UpdatedAt")
VALUES (@purpose, @provider, @issuer, @token_hash, @state_hash, @device_hash, sqlc.narg(user_id), sqlc.narg(session_id), @expires_at,
        @now, @now)
RETURNING "Id";

-- name: TakeOidcFlow :one
UPDATE "OidcFlows"
SET "UsedAt" = @now::timestamptz, "UpdatedAt" = @now
WHERE "TokenHash" = @token_hash AND "Purpose" = @purpose AND "UsedAt" IS NULL AND "ExpiresAt" > @now AND NOT "IsDeleted"
RETURNING "Id", "Provider", "Issuer", "StateHash", "DeviceHash", "UserId", "SessionId";

-- name: FindUserByIdentity :one
SELECT i."Id" AS "IdentityId", u."Id", u."Email", u."Name", u."Locale", u."Disabled"
FROM "UserIdentities" i
JOIN "Users" u ON u."Id" = i."UserId" AND NOT u."IsDeleted"
WHERE i."Issuer" = @issuer AND i."Subject" = @subject AND NOT i."IsDeleted";

-- name: TouchIdentity :exec
UPDATE "UserIdentities"
SET "LastUsedAt" = @now::timestamptz, "Email" = @email, "Login" = COALESCE(sqlc.narg('login')::text, "Login"), "UpdatedAt" = @now
WHERE "Id" = @id AND NOT "IsDeleted";

-- name: ListUserIdentities :many
SELECT "Id", "Provider", "Email", "CreatedAt", "LastUsedAt"
FROM "UserIdentities"
WHERE "UserId" = @user_id AND NOT "IsDeleted"
ORDER BY "CreatedAt", "Id";

-- name: HasIdentityForProvider :one
SELECT EXISTS (
    SELECT 1 FROM "UserIdentities" WHERE "UserId" = @user_id AND "Provider" = @provider AND NOT "IsDeleted"
) AS "Linked";

-- name: InsertIdentity :one
INSERT INTO "UserIdentities" ("UserId", "Provider", "Issuer", "Subject", "Email", "Login", "LastUsedAt", "CreatedAt", "UpdatedAt")
VALUES (@user_id, @provider, @issuer, @subject, @email, sqlc.narg('login')::text, @now::timestamptz, @now, @now)
ON CONFLICT DO NOTHING
RETURNING "Id", "Provider", "Email", "CreatedAt", "LastUsedAt";

-- name: FindIdentityOwner :one
SELECT "UserId", "Provider"
FROM "UserIdentities"
WHERE NOT "IsDeleted" AND (("Issuer" = @issuer AND "Subject" = @subject) OR ("UserId" = @user_id AND "Provider" = @provider))
ORDER BY ("Issuer" = @issuer AND "Subject" = @subject) DESC
LIMIT 1;

-- name: LockUserRow :exec
SELECT 1 FROM "Users" WHERE "Id" = @id AND NOT "IsDeleted" FOR UPDATE;

-- name: LockUserSignInMethods :one
SELECT u."PasswordHash" <> '' AS "HasPassword",
       (SELECT count(*) FROM "UserIdentities" i
        WHERE i."UserId" = u."Id" AND NOT i."IsDeleted" AND (
            i."Provider" = ANY (@env::text[])
            OR (cardinality(@env::text[]) = 0 AND EXISTS (
                SELECT 1 FROM "OidcProviders" op
                WHERE op."ProviderId" = i."Provider" AND NOT op."IsDeleted" AND op."Secret" IS NOT NULL
            ))
        )) AS "Identities",
       (SELECT count(*) FROM "Passkeys" p WHERE p."UserId" = u."Id" AND NOT p."IsDeleted") AS "Passkeys"
FROM "Users" u
WHERE u."Id" = @id AND NOT u."IsDeleted"
FOR UPDATE OF u;

-- name: SignInProviderActive :one
SELECT COALESCE((@provider::text = ANY (@env::text[]))
    OR (cardinality(@env::text[]) = 0 AND EXISTS (
        SELECT 1 FROM "OidcProviders"
        WHERE "ProviderId" = @provider::text AND NOT "IsDeleted" AND "Secret" IS NOT NULL
    )), false)::boolean AS "Active";

-- name: LockOidcProvidersShared :exec
SELECT pg_advisory_xact_lock_shared(@lock_key::bigint);

-- name: CurrentProviderIssuer :one
SELECT "Issuer" FROM "OidcProviders" WHERE "ProviderId" = @provider AND NOT "IsDeleted" AND "Secret" IS NOT NULL;

-- name: RemoveUserIdentity :one
UPDATE "UserIdentities"
SET "IsDeleted" = true, "UpdatedAt" = @now
WHERE "Id" = @id AND "UserId" = @user_id AND NOT "IsDeleted"
RETURNING "Provider";
