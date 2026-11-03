-- dhow:phase expand
-- +goose Up
ALTER TABLE "Jobs" ADD CONSTRAINT "CK_Jobs_Attempts" CHECK ("Attempts" >= 0) NOT VALID;
ALTER TABLE "Jobs" ADD CONSTRAINT "FK_Jobs_Project" FOREIGN KEY ("ProjectId") REFERENCES "Projects" ("Id") NOT VALID;

-- +goose Down
ALTER TABLE "Jobs" DROP CONSTRAINT "FK_Jobs_Project";
ALTER TABLE "Jobs" DROP CONSTRAINT "CK_Jobs_Attempts";
