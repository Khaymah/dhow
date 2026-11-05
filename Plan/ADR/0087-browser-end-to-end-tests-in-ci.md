# ADR-0087: اختبارات المتصفح من أول لآخر في الـ CI

- **الحالة:** مقبول. قرار Sabry يوم 2026-11-05: نثبّت اختبارات Playwright في الـ CI بدل الـ script المحلي.
- **التاريخ:** 2026-11-05
- **بيكمّل:** [خطة M5](../M5-Secrets-Environments.md) (قبول M5.7 وجدول «إيه بيشتغل فين»)، و[خطة M4](../M4-Pipelines.md) (نفس السطر في جدول الاختبارات)، و[ADR-0086](0086-deployment-approvals.md) (الموافقات).

## السياق
- خطة M4 وخطة M5 بيقولوا إن Playwright على سيرفر حقيقي وForgejo بيشتغل مع أي PR بيلمس `apps/web`. لكن في M3 وM4 اتعمل script محلي بس، والنتيجة اتسجلت في `Plan/Progress`، فمفيش حاجة بتمسك أي كسر بعد كده.
- `deploy/m4-demo` بيشغّل فعلًا Postgres والسيرفر وForgejo وrunner في Docker Compose، وبيشتغل في `demo.yml`.
- اختبارات Vitest في `apps/web` بتشتغل على backend وهمي، فمش بتشوف الـ CSP الحقيقية ولا الـ cookies ولا البوابة اللي في السيرفر.

## القرار
1. **الـ stack:** `deploy/e2e/run.sh` بيبني على ملفات `deploy/m4-demo` (نفس الـ compose ونفس صورة Forgejo بالـ digest، والدوال المشتركة اتنقلت لـ `deploy/m4-demo/project.sh`)، وبيشغّل Postgres والسيرفر بالواجهة المبنية جوه الـ binary (`make web` الأول) وForgejo وrunner بـ Podman rootless وصورة السحب `dhow-git`. الـ runner الوهمي مش كفاية: من M4.8 التنفيذ على مستودع Git عادي بيبدأ بـ job بيقرا ملف الخطوات ويسلّم كائنات Git بتثبت إن الملف في الـ commit، والـ runner الوهمي مابيسحبش كود. فالاختبار محتاج cgroup v2 زي `m4-demo`.
2. **الإعداد بالـ API:** الـ script بيعمل الـ Owner والمؤسسة والـ runner ومستودع Forgejo والمشروع والربط والـ webhook. واللي الاختبار بيتأكد منه (البيئة وقواعدها وسرها والموافقة) بيتعمل من الواجهة.
3. **الاختبارات:** `apps/web/E2E/*.spec.ts` بـ `@playwright/test` بنسخة مثبّتة بالظبط (نفس نسخة Chromium الموجودة في البيئة). الـ push بيتعمل من `deploy/e2e/push.sh` جوه حاوية `tools`، والاختبار بيناديه كـ argv من غير shell.
4. **في كل صفحة:** مفيش console error، ولا مخالفة CSP (`securitypolicyviolation`)، ولا `<style>` اتحقن، وقيمة السر مش في الـ URL ولا الـ history ولا `localStorage` ولا `sessionStorage` ولا الـ DOM. بالعربي والإنجليزي وعلى 1440 و390.
5. **الـ CI:** job `e2e` في `demo.yml` (نفس الـ Ubuntu 26.04 اللي فيه Podman)، و`deploy/ci/demo-areas.sh` بيقرر: تغيير في الواجهة لوحدها بيشغّل `e2e` بس، وتغيير في السيرفر أو `m4-demo` بيشغّل الاتنين، وتغيير في الـ runner بيشغّل الـ demos بس. ومش على الـ draft (نفس شرط `!github.event.pull_request.draft`)، وبيشتغل كمان ليلي ومن `workflow_dispatch`. والـ trace والصور بيترفعوا كـ artifact، وlogs الـ stack بتطلع في الـ log لو فشل.
6. **محليًا:** `make e2e`.

## البدائل اللي اترفضت
- **script محلي زي M3 وM4:** أسرع، لكن مفيش حاجة بتمسك الكسر بعد كده.
- **السيرفر والـ runner على الـ host من غير Compose:** أخف، لكن stack تاني لازم يتصان جنب `m4-demo`، وForgejo محتاج يوصل للسيرفر من حاوية على كل حال.
- **الـ runner الوهمي:** أخف ومش محتاج cgroup v2، لكنه مابيقدرش يعمل job قراية الملف، فالتنفيذ بيفضل مستني.

## النتايج
- اعتمادية جديدة للتطوير بس: `@playwright/test`، ومش بتدخل الـ bundle.
- job جديد في الـ CI حوالي 15 دقيقة، بس لما الأجزاء دي تتغيّر. ومحليًا `make e2e` محتاج Docker بـ cgroup v2.
- `deploy/m5-demo` في M5.9 هيستخدم نفس الإعداد.

## المراجعة الأمنية
- مفيش Critical ولا High.
- **(MEDIUM، اتصلّح)** فحص الأسرار في الـ argv (`secrets-argv-test.sh`) كان بيقرا `m4-demo/run.sh` بس، والدوال اتنقلت لـ `project.sh`. بقى بيقرا `project.sh` و`deploy/e2e/run.sh` و`push.sh` كمان، وبيتأكد إن كلمة سر Forgejo بتوصل لـ curl على stdin ولـ git من `GIT_ASKPASS`.
- **(MEDIUM، اتصلّح)** الـ e2e ماكانش بيعمل فحص التسريب. دلوقتي `leak-sweep.sh` بيشتغل بعد الاختبارات، وبيدوّر كمان على قيمة سر البيئة (عشوائية لكل تشغيل) وكوكي الجلسة والـ CSRF وكلمة السر وتوكن التسجيل.
- **(LOW، اتصلّح)** تغيير في `apps/runner` بقى بيشغّل الـ e2e كمان.
- **(LOW، مقبول)** الـ traces والصور اللي بتترفع فيها كلمات سر وكوكيز وأكواد TOTP بتاعة stack بيتعمل ويتمسح في نفس التشغيل، ومفيش فيها سر حقيقي. ممنوع نمرّر أي توكن حقيقي للاختبارات.
