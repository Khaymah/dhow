# ADR-0088: الـ contract لصيغة `0x01` وشيل القراية القديمة (M5.9أ)

- **الحالة:** مقبول. البنود 5 و10 و15 في [ADR-0082](0082-sealer-users-move-to-the-envelope.md) كانت مؤجّلة للـ contract، و[خطة M5](../M5-Secrets-Environments.md) §M5.9.
- **التاريخ:** 2026-11-05
- **بيكمّل:** [ADR-0082](0082-sealer-users-move-to-the-envelope.md) (الصيغتين والـ backfills وقفل الصيغة القديمة)، و[ADR-0077](0077-upgrades-migrations-and-update-policy.md) (expand/contract والقاعدة 8 قبل أول tag)، و[ADR-0081](0081-keyring-and-envelope-encryption.md) (الصيغة `0x03`).

## السياق
- ADR-0082 خلّى `keys.Column` يقرا `0x01` (مشتق من `DHOW_SECRET_KEY`) و`0x03` (الـ envelope) ويكتب `0x03` بس، وعمل backfill لكل عمود، وأجّل الـ contract لـ M5.9.
- لسه مفيش tag، يعني مفيش binary حد يرجع له. فالـ contract بيتكتب بـ `-- dhow:floor 0.0.0` وبيتراجع في الـ PR (القاعدة 8).
- طول ما القراية القديمة موجودة، اللي معاه كتابة في القاعدة و`DHOW_SECRET_KEY` يقدر يزرع قيمة `0x01` في عمود الـ backfill بتاعه لسه ماخلصش (البند 15 كان بيقفل ده بحالة لكل عمود في الذاكرة). والـ `appcrypto.NewSealer` لسه مسموح في `Core/Keys/Values.go` وفي الاختبارات.

## القرار
1. **الأعمدة التمانية اللي بتتقري بالصيغتين** (كل مكان قيمة مسجّل في `App.ValueRegistry()`) بتاخد `CHECK` على الصيغة:

   | العمود | الـ backfill | الـ CHECK |
   |---|---|---|
   | `"TotpFactors"."SecretSealed"` | `keys.envelope.TotpFactors.SecretSealed` | `CK_TotpFactors_SecretSealed_Envelope` |
   | `"InstanceSettings"."Secrets"` | `keys.envelope.InstanceSettings.Secrets` | `CK_InstanceSettings_Secrets_Envelope` |
   | `"OidcProviders"."Secret"` | `keys.envelope.OidcProviders.Secret` | `CK_OidcProviders_Secret_Envelope` |
   | `"Repositories"."DeployKeySealed"` | `keys.envelope.Repositories.DeployKeySealed` | `CK_Repositories_DeployKeySealed_Envelope` |
   | `"Repositories"."PreviousDeployKeySealed"` | `keys.envelope.Repositories.PreviousDeployKeySealed` | `CK_Repositories_PreviousDeployKeySealed_Envelope` |
   | `"Repositories"."WebhookSecretSealed"` | `keys.envelope.Repositories.WebhookSecretSealed` | `CK_Repositories_WebhookSecretSealed_Envelope` |
   | `"SecretVersions"."ValueSealed"` | `keys.envelope.SecretVersions.ValueSealed` | `CK_SecretVersions_ValueSealed_Envelope` |
   | `"RegistryCredentials"."PasswordSealed"` | `keys.envelope.RegistryCredentials.PasswordSealed` | `CK_RegistryCredentials_PasswordSealed_Envelope` |

   - الـ CHECK هو `col IS NULL OR (octet_length(col) >= 1 AND get_byte(col, 0) = 3)`، يعني بيقبل `NULL` (الأعمدة اللي nullable) وبيمنع `get_byte` على قيمة فاضية.
   - `"GitConnections"."TokenSealed"` لسه بره (مفيش كود بيكتبه، ADR-0082 بند 2).
2. **خطوتين، زي 00061/00062 و00063/00064:**
   - `00065_sealed_format_contract.sql`: `-- dhow:phase contract` و`-- dhow:floor 0.0.0`. بيبدأ بـ guard وبعده `ADD CONSTRAINT ... NOT VALID` لكل عمود.
   - `00066_sealed_format_contract_validate.sql`: `NO TRANSACTION` و`VALIDATE CONSTRAINT` لكل واحد. الـ `NOT VALID` بيمنع أي كتابة جديدة من لحظتها، والـ `VALIDATE` بيمسح الصفوف الموجودة من غير قفل كتابة طويل.
3. **الـ guard بيستدعي `"RequireBackfillDone"` بس لو العمود فيه قيمة مش `0x03`.** القاعدة الجديدة (تنصيب جديد) مفيهاش صف في `"Backfills"` أصلًا، لأن الـ backfill بيتعمل لما السيرفر يقوم والـ migrations بتعدّي قبله. لو الـ guard بيطلب `DoneAt` دايمًا، كل تنصيب جديد كان هيقف بـ `BACKFILL_NOT_DONE`. فالـ guard بيدوّر الأول على قيمة قديمة (`octet_length < 1` أو `get_byte <> 3`) وبيطلب الـ backfill بس لو لقاها. النتيجة:
   - تنصيب جديد أو قاعدة مفيهاش قيم قديمة: بيعدّي من غير أي backfill.
   - قيمة قديمة والـ backfill ماخلصش: `BACKFILL_NOT_DONE` باسم الـ backfill، والعلاج تشغيل الإصدار اللي قبله لحد ما `migrate status` يقول إنه خلص.
   - الـ backfill خلص وفضلت قيمة مابتتحوّلش (مفتاح بياناتها اتمسح، أو صيغة `0x02` من مسودة M5.1): الـ guard بيعدّي لأن الـ backfill خلص، وخطوة `VALIDATE` في 00066 بتقف بـ `check_violation`. الأمر هنا مسح القيمة أو إعادة حفظها.
4. **القراية القديمة اتشالت:**
   - `keys.Column.Open` بيقبل `0x03` بس. أي byte أول تاني (`0x01` و`0x02` والصفر والفاضي) `ErrSealed`.
   - اتشال من `Core/Keys`: `LegacyFormat` و`IsLegacy` والـ `Sealer` القديم في `Column` و`Values.CloseLegacy` و`LegacyClosed` و`BackfillDone` و`LoadLegacyState`، والمعامل التاني في `keys.NewValues`. بند 15 في ADR-0082 (حالة الـ backfill لكل عمود) كان بيخدم رفض `0x01` بس، فاتشال كله: القاعدة نفسها بقت بترفض الكتابة، والكود بيرفض القراية.
   - `DHOW_SECRET_KEY` بيفضل لمفاتيح التوقيع والـ HMAC (بند 6 في ADR-0082): الـ runner plane والتسجيل والتحدّي وتوكنات API وأكواد الطوارئ وCSRF وflow الـ OIDC ومنح الـ artifacts. مفيش قيمة متخزّنة بتتفتح بيه.
5. **الـ backfills:**
   - الـ jobs اللي بتحوّل `0x01` لـ `0x03` (`Registry.BackfillJobs`) اتشالت، لأنها ماعادتش تقدر تفتح القيمة القديمة، فكانت هتفضل ديكور بيعدّ صفر. والسيرفر ماعادش يشغّل `Backfill` runner في الإقلاع (`StartBackfills`).
   - اتبقى `Core/Backfill` نفسه (الـ runner والجدول `"Backfills"` والدالة `"RequireBackfillDone"`) لأنه U4 من خطة الترقيات، والـ guard بيعتمد عليه، وأي تحويل كبير جاي هيستخدمه. و`migrate status` بيعرض الصفوف اللي في `"Backfills"` فعلًا (قاعدة اترقّت بتفضل تشوف تحويلاتها المنتهية).
   - `Location` اتشال منه `CountLegacy` وبارامتر `legacyOnly` من `Scan`، وqueries `Count...Legacy...` وشرط `legacy_only` من الـ sqlc. اللي فضل `Scan` و`Replace` و`CountUnder` لـ `keys rekey`.
6. **الـ hook والـ lint:**
   - `NewSealer` مسموح في `apps/server/internal/Core/Crypto/` بس. اتشال الاستثناء بتاع `Core/Keys/Values.go` والاختبارات في `crypto_rules.py` وفي `forbidigo` بتاع `.golangci.yml`، واختبارات الـ hook اتعدّلت.
   - `Core/Crypto.Sealer` نفسه لسه موجود بـ اختباراته: مفيش حد في الإنتاج بيستخدمه بعد النهارده، وشيله قرار منفصل (مش جزء من الـ contract).
7. **الاختبارات:**
   - `Core/Keys`: `0x01` (بجسم envelope وبجسم بحجم القديم وبـ byte لوحده) `ErrSealed`.
   - `Database/Migrations`: `BACKFILL_NOT_DONE` لما قيمة قديمة والـ backfill ماخلصش، و`check_violation` في `VALIDATE` لو خلص وفضلت قيمة، والنجاح بعد الإصلاح، ورفض كتابة `0x01` بعد الـ contract، والتنصيب الجديد من غير أي backfill.
   - `App`: كل مكان قيمة في `ValueRegistry()` ليه guard وCHECK و`VALIDATE` في الملفين، و`0x01` متزرعة في TOTP بترفضها القاعدة، ولو اتشال الـ CHECK بيترفض الدخول برضه، وكل القيم بتشتغل بعد تغيير `DHOW_SECRET_KEY`.

## فحص التسريب (بند 11 في ADR-0082 يتوسّع)
8. **`App/LeakSweep` و`deploy/m4-demo/leak-sweep.sh`:**
   - الـ demo (`run.sh`) بيخزّن سر لكل نطاق (المؤسسة والمشروع والبيئة `staging`) وكلمة سر registry، وبيكتب قيمهم في ملف الـ needles بأسماء `organization secret` و`project secret` و`environment secret` و`registry password`.
   - الفحص بيفشل لو نطاق ماتخزّنش فيه حاجة (فحص بيثبت مفيش حاجة)، أو لو القيمة في ملف الـ needles مش هي اللي القاعدة محتفظة بيها مختومة (بيتقارن بالقيمة بعد الفتح).
   - أسماء الـ needles بتقول النوع (`organization secret of row ...`) مش اسم العمود، فالنتيجة بتوضّح مين اتسرّب.
   - KEKs (`keyring key N`) وDEKs بعد الفك (`data key N after unwrapping`) كانوا موجودين، وبقى ليهم اختبار «مزروع»: `TestEachKindOfCredentialIsCaughtWhenPlanted` بيزرع كل نوع (DEK وKEK وسر من كل نطاق وكلمة سر registry) في ملف لوحده، بالشكل الخام وبـ base64، ويتأكد إن الفحص بيلاقيه باسمه وفي الملف ده بس.
   - `leak-sweep.sh` بيزرع كمان سر البيئة وكلمة سر الـ registry (base64 في `Authorization`) والـ KEK الأول من ملف السلسلة، ويتأكد إن كل واحد بيتلقط، وبيشغّل اختبار الزرع ده قبل الفحص الحقيقي.

## البدائل اللي اترفضت
- **الـ guard من غير شرط (`RequireBackfillDone` لكل عمود دايمًا):** بيكسر كل تنصيب جديد. الـ tag لسه مفيش، فمفيش قاعدة قديمة محتاجة الـ guard الأعمى، والـ guard الشرطي بيغطّي الاتنين.
- **إبقاء الـ backfill jobs «no-op»:** كانت هتفضل بتعدّ صفوف مفيش منها ولا هتقدر تفتحها لو اتزرعت. الـ guard والـ CHECK بيغطّوا الحالة، والكود الميت مابيتصلحش.
- **`CHECK` واحد `VALID` في خطوة واحدة:** بيمسك الجدول وقت المسح (M4 في `migration_rules.py`). الخطوتين زي 00061/00062.
- **شيل `Core/Crypto.Sealer` معاه:** مش جزء من قفل الصيغة، ومحتاج ADR لو حد عايز يستخدمه تاني. اتأجّل.

## النتايج
- القاعدة نفسها بترفض أي قيمة متشفّرة مش `0x03` في الأعمدة التمانية، فزرع قيمة `0x01` ماعادش ينفع حتى لو معاه `DHOW_SECRET_KEY` وكتابة في القاعدة.
- مفيش حاجة في السيرفر بتفتح قيمة بـ `DHOW_SECRET_KEY`.
- ترقية من نسخة dev فيها قيم `0x01` لازم تعدّي على الإصدار اللي فيه الـ backfill أولًا. قبل أول tag مفيش حد كده.
- لما التحويل الكبير الجاي يتعمل، هيستخدم `Core/Backfill` من جديد بـ guard شرطي زي ده.
