import re

from .common import GENERATED_PATH, is_test, new_lines

SERVER = "apps/server/"
CRYPTO_PACKAGE = "apps/server/internal/Core/Crypto/"
LEGACY_READ = ("apps/server/internal/Core/Keys/Values.go",)
NEW_SEALER = re.compile(r"(?<![\w])(?:\w+\.)?NewSealer\(")
DEFINITION = re.compile(r"^\s*func\s+NewSealer\(")
ADR = "Plan/ADR/0082-sealer-users-move-to-the-envelope.md"


def check_new_sealer(rel, added, old, **_):
    if not rel.endswith(".go") or not rel.startswith(SERVER) or GENERATED_PATH.search(rel):
        return None
    if rel.startswith(CRYPTO_PACKAGE) or rel in LEGACY_READ or is_test(rel) or "/Test/" in rel:
        return None
    for line in new_lines(added, old):
        if NEW_SEALER.search(line) and not DEFINITION.match(line):
            return (f"{rel}: appcrypto.NewSealer is only for reading values sealed before the keyring, in "
                    f"{LEGACY_READ[0]}. Seal new values with Core/Keys: register the column as a value location "
                    f"(keys.Spec with its owner, label and context) and use values.Column(spec) ({ADR}).")
    return None
