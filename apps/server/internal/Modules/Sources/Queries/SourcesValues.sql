-- name: ScanRepositoryDeployKeys :many
SELECT "Id", "ProjectId", "DeployKeySealed" AS "Sealed"
FROM "Repositories"
WHERE "DeployKeySealed" IS NOT NULL AND "Id" > @after::uuid
ORDER BY "Id"
LIMIT @batch
FOR UPDATE SKIP LOCKED;

-- name: ReplaceRepositoryDeployKeys :execrows
UPDATE "Repositories" SET "DeployKeySealed" = @sealed WHERE "Id" = @id AND "DeployKeySealed" = @old;

-- name: CountRepositoryDeployKeysUnder :one
SELECT count(*)::bigint FROM "Repositories"
WHERE "DeployKeySealed" IS NOT NULL AND get_byte("DeployKeySealed", 0) = 3 AND substring("DeployKeySealed" FROM 2 FOR 16) = ANY(@data_keys::bytea[]);

-- name: ScanRepositoryPreviousDeployKeys :many
SELECT "Id", "ProjectId", "PreviousDeployKeySealed" AS "Sealed"
FROM "Repositories"
WHERE "PreviousDeployKeySealed" IS NOT NULL AND "Id" > @after::uuid
ORDER BY "Id"
LIMIT @batch
FOR UPDATE SKIP LOCKED;

-- name: ReplaceRepositoryPreviousDeployKeys :execrows
UPDATE "Repositories" SET "PreviousDeployKeySealed" = @sealed WHERE "Id" = @id AND "PreviousDeployKeySealed" = @old;

-- name: CountRepositoryPreviousDeployKeysUnder :one
SELECT count(*)::bigint FROM "Repositories"
WHERE "PreviousDeployKeySealed" IS NOT NULL AND get_byte("PreviousDeployKeySealed", 0) = 3 AND substring("PreviousDeployKeySealed" FROM 2 FOR 16) = ANY(@data_keys::bytea[]);

-- name: ScanRepositoryWebhookSecrets :many
SELECT "Id", "ProjectId", "WebhookSecretSealed" AS "Sealed"
FROM "Repositories"
WHERE "WebhookSecretSealed" IS NOT NULL AND "Id" > @after::uuid
ORDER BY "Id"
LIMIT @batch
FOR UPDATE SKIP LOCKED;

-- name: ReplaceRepositoryWebhookSecrets :execrows
UPDATE "Repositories" SET "WebhookSecretSealed" = @sealed WHERE "Id" = @id AND "WebhookSecretSealed" = @old;

-- name: CountRepositoryWebhookSecretsUnder :one
SELECT count(*)::bigint FROM "Repositories"
WHERE "WebhookSecretSealed" IS NOT NULL AND get_byte("WebhookSecretSealed", 0) = 3 AND substring("WebhookSecretSealed" FROM 2 FOR 16) = ANY(@data_keys::bytea[]);
