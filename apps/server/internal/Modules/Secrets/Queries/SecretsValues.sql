-- name: ScanSecretValues :many
SELECT v."Id", v."SecretId", v."Version", s."Scope", s."OrganizationId", s."ProjectId", s."EnvironmentId", v."ValueSealed" AS "Sealed"
FROM "SecretVersions" v
JOIN "Secrets" s ON s."Id" = v."SecretId"
WHERE v."ValueSealed" IS NOT NULL AND v."Id" > @after::uuid
ORDER BY v."Id"
LIMIT @batch
FOR UPDATE OF v SKIP LOCKED;

-- name: ReplaceSecretValue :execrows
UPDATE "SecretVersions" SET "ValueSealed" = @sealed WHERE "Id" = @id AND "ValueSealed" = @old;

-- name: CountSecretValuesUnder :one
SELECT count(*)::bigint
FROM "SecretVersions"
WHERE "ValueSealed" IS NOT NULL AND get_byte("ValueSealed", 0) = 3 AND substring("ValueSealed" FROM 2 FOR 16) = ANY(@data_keys::bytea[]);

-- name: ScanRegistryPasswords :many
SELECT "Id", "OrganizationId", "ProjectId", "PasswordSealed" AS "Sealed"
FROM "RegistryCredentials"
WHERE "PasswordSealed" IS NOT NULL AND "Id" > @after::uuid
ORDER BY "Id"
LIMIT @batch
FOR UPDATE SKIP LOCKED;

-- name: ReplaceRegistryPassword :execrows
UPDATE "RegistryCredentials" SET "PasswordSealed" = @sealed WHERE "Id" = @id AND "PasswordSealed" = @old;

-- name: CountRegistryPasswordsUnder :one
SELECT count(*)::bigint
FROM "RegistryCredentials"
WHERE "PasswordSealed" IS NOT NULL AND get_byte("PasswordSealed", 0) = 3 AND substring("PasswordSealed" FROM 2 FOR 16) = ANY(@data_keys::bytea[]);
