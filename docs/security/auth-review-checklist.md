[English](../en/security/auth-review-checklist.md)

# قائمة مراجعة المصادقة (M2)

الحالة: **اكتملت المراجعة الداخلية، والمراجعة الخارجية معلّقة (M9)** · التاريخ: 2026-10-28 · النطاق: المرحلة M2 (الهوية والمؤسسات)

هذه الصفحة نقطة البداية للمراجعة الخارجية لمسارات المصادقة والصلاحيات، التي انتقلت إلى M9 مع اختبار الاختراق. تذكر كل مسار في النطاق، والتهديدات التي درسناها، والضابط لكل تهديد، ومكان الضابط في الكود، والاختبار الذي يثبته. وتنتهي بنتائج المراجعة الداخلية والبنود التي نعرف أنها مفتوحة. قرارات التصميم خلف الضوابط في ADR-0033 وADR-0035 وADR-0037 وADR-0038 وADR-0039 وADR-0040 (في `Plan/ADR/`، بالعربية). والوصف العام في [نموذج الأمان](../security-model.md).

المسارات التالية نسبية إلى `apps/server/internal/`، إلا إذا بدأت بمجلد رئيسي آخر.

## 1. الطريقة

- قراءة جديدة لكل مسار في M2 (الإعداد، وكلمة السر، والخطوة الثانية، والجلسات، والمؤسسات، والدعوات، وOIDC، ومفاتيح API، وسجل الأحداث) مقابل [نموذج الأمان](../security-model.md) وOWASP ASVS 4.0.3 المستوى 2، بعد اكتمال تحصين M2.5.
- تبعتها مراجعة مستقلة للفرع ولم تجد ما يمنع الدمج. نتائجها من F16 إلى F24 أدناه، وكلها عولجت في M2.5.
- كل نتيجة حرجة (Critical) أو عالية (High) مُصلحة في M2.5. وكل نتيجة متوسطة (Medium) مُصلحة أو مسجّلة مع سببها. تكتمل M2 عندما تنجح كل اختبارات الصلاحيات ولا تبقى نتيجة حرجة أو عالية مفتوحة.
- الأدلة الآلية: `make check` (الـ lint، و`go test -race` مع PostgreSQL 18، وgovulncheck، واختبارات الـ hooks)، و`make fuzz FUZZTIME=60s` (13 هدفًا)، و`make timing` (اختبارات توقيت إحصائية)، و`deploy/smoke.sh` على صورة الحاوية.

## 2. المسارات في النطاق

كل المسارات تحت `/api/v1`. الحدود لكل شبكة لا تعدّ إلا الطلبات التي نجحت في فحص Origin. "Origin" تعني أن الطلب يجب أن يحمل `Origin` مساويًا لـ `DHOW_PUBLIC_URL`. و"CSRF" تعني أن `X-CSRF-Token` الخاص بالجلسة مطلوب أيضًا. لكل مسار سطر في `Policies()` لوحدة ما، و`App/RouteTable_test.go` يمر على جدول المسارات كله لكل دور.

| المسار | من | فحوص الطلب | الحدود |
|---|---|---|---|
| `POST /setup` | أي شخص، قبل وجود صاحب النسخة الأول | Origin، ورمز إعداد يعمل مرة واحدة في الجسم | لكل شبكة عميل (`DHOW_RATE_LIMIT_PUBLIC`) |
| `POST /sessions` | أي شخص | Origin | قفل لكل حساب ولكل شبكة عميل، ولكل شبكة عميل (`DHOW_RATE_LIMIT_PUBLIC`) |
| `POST /sessions/mfa` | حامل `__Host-dhow_mfa` | Origin، وتحدٍ مربوط بالمستخدم والـ User-Agent | قفل لكل مستخدم ولكل شبكة عميل، و5 أكواد لكل تحدٍ، ولكل شبكة عميل |
| `POST /sessions/mfa/passkey-options` | حامل `__Host-dhow_mfa` | Origin | مربوط بالتحدي المعلّق (الاستثناء الوحيد المذكور من الحدود لكل شبكة) |
| `POST /sessions/passkey-options` | أي شخص | Origin | لكل شبكة عميل، مشترك مع `POST /sessions/passkey` |
| `POST /sessions/passkey` | أي شخص | Origin، وتحدٍ محفوظ يعمل مرة واحدة | قفل لكل شبكة عميل، ولكل شبكة عميل |
| `GET /oidc/providers` | أي شخص | لا يوجد (قراءة فقط) | لا يوجد |
| `POST /oidc/authorize`، `POST /oidc/callback` | أي شخص | Origin، و`__Host-dhow_oidc`، وstate، وPKCE، وnonce | لكل شبكة عميل، مشترك |
| `POST /invitations/preview`، `POST /invitations/accept` | أي شخص معه رابط | Origin، والرمز في الجسم | لكل شبكة عميل، مشترك |
| `GET /me`، `/sessions`، `/me/*` | مستخدم مسجّل الدخول | الجلسة، وCSRF للكتابة، والخطوة الحالية لتغيير الخطوات وربط الهويات وإنشاء المفاتيح | تغيير كلمة السر يشترك في قفل الحساب |
| `POST /organizations` | صاحب النسخة، أو أي مستخدم مع `DHOW_ORG_CREATION=any-user` | الجلسة وCSRF | لا يوجد |
| `/organizations/{id}/**` | الأعضاء، حسب الدور | الجلسة وCSRF، أو مفتاح API شخصي على المسارات التي تسمح به | الدعوات لكل عضو ولكل مؤسسة في كل ساعة |
| `PUT /organizations/{id}/second-factor-rule` | صاحب مؤسسة له خطوة ثانية | الجلسة وCSRF والخطوة الحالية، وليس مفتاحًا أبدًا | لا يوجد |
| `/organizations/{id}/audit-events`، `.../verify` | `audit.read` | جلسة أو مفتاح | لا يوجد |

## 3. التهديدات والضوابط والإثبات

### الإعداد

| التهديد | الضابط | الكود | الإثبات |
|---|---|---|---|
| شخص آخر يتملّك السيرفر أولًا | الإعداد يحتاج إلى رمز 256 بت مطبوع في سجل السيرفر، يعمل مرة واحدة، لمدة ساعة | `Modules/Identity/Identity.Setup.go` | `Modules/Identity/Test/Integration/Setup_test.go`، `Modules/Session/Test/E2E/Flow_test.go` (`TestSetupIsOneShot`) |
| طلبا إعداد يتسابقان | معاملة واحدة تقفل صف السيرفر والرمز | `Identity.Setup.go` (`create`) | `Setup_test.go` |
| استنزاف المعالج عبر الإعداد | نفحص الرمز قبل فحص التسريب وArgon2id، وحد لكل شبكة عميل | `Identity.Setup.go`، `Modules/Identity/Identity.Routes.go` | `App/RateLimit_test.go` |

### الدخول بكلمة السر

| التهديد | الضابط | الكود | الإثبات |
|---|---|---|---|
| التخمين عبر الشبكة وحشو بيانات الدخول | نعدّ الفشل قبل التحقق، لكل حساب (5) ولكل شبكة عميل (20)، مع قفل يتضاعف من 30 ثانية إلى 15 دقيقة. وIPv6 يُعدّ لكل /64 | `Modules/Session/Session.Throttle.go`، `Core/Auth/Throttle.go` | `Modules/Session/Test/Integration/Throttle_test.go` (ومنه `TestAnIPv6AttackerCannotSpreadOverItsSlash64`) |
| تخمينات متوازية تتجاوز العدّاد | حجز في معاملة واحدة مع `FOR UPDATE` على العدّادين | `Session.Throttle.go` | `Throttle_test.go` (`TestParallelGuesses...`) |
| تعداد الحسابات من الرد | `INVALID_CREDENTIALS` واحد لكل فشل، مقفلًا كان أو لا | `Modules/Session/Session.Login.go` | `Modules/Session/Test/Edge/Generic_test.go`، `Flow_test.go` (`TestLoginErrorsAreGeneric`) |
| تعداد الحسابات من التوقيت | الحسابات غير الموجودة والموقوفة تنفّذ تحقق Argon2id كاملًا. لا فرق أكبر من max(10%, 300 µs) من الوسيط، حوالي 3.5 ms هنا | `Session.Login.go`، `Core/Auth/Password.go` | `App/Timing/Timing_test.go` (`make timing`)، مع عينة ضابطة يجب أن تلاحظ 6 ms في الدخول |
| كسر كلمات السر بعد تسريب قاعدة البيانات | Argon2id (19 MiB، t=2)، وإعادة الـ hash عند الدخول بعد تغيير المعاملات | `Core/Auth/Password.go` | `Core/Auth/Password_test.go` |
| كلمات سر ضعيفة أو مسرّبة | من 12 إلى 128 حرفًا، وفحص range في Have I Been Pwned (k-anonymity) | `Core/Auth/PasswordPolicy.go`، `Core/Auth/Breached.go` | `Core/Auth/Breached_test.go`، `FuzzPasswordLength` |

### الخطوة الثانية

| التهديد | الضابط | الكود | الإثبات |
|---|---|---|---|
| تخطي الخطوة الثانية | تحدي `mfa_pending` في السيرفر، مربوط بالمستخدم وبـ hash الـ User-Agent. لا معرّف مستخدم من العميل | `Modules/Session/Mfa.Pending.go` | `Modules/Session/Test/E2E/Totp_test.go`، `Passkey_test.go` |
| تخمين أكواد TOTP | قفل لكل مستخدم منفصل عن عدّاد كلمة السر، و5 أكواد لكل تحدٍ | `Mfa.Pending.go` | `Modules/Session/Test/Integration/MfaThrottle_test.go` |
| إعادة استخدام كود TOTP | نحفظ آخر خطوة مقبولة ونحدّثها بشكل ذري | `Modules/Session/Mfa.Factor.go`، `Core/Auth/Totp.go` | `Core/Auth/Totp_test.go`، `FuzzTotp` |
| جلسة مسروقة تسجّل خطوة المهاجم | كل تغيير خطوة يحتاج إلى الخطوة الحالية في الطلب نفسه، ويلغي الجلسات الأخرى | `Modules/Session/Mfa.Factor.go` | `Modules/Session/Test/E2E/FactorChange_test.go` |
| تصيّد مفتاح المرور أو نسخه | معرّف RP والمصدر من `DHOW_PUBLIC_URL`. التحقق من المستخدم مطلوب للدخول بلا كلمة سر. عدّاد التوقيع الذي يرجع إلى الخلف مرفوض ومسجّل | `Core/Auth/WebAuthn.go`، `Modules/Session/Mfa.Passkey*.go` | `Passkey_test.go`، `FuzzWebAuthnResponses` |

### الجلسات وCSRF والرموز أثناء النقل

| التهديد | الضابط | الكود | الإثبات |
|---|---|---|---|
| سرقة الكوكي بسكربت أو نطاق فرعي | كوكيز `__Host-`، و`HttpOnly`، و`Secure`، و`SameSite=Strict`، و`Path=/` | `Core/Auth/Cookie.go` | `Modules/Session/Test/E2E/Secrets_test.go`، واختبار smoke |
| تثبيت الجلسة (session fixation) | معرّف جديد 256 بت في كل دخول، والقديم يُلغى | `Session.Login.go` (`openSession`) | `Modules/Session/Test/Integration/Lifecycle_test.go` |
| جلسات قديمة | 12 ساعة خمول (4 ساعات لكل من هو صاحب مؤسسة أو مدير في أي مكان، من الدور الحالي مع كل طلب)، و7 أيام مطلقة. وإلغاء عند الخروج، وتغيير كلمة السر والخطوة والدور | `Modules/Session/Session.Service.go`، `Session.Policy.go` | `Lifecycle_test.go`، `SessionPolicy_test.go`، `Flow_test.go` (`TestRevokeSessions`) |
| CSRF | Origin على كل طلب غير آمن، مع رمز HMAC مربوط بالجلسة | `Core/Auth/Guard.go`، `Core/Auth/CSRF.go` | `Modules/Session/Test/E2E/Csrf_test.go`، `Core/Auth/CSRF_test.go` |
| رمز أو كوكي في رابط أو سجل | الرموز تنتقل في الأجسام أو الكوكيز أو جزء الـ fragment. سجل الوصول يحذف الـ query. مفاتيح السجل منقّحة | `Core/Middleware/AccessLog.go`، `internal/logging` | `Secrets_test.go`، `Core/Middleware/Middleware_test.go` |
| رموز مشوّهة أو بكتابات بديلة | طول مرمّز دقيق، وbase64url صارم، ثم SHA-256 أو HMAC. والشيء نفسه لرموز أجهزة التشغيل | `Core/Auth/Token.go`، `Core/Auth/ApiToken.go`، `Core/RunnerAuth/Tokens.go` | `Core/Auth/Token_test.go`، `Core/RunnerAuth/RunnerAuth_test.go`، `FuzzHashSessionToken`، `FuzzAPITokenHash` |

### مزودو الهوية الخارجيون

| التهديد | الضابط | الكود | الإثبات |
|---|---|---|---|
| CSRF في الدخول وحقن المسار | كوكي `__Host-dhow_oidc`، وstate عشوائي، وPKCE S256، وnonce. نستهلك المسار قبل تبادل الكود | `Modules/Session/Oidc.Flow.go` | `Modules/Session/Test/E2E/OidcNegative_test.go` |
| الاستيلاء على حساب عبر البريد | الهويات تطابق `(issuer, subject)` فقط. لا ربط ولا إنشاء حساب عبر البريد | `Modules/Session/Oidc.Login.go` | `OidcNegative_test.go`، `Oidc_test.go` |
| رموز هوية مزوّرة أو معادة | تحقق `go-oidc`، والـ nonce بزمن ثابت، و`azp`، ومهلة 10 ثوانٍ، وحد 1 MB، وبلا تحويلات | `Core/Oidc/Oidc.go`، `Core/Oidc/Provider.go` | `Core/Oidc/Oidc_test.go`، `Core/Oidc/Discovery_test.go` |
| نطاق خاطئ أو بريد غير موثّق | مطابقة دقيقة للنطاق على بريد موثّق | `Core/Oidc/Domains.go` | `FuzzAcceptedEmail`، `FuzzEmailVerifiedClaim` |
| فك آخر طريقة دخول | قفل على صف المستخدم وعدّ كل طرق الدخول | `Modules/Session/Oidc.Link.go` | `Modules/Session/Test/E2E/SignInMethods_test.go` |

### مفاتيح API الشخصية

| التهديد | الضابط | الكود | الإثبات |
|---|---|---|---|
| مفتاح يتجاوز منشئه | كل طلب يحتاج إلى الصلاحية وإلى الدور الفعلي الحالي للمنشئ | `Core/Policy/Policy.go`، `Modules/Tokens/Tokens.Resolve.go` | `Modules/Tokens/Test/E2E/Tokens_test.go`، `App/RouteTable_tokens_test.go` |
| مفتاح على مسار للمتصفح فقط | المسارات تقبل المفاتيح فقط عندما يقول سطر سياستها ذلك، و`openapi.yaml` يعلن المجموعة نفسها | `Core/Policy/Table.go` | `App/RouteTable_tokens_test.go`، `App/OpenAPI_tokens_test.go` |
| مفتاح يؤكد خطوة تحقق | `ConfirmFactor` يرفض أي هوية آتية من مفتاح | `Modules/Session/Session.Service.go` | `Tokens_test.go` |
| مفتاح مسرّب بعد مغادرة العضو | مربوط بصف العضوية والحساب والمؤسسة | `Modules/Tokens/Queries/Tokens.sql` | `Tokens_test.go` |
| كشف الوجود من التوقيت | المفاتيح غير المعروفة والملغاة والمنتهية تسلك المسار نفسه | `Tokens.Resolve.go` | `App/Timing/Timing_test.go` |

### المؤسسات والأدوار والدعوات

| التهديد | الضابط | الكود | الإثبات |
|---|---|---|---|
| IDOR بين المؤسسات | الحارس يرد بـ 404 لغير الأعضاء، والمحمّلات محصورة في المؤسسة | `Core/Auth/Guard.go` | `App/RouteTable_test.go`، `App/RouteTable_orgs_test.go` |
| رفع الصلاحيات عبر المنح | لا أحد يمنح أعلى من دوره، أو يغيّر من هو أعلى، أو يغيّر دوره بنفسه. الروابط لا تمنح دور صاحب المؤسسة أبدًا | `Core/Policy/Rank.go`، `Modules/Identity/Members.Service.go` | `Core/Policy/Rank_test.go`، `Modules/Identity/Test/E2E/Members_test.go`، `Modules/Identity/Test/Integration/Bindings_test.go` |
| تغيير صاحب المؤسسة بلا تحقق إضافي | منح دور صاحب المؤسسة وخفضه ونقله يحتاج إلى الخطوة الحالية. وصاحب المؤسسة لا يُزال إلا بعد خفض دوره | `Members.Service.go`، `Modules/Identity/Orgs.Service.go` | `Modules/Identity/Test/E2E/OwnerRemoval_test.go` |
| فقد آخر صاحب مؤسسة في سباق | قفل على صف المؤسسة مع كل تغيير في العضوية | `Orgs.Service.go` | `Modules/Identity/Test/Integration/Owners_test.go`، `Locks_test.go` |
| إعادة ضبط MFA من المدير كطريق للاستيلاء | رتبة أعلى بوضوح في كل مؤسسة للعضو، والخطوة الحالية، وتسجيل بخطورة عالية | `Members.Service.go` | `Modules/Identity/Test/E2E/MfaRank_test.go`، `MfaReset_test.go` |
| مديرون بلا خطوة ثانية | `RequireSecondFactor` لكل مؤسسة، ومهلة 7 أيام، ثم `403 SECOND_FACTOR_REQUIRED` على الإجراءات الحساسة للجلسات والمفاتيح | `Core/Policy/SecondFactor.go`، `Modules/Identity/Identity.SecondFactor.go` | `App/RouteTable_secondfactor_test.go`، `Modules/Identity/Test/E2E/SecondFactor_test.go`، `Core/Policy/SecondFactor_test.go` |
| تغيير في بيانات الدخول لا يلاحظه المستخدم | رسالة بلغة المستخدم بعد كل حدث من 10 أحداث لبيانات الدخول، في الخلفية، بلا أي سر. وسجل الأحداث في كل الحالات | `Modules/Alerts` | `Modules/Alerts/Test/E2E/Alerts_test.go`، `Modules/Alerts/Test/Unit/Message_test.go` |
| تسريب رابط الدعوة أو إعادة استخدامه | الرمز في الـ fragment، ونحفظ SHA-256 له، ويعمل مرة واحدة، 7 أيام، ورد واحد لكل حالة غير صالحة | `Modules/Identity/Invites.Accept.go` | `Modules/Identity/Test/E2E/Invitations_test.go`، `App/Timing/Timing_test.go` |
| السيرفر كمرحّل للرسائل المزعجة | دعوات لكل عضو ولكل مؤسسة في كل ساعة، و5 إرسالات لكل دعوة | `Modules/Identity/Invites.Service.go` | `Modules/Identity/Test/E2E/InviteLimits_test.go` |

### سجل الأحداث والحدود والاحتفاظ

| التهديد | الضابط | الكود | الإثبات |
|---|---|---|---|
| التلاعب بالسجل | سلسلة hashes لكل مؤسسة، وtrigger يرفض `UPDATE` و`DELETE` و`TRUNCATE`، ومسار للتحقق | `Modules/Audit/Audit.Service.go` | `Modules/Audit/Test/Integration/Chain_test.go` |
| أسرار في السجل | قائمة مسموحة صريحة لحقول الأحداث. نحذف القيم التي فيها بادئة بيانات دخول داو | `Modules/Audit/Audit.Data.go` | `Modules/Audit/Test/Integration/Sanitizer_test.go`، `Modules/Audit/Test/Unit/DataKeys_test.go` |
| أحداث ناقصة | أداة الاختبار تُفشل كل اختبار E2E يفقد مساره حقلًا في سجل الأحداث | `App/Testing/Harness.go` | كل اختبار E2E |
| إغراق المسارات العامة | نوافذ ثابتة في PostgreSQL لكل شبكة عميل بساعة قاعدة البيانات، تشترك فيها كل النسخ، ولا تُعدّ إلا بعد فحص Origin. المفاتيح فوق الحد تُرفض من الذاكرة. نتجاهل `X-Forwarded-For` المزوّر بلا وكيل موثوق. كل كتابة عامة محدودة أو استثناء مذكور | `Core/RateLimit` | `Core/RateLimit/Postgres_test.go`، `App/RateLimit_test.go` (ومنه `TestACrossSitePageCannotSpendTheLimitOfAnOffice` و`TestEveryPublicWriteIsRateLimitedOrAnExplicitException`)، `FuzzClientIP`، `FuzzNetwork` |
| جداول تكبر مع كل محاولة | حذف دفعات من الصفوف المؤقتة المنتهية تحت advisory lock، بساعة قاعدة البيانات. والعنوان المقفل لا ينشئ صفوف حد جديدة | `Database/Retention`، `Modules/Session/Session.Throttle.go` | `Database/Retention/Retention_test.go`، `Throttle_test.go` (`TestALockedAddressCreatesNoMoreThrottleRows`) |
| تسرّب SQL مدمّر إلى الاستعلامات | hook في المشروع يحذف تعليقات SQL ويرفض `DELETE` و`TRUNCATE` في كل ملف استعلامات، إلا قالب الدفعات على الجداول المؤقتة في `Retention.sql` | `.claude/hooks/dhow_hooks/edit_rules.py` | `.claude/hooks/tests/test_hooks.py` (`make hooks-test`) |
| أخطاء الإعدادات | يرفض السيرفر البدء مع سر ناقص أو مؤقت أو مكرر، ومع أي مزوّد أو حد غير صالح | `Config` | `Config/Secrets_test.go`، `Config/Oidc_test.go`، `Config/RateLimit_test.go` |

## 4. ASVS 4.0.3 المستوى 2

| المجال | الحالة | ملاحظات |
|---|---|---|
| V2.1 أمان كلمة السر | متحقق | من 12 إلى 128 حرفًا، بلا قواعد تركيب، مع فحص كلمات السر المسرّبة، واللصق مسموح |
| V2.2 المصادقة العامة | متحقق | منع الأتمتة متحقق. 2.2.3: رسالة بعد كل تغيير في بيانات الدخول عند ضبط SMTP، وسجل الأحداث في كل الحالات (F12) |
| V2.4 حفظ بيانات الدخول | متحقق | Argon2id. وأكواد الطوارئ ومفاتيح API تحت HMAC بمفاتيح مشتقة |
| V2.5 استرجاع بيانات الدخول | جزئي | لا استرجاع ذاتي. الخطوة الثانية المفقودة يعيد ضبطها مدير برتبة أعلى مع خطوته. ولكلمة السر المنسية لا يوجد مسار (النتيجة F13) |
| V2.8 كلمات السر لمرة واحدة | متحقق | TOTP مع الحماية من إعادة الاستخدام، وفرق خطوة واحدة |
| V2.9 المصادقة التشفيرية | متحقق | مفاتيح المرور مع التحقق من المستخدم في الدخول بلا كلمة سر |
| V3.2 ربط الجلسة | متحقق | معرّفات 256 بت، ومعرّف جديد عند الدخول |
| V3.3 إنهاء الجلسة | جزئي | 7 أيام مطلقة، و12 ساعة خمول، و4 ساعات لأصحاب المؤسسات والمديرين. يطلب 3.3.2 إعادة التحقق بعد 12 ساعة استخدام، والأيام السبعة قرار منتج (F10، ADR-0054) |
| V3.4 الكوكيز | متحقق | `__Host-`، و`Secure`، و`HttpOnly`، و`SameSite=Strict` |
| V3.5 الجلسات بالرموز | متحقق | مفاتيح API محدودة الصلاحيات، ولها انتهاء، وقابلة للإلغاء، ومحفوظة كـ HMAC |
| V3.7 إعادة التحقق | متحقق | الخطوة الحالية في الطلب نفسه لكل تغيير حساس |
| V4.1 وV4.2 التحكم في الوصول | متحقق | دالة سياسة واحدة، والمنع افتراضي، واختبارات جدول المسارات، و404 بين المؤسسات، وOrigin وCSRF |
| V4.3.1 MFA للواجهات الإدارية | متحقق عند التفعيل | تستطيع المؤسسة اشتراط خطوة ثانية لأصحاب المؤسسة والمديرين أو للجميع، مع مهلة 7 أيام (F11) |
| V7.1 إلى V7.3 السجلات | متحقق | لا بيانات دخول في السجلات أو بيانات سجل الأحداث، والأحداث الأمنية مسجّلة، وسلسلة كاشفة للتلاعب |
| V8.3.1 البيانات الحساسة في الروابط | متحقق | الرموز في الأجسام أو الكوكيز أو الـ fragments فقط |
| V11.1.4 منع الأتمتة | متحقق | حدود لكل شبكة على المسارات العامة بعد فحص Origin، وحدود للدعوات |
| V14.4 ترويسات أمان HTTP | جزئي | CSP مع Trusted Types (النتيجة F9 أُصلحت في M7.10) |

## 5. نتائج المراجعة الداخلية

| المعرّف | الخطورة | النتيجة | الحالة |
|---|---|---|---|
| F1 | عالية | كان صاحب المؤسسة يستطيع إزالة صاحب مؤسسة آخر بدون الخطوة الحالية، وكذلك مفتاح API لصاحب مؤسسة مع `member.manage`، بينما خفض دور صاحب المؤسسة كان يحتاج إلى الخطوة. | أُصلحت: صاحب المؤسسة لا يُزال إلا بعد خفض دوره، وهذا يحتاج إلى الخطوة. `OwnerRemoval_test.go` |
| F2 | متوسطة | أقفال العناوين عدّت IPv6 لكل عنوان، فجهاز واحد معه /64 هرب من قفل الـ 20 محاولة للعنوان. | أُصلحت: لكل /64. `Throttle_test.go` |
| F3 | متوسطة | معاينة الدعوة وقبولها، وبدء OIDC والعودة منه، وخيارات مفتاح المرور، لم يكن لها حد لكل عنوان، والدعوات لم يكن لها سقف (مرحّل للرسائل المزعجة). | أُصلحت في T1: `Core/RateLimit`، `App/RateLimit_test.go`، `InviteLimits_test.go` |
| F4 | متوسطة | منقّي سجل الأحداث حذف كل حقل فيه "session"، فلم يُسجَّل `RevokedSessions` أبدًا. | أُصلحت في T2: قائمة مسموحة. `Sanitizer_test.go`، `DataKeys_test.go` |
| F5 | منخفضة | مفكك base64 في Go يتخطى CR وLF، فقُبل رمز فيه سطر جديد ككتابة أخرى للرمز نفسه (اكتُشفت بالـ fuzzing). | أُصلحت: طول دقيق. `Token_test.go` |
| F6 | منخفضة | تغيير دور قرأ الهدف كغير صاحب مؤسسة كان يستطيع خفض دور شخص رُقّي إلى صاحب مؤسسة بالتوازي بدون الخطوة. | أُصلحت: يرد التغيير بـ `409 CONFLICT` عندما يتغير الدور تحته. |
| F7 | منخفضة | صفوف الحدود والتحديات والمسارات والعدّادات لم تُحذف أبدًا. | أُصلحت في T3: `Database/Retention` |
| F8 | منخفضة | `POST /setup` لم يكن له حد لكل عنوان. | أُصلحت: لكل شبكة عميل. `App/RateLimit_test.go` |
| F9 | متوسطة | الـ CSP ليس فيها `require-trusted-types-for 'script'`، مع أن نموذج الأمان ذكر Trusted Types. | أُصلحت في M7.10 ([ADR-0115](../../Plan/ADR/0115-trusted-types-enforced.md)). الـ CSP فيها الآن `require-trusted-types-for 'script'; trusted-types 'none'` في كل الأوضاع، بعد أن مرّت اختبارات المتصفح على كل الشاشات بلا مخالفة. |
| F10 | متوسطة | الجلسات تدوم حتى 14 يومًا (12 ساعة خمول)، ويطلب ASVS 3.3.2 إعادة التحقق بعد 12 ساعة. | أُصلحت في M2.6 (ADR-0054): 7 أيام مطلقة، حتى للجلسات المفتوحة من قبل، و4 ساعات خمول لكل من هو صاحب مؤسسة أو مدير في أي مؤسسة، من الدور الحالي. `SessionPolicy_test.go` |
| F11 | متوسطة | أصحاب المؤسسات والمديرون غير ملزمين بخطوة ثانية (ASVS 4.3.1). | أُصلحت في M2.6 (ADR-0054): إعداد `RequireSecondFactor` للمؤسسة مع مهلة 7 أيام، ثم `403 SECOND_FACTOR_REQUIRED` على الإجراءات الحساسة. `RouteTable_secondfactor_test.go`، `Modules/Identity/Test/E2E/SecondFactor_test.go` |
| F12 | متوسطة | لا رسالة للمستخدم بعد تغيير كلمة السر أو الخطوة أو المفتاح (ASVS 2.2.3). | أُصلحت في M2.6 (ADR-0054): `Modules/Alerts` يرسل للمستخدم بعد كل حدث من 10 أحداث لبيانات الدخول عند ضبط SMTP، في الخلفية وبلا أي سر. `Modules/Alerts/Test` |
| F13 | منخفضة | لا استرجاع لكلمة السر المنسية. | مقبولة في M2: يعيد المدير ضبط الخطوة الثانية، ومفاتيح المرور بلا كلمة سر والهويات المربوطة ما زالت تُدخل. مسار الاسترجاع قرار منتج. |
| F14 | منخفضة | مهاجم يعرف بريدًا يستطيع إبقاء الدخول بكلمة السر لهذا الحساب مقفلًا (ADR-0035). | مقبولة: القفل قصير ويتضاعف، والدخول بمفتاح المرور وبالمزوّد لا يُحسب على الحساب. |
| F15 | معلومة | تخمين مفاتيح API ليس له حد لكل عنوان. | مقبولة: مفاتيح 256 بت، وكل محاولة تكلّف بحثًا واحدًا في فهرس. |
| F16 | متوسطة | مراجعة مستقلة: المحدِّد عمل قبل فحص Origin، فكانت صفحة من موقع آخر تستطيع استهلاك حد مكتب خلف NAT واحد بطلبات no-cors. | أُصلحت: المحدِّد يعمل داخل الحارس. `TestACrossSitePageCannotSpendTheLimitOfAnOffice` |
| F17 | منخفضة | مراجعة مستقلة: العنوان المقفل ظل ينشئ صف `LoginThrottles` لكل بريد جديد، والدخول بكلمة السر والخطوة الثانية والدخول بمفتاح المرور لم يكن لها حد لكل شبكة. | أُصلحت: الحجز يتوقف عند أول مفتاح مقفل، والمسارات الثلاثة محدودة. `TestALockedAddressCreatesNoMoreThrottleRows`، `App/RateLimit_test.go` |
| F18 | منخفضة | مراجعة مستقلة: نسخة واحدة بساعة سريعة جمّدت نافذة حد الطلبات للنسخ الأخرى. | أُصلحت: النافذة والاحتفاظ يستخدمان ساعة قاعدة البيانات. `TestWithoutAClockTheDatabaseDecidesTheWindow`، `TestWithoutAClockRetentionUsesTheDatabaseTime` |
| F19 | منخفضة | مراجعة مستقلة: `DELETE /**/ FROM`، و`FROM"X"`، وتعليق بين الكلمات، و`WHERE true` أو `OR` في المستوى الأعلى في `Retention.sql` تجاوزت الـ hook. | أُصلحت: نحذف التعليقات، ونفحص الملف كله، وقالب دفعات واحد في `Retention.sql`. اختبارات hook لكل صيغة |
| F20 | منخفضة | مراجعة مستقلة: إعادة الإرسال استهلكت حدود الساعة قبل فحوص الوجود والدور وعدد الإرسالات. | أُصلحت: الحدود تُؤخذ داخل المعاملة بعدها. `TestAResendThatCannotHappenSpendsNothing` |
| F21 | منخفضة | مراجعة مستقلة: العميل الذي تجاوز الحد بالفعل ظل يكلّف upsert لكل طلب. | أُصلحت: ذاكرة لكل نسخة للمفاتيح الممنوعة حتى تنتهي النافذة. `TestAKeyKnownToBeOverTheLimitIsRefusedWithoutTheDatabase` |
| F22 | معلومة | مراجعة مستقلة: العينة الضابطة للتوقيت أثبتت 1 ms فقط على مسار أقل من ميلي ثانية. | عولجت: الضمان مكتوب الآن كـ max(10%, 300 µs) من الوسيط، وعينة ضابطة ثانية يجب أن تلاحظ 6 ms في الدخول. |
| F23 | معلومة | مراجعة مستقلة: رموز أجهزة التشغيل قبلت سطرًا جديدًا كما فعلت الرموز الأخرى. | أُصلحت: طول دقيق. `Core/RunnerAuth/RunnerAuth_test.go` (`TestTokens`) |
| F24 | معلومة | مراجعة مستقلة: لا شيء منع مسار كتابة عامًا جديدًا من الوصول بلا حد. | أُصلحت: `TestEveryPublicWriteIsRateLimitedOrAnExplicitException` |
| F25 | متوسطة | مراجعة M2.6: تسجيل الحدث وتنبيه بيانات الدخول عملا على سياق الطلب بعد الحفظ، فالعميل الذي قطع الاتصال في تلك اللحظة احتفظ بالتغيير بدونهما. | أُصلحت: المعالجان ينفصلان عن الطلب. `TestAnEventIsRecordedEvenWhenTheRequestIsGone`، `TestTheAlertIsSentEvenWhenTheRequestIsGone` |
| F26 | منخفضة | مراجعة M2.6: توسيع قاعدة الخطوة الثانية من المديرين إلى الجميع أعاد مهلة المديرين المتأخرين بالفعل. | أُصلحت: بداية منفصلة لأصحاب المؤسسات والمديرين. `TestWideningGivesOnlyNewlyCoveredRolesANewGracePeriodAndNarrowingKeepsIt` |
| F27 | منخفضة | مراجعة M2.6: كل تنبيه فتح goroutine واتصال SMTP خاصًا به بلا سقف. | أُصلحت: 32 إرسالًا معلّقًا بالأكثر، ونتجاهل الباقي مع تحذير. `TestPendingAlertsAreCapped` |
| F28 | منخفضة | مراجعة M2.6: تنبيه إعادة ضبط MFA أظهر شبكة المدير ومتصفحه للعضو. | أُصلحت: نص منفصل بدونهما. `TestTheMfaResetAlertLeavesOutTheAdminsNetworkAndBrowser` |
| F29 | معلومة | مراجعة M2.6: كان التنبيه يستطيع البدء بينما ينتظر الإيقاف التنبيهات المعلّقة، وقائمة الجلسات عرضت جلسات تجاوزت نافذة الخمول بـ 4 ساعات. | أُصلحت: خدمة التنبيهات تُغلق قبل أن تنتظر، والقائمة تستخدم نافذة الخمول الحالية. `TestNoAlertStartsAfterShutdown`، `TestTheSessionListLeavesOutSessionsPastTheCurrentIdleWindow` |
| F30 | معلومة | مراجعة M2.6: رد الخطأ من خادم البريد، الذي قد يذكر العنوان، يصل إلى سجل السيرفر. و`token.manage` و`job.cancel` لا يُمنعان بعد المهلة. | مقبولة (ADR-0054): السجل للمشغّل، كما في الدعوات. وإلغاء المفاتيح وإلغاء التشغيلات يقلّلان الوصول فقط. |

لا توجد نتيجة حرجة أو عالية مفتوحة.

## 6. للمراجع الخارجي

- ابدأ من فضلك بـ: ترتيب الحارس في `Core/Auth` (Origin، ثم Bearer، ثم الجلسة، ثم CSRF، ثم السياسة)، ومسار OIDC والربط، وقواعد صاحب المؤسسة وإعادة ضبط MFA مع التزامن، وسلسلة سجل الأحداث، ومنطق شبكة العميل في محدِّد الطلبات خلف الوكلاء.
- فجوات معروفة ستراها: النتائج من F9 إلى F15 أعلاه، وغياب الخروج عبر القناة الخلفية من المزوّدين (ADR-0039)، ونطاقات المشروع أو البيئة للمفاتيح (M4).
- سيرفر للاختبار: `make compose-up` بعد `deploy/compose/init.sh`، أو `deploy/smoke.sh <image>` لتشغيل مكتوب.

## 7. إعادة إنتاج الأدلة

| الفحص | الأمر |
|---|---|
| كل ما يشغّله CI | `make check` مع `DHOW_TEST_DATABASE_URL` يشير إلى PostgreSQL 18 |
| الـ fuzzing | `make fuzz FUZZTIME=60s` |
| التوقيت | `make timing` |
| الصورة من البداية إلى النهاية | `make smoke` |
| hooks المشروع | `make hooks-test` |
