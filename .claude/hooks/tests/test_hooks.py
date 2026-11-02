import json
import os
import subprocess
import sys
import tempfile
import unittest

HOOKS = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HOOKS)

from dhow_hooks import bash_rules, edit_rules


def edit(rel, added, old="", exists=False, final=None, root="/nonexistent"):
    return edit_rules.evaluate(rel=rel, root=root, segments=[(added, old)], exists=exists,
                               final_content=final if final is not None else added)


GOOD_TABLE = (
    'CREATE TABLE "Projects" (\n'
    '    "Id" uuid PRIMARY KEY DEFAULT uuidv7(),\n'
    '    "CreatedAt" timestamptz NOT NULL DEFAULT now(),\n'
    '    "UpdatedAt" timestamptz NOT NULL DEFAULT now(),\n'
    '    "IsDeleted" boolean NOT NULL DEFAULT false,\n'
    '    "Name" text NOT NULL\n'
    ');\n'
)
SERVER = "apps/server/internal/"


class CommentRules(unittest.TestCase):
    def test_go_line_comment_blocked(self):
        self.assertIn("no comments", edit("apps/server/x.go", "package x\n\n// Get returns x\nfunc Get() {}\n"))

    def test_go_doc_block_blocked(self):
        self.assertIsNotNone(edit("internal/a/a.go", "package a\n/* doc */\n"))

    def test_go_directives_allowed(self):
        src = "//go:build linux\n\npackage a\n\n//go:embed x\nvar f string\n\nvar x = 1 //nolint:gosec // reason\n"
        self.assertIsNone(edit("internal/a/a.go", src))

    def test_go_important_allowed(self):
        self.assertIsNone(edit("internal/a/a.go", "package a\n// IMPORTANT: lock order matters\n"))

    def test_go_comment_markers_inside_strings_ignored(self):
        src = 'package a\nvar u = "https://x"\nvar r = `/* not */`\nvar c = \'/\'\n'
        self.assertIsNone(edit("internal/a/a.go", src))

    def test_existing_comment_not_flagged(self):
        self.assertIsNone(edit("internal/a/a.go", "// old\nx := 1\n", old="// old\n"))

    def test_ts_comment_blocked(self):
        self.assertIsNotNone(edit("apps/web/Src/Core/Ui/A.ts", "export const IX = 1;\n// hi\n"))

    def test_ts_directive_allowed(self):
        self.assertIsNone(edit("apps/web/Src/Core/Ui/A.ts", "// eslint-disable-next-line no-alert\nexport const A = 1;\n"))

    def test_markdown_not_checked(self):
        self.assertIsNone(edit("docs/a.md", "<!-- c -->\n", exists=True))

    def test_proto_comments_blocked(self):
        proto = "packages/proto/dhow/runner/v1/lease.proto"
        self.assertIn("no comments", edit(proto, 'syntax = "proto3";\n// Lease asks for a job\nmessage A {}\n'))
        self.assertIn("no comments", edit(proto, "message A {\n  int32 b = 1; /* why */\n}\n"))

    def test_proto_directives_allowed(self):
        src = 'syntax = "proto3";\n// buf:lint:ignore RPC_REQUEST_STANDARD_NAME\nmessage A { string b = 1; }\n'
        self.assertIsNone(edit("packages/proto/dhow/runner/v1/lease.proto", src))
        self.assertIsNone(edit("packages/proto/dhow/runner/v1/a.proto", 'option go_package = "x//y";\n'))


class GoRules(unittest.TestCase):
    def test_shell_exec_blocked(self):
        r = edit("apps/runner/x.go", 'package x\nfunc f(){ exec.CommandContext(ctx, "sh", "-c", s) }\n')
        self.assertIn("sh -c", r)

    def test_argv_exec_allowed(self):
        self.assertIsNone(edit("apps/runner/x.go", 'package x\nfunc f(){ exec.CommandContext(ctx, "git", "--", r) }\n'))

    def test_lowercase_json_tag_blocked(self):
        self.assertIn("PascalCase", edit("internal/m/m.go", 'package m\ntype A struct{ B int `json:"setupRequired"` }\n'))

    def test_pascal_json_tag_allowed(self):
        self.assertIsNone(edit("internal/m/m.go", 'package m\ntype A struct{ B int `json:"SetupRequired,omitempty"`; C int `json:"-"` }\n'))

    def test_sql_string_blocked(self):
        self.assertIn("sqlc", edit("internal/r/r.go", 'package r\nvar q = fmt.Sprintf("SELECT * FROM x WHERE id=%d", id)\n'))

    def test_panic_blocked_outside_tests(self):
        self.assertIn("panic", edit("internal/r/r.go", "package r\nfunc f(){ panic(err) }\n"))
        self.assertIsNone(edit("internal/r/r_test.go", "package r\nfunc f(){ panic(err) }\n"))

    def test_getenv_only_in_config(self):
        self.assertIn("config", edit("apps/server/internal/Core/Util/Util.go", 'package util\nvar v = os.Getenv("A")\n'))
        self.assertIsNone(edit("apps/server/internal/Config/Env.go", 'package config\nvar v = os.Getenv("A")\n'))

    def test_fmt_print_blocked(self):
        self.assertIn("slog", edit("apps/server/internal/Core/Util/Util.go", 'package util\nfunc f(){ fmt.Println("x") }\n'))


class TsRules(unittest.TestCase):
    def test_any_blocked(self):
        self.assertIn("any", edit("apps/web/Src/Core/Ui/A.ts", "export const X = (v: any) => v;\n"))

    def test_naming(self):
        self.assertIn("`I`", edit("apps/web/Src/Core/Ui/A.ts", "export interface Props { A: string }\n"))
        self.assertIn("`T`", edit("apps/web/Src/Core/Ui/A.ts", "export type Role = 'a' | 'b';\n"))
        self.assertIn("`E`", edit("apps/web/Src/Core/Ui/A.ts", "export enum Status { Live = 'live' }\n"))
        self.assertIsNone(edit("apps/web/Src/Core/Ui/A.ts", "export interface IProps { A: string }\nexport type TRole = 'a';\nexport enum EStatus { Live = 'live' }\n"))

    def test_console_blocked(self):
        self.assertIsNotNone(edit("apps/web/Src/Core/Ui/A.ts", "console.log(1);\n"))
        self.assertIsNone(edit("apps/web/Src/Core/Ui/A.test.ts", "console.log(1);\n"))

    def test_inner_html_and_eval_blocked(self):
        self.assertIsNotNone(edit("apps/web/Src/Core/Ui/A.tsx", "export const A = () => <div dangerouslySetInnerHTML={x} />;\n"))
        self.assertIsNotNone(edit("apps/web/Src/Core/Ui/A.ts", "const f = new Function('x');\n"))

    def test_function_component_blocked(self):
        self.assertIn("arrow", edit("apps/web/Src/Core/Ui/A.tsx", "export function Card() { return null; }\n"))

    def test_physical_direction_blocked(self):
        self.assertIn("logical", edit("apps/web/Src/Core/Ui/A.tsx", 'export const A = () => <div className="ml-2 text-left" />;\n'))
        self.assertIn("logical", edit("apps/web/Src/Styles/A.css", ".x { margin-left: 4px; }\n"))
        self.assertIsNone(edit("apps/web/Src/Core/Ui/A.tsx", 'export const A = () => <div className="ms-2 text-start" />;\n'))
        self.assertIsNone(edit("apps/web/Src/Styles/A.css", ".x { margin-inline-start: 4px; inset-inline-end: 0; }\n"))


class FileRules(unittest.TestCase):
    def test_generated_blocked(self):
        self.assertIn("generated", edit("apps/server/internal/Modules/System/Gen/System.gen.go", "x", exists=True))
        self.assertIn("generated", edit("apps/server/internal/Database/Sqlc/Instance.sql.go", "x", exists=True))
        self.assertIn("generated", edit("apps/server/internal/legacy/gen/api.gen.go", "x", exists=True))
        self.assertIn("generated", edit("apps/web/pnpm-lock.yaml", "x", exists=True))
        self.assertIn("generated", edit("packages/proto/gen/go/dhow/runner/v1/lease.pb.go", "x", exists=True))
        self.assertIn("generated", edit("packages/proto/gen/go/dhow/runner/v1/runnerv1connect/lease.connect.go", "x"))
        self.assertIsNone(edit("packages/proto/contract_test.go", "package proto_test\n"))

    def test_secret_blocked(self):
        self.assertIn("private key", edit("deploy/x.yaml", "-----BEGIN " + "OPENSSH PRIVATE KEY-----\n"))
        self.assertIn("GitHub token", edit("deploy/x.yaml", "token: ghp_" + "a" * 36 + "\n"))

    def test_old_org_name_blocked(self):
        old = "Arab" + "-Open" + "Source"
        self.assertIn("Khaymah", edit("apps/runner/internal/Agent/X.go", 'import "github.com/' + old + '/dhow/x"\n'))
        self.assertIn("Khaymah", edit("docs/install/compose.md", "ghcr.io/" + old.lower() + "/dhow\n"))
        self.assertIsNone(edit("docs/install/compose.md", "ghcr.io/khaymah/dhow\n"))

    def test_env_file_blocked(self):
        self.assertIsNotNone(edit(".env", "A=1\n"))
        self.assertIsNone(edit(".env.example", "A=\n"))

    def test_markdown_location(self):
        self.assertIsNotNone(edit("apps/server/NOTES.md", "# x\n"))
        self.assertIsNone(edit("Plan/Progress/2026-10-28.md", "# x\n"))
        self.assertIsNone(edit("docs/new.md", "# x\n"))
        self.assertIsNone(edit("docs/en/new.md", "# x\n"))
        self.assertIn("docs/en/", edit("apps/server/NOTES.md", "# x\n"))

    def test_root_markdown_pairs(self):
        for rel in ("README.md", "README.en.md", "CONTRIBUTING.md", "CONTRIBUTING.en.md", "SECURITY.md",
                    "SECURITY.en.md", "CLAUDE.md", "LICENSE.md"):
            self.assertIsNone(edit(rel, "# x\n"), rel)
        for rel in ("README.ar.md", "NOTES.md", "CLAUDE.en.md"):
            self.assertIsNotNone(edit(rel, "# x\n"), rel)

    def test_length_limit(self):
        big = "package a\n" + "var x = 1\n" * 500
        self.assertIn("lines", edit("internal/a/a.go", "var y = 2\n", final=big))

    def test_sql_naming(self):
        self.assertIn("table names", edit("apps/server/internal/Database/Migrations/00002_x.sql", "CREATE TABLE projects (\n"))
        self.assertIn("column names", edit("apps/server/internal/Database/Migrations/00002_x.sql", 'CREATE TABLE "Projects" (\n  created_at timestamptz\n'))
        self.assertIsNone(edit("apps/server/internal/Database/Migrations/00002_x.sql", GOOD_TABLE))


class ServerLayoutRules(unittest.TestCase):
    def test_allowed_locations(self):
        for rel in ("App/App.go", "Config/Env.go", "Core/Errors/AppError.go", "Database/Migrations/Migrator.go",
                    "Modules/System/System.Service.go", "Modules/System/Models/Instance.Model.go",
                    "Modules/System/Test/Unit/Service_test.go", "Core/I18n/I18n.Ar.go"):
            self.assertIsNone(edit(SERVER + rel, "package x\n"), rel)

    def test_unknown_top_level_folder_blocked(self):
        for rel in ("handlers/health.go", "services/Health.go", "Utils/Util.go", "Main.go"):
            self.assertIn("Modules/<Module>", edit(SERVER + rel, "package x\n"), rel)

    def test_files_directly_in_nested_layers_blocked(self):
        for rel in ("Core/Util.go", "Database/Util.go", "Modules/Util.go"):
            self.assertIn("named subfolder", edit(SERVER + rel, "package x\n"), rel)

    def test_folder_names_are_pascal_case(self):
        self.assertIn("PascalCase", edit(SERVER + "Core/eventbus/EventBus.go", "package x\n"))
        self.assertIn("PascalCase", edit(SERVER + "Modules/System/test/Unit/A_test.go", "package x\n"))
        self.assertIn("PascalCase", edit(SERVER + "Modules/system/System.Service.go", "package x\n"))

    def test_file_names_are_pascal_case(self):
        self.assertIn("PascalCase", edit(SERVER + "Modules/System/system.service.go", "package x\n"))
        self.assertIn("PascalCase", edit(SERVER + "Core/Errors/app_error.go", "package x\n"))
        self.assertIn("PascalCase", edit(SERVER + "App/server.go", "package x\n"))

    def test_test_and_generated_files_exempt_from_file_naming(self):
        self.assertIsNone(edit(SERVER + "Core/Errors/app_error_test.go", "package x\n"))
        self.assertIsNone(edit(SERVER + "Core/Errors/AppError_test.go", "package x\n"))

    def test_files_outside_server_internal_untouched(self):
        self.assertIsNone(edit("apps/server/cmd/dhow-server/main.go", "package main\n"))
        self.assertIsNone(edit("internal/logging/logging.go", "package logging\n"))


def imp(path):
    return 'package x\n\nimport "github.com/Khaymah/dhow/apps/server/internal/' + path + '"\n'


GOOD_ROUTES = (
    "package audit\n\n"
    "func Policies() []policy.Rule {\n"
    "\treturn []policy.Rule{policy.Organization(http.MethodGet, \"/organizations/{id}/audit-events\", policy.AuditRead, \"id\")}\n"
    "}\n\n"
    "func Routes(r chi.Router, d Deps) {\n"
    "\tguard := d.Guard.Middleware(policy.NewTable(Policies()...))\n"
    "\tgen.HandlerWithOptions(strict, gen.ChiServerOptions{BaseRouter: r, Middlewares: []gen.MiddlewareFunc{guard}})\n"
    "}\n"
)
ROUTES = SERVER + "Modules/Audit/Audit.Routes.go"


class RoutePolicyRules(unittest.TestCase):
    def test_routes_with_declared_policies_allowed(self):
        self.assertIsNone(edit(ROUTES, GOOD_ROUTES))

    def test_generated_routes_without_any_policy_blocked(self):
        src = "package audit\n\nfunc Routes(r chi.Router) {\n\tgen.HandlerWithOptions(strict, gen.ChiServerOptions{BaseRouter: r})\n}\n"
        self.assertIn("declared policy", edit(ROUTES, src))

    def test_policies_without_enforcement_blocked(self):
        src = GOOD_ROUTES.replace("Middlewares: []gen.MiddlewareFunc{guard}", "BaseRouter: r")
        self.assertIn("declared policy", edit(ROUTES, src))

    def test_enforcement_without_a_table_blocked(self):
        src = GOOD_ROUTES.replace("policy.NewTable(Policies()...)", "nil")
        self.assertIn("declared policy", edit(ROUTES, src))

    def test_enforcement_without_declarations_blocked(self):
        src = (GOOD_ROUTES.replace("policy.Organization(", "other.Organization(")
               .replace("policy.Rule{", "[]int{").replace("[]policy.Rule", "[]int"))
        self.assertIn("declared policy", edit(ROUTES, src))

    def test_hand_registered_routes_blocked(self):
        for call in ('r.Get("/x", h)', 'api.Post("/x", h)', 'r.Delete("/x/{id}", h)', 'r.HandleFunc("/x", h)',
                     'r.Method("GET", "/x", h)', 'r.Mount("/x", h)', "r.Get(`/x`, h)"):
            src = GOOD_ROUTES + "\nfunc More(r chi.Router) {\n\t" + call + "\n}\n"
            self.assertIn("do not register routes by hand", edit(ROUTES, src), call)

    def test_edit_that_only_touches_other_lines_is_checked_against_the_whole_file(self):
        before = GOOD_ROUTES
        after = GOOD_ROUTES.replace("Middlewares: []gen.MiddlewareFunc{guard}", "BaseRouter: r")
        self.assertIn("declared policy", edit(ROUTES, "BaseRouter: r", old="Middlewares: []gen.MiddlewareFunc{guard}", exists=True, final=after))
        self.assertIsNone(edit(ROUTES, "BaseRouter: r", old="x", exists=True, final=before))

    def test_other_go_files_and_tests_are_not_route_files(self):
        raw = "package audit\n\nfunc More(r chi.Router) {\n\tr.Get(\"/x\", h)\n}\n"
        self.assertIsNone(edit(SERVER + "Modules/Audit/Audit.Service.go", raw))
        self.assertIsNone(edit(SERVER + "Modules/Audit/Audit.Routes_test.go", raw))
        self.assertIsNone(edit(SERVER + "Modules/Audit/Test/Unit/Routes_test.go", raw))

    def test_route_files_that_register_nothing_are_fine(self):
        self.assertIsNone(edit(ROUTES, "package audit\n\ntype Deps struct{}\n"))

    def test_instance_rules_count_as_declared_policies(self):
        src = GOOD_ROUTES.replace(
            "policy.Organization(http.MethodGet, \"/organizations/{id}/audit-events\", policy.AuditRead, \"id\")",
            "policy.Instance(http.MethodPost, \"/organizations\", policy.OrgCreate)")
        self.assertIn("policy.Instance(", src)
        self.assertIsNone(edit(ROUTES, src))

    def test_the_system_module_declares_its_policies(self):
        src = GOOD_ROUTES.replace("package audit", "package system")
        self.assertIsNone(edit(SERVER + "Modules/System/System.Routes.go", src))


GOOD_CONNECT_ROUTES = (
    "package runners\n\n"
    "func Procedures() []policy.Rule {\n"
    "\treturn []policy.Rule{policy.Runner(runnerv1connect.AuthServiceChallengeProcedure, policy.RunnerChallenge)}\n"
    "}\n\n"
    "func ConnectRoutes(mux *http.ServeMux, d Deps) {\n"
    "\tgate := d.Gate.RequestGate(policy.NewTable(Procedures()...))\n"
    "\tmux.Handle(runnerv1connect.NewAuthServiceHandler(handler, gate))\n"
    "}\n"
)
RUNNER_ROUTES = SERVER + "Modules/Runners/Runners.Routes.go"


class RunnerPlaneRules(unittest.TestCase):
    def test_connect_mount_with_policies_and_gate_allowed(self):
        self.assertIsNone(edit(RUNNER_ROUTES, GOOD_CONNECT_ROUTES))
        self.assertIsNone(edit(SERVER + "Modules/Jobs/Jobs.Routes.go", GOOD_CONNECT_ROUTES.replace("package runners", "package jobs")))
        self.assertIsNone(edit(SERVER + "Modules/Artifacts/Artifacts.Routes.go",
                               GOOD_CONNECT_ROUTES.replace("package runners", "package artifacts")))

    def test_policy_runner_counts_as_a_declared_policy(self):
        src = GOOD_ROUTES.replace(
            'policy.Organization(http.MethodGet, "/organizations/{id}/audit-events", policy.AuditRead, "id")',
            "policy.Runner(\"/dhow.runner.v1.AuthService/Challenge\", policy.RunnerChallenge)")
        self.assertIsNone(edit(ROUTES, src))

    def test_connect_mount_outside_the_runner_modules_blocked(self):
        for rel in (SERVER + "Modules/Audit/Audit.Routes.go", SERVER + "Modules/Runners/Runners.Service.go",
                    SERVER + "App/App.go", SERVER + "Core/RunnerAuth/Gate.go"):
            self.assertIn("mounted only in", edit(rel, GOOD_CONNECT_ROUTES), rel)

    def test_connect_mount_without_procedure_rules_blocked(self):
        src = GOOD_CONNECT_ROUTES.replace("policy.Runner(", "other.Runner(")
        self.assertIn("needs a declared policy", edit(RUNNER_ROUTES, src))

    def test_connect_mount_without_a_table_blocked(self):
        src = GOOD_CONNECT_ROUTES.replace("policy.NewTable(Procedures()...)", "nil")
        self.assertIn("needs a declared policy", edit(RUNNER_ROUTES, src))

    def test_connect_mount_without_the_gate_blocked(self):
        src = GOOD_CONNECT_ROUTES.replace("d.Gate.RequestGate(policy.NewTable(Procedures()...))",
                                          "connect.WithReadMaxBytes(1)\n\t_ = policy.NewTable(Procedures()...)")
        self.assertIn("needs a declared policy", edit(RUNNER_ROUTES, src))

    def test_connect_mount_checks_the_whole_file_on_partial_edits(self):
        after = GOOD_CONNECT_ROUTES.replace("d.Gate.RequestGate(policy.NewTable(Procedures()...))", "nil")
        self.assertIn("needs a declared policy", edit(RUNNER_ROUTES, "nil", old="x", exists=True, final=after))

    def test_hand_registered_connect_paths_blocked(self):
        src = GOOD_CONNECT_ROUTES + '\nfunc More(mux *http.ServeMux) {\n\tmux.Handle("/dhow.runner.v1.AuthService/", h)\n}\n'
        self.assertIn("do not register routes by hand", edit(RUNNER_ROUTES, src))

    def test_tests_and_test_folders_may_build_handlers(self):
        self.assertIsNone(edit(SERVER + "Modules/Runners/Test/E2E/Gate_test.go", GOOD_CONNECT_ROUTES))
        self.assertIsNone(edit(SERVER + "Core/RunnerAuth/Gate_test.go", GOOD_CONNECT_ROUTES))

    def test_only_app_runners_jobs_and_artifacts_import_runner_auth(self):
        for path in ("App/App.go", "App/Wire.go", "Modules/Runners/Runners.Service.go", "Modules/Jobs/Jobs.Lease.go",
                     "Modules/Artifacts/Artifacts.Routes.go", "Core/RunnerAuth/Keys.go"):
            self.assertIsNone(edit(SERVER + path, imp("Core/RunnerAuth")), path)
        for path in ("Modules/Session/Session.Service.go", "Modules/Audit/Audit.Service.go", "Core/Response/Response.Helper.go",
                     "Core/Middleware/AccessLog.go"):
            self.assertIn("only App, Modules/Runners, Modules/Jobs and Modules/Artifacts import Core/RunnerAuth",
                          edit(SERVER + path, imp("Core/RunnerAuth")), path)

    def test_browser_and_runner_auth_never_import_each_other(self):
        self.assertIn("separate planes", edit(SERVER + "Core/RunnerAuth/Gate.go", imp("Core/Auth")))
        self.assertIn("separate planes", edit(SERVER + "Core/Auth/Guard.go", imp("Core/RunnerAuth")))
        self.assertIsNone(edit(SERVER + "Core/RunnerAuth/Gate.go", imp("Core/Middleware")))

    def test_the_real_runner_routes_file_passes(self):
        path = os.path.join(os.path.dirname(HOOKS), "..", RUNNER_ROUTES)
        with open(path, encoding="utf-8") as fh:
            content = fh.read()
        self.assertIsNone(edit(RUNNER_ROUTES, content))


class ServerImportRules(unittest.TestCase):
    def test_module_cannot_import_another_module(self):
        r = edit(SERVER + "Modules/Billing/Billing.Service.go", imp("Modules/System/Models"))
        self.assertIn("EventBus", r)
        self.assertIn("Modules/Billing must not import Modules/System", r)

    def test_module_may_import_itself_and_core_and_database(self):
        for path in ("Modules/Billing/Models", "Core/Errors", "Database/Sqlc", "Config"):
            self.assertIsNone(edit(SERVER + "Modules/Billing/Billing.Service.go", imp(path)), path)

    def test_core_cannot_import_modules_or_app(self):
        self.assertIn("Only App imports modules", edit(SERVER + "Core/Response/Response.Helper.go", imp("Modules/System")))
        self.assertIn("composition root", edit(SERVER + "Core/Response/Response.Helper.go", imp("App")))

    def test_core_may_import_core(self):
        self.assertIsNone(edit(SERVER + "Core/Response/Response.Helper.go", imp("Core/Errors")))

    def test_config_cannot_import_core_modules_or_app(self):
        for path in ("Core/Errors", "Modules/System", "App"):
            self.assertIsNotNone(edit(SERVER + "Config/Env.go", imp(path)), path)

    def test_database_cannot_import_modules_core_or_app(self):
        for path in ("Modules/System", "Core/Errors", "App"):
            self.assertIsNotNone(edit(SERVER + "Database/Migrations/Migrator.go", imp(path)), path)

    def test_only_app_imports_modules(self):
        self.assertIsNone(edit(SERVER + "App/App.go", imp("Modules/System")))
        self.assertIsNone(edit(SERVER + "App/App.go", imp("Core/Middleware")))
        self.assertIsNotNone(edit(SERVER + "Config/Logger.go", imp("Modules/System")))
        self.assertIsNotNone(edit(SERVER + "Database/Migrations/Migrator.go", imp("Modules/System")))

    def test_modules_cannot_import_app(self):
        self.assertIn("composition root", edit(SERVER + "Modules/System/System.Routes.go", imp("App")))

    def test_aliased_and_grouped_imports_are_scanned(self):
        src = ('package x\n\nimport (\n\t"fmt"\n\tapperrors "github.com/Khaymah/dhow/apps/server/internal/Modules/System"\n)\n')
        self.assertIsNotNone(edit(SERVER + "Core/Errors/AppError.go", src))

    def test_existing_imports_are_not_rescanned(self):
        line = '\t"github.com/Khaymah/dhow/apps/server/internal/Modules/System"\n'
        self.assertIsNone(edit(SERVER + "Core/Errors/AppError.go", "x := 1\n" + line, old=line))

    def test_test_files_and_test_folders_are_exempt(self):
        self.assertIsNone(edit(SERVER + "Core/Errors/AppError_test.go", imp("Modules/System")))
        self.assertIsNone(edit(SERVER + "Modules/System/Test/Support/Fakes.go", imp("App")))

    def test_other_module_imports_are_not_flagged_outside_the_server(self):
        self.assertIsNone(edit("deploy/bench/x.go", imp("Modules/System")))


class DatabaseRules(unittest.TestCase):
    MIGRATION = "apps/server/internal/Database/Migrations/00002_projects.sql"

    def test_table_with_base_columns_allowed(self):
        self.assertIsNone(edit(self.MIGRATION, GOOD_TABLE))

    def test_instance_singleton_is_the_only_exception(self):
        singleton = 'CREATE TABLE "Instance" (\n    "Id" smallint PRIMARY KEY DEFAULT 1,\n    "CreatedAt" timestamptz NOT NULL DEFAULT now()\n);\n'
        self.assertIsNone(edit(self.MIGRATION, singleton))
        self.assertIn("base column", edit(self.MIGRATION, singleton.replace("Instance", "Settings")))

    def test_each_base_column_is_required(self):
        for column in ('"Id" uuid PRIMARY KEY DEFAULT uuidv7()', '"CreatedAt" timestamptz NOT NULL DEFAULT now()',
                       '"UpdatedAt" timestamptz NOT NULL DEFAULT now()', '"IsDeleted" boolean NOT NULL DEFAULT false'):
            broken = GOOD_TABLE.replace("    " + column + ",\n", "")
            self.assertIn(column, edit(self.MIGRATION, broken), column)

    def test_base_column_types_are_checked(self):
        self.assertIn("uuidv7", edit(self.MIGRATION, GOOD_TABLE.replace("uuid PRIMARY KEY DEFAULT uuidv7()", "bigserial PRIMARY KEY")))

    PARTITIONED = (
        'CREATE TABLE "LogChunks" (\n'
        '    "Id" uuid NOT NULL DEFAULT uuidv7(),\n'
        '    "CreatedAt" timestamptz NOT NULL DEFAULT now(),\n'
        '    "UpdatedAt" timestamptz NOT NULL DEFAULT now(),\n'
        '    "IsDeleted" boolean NOT NULL DEFAULT false,\n'
        '    "Data" bytea NOT NULL,\n'
        '    PRIMARY KEY ("Id", "CreatedAt")\n'
        ') PARTITION BY RANGE ("CreatedAt");\n'
    )

    def test_partitioned_table_may_use_a_composite_key_starting_with_id(self):
        self.assertIsNone(edit(self.MIGRATION, self.PARTITIONED))
        self.assertIsNone(edit(self.MIGRATION, self.PARTITIONED.replace(") PARTITION", ")\nPARTITION")))

    def test_composite_key_without_partition_by_is_refused(self):
        plain = self.PARTITIONED.replace(' PARTITION BY RANGE ("CreatedAt")', "")
        self.assertIn("uuidv7", edit(self.MIGRATION, plain))
        later = plain + 'CREATE TABLE "Other" (\n    "Id" uuid PRIMARY KEY DEFAULT uuidv7()\n) PARTITION BY RANGE ("Id");\n'
        self.assertIn("LogChunks", edit(self.MIGRATION, later))

    def test_partitioned_table_still_needs_id_first_in_the_key_and_the_other_base_columns(self):
        self.assertIn("uuidv7", edit(self.MIGRATION, self.PARTITIONED.replace('("Id", "CreatedAt")', '("CreatedAt", "Id")')))
        self.assertIn("uuidv7", edit(self.MIGRATION, self.PARTITIONED.replace("uuid NOT NULL DEFAULT uuidv7()", "uuid NOT NULL")))
        no_flag = self.PARTITIONED.replace('    "IsDeleted" boolean NOT NULL DEFAULT false,\n', "")
        self.assertIn("IsDeleted", edit(self.MIGRATION, no_flag))

    def test_dynamic_partition_names_are_format_identifiers(self):
        ddl = "EXECUTE format('CREATE TABLE %I.%I PARTITION OF %s FOR VALUES FROM (%L) TO (%L)', a, b, c, d, e);\n"
        self.assertIsNone(edit(self.MIGRATION, ddl))
        self.assertIsNone(edit(self.MIGRATION, ddl.replace("%I.%I", "%I")))
        self.assertIn("PascalCase", edit(self.MIGRATION, ddl.replace("%I.%I", "%s")))
        self.assertIn("PascalCase", edit(self.MIGRATION, ddl.replace("%I.%I", "log_chunks_x")))

    def test_a_dynamic_partition_copied_with_like_needs_no_columns_of_its_own(self):
        ddl = "EXECUTE format('CREATE TABLE %I.%I (LIKE %s INCLUDING ALL)', s, n, p);\n"
        self.assertIsNone(edit(self.MIGRATION, ddl))
        self.assertIsNone(edit(self.MIGRATION, ddl.replace("%s INCLUDING ALL", '"LogChunks"')))
        self.assertIn("base column", edit(self.MIGRATION, ddl.replace("LIKE %s INCLUDING ALL", '"Data" bytea')))
        self.assertIn("base column", edit(self.MIGRATION, ddl.replace("%s INCLUDING ALL", "%s INCLUDING ALL, \"Extra\" TEXT")))
        self.assertIn("base column", edit(self.MIGRATION, 'CREATE TABLE "Copy" (LIKE "LogChunks" INCLUDING ALL);\n'),
                      "a named table must declare its columns")

    def test_translation_tables_follow_the_same_rule(self):
        translations = GOOD_TABLE.replace("Projects", "ProjectTranslations").replace('"Name" text NOT NULL', '"Locale" text NOT NULL')
        self.assertIsNone(edit(self.MIGRATION, translations))
        self.assertIsNotNone(edit(self.MIGRATION, 'CREATE TABLE "ProjectTranslations" (\n  "Locale" text\n);\n'))

    def test_existing_table_not_rechecked_on_edit(self):
        old = 'CREATE TABLE "Legacy" (\n  "Id" int\n);\n'
        self.assertIsNone(edit(self.MIGRATION, old + "\n", old=old))

    def test_timestamp_without_time_zone_blocked(self):
        self.assertIn("timestamptz", edit(self.MIGRATION, GOOD_TABLE.replace('"Name" text NOT NULL', '"Seen" timestamp NOT NULL')))
        self.assertIn("timestamptz", edit(self.MIGRATION, GOOD_TABLE.replace('"Name" text NOT NULL', '"Seen" TIMESTAMP(3)')))
        self.assertIsNone(edit(self.MIGRATION, GOOD_TABLE.replace('"Name" text NOT NULL', '"Seen" timestamp with time zone')))

    def test_varchar_blocked(self):
        self.assertIn("TEXT", edit(self.MIGRATION, GOOD_TABLE.replace('"Name" text NOT NULL', '"Name" VARCHAR(50) NOT NULL')))
        self.assertIn("TEXT", edit(self.MIGRATION, GOOD_TABLE.replace('"Name" text NOT NULL', '"Name" character varying(50)')))

    def test_ar_en_columns_blocked(self):
        for column in ("NameAr", "NameEn", "DescriptionAr"):
            r = edit(self.MIGRATION, GOOD_TABLE.replace('"Name" text NOT NULL', '"%s" text NOT NULL' % column))
            self.assertIn("Translations", r, column)
        self.assertIsNone(edit(self.MIGRATION, GOOD_TABLE.replace('"Name" text NOT NULL', '"Token" text NOT NULL')))

    def test_delete_from_in_module_queries_blocked(self):
        path = "apps/server/internal/Modules/System/Queries/Project.sql"
        self.assertIn("soft", edit(path, '-- name: DeleteProject :exec\nDELETE FROM "Projects" WHERE "Id" = $1;\n'))
        self.assertIsNone(edit(path, '-- name: DeleteProject :exec\nUPDATE "Projects" SET "IsDeleted" = true WHERE "Id" = $1;\n'))
        self.assertIsNone(edit("apps/server/internal/Database/Migrations/00003_x.sql", 'DELETE FROM "Instance" WHERE "Id" = 2;\n'))

    RETENTION = "apps/server/internal/Database/Retention/Queries/Retention.sql"
    PURGE = ('-- name: PurgeOidcFlows :execrows\nDELETE FROM "OidcFlows"\nWHERE "Id" IN (\n'
             '    SELECT f."Id" FROM "OidcFlows" f WHERE f."ExpiresAt" < @before LIMIT @batch FOR UPDATE SKIP LOCKED\n);\n')

    def test_retention_may_delete_expired_rows_of_temporary_tables(self):
        self.assertIsNone(edit(self.RETENTION, self.PURGE))
        for table in ("LoginThrottles", "RunnerThrottles", "AuthChallenges", "Invitations", "SetupTokens", "RateLimits"):
            self.assertIsNone(edit(self.RETENTION, self.PURGE.replace('"OidcFlows"', '"%s"' % table)), table)

    def test_retention_may_not_delete_from_a_table_with_history(self):
        for table in ("AuditEvents", "Memberships", "Sessions", "Users", "ApiTokens"):
            self.assertIn("temporary tables", edit(self.RETENTION, self.PURGE.replace('"OidcFlows"', '"%s"' % table)), table)

    def test_retention_selects_from_the_table_it_deletes_from(self):
        mixed = self.PURGE.replace('FROM "OidcFlows" f', 'FROM "AuditEvents" f')
        self.assertIn("same table", edit(self.RETENTION, mixed))

    def test_retention_delete_needs_the_batch_template(self):
        for statement in ('DELETE FROM "OidcFlows";', 'DELETE FROM "OidcFlows" WHERE "Id" = @id;',
                          'DELETE FROM "OidcFlows" USING "Users" WHERE "ExpiresAt" < @before;',
                          'DELETE FROM ONLY "OidcFlows" WHERE "ExpiresAt" < @before;',
                          'WITH d AS (DELETE FROM "AuditEvents" WHERE "CreatedAt" < @before RETURNING 1) SELECT 1;'):
            self.assertIn("exactly", edit(self.RETENTION, "-- name: P :execrows\n" + statement + "\n"), statement)

    def test_retention_delete_needs_an_anded_time_bound(self):
        for conds in ('true', 'f."Id" = @id', '(f."ExpiresAt" < @before OR true)', 'f."ExpiresAt" > @before'):
            purge = self.PURGE.replace('f."ExpiresAt" < @before', conds)
            self.assertIn("fixed period", edit(self.RETENTION, purge), conds)

    def test_retention_delete_may_not_use_a_top_level_or(self):
        for conds in ('f."ExpiresAt" < @before OR true', 'f."ExpiresAt" < @before or f."Id" IS NOT NULL'):
            self.assertIn("OR", edit(self.RETENTION, self.PURGE.replace('f."ExpiresAt" < @before', conds)), conds)

    def test_retention_accepts_alternatives_in_parentheses_and_least(self):
        ok = ('f."UsedAt" IS NULL AND f."ExpiresAt" < @before AND (f."A" IS NULL OR f."A" < @before)',
              'f."UsedAt" IS NULL AND LEAST(f."ExpiresAt", f."UsedAt") < @before')
        for conds in ok:
            self.assertIsNone(edit(self.RETENTION, self.PURGE.replace('f."ExpiresAt" < @before', conds)), conds)

    def test_retention_checks_the_whole_file_on_a_partial_edit(self):
        final = self.PURGE + '\n-- name: Bad :execrows\n' + self.PURGE.split("\n", 1)[1].replace('"OidcFlows"', '"AuditEvents"')
        self.assertIn("temporary tables", edit(self.RETENTION, "LIMIT @batch", old="LIMIT @batch", exists=True, final=final))

    def test_comments_and_missing_spaces_do_not_hide_a_delete(self):
        module = "apps/server/internal/Modules/System/Queries/X.sql"
        for statement in ('DELETE /**/ FROM "Projects" WHERE "Id" = $1;', 'DELETE/* x */FROM "Projects";',
                          'DELETE -- c\nFROM "Projects";', 'DELETE FROM"Projects" WHERE "Id" = $1;', 'delete\n  from "Projects";'):
            self.assertIn("soft", edit(module, "-- name: D :exec\n" + statement + "\n"), statement)
            self.assertIsNotNone(edit(self.RETENTION, "-- name: D :exec\n" + statement.replace('"Projects"', '"AuditEvents"') + "\n"), statement)
        sneaky = self.PURGE.replace('DELETE FROM "OidcFlows"', 'DELETE /**/ FROM"AuditEvents"').replace('FROM "OidcFlows" f', 'FROM "AuditEvents" f')
        self.assertIn("temporary tables", edit(self.RETENTION, sneaky))

    def test_a_delete_inside_a_string_or_after_a_comment_marker_in_a_string_is_still_seen(self):
        module = "apps/server/internal/Modules/System/Queries/X.sql"
        self.assertIn("soft", edit(module, "-- name: D :exec\nSELECT '--'; DELETE FROM \"Projects\";\n"))
        self.assertIsNone(edit(module, "-- name: N :one\n-- DELETE FROM \"Projects\" is never allowed\nSELECT 1;\n"))

    def test_the_exception_is_only_for_the_retention_file(self):
        for path in ("apps/server/internal/Modules/Session/Queries/Retention.sql",
                     "apps/server/internal/Database/Retention/Queries/Other.sql",
                     "apps/server/internal/Core/RateLimit/Queries/RateLimit.sql"):
            self.assertIn("soft", edit(path, self.PURGE), path)

    def test_truncate_is_never_allowed_in_queries(self):
        self.assertIn("TRUNCATE", edit(self.RETENTION, '-- name: T :exec\nTRUNCATE "OidcFlows";\n'))
        self.assertIn("TRUNCATE", edit("apps/server/internal/Modules/System/Queries/X.sql", '-- name: T :exec\nTRUNCATE "Projects";\n'))


class OpenapiRules(unittest.TestCase):
    PATH = "packages/openapi/openapi.yaml"

    def test_kebab_case_paths_allowed(self):
        self.assertIsNone(edit(self.PATH, "paths:\n  /health/live:\n    get: {}\n  /projects/{ProjectId}/deploy-keys:\n    get: {}\n"))

    def test_non_kebab_segments_blocked(self):
        for path in ("/healthLive", "/deploy_keys", "/Projects", "/projects/deployKeys"):
            r = edit(self.PATH, "paths:\n  %s:\n    get: {}\n" % path)
            self.assertIn("kebab-case", r, path)

    def test_other_keys_are_not_treated_as_paths(self):
        self.assertIsNone(edit(self.PATH, "servers:\n  - url: /api/v1\ncomponents:\n  schemas:\n    PageMeta: {}\n"))


class BashRules(unittest.TestCase):
    def test_ai_attribution_blocked(self):
        cmd = 'git commit -m "feat: x\n\nCo-Authored-By: Claude <noreply@anthropic.com>"'
        self.assertIn("AI attribution", bash_rules.evaluate(cmd, "/"))

    def test_heredoc_attribution_blocked(self):
        cmd = "git commit -F - <<'EOF'\nfix: y\n\nGenerated with Claude Code\nEOF"
        self.assertIsNotNone(bash_rules.evaluate(cmd, "/"))

    def test_arabic_message_blocked(self):
        self.assertIn("English", bash_rules.evaluate('git commit -m "fix: تصليح"', "/"))

    def test_conventional_subject(self):
        self.assertIn("Conventional", bash_rules.evaluate('git commit -m "update stuff"', "/"))
        self.assertIsNone(bash_rules.evaluate('git commit -s -m "feat(server): add health"', "/"))

    def test_no_verify_blocked(self):
        self.assertIsNotNone(bash_rules.evaluate('git commit --no-verify -m "fix: a"', "/"))
        self.assertIsNotNone(bash_rules.evaluate('git commit -nm "fix: a"', "/"))

    def test_force_push_main_blocked(self):
        self.assertIsNotNone(bash_rules.evaluate("git push --force origin main", "/"))
        self.assertIsNotNone(bash_rules.evaluate("git push origin +main", "/"))
        self.assertIsNone(bash_rules.evaluate("git push --force-with-lease origin m1/web", "/"))
        self.assertIsNone(bash_rules.evaluate("git push -n origin main", "/"))

    def test_prose_not_flagged(self):
        self.assertIsNone(bash_rules.evaluate('echo "never add Co-Authored-By: Claude"', "/"))

    def test_pr_body_checked(self):
        cmd = 'gh pr create --title "feat: x" --body "Generated with Claude Code"'
        self.assertIsNotNone(bash_rules.evaluate(cmd, "/"))


class EndToEnd(unittest.TestCase):
    def run_hook(self, script, payload):
        out = subprocess.run([sys.executable, os.path.join(HOOKS, script)], input=json.dumps(payload),
                             capture_output=True, text=True, check=True)
        return json.loads(out.stdout) if out.stdout.strip() else None

    def test_edit_hook_denies(self):
        with tempfile.TemporaryDirectory() as root:
            res = self.run_hook("guard_edit.py", {
                "cwd": root,
                "tool_input": {"file_path": os.path.join(root, "internal/a/a.go"), "content": "package a\n// x\n"},
            })
        self.assertEqual(res["hookSpecificOutput"]["permissionDecision"], "deny")

    def test_edit_hook_allows(self):
        with tempfile.TemporaryDirectory() as root:
            res = self.run_hook("guard_edit.py", {
                "cwd": root,
                "tool_input": {"file_path": os.path.join(root, "internal/a/a.go"), "content": "package a\n"},
            })
        self.assertIsNone(res)

    def test_bash_hook_denies(self):
        res = self.run_hook("guard_bash.py", {"cwd": "/", "tool_input": {"command": 'git commit -m "wip"'}})
        self.assertEqual(res["hookSpecificOutput"]["permissionDecision"], "deny")

    def test_malformed_payload_is_ignored(self):
        out = subprocess.run([sys.executable, os.path.join(HOOKS, "guard_bash.py")], input="not json",
                             capture_output=True, text=True, check=True)
        self.assertEqual(out.stdout.strip(), "")


if __name__ == "__main__":
    unittest.main()
