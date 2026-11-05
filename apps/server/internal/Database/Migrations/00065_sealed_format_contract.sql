-- dhow:phase contract
-- dhow:floor 0.0.0
-- +goose Up
-- +goose StatementBegin
DO $$
DECLARE
    c record;
    old_format boolean;
BEGIN
    FOR c IN SELECT * FROM (VALUES
        ('keys.envelope.TotpFactors.SecretSealed', 'TotpFactors', 'SecretSealed'),
        ('keys.envelope.InstanceSettings.Secrets', 'InstanceSettings', 'Secrets'),
        ('keys.envelope.OidcProviders.Secret', 'OidcProviders', 'Secret'),
        ('keys.envelope.Repositories.DeployKeySealed', 'Repositories', 'DeployKeySealed'),
        ('keys.envelope.Repositories.PreviousDeployKeySealed', 'Repositories', 'PreviousDeployKeySealed'),
        ('keys.envelope.Repositories.WebhookSecretSealed', 'Repositories', 'WebhookSecretSealed'),
        ('keys.envelope.SecretVersions.ValueSealed', 'SecretVersions', 'ValueSealed'),
        ('keys.envelope.RegistryCredentials.PasswordSealed', 'RegistryCredentials', 'PasswordSealed')
    ) AS t(backfill, tbl, col)
    LOOP
        EXECUTE format('SELECT EXISTS (SELECT 1 FROM %I WHERE %I IS NOT NULL AND (octet_length(%I) < 1 OR get_byte(%I, 0) <> 3))',
                       c.tbl, c.col, c.col, c.col) INTO old_format;
        IF old_format THEN
            PERFORM "RequireBackfillDone"(c.backfill);
        END IF;
    END LOOP;
END
$$;
-- +goose StatementEnd

ALTER TABLE "TotpFactors" ADD CONSTRAINT "CK_TotpFactors_SecretSealed_Envelope"
    CHECK ("SecretSealed" IS NULL OR (octet_length("SecretSealed") >= 1 AND get_byte("SecretSealed", 0) = 3)) NOT VALID;
ALTER TABLE "InstanceSettings" ADD CONSTRAINT "CK_InstanceSettings_Secrets_Envelope"
    CHECK ("Secrets" IS NULL OR (octet_length("Secrets") >= 1 AND get_byte("Secrets", 0) = 3)) NOT VALID;
ALTER TABLE "OidcProviders" ADD CONSTRAINT "CK_OidcProviders_Secret_Envelope"
    CHECK ("Secret" IS NULL OR (octet_length("Secret") >= 1 AND get_byte("Secret", 0) = 3)) NOT VALID;
ALTER TABLE "Repositories" ADD CONSTRAINT "CK_Repositories_DeployKeySealed_Envelope"
    CHECK ("DeployKeySealed" IS NULL OR (octet_length("DeployKeySealed") >= 1 AND get_byte("DeployKeySealed", 0) = 3)) NOT VALID;
ALTER TABLE "Repositories" ADD CONSTRAINT "CK_Repositories_PreviousDeployKeySealed_Envelope"
    CHECK ("PreviousDeployKeySealed" IS NULL OR (octet_length("PreviousDeployKeySealed") >= 1 AND get_byte("PreviousDeployKeySealed", 0) = 3)) NOT VALID;
ALTER TABLE "Repositories" ADD CONSTRAINT "CK_Repositories_WebhookSecretSealed_Envelope"
    CHECK ("WebhookSecretSealed" IS NULL OR (octet_length("WebhookSecretSealed") >= 1 AND get_byte("WebhookSecretSealed", 0) = 3)) NOT VALID;
ALTER TABLE "SecretVersions" ADD CONSTRAINT "CK_SecretVersions_ValueSealed_Envelope"
    CHECK ("ValueSealed" IS NULL OR (octet_length("ValueSealed") >= 1 AND get_byte("ValueSealed", 0) = 3)) NOT VALID;
ALTER TABLE "RegistryCredentials" ADD CONSTRAINT "CK_RegistryCredentials_PasswordSealed_Envelope"
    CHECK ("PasswordSealed" IS NULL OR (octet_length("PasswordSealed") >= 1 AND get_byte("PasswordSealed", 0) = 3)) NOT VALID;

-- +goose Down
ALTER TABLE "RegistryCredentials" DROP CONSTRAINT "CK_RegistryCredentials_PasswordSealed_Envelope";
ALTER TABLE "SecretVersions" DROP CONSTRAINT "CK_SecretVersions_ValueSealed_Envelope";
ALTER TABLE "Repositories" DROP CONSTRAINT "CK_Repositories_WebhookSecretSealed_Envelope";
ALTER TABLE "Repositories" DROP CONSTRAINT "CK_Repositories_PreviousDeployKeySealed_Envelope";
ALTER TABLE "Repositories" DROP CONSTRAINT "CK_Repositories_DeployKeySealed_Envelope";
ALTER TABLE "OidcProviders" DROP CONSTRAINT "CK_OidcProviders_Secret_Envelope";
ALTER TABLE "InstanceSettings" DROP CONSTRAINT "CK_InstanceSettings_Secrets_Envelope";
ALTER TABLE "TotpFactors" DROP CONSTRAINT "CK_TotpFactors_SecretSealed_Envelope";
