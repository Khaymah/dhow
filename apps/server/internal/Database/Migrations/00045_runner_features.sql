-- +goose Up
ALTER TABLE "Runners" ADD COLUMN "Features" TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE "Runners" ADD CONSTRAINT "CK_Runners_Features" CHECK (
    cardinality("Features") <= 8
    AND "Features" <@ ARRAY['checkout', 'plugins', 'artifacts', 'cache']::text[]
);

-- +goose Down
ALTER TABLE "Runners" DROP CONSTRAINT "CK_Runners_Features";
ALTER TABLE "Runners" DROP COLUMN "Features";
