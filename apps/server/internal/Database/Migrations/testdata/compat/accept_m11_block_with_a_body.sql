-- dhow:phase expand
-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION "NotesNow"() RETURNS timestamptz LANGUAGE plpgsql AS $$
BEGIN
    RETURN now();
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION "NotesNow"();
