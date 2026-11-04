import os
import sys
import unittest

HOOKS = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HOOKS)

from dhow_hooks import edit_rules, port_rules

SERVER = "apps/server/internal/"
REPO = os.path.dirname(os.path.dirname(HOOKS))


def edit(rel, added, old="", exists=False, final=None):
    return edit_rules.evaluate(rel=rel, root="/nonexistent", segments=[(added, old)], exists=exists,
                               final_content=final if final is not None else added)


def imp(path):
    return 'package x\n\nimport "github.com/Khaymah/dhow/apps/server/internal/' + path + '"\n'


GOOD_PORT = (
    "package jobqueue\n\n"
    "import (\n\t\"context\"\n\n\tsqlc \"github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc\"\n)\n\n"
    "const OriginPipeline = \"pipeline\"\n\n"
    "type Request struct {\n\tName string\n}\n\n"
    "type Queue interface {\n\tEnqueue(ctx context.Context, tx *sqlc.Queries, r Request) error\n}\n"
)
PORT = SERVER + "Core/JobQueue/JobQueue.Port.go"


class PortImportDirection(unittest.TestCase):
    def test_every_planned_port_is_known(self):
        self.assertEqual(("JobQueue", "Triggers", "RunEvents", "CommitStatus", "ArtifactIndex", "Deployments"), port_rules.PORTS)

    def test_a_port_with_interfaces_and_types_is_allowed(self):
        self.assertIsNone(edit(PORT, GOOD_PORT))

    def test_modules_and_app_import_a_port(self):
        for path in ("Modules/Jobs/Jobs.Queue.go", "Modules/Pipelines/Pipelines.Service.go", "App/Wire.go"):
            for port in port_rules.PORTS:
                self.assertIsNone(edit(SERVER + path, imp("Core/" + port)), path + " " + port)

    def test_other_core_packages_never_import_a_port(self):
        for path in ("Core/Auth/Guard.go", "Core/EventBus/EventBus.go", "Core/Settings/Store.go"):
            r = edit(SERVER + path, imp("Core/JobQueue"))
            self.assertIn("only modules (the caller and the implementer) and App import the port Core/JobQueue", r, path)
        r = edit(SERVER + "Core/Triggers/Triggers.Port.go", imp("Core/JobQueue"))
        self.assertIn("the port Core/Triggers imports nothing from the server but Database/Sqlc", r)

    def test_config_and_database_never_import_a_port(self):
        for path in ("Config/Env.go", "Database/Tx/Tx.go"):
            self.assertIsNotNone(edit(SERVER + path, imp("Core/RunEvents")), path)

    def test_a_port_imports_only_sqlc_from_the_server(self):
        self.assertIsNone(edit(PORT, imp("Database/Sqlc")))
        self.assertIsNone(edit(PORT, imp("Core/JobQueue/Models")))
        for path in ("Core/Policy", "Core/Auth", "Config", "Database/Tx", "Core/RunEvents"):
            self.assertIn("imports nothing from the server but Database/Sqlc", edit(PORT, imp(path)), path)
        self.assertIn("Only App imports modules", edit(PORT, imp("Modules/Jobs")))

    def test_a_port_may_import_shared_packages_outside_the_server(self):
        src = 'package jobqueue\n\nimport runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"\n'
        self.assertIsNone(edit(PORT, src))

    def test_tests_of_a_port_are_exempt(self):
        self.assertIsNone(edit(SERVER + "Core/JobQueue/JobQueue_test.go", imp("Core/Policy") + "\nfunc TestX() {}\n"))
        self.assertIsNone(edit(SERVER + "Core/Auth/Guard_test.go", imp("Core/JobQueue")))


class PortContents(unittest.TestCase):
    def test_a_port_never_carries_a_func(self):
        for code in ("func New() Queue { return nil }\n", "func (r Request) Valid() bool { return true }\n"):
            self.assertIn("interfaces, data types and constants only", edit(PORT, GOOD_PORT + "\n" + code), code)

    def test_a_port_never_carries_state(self):
        self.assertIn("never a func or a var", edit(PORT, GOOD_PORT + "\nvar Default Queue\n"))

    def test_methods_inside_an_interface_are_fine(self):
        self.assertIsNone(edit(PORT, "x", old="", exists=True, final=GOOD_PORT))

    def test_ordinary_core_packages_may_hold_code(self):
        self.assertIsNone(edit(SERVER + "Core/Policy/Project.go", "package policy\n\nfunc ProjectRole() {}\n"))


class RealPortFiles(unittest.TestCase):
    def test_the_real_ports_pass(self):
        base = os.path.join(REPO, SERVER, "Core")
        found = 0
        for port in port_rules.PORTS:
            folder = os.path.join(base, port)
            if not os.path.isdir(folder):
                continue
            for name in sorted(os.listdir(folder)):
                if not name.endswith(".go"):
                    continue
                with open(os.path.join(folder, name), encoding="utf-8") as fh:
                    content = fh.read()
                found += 1
                self.assertIsNone(edit(SERVER + "Core/" + port + "/" + name, content), name)
        self.assertGreater(found, 0)

    def test_project_rules_count_as_declared_policies(self):
        path = os.path.join(REPO, SERVER, "Modules/Projects/Projects.Routes.go")
        with open(path, encoding="utf-8") as fh:
            content = fh.read()
        self.assertIn("policy.Project(", content)
        self.assertIsNone(edit(SERVER + "Modules/Projects/Projects.Routes.go", content))
        only_projects = content.replace("policy.Organization(", "policy.Project(")
        self.assertIsNone(edit(SERVER + "Modules/Projects/Projects.Routes.go", only_projects))


if __name__ == "__main__":
    unittest.main()
