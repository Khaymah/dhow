-- +goose NO TRANSACTION
-- dhow:phase expand
-- +goose Up
ALTER TABLE "ApiTokens" VALIDATE CONSTRAINT "ApiTokens_Scopes_check";

-- +goose Down
