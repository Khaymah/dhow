# ADR-0108: أساس الإشعارات: القنوات والـ outbox والـ worker والـ webhook الموقّع (M7.2أ)

- **الحالة:** مقبول. بيسجّل القرارات 5 و6 و7 و8 من [`M7-Product-UI.md`](../M7-Product-UI.md) بإجابات Sabry (2026-11-07)، وأحداث الحساب (سؤال 2)، ولغة الرسايل (سؤال 3). التنفيذ في M7.2أ.
- **التاريخ:** 2026-11-07
- **بيكمّل:** [ADR-0025](0025-notifications.md) (القنوات)، وبيقفل وعد [ADR-0054](0054-second-factor-policy-sessions-and-credential-alerts.md) بنقل `Alerts` على outbox. من غير تعديل في القديمين (قاعدة الريبو).

## السياق
`Modules/Alerts` بيبعت بريد التنبيه من الذاكرة بعد الحدث: لو الـ process وقع، التنبيه بيضيع. وفي `Plan/Status.md` بند مفتوح: `runner.revoked` بيتنشر بعد الـ commit فلو الـ process وقع بينهم الـ audit بيضيع. ومفيش أي قناة غير البريد. M7.2أ بيبني الأساس اللي بتقف عليه باقي القنوات (M7.2ب) والواجهة (M7.3).

## القرار

### 1. أسرار القنوات (قرار 5: أ)
- جدول `"NotificationChannels"` بعمود `"ConfigSealed"` مختوم بـ `Core/Keys` (مالك المشروع، أو المؤسسة للقناة الافتراضية)، من غير `appcrypto.NewSealer`. الإعدادات غير السرية (النوع والاسم واللغة والمضيف) أعمدة عادية.
- الـ API بيرجّع `HasSecret` و`Host` بس. مفيش جزء من الرابط ولا آخر حروف. استبدال الرابط أو السر بيحتاج `CurrentFactor` (ADR-0054)، وإنشاء القناة وتعديلها وإزالتها كمان.
- سر توقيع الـ webhook العام بيتولّد من داو (32 بايت) وبيظهر **مرة واحدة** في رد الإنشاء (أو رد «غيّر السر»)، وبعدها `HasSecret` بس.
- منشئ القناة لازم يفضل بدور بيديرها (زي توكن الاستقبال في [ADR-0102](0102-m6-review-fixes-ingest-owner-reach-and-limits.md)): القناة بتتعطّل (`Enabled=false` و`DisabledReason=creator-removed`) لما يتشال أو يخرج، وبنفس الفحص وقت كل إرسال لو دوره نزل تحت `notification.manage`.
- `"ConfigSealed"` اتضاف لسجل `keys.Registry` (فبتدخل في `keys rekey` وفي الـ backfill `keys.envelope.NotificationChannels.ConfigSealed` وتقرير `keys status` بدون كود زيادة).

### 2. إجراءات الصلاحيات
- `notification.manage` (إجراء مشروع): Owner وAdmin وMaintainer. حساس (خطوة تانية)، ومش في إجراءات التوكنات.
- `org.notification.manage` (إجراء مؤسسة): Owner وAdmin بس، للقنوات الافتراضية للمؤسسة. حساس.
- القراية (`GET`) على `project.read` للمشروع و`org.read` للمؤسسة، ومن غير أي سر.

### 3. التسليم (قرار 6: ب) و`Core/Outbox`
- الـ outbox بنية تحتية مشتركة فبتتحط في `Core/Outbox` (جدول `"NotificationOutbox"` وطابور الـ worker)، علشان `Modules/Alerts` و`Modules/Notifications` يكتبوا فيه من غير ما يستورد واحد التاني. `Modules/Notifications` بيسجّل «المسلّمين» (email وwebhook وinternal-email) في الـ worker.
- الصف بيتكتب **في نفس transaction الحدث والـ audit** (`Audited.Tx`)، فلو الـ commit فشل مفيش إشعار، ولو نجح مفيش ضياع. ده بيتحقق لأحداث النشر والموافقة والـ run الفاشل وإلغاء الـ runner. أحداث الحساب `auth.*` (الدخول وتغيير الخطوة التانية) بتتنشر بعد الـ commit من `Session` من غير `Tx`، فالإشعار فيها بيتكتب في transaction لوحده بعد الحدث: لو الـ process وقع بين الاتنين الحدث بيتسجّل في الـ audit بس والإشعار بيضيع. ده مقبول لأن الـ audit هو السجل الحاكم، ونقل نشر `auth.*` جوه الـ transaction مؤجّل (فيه أثر على كل مسارات الدخول).
- الـ worker: قفل advisory على مستوى الـ instance (instance واحد بس بيسلّم)، و`FOR UPDATE SKIP LOCKED` عند الاستلام، وتأجير للصف (`ClaimedUntil`، دقيقتين) فلو الـ process وقع الصف بيرجع للطابور بعد انتهاء التأجير. الصف المستلَم بيزوّد `Attempts`.
- جدول إعادة المحاولة بعد فشل المحاولة رقم n: **1د ثم 5د ثم 30د ثم 2س**، والمحاولة الخامسة لو فشلت الصف بيبقى `dead` (الحد 5 محاولات). الجدول محتفظ بخانة سادسة (6س) لو الحد اترفع مستقبلًا، ومش مستخدمة دلوقتي. الفشل الدائم (رابط محجوب أو قناة معطّلة أو 4xx غير 408/425/429) بيروح `dead` من أول محاولة.
- سقف التزامن: 2 لكل قناة و16 عام، **و4 لكل مؤسسة** (علشان مؤسسة واحدة ما تاخدش الـ 16)، **وخانتين مخصوصتين لـ `internal-email`** برّه الـ 16 (بريد أمان الحساب بيتاخد الأول وما بيتأخرش وراء webhooks). الاستلام بيستثني القنوات والمؤسسات اللي وصلت سقفها في الاستعلام نفسه. مهلة 5 ثواني للاتصال و10 للطلب. بنقرا 64 KiB من الرد ونهمل الباقي. الرد نفسه مابيتخزّنش.
- `LastError` من enum ثابت (`timeout` و`connect` و`http_status` و`blocked` و`channel_disabled` و`render`...) ومعاه `LastStatus` (كود HTTP). **مابنخزّنش جسم الرد ولا الرابط.**
- قاطع الدايرة: 10 فشل متتالي للقناة = `Enabled=false` و`DisabledReason=failures`، وبريد داخلي للمنشئ، وaudit `notification.channel_disabled`.
- الاحتفاظ: صفوف `sent` و`dead` بتتمسح بعد 30 يوم (`DHOW_NOTIFY_RETENTION_DAYS`) عن طريق `Database/Retention` (جدول مؤقت، فبيدخل في قايمة الجداول اللي الـ retention يقدر يمسح منها).
- at-least-once: ممكن يتكرر الإرسال بعد وقوع بين الاستلام والتأكيد. المفتاح `Dhow-Delivery` (معرّف الصف، ثابت بين المحاولات) ومعرّف الحدث في الحمولة. التوثيق بيقول إن المستقبل لازم يتحمّل التكرار.
- تسوية المحاولة بتتأكد إن `Attempts` لسه هي اللي اتاخدت وقت الاستلام: worker انتهى تأجيره وصف اتاخد من worker تاني مابيقدرش يسوّيه (التسوية بتتجاهل ويتسجّل تحذير). مهلة SMTP (10 ثواني) أقصر من التأجير (دقيقتين) واختبار بيثبت ده. وأي خطأ غير متوقع (`recover`) جوه المسلّم بيفشّل المحاولة بالرمز `adapter_failed` من غير ما يوقّع السيرفر.
- عند الإيقاف: الـ worker بيستنى المسلّمين الجارين 5 ثواني بالكتير، وبعدين يبعت `internal-email` المنتظر بمهلة 5 ثواني، ومابيستناش webhook أبدًا (التأجير بيرجّعه بعد الإقلاع).
- فريد: `(EventId, ChannelId, RecipientUserId)` فنفس الحدث ما يتكتبش مرتين لنفس القناة حتى لو اتنشر مرتين.

### 4. الاشتراكات (قرار 7: ج)
- القنوات والقواعد على المشروع، والقنوات الافتراضية على المؤسسة (`ProjectId` فاضي) وبتنطبق على كل مشاريعها. الحدث بيروح لقنوات المشروع ولقنوات المؤسسة.
- القاعدة (واحدة لكل قناة): الأحداث والبيئات (فاضي = الكل) والحالات، ولقناة البريد الأدوار اللي يوصلها البريد (`Roles`: الدور في المؤسسة). الافتراضي عند إنشاء قناة: `failed` و`rolled_back` و`rollback_failed` و`awaiting_approval` (من غير `succeeded`) على أحداث النشر والموافقة والـ run الفاشل.
- **بريد القنوات بيروح لمين يقرا المشروع فعلًا**: المستلم هو عضو دوره الفعلي في المشروع (دور المؤسسة + أدوار الفرق + ربط المشروع، بنفس `policy.EffectiveOf` و`ProjectRole` بتوع `can()`) ضمن أدوار القاعدة وبيسمح بـ `project.read`، والعضو المحذوف أو المعطّل أو من مؤسسة تانية مابيجيلوش حاجة. حد أقصى 100 مستلم للحدث الواحد، وحد 30 بريد قنوات في الساعة لكل مستخدم عبر كل القنوات (بيتحسب وقت الكتابة في الـ outbox). فشل إرسال البريد (`mail_send`) مابيتحسبش على قاطع الدايرة، لأن سيرفر بريد واقع مش غلطة القناة؛ بيظهر في سجل التسليم وبيتعاد. إشعار «القناة اتوقفت» بيروح بريد للمنشئ، وفي كل الأحوال القناة نفسها بتظهر `DisabledReason` في الواجهة والـ audit.
- رسالة الاختبار ليها حد لكل قناة (5 في الدقيقة) **ولكل مستخدم (10) ولكل مؤسسة (20)** عن طريق `Core/RateLimit`.
- الـ API بيرجّع `Host` و`CreatedByUserId` لمن يملك `notification.manage` (أو `org.notification.manage`) بس؛ القارئ بيشوف الاسم والنوع والحالة.
- كتم شخصي في `"NotificationMutes"` (`PUT /me/notification-mute`): المستخدم اللي كاتم مابيجيلوش بريد القنوات (وبريد الحساب الأمني مش بيتكتم).

### 5. الأحداث في أول نسخة
| اسم الإشعار | مصدره على الـ bus | ملاحظة |
|---|---|---|
| `deployment.finished` | `deployment.finished` (الحالة `succeeded` أو `failed` أو `canceled`)، و`deployment.release_rolled_back` (`rolled_back`) و`deployment.release_rollback_failed` (`rollback_failed`) | بيئة ومشروع وإصدار |
| `deployment.approval.requested` | `deployment.approval_requested` | الحالة `awaiting_approval` |
| `run.finished` | حدث جديد `project.run_finished` من `Pipelines` | **الفاشل فقط**، وبيتنشر جوه نفس transaction إنهاء الـ run، ومش في الـ audit |
| `runner.offline` | فحص دوري في الـ worker (الـ runner الفعّال اللي ما اتشافش 5 دقايق)، مرة لكل انقطاع | على مستوى المؤسسة |
| `account.new_device_login` | `auth.login` + جدول `"NotificationDevices"` (بصمة مستخدم + متصفح ونظام) | أول جهاز للمستخدم بيتسجّل من غير إشعار |
| `account.second_factor_changed` | تفعيل أو تعطيل TOTP، وإضافة أو إزالة مفتاح مرور، وتجديد أكواد الاستعادة | |
| `member.removed` و`member.role_changed` | `identity.member.removed` و`identity.member.role_changed` | |
- أحداث الحساب والأعضاء على مستوى المؤسسة: بتروح لقنوات المؤسسة (لو القاعدة فيها الحدث، وهي مش في الافتراضي)، وبريد للمستخدم المعني بنفسه (قناة البريد الداخلية).
- **الحمولة allowlist ثابتة**: النوع والمشروع والبيئة والهدف بالاسم والإصدار (digest قصير) والحالة والسبب (enum) ورابط داو والوقت، وفي أحداث الحساب: نوع التغيير والمتصفح والنظام وجزء الشبكة التقريبي (`/24` أو `/48`، زي `Alerts`). **ممنوع:** توكن أو IP كامل أو سطر سجل أو قيمة متغيّر أو أي سر.

### 6. حماية SSRF والتوقيع (قرار 8: أ)
- `netguard` على **كل إرسال**: الحل والتثبيت وقت الاتصال (`Dialer.DialContext` بيحل الاسم ويفحص كل عنوان ويتصل بالـ IP المثبّت)، والـ loopback والخاص والـ metadata والتمثيلات الغريبة (decimal وoctal وIPv4-mapped) مرفوضة إلا النطاقات في `DHOW_NOTIFY_ALLOW_PRIVATE_CIDRS` (الافتراضي فاضي). بنفس الفحص وقت الحفظ (`CHANNEL_HOST_BLOCKED`) لتأخذ المستخدم رسالة بدري، لكن الفحص الحاكم هو وقت الإرسال.
- **من غير redirects**، و**https بس** (مفيش استثناء، حتى في `DHOW_DEV`؛ الاختبارات بتحط `tls.Config` بتثق في شهادة الـ receiver)، و`Proxy` معطّل في الـ transport.
- التوقيع: `Dhow-Signature: v1=<hex(HMAC-SHA256(secret, timestamp + "." + body))>`، ومعاه `Dhow-Timestamp` (ثواني Unix) و`Dhow-Event` و`Dhow-Delivery`. نافذة التحقق 5 دقايق عند المستقبل. الجسم JSON بحقول PascalCase.
- مضيفين Slack وTelegram وDiscord الثابتة في M7.2ب.

### 7. لغة الرسايل (سؤال 3)
لغة القناة (`Locale`، والافتراضي لغة المؤسسة)، مش لغة اللي ضغط الزرار. بريد الحساب بلغة المستخدم المعني.

### 8. `Alerts` على الـ outbox
`Alerts` بيبقى مصدر الرسالة بس: بيبني الرسالة (بنفس `Message()` واختباراتها) ويكتبها في الـ outbox كصف `internal-email` بنفس transaction الحدث لما يكون فيه `Tx`، والـ worker هو اللي بيبعتها بـ `Core/Mail` مع إعادة المحاولة. السقف القديم في الذاكرة (`AlertMaxPending`) اتشال لأن الطابور في القاعدة. اختبارات `Alerts` الموجودة بتعدّي: مساعد الاختبار `SentMail` (و`Alerts.Wait()`) بيفضّي الطابور قبل ما يرجّع الرسايل. الاستثناء الوحيد اختبار `TestPendingAlertsAreCapped`: سقف الـ 32 اتحوّل من «إرسال في الذاكرة» لـ«صفوف معلّقة في الطابور»، فالاختبار بيتأكد إن الصف الـ 33 بيترفض بنفس رسالة اللوج وإن الطابور مافيهوش أكتر من 32، بدل ما يعدّ الإرسال الجاري.

### 9. `runner.revoked`
`Runners` بينشر الحدث **جوه** transaction الإلغاء (`Tx`)، فالـ audit والإشعارات بيتكتبوا مع الإلغاء أو مايتكتبوش خالص. اختبار: خطأ في الـ audit يرجّع الإلغاء كله والـ runner يفضل فعّال.

## الأثر
- جداول جديدة (كلها expand): `"NotificationChannels"` و`"NotificationRules"` و`"NotificationOutbox"` و`"NotificationMutes"` و`"NotificationDevices"` و`"NotificationRunnerOutages"`. الـ binary القديم بيتجاهلها.
- أخطاء جديدة: `CHANNEL_EXISTS` و`CHANNEL_HOST_NOT_ALLOWED` و`CHANNEL_HOST_BLOCKED` و`CHANNEL_SECRET_INVALID` و`CHANNEL_VERSION_CONFLICT` و`CHANNEL_DISABLED` و`DELIVERY_NOT_RETRYABLE` و`TEST_RATE_LIMITED`، كلها مترجمة بالنسختين.
- audit (أسماء وأنواع بس): `notification.channel_created` و`_updated` و`_removed` و`_disabled` و`notification.rule_updated` و`notification.test_sent`.
- إعدادات جديدة: `DHOW_NOTIFY_ALLOW_PRIVATE_CIDRS` و`DHOW_NOTIFY_RETENTION_DAYS`.
- المتروك: Slack وTelegram وDiscord (M7.2ب)، وصندوق `inapp` والواجهة (M7.3). بريد القناة بيستخدم دور العضو في المؤسسة بس، ومش ربط الأدوار على المشروع (المتروك في M7.3 مع الواجهة).

## البدائل اللي اترفضت
- **fire-and-forget على الـ EventBus:** بيضيع مع أي وقوع.
- **queue خارجية (Redis أو NATS):** خدمة زيادة للمشغّل.
- **أسرار القنوات في `Core/SecretStore`:** أسرار الـ jobs بتتوصّل للـ runner، فرابط Slack يتسرّب لكود المشروع.
- **حفظ نص الرابط بتاع الـ webhook في عمود عادي:** الرابط نفسه سر (فيه توكن في المسار).
