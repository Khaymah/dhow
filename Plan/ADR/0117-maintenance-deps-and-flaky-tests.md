# ADR-0117: تحديث الاعتماديات وأسباب الاختبارات المتقلبة

- **الحالة:** مقبول.
- **التاريخ:** 2026-11-14
- **بيكمّل:** [ADR-0111](0111-org-event-stream-and-deploy-board.md) (قناة الـ canary) و[ADR-0058](0058-chaos-suite-and-final-cancellation.md) (إيقاف الـ runner).

## السياق
خمس طلبات من Dependabot فشلت، واختباران فشلا في أكثر من طلب، وفشل workflow الـ Fuzz الليلي ثلاث ليالٍ (10 و11 و12 نوفمبر). الفحص أظهر أن كل فشل له سبب واحد واضح، وأن المشكلة في الكود أو الاختبار وليست في البنية التحتية.

## القرار
1. **`github/codeql-action`:** الخطوات `init` و`autobuild` و`analyze` لازم تعمل بنفس النسخة، فصارت الثلاث على `v4.39.3` بالـ SHA الكامل للـ commit. وفي `dependabot.yml` مجموعة `codeql-action` تجمعها في طلب واحد.
2. **`postgres:18`:** نفس الـ digest في `deploy/compose/compose.yaml` ونسخته المضمّنة في `Core/Installer/Assets/compose.yaml` (يفحصهما `TestEmbeddedAssetsMatchTheDeployFiles`) و`deploy/bench/compose.yaml`.
3. **`@types/node` 26:** صورة الواجهة تعمل بـ Node 26. والنوع الجديد يجعل `export({format: 'pem'})` يرجع `string`، فاتشال `.toString()` الزائد في `E2E/HostKey.ts`.
4. **`TestTheCanaryKeepsAHealthyListenerAliveAndIsNeverAnEvent`:** الـ `Hub` كان يبدأ «سليم» (`down` صفر) قبل أن يتصل المستمع. فالاختبار كان يلتقط الـ epoch قبل أول `Restart()`، ثم يتغير في أول نبضة. الحل في الكود: `NewService` يضع الـ hub غير سليم إذا كان فيه مستمع، فلا يصير سليمًا إلا بعد أول `LISTEN` ناجح. الأثر في الإنتاج: `/health/ready` ما عاد يقول سليم قبل الاتصال.
5. **`TestAnUnreportableOldAttemptDoesNotStopTheRunnerFromWorking`:** `Run` كان يطلق `RecoverLater` في goroutine لا ينتظرها، فتكتب في `journal` بعد رجوع `Run`، وتتعارض مع حذف مجلد الحالة (`directory not empty`). الحل: `Run` يلغي هذه الـ goroutine وينتظرها قبل الرجوع، والاختبار يتأكد أن مجلد الحالة ثابت بعد `Stop`.
6. **Fuzz `FuzzAllowedRefs`:** `pathglob.MatchPattern` كان يحوّل الاسم إلى `[]rune`، فيصير كل بايت غير صالح `U+FFFD`، فيطابق الاسم `"\xbd"` النمط `"�"`. الحل: أي اسم غير صالح UTF-8 لا يطابق شيئًا (يشمل كل من يستخدم `pathglob`: بوابة البيئات ورفع الـ Ingest وشروط الـ pipeline).
7. **Fuzz `FuzzReportLine`:** الخاصية «الحدث لا يكبر عن سطره» كانت خاطئة: `encoding/json` يحوّل كل بايت غير صالح إلى 3 بايتات. الكود سليم (حقول proto لازم تكون UTF-8)، فاتصحّحت الخاصية إلى «كل حقل UTF-8 صالح والحجم لا يزيد على 3 أضعاف السطر».
8. مدخلات الـ fuzz الفاشلة محفوظة في `testdata/fuzz` فتعمل كل مرة مع `go test` العادي.

## البدائل اللي اترفضت
- إعادة المحاولة أو تخطي الاختبارين: تخفي الخطأ الحقيقي (الاثنان كانا عيبين في الكود).
- رفع `SilenceLimit` في اختبار الـ canary: لا يحل السبب (الـ epoch يتغير في أول نبضة وليس عند الصمت).

## اللي لسه
كل `pull_request` من Dependabot يلزم أن يمر عبر مجموعة `codeql-action` من الآن، ولا يوجد ما يمنع تحديثًا يدويًا لخطوة واحدة.
