import re

from .common import GENERATED_PATH, is_test, new_lines

SERVER_INTERNAL = "apps/server/internal/"
SERVER_IMPORT = re.compile(r'"github\.com/Khaymah/dhow/apps/server/internal/([^"]+)"')
PORTS = ("JobQueue", "Triggers", "RunEvents", "CommitStatus", "ArtifactIndex", "Deployments")
PORT_IMPORTS = ("Database/Sqlc",)
PORT_IMPORTERS = ("App", "Modules")
TOP_LEVEL_CODE = re.compile(r"^(func|var)\s")


def area(path):
    parts = path.split("/")
    return (parts[0], parts[1] if len(parts) > 1 else None)


def is_port(a):
    return a[0] == "Core" and a[1] in PORTS


def server_source(rel):
    if not rel.endswith(".go") or not rel.startswith(SERVER_INTERNAL) or is_test(rel) or "/Test/" in rel:
        return None
    if GENERATED_PATH.search(rel):
        return None
    return area(rel[len(SERVER_INTERNAL):])


def port_import_violation(src, target):
    dst = area(target)
    if is_port(src) and dst != src and target not in PORT_IMPORTS:
        return (f"the port Core/{src[1]} imports nothing from the server but Database/Sqlc, for the caller's transaction. "
                "The implementation lives in its module and App wires it.")
    if is_port(dst) and src != dst and src[0] not in PORT_IMPORTERS:
        return (f"only modules (the caller and the implementer) and App import the port Core/{dst[1]}. "
                "Another Core package or Config never sees it, so the port stays a contract between two modules that App wires.")
    return None


def check_port_imports(rel, added, old, **_):
    src = server_source(rel)
    if src is None:
        return None
    for line in new_lines(added, old):
        for m in SERVER_IMPORT.finditer(line):
            reason = port_import_violation(src, m.group(1))
            if reason:
                return f"{rel}: forbidden import of internal/{m.group(1)}. {reason}"
    return None


def check_port_contents(rel, added, old, **_):
    src = server_source(rel)
    if src is None or not is_port(src):
        return None
    for line in new_lines(added, old):
        if TOP_LEVEL_CODE.match(line):
            return (f"{rel}: Core/{src[1]} is a port: interfaces, data types and constants only, never a func or a var. "
                    "Put the implementation in the module that owns the tables and wire it in App (Plan/ADR, decision 1 of M4).")
    return None
