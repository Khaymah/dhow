-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" ADD CONSTRAINT "CK_Jobs_Attempts" CHECK ("Attempts" >= 0);

-- +goose Down
ALTER TABLE "Jobs" DROP CONSTRAINT "CK_Jobs_Attempts";
