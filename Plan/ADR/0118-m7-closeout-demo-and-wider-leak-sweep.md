# ADR-0118: إغلاق M7: demo الإشعارات والبحث الموسّع عن التسريب (M7.11)

- **الحالة:** مقبول.
- **التاريخ:** 2026-11-14
- **بيكمّل:** [ADR-0108](0108-notifications-foundation.md) (الـ outbox وat-least-once) و[ADR-0110](0110-notifications-chat-channels.md) (الـ receiver المحلي ومضيفات ثابتة) و[ADR-0111](0111-org-event-stream-and-deploy-board.md) (بث المؤسسة) و[ADR-0109](0109-log-viewer-virtualized.md) (تنزيل السجل).

## السياق
شرط خلاص M7 بيطلب إن `deploy/m7-demo/run.sh` يعدّي في `demo.yml` وإن فحص التسريب يلاقي الحالات المزروعة بس. ADR-0110 قرر إن الاختبارات ما بتغيّرش مضيف الخدمة في الإنتاج وبتحوّل الاتصال بس عن طريق `Options`، لكن الـ demo بيشغّل الـ binary الحقيقي من غير نقاط حقن.

## القرار
1. **الأسماء الحقيقية، والـ receiver في الشبكة.** الـ receiver بيشتغل كخدمة `notify` في compose، وأسماء `hooks.slack.com` و`discord.com` و`discordapp.com` و`api.telegram.org` و`webhook.notify.test` aliases عليه في الشبكة. السيرفر بيثق في الـ CA عن طريق `SSL_CERT_FILE` (Go بيقرا الملف مع مجلد الشهادات الافتراضي، فالشهادات العادية ماتتشالش)، و`DHOW_NOTIFY_ALLOW_PRIVATE_CIDRS` فاتحة عنوان الـ receiver وحده. ما اتضافش أي إعداد جديد في السيرفر.
2. **الـ receiver بيتعلّم webhook وأدوات تحكم.** `/hooks/` بيقبل الـ webhook ويسجّل `Dhow-Event` و`Dhow-Delivery` و`Idempotency-Key` ويتحقق من التوقيع (بسر بيتحط في `/_secret` وبنفس صيغة `v1=HMAC-SHA256(secret, timestamp.body)` ونافذة 5 دقايق). `/_hold` بيمسك الطلب قبل ما يقبله، و`/_stats` بيعدّ المحاولات وقت وصولها، و`/_count` بيعدّ الرسايل بفلاتر. الاسم `webhook.notify.test` اتضاف لقايمة المضيفات اللي الـ CA مقيّد بيها.
3. **القتل وقت الإرسال، والضمان الموثّق.** الـ receiver بيمسك طلب الـ webhook، والسيرفر بيتقتل بـ SIGKILL بعد ما الـ receiver يعدّ الوصول، وبعدين يتشغّل. المتوقّع هو الضمان الموثّق في ADR-0108: at-least-once. الرسالة بتوصل مرة أو مرتين بنفس `Idempotency-Key`، والتسليم بيقفل `sent` بعد انتهاء التأجير. مش exactly-once. اللي بيمنع التكرار في الحالات العادية هو مفتاح الصف الفريد، مش الـ demo.
4. **البحث الموسّع.** `internal/leaksweep/Notify.go` بيضيف أربع أنماط (رابط Slack ورابط Discord وتوكن Telegram وسر التوقيع)، وبيفتح `"ConfigSealed"` لأسرار منفصلة (السر والتوكن والعنوان وجزء مساره) غير الـ blob كله، فقيمة وحدها في log بتتلاقي. الأنماط بشكل القيمة الحقيقي، فمفيش false positive معروف في الأماكن المبحوث فيها، وفيه اختبار على نص عادي. والاسم في `leaksweep.NameSlackWebhook` وأخواتها.
5. **«الحالات المزروعة بس».** اختبار `TestEachNotificationSecretPlantedInAPlaceIsFoundThereAndNothingElseIs` و`caught_only` في `leak-sweep.sh` بيزرعوا قيمة في log وردّ API وبث أحداث وdump وصندوق، ومعاهم ملفات نضيفة وعنوان Slack لقناة تانية، وبيقارنوا مجموعة الملفات المصابة بالمزروعة بالظبط.
6. **الـ fuzz.** هدفان جداد لـ parsers الـ `Last-Event-ID`: `FuzzLastEventID` (`Events.ParseLastEventID`) و`FuzzJobLogLastEventID` (`Jobs.ParseLastEvent` اتحوّل لـ exported). Ansi ومولّد المساعدة TypeScript وملهمش جانب Go، فمش في `FUZZ_TARGETS`. الـ nightly بيقرا القايمة من `make fuzz-list`، فمفيش تعديل في `fuzz-nightly.yml`.
7. **CI.** job `m7-demo` في `demo.yml` بنفس شروط `m6-demo` (الـ `changes` job بيمنع الـ draft)، وبيشتغل ليلي، ومتغيّر `m7` في `deploy/ci/demo-areas.sh`.

## اللي ما اتعملش
- إشعار «نشر ينتظر موافقتك» والموافقة بخطوة تانية مش في الـ demo. بتتغطى في اختبارات Go وفي `deploy/m5-demo/run.sh`. السبب: الموافقة محتاجة مراجع بـ TOTP وبيئة محمية، وده بيكبّر الـ demo بدقايق من غير ما يضيف دليل على الإشعار نفسه.
- قتل السيرفر بين الاستلام والـ commit بتغطيه اختبارات الـ outbox بساعة مزيّفة. الـ demo بيغطي القتل أثناء الإرسال فعلًا.
- الـ demo ما اتشغّلش محليًا (محتاج Podman rootless وcgroup v2). أول تشغيل له على الـ CI.

## الأثر
- `deploy/m7-demo/` (`run.sh` و`notify.sh` و`scenario.sh` و`compose.yaml`) وتعديل `leak-sweep.sh` و`secrets-argv-test.sh`.
- لا migrations، ولا routes، ولا تغيير في `openapi.yaml`.
