# ADR-0081: سلسلة مفاتيح في ملف منفصل، وDEK لكل صاحب، والتدوير بإعادة اللف بس

- **الحالة:** مقبول. القرارات من 1 لـ 5 في [خطة M5](../M5-Secrets-Environments.md#القرارات-المفوَّضة-كل-واحد-بيبقى-adr-في-أول-مرحلته) مفوَّضة ومكتوبة بالتوصية، ومكان الـ KEK [قرار Sabry 1](../M5-Secrets-Environments.md#قرارات-sabry-2026-11-03).
- **التاريخ:** 2026-11-03
- **بيكمّل:** [ADR-0014](0014-secrets-management.md) (envelope وKEK بإصدارات، وVault وKMS في Enterprise)، و[ADR-0045](0045-secrets-transport-and-masking.md) (توصيل الأسرار للـ runner بـ HPKE)، و[ADR-0077](0077-upgrades-migrations-and-update-policy.md) (كله ببلاش، وexpand/contract، و`"ServerInstances"`).

## السياق
- كل حاجة متشفّرة في القاعدة النهارده بتعدّي على `Core/Crypto.Sealer` بمفتاح مشتق من `DHOW_SECRET_KEY`. يعني مفتاح واحد في متغيّر بيئة، ومفيش تدوير من غير ما كل القيم تتعاد، ومفيش مسح بالتشفير لما بيئة أو مشروع يتمسح.
- M5 بيضيف أسرار المؤسسات والمشاريع والبيئات، وده محتاج حماية أقوى من غير ما يبقى فيه شرط على خدمة أو جهاز مدفوع (ADR-0077).
- M5.1 بيبني الطبقة بس. نقل مستخدمي `Sealer` (TOTP وإعدادات السيرفر ومزوّدي الدخول ومفاتيح المستودعات) في M5.2، و`Sealer` بيفضل شغّال زي ما هو لحد كده.

## القرار
1. **طبقتين** (القرار 1):
   - **KEK** بـ 32 byte عشوائي ورقم إصدار، عمره ما بيدخل القاعدة، وبييجي من `KeyProvider` (`Name` و`Versions` و`CheckValue` و`Wrap` و`Unwrap`). في M5 فيه provider الملف بس، ومعاه `Fake` للاختبارات.
   - **DEK** بـ 32 byte عشوائي لكل صاحب: `instance` و`organization:<id>` و`project:<id>` و`environment:<id>` و`user:<id>`. بيتعمل lazy أول ما الصاحب يختم قيمة (`INSERT ... ON CONFLICT DO NOTHING` وبعده `SELECT`)، وبيتخزّن في `"DataKeys"` ملفوف بـ AES-256-GCM و nonce عشوائي: `nonce (12) ‖ ciphertext (32) ‖ tag (16)`، يعني 60 byte بالظبط (CHECK).
   - **AAD لف الـ DEK:** `dhow/dek/v1 ‖ 0x00 ‖ DataKeyId ‖ len(OwnerKind) ‖ OwnerKind ‖ OwnerId (16) ‖ KekVersion (uint32)`. الـ DEK المتنقل لصف تاني أو صاحب تاني أو إصدار تاني مابيتفكّش. والـ `DataKeyId` بيتعمل في Go (UUIDv7) عشان يدخل الـ AAD قبل الـ INSERT.
   - **`Destroy(owner)`** بيصفّر `"Wrapped"` ويخلّي الحالة `destroyed` (مسح بالتشفير). القيم بعدها بترجع `ErrShredded`، وهو نوع من `ErrSealed`. والختم الجديد لنفس الصاحب بيعمل DEK جديد.
2. **صيغة القيمة `0x03`** (القرار 2، واتعدّل بعد المراجعة الأمنية): `0x03 ‖ DataKeyId (16) ‖ salt (32) ‖ nonce (12) ‖ ciphertext ‖ tag (16)`، يعني 77 byte زيادة. كل قيمة بمفتاح فرعي = HKDF-SHA256(DEK، salt عشوائي، info = `dhow/value/v2` ‖ label)، و nonce عشوائي تحت مفتاح جديد كل مرة، فمفيش حد 2^32 لكل DEK (حد الـ nonce العشوائي في GCM). الـ `0x02` (من غير salt) ماطلعش في أي نسخة ومفيش قيم بيه، فالرقم اتغيّر من غير migration، و`0x01` بتاع `Sealer` بيفضل مختلف. والـ AAD = `0x03 ‖ DataKeyId ‖ uint16(len(label)) ‖ label ‖ context`، والطول بيمنع إن label وcontext يتبادلوا حروف، والـ context الفاضي مرفوض (`ErrBadContext`). و`Open` بياخد الصاحب المتوقع، ولو صاحب الـ `DataKeyId` مختلف بيرجّع `ErrSealed` حتى لو الـ context اتزوّر. القيود في M5.2: TOTP 20 byte بيبقى 97 (القيد 29 لـ 128).
3. **قيمة فحص لكل إصدار والإقلاع بيقف** (القرار 3):
   - `"CheckValue"` = HMAC-SHA256(KEK، `dhow/kek-check/v1`)، والمقارنة بـ `hmac.Equal`.
   - عند الإقلاع وجوه transaction تحت advisory lock: كل إصدار مش `retired` لازم يبقى في الملف بنفس قيمة الفحص. الإصدار اللي في الملف ومش في القاعدة بيتسجّل `pending`، ولو مفيش إصدار `active` (تنصيب جديد)، أعلى إصدار في الملف بيبقى `active` في نفس الـ transaction.
   - الفشل بيرجع `keys.Error` بكود ثابت (`KEYRING_MISSING` و`KEYRING_INSECURE` و`KEYRING_MALFORMED` و`KEYRING_MISSING_VERSION` و`KEYRING_MISMATCH`) ورقم الإصدار، والـ binary بيخرج بـ **79** قبل ما يسمع على أي port. اخترنا رقم غير `78` (الـ schema أجدد من الـ binary) عشان السكربتات تفرّق بينهم، والـ unit فيه `RestartPreventExitStatus=78 79`.
   - `/health/ready` فيه فحص `keyring` بيتأكد إن الإصدارات `active` و`retiring` لسه في السلسلة اللي السيرفر حمّلها، والـ `pending` مش شرط هنا عشان سيرفر لسه ماخدش الملف الجديد مايطلعش من الـ load balancer.
4. **التفعيل بأمر وبيتأكد من كل instance حي** (القرار 4):
   - كل instance بيكتب أرقام إصداراته في `"ServerInstances"."KekVersions"` مع التسجيل بتاع U2.
   - `keys activate <n>` جوه transaction: `n` لازم يكون في القاعدة (`KEK_NOT_FOUND`)، ومش `active` (`KEK_ALREADY_ACTIVE`) ولا `retired` (`KEK_RETIRED`)، وفي ملف الأمر نفسه بنفس قيمة الفحص، وفي `"KekVersions"` بتاع كل instance حي. «حي» = نبضة في آخر 90 ثانية، نفس `ServerInstanceStaleAfter` بتاع U2 بدل الدقيقتين اللي في الخطة، عشان يبقى فيه تعريف واحد. لو instance ناقصه، `KEK_NOT_EVERYWHERE` بأسماء الـ instances ومفيش أي تغيير.
   - بعدها `n` بيبقى `active` والقديم `retiring`، والتفعيل بيتسجّل في الـ audit (`system.keys.activated`).
5. **التدوير بيعيد لف `"DataKeys"` بس** (القرار 5): worker في كل سيرفر كل 30 ثانية، كل دفعة 500 صف `FOR UPDATE SKIP LOCKED` في transaction لوحدها تحت `pg_try_advisory_xact_lock`. بيفك بالإصدار القديم ويلف بالجديد ويحدّث الصف بشرط إن إصداره ماتغيّرش. القتل في النص بيرجّع الدفعة اللي شغّالة بس، والقراية شغّالة طول الوقت لأن كل صف بيقول إصداره. ولما مفيش صف على الإصدار `retiring`، بيبقى `retired`. الـ DEK نفسه مابيتدوّرش في M5.
6. **مكان الـ KEK** (قرار Sabry 1): ملف سلسلة منفصل عن `DHOW_SECRET_KEY`:
   - **الصيغة:** سطر لكل إصدار `kek <n> <base64url بدون حشو لـ 32 byte>`، 16 إصدار بالكتير، والأرقام تصاعدية من غير أصفار في الأول، ومسافة واحدة. أي سطر غلط، أو مفتاح قصير، أو مفتاح مش عشوائي (16 قيمة مختلفة على الأقل، زي `looksRandom`)، أو نفس المفتاح تحت رقمين، بيوقف الإقلاع. الرسالة فيها رقم السطر والإصدار بس.
   - **الملف:** ملف عادي مش symlink (`Lstat` و`O_NOFOLLOW` و`SameFile`)، و64 KiB بالكتير، وصاحبه اللي مشغّل السيرفر أو root، ومفيش صلاحية لحد غيره (`0600` أو أقل). مع systemd credential بس بنقبل bit القراية للمجموعة، لأن systemd ممكن يدّي الصلاحية بـ ACL والـ mask بيبان في المجموعة، والمجلد بتاعه خاص بالخدمة.
   - **المصادر:** systemd `LoadCredential=dhow-keyring:/etc/dhow/credentials/dhow-keyring`، أو Compose `secrets:` من `./secrets/dhow-keyring` (صاحبه مستخدم الحاوية `65532`، و`init.sh` بيعمل `chown` بـ root أو `sudo`)، أو `DHOW_KEYRING_FILE` بمسار كامل. مصدرين مع بعض مرفوض، و`DHOW_KEYRING` كمتغيّر بيئة مرفوض لأن `docker inspect` بيكشفه.
   - **`keys generate`** بيضيف سطر لآخر الملف بـ `O_APPEND` بعد ما يفحصه، فالصاحب والصلاحيات بيفضلوا زي ما هم، ومش محتاج كتابة في المجلد (secret الـ Compose مربوط ملف لوحده). كتابة واحدة قصيرة يا إما بتوصل كاملة يا إما بتسيب سطر الـ parser بيرفضه في الإقلاع الجاي.
   - **KMS خارجي** (Vault Transit وKMS السحابة وHSM) ورا نفس `KeyProvider` في `ee/` مع M8، وعمره ما بيبقى شرط. العمود `"Provider"` بـ CHECK على شكل الاسم مش على قايمة، عشان provider جديد مايحتاجش contract.
7. **الأوامر بالـ argv بس:** `keys init|status|generate|activate|export`. و`export --recipient age1...` بيشفّر بـ `filippo.io/age` v1.3.2 (نفس نسخة U5) بـ armor، وبيرفض المفتاح الخاص لو اتبعت بالغلط، ومابيكتبش أي حاجة تتفك على السيرفر، وبيسجّل الوقت بس في `"LastExportedAt"` وفي الـ audit. و`keys init --discard-unreadable` للقاعدة اللي رجعت من غير سلسلتها: من غير `--confirm discard-unreadable-secrets` بيعرض الإصدارات وعدد الـ DEKs ومابيغيّرش حاجة، ومعاه بيعلّم الـ DEKs دي `destroyed` (الإصدار الناقص `retired`، والمختلف بيتسجّل من جديد `pending`) وبيكتب `system.keys.discarded` بخطورة عالية.
8. **`GET /system/keys`** بـ `instance.settings.manage` (صاحب النسخة، ومن غير توكنات): الإصدارات وحالاتها وعدد الـ DEKs وآخر تدوير وآخر تصدير وهل الإصدار محمّل على السيرفر اللي جاوب. عمره ما بيرجّع مفتاح ولا قيمة فحص ولا DEK ملفوف. والتدوير من الـ CLI بس لأنه محتاج الملف.
9. **مفيش cache للـ DEKs:** فك الـ DEK بالملف عملية AES واحدة. الـ cache بحد ووقت بييجي مع KMS.
10. **الجداول** (`00052_keyring.sql`، expand): `"KeyEncryptionKeys"` (`"Version"` unique، وواحد بس `active` بـ partial unique index، و`"CheckValue"` 32 byte) و`"DataKeys"` (`"OwnerId"` NULL لـ `instance` بس، وunique على الصاحب `NULLS NOT DISTINCT WHERE "State" = 'active'`، وFK على إصدار الـ KEK، وindex للـ worker)، و`"ServerInstances"."KekVersions"` (`integer[] NOT NULL DEFAULT '{}'`)، فالـ binary القديم بيسجّل عادي.

## بعد المراجعة الأمنية
11. **الـ DEK الجديد مايتلفّش بإصدار اتقاعد وهو بيتعمل:** `InsertDataKey` بيعمل `INSERT ... SELECT ... FROM "KeyEncryptionKeys" WHERE "Version" = @v AND "State" = 'active' FOR SHARE`، فالتفعيل والتقاعد بيستنوا الإدراج، ولو الإصدار ماعادش active الإدراج مابيحصلش و`Seal` بيعيد بالإصدار الحالي (3 محاولات). و`Status.Removable()` ورسالة «احذف سطره» بيشترطوا صفر DEK على الإصدار.
12. **`keys generate` بقفل:** `flock(LOCK_EX)` على الـ descriptor، و`fstat`، والـ parse، و`pwrite` عند الحجم القديم، و`fsync`، وقراية تانية وparse للتأكد، وأي فشل بيعمل `ftruncate` للحجم القديم و`fsync`. بيشتغل على secret الـ Compose (bind mount لملف واحد). والسطر الأخير البايظ رسالته بتقول «احذف السطر N».
13. **`Register` بيفشل لو مفيش active:** لو التفعيل ماغيّرش أي صف (أحدث إصدار في الملف `retired`، زي استرجاع تصدير قديم) بيرجع `KEYRING_RETIRED` بالـ exit 79.
14. **التدوير مابيحميش من سلسلة اتسرّبت لوحده:** السلسلة المسرّبة مع نسخة قاعدة من قبل التدوير بتفك DEKs لسه بتحمي القيم الحالية، و`Destroy` مابيمنعش نسخة أقدم مع سلسلتها. الحماية الكاملة محتاجة `keys rekey` (DEK جديد لكل صاحب وإعادة ختم كل قيمة)، وده شرط على M5.2 ومكتوب في الخطة، لأن مفيش قيم envelope قبل M5.2.
15. **`--discard-unreadable`:** بيرفض (`KEYS_IN_USE`) لو instance حي بيقول إن معاه أي إصدار من اللي هيتمسحوا، ومابيعملش سلسلة لوحده لو `--file` اتكتب، والتأكيد مربوط بالخطة: `--confirm destroy-<عدد الـ DEKs>-keys`، ولو العدد اتغيّر جوه الـ transaction مفيش تغيير، وحدث الـ audit في نفس الـ transaction (`Audited.Tx`).
16. **الصف البايظ مابيوقفش إعادة اللف:** الـ DEK اللي مابيتفكّش بيتعلّم `"UnwrapFailedAt"` ويتسجّل في الـ log (الـ id والإصدار بس)، والـ worker بيكمّل، والإصدار مابيتقاعدش طول ما عليه صف من دول. بيبانوا في `keys status` و`doctor`، والإقلاع بيمسح العلامة عشان يجرّبهم تاني بعد إصلاح السلسلة.
17. **Compose و79:** `DHOW_ON_INCOMPATIBLE=wait` (الافتراضي في الصورة) بقى بيشمل أخطاء `KEYRING_*`: سطر log واحد وانتظار إشارة الإيقاف، والـ healthcheck فاشل. والسلسلة بتتقارن بالقاعدة قبل `PrepareSchema` لو الجدول موجود، فالسلسلة الغلط مابتطبّقش migration.
18. **`doctor`** بيستخدم `Lstat` وبيرفض الـ symlink زي السيرفر، والـ parser بيشتغل على `[]byte` من غير تحويل لـ string وبيصفّر الـ buffers. و`filippo.io/age` v1.3.2 و`filippo.io/hpke` v0.4.0 نضاف في `govulncheck`.

## البدائل اللي اترفضت
- **KEK مشتق من `DHOW_SECRET_KEY`:** مفيش تدوير حقيقي من غير ما كل التوكنات والـ runners تقع، والمفتاح في متغيّر بيئة ممكن يبان.
- **DEK لكل سر:** صف `DataKeys` لكل قيمة من غير مكسب، لأن الاتنين بيتفكّوا في نفس الـ process بنفس الـ KEK.
- **DEK لكل مؤسسة:** مسح بيئة مايقدرش يمسح مفتاحها.
- **إعادة تشفير كل القيم مع التدوير:** ساعات بدل ثواني، وكل جدول فيه أسرار لازم يعرف التدوير. وبعد التسريب الحل الحقيقي تغيير القيمة.
- **التفعيل أوتوماتيك أول ما الإصدار يظهر:** instance لسه ماخدش الملف الجديد مايقدرش يفك اللي اتلف بيه.
- **DEK يشفّر القيم مباشرة بـ nonce عشوائي:** حد 2^32 قيمة لكل DEK قبل ما خطر تكرار الـ nonce يبقى مش مقبول، والـ DEK بيعيش طول عمر الصاحب.
- **`ReplaceKeyringFile` بملف مؤقت و`rename`:** محتاج كتابة في المجلد، وده مش متاح في secret الـ Compose، والـ rename بيكسر الـ bind mount.
- **الملف بصلاحية المجموعة زي `secretValue`:** السلسلة أهم من أي سر تاني، فالقاعدة أشد، والاستثناء الوحيد ACL الـ systemd.

## النتايج
- نسخة القاعدة لوحدها، وملف السلسلة لوحده، مابيفتحوش أي سر. والملف الغلط عمره ما بيطلّع بيانات بايظة بصمت.
- كل تنصيب لازم يحفظ ويصدّر السلسلة. الـ installer و`init.sh` بيعملوها وبيقولوا «صدّرها»، و`doctor` بيحذّر لو مفيش تصدير أو لو الإصدار النشط عدّى عليه سنة، والنسخة الاحتياطية (U5) بتاخد `./secrets/` كمان.
- M5.2 بينقل مستخدمي `Sealer` لـ `Envelope` بنفس الـ labels والـ contexts، وM5.4 بيستخدم `environment:<id>` و`Destroy`.
