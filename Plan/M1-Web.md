# خطة M1: الواجهة (`apps/web`)

**الحالة:** اتنفّذت على فرع `m1/web-shell`، ومستنية مراجعة Sabry · **التاريخ:** 2026-10-27

## الهدف
أول نسخة من الواجهة بتشتغل فوق السيرفر اللي اندمج في #2. الهيكل وقواعده بيتثبّتوا من دلوقتي، والشاشات نفسها هتتبني بعدين. يعني M1 فيه الـ shell والتنقّل والترجمة والاتصال بالـ API بس، ومفيش features.

## المراجع
- الـ Design System «داو» (الـ artifact الخاص): 37 component و32 أيقونة والـ tokens. [ADR-0029](ADR/0029-design-system-aurora-dusk.md).
- [ADR-0021](ADR/0021-arabic-first-ui.md): عربي أولاً، RTL، logical properties، ومفيش نصوص في الكود.
- [ADR-0022](ADR/0022-navigation-model.md): مساحة واحدة، وslide-over، و⌘K، وكل حالة ليها رابط.
- [ADR-0023](ADR/0023-ui-stack.md): React + Vite + TS + Tailwind + Radix + Motion + TanStack Query، وclient متولّد من OpenAPI.
- قواعد Sabry: `Stacks/React.md` و`Stacks/NextJS.md` (نفس الأسلوب، من غير أجزاء Next.js الخاصة بالسيرفر).

## القرارات (وافق عليها Sabry في 2026-10-27، [ADR-0031](ADR/0031-web-structure-and-ui-components.md))

| # | الموضوع | الاقتراح | السبب |
|---|---|---|---|
| 1 | الهيكل | Modules زي السيرفر: `Src/Modules/<Module>/` وجواه Pages وComponents وHooks وApi وStore وTypes. وجنبهم `Src/App` و`Src/Core` و`Src/I18n` و`Src/Styles` | نفس الشكل في السيرفر والواجهة، وكل module بيتفصل لوحده. ده بيسهّل `ee/` بعدين |
| 2 | التسمية | PascalCase للفولدرات والملفات والـ components والـ state والـ handlers. والـ hooks بتبدأ بـ `use` صغيرة | `UseX` اللي في NextJS.md بتعطّل `eslint-plugin-react-hooks`، لأنه بيتعرّف على الـ hook من `use` الصغيرة. وده اللي مسك bug الـ Rollback في Deploy Center |
| 3 | الـ components | `Src/Core/Ui/` فيه الـ 37 component من الـ Design System، مكتوبين بإيدينا فوق Radix للحاجات التفاعلية (Dialog وTabs وSwitch وCheckbox وTooltip). من غير shadcn | الـ Design System بتاعنا مختلف كتير عن شكل shadcn، فنسخه وتعديله هيبقى شغل زيادة من غير فايدة |
| 4 | الحالة | TanStack Query للبيانات من السيرفر، وZustand لحالة الواجهة المشتركة، وReact Hook Form مع zod للفورمات | زي React.md بالظبط |
| 5 | التنقّل | TanStack Router، والـ slide-over مربوط بـ search param (`?panel=...`) | اتقرر قبل كده، والـ search params عنده typed |
| 6 | الـ API | `openapi-typescript` + `openapi-fetch` من `packages/openapi/openapi.yaml` نفسه، والملف المتولّد ممنوع يتعدّل | spec-first. لو السيرفر غيّر الـ envelope، الـ typecheck هيكسر فورًا |
| 7 | الترجمة | `Src/I18n/Locales/Ar.ts` و`En.ts`، والعربي هو الافتراضي، و`dir` و`lang` على `<html>` بيتغيّروا مع اللغة | ADR-0021. NextJS.md بيقول الافتراضي إنجليزي، بس قرار داو هو العربي |
| 8 | الخطوط | Poppins وIBM Plex Sans Arabic وIBM Plex Mono بـ `@fontsource`، والوزن 600 بالكتير | Self-hosted، ومفيش أي طلب لـ Google Fonts |
| 9 | التسليم | السيرفر بيخدم `dist` بـ `go:embed`. الـ Makefile بيبني الواجهة وينسخها في `apps/server/internal/App/WebDist/`، والفولدر ده في `.gitignore` ومعاه ملف placeholder | binary واحد بيتنصّب عند العميل |
| 10 | الشغل القديم | نبدأ من الأول، وناخد من worktree `m1/web` القديم الإعدادات بس (Vite وESLint وVitest وسكريبت حجم الـ bundle)، بعد ما نراجعها | الـ components القديمة مبنية على Design System اترفض (Glass وAuroraSky وخط Alexandria) |

## المهام (كل مهمة تقدر تتنفّذ لوحدها)

### 1. Scaffold
- `apps/web` بـ pnpm 10 وNode 24، وVite وReact 19 وTS strict.
- كل الاعتماديات بنسخ مثبّتة بالظبط، و`pnpm-lock.yaml` متولّد ومحدش يعدّله بإيده.
- ESLint بـ `--max-warnings 0`، وفيه react-hooks وjsx-a11y، و`max-lines` 500 و`max-lines-per-function` 100، ومنع `any` و`console`.
- Vitest مع Testing Library.
- **الملفات:** `apps/web/{package.json,vite.config.ts,tsconfig.json,eslint.config.js,index.html}`.

### 2. الـ Tokens والـ Styles
- `Src/Styles/Tokens.css` متولّد من `tokens.json` بتاع الـ Design System بسكريبت، والملف المتولّد ممنوع يتعدّل.
- `Src/Styles/Globals.css` فيه `@import "tailwindcss"` و`@theme`. مفيش `tailwind.config`، ومفيش ألوان مكتوبة في الكود.
- الـ gradients بتتعكس في RTL بالـ CSS vars، زي `bundle.css`.

### 3. `Core/Ui`
- الـ 37 component، كل واحد في ملف لوحده وليه اختبار سلوك (keyboard وfocus وaria).
- الأولوية لأجزاء الـ shell: IconRail وTopBar وButton وIconButton وSlideOver وCommandPalette وTooltip وEmptyState وBanner وSkeleton.
- الباقي هيتبني مع الشاشة اللي محتاجاه، يعني في M2.

### 4. الـ Shell (`Src/App`)
- `App.tsx` و`Router.tsx` و`Providers.tsx`.
- الـ layout: icon rail بعرض 72px، وtop bar، ومساحة شغل واحدة.
- slide-over بيتفتح من حافة الـ inline-end، ومربوط بالـ URL، والـ focus بيتحبس جواه، وEsc بيقفله.
- ⌘K وCtrl+K بيفتحوا الـ palette.
- `prefers-reduced-motion` بيوقّف الحركة.

### 5. الترجمة واللغة
- `I18n/Locales/{Ar,En}.ts` بنفس المفاتيح، واختبار بيتأكد إن المفاتيح متطابقة في الملفين.
- `useT()` للترجمة، وزرار لتغيير اللغة، والاختيار بيتحفظ في `localStorage` من غير وميض.
- `Accept-Language` بيتبعت مع كل طلب للـ API، فرسايل الأخطاء بترجع بنفس اللغة.

### 6. الـ API
- `Src/Core/Api/Schema.d.ts` متولّد، و`Client.ts` فيه `openapi-fetch` وبيفك الـ envelope.
- الأخطاء بتتحوّل لـ `TApiError` فيه `Code` و`Message`.
- `Modules/System`: `useMeta()` و`useReadiness()`. لو `SetupRequired` بـ true، بيظهر Banner بيقول إن الإعداد الأولاني لسه ماخلصش.

### 7. Module أول: `System`
- صفحة الـ workspace فيها EmptyState، وslide-over لحالة السيرفر (الـ checks والنسخة والـ commit).
- 3 حالات لكل شاشة: Loading (Skeleton)، وError (InlineAlert)، وEmpty.

### 8. التسليم من السيرفر
- `make web` بيعمل `pnpm install --frozen-lockfile` و`pnpm build`، وبعدين بينسخ الـ build لـ `App/WebDist`.
- `App/Web.go` بيخدم الملفات بـ `go:embed`، ولو المسار مش موجود بيرجّع `index.html` عشان الـ SPA.
- الـ assets ليها cache طويل وimmutable، و`index.html` بـ `no-store`.
- الـ CSP بيفضل `'self'`، ومفيش inline scripts. سكريبت الثيم بيبقى ملف لوحده.

### 9. الـ Hooks (قاعدة جديدة يبقى ليها hook واختبار)
- مكان ملفات `apps/web/Src`: `App` أو `Core/<X>` أو `I18n` أو `Styles` أو `Modules/<Name>/...`. والأسماء PascalCase، ما عدا الـ hooks (`useX.ts`) وملفات الإعدادات.
- اتجاه الـ imports: module ميعملش import لـ module تاني، و`Core` ميعملش import لـ `Modules`.
- أي نص عربي أو إنجليزي جوه JSX ممنوع، ومكانه الترجمة. الاستثناءات: `Locales/` والاختبارات.
- القواعد الحالية زي ما هي: logical properties، و`any`، و`console`، وI/T/E.

### 10. الـ CI
- job اسمه `web` بـ pnpm cache، بيشغّل `lint` و`typecheck` و`test` و`build` و`size`، ومن غير `hashFiles` على مستوى الـ job.
- `javascript-typescript` يتضاف لـ CodeQL.
- ميزانية الحجم: الـ JS الأولاني 128 KB gzip كحد مفروض والهدف 120 أو أقل، وكل chunk كسول 60 KB (35 KB لعارض السجل)، والـ CSS أقل من 30 KB gzip ([ADR-0103](ADR/0103-web-size-budgets.md)).

### 11. التحقق قبل الـ push
- `pnpm lint && pnpm typecheck && pnpm test && pnpm build && pnpm size`.
- `make check` و`make build`، والسيرفر بيخدم الواجهة فعلًا.
- Playwright (موجود في البيئة): صور للـ shell بالعربي RTL والإنجليزي LTR، وتجربة فتح الـ slide-over بالـ URL، وتجربة ⌘K.

## Edge cases
- الـ API مش متاح: Banner خطأ، والـ shell فاضل شغّال.
- لغة مش مدعومة في `Accept-Language`: العربي هو اللي يشتغل.
- رابط slide-over لحاجة مش موجودة: الـ panel ميتفتحش، وبيظهر Toast.
- `localStorage` مقفول (private mode): الإعدادات الافتراضية تشتغل من غير crash.
- اسم مشروع طويل أو مكتوب بالإنجليزي جوه نص عربي: بيتعزل بـ `<bdi>`.

## اللي اتنفّذ فعلًا

**اختلافات عن الخطة (وليه):**
- **الترتيب:** الترجمة (5) والـ API (6) اتعملوا قبل الـ shell (4)، لأن الـ shell مفيهوش نص مكتوب في الكود، ومحتاج `useT` والـ client من الأول.
- **الاعتماديات:** `react-hook-form` و`zod` مش متضافين، لأن M1 مفيهوش فورم. وRadix فيه `Dialog` و`Tooltip` بس، و`Tabs` و`Switch` و`Checkbox` يتضافوا مع أول شاشة بتستخدمهم.
- **TypeScript 6.0.3 مش 7.0.2:** `typescript-eslint` بيقبل `<6.1.0` و`openapi-typescript` بيقبل `5.x`. ESLint 10 شغّال مع `eslint-plugin-jsx-a11y` (بيعلن دعم لحد 9) وكل قواعده بتشتغل، وده متجرّب.
- **الـ overlay:** `Dialog.Overlay` من Radix بيركّب `react-remove-scroll`، وده بيحقن `<style>` والـ CSP (`style-src 'self'`) بترفضه وبتظهر أخطاء في الـ console. فالـ backdrop بتاعنا عنصر عادي، والـ Dialog content (focus trap وEsc وaria) فاضل من Radix. اختبارات `SlideOver` و`CommandPalette` بتتأكد إن مفيش `<style>` اتحقن، والتحقق في المتصفح على السيرفر الحقيقي طلع من غير أي خطأ.
- **الـ backdrop blur:** الـ Design System بيحدد blur خفيف (8px) ورا الـ overlay بس، فاتطبّق زي ما هو. مفيش blur على أي كارت.
- **الخطوط:** Poppins وIBM Plex Sans Arabic وIBM Plex Mono بوزن 400 و500 بس (الـ wordmark SVG). مفيش وزن 300 أو 600 لحد ما حد يحتاجهم.
- **`WebDist`:** فيه `.keep` بدل `index.html` placeholder، فلو مفيش build كل المسارات اللي مش `/api/` بترجع 404 بنفس الـ envelope. كده اختبارات السيرفر القديمة فاضلة صح، واختبارات `App/Web_test.go` بتشتغل على filesystem وهمي فمش بتتأثر بـ `make web`.
- **الـ CI:** pnpm بييجي من `corepack` ومن حقل `packageManager`، بدل `pnpm/action-setup`، لأن SHA الـ action مش متاح للتثبيت في الجلسة دي. الـ cache من `actions/setup-node`.
- **hooks زيادة:** `Modules` ممنوع يستورد `App`، وأي ملف اسمه `UseX` (بـ U كبيرة) مرفوض، وأي مسار `apps/web/src/` بحروف صغيرة مرفوض.

**أهداف الـ build:**
- `make check` و`make build` Go بس، من غير الواجهة، وبيشتغلوا على `WebDist` الفاضي. الـ CI (job `go`) بيشغّلهم زي ما هم.
- job `web` في الـ CI بيشغّل `pnpm` مباشرة: install وفحص الـ drift للـ API والـ tokens وlint وtypecheck وtest وbuild وsize.
- `make web` بيبني ويكوبي في `WebDist`، و`make release` بيعمل `web` وبعده `build`. ده للإصدارات ولتشغيل الواجهة محليًا، ومش داخل في الـ CI لسه.

**الحجم (gzip):** الـ JS الأولاني حوالي 149 KB من 180، والـ CSS حوالي 6 KB من 30. حركة Motion (`domAnimation`) بتتحمّل lazy مع أول overlay.

## مفتوح
- مراجعة Sabry للفرع.
- branch protection على `main`.
- مسح worktree `m1/web` القديم (Sabry هو اللي هيمسحه).
- هل نضيف `make release` للـ CI؟

## تعديل بعد مراجعة Sabry (2026-10-27)
- **الشرائح المتراكبة** بتدخل من اليمين في العربي، و**نظام الثيمات** (4 ثيمات للنظام + ثيم مخصص)، والتفاصيل في [ADR-0032](ADR/0032-stacked-slides-and-themes.md).
- **النموذج المرجعي:** artifact «شرائح داو» (`https://claude.ai/artifact/S9pfYHq51uCsbTTzfw4UQ8`). أي سلوك فيه هو المطلوب في الكود.
- **في M1:**
  - الـ slide-over الواحد هيتبدّل بـ `SlideStack`.
  - حالة السيرفر وتفاصيل كل فحص هيتعرضوا كشريحتين فوق بعض.
  - المظهر ومحرر الثيم هيتعرضوا كشريحتين فوق بعض.
  - الإشعارات شريحة واحدة.

### اللي اتنفّذ فعلًا من التعديل ده
- **`Core/Ui/SlideStack`** بدل `SlideOver`. الـ stack في search param اسمه `stack` (مثلًا `?stack=system-status,check-database`)، وبيتفحص على registry للـ frames في `App/Panels.ts`. أي frame مش معروف أو متكرر، أو أكتر من 6، أو محرر ثيم تاني: الـ stack بيتشال وبيظهر الـ Toast الموجود. رابط `?panel=` القديم مابقاش مدعوم.
- **الـ frames:** `system-status` و`check-database` و`check-migrations` و`notifications` و`appearance` و`theme-edit` (جديد) و`theme-edit-<id>` (ثيم مخصص موجود).
- **الثيمات:** `Src/Styles/Themes.json` فيه الـ 25 token للأربع ثيمات، و`GenTokens.mjs` بيولّد `:root` للافتراضي (بحر الليل) وبلوك `[data-theme]` لكل ثيم. الـ `frame` والـ `bubble` والتدرّجات متشتقة بـ `color-mix`. المحرك في `Core/Theme` (تحقق صارم وتخزين بـ try/catch وstore بـ Zustand)، و`public/ThemeInit.js` بيطبّق الثيم قبل أول رسم.
- **المحرر:** `Modules/Appearance`، ومحرر الثيم بيتحمّل lazy عشان `react-hook-form` وzod ما يزوّدوش حجم الـ JS الأولاني (حوالي 160 KB من 180).
- **اختلافات:**
  - نقطة التحويل للموبايل بقت 721px (كانت 768px) عشان تطابق الـ 720px في النموذج، وعرض الشريحة 500px (كان 480px).
  - `@hookform/resolvers` اتضاف مع `react-hook-form` وzod عشان يربط الـ schema بالفورم. النسخ اللي عدّى عليها أكتر من 10 أيام: `react-hook-form` 7.90.0 وzod 4.6.7 و`@hookform/resolvers` 5.9.2.
  - الشريحة اللي تحت `inert` والضغط عليها بيتم من زرار شفاف جنبها، لأن `inert` بيمنع أحداث الماوس. الـ Tab بيلف جوه الشريحة اللي فوق بمستمع خاص، لأن Radix بيحسب الشرايح الـ inert في حواف الـ focus trap.
  - الـ focus بيرجع للزرار الأصلي بعد ما حركة الخروج تخلص (450ms)، عشان مايتحطش على عنصر جوه `aria-hidden`.
  - الـ rail فوق الـ backdrop وبيفضل شغّال زي النموذج.

