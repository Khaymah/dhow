-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
ALTER TABLE "DataKeys" VALIDATE CONSTRAINT "CK_DataKeys_State";

-- +goose Down

