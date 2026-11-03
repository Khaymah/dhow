-- dhow:phase expand
-- +goose Up
-- COMMIT;
/* ROLLBACK; */
ALTER TABLE "Jobs" ADD COLUMN "Commit" TEXT DEFAULT 'BEGIN; COMMIT; END;';

-- +goose StatementBegin
CREATE FUNCTION "NotesGuard"() RETURNS trigger LANGUAGE plpgsql AS $body$
BEGIN
    IF NEW."Commit" IS NULL THEN
        RAISE EXCEPTION 'end is required';
    END IF;
    RETURN NEW;
END
$body$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION "NotesCount"() RETURNS bigint LANGUAGE sql
BEGIN ATOMIC
    SELECT count(*) FROM "Jobs";
END;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION "NotesCount"();
DROP FUNCTION "NotesGuard"();
ALTER TABLE "Jobs" DROP COLUMN "Commit";
