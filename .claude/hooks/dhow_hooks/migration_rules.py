import re
import subprocess

MIGRATION_FILE = re.compile(r"(?:^|/)apps/server/internal/Database/Migrations/(\d+)_[^/]+\.sql$")
GRANDFATHERED_THROUGH = 48
ADR = "Plan/ADR/0077-upgrades-migrations-and-update-policy.md"
FLAGS = re.IGNORECASE | re.ASCII

IDENT = r'(?:"(?:[^"]|"")+"|[A-Za-z_][A-Za-z0-9_$]*)'
NAME = IDENT + r"(?:\s*\.\s*" + IDENT + r")?"

PHASE = re.compile(r"^--\s*dhow:phase\s+(\S+)\s*$", FLAGS)
FLOOR = re.compile(r"^--\s*dhow:floor\s+(\S+)\s*$", FLAGS)
BACKFILL = re.compile(r"^--\s*dhow:backfill\s+small\s*$", FLAGS)
SEMVER = re.compile(r"^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$")
NO_TX = re.compile(r"^--\s*\+goose\s+NO\s+TRANSACTION\s*$", FLAGS)
GOOSE_UP = re.compile(r"^--\s*\+goose\s+Up\b", FLAGS)
GOOSE_DOWN = re.compile(r"^--\s*\+goose\s+Down\b", FLAGS)
BEGIN = re.compile(r"^--\s*\+goose\s+StatementBegin\b", FLAGS)
END = re.compile(r"^--\s*\+goose\s+StatementEnd\b", FLAGS)
DOLLAR_TAG = re.compile(r"\$(?:[A-Za-z_][A-Za-z0-9_]*)?\$")

NAME_RE = re.compile(NAME, FLAGS)
CREATE_TABLE = re.compile(r"^CREATE\s+(?:(?:GLOBAL|LOCAL)\s+)?(?:(?:TEMP|TEMPORARY|UNLOGGED)\s+)?TABLE\s+"
                          r"(?:IF\s+NOT\s+EXISTS\s+)?(" + NAME + ")", FLAGS)
CREATE_ROUTINE = re.compile(r"^CREATE\s+(FUNCTION|TYPE)\s+(" + NAME + ")", FLAGS)
DROP_TABLE = re.compile(r"^DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?(.*)$", FLAGS)
DROP_ROUTINE = re.compile(r"^DROP\s+(FUNCTION|TYPE)\s+(?:IF\s+EXISTS\s+)?(" + NAME + ")", FLAGS)
TRUNCATE = re.compile(r"^TRUNCATE\s+(?:TABLE\s+)?(.*)$", FLAGS)
DELETE = re.compile(r"\bDELETE\s+FROM\s+(?:ONLY\s+)?(" + NAME + ")", FLAGS)
ALTER_TABLE = re.compile(r"^ALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:ONLY\s+)?(" + NAME + r")\s*(.*)$", FLAGS)
ALTER_OTHER = re.compile(r"^ALTER\s+", FLAGS)
RENAME = re.compile(r"\bRENAME\b", FLAGS)
ACTION_RENAME = re.compile(r"^RENAME\b", FLAGS)
ACTION_DROP = re.compile(r"^DROP\s+", FLAGS)
ACTION_DROP_CONSTRAINT = re.compile(r"^DROP\s+CONSTRAINT\b", FLAGS)
ACTION_TYPE = re.compile(r"^ALTER\s+(?:COLUMN\s+)?" + IDENT + r"\s+(?:SET\s+DATA\s+)?TYPE\b", FLAGS)
ACTION_SET_NOT_NULL = re.compile(r"^ALTER\s+(?:COLUMN\s+)?" + IDENT + r"\s+SET\s+NOT\s+NULL\b", FLAGS)
ADD_CONSTRAINT = re.compile(r"^ADD\s+(?:CONSTRAINT\b|CHECK\b|FOREIGN\s+KEY\b|UNIQUE\b|PRIMARY\s+KEY\b|EXCLUDE\b)", FLAGS)
ADD_CHECK_OR_FK = re.compile(r"^ADD\s+(?:CONSTRAINT\s+" + IDENT + r"\s+)?(?:CHECK|FOREIGN\s+KEY)\b", FLAGS)
ADD_COLUMN = re.compile(r"^ADD\s+(?:COLUMN\s+)?(?:IF\s+NOT\s+EXISTS\s+)?(" + IDENT + r")\s*(.*)$", FLAGS)
NOT_NULL = re.compile(r"\bNOT\s+NULL\b", FLAGS)
HAS_VALUE = re.compile(r"\bDEFAULT\b|\bGENERATED\b", FLAGS)
NOT_VALID = re.compile(r"\bNOT\s+VALID\b", FLAGS)
CREATE_INDEX = re.compile(r"^CREATE\s+(?:UNIQUE\s+)?INDEX\s+(CONCURRENTLY\s+)?(IF\s+NOT\s+EXISTS\s+)?(?:" + NAME +
                          r"\s+)?ON\s+(?:ONLY\s+)?(" + NAME + ")", FLAGS)
UPDATE = re.compile(r"\bUPDATE\s+(?:ONLY\s+)?(" + NAME + r")(?:\s+(?:AS\s+)?" + IDENT + r")?\s+SET\b", FLAGS)
IDEMPOTENT = re.compile(r"\bIF\s+NOT\s+EXISTS\b|\bIF\s+EXISTS\b|\bVALIDATE\b", FLAGS)
FLOOR_UPDATE = re.compile(r"\bUPDATE\s+\"Instance\"\s+SET\s+\"SchemaFloor\"\s*=\s*'([^']*)'", FLAGS)
TX_CONTROL = re.compile(r"^(?:BEGIN|START\s+TRANSACTION|COMMIT|END|ROLLBACK|ABORT|SAVEPOINT|RELEASE|"
                        r"PREPARE\s+TRANSACTION)\b", FLAGS)
ATOMIC_BODY = re.compile(r"^CREATE\s+(?:OR\s+REPLACE\s+)?(?:FUNCTION|PROCEDURE)\b.*\bBEGIN\s+ATOMIC\b", FLAGS)
LIST_KEYWORDS = {"only", "cascade", "restrict", "restart", "continue", "identity"}


def is_ident_char(c):
    return c.isascii() and (c.isalnum() or c in "_$")


def blank(out, start, end, fill=" "):
    for k in range(start, end):
        if out[k] != "\n":
            out[k] = fill


def string_end(text, i, escaped):
    n = len(text)
    while i < n:
        if escaped and text[i] == "\\":
            i += 2
        elif text[i] == "'" and text.startswith("''", i):
            i += 2
        elif text[i] == "'":
            return i + 1
        else:
            i += 1
    return n


def identifier_end(text, i):
    n = len(text)
    while i < n:
        if text.startswith('""', i):
            i += 2
        elif text[i] == '"':
            return i + 1
        else:
            i += 1
    return n


def mask(text):
    out, i, n = list(text), 0, len(text)
    while i < n:
        c = text[i]
        if text.startswith("--", i):
            j = text.find("\n", i)
            j = n if j < 0 else j
            blank(out, i, j)
            i = j
        elif text.startswith("/*", i):
            j = text.find("*/", i + 2)
            j = n if j < 0 else j + 2
            blank(out, i, j)
            i = j
        elif c == "'":
            escaped = i > 0 and text[i - 1] in "eE" and (i < 2 or not is_ident_char(text[i - 2]))
            j = string_end(text, i + 1, escaped)
            blank(out, i + 1, max(i + 1, j - 1), "x")
            i = j
        elif c == '"':
            i = identifier_end(text, i + 1)
        elif c == "$" and (i == 0 or not is_ident_char(text[i - 1])) and DOLLAR_TAG.match(text, i):
            tag = DOLLAR_TAG.match(text, i).group(0)
            j = text.find(tag, i + len(tag))
            j = n if j < 0 else j + len(tag)
            blank(out, i, j)
            i = j
        else:
            i += 1
    return "".join(out)


def sections(text):
    header, up, state = [], [], "header"
    for line in text.split("\n"):
        stripped = line.strip()
        if GOOSE_UP.match(stripped):
            state = "up"
        elif GOOSE_DOWN.match(stripped):
            state = "down"
        elif state == "header":
            header.append(stripped)
        elif state == "up":
            up.append(line)
    return [ln for ln in header if ln], "\n".join(up)


def statements(up):
    out, buf, block = [], [], False

    def flush():
        stmt = " ".join(" ".join(buf).split())
        if stmt:
            out.append(stmt)
        buf.clear()

    for raw, line in zip(up.split("\n"), mask(up).split("\n")):
        stripped = raw.strip()
        if BEGIN.match(stripped):
            flush()
            block = True
        elif END.match(stripped):
            flush()
            block = False
        elif block:
            buf.append(line)
        else:
            parts = line.split(";")
            for k, part in enumerate(parts):
                buf.append(part)
                if k < len(parts) - 1:
                    flush()
    flush()
    return out


def last_part(name):
    parts, i, cur = [], 0, ""
    while i < len(name):
        c = name[i]
        if c == '"':
            j = identifier_end(name, i + 1)
            cur += name[i:j]
            i = j
        elif c == ".":
            parts.append(cur.strip())
            cur, i = "", i + 1
        else:
            cur += c
            i += 1
    parts.append(cur.strip())
    last = parts[-1]
    if last.startswith('"'):
        return last[1:-1].replace('""', '"')
    return last.lower()


def names(text):
    return [last_part(m.group(0)) for m in NAME_RE.finditer(text) if m.group(0).lower() not in LIST_KEYWORDS]


def top_level(text):
    out, depth = [], 0
    for c in text:
        if c == "(":
            depth += 1
        elif c == ")":
            depth = max(0, depth - 1)
        elif depth == 0:
            out.append(c)
    return "".join(out)


def split_actions(text):
    parts, depth, cur = [], 0, []
    for c in text:
        if c == "(":
            depth += 1
        elif c == ")":
            depth = max(0, depth - 1)
        if c == "," and depth == 0:
            parts.append("".join(cur).strip())
            cur = []
        else:
            cur.append(c)
    parts.append("".join(cur).strip())
    return [p for p in parts if p]


def existing_table(stmt, new):
    m = ALTER_TABLE.match(stmt)
    if not m or last_part(m.group(1)) in new["table"]:
        return None, []
    return m.group(1), split_actions(m.group(2))


def record_new(stmt, new):
    m = CREATE_TABLE.match(stmt)
    if m:
        new["table"].add(last_part(m.group(1)))
    m = CREATE_ROUTINE.match(stmt)
    if m:
        new[m.group(1).lower()].add(last_part(m.group(2)))


def m2(stmt, ctx, new):
    if ctx["phase"] != "expand":
        return None
    for pattern, what in ((DROP_TABLE, "DROP TABLE"), (TRUNCATE, "TRUNCATE")):
        m = pattern.match(stmt)
        if m and any(n not in new["table"] for n in names(m.group(1))):
            return what, m.group(1)
    m = DROP_ROUTINE.match(stmt)
    if m and last_part(m.group(2)) not in new[m.group(1).lower()]:
        return "DROP " + m.group(1).upper(), m.group(2)
    m = DELETE.search(stmt)
    if m and last_part(m.group(1)) not in new["table"]:
        return "DELETE", m.group(1)
    if ALTER_TABLE.match(stmt):
        table, actions = existing_table(stmt, new)
        for action in actions:
            what = alter_action(action)
            if what:
                return what, table
        return None
    if ALTER_OTHER.match(stmt) and RENAME.search(stmt):
        return "RENAME", stmt[:60]
    return None


def alter_action(action):
    if ACTION_RENAME.match(action):
        return "RENAME"
    if ACTION_DROP.match(action) and not ACTION_DROP_CONSTRAINT.match(action):
        return "DROP COLUMN"
    if ACTION_TYPE.match(action):
        return "ALTER COLUMN ... TYPE"
    if ACTION_SET_NOT_NULL.match(action):
        return "SET NOT NULL"
    return None


def m3(stmt, ctx, new):
    if ctx["phase"] != "expand":
        return None
    table, actions = existing_table(stmt, new)
    for action in actions:
        if ADD_CONSTRAINT.match(action):
            continue
        m = ADD_COLUMN.match(action)
        if not m:
            continue
        rest = top_level(m.group(2))
        if NOT_NULL.search(rest) and not HAS_VALUE.search(rest):
            return m.group(1), table
    return None


def m4(stmt, _ctx, new):
    table, actions = existing_table(stmt, new)
    for action in actions:
        if ADD_CHECK_OR_FK.match(action) and not NOT_VALID.search(action):
            return (table,)
    return None


def m5(stmt, ctx, new):
    m = CREATE_INDEX.match(stmt)
    if not m:
        return None
    concurrently, if_not_exists, table = m.group(1), m.group(2), m.group(3)
    if concurrently and not ctx["no_tx"]:
        return "transaction", table
    if last_part(table) not in new["table"] and not (concurrently and if_not_exists):
        return "lock", table
    return None


def m6(stmt, ctx, new):
    if ctx["phase"] != "expand" or ctx["backfill"]:
        return None
    m = UPDATE.search(stmt)
    if m and last_part(m.group(1)) not in new["table"]:
        return (m.group(1),)
    return None


def m10(stmt, _ctx, _new):
    if ATOMIC_BODY.match(stmt):
        return None
    for piece in stmt.split(";"):
        m = TX_CONTROL.match(piece.strip())
        if m:
            return (m.group(0).upper(),)
    return None


def empty_block(up):
    inside, body = False, False
    for raw, line in zip(up.split("\n"), mask(up).split("\n")):
        stripped = raw.strip()
        if BEGIN.match(stripped):
            inside, body = True, False
        elif END.match(stripped):
            if inside and not body:
                return True
            inside = False
        elif inside and line.strip():
            body = True
    return False


def m9(stmt, ctx, _new):
    if ctx["no_tx"] and not IDEMPOTENT.search(stmt):
        return (stmt[:80],)
    return None


STATEMENT_RULES = (("M2", m2), ("M3", m3), ("M4", m4), ("M5", m5), ("M6", m6), ("M10", m10), ("M9", m9))


def phase_of(directives):
    if not directives:
        return None
    first = directives[0]
    if NO_TX.match(first) and len(directives) > 1:
        first = directives[1]
    m = PHASE.match(first)
    if m and m.group(1) in ("expand", "contract"):
        return m.group(1)
    return None


def floor_updates(up):
    masked = mask(up)
    return [up[m.start(1):m.end(1)] for m in FLOOR_UPDATE.finditer(masked)]


def header_violation(directives, phase, tags):
    if phase is None:
        return ("M1",)
    floor = next((m.group(1) for m in map(FLOOR.match, directives) if m), None)
    if phase != "contract":
        return None
    if floor is None or not SEMVER.match(floor):
        return "M7", "floor", floor
    if floor != "0.0.0" and "v" + floor not in tags:
        return "M8", floor
    return None


def verdict(text, tags):
    directives, up = sections(text)
    phase = phase_of(directives)
    bad = header_violation(directives, phase, tags)
    if bad:
        return bad
    ctx = {
        "phase": phase,
        "no_tx": any(NO_TX.match(ln.strip()) for ln in text.split("\n")),
        "backfill": any(BACKFILL.match(d) for d in directives),
    }
    if empty_block(up):
        return ("M11",)
    new = {"table": set(), "function": set(), "type": set()}
    for stmt in statements(up):
        record_new(stmt, new)
        for rule, check in STATEMENT_RULES:
            found = check(stmt, ctx, new)
            if found:
                return (rule,) + tuple(found)
    if phase == "contract":
        floor = next(m.group(1) for m in map(FLOOR.match, directives) if m)
        values = floor_updates(up)
        if any(v != floor for v in values) or (floor != "0.0.0" and not values):
            return "M7", "update", floor
    return None


def message(found):
    rule, args = found[0], found[1:]
    if rule == "M1":
        return ("the first non-blank line must declare the phase: `-- dhow:phase expand` (additions only) or "
                "`-- dhow:phase contract` (removes or changes something). If the file has `-- +goose NO TRANSACTION`, "
                "the phase goes on the line right after it.")
    if rule == "M2":
        return (f"`{args[0]}` on {args[1]} removes or changes something the previous binary still uses, so it is "
                "a contract change, not an expand. Ship the expand first (add the new shape and write to both), wait "
                "a full release, then remove the old one in its own `-- dhow:phase contract` file with `-- dhow:floor`.")
    if rule == "M3":
        return (f"`ADD COLUMN {args[0]} ... NOT NULL` on {args[1]} has no DEFAULT, so the previous binary's INSERT "
                "fails. Give it a constant DEFAULT or make it nullable.")
    if rule == "M4":
        return (f"a CHECK or FOREIGN KEY on existing table {args[0]} without NOT VALID locks and scans the whole "
                "table. Add it with `NOT VALID` here and `VALIDATE CONSTRAINT` in a later `-- +goose NO TRANSACTION` "
                "file (like 00041 and 00042).")
    if rule == "M5" and args[0] == "transaction":
        return (f"`CREATE INDEX CONCURRENTLY` on {args[1]} cannot run inside a transaction. Put it in its own file "
                "that starts with `-- +goose NO TRANSACTION`.")
    if rule == "M5":
        return (f"`CREATE INDEX` on existing table {args[1]} blocks writes for the whole build. Use "
                "`CREATE INDEX CONCURRENTLY IF NOT EXISTS` in its own `-- +goose NO TRANSACTION` file.")
    if rule == "M6":
        return (f"`UPDATE {args[0]}` in an expand migration rewrites rows during the upgrade. A large backfill runs "
                "in the background in batches (U4). If the table is known to stay small (under 10,000 rows, like "
                "settings), declare `-- dhow:backfill small` in the header.")
    if rule == "M7" and args[0] == "floor":
        return ("a contract migration declares `-- dhow:floor X.Y.Z` in its header: the oldest version that no "
                "longer uses what it removes. Before the first release it is `-- dhow:floor 0.0.0`.")
    if rule == "M7":
        return (f"the contract sets the floor to {args[1]}, so the Up section needs "
                f"`UPDATE \"Instance\" SET \"SchemaFloor\" = '{args[1]}'` with exactly that value (and no other). "
                "The floor is what stops an older binary from running on this schema.")
    if rule == "M8":
        return (f"`-- dhow:floor {args[0]}` is not a released version: there is no tag v{args[0]} in the repo. "
                "The floor is a version that shipped, or 0.0.0 before the first release.")
    if rule == "M10":
        return (f"`{args[0]}` controls the transaction. goose runs each file in its own transaction (or none, with "
                "`-- +goose NO TRANSACTION`), and `dhow-server migrate dry-run` relies on one transaction it can roll "
                "back. A migration never runs BEGIN, COMMIT, END, ROLLBACK, ABORT, SAVEPOINT, RELEASE or PREPARE "
                "TRANSACTION. Split the work into separate files instead.")
    if rule == "M11":
        return ("a `-- +goose StatementBegin` ... `-- +goose StatementEnd` block has no statement in it. goose then "
                "reads the next line as a statement of its own, while dhow's splitter does not. Remove the empty block.")
    return (f"`{args[0]}` in a NO TRANSACTION file is not idempotent. If the migration stops halfway it runs "
            "again, so every statement uses IF NOT EXISTS, IF EXISTS or VALIDATE.")


def git(root, *args):
    try:
        r = subprocess.run(["git", "-C", root, *args], capture_output=True, text=True, timeout=5, check=False)
    except (OSError, subprocess.SubprocessError):
        return None
    return r


def on_main(root, rel):
    for ref in ("origin/main", "main"):
        r = git(root, "cat-file", "-e", f"{ref}:{rel}")
        if r is not None and r.returncode == 0:
            return True
    return False


def repo_tags(root):
    r = git(root, "tag", "--list", "v*")
    if r is None or r.returncode != 0:
        return set()
    return {ln.strip() for ln in r.stdout.splitlines() if ln.strip()}


def check_migration_compat(rel, root, final_content, **_):
    m = MIGRATION_FILE.search(rel)
    if not m or final_content is None or int(m.group(1)) <= GRANDFATHERED_THROUGH:
        return None
    if on_main(root, rel):
        return None
    found = verdict(final_content, repo_tags(root))
    if not found:
        return None
    return f"{rel}: [{found[0]}] {message(found)} The rule is expand/contract ({ADR}, Plan/Upgrades-and-Help.md §5)."
