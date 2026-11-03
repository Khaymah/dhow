-- name: ListOidcProviders :many
SELECT "Id", "ProviderId", "Type", "Name", "Issuer", "ClientId", "AllowedDomains", "Secret", "Version", "CreatedAt",
       "UpdatedAt", "UpdatedBy"
FROM "OidcProviders"
WHERE NOT "IsDeleted"
ORDER BY "CreatedAt", "Id";

-- name: CountOidcProviders :one
SELECT count(*) FROM "OidcProviders" WHERE NOT "IsDeleted";

-- name: LockOidcProviderList :exec
SELECT pg_advisory_xact_lock(@lock_key::bigint);

-- name: InsertOidcProvider :one
INSERT INTO "OidcProviders" ("ProviderId", "Type", "Name", "Issuer", "ClientId", "AllowedDomains", "Secret", "CreatedBy", "UpdatedBy")
VALUES (@provider_id, @type, @name, @issuer, @client_id, @allowed_domains::text[], @secret, @actor, @actor)
ON CONFLICT ("ProviderId") WHERE NOT "IsDeleted" DO NOTHING
RETURNING "Version";

-- name: UpdateOidcProvider :one
UPDATE "OidcProviders"
SET "Type" = @type, "Name" = @name, "Issuer" = @issuer, "ClientId" = @client_id, "AllowedDomains" = @allowed_domains::text[],
    "Secret" = @secret, "Version" = "Version" + 1, "UpdatedBy" = @actor, "UpdatedAt" = now()
WHERE "Id" = @id AND "Version" = @version AND NOT "IsDeleted"
RETURNING "Version";

-- name: RemoveOidcProvider :one
UPDATE "OidcProviders"
SET "IsDeleted" = true, "Version" = "Version" + 1, "UpdatedBy" = @actor, "UpdatedAt" = now()
WHERE "Id" = @id AND "Version" = @version AND NOT "IsDeleted"
RETURNING "Version";

-- name: CountIdentitiesByProvider :many
SELECT "Provider", count(*) AS "Identities"
FROM "UserIdentities"
WHERE NOT "IsDeleted"
GROUP BY "Provider";

-- name: ProviderIdentityIssuers :one
SELECT EXISTS (SELECT 1 FROM "UserIdentities" a WHERE a."Provider" = @provider::text AND NOT a."IsDeleted") AS "Linked",
       EXISTS (
           SELECT 1 FROM "UserIdentities" b WHERE b."Provider" = @provider::text AND b."Issuer" <> @issuer::text AND NOT b."IsDeleted"
       ) AS "OtherIssuer";

-- name: LockUsersOfProvider :many
SELECT u."Id"
FROM "Users" u
WHERE NOT u."IsDeleted"
  AND EXISTS (SELECT 1 FROM "UserIdentities" i WHERE i."UserId" = u."Id" AND i."Provider" = @provider::text AND NOT i."IsDeleted")
ORDER BY u."Id"
FOR UPDATE OF u;

-- name: CountUsersOnlyOnProvider :one
SELECT count(*)
FROM "Users" u
WHERE NOT u."IsDeleted"
  AND u."PasswordHash" = ''
  AND EXISTS (SELECT 1 FROM "UserIdentities" i WHERE i."UserId" = u."Id" AND i."Provider" = @provider::text AND NOT i."IsDeleted")
  AND NOT EXISTS (SELECT 1 FROM "Passkeys" p WHERE p."UserId" = u."Id" AND NOT p."IsDeleted")
  AND NOT EXISTS (
      SELECT 1 FROM "UserIdentities" o
      WHERE o."UserId" = u."Id" AND NOT o."IsDeleted" AND o."Provider" <> @provider::text AND o."Provider" = ANY (@others::text[])
  );

-- name: CountUsersOutsideDomains :one
SELECT count(*)
FROM "Users" u
WHERE NOT u."IsDeleted"
  AND u."PasswordHash" = ''
  AND EXISTS (
      SELECT 1 FROM "UserIdentities" i
      WHERE i."UserId" = u."Id" AND i."Provider" = @provider::text AND NOT i."IsDeleted"
        AND NOT (lower(COALESCE(substring(i."Email" FROM '@([^@]+)$'), '')) = ANY (@domains::text[]))
  )
  AND NOT EXISTS (SELECT 1 FROM "Passkeys" p WHERE p."UserId" = u."Id" AND NOT p."IsDeleted")
  AND NOT EXISTS (
      SELECT 1 FROM "UserIdentities" o
      WHERE o."UserId" = u."Id" AND NOT o."IsDeleted" AND o."Provider" <> @provider::text AND o."Provider" = ANY (@others::text[])
  );

-- name: CountStrandedIdentities :one
SELECT count(DISTINCT p."ProviderId")::bigint AS "Providers", count(i."Id")::bigint AS "Identities"
FROM "OidcProviders" p
JOIN "UserIdentities" i ON i."Provider" = p."ProviderId" AND NOT i."IsDeleted"
JOIN "Users" u ON u."Id" = i."UserId" AND NOT u."IsDeleted"
WHERE NOT p."IsDeleted" AND NOT (p."ProviderId" = ANY (@active::text[]));

-- name: ListOidcProvidersForUpdate :many
SELECT "Id", "ProviderId", "Type", "Name", "Issuer", "ClientId", "AllowedDomains", "Secret", "Version", "CreatedAt",
       "UpdatedAt", "UpdatedBy"
FROM "OidcProviders"
WHERE NOT "IsDeleted"
ORDER BY "CreatedAt", "Id"
FOR UPDATE;
