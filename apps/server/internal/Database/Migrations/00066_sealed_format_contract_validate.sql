-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
ALTER TABLE "TotpFactors" VALIDATE CONSTRAINT "CK_TotpFactors_SecretSealed_Envelope";
ALTER TABLE "InstanceSettings" VALIDATE CONSTRAINT "CK_InstanceSettings_Secrets_Envelope";
ALTER TABLE "OidcProviders" VALIDATE CONSTRAINT "CK_OidcProviders_Secret_Envelope";
ALTER TABLE "Repositories" VALIDATE CONSTRAINT "CK_Repositories_DeployKeySealed_Envelope";
ALTER TABLE "Repositories" VALIDATE CONSTRAINT "CK_Repositories_PreviousDeployKeySealed_Envelope";
ALTER TABLE "Repositories" VALIDATE CONSTRAINT "CK_Repositories_WebhookSecretSealed_Envelope";
ALTER TABLE "SecretVersions" VALIDATE CONSTRAINT "CK_SecretVersions_ValueSealed_Envelope";
ALTER TABLE "RegistryCredentials" VALIDATE CONSTRAINT "CK_RegistryCredentials_PasswordSealed_Envelope";

-- +goose Down
