import os
import sys
import unittest

HOOKS = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HOOKS)

from dhow_hooks import edit_rules

CLI = "apps/cli/internal/"
ROOT = os.path.dirname(os.path.dirname(HOOKS))


def edit(rel, added, old=""):
    return edit_rules.evaluate(rel=rel, root="/nonexistent", segments=[(added, old)], exists=False, final_content=added)


def imp(path):
    return 'package x\n\nimport "github.com/Khaymah/dhow/apps/cli/internal/' + path + '"\n'


def std(path):
    return 'package x\n\nimport "' + path + '"\n'


class CliLayoutRules(unittest.TestCase):
    def test_allowed_locations(self):
        for rel in ("App/App.go", "App/App.Lint.go", "Lint/Lint.go", "Output/Output.go", "Output/Messages.Ar.go",
                    "Output/Codes.Ar.go", "Output/Json.go", "Output/Golden/Golden.go", "Lint/lint_test.go"):
            self.assertIsNone(edit(CLI + rel, "package x\n"), rel)

    def test_unknown_layer_blocked(self):
        for rel in ("Utils/Util.go", "lint/Lint.go", "Main.go", "Config/Config.go"):
            self.assertIn("lives in one of", edit(CLI + rel, "package x\n"), rel)

    def test_folder_names_are_pascal_case(self):
        self.assertIn("PascalCase", edit(CLI + "Output/golden/Golden.go", "package x\n"))

    def test_file_names_are_pascal_case(self):
        for rel in ("Lint/lint.go", "Output/messages_ar.go", "App/app-lint.go"):
            self.assertIn("PascalCase", edit(CLI + rel, "package x\n"), rel)

    def test_test_files_keep_go_naming(self):
        self.assertIsNone(edit(CLI + "Output/golden_test.go", "package x\n"))

    def test_the_command_is_not_a_layer(self):
        self.assertIsNone(edit("apps/cli/cmd/dhow/main.go", "package main\n"))


class CliImportRules(unittest.TestCase):
    def test_app_wires_lint_and_output(self):
        for layer in ("Lint", "Output"):
            self.assertIsNone(edit(CLI + "App/App.go", imp(layer)), layer)

    def test_output_renders_lint_results(self):
        self.assertIsNone(edit(CLI + "Output/Text.go", imp("Lint")))

    def test_lint_is_a_leaf(self):
        for dst in ("Output", "App"):
            self.assertIsNotNone(edit(CLI + "Lint/Lint.go", imp(dst)), dst)
        self.assertIn("forbidden import", edit(CLI + "Lint/Lint.go", imp("Output")))

    def test_nothing_imports_app(self):
        for src in ("Lint/Lint.go", "Output/Text.go"):
            self.assertIn("composition root", edit(CLI + src, imp("App")), src)

    def test_unknown_cli_package_blocked(self):
        self.assertIn("not a CLI layer", edit(CLI + "App/App.go", imp("Utils")))

    def test_the_command_imports_app_only(self):
        self.assertIsNone(edit("apps/cli/cmd/dhow/main.go", imp("App")))
        for dst in ("Lint", "Output"):
            self.assertIn("imports App only", edit("apps/cli/cmd/dhow/main.go", imp(dst)), dst)

    def test_other_apps_are_never_imported(self):
        for target in ("apps/server/internal/Config", "apps/runner/internal/Config", "apps/web/x"):
            src = 'package x\n\nimport "github.com/Khaymah/dhow/' + target + '"\n'
            for rel in (CLI + "Lint/Lint.go", "apps/cli/cmd/dhow/main.go"):
                self.assertIn("never imports server, runner or web", edit(rel, src), rel + " -> " + target)

    def test_shared_packages_are_allowed(self):
        src = 'package x\n\nimport "github.com/Khaymah/dhow/internal/pipelinespec"\n'
        self.assertIsNone(edit(CLI + "Lint/Lint.go", src))

    def test_the_cli_stays_offline(self):
        for path in ("net", "net/http", "net/http/httptrace", "net/rpc", "net/smtp", "net/mail", "crypto/tls", "os/exec",
                     "plugin", "syscall", "golang.org/x/sys/unix", "golang.org/x/sys/windows", "golang.org/x/net/proxy",
                     "unsafe", "C"):
            for rel in (CLI + "Lint/Lint.go", CLI + "App/App.go", "apps/cli/cmd/dhow/main.go"):
                self.assertIn("works offline", edit(rel, std(path)), rel + " -> " + path)

    def test_the_upload_package_is_the_only_way_to_the_network(self):
        src = 'package x\n\nimport "github.com/Khaymah/dhow/internal/ingestpush"\n'
        for rel in (CLI + "App/PushCommand.go", CLI + "App/PushOutput.go"):
            self.assertIsNone(edit(rel, src), rel)
        self.assertIn("internal/ingestpush", edit(CLI + "App/PushCommand.go", std("net/http")))
        self.assertIsNone(edit("internal/ingestpush/Client.go", std("net/http")))

    def test_backquoted_imports_are_caught(self):
        for path in ("net", "os/exec", "syscall", "golang.org/x/sys/unix"):
            self.assertIn("works offline", edit(CLI + "Lint/Lint.go", "package x\n\nimport `" + path + "`\n"), path)
        src = "package x\n\nimport (\n\tx `net/http`\n)\n"
        self.assertIn("works offline", edit("apps/cli/cmd/dhow/main.go", src))
        self.assertIn("forbidden import", edit(CLI + "Lint/Lint.go", imp("Output").replace('"', "`")))
        src = "package x\n\nimport `github.com/Khaymah/dhow/apps/server/internal/Config`\n"
        self.assertIn("never imports server", edit(CLI + "Lint/Lint.go", src))

    def test_process_and_raw_system_calls_are_refused_everywhere(self):
        calls = ("os.StartProcess(name, argv, attr)", "syscall.Exec(p, a, e)", "syscall.ForkExec(p, a, at)",
                 "syscall.Syscall(n, a, b, c)", "syscall.RawSyscall6(n, a, b, c, d, e, f)", "syscall.Socket(1, 2, 3)",
                 "syscall.Connect(fd, sa)", "syscall.StartProcess (p, a, at)")
        for call in calls:
            for rel in (CLI + "App/App.go", "apps/cli/cmd/dhow/main.go", CLI + "App/app_test.go", "apps/cli/cmd/dhow/main_test.go"):
                self.assertIn("never starts a process", edit(rel, "package x\n\nfunc f() { _ = " + call + " }\n"), rel + " " + call)

    def test_process_calls_outside_the_cli_are_untouched(self):
        self.assertIsNone(edit("internal/safefile/x.go", "package x\n\nfunc f() { _, _ = os.StartProcess(a, b, c) }\n"))
        self.assertIsNone(edit(CLI + "App/App.go", "package x\n\nfunc f() { _ = myos.StartProcessName }\n"))

    def test_parsing_packages_are_not_network(self):
        for path in ("net/url", "net/netip", "os", "io/fs", "encoding/json"):
            self.assertIsNone(edit(CLI + "Lint/Lint.go", std(path)), path)

    def test_grouped_and_aliased_imports_are_scanned(self):
        src = 'package x\n\nimport (\n\t"fmt"\n\tout "github.com/Khaymah/dhow/apps/cli/internal/Output"\n)\n'
        self.assertIn("forbidden import", edit(CLI + "Lint/Lint.go", src))
        src = 'package x\n\nimport (\n\t"fmt"\n\thttp "net/http"\n)\n'
        self.assertIn("works offline", edit(CLI + "Output/Json.go", src))

    def test_existing_imports_are_not_rescanned(self):
        line = '\t"github.com/Khaymah/dhow/apps/cli/internal/Output"\n'
        self.assertIsNone(edit(CLI + "Lint/Lint.go", "x := 1\n" + line, old=line))

    def test_tests_are_exempt(self):
        self.assertIsNone(edit(CLI + "Lint/lint_test.go", imp("Output")))
        self.assertIsNone(edit("apps/cli/cmd/dhow/main_test.go", std("os/exec")))

    def test_other_trees_untouched(self):
        self.assertIsNone(edit("apps/runner/internal/Client/Client.go", std("net/http")))

    def test_the_real_cli_tree_passes(self):
        base = os.path.join(ROOT, "apps", "cli")
        if not os.path.isdir(base):
            self.skipTest("apps/cli is not in this checkout")
        checked = 0
        for folder, _, files in os.walk(base):
            for name in files:
                if not name.endswith(".go"):
                    continue
                path = os.path.join(folder, name)
                rel = os.path.relpath(path, ROOT).replace(os.sep, "/")
                with open(path, encoding="utf-8") as fh:
                    content = fh.read()
                self.assertIsNone(edit_rules.evaluate(rel=rel, root=ROOT, segments=[(content, "")], exists=True,
                                                      final_content=content), rel)
                checked += 1
        self.assertGreater(checked, 0)


if __name__ == "__main__":
    unittest.main()
