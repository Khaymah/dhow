import re

from .common import GENERATED_PATH, new_lines

SERVER = "apps/server/"
CRYPTO_PACKAGE = "apps/server/internal/Core/Crypto/"
NEW_SEALER = re.compile(r"(?<![\w])NewSealer(?![\w])")
DEFINITION = re.compile(r"^\s*func\s+NewSealer\s*\(")
ADR = "Plan/ADR/0088-contract-the-old-sealed-format.md"


def check_new_sealer(rel, added, old, **_):
    if not rel.endswith(".go") or not rel.startswith(SERVER) or GENERATED_PATH.search(rel):
        return None
    if rel.startswith(CRYPTO_PACKAGE):
        return None
    for line in new_lines(added, old):
        if NEW_SEALER.search(line) and not DEFINITION.match(line):
            return (f"{rel}: appcrypto.NewSealer lives in Core/Crypto only; the old sealed format is not read any more. "
                    f"Seal new values with Core/Keys: register the column as a value location "
                    f"(keys.Spec with its owner, label and context) and use values.Column(spec) ({ADR}).")
    return None
