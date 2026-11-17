-- dhow:phase expand
-- dhow:backfill small
-- +goose Up
-- +goose StatementBegin
DO $$
DECLARE
    rec record;
BEGIN
    FOR rec IN
        SELECT conname AS name FROM pg_constraint
        WHERE conrelid = '"NotificationChannels"'::regclass AND contype = 'c'
          AND pg_get_constraintdef(oid) LIKE '%"ConfigSealed" IS NOT NULL%'
    LOOP
        EXECUTE format('ALTER TABLE "NotificationChannels" DROP CONSTRAINT %I', rec.name);
    END LOOP;
END
$$;
-- +goose StatementEnd

ALTER TABLE "NotificationChannels" ADD CONSTRAINT "CK_NotificationChannels_Config"
    CHECK ("Kind" IN ('email', 'inapp') OR "IsDeleted" OR "ConfigSealed" IS NOT NULL) NOT VALID;

UPDATE "NotificationChannels" SET "ConfigSealed" = NULL, "UpdatedAt" = now() WHERE "IsDeleted" AND "ConfigSealed" IS NOT NULL;

-- +goose Down
ALTER TABLE "NotificationChannels" DROP CONSTRAINT IF EXISTS "CK_NotificationChannels_Config";
