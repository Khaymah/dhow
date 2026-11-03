-- dhow:phase expand
-- +goose Up
CREATE TABLE "Backfills" (
    "Id"        uuid        PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt" timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt" timestamptz NOT NULL DEFAULT now(),
    "IsDeleted" boolean     NOT NULL DEFAULT false,
    "Name"      TEXT        NOT NULL CHECK ("Name" ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$'),
    "LastKey"   TEXT        NOT NULL DEFAULT '' CHECK (octet_length("LastKey") <= 512),
    "Processed" bigint      NOT NULL DEFAULT 0 CHECK ("Processed" >= 0),
    "Failed"    bigint      NOT NULL DEFAULT 0 CHECK ("Failed" >= 0),
    "Remaining" bigint      CHECK ("Remaining" >= 0),
    "Passes"    integer     NOT NULL DEFAULT 0 CHECK ("Passes" >= 0),
    "DoneAt"    timestamptz
);

CREATE UNIQUE INDEX "UX_Backfills_Name" ON "Backfills" ("Name");

-- +goose StatementBegin
CREATE FUNCTION "RequireBackfillDone"("BackfillName" TEXT) RETURNS void LANGUAGE plpgsql STABLE AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM "Backfills" AS b WHERE b."Name" = "BackfillName" AND b."DoneAt" IS NOT NULL AND NOT b."IsDeleted") THEN
        RAISE EXCEPTION 'BACKFILL_NOT_DONE: the background backfill % has not finished; run the release that starts it and wait until dhow-server migrate status shows it done', "BackfillName";
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION "RequireBackfillDone"(TEXT);
DROP TABLE "Backfills";
