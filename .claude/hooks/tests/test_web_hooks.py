import os
import sys
import unittest

HOOKS = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HOOKS)

from dhow_hooks import edit_rules

SRC = "apps/web/Src/"


def edit(rel, added, old="", exists=False):
    return edit_rules.evaluate(rel=rel, root="/nonexistent", segments=[(added, old)], exists=exists, final_content=added)


def component(body):
    return "export const A = () => (\n  <div>\n" + body + "\n  </div>\n);\n"


class WebLayoutRules(unittest.TestCase):
    def test_allowed_locations_and_names(self):
        for rel in ("Main.tsx", "App/App.tsx", "App/Layout/AppShell.tsx", "App/Layout/useShellNavigation.ts",
                    "Core/Ui/Button.tsx", "Core/Ui/Button.test.tsx", "Core/Hooks/useDelayedFlag.ts",
                    "Core/Hooks/Hooks.test.tsx", "Core/Api/Client.ts", "Core/Testing/Setup.ts", "App/RouterRegister.d.ts",
                    "I18n/I18nProvider.tsx", "I18n/Locales/Ar.ts", "Styles/Globals.css", "Styles/Tokens.json",
                    "Modules/System/Pages/WorkspacePage.tsx", "Modules/System/Hooks/useMeta.ts",
                    "Modules/System/Hooks/useMeta.test.ts", "Modules/System/Components/StatusPanel.test.tsx"):
            self.assertIsNone(edit(SRC + rel, "export const A = 1;\n"), rel)

    def test_lowercase_src_folder_is_rejected(self):
        self.assertIn("`Src/`", edit("apps/web/src/Core/Ui/Button.tsx", "export const A = 1;\n"))

    def test_unknown_top_level_folder_or_root_file_is_rejected(self):
        for rel in ("Utils/format.ts", "Components/Button.tsx", "lib/Util.ts"):
            self.assertIn("must live in", edit(SRC + rel, "export const A = 1;\n"), rel)
        for rel in ("Helpers.ts", "Router.tsx"):
            self.assertIn("only Main.tsx", edit(SRC + rel, "export const A = 1;\n"), rel)

    def test_files_directly_in_core_or_modules_are_rejected(self):
        self.assertIn("named folder", edit(SRC + "Core/Button.tsx", "export const A = 1;\n"))
        self.assertIn("named folder", edit(SRC + "Modules/Billing.tsx", "export const A = 1;\n"))
        self.assertIn("named folder", edit(SRC + "Modules/System/Page.tsx", "export const A = 1;\n"))

    def test_folders_are_pascal_case(self):
        for rel in ("Core/ui/Button.tsx", "Modules/system/Pages/A.tsx", "Modules/System/pages/A.tsx", "App/layout/A.tsx"):
            self.assertIn("PascalCase", edit(SRC + rel, "export const A = 1;\n"), rel)

    def test_files_are_pascal_case_and_only_hooks_are_camel_case(self):
        for name in ("button.tsx", "button-card.tsx", "Button_card.tsx", "usemeta.ts", "use_meta.ts",
                     "Button.stories.tsx", "Button.js", "button.test.tsx"):
            self.assertIn("file names are PascalCase", edit(SRC + "Core/Ui/" + name, "export const A = 1;\n"), name)

    def test_hooks_keep_a_lowercase_use_prefix(self):
        for name in ("UseMeta.ts", "UseMeta.tsx", "UseMeta.test.ts"):
            self.assertIn("lowercase `use`", edit(SRC + "Core/Hooks/" + name, "export const A = 1;\n"), name)

    def test_config_scripts_and_public_files_are_not_covered(self):
        for rel in ("apps/web/vite.config.ts", "apps/web/eslint.config.js", "apps/web/Scripts/GenTokens.mjs",
                    "apps/web/public/ThemeInit.js", "apps/web/index.html"):
            self.assertIsNone(edit(rel, "export default {};\n"), rel)


class WebGeneratedRules(unittest.TestCase):
    def test_generated_web_files_are_blocked(self):
        for rel in ("apps/web/Src/Core/Api/Schema.d.ts", "apps/web/Src/Styles/Tokens.css", "apps/web/pnpm-lock.yaml"):
            self.assertIn("generated", edit(rel, "x", exists=True), rel)

    def test_generated_help_files_are_blocked(self):
        for name in ("HelpContentAr.ts", "HelpContentEn.ts", "ReleaseMeta.ts"):
            rel = "apps/web/Src/Core/Help/Generated/" + name
            message = edit(rel, "export const A = 1;\n", exists=True)
            self.assertIn("generated", message, rel)
            self.assertIn("pnpm gen:help", message, rel)

    def test_a_new_file_in_the_generated_help_folder_is_blocked_too(self):
        self.assertIn("generated", edit("apps/web/Src/Core/Help/Generated/Other.ts", "export const A = 1;\n"))

    def test_the_help_types_and_links_are_not_generated(self):
        for rel in ("apps/web/Src/Core/Help/HelpTypes.ts", "apps/web/Src/Core/Help/DocLinks.ts",
                    "apps/web/Src/Core/Help/DocsUrl.ts"):
            self.assertIsNone(edit(rel, "export const A = 1;\n", exists=True), rel)

    def test_the_token_source_is_not_generated(self):
        self.assertIsNone(edit("apps/web/Src/Styles/Tokens.json", "{}\n", exists=True))


def imp(source):
    return "import { A } from '" + source + "';\nexport const B = A;\n"


class WebImportRules(unittest.TestCase):
    def test_module_cannot_import_another_module(self):
        for spec in ("@/Modules/Billing/Hooks/useInvoices", "../../Billing/Hooks/useInvoices"):
            reason = edit(SRC + "Modules/System/Components/Panel.tsx", imp(spec))
            self.assertIn("Modules/System must not import Modules/Billing", reason, spec)

    def test_module_may_import_itself_core_and_i18n(self):
        for spec in ("@/Modules/System/Api/SystemApi", "../Api/SystemApi", "./Sibling", "@/Core/Ui/Button", "@/I18n/useT"):
            self.assertIsNone(edit(SRC + "Modules/System/Components/Panel.tsx", imp(spec)), spec)

    def test_modules_cannot_import_app(self):
        self.assertIn("composition root", edit(SRC + "Modules/System/Pages/Page.tsx", imp("@/App/Layout/useShellNavigation")))

    def test_core_cannot_import_modules_or_app(self):
        self.assertIn("Only App imports modules", edit(SRC + "Core/Ui/Button.tsx", imp("@/Modules/System/Hooks/useMeta")))
        self.assertIn("composition root", edit(SRC + "Core/Ui/Button.tsx", imp("@/App/Providers")))
        self.assertIn("Only App imports modules", edit(SRC + "Core/Hooks/useX.ts", imp("../../Modules/System/Hooks/useMeta")))

    def test_core_may_import_core_and_i18n(self):
        self.assertIsNone(edit(SRC + "Core/Ui/Button.tsx", imp("@/Core/Ui/Icon")))
        self.assertIsNone(edit(SRC + "Core/Hooks/useApiErrorText.ts", imp("@/I18n/useT")))

    def test_i18n_and_styles_cannot_import_modules_or_app(self):
        for rel in ("I18n/I18nProvider.tsx", "I18n/useT.ts", "Styles/Helpers.ts"):
            self.assertIsNotNone(edit(SRC + rel, imp("@/Modules/System/Hooks/useMeta")), rel)
            self.assertIsNotNone(edit(SRC + rel, imp("@/App/Providers")), rel)

    def test_app_imports_modules_and_core(self):
        for spec in ("@/Modules/System/Pages/WorkspacePage", "@/Core/Ui/Button", "@/I18n/useT", "./Panels"):
            self.assertIsNone(edit(SRC + "App/Router.tsx", imp(spec)), spec)

    def test_every_import_form_is_scanned(self):
        path = SRC + "Core/Ui/Button.tsx"
        for code in ("import('@/Modules/System/Hooks/useMeta');\n", "import '@/Modules/System/Styles';\n",
                     "export * from '@/Modules/System/Hooks/useMeta';\n",
                     "import type { T } from '@/Modules/System/Types/SystemTypes';\n",
                     "import {\n  A,\n} from\n  '@/Modules/System/Hooks/useMeta';\n"):
            self.assertIsNotNone(edit(path, code), code)

    def test_packages_and_existing_lines_are_ignored(self):
        path = SRC + "Core/Ui/Button.tsx"
        self.assertIsNone(edit(path, "import { x } from 'react';\nimport y from '@tanstack/react-router';\nimport z from '@radix-ui/react-dialog';\n"))
        line = "import { A } from '@/Modules/System/Hooks/useMeta';\n"
        self.assertIsNone(edit(path, "const x = 1;\n" + line, old=line))

    def test_tests_are_exempt(self):
        self.assertIsNone(edit(SRC + "Core/Ui/Button.test.tsx", imp("@/Modules/System/Hooks/useMeta")))


class WebTextRules(unittest.TestCase):
    path = SRC + "Core/Ui/Panel.tsx"

    def test_text_nodes_are_blocked(self):
        for body in ("    <p>مرحبا بيك</p>", "    <Button>Save changes</Button>", "    <span>Hello {name}</span>",
                     "    <p>{count} items left</p>", "    <p>\n      Open the panel\n    </p>",
                     "    <p>Hello\n      there</p>", "    <h1>اقفل</h1>", "    <p>{'Hello world'}</p>"):
            reason = edit(self.path, component(body))
            self.assertIn("no hardcoded user-facing text", reason or "", body)

    def test_attribute_literals_are_blocked(self):
        for attr in ('aria-label="Close"', 'title="إغلاق"', "placeholder='Search here'", "aria-label={'Close it'}",
                     'alt="Dhow logo"'):
            reason = edit(self.path, component("    <input " + attr + " />"))
            self.assertIn("no hardcoded user-facing text", reason or "", attr)

    def test_translated_and_dynamic_content_is_allowed(self):
        for body in ("    <p>{t('workspace.title')}</p>", "    <p>{label}</p>", "    <kbd>{shortcut}</kbd>",
                     "    <span>42</span>", "    <p>{' '}</p>", "    <Icon name=\"search\" size={18} />",
                     "    <input aria-label={t('shell.search')} placeholder={placeholder} title={title} />",
                     "    <div className=\"grid gap-2 text-start\" role=\"status\" aria-hidden=\"true\" />",
                     "    <bdi dir=\"ltr\" className=\"font-mono\">{version}</bdi>"):
            self.assertIsNone(edit(self.path, component(body)), body)

    def test_typescript_syntax_is_not_mistaken_for_text(self):
        code = ("const [Open, setOpen] = useState<string>('closed');\n"
                "const f = (a: number) => a < 3 && a > 1;\n"
                "const g = async <T,>(value: T): Promise<T> => value;\n"
                "const map: Record<string, number> = {};\n")
        self.assertIsNone(edit(self.path, code))

    def test_locales_tests_and_non_tsx_files_are_exempt(self):
        text = component("    <p>Hello there</p>")
        self.assertIsNone(edit(SRC + "Core/Ui/Panel.test.tsx", text))
        self.assertIsNone(edit(SRC + "Core/Testing/Render.tsx", text))
        self.assertIsNone(edit(SRC + "I18n/Locales/Ar.ts", "export const Ar = { 'a': 'مرحبا' };\n"))
        self.assertIsNone(edit(SRC + "Core/Hooks/useX.ts", "export const title = 'Hello world';\n"))

    def test_existing_text_is_not_flagged_again(self):
        old = component("    <p>Hello there</p>")
        self.assertIsNone(edit(self.path, old + "const x = 1;\n", old=old))


if __name__ == "__main__":
    unittest.main()
