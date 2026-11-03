-- dhow:phase expand
-- +goose Up
ALTER TABLE "DataKeys" DROP CONSTRAINT "DataKeys_State_check";
ALTER TABLE "DataKeys" ADD CONSTRAINT "CK_DataKeys_State" CHECK ("State" IN ('active', 'superseded', 'destroyed')) NOT VALID;
ALTER TABLE "DataKeys" ADD COLUMN "SupersededAt" timestamptz;

CREATE TABLE "KeyRekeys" (
    "Id"         uuid        PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"  timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"  timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"  boolean     NOT NULL DEFAULT false,
    "Cutoff"     timestamptz NOT NULL DEFAULT now(),
    "Superseded" bigint      NOT NULL DEFAULT 0 CHECK ("Superseded" >= 0),
    "Resealed"   bigint      NOT NULL DEFAULT 0 CHECK ("Resealed" >= 0),
    "Destroyed"  bigint      NOT NULL DEFAULT 0 CHECK ("Destroyed" >= 0),
    "DoneAt"     timestamptz
);

CREATE UNIQUE INDEX "UX_KeyRekeys_Open" ON "KeyRekeys" ((true)) WHERE "DoneAt" IS NULL AND NOT "IsDeleted";

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM "DataKeys" WHERE "State" = 'superseded') THEN
        RAISE EXCEPTION 'some data keys are superseded: finish dhow-server keys rekey before rolling back 00054, or their values would stop opening';
    END IF;
END
$$;
-- +goose StatementEnd
DROP TABLE "KeyRekeys";
ALTER TABLE "DataKeys" DROP COLUMN "SupersededAt";
ALTER TABLE "DataKeys" DROP CONSTRAINT "CK_DataKeys_State";
ALTER TABLE "DataKeys" ADD CONSTRAINT "DataKeys_State_check" CHECK ("State" IN ('active', 'destroyed'));
