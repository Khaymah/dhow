# ADR-0087: اختبارات المتصفح من أول لآخر في الـ CI

- **الحالة:** مقبول. قرار Sabry يوم 2026-11-05: نثبّت اختبارات Playwright في الـ CI بدل الـ script المحلي.
- **التاريخ:** 2026-11-05
- **بيكمّل:** [خطة M5](../M5-Secrets-Environments.md) (قبول M5.7 وجدول «إيه بيشتغل فين»)، و[خطة M4](../M4-Pipelines.md) (نفس السطر في جدول الاختبارات)، و[ADR-0086](0086-deployment-approvals.md) (الموافقات).

## السياق
- خطة M4 وخطة M5 بيقولوا إن Playwright على سيرفر حقيقي وForgejo بيشتغل مع أي PR بيلمس `apps/web`. لكن في M3 وM4 اتعمل script محلي بس، والنتيجة اتسجلت في `Plan/Progress`، فمفيش حاجة بتمسك أي كسر بعد كده.
- `deploy/m4-demo` بيشغّل فعلًا Postgres والسيرفر وForgejo وrunner في Docker Compose، وبيشتغل في `demo.yml`.
- اختبارات Vitest في `apps/web` بتشتغل على backend وهمي، فمش بتشوف الـ CSP الحقيقية ولا الـ cookies ولا البوابة اللي في السيرفر.

## القرار
1. **الـ stack:** `deploy/e2e/run.sh` بيبني على ملفات `deploy/m4-demo` (نفس الـ compose ونفس صورة Forgejo بالـ digest)، وبيشغّل Postgres والسيرفر بالواجهة المبنية جوه الـ binary (`make web` الأول) وForgejo والـ runner الوهمي (`dhowtest`). الـ runner الوهمي كفاية لأن البوابة والموافقة في السيرفر، وكل خطوة فيه بتطبع سطر وبتنجح.
2. **الإعداد بالـ API:** الـ script بيعمل الـ Owner والمؤسسة والـ runner ومستودع Forgejo والمشروع والربط والـ webhook. واللي الاختبار بيتأكد منه (البيئة وقواعدها وسرها والموافقة) بيتعمل من الواجهة.
3. **الاختبارات:** `apps/web/E2E/*.spec.ts` بـ `@playwright/test` بنسخة مثبّتة بالظبط (نفس نسخة Chromium الموجودة في البيئة). الـ push بيتعمل من `deploy/e2e/push.sh` جوه حاوية `tools`، والاختبار بيناديه كـ argv من غير shell.
4. **في كل صفحة:** مفيش console error، ولا مخالفة CSP (`securitypolicyviolation`)، ولا `<style>` اتحقن، وقيمة السر مش في الـ URL ولا الـ history ولا `localStorage` ولا `sessionStorage` ولا الـ DOM. بالعربي والإنجليزي وعلى 1440 و390.
5. **الـ CI:** job `e2e` بيشتغل لما `apps/web` أو `apps/server` أو `packages/openapi` أو `deploy/e2e` يتغيّروا، ومش على الـ draft (نفس شرط `!github.event.pull_request.draft`). ولو فشل بيرفع الـ trace والصور وlogs الـ stack كـ artifact.
6. **محليًا:** `make e2e`.

## البدائل اللي اترفضت
- **script محلي زي M3 وM4:** أسرع، لكن مفيش حاجة بتمسك الكسر بعد كده.
- **السيرفر والـ runner على الـ host من غير Compose:** أخف، لكن stack تاني لازم يتصان جنب `m4-demo`، وForgejo محتاج يوصل للسيرفر من حاوية على كل حال.
- **runner بـ Podman:** أبطأ ومحتاج cgroup v2، ومش بيضيف حاجة للواجهة. ده شغل `m4-demo` و`m5-demo`.

## النتايج
- اعتمادية جديدة للتطوير بس: `@playwright/test`، ومش بتدخل الـ bundle.
- job جديد في الـ CI حوالي 10 دقايق، بس لما الأجزاء دي تتغيّر.
- `deploy/m5-demo` في M5.9 هيستخدم نفس الإعداد.
