# ADR-0107: Trusted Types: report-only في التطوير الآن والتفعيل في M7.10

- **الحالة:** مقبول. Sabry جاوب قرار 12 في [`M7-Product-UI.md`](../M7-Product-UI.md) بالخيار ب (2026-11-07). الجزء الأول اتنفّذ في M7.0.
- **التاريخ:** 2026-11-07
- **بيكمّل:** [ADR-0049](0049-web-code-splitting-and-message-catalogs.md) و[`docs/security-model.md`](../../docs/security-model.md) (الـ CSP).

## السياق
الـ CSP الحالية `'self'` بدون inline. توجيه `require-trusted-types-for 'script'` بيمنع مداخل الـ DOM الخطرة (`innerHTML` وغيره) إلا بكائنات موثوقة. ده كان F9 (متوسطة) في مراجعة الهوية. تفعيله من غير فحص بيكسر أي مكتبة بتستخدم مدخل، فنبدأ بالتقرير.

## القرار
1. **M7.0 (اتنفّذ):** لما `DHOW_DEV=true` (وده اللي بيشتغل عليه stack الـ e2e) السيرفر بيضيف `Content-Security-Policy-Report-Only: require-trusted-types-for 'script'` (`Core/Middleware/SecurityHeaders.go`، الخيار `ReportTrustedTypes` من `Config.Dev`). **الـ CSP الفعلية ما اتغيّرتش** ولا الإنتاج: اختبار الـ middleware واختبار في `App` بيثبتوا إن الـ header مش بيظهر من غير `Dev` وإن الـ CSP المفروضة ما فيهاش `trusted-types`.
2. **حارس الـ e2e (`E2E/Guard.ts`):** كل `securitypolicyviolation` (المفروضة والـ report-only) بتتسجّل بنوعها (`disposition`) وبتفشّل `expectClean`، وبتفضل مسجّلة عبر التنقل بين الصفحات الكاملة.
3. **M7.10:** بعد ما كل شاشات M7 تعدّي من غير مخالفة، التوجيه ينتقل للـ CSP الفعلية مع `trusted-types 'none'` (من غير policies) لو التطبيق مابيحتاجش مدخل. لو أي مكتبة احتاجت policy: ADR صريح باسم الـ policy وسببها.
4. **hook المنع القديم** لـ `innerHTML` و`dangerouslySetInnerHTML` بيفضل شغّال.

## الأثر
- أي مخالفة في الـ e2e تفشّل الـ CI من دلوقتي. الـ build الحالي مافيهوش مدخل بنعرفه في كود التطبيق (بحث `innerHTML` و`insertAdjacentHTML` و`eval` و`new Function` و`DOMParser` نضيف)، بس مكتبات الـ bundle ممكن تظهر مخالفة، وتتعالج وقتها.
- مفيش `report-uri`: الإبلاغ عن طريق حدث المتصفح في الاختبارات بس.

## البدائل اللي اترفضت
- **تفعيل مباشر:** ممكن يكسر مكتبة من غير ما نعرف.
- **ما نعملش حاجة:** F9 تفضل مفتوحة.
