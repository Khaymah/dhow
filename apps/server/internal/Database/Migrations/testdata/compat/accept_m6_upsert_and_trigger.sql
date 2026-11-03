-- dhow:phase expand
-- +goose Up
INSERT INTO "InstanceSettings" ("Group", "Value") VALUES ('updates', '{}')
ON CONFLICT ("Group") DO UPDATE SET "Value" = EXCLUDED."Value";
CREATE TRIGGER "TR_Jobs_Note" BEFORE INSERT OR UPDATE ON "Jobs"
    FOR EACH ROW EXECUTE FUNCTION "NoteGuard"();

-- +goose Down
DROP TRIGGER "TR_Jobs_Note" ON "Jobs";
