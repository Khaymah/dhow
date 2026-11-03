-- name: ScanTotpSecrets :many
SELECT "Id", "UserId", "SecretSealed" AS "Sealed"
FROM "TotpFactors"
WHERE "SecretSealed" IS NOT NULL AND "Id" > @after::uuid AND (NOT @legacy_only::boolean OR get_byte("SecretSealed", 0) = 1)
ORDER BY "Id"
LIMIT @batch
FOR UPDATE SKIP LOCKED;

-- name: ReplaceTotpSecrets :execrows
UPDATE "TotpFactors" SET "SecretSealed" = @sealed WHERE "Id" = @id AND "SecretSealed" = @old;

-- name: CountLegacyTotpSecrets :one
SELECT count(*)::bigint FROM "TotpFactors" WHERE "SecretSealed" IS NOT NULL AND get_byte("SecretSealed", 0) = 1;

-- name: CountTotpSecretsUnder :one
SELECT count(*)::bigint FROM "TotpFactors"
WHERE "SecretSealed" IS NOT NULL AND get_byte("SecretSealed", 0) = 3 AND substring("SecretSealed" FROM 2 FOR 16) = ANY(@data_keys::bytea[]);
