# CLAUDE.md — داو (Dhow)

إرشادات لأي agent بيشتغل على الـ repo ده. البشر يبدأوا من [README.md](README.md) و[CONTRIBUTING.md](CONTRIBUTING.md).

## المشروع

داو منصة Continuous Deployment آمنة بتتنصّب عند العميل. فيها control plane بـ Go، وrunners بـ Go بتشغّل الـ jobs في sandbox معزول، وواجهة React عربي أولاً.
- **الوثائق العامة** بالعربي المبسّط في `docs/` والإنجليزي في `docs/en/` (ADR-0064). العربي هو المرجع، وأي تغيير بيتكتب في النسختين في نفس الـ commit.
- **القرارات** (ADRs بالعربي) في `Plan/ADR/`. اقرا الـ ADR الخاص بأي جزء قبل ما تغيّر فيه، وأي قرار جديد يبقى ADR جديد مش تعديل في القديم.
- **السجل اليومي** في `Plan/Progress/YYYY-MM-DD.md`.
- **الحالة** (اللي اتعمل واللي شغّال واللي لسه) في `Plan/Status.md`. ابدأ منها في أي جلسة جديدة، وحدّثها بعد أي merge.

## الهيكل (ADR-0004)

| المسار | المحتوى |
|---|---|
| `apps/server` | الـ control plane (Go) |
| `apps/runner` | الـ runner (Go) |
| `apps/web` | الواجهة (React + TypeScript + Tailwind، pnpm) |
| `apps/cli` | الـ CLI `dhow` (Go) |
| `packages/openapi` | `openapi.yaml`، عقد الـ REST API (spec-first) |
| `packages/proto` | بروتوكول الـ runner مع الـ control plane (Connect/gRPC) |
| `internal/` | packages Go مشتركة بين التطبيقات |
| `ee/` | المميزات التجارية (Dhow Enterprise License)، وكل الباقي AGPL-3.0 |
| `deploy/` | Dockerfiles وCompose وHelm |
| `docs/` | وثائق عامة بالعربي المبسّط، ونسختها الإنجليزي في `docs/en/` بنفس المسارات (ADR-0064) |
| `Plan/` | خطط وADRs وسجل يومي، بالعربي |

### هيكل `apps/server/internal` (Modular Monolith، ADR-0030)

| المجلد | الدور |
|---|---|
| `App/` | الـ router والـ middleware العامة والـ bootstrap (`Run()`)، وهو الوحيد اللي بيستورد `Modules/` |
| `Config/` | الـ env والـ validator وفتح الـ DB والـ logger والثوابت |
| `Core/<X>/` | حاجات مشتركة: `Errors` (`package apperrors`) و`Response` و`I18n` و`Middleware` و`EventBus` |
| `Database/<X>/` | `Migrations` (ملفات SQL والـ migrator) و`Sqlc` (متولّد) |
| `Modules/<Name>/` | كل ميزة لوحدها: `<Name>.Routes.go` و`.Controller.go` و`.Service.go` و`Models/` و`Queries/` و`Gen/` و`Test/{Unit,Integration,E2E,Edge}` |

- أسماء المجلدات وملفات Go PascalCase بنقط للأدوار (`System.Service.go`)، وأسماء الـ packages lowercase.
- الموديول ممنوع يستورد موديول تاني. التواصل بينهم عن طريق `Core/EventBus`. و`Core` و`Config` و`Database` ممنوع يستوردوا `Modules` أو `App`.
- كل موديول ليه `oapi-codegen.yaml` بـ `include-tags`، و`make gen` بيشغّلهم كلهم.
- أي جدول جديد فيه الأعمدة الأربعة: `"Id" uuid DEFAULT uuidv7()` و`"CreatedAt"` و`"UpdatedAt"` و`"IsDeleted"`. الاستثناء الوحيد جدول `"Instance"`.
- الحذف soft بس (ممنوع `DELETE FROM` في `Queries/`)، والوقت `timestamptz` بـ UTC، والنص `TEXT` مش `VARCHAR`، والترجمات في جدول `<Entity>Translations` مش أعمدة `NameAr`/`NameEn`.
- كل migration جديد بيبدأ بـ `-- dhow:phase expand` أو `-- dhow:phase contract` (ADR-0077): الـ expand إضافات بس (جدول، أو عمود nullable أو بـ `DEFAULT`، أو index بـ `CONCURRENTLY IF NOT EXISTS` في ملف `NO TRANSACTION`، أو constraint بـ `NOT VALID`)، وأي شيل أو تغيير contract بـ `-- dhow:floor X.Y.Z` بعد release كامل. التفاصيل في `Plan/Upgrades-and-Help.md` §5.
- مسارات الـ API بـ kebab-case، وأكواد الأخطاء UPPER_SNAKE، ورسالة الخطأ بتتترجم في السيرفر من `Accept-Language`، ورسالة النجاح هي مفتاح i18n.
- PostgreSQL 18 (لـ `uuidv7()` المدمجة).

### هيكل `apps/web/Src` (ADR-0031)

| المجلد | الدور |
|---|---|
| `Main.tsx` | الملف الوحيد المسموح في جذر `Src` |
| `App/` | الـ router والـ providers والـ layout (`Layout/`)، وهو الوحيد اللي بيستورد `Modules/` |
| `Core/<X>/` | `Ui` (مكوّنات الـ Design System) و`Api` (`Schema.d.ts` متولّد و`Client.ts`) و`Hooks` و`Store` و`Testing` |
| `I18n/` | `Locales/{Boot,Workspace}/{Ar,En}.ts` بنفس المفاتيح (و`Locales/{Ar,En}.ts` الكاملين للأنواع والاختبارات) و`I18nProvider` و`useT` |
| `Styles/` | `Tokens.json` مصدر الـ tokens، و`Themes.json` ألوان ثيمات النظام، و`Tokens.css` متولّد منهم، و`Globals.css` (Tailwind v4 بـ `@theme`) |
| `Modules/<Name>/<Folder>/` | `Pages` و`Components` و`Hooks` و`Api` و`Store` و`Types` |

- PascalCase للفولدرات والملفات والـ components والـ state والـ handlers، والـ hooks بس `useX.ts` بـ `use` صغيرة. الاختبار جنب الملف: `X.test.tsx`.
- module ممنوع يستورد module تاني، و`Core` و`I18n` و`Styles` ممنوع يستوردوا `Modules` أو `App`، و`Modules` ممنوع يستورد `App`. الاستيراد بـ `@/` (مثلًا `@/Core/Ui/Button`).
- ممنوع أي نص مكتوب في الـ JSX أو في `aria-label` و`title` و`placeholder`: كل نص مفتاح بنفس المفاتيح في العربي والإنجليزي، ويتعرض بـ `t('key')`. نصوص الدخول والتنصيب وإطار الـ shell في `I18n/Locales/Boot/{Ar,En}.ts`، والباقي في `I18n/Locales/Workspace/{Ar,En}.ts` (ADR-0049).
- الألوان من الـ tokens بس (`bg-canvas` و`text-secondary`)، ومفيش hex في الكود. الزرار الأساسي `bg-accent text-on-accent`، والخطوط الرفيعة `border-line` و`border-line-2`. ممنوع أي gradient أو edge-light في المكوّنات (بيفضل في اللوجو بس، ومختبر)، وممنوع drop shadow على الكروت، وممنوع نص أصغر من 13px ([ADR-0034](Plan/ADR/0034-ui-direction-calm-and-organized.md)).
- الـ CSP هي `'self'`: ممنوع inline scripts أو styles، وممنوع `Dialog.Overlay` من Radix لأنه بيحقن `<style>` (استخدم الـ backdrop اللي في `SlideStack`، واختبارات `SlideStack` و`CommandPalette` بتتأكد إن مفيش `<style>` اتحقن). وzod لازم يشتغل بـ `z.config({ jitless: true })` (في `ThemeForm.ts`) لأن الـ JIT بيجرّب `new Function` وده بيطلّع violation.
- `Schema.d.ts` بيتولّد بـ `pnpm gen:api` من `packages/openapi/openapi.yaml`، و`Tokens.css` بـ `pnpm gen:tokens`، وممنوع تعديلهم بإيدك.

ممنوع ملفات Markdown بره `docs/` و`Plan/` و`.github/` و`.claude/`. الاستثناء: README وCONTRIBUTING وSECURITY (بالعربي) وجنبهم README.en وCONTRIBUTING.en وSECURITY.en، وCLAUDE وLICENSE في الـ root. ومفيش README.ar، لأن README نفسه بقى العربي.

## الأوامر

- `make check`: lint وactionlint واختبارات Go بـ `-race` وgovulncheck واختبارات الـ hooks و`make docs-check`. الـ CI بيشغّل نفس الـ targets، فشغّلها قبل أي push.
- `make docs-check`: بيتأكد إن `docs/` و`docs/en/` (وملفات الـ root وجنبها `.en.md`) فيهم نفس الملفات، ونفس عدد العناوين في كل مستوى، ونفس الجداول والصفوف، ونفس الـ code blocks بالحرف، وإن كل رابط وanchor بيوصل جوه نسخته، وإن روابط الكود للوثائق موجودة (`deploy/docs-check`).
- `make help` بيعرض كل الـ targets. نسخة Go مثبّتة في `go.mod`، والـ Makefile بيبني الأدوات بيها.
- الواجهة: جوه `apps/web` شغّل `pnpm lint` و`pnpm typecheck` و`pnpm test` و`pnpm build` و`pnpm size` (ميزانية الحجم: JS أقل من 180 KB gzip وCSS أقل من 30 KB).
- `make web` بيبني الواجهة وينسخها في `apps/server/internal/App/WebDist/` (مش بيتعمله commit)، و`make release` بيبني الـ binary بيها. `make check` و`make build` Go بس، من غير الواجهة، والـ CI بيشغّل job الواجهة (`web`) لوحده.

## الـ Hooks (`.claude/settings.json`)

القواعد اللي تحت متطبّقة كـ hooks قبل أي Write أو Edit أو Bash، يعني الـ hook هيرفض أي تعديل مخالف ويقولك السبب. ده بيشمل:
- الكومنتات.
- `sh -c`.
- SQL مبني كنص.
- JSON tags مش PascalCase.
- `panic`، و`fmt.Print`، و`os.Getenv` بره الـ config.
- `any` والتسمية في TypeScript.
- الخصائص الفيزيائية (left/right) في الواجهة.
- مكان ملفات `apps/web/Src` وتسمية PascalCase (والـ hooks بـ `use`)، واتجاه الاستيراد بين `App` و`Core` و`I18n` و`Styles` و`Modules`.
- أي نص عربي أو إنجليزي مكتوب في JSX أو في `aria-label` و`title` و`placeholder` بره `I18n/Locales/` والاختبارات.
- العامية المصرية والفصحى التقيلة في كتالوجات الرسائل العربي (ADR-0062)، وفي الوثائق العربي: `docs/` بره `docs/en/`، وREADME وCONTRIBUTING وSECURITY (من غير الـ code blocks والـ inline code).
- الوثائق في النسختين: أي `git commit` فيه ملف تحت `docs/` من غير مقابله في `docs/en/` (أو العكس، أو ملف root من غير `.en.md` بتاعه) بيترفض ويقول الملف الناقص. الـ rename من `docs/X` لـ `docs/en/X` بيتحسب الاتنين (ADR-0064).
- الملفات المتولّدة.
- الأسرار.
- مكان الـ Markdown.
- طول الملف.
- الـ migrations اللي اتعملها merge.
- مكان ملفات Go في `apps/server/internal`، وتسمية PascalCase، واتجاه الاستيراد بين الطبقات.
- قواعد الـ migrations (الأعمدة الأساسية، `timestamptz`، `TEXT`، ممنوع أعمدة `Ar`/`En`) وممنوع `DELETE FROM` في الـ queries.
- قاعدة expand/contract لأي migration جديد بعد `00048` (ADR-0077، القواعد M1 لـ M11 في `migration_rules.py`). ونفس القواعد في `Database/Migrations/Compat_test.go` عشان الـ CI يطبّقها على أي PR، والاتنين بيمشوا على نفس الحالات في `Database/Migrations/testdata/compat/`.
- مسارات `openapi.yaml` بـ kebab-case.
- رسائل الـ commits (Conventional، إنجليزي، ومن غير أي إشارة لـ AI).
- اسم المشروع: `github.com/Khaymah/dhow` والـ image `ghcr.io/khaymah/dhow`، وأي ذكر لاسم المنظمة القديم مرفوض.
- `--no-verify` والـ force-push على main.

اختباراتها في `.claude/hooks/tests/` وبتتشغّل بـ `make hooks-test`. أي قاعدة جديدة لازم يبقى ليها hook واختبار.

## قواعد ما بتتكسرش

- **ممنوع تشغيل نص جاي من المستخدم من خلال shell.** الأوامر تتبني كـ argv (`exec.CommandContext(ctx, name, args...)`)، والـ refs والمسارات يتعملها validation، و`--` قبل أي قيمة جاية من المستخدم.
- **ممنوع تقييم expressions بمفسّر عام.** شروط الـ pipeline بـ CEL بس.
- **الأسرار للكتابة بس.** ممنوع أي response أو log أو event أو إشعار يحتوي قيمة سر، ولازم اختبار يثبت ده.
- **Fail closed.** أي سر ناقص أو افتراضي، أو إعداد غلط، أو migration فشل، بيوقف الإقلاع.
- **كل endpoint بيعدّي على policy مركزية `can()`**، ومع كل route جديد صف في اختبار الـ route-table لكل دور.
- **الـ SQL من خلال sqlc بس** وبالـ parameters. ممنوع SQL مبني كنص.
- **ممنوع `panic` في كود الإنتاج، وممنوع تجاهل أي error، وممنوع global mutable state** بره `main`.
- **الاعتماديات:** تتضاف بس لما تشيل تعقيد حقيقي، وبنسخ مثبّتة بالظبط، والـ CI بيفحص كل PR.

## الأسلوب

- **الكود من غير أي كومنتات:** لا line ولا block ولا JSDoc ولا Go doc comments. الاستثناء الوحيد كومنت بيبدأ بـ `IMPORTANT:` لحاجة استثنائية فعلاً، والـ directives (`//go:`، `//nolint:`، `eslint-`...). المبرّرات مكانها `Plan/ADR`.
- **حدود الحجم:** الملف 500 سطر بالكتير، والـ function 100 سطر (`funlen` في golangci-lint، وESLint في الواجهة).
- **Go:** الأسلوب الرسمي (exported بـ PascalCase، وerrors بـ `Err` prefix)، والـ standard library الأول، و`log/slog` بـ JSON، و`context.Context` أول argument، والـ errors تتلف بـ `%w`، واختبارات table-driven.
- **الـ API:** أسماء الحقول في JSON بـ **PascalCase** (`SetupRequired`، `CreatedAt`)، والـ envelope هو `{Success, Data, Error}`. عدّل `packages/openapi/openapi.yaml` الأول، وبعدين `make gen`، وبعدين التنفيذ.
- **قاعدة البيانات:** أسماء الجداول والأعمدة بـ **PascalCase** بين علامات تنصيص (`"Projects"`، `"CreatedAt"`). أي migration بعد ما يتعمله merge ما يتعدّلش، ويتضاف واحد جديد.
- **TypeScript:**
  - الـ interfaces بـ `I` والـ types بـ `T` والـ enums بـ `E`.
  - الـ components arrow functions.
  - PascalCase للـ public methods وللـ handlers (`HandleSubmit`) ولمتغيرات الـ state (`const [Loading, setLoading]`).
  - ممنوع `any` و`console.*`.
- **الواجهة:** العربي هو اللغة الأساسية، والـ RTL هو الاتجاه الافتراضي، والـ CSS بالـ logical properties بس. الاتجاه «غرفة التشغيل» ([ADR-0059](Plan/ADR/0059-ui-direction-control-room.md)، بيعدّل [ADR-0034](Plan/ADR/0034-ui-direction-calm-and-organized.md)): قايمة جانبية بعرض 248px مقسومة مجموعات (شريط تحت على الموبايل بخمس أسماء)، وكروت مسطّحة بخط رفيع، وزرار أساسي واحد مصمت في كل منطقة والخطر مرجاني بأيقونة، ومفيش blur ولا hover lift. والمصطلحات العربي أولًا من `Plan/Glossary.md`، وأي نص المستخدم بيقراه (الواجهة ورسائل السيرفر والإيميلات والـ installer) بالعربي المبسّط: فصحى بجمل قصيرة، لا عامية ولا فصحى تقيلة ([ADR-0062](Plan/ADR/0062-simple-arabic-for-users.md))، والزرار بيقول هيعمل إيه بالظبط.

## الـ git

- رسائل الـ commits والـ PRs **بالإنجليزي** وبـ Conventional Commits، ومع DCO sign-off (`git commit -s`).
- **ممنوع أي إشارة لـ AI** في أي commit أو PR: لا `Co-Authored-By: Claude` ولا «Generated with Claude Code» ولا روابط sessions.
- ممنوع `--no-verify`، وممنوع force-push على `main`.
- **أي branch بيتفتح له draft PR من أول commit**، وفي وصفه «اتعمل» و«لسه»، عشان الشغل يبقى محفوظ وظاهر.
- **الـ CI مابيشتغلش على الـ draft.** كل job فيه شرط `!github.event.pull_request.draft`، والـ `pull_request` بيسمع لـ `ready_for_review`. والـ PR بيتحوّل لـ «Ready for review» بعد ما الميزة تخلص و`make check` والمراجعة الأمنية يعدّوا، فالـ CI يشتغل ساعتها. وأي workflow جديد لازم ياخد نفس الشرط.
- **الـ branch بيتمسح تلقائي بعد الدمج** (`delete-merged-branch.yml`). والـ branches القديمة بتتمسح بـ `cleanup-merged-branches.yml` يدوي، وهو بيمسح بس الـ branch اللي آخر commit فيه هو نفس آخر commit في PR اندمج.

## التواصل

- مع الـ maintainer بالعربي دائمًا ومن غير استثناء، بما فيها أسئلة `AskUserQuestion` نفسها (السؤال والـ header والخيارات ووصفها) ورسايل الحالة. الإنجليزي بس جوه الكود والـ commits والـ PRs، والـ `Plan/` بالعربي.
- أي سؤال للـ maintainer يبقى من خلال `AskUserQuestion` بخيارات ومعاها trade-offs، ومع توصية واضحة.
- الـ subagents بـ `model: "sonnet"`.
