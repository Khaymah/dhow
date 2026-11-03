import os
import re
import subprocess

from . import (arabic_rules, cli_rules, go_comments, js_comments, migration_rules, port_rules, runner_rules, web_rules,
               webhook_rules)
from .common import GENERATED_PATH, is_test, is_ts, new_lines

CSS_EXT = (".css",)
PROTO_DIRECTIVE = re.compile(r"^//\s*buf:lint:ignore\s")
MAX_LINES = 500

ROOT_MARKDOWN = {
    "README.md", "README.en.md", "CLAUDE.md", "CONTRIBUTING.md", "CONTRIBUTING.en.md", "SECURITY.md", "SECURITY.en.md",
    "LICENSE.md",
}
MARKDOWN_DIRS = ("docs/", "Plan/", ".github/", ".claude/")

SERVER_INTERNAL = "apps/server/internal/"
SERVER_IMPORT_PREFIX = "github.com/Khaymah/dhow/apps/server/internal/"
SERVER_TOP_LEVEL = ("App", "Config", "Core", "Database", "Modules")
SERVER_NESTED = ("Core", "Database", "Modules")
PASCAL_DIR = re.compile(r"^[A-Z][A-Za-z0-9]*$")
PASCAL_GO_FILE = re.compile(r"^[A-Z][A-Za-z0-9]*(\.[A-Z][A-Za-z0-9]*)*\.go$")
GO_IMPORT_PATH = re.compile(r'"' + re.escape(SERVER_IMPORT_PREFIX) + r'([^"]+)"')

ROUTES_FILE = re.compile(r"(^|/)Modules/[^/]+/[^/]+\.Routes\.go$")
GO_RAW_ROUTE = re.compile(
    r"\b\w+\.(Get|Post|Put|Patch|Delete|Head|Options|Connect|Trace|Handle|HandleFunc|Method|MethodFunc|Mount)\(\s*[\"`]")
GO_GENERATED_ROUTES = re.compile(r"\bgen\.(HandlerWithOptions|HandlerFromMux\w*|Handler)\(")
GO_POLICY_RULE = re.compile(r"\bpolicy\.(Public|Authenticated|Organization|Project|Instance|Runner|Webhook)\(")
GO_RUNNER_RULE = re.compile(r"\bpolicy\.Runner\(")
GO_CONNECT_HANDLER = re.compile(r"\b\w+connect\.New\w+Handler\(")
GO_RUNNER_GATE = re.compile(r"\.RequestGate\(")
CONNECT_ROUTE_FILES = ("Modules/Runners/Runners.Routes.go", "Modules/Jobs/Jobs.Routes.go", "Modules/Artifacts/Artifacts.Routes.go")
RUNNER_AUTH_IMPORTERS = (("App", None), ("Modules", "Runners"), ("Modules", "Jobs"), ("Modules", "Artifacts"), ("Core", "RunnerAuth"))
GO_POLICY_TABLE = re.compile(r"\bpolicy\.NewTable\(")
GO_ROUTE_MIDDLEWARES = re.compile(r"\bMiddlewares\s*:")

MIGRATION_SQL = re.compile(r"(^|/)migrations/[^/]+\.sql$", re.IGNORECASE)
MODULE_QUERY_SQL = re.compile(r"(^|/)(Modules|Core|Database)/[^/]+/Queries/[^/]+\.sql$", re.IGNORECASE)
RETENTION_SQL = "apps/server/internal/Database/Retention/Queries/Retention.sql"
RETENTION_TABLES = (
    '"LoginThrottles"', '"RunnerThrottles"', '"AuthChallenges"', '"OidcFlows"', '"Invitations"', '"SetupTokens"',
    '"RateLimits"',
)
SQL_ANY_DELETE = re.compile(r"(?i)\bdelete\b\s*from\b")
SQL_TRUNCATE = re.compile(r"(?i)\btruncate\b")
RETENTION_TEMPLATE = re.compile(
    r'(?is)^delete from ("[A-Za-z]+") where "Id" in \(select ([a-z]+)\."Id" from ("[A-Za-z]+") \2 '
    r'where (?P<conds>.+?) limit @batch for update skip locked\)$')
RETENTION_TIME_BOUND = re.compile(r'(?i)^(?:least\([a-z]+\."[A-Za-z]+"(?:, ?[a-z]+\."[A-Za-z]+")+\)|[a-z]+\."[A-Za-z]+") < @before$')
BASE_COLUMN_TABLE_ALLOWLIST = ('"Instance"',)
BASE_COLUMNS = (
    (re.compile(r'(?i)"Id"\s+uuid\s+PRIMARY\s+KEY\s+DEFAULT\s+uuidv7\(\)'), '"Id" uuid PRIMARY KEY DEFAULT uuidv7()'),
    (re.compile(r'(?i)"CreatedAt"\s+timestamptz\s+NOT\s+NULL\s+DEFAULT\s+now\(\)'), '"CreatedAt" timestamptz NOT NULL DEFAULT now()'),
    (re.compile(r'(?i)"UpdatedAt"\s+timestamptz\s+NOT\s+NULL\s+DEFAULT\s+now\(\)'), '"UpdatedAt" timestamptz NOT NULL DEFAULT now()'),
    (re.compile(r'(?i)"IsDeleted"\s+boolean\s+NOT\s+NULL\s+DEFAULT\s+false'), '"IsDeleted" boolean NOT NULL DEFAULT false'),
)
SQL_TABLE_HEAD = re.compile(r"(?i)\bcreate\s+table\s+(?:if\s+not\s+exists\s+)?([^\s(]+)\s*\(")
PARTITIONED_ID = re.compile(r'(?i)"Id"\s+uuid\s+NOT\s+NULL\s+DEFAULT\s+uuidv7\(\)')
PARTITIONED_KEY = re.compile(r'(?i)\bPRIMARY\s+KEY\s*\(\s*"Id"\s*,\s*"[A-Z][A-Za-z0-9]*"')
PARTITION_BY = re.compile(r"(?i)^\s*PARTITION\s+BY\b")
FORMAT_IDENTIFIER = re.compile(r"^(%I\.)?%I$")
LIKE_COPY = re.compile(r"(?i)^\s*LIKE\s+(%s|%I|\"[A-Z][A-Za-z0-9]*\")(\s+INCLUDING\s+ALL)?\s*$")
SQL_TIMESTAMP_NO_TZ = re.compile(r"(?i)\btimestamp\b(?!\s*(?:\(\d+\))?\s*with\s+time\s+zone)")
SQL_VARCHAR = re.compile(r"(?i)\b(varchar|character\s+varying)\b")
SQL_TRANSLATED_COLUMN = re.compile(r'(?m)^\s*"([A-Z][A-Za-z0-9]*[a-z0-9](?:Ar|En))"\s+[A-Za-z]')
OPENAPI_PATH_KEY = re.compile(r"""^\s*["']?(/[^\s:"']*)["']?:\s*$""")
KEBAB_SEGMENT = re.compile(r"^[a-z0-9]+(-[a-z0-9]+)*$")
PATH_PARAM_SEGMENT = re.compile(r"^\{[A-Za-z][A-Za-z0-9_]*\}$")

SECRET_PATTERNS = (
    (re.compile(r"-----BEGIN (RSA |EC |OPENSSH |DSA |ENCRYPTED )?PRIVATE KEY-----"), "a private key"),
    (re.compile(r"\bAKIA[0-9A-Z]{16}\b"), "an AWS access key"),
    (re.compile(r"\bgh[pousr]_[A-Za-z0-9]{36,}\b"), "a GitHub token"),
    (re.compile(r"\bglpat-[A-Za-z0-9_-]{20,}\b"), "a GitLab token"),
    (re.compile(r"\bxox[abprs]-[A-Za-z0-9-]{10,}\b"), "a Slack token"),
    (re.compile(r"\bsk-(live|test|proj|ant)-[A-Za-z0-9_-]{16,}\b"), "an API secret key"),
)

GO_SHELL = re.compile(
    r"exec\.Command(Context)?\(\s*(\w+\s*,\s*)?\"(/bin/|/usr/bin/)?(sh|bash|zsh|dash|ash)\"\s*,\s*\"-c\"")
GO_JSON_TAG = re.compile(r"json:\"([^\",]*)")
GO_PANIC = re.compile(r"(^|[^\w.])panic\(")
GO_SQL_FMT = re.compile(
    r"fmt\.Sprintf\(\s*[\"`]\s*(?i:select|insert|update|delete|with|create|alter|drop)\b")
GO_SQL_CONCAT = re.compile(
    r"[\"`]\s*(?i:select|insert\s+into|update|delete\s+from)\b[^\"`]*[\"`]\s*\+")
GO_ENV = re.compile(r"\bos\.(Getenv|LookupEnv|Environ)\(")
GO_CONFIG_DIR = re.compile(r"(^|/)internal/[Cc]onfig/")
GO_PRINT = re.compile(r"\b(fmt\.Print(ln|f)?|log\.(Print|Fatal|Panic)(ln|f)?)\(")

TS_ANY = re.compile(r"(:\s*any\b|\bas\s+any\b|<any>|\bany\[\]|Array<any>|Record<[^>]*\bany\b)")
TS_CONSOLE = re.compile(r"\bconsole\.(log|debug|info|warn|error|trace)\(")
TS_EVAL = re.compile(r"(\beval\(|\bnew\s+Function\()")
TS_INNER_HTML = re.compile(r"dangerouslySetInnerHTML|\.innerHTML\s*=")
TS_INTERFACE = re.compile(r"\binterface\s+([A-Za-z_]\w*)")
TS_TYPE = re.compile(r"(?m)^\s*(?:export\s+)?(?:declare\s+)?type\s+([A-Za-z_]\w*)\s*(?:<[^=]*>)?\s*=")
TS_ENUM = re.compile(r"\benum\s+([A-Za-z_]\w*)")
TS_FUNCTION_COMPONENT = re.compile(r"(?m)^\s*(?:export\s+(?:default\s+)?)?function\s+([A-Z]\w*)\s*\(")

PHYSICAL_CLASS = re.compile(
    r"(?<![\w-])-?(ml|mr|pl|pr|left|right|border-l|border-r|rounded-l|rounded-r|rounded-tl|"
    r"rounded-tr|rounded-bl|rounded-br|scroll-ml|scroll-mr|scroll-pl|scroll-pr)-[\w\[./]+|"
    r"(?<![\w-])(text-left|text-right|float-left|float-right|clear-left|clear-right)(?![\w-])")
PHYSICAL_CSS = re.compile(
    r"(?<![\w-])(margin|padding|border|scroll-margin|scroll-padding)-(left|right)\s*:|"
    r"(?<![\w-])(left|right)\s*:\s*[^;{]|text-align\s*:\s*(left|right)|float\s*:\s*(left|right)")
PHYSICAL_STYLE = re.compile(
    r"\b(marginLeft|marginRight|paddingLeft|paddingRight|borderLeft\w*|borderRight\w*|"
    r"left|right)\s*:\s*[^,}\n]")

SQL_CREATE_TABLE = re.compile(r"(?i)\bcreate\s+table\s+(?:if\s+not\s+exists\s+)?([^\s(]+)")
SQL_COLUMN = re.compile(
    r"(?im)^\s*([A-Za-z_][\w]*)\s+(uuid|text|varchar|char|int|integer|bigint|smallint|boolean|bool|"
    r"timestamptz|timestamp|date|jsonb|json|bytea|numeric|real|double|serial|bigserial)\b")
QUOTED_PASCAL = re.compile(r'^(?:"[A-Z][A-Za-z0-9]*"\.)?"[A-Z][A-Za-z0-9]*"$')


def is_go(path):
    return path.endswith(".go")


def check_generated(rel, **_):
    if GENERATED_PATH.search(rel):
        return (f"{rel} is generated or a lockfile. Do not edit it by hand: change the source "
                "(packages/openapi/openapi.yaml, packages/proto/**/*.proto, SQL queries, go.mod, package.json, apps/web/Src/Styles/Tokens.json) and run "
                "`make gen`, `go mod tidy`, `pnpm install`, `pnpm gen:api` or `pnpm gen:tokens`.")
    return None


def check_secrets(rel, added, **_):
    for pattern, what in SECRET_PATTERNS:
        if pattern.search(added):
            return f"This edit writes {what} into {rel}. Secrets never go into the repository."
    return None


OLD_ORG_NAME = re.compile(r"(?i)arab[-_ ]?open[-_ ]?source")


def check_org_name(rel, added, **_):
    if OLD_ORG_NAME.search(added):
        return (f"{rel}: the project lives at github.com/Khaymah/dhow (module github.com/Khaymah/dhow, "
                "image ghcr.io/khaymah/dhow). The old organization name is never written again.")
    return None


def check_env_file(rel, **_):
    base = os.path.basename(rel)
    if base.startswith(".env") and base != ".env.example":
        return f"{rel}: environment files with real values are never written to the repository. Use .env.example with placeholders."
    return None


def check_markdown_location(rel, exists, **_):
    if not rel.endswith(".md") or exists:
        return None
    if rel in ROOT_MARKDOWN or rel.startswith(MARKDOWN_DIRS):
        return None
    return (f"{rel}: documentation lives in docs/ (public, Arabic, with English in docs/en/) or Plan/ (internal, Arabic). "
            "Do not create Markdown files elsewhere.")


def proto_comment_allowed(kind, text):
    return (kind == "line" and PROTO_DIRECTIVE.match(text) is not None) or go_comments.is_allowed(kind, text)


def check_comments(rel, added, old, **_):
    if is_go(rel):
        find, allowed = go_comments.find_comments, go_comments.is_allowed
    elif is_ts(rel):
        find, allowed = js_comments.find_comments, js_comments.is_allowed
    elif rel.endswith(".proto"):
        find, allowed = go_comments.find_comments, proto_comment_allowed
    else:
        return None
    for start, end, kind in find(added):
        text = added[start:end]
        if allowed(kind, text):
            continue
        stripped = text.strip()
        if stripped and stripped in old:
            continue
        snippet = " ".join(stripped.split())[:90]
        return (f"{rel}: code carries no comments (no line comments, block comments, JSDoc or Go doc comments). "
                f"Offending comment: {snippet}. Rationale belongs in Plan/ADR. The only exceptions are build "
                "directives (//go:, //nolint:, eslint-, ts-expect-error, buf:lint:ignore, ...) and a comment that starts with `IMPORTANT:`.")
    return None


def check_go(rel, added, old, **_):
    if not is_go(rel):
        return None
    lines = new_lines(added, old)
    text = "\n".join(lines)
    if GO_SHELL.search(text):
        return (f"{rel}: never run a shell with a command string (`sh -c`). Build an argv slice: "
                "exec.CommandContext(ctx, \"git\", \"checkout\", \"--\", ref).")
    for m in GO_JSON_TAG.finditer(text):
        name = m.group(1)
        if name and name != "-" and not name[0].isupper():
            return f"{rel}: JSON field names are PascalCase (`json:\"{name[:1].upper()}{name[1:]}\"`), not `{name}`."
    if GO_SQL_FMT.search(text) or GO_SQL_CONCAT.search(text):
        return f"{rel}: SQL is never built with strings. Write the query in a .sql file and use the sqlc-generated function."
    if is_test(rel):
        return None
    if GO_PANIC.search(text):
        return f"{rel}: no panic in production code. Return a wrapped error (`fmt.Errorf(\"...: %w\", err)`)."
    if GO_PRINT.search(text) and "/cmd/" not in rel:
        return f"{rel}: use the structured logger (log/slog), not fmt.Print or log.Print."
    if GO_ENV.search(text) and not GO_CONFIG_DIR.search(rel):
        return f"{rel}: read environment variables only in the config package, which validates them and fails closed."
    return None


def server_area(path):
    parts = path.split("/")
    if parts[0] in SERVER_NESTED and len(parts) > 1:
        return (parts[0], parts[1])
    return (parts[0], None)


def check_server_layout(rel, **_):
    if not is_go(rel) or not rel.startswith(SERVER_INTERNAL) or GENERATED_PATH.search(rel):
        return None
    parts = rel[len(SERVER_INTERNAL):].split("/")
    layers = "App, Config, Core/<Name>, Database/<Name> or Modules/<Module>/..."
    if len(parts) < 2 or parts[0] not in SERVER_TOP_LEVEL:
        return (f"{rel}: Go code under apps/server/internal must live in {layers}. "
                "Put cross-cutting code in Core, wiring in App, and feature code in a module.")
    if parts[0] in SERVER_NESTED and len(parts) < 3:
        return f"{rel}: files under {parts[0]}/ sit in a named subfolder ({parts[0]}/<Name>/File.go), not directly in {parts[0]}/."
    for folder in parts[:-1]:
        if not PASCAL_DIR.match(folder):
            return f"{rel}: folder `{folder}` must be PascalCase (Modules/System/Test/Unit)."
    name = parts[-1]
    if not name.endswith("_test.go") and not PASCAL_GO_FILE.match(name):
        return f"{rel}: Go file names are PascalCase with dot roles (System.Service.go, Instance.Model.go), not `{name}`."
    return None


def runner_auth_violation(src, dst):
    if {src, dst} == {("Core", "RunnerAuth"), ("Core", "Auth")}:
        return ("Core/Auth and Core/RunnerAuth must not import each other. Browser sessions and runner sessions are "
                "separate planes, so a credential of one never works on the other.")
    if dst == ("Core", "RunnerAuth") and src not in RUNNER_AUTH_IMPORTERS:
        return ("only App, Modules/Runners, Modules/Jobs and Modules/Artifacts import Core/RunnerAuth. The runner plane and its "
                "principal stay out of every other layer.")
    return None


def import_violation(src, dst):
    reason = runner_auth_violation(src, dst)
    if reason:
        return reason
    s_top, s_name = src
    d_top, d_name = dst
    if d_top == "Modules":
        if s_top == "App":
            return None
        if s_top == "Modules":
            if s_name == d_name:
                return None
            return (f"Modules/{s_name} must not import Modules/{d_name}. Modules talk to each other through "
                    "Core/EventBus events, never through direct imports.")
        return f"{s_top} must not import Modules. Only App imports modules."
    if d_top == "App" and s_top != "App":
        return f"{s_top} must not import App. App is the composition root and depends on everything else."
    if d_top == "Core" and s_top in ("Config", "Database"):
        return f"{s_top} must not import Core. Allowed direction: App > Modules > Core > (Config, Database)."
    return None


def check_server_imports(rel, added, old, **_):
    if not is_go(rel) or not rel.startswith(SERVER_INTERNAL) or is_test(rel) or GENERATED_PATH.search(rel):
        return None
    if "/Test/" in rel:
        return None
    parts = rel[len(SERVER_INTERNAL):].split("/")
    if parts[0] not in SERVER_TOP_LEVEL:
        return None
    src = server_area("/".join(parts))
    for line in new_lines(added, old):
        for m in GO_IMPORT_PATH.finditer(line):
            target = m.group(1).split("/")
            if target[0] not in SERVER_TOP_LEVEL:
                continue
            reason = import_violation(src, server_area("/".join(target)))
            if reason:
                return f"{rel}: forbidden import of internal/{m.group(1)}. {reason}"
    return None


def check_route_policies(rel, added, old, final_content, **_):
    if not ROUTES_FILE.search(rel) or is_test(rel):
        return None
    m = GO_RAW_ROUTE.search("\n".join(new_lines(added, old)))
    if m:
        return (f"{rel}: do not register routes by hand (`{m.group(0).strip()}`). Add the operation to "
                "packages/openapi/openapi.yaml, run `make gen`, and declare its policy in Policies() with "
                "policy.Public, policy.Authenticated, policy.Organization, policy.Project or policy.Instance.")
    text = final_content if final_content is not None else added
    if GO_GENERATED_ROUTES.search(text) and not (
            GO_POLICY_RULE.search(text) and GO_POLICY_TABLE.search(text) and GO_ROUTE_MIDDLEWARES.search(text)):
        return (f"{rel}: every route needs a declared policy. Declare each route in Policies() with policy.Public, "
                "policy.Authenticated, policy.Organization, policy.Project or policy.Instance, build policy.NewTable(Policies()...) and pass "
                "guard.Middleware(table) in the generated ChiServerOptions.Middlewares.")
    return None


def check_connect_mounts(rel, added, old, final_content, **_):
    if not is_go(rel) or not rel.startswith(SERVER_INTERNAL) or is_test(rel) or GENERATED_PATH.search(rel):
        return None
    if "/Test/" in rel:
        return None
    text = final_content if final_content is not None else added
    if not GO_CONNECT_HANDLER.search(text):
        return None
    inner = rel[len(SERVER_INTERNAL):]
    if inner not in CONNECT_ROUTE_FILES:
        return (f"{rel}: Connect handlers of the runner plane are mounted only in "
                "Modules/Runners/Runners.Routes.go, Modules/Jobs/Jobs.Routes.go and Modules/Artifacts/Artifacts.Routes.go, "
                "next to their policy rows.")
    if not (GO_RUNNER_RULE.search(text) and GO_POLICY_TABLE.search(text) and GO_RUNNER_GATE.search(text)):
        return (f"{rel}: every runner procedure needs a declared policy. Declare each procedure with policy.Runner, "
                "build policy.NewTable(...) from them and pass gate.RequestGate(table) to every Connect handler.")
    return None


def check_ts(rel, added, old, **_):
    if not is_ts(rel):
        return None
    text = "\n".join(new_lines(added, old))
    if TS_ANY.search(text):
        return f"{rel}: `any` is not allowed. Use a precise type, generics or `unknown` with a type guard."
    if TS_EVAL.search(text):
        return f"{rel}: eval and new Function are never allowed."
    if TS_INNER_HTML.search(text):
        return f"{rel}: do not inject HTML (dangerouslySetInnerHTML / innerHTML). Render React elements."
    if not is_test(rel) and TS_CONSOLE.search(text):
        return f"{rel}: no console.* in application code."
    for m in TS_INTERFACE.finditer(text):
        if not re.match(r"^I[A-Z]", m.group(1)):
            return f"{rel}: interface names start with `I` (`I{m.group(1)}`)."
    for m in TS_TYPE.finditer(text):
        if not re.match(r"^T[A-Z]", m.group(1)):
            return f"{rel}: type alias names start with `T` (`T{m.group(1)}`)."
    for m in TS_ENUM.finditer(text):
        if not re.match(r"^E[A-Z]", m.group(1)):
            return f"{rel}: enum names start with `E` (`E{m.group(1)}`)."
    if rel.endswith((".tsx", ".jsx")):
        m = TS_FUNCTION_COMPONENT.search(text)
        if m:
            return f"{rel}: components are arrow functions (`export const {m.group(1)} = (...) => ...`)."
    return None


def check_direction(rel, added, old, **_):
    if not rel.startswith("apps/web/"):
        return None
    if not (is_ts(rel) or rel.endswith(CSS_EXT)):
        return None
    for line in new_lines(added, old):
        if "IMPORTANT:" in line:
            continue
        hit = PHYSICAL_CLASS.search(line) if is_ts(rel) else None
        if rel.endswith(CSS_EXT):
            hit = PHYSICAL_CSS.search(line)
        elif hit is None and "style" in line:
            hit = PHYSICAL_STYLE.search(line)
        if hit:
            return (f"{rel}: the UI is RTL-first, so use logical properties only "
                    f"(ms-/me-/ps-/pe-/start-/end-/text-start/text-end, margin-inline-*, inset-inline-*). "
                    f"Offending: `{hit.group(0).strip()}`. A deliberate exception needs an `IMPORTANT:` comment on the line.")
    return None


def check_sql(rel, added, old, **_):
    if not rel.endswith(".sql"):
        return None
    text = "\n".join(new_lines(added, old))
    for m in SQL_CREATE_TABLE.finditer(text):
        if not QUOTED_PASCAL.match(m.group(1)) and not FORMAT_IDENTIFIER.match(m.group(1)):
            return f"{rel}: table names are quoted PascalCase (\"Projects\"), not {m.group(1)}."
    for m in SQL_COLUMN.finditer(text):
        return f"{rel}: column names are quoted PascalCase (\"CreatedAt\"), not {m.group(1)}."
    return None


def table_bodies(text):
    for m in SQL_TABLE_HEAD.finditer(text):
        depth, i = 1, m.end()
        while i < len(text) and depth:
            depth += {"(": 1, ")": -1}.get(text[i], 0)
            i += 1
        end = text.find(";", i)
        trailer = text[i:] if end < 0 else text[i:end]
        yield m.group(1), text[m.start():i], text[m.end():i], trailer


def partitioned_id(body, trailer):
    return PARTITION_BY.search(trailer) is not None and PARTITIONED_ID.search(body) is not None and \
        PARTITIONED_KEY.search(body) is not None


def check_migration_rules(rel, added, old, **_):
    if not MIGRATION_SQL.search(rel):
        return None
    for name, statement, body, trailer in table_bodies(added):
        if name in BASE_COLUMN_TABLE_ALLOWLIST or statement in old:
            continue
        if FORMAT_IDENTIFIER.match(name) and LIKE_COPY.match(body.rstrip(")")):
            continue
        for index, (pattern, wanted) in enumerate(BASE_COLUMNS):
            if index == 0 and partitioned_id(body, trailer):
                continue
            if not pattern.search(body):
                return (f"{rel}: table {name} is missing the base column `{wanted}`. Every table has \"Id\", "
                        "\"CreatedAt\", \"UpdatedAt\" and \"IsDeleted\" (only \"Instance\" is exempt). A table with "
                        "PARTITION BY may instead declare `\"Id\" uuid NOT NULL DEFAULT uuidv7()` and a composite "
                        "`PRIMARY KEY (\"Id\", \"<PartitionColumn>\")`, because Postgres needs the partition key in the primary key.")
    text = "\n".join(new_lines(added, old))
    if SQL_TIMESTAMP_NO_TZ.search(text):
        return f"{rel}: use timestamptz (UTC), never timestamp without time zone."
    if SQL_VARCHAR.search(text):
        return f"{rel}: use TEXT, never VARCHAR."
    m = SQL_TRANSLATED_COLUMN.search(text)
    if m:
        return (f"{rel}: column {m.group(1)} looks like a translated column. Put translations in a separate "
                "<Entity>Translations table, never in NameAr/NameEn style columns.")
    return None


def strip_sql_comments(text):
    out, i, n = [], 0, len(text)
    while i < n:
        c = text[i]
        if text.startswith("--", i):
            j = text.find("\n", i)
            i = n if j < 0 else j
            out.append(" ")
        elif text.startswith("/*", i):
            j = text.find("*/", i + 2)
            i = n if j < 0 else j + 2
            out.append(" ")
        elif c in "'\"":
            j = i + 1
            while j < n:
                if text[j] == c and text.startswith(c * 2, j):
                    j += 2
                    continue
                if text[j] == c:
                    break
                j += 1
            out.append(text[i:j + 1])
            i = j + 1
        else:
            out.append(c)
            i += 1
    return "".join(out)


def top_level_split(conds, word):
    parts, depth, start = [], 0, 0
    pattern = re.compile(r"(?i)\b" + word + r"\b")
    i = 0
    while i < len(conds):
        ch = conds[i]
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
        elif depth == 0:
            m = pattern.match(conds, i)
            if m and (i == 0 or not conds[i - 1].isalnum()):
                parts.append(conds[start:i].strip())
                start = i = m.end()
                continue
        i += 1
    parts.append(conds[start:].strip())
    return parts


def retention_statement_violation(rel, statement):
    flat = re.sub(r"\s+", " ", statement).strip()
    flat = re.sub(r"\s+\)", ")", re.sub(r"\(\s+", "(", flat))
    flat = re.sub(r'(?i)\b(from|delete)"', r'\1 "', flat)
    m = RETENTION_TEMPLATE.match(flat)
    if not m:
        return (f"{rel}: a retention DELETE must be exactly `DELETE FROM \"T\" WHERE \"Id\" IN (SELECT x.\"Id\" FROM \"T\" x "
                "WHERE ... LIMIT @batch FOR UPDATE SKIP LOCKED)`, so it deletes in batches and never touches a row in use.")
    outer, inner, conds = m.group(1), m.group(3), m.group("conds")
    if outer != inner:
        return f"{rel}: a retention DELETE selects its rows from the same table it deletes from ({outer} is not {inner})."
    if outer not in RETENTION_TABLES:
        return (f"{rel}: retention may only DELETE from the temporary tables {', '.join(RETENTION_TABLES)}, "
                f"not {outer}. Rows with history (audit, memberships, sessions) are soft-deleted only.")
    if len(top_level_split(conds, "or")) > 1:
        return f"{rel}: a retention DELETE on {outer} may not use OR at the top level of its WHERE: put alternatives in parentheses."
    if not any(RETENTION_TIME_BOUND.match(c) for c in top_level_split(conds, "and")):
        return (f"{rel}: every retention DELETE on {outer} needs a WHERE bounded by a fixed period "
                "(an `AND`ed `x.\"Column\" < @before`), so it never removes live rows.")
    return None


def retention_violation(rel, text):
    for statement in text.split(";"):
        if SQL_ANY_DELETE.search(statement):
            reason = retention_statement_violation(rel, statement)
            if reason:
                return reason
    return None


def check_module_queries(rel, added, old, final_content=None, **_):
    if not MODULE_QUERY_SQL.search(rel):
        return None
    whole = strip_sql_comments(final_content if final_content is not None else added)
    text = strip_sql_comments("\n".join(new_lines(added, old))) + "\n" + whole
    if SQL_TRUNCATE.search(text):
        return f"{rel}: no TRUNCATE in queries."
    if rel == RETENTION_SQL:
        return retention_violation(rel, whole)
    if SQL_ANY_DELETE.search(text):
        return (f"{rel}: no DELETE FROM in queries. Deletion is soft: UPDATE ... SET \"IsDeleted\" = true, \"UpdatedAt\" = now(). "
                f"The only exception is {RETENTION_SQL}, for expired rows of temporary tables.")
    return None


def check_openapi_paths(rel, added, old, **_):
    if not rel.endswith("openapi.yaml"):
        return None
    for line in new_lines(added, old):
        m = OPENAPI_PATH_KEY.match(line)
        if not m:
            continue
        for segment in m.group(1).split("/")[1:]:
            if segment and not KEBAB_SEGMENT.match(segment) and not PATH_PARAM_SEGMENT.match(segment):
                return (f"{rel}: API path segments are kebab-case (`/health-checks`), not `{segment}` in {m.group(1)}. "
                        "Path parameters are written {Name}.")
    return None


def migration_on_main(root, rel):
    if not re.search(r"(^|/)migrations/[^/]+\.sql$", rel):
        return False
    for ref in ("origin/main", "main"):
        try:
            r = subprocess.run(["git", "-C", root, "cat-file", "-e", f"{ref}:{rel}"],
                               capture_output=True, timeout=5, check=False)
        except (OSError, subprocess.SubprocessError):
            return False
        if r.returncode == 0:
            return True
    return False


def check_migration_immutable(rel, root, exists, **_):
    if exists and migration_on_main(root, rel):
        return f"{rel} is already on main. Never edit a merged migration: add a new one."
    return None


def check_length(rel, final_content, **_):
    if final_content is None or GENERATED_PATH.search(rel):
        return None
    if not (is_go(rel) or is_ts(rel)):
        return None
    count = final_content.count("\n") + (0 if final_content.endswith("\n") or not final_content else 1)
    if count > MAX_LINES:
        return f"{rel} would be {count} lines. Files stay at or under {MAX_LINES} lines: split by responsibility."
    return None


CHECKS = (
    check_generated,
    web_rules.check_web_layout,
    check_secrets,
    check_org_name,
    check_env_file,
    check_markdown_location,
    check_migration_immutable,
    check_comments,
    check_go,
    check_server_layout,
    check_server_imports,
    port_rules.check_port_imports,
    port_rules.check_port_contents,
    check_route_policies,
    webhook_rules.check_webhook_rule_location,
    webhook_rules.check_webhook_routes,
    check_connect_mounts,
    runner_rules.check_runner_layout,
    runner_rules.check_runner_imports,
    runner_rules.check_shared_imports,
    cli_rules.check_cli_layout,
    cli_rules.check_cli_imports,
    cli_rules.check_cli_process_calls,
    check_ts,
    web_rules.check_web_imports,
    web_rules.check_web_text,
    arabic_rules.check_arabic_register,
    check_direction,
    check_sql,
    check_migration_rules,
    migration_rules.check_migration_compat,
    check_module_queries,
    check_openapi_paths,
    check_length,
)


def evaluate(rel, root, segments, exists, final_content):
    for added, old in segments:
        for check in CHECKS:
            reason = check(rel=rel, root=root, added=added, old=old, exists=exists, final_content=final_content)
            if reason:
                return reason
    if not segments:
        return check_generated(rel=rel)
    return None
