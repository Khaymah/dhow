-- dhow:phase contract
-- dhow:floor 0.0.0
-- +goose Up
UPDATE "Repositories"
SET "DeployKeySealed" = NULL, "PreviousDeployKeySealed" = NULL, "WebhookSecretSealed" = NULL,
    "PendingHostKey" = NULL, "PendingHostKeyFetchedAt" = NULL, "UpdatedAt" = now()
WHERE "IsDeleted";

ALTER TABLE "Repositories" DROP CONSTRAINT "Repositories_check2";
ALTER TABLE "Repositories" DROP CONSTRAINT "Repositories_check3";
ALTER TABLE "Repositories" DROP CONSTRAINT "Repositories_check4";

ALTER TABLE "Repositories" ADD CONSTRAINT "CK_Repositories_HookSecret"
    CHECK ("IsDeleted" OR (("HookId" IS NULL) = ("WebhookSecretSealed" IS NULL)));
ALTER TABLE "Repositories" ADD CONSTRAINT "CK_Repositories_DeployKey"
    CHECK ("IsDeleted" OR (("DeployKeyPublic" IS NULL) = ("DeployKeySealed" IS NULL)));
ALTER TABLE "Repositories" ADD CONSTRAINT "CK_Repositories_PreviousDeployKey"
    CHECK ("IsDeleted" OR (("PreviousDeployKeyPublic" IS NULL) = ("PreviousDeployKeySealed" IS NULL)));
ALTER TABLE "Repositories" ADD CONSTRAINT "CK_Repositories_UnlinkedHasNoSecrets"
    CHECK (NOT "IsDeleted" OR ("DeployKeySealed" IS NULL AND "PreviousDeployKeySealed" IS NULL
                                AND "WebhookSecretSealed" IS NULL AND "PendingHostKey" IS NULL));

-- +goose Down
ALTER TABLE "Repositories" DROP CONSTRAINT "CK_Repositories_UnlinkedHasNoSecrets";
ALTER TABLE "Repositories" DROP CONSTRAINT "CK_Repositories_PreviousDeployKey";
ALTER TABLE "Repositories" DROP CONSTRAINT "CK_Repositories_DeployKey";
ALTER TABLE "Repositories" DROP CONSTRAINT "CK_Repositories_HookSecret";
ALTER TABLE "Repositories" ADD CONSTRAINT "Repositories_check2" CHECK (("HookId" IS NULL) = ("WebhookSecretSealed" IS NULL)) NOT VALID;
ALTER TABLE "Repositories" ADD CONSTRAINT "Repositories_check3" CHECK (("DeployKeyPublic" IS NULL) = ("DeployKeySealed" IS NULL)) NOT VALID;
ALTER TABLE "Repositories" ADD CONSTRAINT "Repositories_check4" CHECK (("PreviousDeployKeyPublic" IS NULL) = ("PreviousDeployKeySealed" IS NULL)) NOT VALID;
