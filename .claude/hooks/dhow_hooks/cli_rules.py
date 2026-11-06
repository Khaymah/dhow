import re

from .common import GENERATED_PATH, is_test, new_lines

CLI_ROOT = "apps/cli/"
CLI_INTERNAL = "apps/cli/internal/"
CLI_IMPORT_PREFIX = "github.com/Khaymah/dhow/apps/cli/internal/"
CLI_LAYERS = ("App", "Lint", "Output")
CLI_ALLOWED_IMPORTS = {
    "App": {"Lint", "Output"},
    "Output": {"Lint"},
    "Lint": set(),
}
PASCAL_DIR = re.compile(r"^[A-Z][A-Za-z0-9]*$")
PASCAL_GO_FILE = re.compile(r"^[A-Z][A-Za-z0-9]*(\.[A-Z][A-Za-z0-9]*)*\.go$")
GO_IMPORT_PATH = re.compile(r'["`]' + re.escape(CLI_IMPORT_PREFIX) + r'([^"`]+)["`]')
OTHER_APP_IMPORT = re.compile(r'["`]github\.com/Khaymah/dhow/apps/(server|runner|web)/')
NETWORK_IMPORT = re.compile(
    r'["`](net|net/http(/[^"`]*)?|net/rpc(/[^"`]*)?|net/smtp|net/mail|crypto/tls|os/exec|plugin|syscall|'
    r'golang\.org/x/sys(/[^"`]*)?|golang\.org/x/net(/[^"`]*)?|unsafe|C)["`]')
PROCESS_CALL = re.compile(r"\b(os\.StartProcess|syscall\.(Exec|ForkExec|StartProcess|Syscall6?|RawSyscall6?|Socket|Connect))\s*\(")


def is_cli_go(rel):
    return rel.endswith(".go") and rel.startswith(CLI_ROOT) and not GENERATED_PATH.search(rel)


def check_cli_layout(rel, **_):
    if not is_cli_go(rel) or not rel.startswith(CLI_INTERNAL):
        return None
    parts = rel[len(CLI_INTERNAL):].split("/")
    layers = ", ".join(CLI_LAYERS)
    if len(parts) < 2 or parts[0] not in CLI_LAYERS:
        return (f"{rel}: Go code under apps/cli/internal lives in one of {layers} (Plan/M4-Pipelines.md), "
                "each in its own folder (Lint/Lint.go).")
    for folder in parts[:-1]:
        if not PASCAL_DIR.match(folder):
            return f"{rel}: folder `{folder}` must be PascalCase (Output/Golden)."
    name = parts[-1]
    if not name.endswith("_test.go") and not PASCAL_GO_FILE.match(name):
        return f"{rel}: Go file names are PascalCase with dot roles (Messages.Ar.go), not `{name}`."
    return None


def cli_import_violation(src, dst):
    if dst == src:
        return None
    if dst == "App":
        return "App is the CLI's composition root, so nothing imports it."
    if dst not in CLI_ALLOWED_IMPORTS.get(src, set()):
        allowed = ", ".join(sorted(CLI_ALLOWED_IMPORTS.get(src, set()))) or "no other CLI layer"
        return f"{src} may import {allowed}. Pass what it needs in from App instead."
    return None


def check_cli_process_calls(rel, added, old, **_):
    if not is_cli_go(rel):
        return None
    for line in new_lines(added, old):
        found = PROCESS_CALL.search(line)
        if found:
            return (f"{rel}: dhow lint never starts a process or makes a raw system call ({found.group(1)}). "
                    "Shared helpers in internal/ (safefile) do the few system calls the CLI needs.")
    return None


def check_cli_imports(rel, added, old, **_):
    if not is_cli_go(rel) or is_test(rel):
        return None
    parts = rel[len(CLI_ROOT):].split("/")
    in_internal = parts[0] == "internal" and len(parts) > 2 and parts[1] in CLI_LAYERS
    for line in new_lines(added, old):
        if OTHER_APP_IMPORT.search(line):
            return (f"{rel}: the CLI never imports server, runner or web code. Shared code lives in internal/ "
                    "(pipelinespec, buildinfo).")
        if NETWORK_IMPORT.search(line):
            return (f"{rel}: dhow lint works offline, so the CLI's own code never imports network, process or raw system "
                    "packages (net, net/http, crypto/tls, os/exec, syscall, golang.org/x/sys, unsafe, cgo). "
                    "Uploads go through internal/ingestpush, the one package that talks to the network (ADR-0099). "
                    "The no-network test in apps/cli/cmd/dhow proves it.")
        for m in GO_IMPORT_PATH.finditer(line):
            target = m.group(1).split("/")[0]
            if target not in CLI_LAYERS:
                return f"{rel}: internal/{m.group(1)} is not a CLI layer ({', '.join(CLI_LAYERS)})."
            if not in_internal:
                if target != "App":
                    return f"{rel}: the command imports App only, and App wires the other layers."
                continue
            reason = cli_import_violation(parts[1], target)
            if reason:
                return f"{rel}: forbidden import of internal/{m.group(1)}. {reason}"
    return None
