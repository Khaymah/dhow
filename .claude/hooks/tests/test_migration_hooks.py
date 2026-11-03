import os
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

HOOKS = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HOOKS)

from dhow_hooks import edit_rules, migration_rules

REPO = os.path.dirname(os.path.dirname(HOOKS))
MIGRATIONS = os.path.join(REPO, "apps", "server", "internal", "Database", "Migrations")
CASES = os.path.join(MIGRATIONS, "testdata", "compat")
NEW = "apps/server/internal/Database/Migrations/00049_x.sql"


def read(path):
    with open(path, encoding="utf-8") as fh:
        return fh.read()


def case_tags():
    return set(read(os.path.join(CASES, "tags.txt")).split())


def case_files():
    return sorted(f for f in os.listdir(CASES) if f.endswith(".sql"))


def write(rel, content, tags=frozenset()):
    with mock.patch.object(migration_rules, "repo_tags", return_value=set(tags)):
        return edit_rules.evaluate(rel=rel, root="/nonexistent", segments=[(content, "")], exists=False,
                                   final_content=content)


def expand(body, header=""):
    return f"-- dhow:phase expand\n{header}-- +goose Up\n{body}\n\n-- +goose Down\nSELECT 1;\n"


def contract(body, floor="0.0.0"):
    return f"-- dhow:phase contract\n-- dhow:floor {floor}\n-- +goose Up\n{body}\n\n-- +goose Down\nSELECT 1;\n"


def no_tx(body):
    return f"-- +goose NO TRANSACTION\n-- dhow:phase expand\n-- +goose Up\n{body}\n\n-- +goose Down\nSELECT 1;\n"


class SharedCases(unittest.TestCase):
    def test_every_case_gets_the_verdict_in_its_name(self):
        tags = case_tags()
        for name in case_files():
            kind, rule = name.split("_")[:2]
            found = migration_rules.verdict(read(os.path.join(CASES, name)), tags)
            want = None if kind == "accept" else rule.upper()
            self.assertEqual(found[0] if found else None, want, name)

    def test_every_rule_has_an_accept_and_a_reject_case(self):
        names = case_files()
        for n in range(1, 10):
            for kind in ("accept", "reject"):
                self.assertTrue(any(f.startswith(f"{kind}_m{n}_") for f in names), f"{kind} M{n}")

    def test_hook_rejects_every_reject_case_with_its_rule(self):
        for name in case_files():
            reason = write(NEW, read(os.path.join(CASES, name)), case_tags())
            if name.startswith("accept_"):
                self.assertIsNone(reason, name)
            else:
                self.assertIn(f"[{name.split('_')[1].upper()}]", reason, name)
                self.assertIn("0077", reason, name)


class Rules(unittest.TestCase):
    def assertRule(self, rule, content, tags=frozenset()):
        reason = write(NEW, content, tags)
        self.assertIsNotNone(reason, content)
        self.assertIn(f"[{rule}]", reason)

    def test_m1_missing_phase_rejected_and_expand_add_column_allowed(self):
        self.assertRule("M1", '-- +goose Up\nALTER TABLE "Jobs" ADD COLUMN "X" TEXT;\n')
        self.assertIn("dhow:phase expand", write(NEW, '-- +goose Up\nALTER TABLE "Jobs" ADD COLUMN "X" TEXT;\n'))
        self.assertIsNone(write(NEW, expand('ALTER TABLE "Jobs" ADD COLUMN "X" TEXT;')))

    def test_m1_phase_after_no_transaction(self):
        self.assertIsNone(write(NEW, no_tx('ALTER TABLE "Jobs" VALIDATE CONSTRAINT "CK_X";')))
        self.assertRule("M1", "-- dhow:phase later\n-- +goose Up\nSELECT 1;\n")

    def test_m2_destructive_in_expand(self):
        for stmt in ('DROP TABLE "Jobs";', 'ALTER TABLE "Jobs" DROP COLUMN "X";', 'ALTER TABLE "Jobs" RENAME TO "Work";',
                     'ALTER TABLE "Jobs" ALTER "X" SET DATA TYPE bigint;', 'ALTER TABLE "Jobs" ALTER COLUMN "X" SET NOT NULL;',
                     'DROP FUNCTION "F"();', 'DROP TYPE "T";', 'TRUNCATE "Jobs";', 'DELETE FROM "Jobs";'):
            self.assertRule("M2", expand(stmt))
            self.assertIsNone(write(NEW, contract(stmt)), stmt)

    def test_m2_allows_new_table_and_ignores_strings_comments_bodies_and_down(self):
        self.assertIsNone(write(NEW, expand('CREATE TABLE "T" ("Id" uuid PRIMARY KEY DEFAULT uuidv7(), '
                                            '"CreatedAt" timestamptz NOT NULL DEFAULT now(), "UpdatedAt" timestamptz '
                                            'NOT NULL DEFAULT now(), "IsDeleted" boolean NOT NULL DEFAULT false, "X" TEXT);\n'
                                            'ALTER TABLE "T" DROP COLUMN "X";')))
        self.assertIsNone(write(NEW, expand("SELECT 'DROP TABLE \"Jobs\";';\n-- DROP TABLE \"Jobs\";\n"
                                            "DO $$ BEGIN DROP TABLE \"Jobs\"; END $$;")))
        self.assertIsNone(write(NEW, "-- dhow:phase expand\n-- +goose Up\nSELECT 1;\n-- +goose Down\nDROP TABLE \"Jobs\";\n"))

    def test_m2_case_quotes_and_lines(self):
        self.assertRule("M2", expand("alter TABLE\n  public.\"Jobs\"\n  drop\n  column x;"))

    def test_m3_not_null_without_default(self):
        self.assertRule("M3", expand('ALTER TABLE "Jobs" ADD COLUMN "X" integer NOT NULL;'))
        self.assertIsNone(write(NEW, expand('ALTER TABLE "Jobs" ADD COLUMN "X" integer NOT NULL DEFAULT 0;')))

    def test_m4_constraint_without_not_valid(self):
        self.assertRule("M4", expand('ALTER TABLE "Jobs" ADD CONSTRAINT "CK" CHECK ("X" > 0);'))
        self.assertRule("M4", contract('ALTER TABLE "Jobs" ADD FOREIGN KEY ("P") REFERENCES "Projects" ("Id");'))
        self.assertIsNone(write(NEW, expand('ALTER TABLE "Jobs" ADD CONSTRAINT "CK" CHECK ("X" > 0) NOT VALID;')))

    def test_m5_index_on_existing_table(self):
        self.assertRule("M5", expand('CREATE INDEX "IX" ON "Jobs" ("X");'))
        self.assertIn("NO TRANSACTION", write(NEW, expand('CREATE INDEX CONCURRENTLY IF NOT EXISTS "IX" ON "Jobs" ("X");')))
        self.assertIsNone(write(NEW, no_tx('CREATE INDEX CONCURRENTLY IF NOT EXISTS "IX" ON "Jobs" ("X");')))

    def test_m6_update_needs_small_backfill(self):
        self.assertRule("M6", expand('UPDATE "Jobs" SET "X" = 1;'))
        self.assertIsNone(write(NEW, expand('UPDATE "Jobs" SET "X" = 1;', "-- dhow:backfill small\n")))
        self.assertIsNone(write(NEW, expand('INSERT INTO "Jobs" ("X") VALUES (1) ON CONFLICT ("X") DO UPDATE SET "X" = 2;')))

    def test_m7_floor_and_schema_floor_update(self):
        self.assertRule("M7", "-- dhow:phase contract\n-- +goose Up\nSELECT 1;\n")
        self.assertRule("M7", contract("SELECT 1;", "1.0.0"), {"v1.0.0"})
        self.assertRule("M7", contract("UPDATE \"Instance\" SET \"SchemaFloor\" = '0.9.0';", "1.0.0"), {"v1.0.0"})
        self.assertIsNone(write(NEW, contract("UPDATE \"Instance\" SET \"SchemaFloor\" = '1.0.0';", "1.0.0"), {"v1.0.0"}))
        self.assertIsNone(write(NEW, contract("SELECT 1;")))

    def test_m8_floor_must_be_a_tag(self):
        self.assertRule("M8", contract("UPDATE \"Instance\" SET \"SchemaFloor\" = '2.0.0';", "2.0.0"), {"v1.0.0"})

    def test_m9_no_transaction_statements_are_idempotent(self):
        self.assertRule("M9", no_tx('ALTER TABLE "Jobs" ADD COLUMN "X" TEXT;'))
        self.assertIsNone(write(NEW, no_tx('ALTER TABLE "Jobs" ADD COLUMN IF NOT EXISTS "X" TEXT;')))

    def test_statement_begin_end_is_one_statement(self):
        body = ("-- +goose StatementBegin\nCREATE FUNCTION \"F\"() RETURNS trigger LANGUAGE plpgsql AS $$\n"
                "BEGIN\n  DELETE FROM \"Jobs\";\n  RETURN NEW;\nEND\n$$;\n-- +goose StatementEnd")
        _, up = migration_rules.sections(expand(body))
        self.assertEqual(len(migration_rules.statements(up)), 1)
        self.assertIsNone(write(NEW, expand(body)))


class Scope(unittest.TestCase):
    def test_edit_checks_the_final_file(self):
        old = expand('ALTER TABLE "Jobs" ADD COLUMN "X" TEXT;')
        final = old.replace("ADD COLUMN", "DROP COLUMN")
        reason = edit_rules.evaluate(rel=NEW, root="/nonexistent", segments=[("DROP COLUMN", "ADD COLUMN")],
                                     exists=True, final_content=final)
        self.assertIn("[M2]", reason)

    def test_grandfathered_migrations_are_not_checked(self):
        names = sorted(f for f in os.listdir(MIGRATIONS) if f.endswith(".sql"))
        self.assertEqual(len(names), 26)
        failing = 0
        for name in names:
            rel = "apps/server/internal/Database/Migrations/" + name
            content = read(os.path.join(MIGRATIONS, name))
            self.assertIsNone(migration_rules.check_migration_compat(rel=rel, root="/nonexistent",
                                                                     final_content=content), name)
            failing += migration_rules.verdict(content, set()) is not None
        self.assertGreater(failing, 0)

    def test_other_paths_are_not_checked(self):
        for rel in ("apps/server/internal/Database/Migrations/testdata/compat/reject_m1_x.sql",
                    "apps/server/internal/Modules/Jobs/Queries/Jobs.sql", "deploy/x/00049_x.sql"):
            self.assertIsNone(migration_rules.check_migration_compat(rel=rel, root="/nonexistent",
                                                                     final_content="DROP TABLE \"Jobs\";"), rel)

    def test_file_on_main_is_not_checked_again_and_tags_come_from_git(self):
        with tempfile.TemporaryDirectory() as root:
            def git(*args):
                subprocess.run(["git", "-C", root, "-c", "user.name=t", "-c", "user.email=t@t", *args],
                               check=True, capture_output=True)
            os.makedirs(os.path.join(root, os.path.dirname(NEW)))
            with open(os.path.join(root, NEW), "w", encoding="utf-8") as fh:
                fh.write("-- +goose Up\nSELECT 1;\n")
            git("init", "-q", "-b", "main")
            git("add", ".")
            git("commit", "-q", "-m", "init")
            git("tag", "v1.0.0")
            self.assertIsNone(migration_rules.check_migration_compat(rel=NEW, root=root, final_content="SELECT 1;\n"))
            self.assertEqual(migration_rules.repo_tags(root), {"v1.0.0"})
            other = "apps/server/internal/Database/Migrations/00050_y.sql"
            self.assertIn("[M1]", migration_rules.check_migration_compat(rel=other, root=root, final_content="SELECT 1;\n"))


if __name__ == "__main__":
    unittest.main()
