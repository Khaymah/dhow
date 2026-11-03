import os
import sys
import unittest

HOOKS = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HOOKS)

from dhow_hooks import edit_rules


def edit(rel, added, old=""):
    return edit_rules.evaluate(rel=rel, root="/nonexistent", segments=[(added, old)], exists=bool(old), final_content=added)


SERVER = "apps/server/internal/"
CALL = "package x\n\nfunc f(secret []byte) {\n\ts, err := appcrypto.NewSealer(secret, config.DeployKeySealLabel)\n\t_, _ = s, err\n}\n"


class NewSealerRule(unittest.TestCase):
    def test_new_sealers_outside_the_allowlist_blocked(self):
        for rel in (SERVER + "Modules/Sources/Sources.Service.go", SERVER + "App/Wire.go", SERVER + "Core/Settings/Store.go",
                    SERVER + "Modules/Secrets/Secrets.Service.go", SERVER + "Core/Keys/Envelope.go", SERVER + "App/Testing/Seed.go",
                    "apps/server/cmd/dhow-server/main.go"):
            reason = edit(rel, CALL)
            self.assertIsNotNone(reason, rel)
            self.assertIn("NewSealer", reason)
            self.assertIn("Core/Keys", reason)

    def test_another_import_name_is_still_blocked(self):
        src = CALL.replace("appcrypto.NewSealer", "crypt.NewSealer")
        self.assertIsNotNone(edit(SERVER + "Modules/Sources/Sources.Keys.go", src))
        src = CALL.replace("appcrypto.NewSealer", "NewSealer")
        self.assertIsNotNone(edit(SERVER + "Modules/Sources/Sources.Keys.go", src))

    def test_the_crypto_package_and_the_legacy_reader_may_make_sealers(self):
        self.assertIsNone(edit(SERVER + "Core/Crypto/Sealer.go",
                               "package appcrypto\n\nfunc NewSealer(secret []byte, label string) (*Sealer, error) {\n\treturn nil, nil\n}\n"))
        self.assertIsNone(edit(SERVER + "Core/Keys/Values.go", CALL))

    def test_tests_may_seal_old_values_to_prove_they_still_read(self):
        self.assertIsNone(edit(SERVER + "App/SealedValues_test.go", CALL))
        self.assertIsNone(edit(SERVER + "Modules/Settings/Test/E2E/Reload_test.go", CALL))
        self.assertIsNone(edit(SERVER + "Modules/Session/Test/E2E/Helpers.go", CALL))

    def test_other_crypto_helpers_and_unchanged_lines_pass(self):
        self.assertIsNone(edit(SERVER + "Core/Auth/ApiToken.go",
                               "package auth\n\nfunc f(s []byte) { _, _ = appcrypto.DeriveKey(s, config.APITokenKeyLabel) }\n"))
        self.assertIsNone(edit(SERVER + "Modules/Sources/Sources.Service.go", CALL + "\nvar x = 1\n", old=CALL))
        self.assertIsNone(edit(SERVER + "Modules/Sources/Sources.Service.go", "package x\n\nfunc MyNewSealerFactory() {}\n"))

    def test_the_runner_and_shared_packages_are_not_this_rule(self):
        self.assertIsNone(edit("internal/leaksweep/leaksweep.go", "package leaksweep\n\nfunc NewSealer() {}\nvar _ = NewSealer()\n"))


if __name__ == "__main__":
    unittest.main()
