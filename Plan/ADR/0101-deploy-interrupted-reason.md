# ADR-0101: سبب `DEPLOY_INTERRUPTED` لـ job نشر قطعه سقوط الـ runner (M6.9ب)

- **الحالة:** مقبول. [خطة M6](../M6-Deploy-Targets.md) (جدول الحالات: «بعد التبديل: `DEPLOY_INTERRUPTED`، والنشر الجاي يكتشف»). Sabry قرر 2026-11-06 إضافة السبب في الكود وتبقى الخطة كما هي.
- **التاريخ:** 2026-11-06
- **بيكمّل:** [ADR-0047](0047-job-state-machine-and-reconciliation.md) (`LOST_AFTER_MUTATION`)، و[ADR-0095](0095-deploy-failure-reasons-unknown-state-and-target-check.md) (الحالة `unknown`).

## السياق
- لما الـ runner يقع بعد ما خطوة `Mutating` بدأت، الـ job بينتهي `lost` بالسبب `LOST_AFTER_MUTATION` ومابيتعادش.
- لو الـ job بينشر على هدف، السيرفر بيعلّم الإصدار `unknown` وسيرفراته اللي ماخلصتش. المستخدم محتاج يعرف إن السبب نشر اتقطع، وإن الحل «افحص الهدف» أو نشر جديد، مش إن runner وقع بس.

## القرار
1. الـ job اللي في مواصفاته `Targets` (يعني بينشر على هدف) وانتهى `lost` بعد بدء خطوة تغيّر شيئًا بياخد `DEPLOY_INTERRUPTED` بدل `LOST_AFTER_MUTATION`. باقي الـ jobs بتفضل `LOST_AFTER_MUTATION`.
2. القرار جوه `Modules/Jobs` (`decide`) من مواصفات الـ job المخزّنة، من غير استيراد أي موديول تاني. تعليم الإصدار `unknown` بيفضل زي ما هو في `Environments` (الكنس والـ reconciler).
3. الحالة `lost` وعدم إعادة المحاولة بيفضلوا كما هم.
4. السبب جديد في `FAILURE_REASONS` وفي رسايل الواجهة بالعربي والإنجليزي، وفي وصف `FailureReason` في `openapi.yaml`.

## النتائج
- الاختبار: `TestAKilledRunnerLeavesTheRunningReleaseUnknown` بيتأكد من `DEPLOY_INTERRUPTED` والإصدار `unknown`، و`TestALostAttemptAfterAMutatingStepBeganIsNeverRetried` بيثبت إن job عادي بيفضل `LOST_AFTER_MUTATION`.
- لو الإضافة كانت بلّغت `verified` قبل السقوط، الـ job بيتسمّى `DEPLOY_INTERRUPTED` برضه، والإصدار بيفضل `succeeded` أو بيتنزّل حسب `SettleReleases` الموجودة.
