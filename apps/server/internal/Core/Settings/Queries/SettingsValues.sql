-- name: ScanInstanceSettingSecrets :many
SELECT "Id", "Group", "Secrets" AS "Sealed"
FROM "InstanceSettings"
WHERE "Secrets" IS NOT NULL AND "Id" > @after::uuid AND (NOT @legacy_only::boolean OR get_byte("Secrets", 0) = 1)
ORDER BY "Id"
LIMIT @batch
FOR UPDATE SKIP LOCKED;

-- name: ReplaceInstanceSettingSecrets :execrows
UPDATE "InstanceSettings" SET "Secrets" = @sealed WHERE "Id" = @id AND "Secrets" = @old;

-- name: CountLegacyInstanceSettingSecrets :one
SELECT count(*)::bigint FROM "InstanceSettings" WHERE "Secrets" IS NOT NULL AND get_byte("Secrets", 0) = 1;

-- name: CountInstanceSettingSecretsUnder :one
SELECT count(*)::bigint FROM "InstanceSettings"
WHERE "Secrets" IS NOT NULL AND get_byte("Secrets", 0) = 3 AND substring("Secrets" FROM 2 FOR 16) = ANY(@data_keys::bytea[]);

-- name: ScanOidcProviderSecrets :many
SELECT "Id", "ProviderId", "Secret" AS "Sealed"
FROM "OidcProviders"
WHERE "Secret" IS NOT NULL AND "Id" > @after::uuid AND (NOT @legacy_only::boolean OR get_byte("Secret", 0) = 1)
ORDER BY "Id"
LIMIT @batch
FOR UPDATE SKIP LOCKED;

-- name: ReplaceOidcProviderSecrets :execrows
UPDATE "OidcProviders" SET "Secret" = @sealed WHERE "Id" = @id AND "Secret" = @old;

-- name: CountLegacyOidcProviderSecrets :one
SELECT count(*)::bigint FROM "OidcProviders" WHERE "Secret" IS NOT NULL AND get_byte("Secret", 0) = 1;

-- name: CountOidcProviderSecretsUnder :one
SELECT count(*)::bigint FROM "OidcProviders"
WHERE "Secret" IS NOT NULL AND get_byte("Secret", 0) = 3 AND substring("Secret" FROM 2 FOR 16) = ANY(@data_keys::bytea[]);
