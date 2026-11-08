import re

TS_EXT = (".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs")

GENERATED_PATH = re.compile(
    r"(^|/)(gen|sqlc)/|\.gen\.go$|_gen\.go$|"
    r"(^|/)apps/web/Src/Core/Api/Schema\.d\.ts$|(^|/)apps/web/Src/Styles/Tokens\.css$|"
    r"(^|/)apps/web/Src/Core/Help/Generated/|"
    r"(^|/)(go\.sum|pnpm-lock\.yaml)$", re.IGNORECASE)


def is_ts(path):
    return path.lower().endswith(TS_EXT) and not path.endswith(".d.ts")


def is_test(path):
    return path.endswith("_test.go") or re.search(r"\.(test|spec)\.[cm]?[jt]sx?$", path) is not None


def new_lines(added, old):
    old_set = {ln.strip() for ln in old.splitlines()}
    return [ln for ln in added.splitlines() if ln.strip() and ln.strip() not in old_set]


CMD_START = r"(?:^|[\n;&|(`{!]|&&|\|\||\bthen\b|\bdo\b|\belse\b)\s*"
SHELL_WORD = r"(?:[^\s\"';&|`()]|\"[^\"]*\"|'[^']*')"
SHELL_VALUE = SHELL_WORD + "+"
ASSIGNMENT = r"\w+=" + SHELL_WORD + "*"
RUNNER = (r"\\?(?:(?:command|builtin|exec|nohup|nice|time)(?:\s+-[A-Za-z]+(?:\s+\d+)?)*"
          r"|env(?:\s+(?:-[A-Za-z]+|" + ASSIGNMENT + r"))*"
          r"|sudo(?:\s+-[A-Za-z]+(?:\s+(?!-|git(?:\s|$))" + SHELL_VALUE + r")?)*)")
VALUED_OPTION = r"(?:-[cC]|--(?:git-dir|work-tree|namespace|super-prefix|config-env|exec-path|attr-source))"
GIT_GLOBAL_OPTION = VALUED_OPTION + r"\s+" + SHELL_VALUE + r"|(?!" + VALUED_OPTION + r"(?:\s|$))-" + SHELL_VALUE
GIT_PREFIX = (CMD_START + r"(?:(?:" + ASSIGNMENT + r"|" + RUNNER + r")\s+)*\\?(?:" + SHELL_WORD + r"*/)?git"
              r"((?:\s+(?:" + GIT_GLOBAL_OPTION + r"))*)\s+")
SUBCOMMAND_END = r"(?=\s|$|[;&|)`}])"
HOOKS_PATH = re.compile(r"core\.hookspath|GIT_CONFIG_(?:PARAMETERS|COUNT|KEY_\d+)", re.IGNORECASE)


def mask_strings(command):
    out = list(command)
    i, n = 0, len(command)
    while i < n:
        c = command[i]
        if c == "\\":
            i += 2
        elif c == "'":
            end = command.find("'", i + 1)
            if end < 0:
                break
            out[i + 1:end] = "x" * (end - i - 1)
            i = end + 1
        elif c == '"':
            i = mask_double(command, out, i + 1)
        else:
            i += 1
    return "".join(out)


def mask_double(command, out, i):
    n = len(command)
    while i < n:
        c = command[i]
        if c == '"':
            return i + 1
        if c == "\\" and i + 1 < n:
            out[i:i + 2] = "xx"
            i += 2
        elif command.startswith("$(", i):
            i = substitution_end(command, i + 2)
        elif c == "`":
            end = command.find("`", i + 1)
            i = n if end < 0 else end + 1
        else:
            out[i] = "x"
            i += 1
    return n


def substitution_end(command, i):
    depth = 1
    while i < len(command) and depth:
        depth += {"(": 1, ")": -1}.get(command[i], 0)
        i += 1
    return i


def git_subcommand(subcommand):
    return re.compile(GIT_PREFIX + r"(?:" + subcommand + r")" + SUBCOMMAND_END)


def skips_hooks(command, match):
    return HOOKS_PATH.search(command[match.start():match.end()]) is not None


def find_git(pattern, command):
    return pattern.search(mask_strings(command))


def git_global_options(command, match):
    return command[match.start(1):match.end(1)]
