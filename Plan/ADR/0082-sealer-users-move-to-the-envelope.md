# ADR-0082: مستخدمي `Sealer` بينتقلوا للتشفير المغلّف من خلال «أماكن قيم» مسجّلة، و`keys rekey`، والـ backfills في الخلفية (U4)

- **الحالة:** مقبول. القرارين 6 و7 في [خطة M5](../M5-Secrets-Environments.md#القرارات-المفوَّضة-كل-واحد-بيبقى-adr-في-أول-مرحلته) مفوَّضين ومكتوبين بالتوصية، و`keys rekey` شرط من المراجعة الأمنية لـ M5.1، وU4 من [خطة الترقيات](../Upgrades-and-Help.md#u4-الـ-backfills-في-الخلفية).
- **التاريخ:** 2026-11-03
- **بيكمّل:** [ADR-0081](0081-keyring-and-envelope-encryption.md) (السلسلة والـ envelope، والنقطة 14 فيه)، و[ADR-0077](0077-upgrades-migrations-and-update-policy.md) (expand/contract والقاعدة 8 قبل أول tag)، و[ADR-0063](0063-instance-settings-in-the-database.md) و[ADR-0072](0072-webhooks-and-git-credentials.md) (الـ labels والـ contexts الموجودة).

## السياق
- لحد M5.1 كل قيمة متشفّرة في القاعدة بتعدّي على `Core/Crypto.Sealer` بمفتاح مشتق من `DHOW_SECRET_KEY` (صيغة `0x01`). M5.1 بنى `Core/Keys` (صيغة `0x03`) من غير ما ينقل حد.
- المراجعة الأمنية لـ M5.1 قالت إن تدوير الـ KEK مابيحميش من سلسلة اتسرّبت مع نسخة قاعدة قديمة، و`Destroy` مابيمنعش نسخة أقدم، فلازم أمر يغيّر الـ DEKs نفسها ويعيد ختم كل قيمة.
- إعادة ختم «كل قيمة» محتاجة الكود يعرف كل عمود متشفّر وصاحبه والـ label والـ context بتوعه. و`Core` ممنوع يستورد `Modules`.
- تحويل كل القيم القديمة محتاج backfill في الخلفية، وU4 لسه مااتعملش.

## القرار
1. **الأعمدة اللي اتنقلت، بنفس الـ label والـ context بالظبط** (عشان قيمة اتنقلت لصف تاني أو عمود تاني ماتتفتحش):

   | العمود | الصاحب | الـ label | الـ context |
   |---|---|---|---|
   | `"TotpFactors"."SecretSealed"` | `user` (`"UserId"`) | `dhow/totp-secret/v1` | `totp:<UserId>` |
   | `"InstanceSettings"."Secrets"` | `instance` | `dhow/instance-settings/v1` | `instance-settings:<Group>` |
   | `"OidcProviders"."Secret"` | `instance` | `dhow/instance-settings/v1` | `instance-settings:oidc:<ProviderId>` |
   | `"Repositories"."DeployKeySealed"` و`"PreviousDeployKeySealed"` | `project` (`"ProjectId"`) | `dhow/git-deploy-key/v1` | `deploy-key:<Id>` |
   | `"Repositories"."WebhookSecretSealed"` | `project` (`"ProjectId"`) | `dhow/git-webhook-secret/v1` | `webhook-secret:<Id>` |

   - الـ contexts هي اللي في الكود فعلًا (`totp:` و`instance-settings:` قبل الـ id)، مش النص المختصر في جدول الخطة.
   - صف كل عمود فيه id الصاحب (`"UserId"` و`"ProjectId"`)، والـ instance مالوش id، فالـ backfill والـ rekey بيعرفوا الصاحب من الصف نفسه من غير join.
2. **اللي اتأجّل:** `"GitConnections"."TokenSealed"` موجود في الـ schema من `00043` بس مفيش كود بيكتبه ولا label ليه، لأن M4.6 مااتعملش. و`"GitHubApps"` مش موجود (M4.5). الاتنين لما يتعملوا لازم يسجّلوا أماكنهم (الـ hook بيرفض `NewSealer`)، واختبار في `App` بيفشل لو عمود `...Sealed` جديد مش مسجّل، والاستثناء الوحيد `TokenSealed` لحد M4.6.
3. **«مكان القيمة»** (`keys.Location`): كل عمود بيسجّل `keys.Spec` (الجدول والعمود، ونوع الصاحب، والـ label، وأكبر نص، وأكبر قيمة في الـ CHECK)، ومعاه أربع queries بـ sqlc: `Scan` (بالـ `"Id"` كـ cursor و`FOR UPDATE SKIP LOCKED`، والقديم بس لو طلبنا)، و`Replace` (compare-and-swap على القيمة القديمة)، و`CountLegacy`، و`CountUnder` (القيم اللي تحت DEKs معيّنة بـ `substring(col FROM 2 FOR 16)`). `keys.RowLocation[R]` بيلف الـ queries المتولّدة، فكل موديول بيكتب الـ closures بس.
   - **مكان التسجيل:** كل موديول فيه `ValueLocations()` (`session` و`sources`، و`Core/Settings`)، و`App.ValueRegistry()` بيجمعهم في `keys.Registry`. كده `Core/Keys` مابيستوردش أي موديول، والموديول مابيستوردش موديول تاني، وApp هو اللي بيعرف الكل زي أي حاجة تانية.
   - **الرفض:** مكان مكرر، أو `MaxPlaintext + 77 > MaxSealed`، بيوقف الإقلاع. واختبار بيقرا الـ CHECK الحقيقي من `pg_get_constraintdef` ويتأكد إنه `MaxSealed` بالظبط، وإن أكبر قيمة حقيقية (TOTP 20، وsecret الـ webhook 43، والـ deploy key بالـ PEM، وكلمة سر SMTP 1024 بأسوأ escaping، وسر OIDC 1024) أصغر من `MaxPlaintext`.
4. **`keys.Column`** بيقرا الصيغتين ويكتب `0x03` بس (القرار 7): `0x03` بالـ envelope بالصاحب والـ label والـ context، و`0x01` بـ `appcrypto.Sealer` مشتق من `DHOW_SECRET_KEY` بنفس الـ label، وأي byte تاني `ErrSealed`. وبيرفض صاحب من نوع تاني (`ErrWrongOwnerKind`) ونص أكبر من `MaxPlaintext`. الموديولات بتاخد `*keys.Column` (أو `*keys.Values`) بدل `*appcrypto.Sealer`، وفيه مكان واحد بس بيعمل `NewSealer` للقراية القديمة: `Core/Keys/Values.go`.
5. **الخطوات قبل أول tag:** القراية والكتابة والـ backfill في PR واحد (القاعدة 8 في `Upgrades-and-Help.md`)، لأن مفيش binary حد يرجع له. الـ contract (`CHECK (get_byte(col, 0) = 3)` بـ `NOT VALID` و`VALIDATE`، وشيل القراية القديمة و`NewSealer` من `Values.go`، بـ guard `RequireBackfillDone` لكل عمود و`-- dhow:floor 0.0.0`) في M5.9. بعد أول tag لازم release كامل بين كل خطوة.
6. **`DHOW_SECRET_KEY` بعد الـ backfill** (القرار 6): مابيفتحش أي قيمة متخزّنة، وبيشتق بس مفاتيح التوقيع والـ HMAC: الـ runner plane والتسجيل والتحدّي، وتوكنات API، وأكواد الطوارئ، وCSRF، وflow الـ OIDC، ومنح الـ artifacts. اختبار بيحوّل كل القيم لـ `0x01`، ويشغّل الـ backfill، ويقوّم سيرفر بمفتاح تاني على نفس القاعدة، وTOTP والـ checkout وتوقيع الـ webhook والإعدادات بيشتغلوا. تغييره لسه بيعيد تسجيل الـ runners ويبطّل التوكنات وأكواد الطوارئ، فتدويره بعد أول نسخة بإجراء لوحده، ومكتوب في `docs/install/keys.md`.
7. **U4: `Core/Backfill`:**
   - جدول `"Backfills"` (`"Name"` unique، و`"LastKey"`، و`"Processed"` و`"Failed"` و`"Remaining"` و`"Passes"`، و`"DoneAt"`).
   - كل job بيتسجّل باسمه (`keys.envelope.<Table>.<Column>`، واحد لكل عمود)، وكل دفعة (1000 صف) transaction لوحدها: `pg_try_advisory_xact_lock` باسم الـ job، وقفل الصف بتاعه في `"Backfills"`، والدفعة، وتحديث الـ cursor والعدّادات، كلهم مع بعض. فالقتل في النص بيرجّع الدفعة وتقدّمها مع بعض، ومفيش صف بيتحوّل مرتين.
   - الـ cursor لازم يتقدّم (`ErrKeyNotAhead`)، والصف المقفول اللي `SKIP LOCKED` عدّاه بيتلقط في المرور الجاي: آخر المرور بيعدّ اللي فاضل من غير `SKIP LOCKED`، ولو مش صفر الـ cursor بيرجع للأول ومابيتعلّمش `DoneAt`.
   - القيمة اللي مابتتفتحش بتتعدّ في `"Failed"` وتتسجّل في الـ log (العمود والصف بس) وتفضل زي ما هي، والـ backfill مابيخلصش لحد ما تتصلّح، فالـ contract مايقدرش يكسرها.
   - throttle بين الدفعات (100ms)، وكل 30 ثانية مرور جديد على الـ jobs اللي ماخلصتش. والتقدّم في `dhow-server migrate status`. و«عن داو» مستنية U6.
   - **الـ guard:** function في القاعدة `"RequireBackfillDone"(name)` بترمي `BACKFILL_NOT_DONE`، والـ contract بيبدأ بيها. ومعاها `backfill.RequireDone` في Go بنفس الكود.
   - **الاختبار:** 100,000 صف، وworker في process لوحده بيتقتل بـ `SIGKILL` 3 مرات، وفي الآخر كل صف اتحوّل مرة واحدة بالظبط و`"Processed"` = 100,000، وp99 قراية الصفوف طول الوقت أقل من الضعف.
8. **`keys rekey`:**
   - **الحالة `superseded`:** الـ DEK القديم مابيتمسحش على طول. في transaction لكل دفعة أصحاب، الـ DEK القديم بيبقى `superseded` (بيتقري بس مابيختمش) ومعاه DEK جديد `active` ملفوف بالـ KEK الـ active، فأي كتابة بعدها بتروح للجديد على طول.
   - **إعادة الختم:** مرور على كل الأماكن المسجّلة، وكل قيمة تحت DEK `superseded` بتتفتح وتتختم من جديد بنفس الـ label والـ context، وتتكتب بـ compare-and-swap.
   - **المسح:** بعدها انتظار `--grace` (10 ثواني) عشان الكتابات اللي بدأت بالقديم تخلص، وعدّ بـ `CountUnder`، ومرور تاني لو فيه حاجة (3 مرات بالكتير، وإلا `REKEY_NOT_DRAINED`)، وبعدين الـ DEKs القديمة بتبقى `destroyed` في transaction واحدة مع `"DoneAt"`.
   - **الاستكمال:** التقدّم في `"KeyRekeys"` (`"Cutoff"` والعدّادات)، والتشغيل الجاي بيكمّل نفس الـ run: الـ DEKs اللي اتعملت بعد الـ `"Cutoff"` مابتتغيّرش تاني.
   - **القفل:** session advisory lock على connection ثابتة (`REKEY_RUNNING`)، والأمر بالـ argv بس.
   - **القيمة المكسورة:** قيمة تحت DEK `superseded` ومابتتفتحش بتوقف الأمر بـ `REKEY_UNREADABLE` بالعمود والصف، ومفيش DEK بيتمسح وفيه قيمة لسه بتستخدمه.
   - **الـ audit:** `system.keys.rekeyed` بخطورة عالية بالأعداد بس.
   - **مع الباقي:** إعادة لف الـ KEK و`Destroy(owner)` و`--discard-unreadable` وعدّاد `keys status` بيشملوا `superseded`، فالـ KEK مابيتقاعدش وفيه DEK `superseded` عليه، ومسح الصاحب بيمسح الاتنين.
   - **الاختبار:** سلسلة قديمة ونسخة `"DataKeys"` من قبل الـ rekey مابيفتحوش أي قيمة حالية، والسلسلة القديمة مع القاعدة الحالية مابتفكّش الـ DEK الجديد الملفوف بالـ KEK الجديد.
9. **الـ migrations** (كلهم `-- dhow:phase expand`):
   - `00053`: `"Backfills"` و`"RequireBackfillDone"`.
   - `00054`: CHECK الحالة في `"DataKeys"` بقى `('active', 'superseded', 'destroyed')` بـ `NOT VALID` (الشيل بيوسّع بس، والـ binary القديم بيقرا `superseded` كـ مش ممسوح)، و`"SupersededAt"`، و`"KeyRekeys"`.
   - `00055`: `VALIDATE` في ملف `NO TRANSACTION`.
10. **الـ hook:** `appcrypto.NewSealer` مرفوض في أي ملف Go في `apps/server` بره `Core/Crypto` و`Core/Keys/Values.go` والاختبارات (`_test.go` و`/Test/`)، بأي اسم للـ import. الاختبارات مسموحة لأنها بتختم قيم `0x01` عشان تثبت إن القراية القديمة شغّالة، والاستثناء ده بيتشال مع الـ contract.
11. **فحص التسريب:**
    - `App/LeakSweep` (الـ demo): بيفتح كل قيمة في كل مكان مسجّل بالصيغتين بالسلسلة و`DHOW_SECRET_KEY`، وبيفشل لو قيمة مابتتفتحش أو عمود متشفّر فيه قيم ومش مسجّل، وبيدوّر على النصوص وعلى كل DEK بعد فكه وعلى مفاتيح السلسلة. و`deploy/m4-demo/leak-sweep.sh` بيدّيه `DHOW_LEAK_SWEEP_KEYRING_FILE`.
    - اختبار في `App` بيعمل dump لكل الجداول بـ `COPY` بعد الـ backfill ويدوّر على نفس الحاجات، ومعاه DEK مزروع عشان يتأكد إن الفحص بيلاقي.

## البدائل اللي اترفضت
- **السجل في `Core/Keys` بأسماء جداول ثابتة:** `Core` كان هيعرف جداول الموديولات، والـ SQL كان هيتبني كنص. والسجل اللي بيتجمّع في App بيخلّي كل موديول مسؤول عن queries جداوله بـ sqlc.
- **migration واحد بيحوّل كل القيم:** محتاج المفاتيح جوه الـ migration ويقفل الجداول وقت الإقلاع، وده عكس ADR-0077.
- **إعادة ختم كل صاحب في transaction واحدة من غير `superseded`:** كتابة بدأت بالـ DEK القديم وخلصت بعد المسح كانت هتسيب قيمة مابتتفتحش، والصاحب اللي قيمه كتير كان هيقفل الصفوف وقت طويل.
- **تحويل `0x01` جوه الـ rekey:** الـ rekey بيشتغل من غير `DHOW_SECRET_KEY`، والقيم القديمة مش تحت أي DEK أصلًا، فالـ backfill هو اللي بيحوّلها.
- **الـ backfill بـ cursor بس من غير عدّ في الآخر:** صف كان مقفول وقت دفعته كان هيتعدّى للأبد، والـ contract كان هيكسره.

## النتايج
- مفيش حاجة متخزّنة بتعتمد على `DHOW_SECRET_KEY` بعد الـ backfill، وكل عمود متشفّر جديد لازم يتسجّل كمكان قيمة عشان يكتب أصلًا.
- بعد تسريب السلسلة: تدوير ثم `keys rekey`، ودليل الطوارئ في `docs/install/keys.md` اتعدّل.
- `deploy/upgrade-test` (U3) لسه مش موجود، فسطر القبول بتاعه مستني U3.
- الـ contract لصيغة `0x01` في M5.9، والـ `TokenSealed` مع M4.6.
