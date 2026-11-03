-- dhow:phase expand
-- +goose Up
-- DROP TABLE "Jobs";
/* ALTER TABLE "Jobs" DROP COLUMN "Note"; */
INSERT INTO "AuditEvents" ("Action", "Detail") VALUES ('DROP TABLE "Jobs"; DELETE FROM "Jobs"', 'it''s; TRUNCATE "Jobs"');
INSERT INTO "AuditEvents" ("Action", "Detail") VALUES (E'escaped \' DROP TABLE "Jobs";', 'x');

-- +goose StatementBegin
CREATE FUNCTION "NoteGuard"() RETURNS trigger LANGUAGE plpgsql AS $body$
BEGIN
    DELETE FROM "Jobs" WHERE false;
    UPDATE "Jobs" SET "Note" = NULL WHERE false;
    EXECUTE 'DROP TABLE "Jobs"';
    RETURN NEW;
END
$body$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION "NoteGuard"();
ALTER TABLE "Jobs" DROP COLUMN "Note";
DROP TABLE "Jobs";
