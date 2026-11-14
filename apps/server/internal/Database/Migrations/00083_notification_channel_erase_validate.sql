-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
ALTER TABLE "NotificationChannels" VALIDATE CONSTRAINT "CK_NotificationChannels_Config";

-- +goose Down
SELECT 1;
