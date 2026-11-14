# ADR-0115: Trusted Types مفروضة في الـ CSP لكل الأوضاع (M7.10)

- **الحالة:** مقبول. بينفّذ البند 3 في [ADR-0107](0107-trusted-types-report-only-then-enforce.md) بعد ما اتحقق شرطه (صفر مخالفة).
- **التاريخ:** 2026-11-14
- **بيكمّل:** [`docs/security-model.md`](../../docs/security-model.md) (الـ CSP) والنتيجة F9 في `docs/security/auth-review-checklist.md`.

## السياق
ADR-0107 قرر: الترويسة `Content-Security-Policy-Report-Only: require-trusted-types-for 'script'` في وضع التطوير والـ e2e من M7.0، وحارس الـ e2e يفشّل على أي `securitypolicyviolation`، والتفعيل في M7.10 لو صفر مخالفة، ولو مكتبة احتاجت policy يبقى ADR صريح.

أدلة الصفر (اتجمعت في M7.10):
1. **الـ e2e:** كل spec بيعدّي على `expectClean` اللي بيفشّل على أي مخالفة (المفروضة والـ report-only). عمل Demo الليلي على `main` نجح في 11 و12 و13 و14 نوفمبر، يعني بعد ما كل شاشات M7 (M7.1 لـ M7.9) اندمجت وفي كل واحدة منها الـ Guard شغّال.
2. **بحث في الكود:** مفيش في `Src` ولا `E2E` أي من `innerHTML` و`outerHTML` (كتابة) و`insertAdjacentHTML` و`document.write` و`createContextualFragment` و`srcdoc` و`DOMParser` و`new Function` و`eval` و`new Worker` و`importScripts` ولا إنشاء عنصر `<script>`. القراءة `document.documentElement.outerHTML` في فحص التسريب مش مدخل.
3. **بحث في الـ bundle المبني:** كل chunks `dist/assets` (219 كسول + الأولاني). المداخل ظاهرة في chunk الأولاني بس، وكلها من `react-dom`: `innerHTML` (5 مرات) و`createElement('script')` (3 مرات). دي مسارات React لـ `dangerouslySetInnerHTML` ولعنصر `<script>` في الـ JSX، وكلاهما ممنوع بالـ hook ومش بيتنفذ في التطبيق. باقي الـ chunks (Radix والـ virtualizer وzod وlean-qr وmotion) مفيهاش مدخل.
4. **`ThemeInit.js`:** ملف ثابت من نفس الأصل، مش بيكتب HTML.

## القرار
1. **الـ CSP الفعلية** في `Core/Middleware/SecurityHeaders.go` بقت: `... form-action 'self'; require-trusted-types-for 'script'; trusted-types 'none'` لكل الأوضاع (التطوير والإنتاج، http وhttps). `trusted-types 'none'` معناها مفيش policy يتعمل خالص (حتى `default`)، فأي مدخل بيرمي `TypeError` بدل ما يتنفذ. فمفيش policy مسمّى ومفيش ADR لـ policy لأن مفيش مكتبة محتاجة واحد.
2. **ترويسة الـ report-only اتشالت**، وخيار `ReportTrustedTypes` اتشال من `SecurityOptions` (و`Config.Dev` مابقاش بيأثر على الـ CSP).
3. **الاختبارات:**
   - `Middleware_test.go`: الـ CSP فيها التوجيهين، ومفيش report-only.
   - `App/TrustedTypes_test.go`: التوجيهين موجودين على صفحة وSPA fallback وملف asset ومسار API، في وضع التطوير والإنتاج وعلى http وhttps، ومفيش `unsafe-inline`.
   - `E2E/Guard.ts`: `expectClean` بيتأكد إن كل استجابة تنقّل فيها التوجيهين في الـ CSP المفروضة، وبيفشّل على أي `securitypolicyviolation` (disposition `enforce` أو `report`).
4. **hook المنع** لـ `innerHTML` و`dangerouslySetInnerHTML` بيفضل شغّال.
5. **لو مكتبة جديدة احتاجت مدخل:** تفشل في الـ e2e فورًا. الحل إما نشيل المدخل (الأفضل)، أو ADR صريح باسم الـ policy وسببها بيستبدل `trusted-types 'none'` بقايمة أسماء.

## الأثر
- النتيجة F9 (متوسطة) في مراجعة الهوية اتقفلت. بيتحدّث `docs/security-model.md` والـ checklist بالنسختين.
- المتصفحات اللي ما بتدعمش التوجيه بتتجاهله ومش بتتأثر، وبتفضل حمايتها من `script-src 'self'`. المتصفحات اللي بتدعمه بتحميه من مدخل DOM جديد حتى لو فات الـ review.
- الـ e2e بيتشغّل على Chromium بس. لو متصفح تاني اكتشف مدخل ما بيظهرش في Chromium، فالفحص الثابت للـ bundle (البند 3 فوق) هو الشبكة التانية. أي ترقية لـ `react-dom` تتراجع على الفحص ده.
- الملفات الثابتة الجديدة في `public/` ما تكتبش HTML من نص.

## البدائل اللي اترفضت
- **نفضل report-only:** مابيحمّيش فعلًا، وF9 تفضل مفتوحة.
- **policy اسمها `default` بتعقّم:** بتفتح المدخل لكل مكتبة وبتخفي المشكلة بدل ما نشيلها.
- **`trusted-types` من غير `'none'`:** بتسمح بأي policy، فأي سكربت بيحقن policy يتجاوز الحماية.
