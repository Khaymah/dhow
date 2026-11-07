# خطة M5: الأسرار والبيئات (Secrets & Environments)

**الحالة:** مقبولة. Sabry اختار التوصية في الأسئلة الستة يوم 2026-11-03 ([قراراته](#قرارات-sabry-2026-11-03)). القرارات التقنية مفوَّضة ومكتوبة بالتوصية، وكل واحد منها بيبقى ADR في أول المرحلة بتاعته · **التاريخ:** 2026-11-03

## الهدف
شرط خلاص M5 في `docs/roadmap.md` (الصف M5، بعد [ADR-0068](ADR/0068-first-production-release-scope.md)):
- **الأسرار عمرها ما بتظهر** في الـ API ولا السجلات ولا الإشعارات، ومختبَر: فحص التسريب بتاع M4.11 بيتوسّع لكل سر بكل نطاقاته، ولمفاتيح التغليف نفسها.
- **مسار الموافقة شغّال من الأول للآخر:** push على فرع مش مسموح بيتخطّى بسبب واضح، وpush على `main` بيستنى مراجع، والمراجع بيوافق بخطوة تانية، والـ job بينشر بأسرار البيئة، والنشر بيتسجّل.
- **التشفير المغلّف** (envelope encryption) لكل حاجة متشفّرة في القاعدة النهارده بـ `Core/Crypto.Sealer`، و**تدوير مفتاح التغليف (KEK) من غير توقف**.
- **قاعدة البيئة على السيرفر** بتحدد الفروع والـ tags اللي تنشر عليها (قرار Sabry يوم 2026-11-02)، ومفيش commit يقدر يغيّرها.

المراجع:
- [ADR-0014](ADR/0014-secrets-management.md) (envelope وKEK بإصدارات، والـ write-only، وVault/KMS في Enterprise، وworkload OIDC ببلاش).
- [ADR-0045](ADR/0045-secrets-transport-and-masking.md) (`SecretRef` و`SecretResolver` وHPKE للـ runner والـ masker): ده العقد بين M3 وM5.
- [ADR-0072](ADR/0072-webhooks-and-git-credentials.md) و[ADR-0074](ADR/0074-git-connection-oauth-for-setup-deploy-keys-for-access.md) (credentials المزوّدين بـ `Sealer` لحد M5، القرار 12 في M4).
- [ADR-0075](ADR/0075-multi-container-attempts-checkout-and-plugins.md) (أسرار الإضافات `DHOW_INPUT_*` محصورة في خطوتها) و[ADR-0076](ADR/0076-pipeline-runs-definitions-and-forks.md) (الـ forks من غير أسرار ولا بيئات أبدًا) و[ADR-0078](ADR/0078-artifacts-cache-and-blob-store.md) (artifacts الـ forks `Untrusted`).
- [ADR-0077](ADR/0077-upgrades-migrations-and-update-policy.md) و[`Upgrades-and-Help.md`](Upgrades-and-Help.md) (قاعدة expand/contract والـ hook U1، والنسخ الاحتياطي U5).
- `docs/architecture.md` (القسمين 6 و7)، و`docs/pipeline-spec.md` (§3 و§8 و§9 و§12.4 و§12.5)، و`docs/security-model.md` (§8 و§12)، و`docs/editions-and-licensing.md` (§3).

**قبلها:**
- **U2** (توافق الـ schema في الـ binary): M5.1 محتاج `"ServerInstances"` والـ heartbeat بتوعه عشان تفعيل KEK جديد، و`"SchemaFloor"` عشان الـ contract.
- **U4** (الـ backfills في الخلفية): أول حد محتاجه هو نقل مستخدمي `Sealer` في M5.2، فبيتعمل معاه لو لسه ما اتعملش.
- M4.5 وM4.6 مش شرط (التفاصيل في [الترتيب](#الترتيب-مع-الشغل-الباقي)).

## قرارات Sabry اللي الخطة مبنية عليها (ملزمة)
1. **قاعدة البيئة بتحدد الفروع والـ tags اللي بتنشر عليها**، على السيرفر، ومفيش commit يقدر يغيّرها. الـ job بيكتب `environment:` واحد. الـ job اللي الـ ref بتاعه مش مسموح بيخلص `skipped` بسبب واضح مش `failed`. المستودع بيفضل بـ deploy key واحد للـ checkout لكل الفروع، ومفاتيح السيرفرات اللي بننشر عليها أسرار جوه كل بيئة (Status، 2026-11-02).
2. **تنفيذات الـ forks:** من غير أسرار ولا بيئات، أبدًا (ADR-0076).
3. **الـ cache:** أول ما الأسرار تيجي، تنفيذات الـ forks ماترجّعش نطاقات موثوقة، أو ترجّع قايمة صريحة بس (Status، من مراجعة M4.11). M5 بينفّذ [قرار Sabry 4](#قرارات-sabry-2026-11-03).
4. **كله ببلاش** (ADR-0077): مفيش خدمة ولا جهاز مدفوع مطلوب. KMS أو HSM ممكن يبقى اختياري، عمره ما يبقى شرط.
5. **expand/contract** (ADR-0077، الـ hook U1): كل migration في M5 ماشي بيها، وكمان تغيير صيغة القيم المتشفّرة نفسها.

## النطاق
1. **سلسلة المفاتيح والتشفير المغلّف:** KEK بإصدارات بره القاعدة، وDEK لكل صاحب (مؤسسة، مشروع، بيئة، مستخدم، النسخة) جوه القاعدة ملفوف بالـ KEK، والتدوير من غير توقف، وأوامر `dhow-server keys`.
2. **نقل كل مستخدمي `Sealer`** للتشفير المغلّف: TOTP، وأسرار إعدادات السيرفر، وأسرار مزوّدي الدخول، والـ deploy keys، وأسرار الـ webhooks، وتوكنات الاتصال، ومفتاح الـ GitHub App لو M4.5 خلص.
3. **الأسرار:** على مستوى المؤسسة والمشروع والبيئة، بأولوية واضحة، وwrite-only، بإصدارات وتدوير وaudit، وتوصيلها للـ runner وقت `Start` بس، وملفات أسرار.
4. **المتغيّرات** (`vars.*`، مش سرية) بنفس النطاقات، لأن الـ spec بيقبلها والسيرفر بيدّيها فاضية النهارده، والبيئات محتاجاها (عنوان السيرفر مثلًا).
5. **بيانات دخول الـ registry الخاص:** للسيرفر وهو بيثبّت الـ tag بالـ digest (§12.5 بيقول M5)، وللـ runner وهو بيسحب الصورة.
6. **البيئات:** النموذج، وقواعد الحماية (§9)، والموافقات بخطوة تانية، ومؤقت الانتظار، والتزامن لكل بيئة، وسجل النشر، والتخطّي الإداري المسجّل.
7. **الـ forks والـ PRs والـ cache:** القاعدة الجديدة للـ cache، وتعليم الـ cache اللي اتكتب من job شاف أسرار.
8. **مُصدِر OIDC للأحمال (تصميم بس):** التصميم كامل هنا في M5.8، والتنفيذ بعد أول نسخة ([قرار Sabry 3](#قرارات-sabry-2026-11-03))، فمش جزء من شرط خلاص M5.
9. **الواجهة:** صفحة الأسرار (مابتعرضش قيمة أبدًا)، والبيئات، والموافقات، وصفحة التنفيذ وهي مستنية موافقة أو متخطّية.
10. **الوثائق بالنسختين**، وصفوف M5 في جدول التهديدات، و`deploy/m5-demo/run.sh`.

## مش في M5
- **Vault وKMS السحابة وHSM:** في Enterprise (ADR-0014، M8 بعد أول نسخة). M5 بيعمل الـ interface `KeyProvider` ومعاه provider الملف بس.
- **أكتر من مراجع مطلوب، ونوافذ النشر، والتجميد المجدول:** في Business مع M8 بعد أول نسخة، زي جدول النسخ ([قرار Sabry 2](#قرارات-sabry-2026-11-03)). Community فيها مراجع واحد وزرار «أوقف النشر» يدوي، والنموذج بيتعمل جاهز للباقي من غير ما يتنفّذ.
- **مُصدِر OIDC للأحمال:** بعد أول نسخة ([قرار Sabry 3](#قرارات-sabry-2026-11-03)). M5.8 مصمّمة هنا ومتأجّلة، ومش من شرط خلاص M5.
- **تدوير `DHOW_SECRET_KEY` نفسه:** بعد M5.2 مابقاش بيشفّر حاجة متخزّنة، بس لسه بيشتق مفاتيح التوقيع والـ HMAC (الـ runner plane وتوكنات API وCSRF وأكواد الطوارئ). تدويره بيعيد تسجيل الـ runners ويبطّل التوكنات، فبيتأجّل لبعد أول نسخة بإجراء موثّق.
- **تدوير DEK بإعادة تشفير القيم:** تدوير الـ KEK بيعيد لف الـ DEKs بس (ADR-0014). بعد أي تسريب، الحل الحقيقي إن صاحب السر يغيّر قيمته، وده مكتوب في دليل الطوارئ.
- **الإشعارات** (إيميل للمراجعين، وSlack، والموافقة من Slack): M7. في M5 الموافقة بتظهر في الواجهة وفي عدّاد «مستنياك».
- **أهداف النشر نفسها** (`deploy-ssh` و`deploy-compose` والرجوع): M6. M5 بيدّي البيئة وأسرارها وسجل النشر اللي M6 هيبني عليه.
- **كتابة الأسرار أو الموافقة بتوكن API:** مش في M5 ([قرار Sabry 6](#قرارات-sabry-2026-11-03)). الكتابة من جلسة بس، والتوكن بياخد `secret.read` (الأسماء والـ metadata، عمره ما بيقرا قيمة).
- **سياسات OPA:** Enterprise.

## المعمارية
```
 browser ─► proxy ─► /api/v1 ─┬─ Modules/Secrets        secrets, variables, registry credentials (write-only)
                              ├─ Modules/Environments   environments, rules, approvals, deployments, env lock
                              ├─ Modules/Pipelines      gate per RunJob ──► Core/Deployments (port, same tx)
                              └─ Modules/Settings ...   (TOTP, settings, OIDC providers, Sources: all via Core/Keys)
                                           │
                                           ▼
 /etc/dhow/credentials/dhow-keyring ─► Core/Keys ── KeyProvider(file) ── KEK v1, v2 … (never in the DB)
                                           │   wraps/unwraps
                                           ▼
 PostgreSQL 18:  "KeyEncryptionKeys" (version, check value, state)   "DataKeys" (owner, kek version, wrapped DEK)
                 value = 0x02 ‖ DataKeyId ‖ nonce ‖ AES-256-GCM(DEK, value, AAD = record)

 dhow-runner ─► /runner-api  Start(lease, fencing) ──► Jobs ─► SecretResolver (App)
                                 │                               ├─ checkout credential (M4)
                                 │                               └─ Core/SecretStore (Secrets): env > project > org,
                                 │                                  re-check fork, environment, rule version
                                 ◄── SealedSecrets: HPKE to the leasing runner (ADR-0045), once per attempt
   runner: masker ◄─ values · env/file per step · registry authfile outside the sandbox · scan artifacts and cache
   (M5.8, after the first release) /run/dhow/bin/id-token ─► agent ─► runner ─► IdentityService.MintIdToken ─► JWT (RS256)
 public:  /.well-known/openid-configuration   /.well-known/jwks.json
```

## سلسلة المفاتيح والتشفير المغلّف

### الطبقات
- **KEK (مفتاح التغليف):** 32 byte عشوائي، ليه رقم إصدار، **عمره ما بيدخل القاعدة**. بييجي من `KeyProvider`، وفي Community الـ provider هو ملف «سلسلة المفاتيح» (keyring).
- **DEK (مفتاح البيانات):** 32 byte عشوائي لكل صاحب: `instance` (إعدادات السيرفر ومزوّدي الدخول والـ GitHub App ومفاتيح OIDC)، و`organization:<id>` (أسرار المؤسسة واتصالات Git)، و`project:<id>` (أسرار المشروع ومفاتيح المستودع وسر الـ webhook)، و`environment:<id>` (أسرار البيئة)، و`user:<id>` (TOTP). بيتخزّن في `"DataKeys"` ملفوف بـ KEK بـ AES-256-GCM، والـ AAD = `dhow/dek/v1 ‖ DataKeyId ‖ OwnerKind ‖ OwnerId ‖ KekVersion`.
- **القيمة:** متشفّرة بالـ DEK بتاع صاحبها بـ AES-256-GCM وnonce عشوائي 12 byte.
- **ليه DEK لكل صاحب مش لكل سر ومش لكل مؤسسة:** لكل سر معناه DEK لكل صف وصف `DataKeys` لكل قيمة من غير مكسب حقيقي (الاتنين بيتفكّوا في نفس الـ process بنفس الـ KEK). ولكل مؤسسة معناه إن مسح بيئة مايقدرش يمسح مفتاحها. لكل صاحب بيدّي مسح بالتشفير (crypto-shredding): لما البيئة أو المشروع يتمسح، الـ DEK بتاعه بيتصفّر ومفيش قيمة تتفك تاني. وده نفس كلام `docs/architecture.md` §6.

### الصيغة
- **الإصدار 2:** `0x02 ‖ DataKeyId (16 byte) ‖ nonce (12) ‖ ciphertext ‖ tag (16)`، يعني 45 byte زيادة على القيمة. **اتعدّل في M5.1 بعد المراجعة الأمنية ([ADR-0081](ADR/0081-keyring-and-envelope-encryption.md)):** الصيغة بقت `0x03 ‖ DataKeyId ‖ salt (32) ‖ nonce ‖ ciphertext ‖ tag` بمفتاح فرعي HKDF-SHA256(DEK، salt، `dhow/value/v2` ‖ label) لكل قيمة، يعني 77 byte زيادة، والـ context الفاضي مرفوض.
- **الـ AAD:** `0x02 ‖ DataKeyId ‖ label الغرض ‖ context الصف`. الـ label والـ context نفس اللي بيتستخدموا النهارده (`dhow/git-deploy-key/v1` و`deploy-key:<RepositoryId>` مثلًا)، فقيمة اتنقلت لصف تاني أو لعمود تاني مابتتفكّش.
- **فحص الصاحب:** `Open` بياخد الصاحب المتوقع، ولو صف `DataKeys` بتاع الـ `DataKeyId` صاحبه مختلف بيرجّع `ErrSealed`. فنسخ قيمة من مؤسسة لمؤسسة مابيعدّيش حتى لو الـ context اتزوّر.
- **الإصدار 1** (`Sealer` النهارده): `0x01 ‖ nonce ‖ ciphertext ‖ tag` بمفتاح مشتق من `DHOW_SECRET_KEY`. بيتقري لحد الـ contract في M5.9.
- **القيود الموجودة كفاية:** TOTP 20 byte بيبقى 97 مع صيغة `0x03` (القيد 29 لـ 128)، وإعدادات السيرفر (لحد 65536) ومزوّدي الدخول (لحد 4096) والمستودعات (8192 و1024) فيهم مكان. اختبار في M5.2 بيتأكد من أكبر قيمة ممكنة في كل عمود.

### مكان الـ KEK ([قرار Sabry 1](#قرارات-sabry-2026-11-03))
- **القرار:** ملف سلسلة مفاتيح منفصل عن `DHOW_SECRET_KEY`، وKMS خارجي اختياري بعدين ورا نفس الـ interface:
  - **systemd:** `LoadCredential=dhow-keyring:/etc/dhow/credentials/dhow-keyring` في `dhow.service`، و`root:root 0600`.
  - **Compose:** `secrets:` في `compose.yaml` من `./secrets/dhow-keyring`، و`./init.sh` بيعمله جنب `.env`. مش متغيّر بيئة، لأن `docker inspect` وبيئة الـ process بيكشفوه.
  - **غير كده:** `DHOW_KEYRING_FILE`، بنفس قواعد `secretValue` الموجودة (مفيش صلاحية لمستخدمين تانيين، و64 KiB بالكتير، وملف عادي).
- **الصيغة:** سطر لكل إصدار: `kek <رقم> <base64url لـ 32 byte>`، 16 إصدار بالكتير. أي سطر غلط، أو رقم متكرر، أو مفتاح أقصر، أو مفتاح شكله مش عشوائي (نفس `looksRandom`)، الإقلاع بيقف.
- **`KeyProvider`** (`Core/Keys`): `Versions()` و`Wrap(ctx, version, dek, aad)` و`Unwrap(ctx, version, wrapped, aad)` و`Name()`. التنفيذ في M5 `file` بس، ومعاه `fake` للاختبارات. Vault Transit وKMS السحابة وPKCS#11 بيتركّبوا على نفس الـ interface في `ee/` (M8)، ومن غير ما يبقوا شرط لأي حاجة.
- **مفيش cache للـ DEKs في M5:** فك الـ DEK بالملف عملية AES واحدة. الـ cache (بحد ووقت) بييجي مع KMS، لأن كل فك هناك نداء شبكة.

### الإقلاع (fail closed)
- `"KeyEncryptionKeys"` فيه لكل إصدار `"CheckValue"` = HMAC-SHA256(KEK، `dhow/kek-check/v1`)، مش المفتاح.
- **عند الإقلاع:** أي إصدار في القاعدة حالته مش `retired` لازم يبقى في الملف وبنفس الـ check value، وإلا السيرفر بيقف برسالة فيها رقم الإصدار والسبب (`KEYRING_MISSING_VERSION` أو `KEYRING_MISMATCH`). فملف غلط عمره ما بيطلّع بيانات بايظة بصمت.
- **إصدار في الملف مش في القاعدة:** بيتسجّل `pending` بالـ check value بتاعه. ولو القاعدة فاضية خالص (تنصيب جديد)، أعلى إصدار بيبقى `active` في نفس الـ transaction.
- **`/health/ready`:** بيرجّع `keyring` في الفحوص (`ok` أو السبب)، و`doctor` بيقول نفس الكلام.
- **كل instance** بيكتب الإصدارات اللي معاه (أرقام بس) في `"ServerInstances"."KekVersions"` مع الـ heartbeat بتاع U2.

### التدوير من غير توقف
1. **`dhow-server keys generate`** (root): بيضيف سطر `kek <n+1> ...` لملف السلسلة بصلاحياته، وبيطبع الخطوة الجاية. في Compose نفس الأمر من الـ host على `./secrets/dhow-keyring`. لو فيه أكتر من instance، نفس الملف لازم يتنسخ للكل.
2. **إعادة تشغيل** كل instance (rolling). الإصدار الجديد بيتسجّل `pending` ومحدش بيلف بيه لسه.
3. **`dhow-server keys activate <n>`:** بيتأكد إن كل instance حي (heartbeat في آخر دقيقتين) معاه الإصدار ده، وإلا بيرفض بـ `KEK_NOT_EVERYWHERE` ومعاه أسماء الـ instances. بعدها `n` بيبقى `active` والقديم `retiring`، والـ DEKs الجديدة بتتلف بـ `n` على طول.
4. **إعادة اللف في الخلفية:** worker تحت advisory lock بياخد 500 صف من `"DataKeys"` على إصدار مش الـ active بـ `FOR UPDATE SKIP LOCKED`، يفك بالقديم ويلف بالجديد ويحدّث الصف، ويكمّل بعد أي إعادة تشغيل. القيم نفسها مابتتلمسش، فالـ DEKs بالآلاف بتخلص في ثواني. وطول الوقت ده القراية شغّالة لأن كل صف بيقول إصداره.
5. **لما مفيش صف على الإصدار القديم:** بيبقى `retired` لوحده، و`keys status` بيقول إنه ينفع يتشال من الملف بعد ما النسخ الاحتياطية القديمة تتمسح (كل نسخة شايلة سلسلتها، تحت).
- **`keys status`:** الإصدارات وحالاتها وعدد الـ DEKs على كل واحد وتقدّم إعادة اللف وآخر تدوير وآخر تصدير. ونفس الحالة للقراية بس في «إعدادات السيرفر» (من غير أي مفتاح طبعًا).
- **التذكير:** `doctor` بيحذّر لو الـ KEK الـ active عدّى عليه سنة، أو لو مفيش تصدير متسجّل للسلسلة.

### نقل مستخدمي `Sealer` (M5.2)
| العمود | الصاحب | الـ label والـ context (زي ما هم) |
|---|---|---|
| `"TotpFactors"."SecretSealed"` | `user` | `dhow/totp-secret/v1`، الـ user id |
| `"InstanceSettings"."Secrets"` | `instance` | `dhow/instance-settings/v1`، المجموعة |
| `"OidcProviders"."Secret"` | `instance` | `dhow/instance-settings/v1`، `oidc:<id>` |
| `"Repositories"."DeployKeySealed"` و`"PreviousDeployKeySealed"` | `project` | `dhow/git-deploy-key/v1`، `deploy-key:<id>` |
| `"Repositories"."WebhookSecretSealed"` | `project` | `dhow/git-webhook-secret/v1`، `webhook-secret:<id>` |
| `"GitConnections"."TokenSealed"` | `organization` | label التوكن اللي M4.6 هيحطه |
| `"GitHubApps".*Sealed` (لو M4.5 خلص) | `instance` | labels الـ App |

- **الخطوات بقاعدة expand/contract على صيغة القيمة:**
  - **N (M5.2):** القراية بتقبل `0x01` و`0x02`، والكتابة لسه `0x01`. فالـ binary اللي قبل M5 بيقرا كل حاجة.
  - **N+1 (M5.2، في PR تاني أو نفس الـ PR قبل أول tag):** الكتابة `0x02`، وbackfill في الخلفية (U4) بيحوّل `0x01` لـ `0x02` صف صف بنفس الـ context، ونسخة N لسه بتقرا الاتنين.
  - **N+2 (M5.9 أو أول minor بعدها):** contract بيضيف `CHECK (get_byte(col, 0) = 2)` بـ `NOT VALID` و`VALIDATE`، وبيشيل كود القراية القديم، بـ guard `BACKFILL_NOT_DONE` و`-- dhow:floor` = N+1.
  - **قبل أول tag:** مفيش حد يرجع له (القاعدة 8 في `Upgrades-and-Help.md`)، فـ N وN+1 ممكن يبقوا PR واحد، والـ contract في M5.9 بـ `-- dhow:floor 0.0.0`. بعد أول tag لازم release كامل بين كل خطوة.
- **بعدها `DHOW_SECRET_KEY`** بيشتق مفاتيح التوقيع والـ HMAC بس (الـ runner plane، والتسجيل، والتحدّي، وتوكنات API، وأكواد الطوارئ، وCSRF، وflow الـ OIDC، ومنح الـ artifacts).
- **hook جديد:** أي `appcrypto.NewSealer` بره `Core/Crypto` وبره قايمة القراية القديمة مرفوض، ومعاه اختبار في `.claude/hooks/tests/`. كده M4.5 وM4.6 لو اتعملوا بعد M5.2 لازم يستخدموا `Core/Keys`.

### النسخ الاحتياطي والاسترجاع (مع U5)
- **النسخة لازم فيها السلسلة:** U5 بياخد `/etc/dhow` كله (فيه `credentials/dhow-keyring`)، وفي Compose `.env` لوحده مش كفاية: U5 بيتعدّل في M5.1 عشان ياخد `./secrets/` كمان. النسخة متشفّرة بمفتاح النسخ المنفصل (القرار 10 في U5)، فتسريب النسخة لوحدها مايكفيش.
- **الاسترجاع** بيقارن الـ check values في القاعدة المسترجعة بالسلسلة اللي في النسخة قبل ما يبدأ السيرفر، وبيقف لو مش متطابقين.
- **`pg_dump` من غير السلسلة** مالوش لازمة: كل الأسرار بتضيع. ده مكتوب بخط واضح في `docs/install/keys.md`، و`doctor` بيقوله.
- **`dhow-server keys export --recipient <age>`:** بيطلّع السلسلة متشفّرة بمفتاح `age` عام (نفس مكتبة U5)، عشان المشغّل يحفظها بره الجهاز. والسيرفر مايقدرش يفكّها. آخر تصدير بيتسجّل (الوقت بس).
- **سجل `pg_dump` العادي في U3** (لحد U5) بيفضل شغّال لأن السلسلة على القرص.

## الأسرار والمتغيّرات

### النطاقات والأولوية
- **المؤسسة:** لكل المشاريع، أو لمشاريع مختارة بس (`"Visibility"` = `all_projects` أو `selected_projects`).
- **المشروع:** لكل jobs المشروع اللي مش من fork.
- **البيئة:** للـ jobs اللي `environment:` بتاعها البيئة دي، وبعد ما البوابة تفتح بس.
- **الأولوية:** البيئة، وبعدها المشروع، وبعدها المؤسسة. نفس الاسم في أكتر من نطاق مسموح، والواجهة بتبيّن «بيغطي على سر المؤسسة».
- **الـ PRs من نفس المستودع:** أسرار المؤسسة والمشروع مابتوصلهاش إلا لو السر عليه `"AvailableToPullRequests"` (افتراضيًا لأ)، وأسرار البيئة حسب قاعدة البيئة (تحت).
- **الـ forks:** ولا حاجة: لا أسرار، ولا متغيّرات، ولا بيانات دخول registry، ولا توكن OIDC.

### الـ API: للكتابة بس
- **`PUT .../secrets/{secretName}`** بيعمل السر أو إصدار جديد منه. الـ body: `Value` و`Description` و`AvailableToPullRequests`، و`Visibility` و`ProjectIds` للمؤسسة بس. الرد هو الـ metadata بس.
- **الـ metadata:** `Name` و`Scope` و`Description` و`Version` و`UpdatedAt` و`UpdatedBy` و`LastUsedAt` و`SizeBytes` و`ShadowedBy`. مفيش `Value` ولا hash ولا بصمة للقيمة في أي DTO، لأن البصمة بتكشف إن قيمتين متساويين.
- **الإصدارات:** آخر 5 إصدارات بتفضل 30 يوم، وبعدها القيمة بتتمسح (`"ValueSealed" = NULL`، مش `DELETE`). `POST .../secrets/{secretName}/restore-version` بيرجّع إصدار قديم كإصدار جديد. ومفيش أي طريقة تقرا قيمة.
- **المسح:** `POST .../secrets/{secretName}/remove` بيعلّم السر ممسوح وبيمسح قيم كل إصداراته في نفس الـ transaction.
- **الـ audit** (بعد الـ commit، والبيانات أسماء ونطاقات وأرقام إصدارات بس): `secret.created` و`secret.updated` و`secret.version_restored` و`secret.removed` و`variable.*` و`registry_credential.*`. ومع كل `Start`: `job.secrets_delivered` بأسماء الأسرار ونطاقاتها وإصداراتها.
- **الـ access log:** الـ routes دي مابتسجّلش الـ body أصلًا (زي النهارده)، واختبار بيبعت قيمة مميزة ويدوّر عليها في كل log.

### الأسماء والحدود
- **الاسم:** `^[A-Za-z_][A-Za-z0-9_]{0,63}$` (نفس `jobspec`)، و`DHOW_` في الأول محجوز، وفريد في النطاق من غير فرق بين الحروف الكبيرة والصغيرة (index على `lower("Name")`).
- **القيمة:** UTF-8 من غير NUL، من 8 byte (أقل من كده مايتقنّعش كويس، ADR-0045 قال M5 هيمنعه) لـ 32 KiB. المفاتيح الثنائية بتتكتب base64 وتتفك في الخطوة.
- **العدد:** 500 سر للمؤسسة، و200 للمشروع، و100 للبيئة، و256 متغيّر لكل نطاق (حد `vars.*` في الـ spec).
- **لكل job:** 50 سر (`jobspec.MaxSecrets`)، و256 KiB مجموع القيم بعد الفك. أكتر من كده: `SECRETS_TOO_LARGE` قبل الإدراج، لأن رسالة `Start` حدها 1 MiB.

### المتغيّرات (`vars.*`)
- `"Variables"` بنفس النطاقات والأولوية والأسماء، والقيمة نص 4 KiB بالكتير. بتتقري بالـ API لأي حد عنده `project.read`، فمكانها مش للأسرار، والواجهة بتقول كده.
- بتتملي في سياق CEL وقت الإدراج (بعد ما البيئة تتعرف)، فـ `if` و`env` يقدروا يستخدموها.

### بيانات دخول الـ registry الخاص
- `"RegistryCredentials"` على المؤسسة أو المشروع: `Host` (بقواعد `jobspec.ParseEgress`، ومعاه بورت لو موجود) و`Username` و`PasswordSealed`. الأولوية للمشروع.
- **السيرفر** بيستخدمها وهو بيثبّت الـ tag بالـ digest (§12.5) للـ host ده بس، وللـ token realm اللي بيعدّي على قواعد §12.5 بس، ومن خلال `netguard`.
- **الـ runner** بياخدها في `SealedSecrets` باسم محجوز `dhow.registry.<n>`، ويكتب authfile مؤقت في مجلد حالته الخاص (`0600`، بره أي sandbox)، ويستخدمه في `podman pull --authfile`، ويمسحه بعد السحب. الـ job نفسه عمره ما بيشوفها، والقيمة في الـ masker.

### في `.dhow.yml`
- **`${{ secrets.NAME }}`** زي ما هو: القيمة كلها في `env` أو `with` بس، ومفيش في `if` ولا `concurrency` ولا `cache` ولا جوه تعبير أكبر.
- **جديد: `files:`** على الخطوة أو على الـ job:
  ```yaml
  steps:
    - run: ssh -i "$DHOW_SECRETS_DIR/deploy_key" deploy@app.example.com ./release.sh
      files:
        deploy_key: ${{ secrets.DEPLOY_KEY }}
  ```
  كل ملف بيبقى `SecretRef.as_file` بـ `step_indexes` الخطوة دي (أو كل خطوات `run` لو على الـ job، ومش الإضافات)، في `/run/dhow/secrets/<name>` بصلاحية `0400`، و`DHOW_SECRETS_DIR` بيشاور عليه. الاسم بنفس قاعدة `jobspec` لملفات الأسرار، و20 ملف بالكتير. الـ runner بيدعم `as_file` من M3، فمفيش feature جديدة.
- **`SECRETS_UNAVAILABLE` و`ENVIRONMENTS_UNAVAILABLE`** مابقوش warnings في الـ validator. الأكواد بتفضل محجوزة.
- **`dhow lint`** offline، فمايقدرش يعرف السر موجود ولا لأ. السيرفر بيقول وقت الإدراج.

### التوصيل للـ runner
1. **وقت الإدراج** (`Pipelines`، نفس الـ transaction): `Core/SecretStore.Check(scope, names, trigger)` بيتأكد إن كل اسم موجود ومسموح للتنفيذ ده. الناقص: الـ `RunJob` بيفشل `SECRET_NOT_FOUND` والأسماء في `"FailureDetail"` (أسماء بس). والـ PR اللي سره مش مسموح: `SECRET_NOT_AVAILABLE_TO_PULL_REQUESTS`.
2. **وقت `Start` بس:** `SecretResolver` في `App` بيوصل من `Jobs.OriginId` للـ `RunJob` والـ run، ومش بيصدّق الـ spec:
   - الـ run مش fork، والـ `RunJob` لسه `active` والـ job ده هو الـ `CurrentJobId`.
   - لو فيه بيئة: البيئة موجودة ومش متوقفة، والـ ref لسه مسموح بالقاعدة الحالية، والموافقة (أو التخطّي) لسه صالحة لنفس `"RuleVersion"`. وإلا المحاولة بتفشل `ENVIRONMENT_RULE_CHANGED` من غير ما أي قيمة تتفك.
   - بيحل كل اسم بالأولوية، ويفك، ويتأكد من الحد، ويختم للـ runner بـ HPKE (ADR-0045)، ويمسح الـ buffers، ويحدّث `"LastUsedAt"`، ويكتب `job.secrets_delivered`.
3. **في الـ runner:** زي ADR-0045 بالظبط (الذاكرة بس، والـ env للخطوة، والملفات tmpfs، والـ masker قبل أي حاجة تخرج).
4. **إعادة `Start`** لنفس الـ attempt مرفوضة زي M3، والإعادة attempt جديد بيعدّي على نفس الفحص.

### الإخفاء وحدوده
- الـ masker بتاع M3 كما هو: القيمة، وbase64 بأنواعه وإزاحاته، وURL، وJSON، وكل سطر لوحده، وعلى الـ outputs بالتجميعات (ADR-0075).
- **جديد: فحص الـ artifacts والـ cache:** الـ runner بيعدّي الـ tar قبل الضغط على نفس Aho-Corasick بقيم الـ job. artifact فيه قيمة سر: الـ job بيفشل `ARTIFACT_CONTAINS_SECRET` باسم الـ artifact والملف (من غير القيمة). cache فيه قيمة: مابيتحفظش، وتحذير في السجل. الفحص على الملفات زي ما هي، فالملف المضغوط أو المتشفّر جوه الـ artifact مش متغطي، ومكتوب.
- **الحد المعروف (مقبول، ومكتوب في §13):** اللي يقدر يغيّر الكود اللي بيشتغل بسر يقدر يطلّعه بتحويل الـ masker مايعرفوش (يقلب الحروف مثلًا). الحماية الحقيقية: أسرار البيئة ماتوصلش غير بعد قاعدة الـ ref والمراجعة.

## البيئات

### النموذج
- **البيئة تابعة لمشروع** واسمها بنفس قاعدة الـ spec (`^[a-z0-9][a-z0-9_-]{0,63}$`). بيعملها Admin من الواجهة أو الـ API بس. الـ job اللي بيشاور على بيئة مش موجودة بيفشل `ENVIRONMENT_NOT_FOUND` (غلطة إعداد لازم تبان، ومفيش بيئة بتتعمل من غير حماية لأن حد كتب اسمها في YAML).
- **قواعد الحماية** على البيئة نفسها، وأي تغيير فيها بيزوّد `"RuleVersion"` وبيتسجّل في الـ audit بالقيم القديمة والجديدة.
- **سجل النشر:** كل job بيعدّي البوابة بيعمل صف في `"Deployments"` (البيئة والـ run والـ job والـ sha والـ ref والموافقة والحالة والأوقات). «المنشور دلوقتي» هو آخر واحد `succeeded`. M6 بيوسّعه بالـ release والرجوع.

### قواعد الحماية (§9)
| القاعدة | الحقل | الافتراضي | ملاحظة |
|---|---|---|---|
| الفروع والـ tags المسموحة | `AllowedRefs` (`Kind` = `branch` أو `tag`، و`Pattern`) | فرع المشروع الافتراضي | glob بتاع `internal/pathglob` (`*` و`**` و`?`)، و50 pattern بالكتير. فاضية = مفيش حاجة تنشر. **مفيش تخطّي ليها** |
| الـ PRs من نفس المستودع | `AllowPullRequests` | لأ | لبيئات المعاينة. الـ forks لأ أبدًا |
| المراجعين | `Reviewers: {UserIds, TeamIds}` | مفيش | 20 بالكتير. المراجع لازم عضو ودوره Developer أو أعلى على المشروع |
| عدد الموافقات | `RequiredApprovals` | 1 | أكتر من 1 في Business مع M8 ([قرار Sabry 2](#قرارات-sabry-2026-11-03)). الـ CHECK = 1 دلوقتي، والتوسيع expand بعدين |
| منع الموافقة على النفس | `PreventSelfReview` | آه | [قرار Sabry 5](#قرارات-sabry-2026-11-03): Admin يقدر يقفله للبيئة دي بس (فريق شخص واحد)، بخطوة تانية، والتغيير في الـ audit |
| مؤقت الانتظار | `WaitMinutes` | 0 | من 0 لـ 43200 (30 يوم)، بيبدأ لما الـ job يوصل البوابة |
| مدة صلاحية الطلب | `ApprovalTimeoutHours` | 72 | من 1 لـ 720 |
| الصور بالـ digest | `RequirePinnedImages` | لأ | الـ digest لازم يبقى **مكتوب في الملف**. التثبيت اللي السيرفر بيعمله للـ tag (§12.5) مايكفيش، لأن الـ tag وقت التنفيذ ممكن يبقى اتنقل لصورة حد تاني |
| الإضافات الموثوقة | `AllowedPlugins: [prefix]` | فاضي = أي إضافة | بادئة اسم صورة (`ghcr.io/khaymah/`)، والتحقق بالتوقيع مع سجل الإضافات بعد GA (ADR-0009) |
| مستوى العزل | `RequiredIsolation` | `container` | في أول نسخة مفيش غير `container`، فالقاعدة بتتحقق بس |
| التخطّي الإداري | `AllowAdminBypass` | لأ | للمراجعين والمؤقت بس (تحت) |
| استبدال الطلبات القديمة | `SupersedePending` | آه | تحت |
| الإيقاف («أوقف النشر») | `Paused` و`PausedReason` | — | يدوي، في Community بدل التجميد المجدول (قرار Sabry 2) |
| نوافذ النشر والتجميد المجدول | — | — | Business مع M8 (قرار Sabry 2) |

### البوابة: ترتيب الفحوص
لما الـ `needs` كلها تخلص صح، `Pipelines` بينادي `Core/Deployments.Gate` جوه نفس الـ transaction (port زي ADR-0070) بالترتيب ده:
1. **fork:** `skipped` بـ `FORK_PR_NO_ENVIRONMENT` (زي النهارده).
2. **البيئة موجودة:** وإلا `failed` بـ `ENVIRONMENT_NOT_FOUND`.
3. **المتغيّرات:** `vars` من البيئة والمشروع والمؤسسة، وبعدها `if` بيتقيّم. `false`: `skipped` بـ `CONDITION_FALSE`. (النهارده `if` بيتقيّم بعد فحص البيئة، والترتيب بيتعدّل عشان job شرطه `false` مايستناش موافقة.)
4. **قاعدة الـ ref:** الـ ref بتاع التنفيذ (والـ PR لو مسموح) لازم يطابق `AllowedRefs`. وإلا `skipped` بـ `ENVIRONMENT_REF_NOT_ALLOWED`، ومعاه في `"FailureDetail"` الـ ref والبيئة (من غير الـ patterns، والواجهة بتجيبهم بالصلاحية).
5. **القواعد الثابتة على الـ spec بعد الـ compile:** `ENVIRONMENT_REQUIRES_PINNED_IMAGES` و`ENVIRONMENT_PLUGIN_NOT_ALLOWED` و`ENVIRONMENT_ISOLATION_REQUIRED` (كلهم `failed`، لأنها غلطة في الملف تتصلّح).
6. **الأسرار موجودة:** `SECRET_NOT_FOUND` قبل ما حد يتطلب منه يوافق على job هيفشل.
7. **متوقفة:** الـ `RunJob` بيفضل `waiting` و`"Gate" = 'paused'`.
8. **الموافقة:** لو فيه مراجعين ومفيش موافقة صالحة للـ run والبيئة: طلب جديد، و`"Gate" = 'approval'`.
9. **المؤقت:** `"Gate" = 'timer'` لحد `"GateOpensAt"`.
10. **التزامن:** لو run تاني ماسك البيئة: `"Gate" = 'environment_busy'`.
11. **الإدراج:** الـ run بيمسك البيئة، وصف `"Deployments"`، و`"RunJobs"."EnvironmentId"` بيتثبّت، والـ job بيتدرج.
- **إعادة التقييم:** أي قرار موافقة بيعيد البوابة في نفس الـ transaction، والـ worker بتاع `Pipelines` كل 5 ثواني بيعدّي على الـ `RunJobs` اللي ليها `"Gate"` (المؤقت خلص، أو البيئة فضيت، أو اتشغّلت تاني).
- **الحالة:** الـ `RunJob` بيفضل `waiting`، والـ run بيفضل `running`. مفيش حالة جديدة في الـ triggers بتاعة الحالات، فالـ binary القديم (M4) لو رجعنا له بيشوف job فيه بيئة ويعلّمه `skipped` بـ `ENVIRONMENTS_UNAVAILABLE`: بيقفل مش بيفتح.

### الموافقات
- **طلب واحد لكل (run، بيئة، جولة):** كل jobs التنفيذ اللي على البيئة دي (ومعاها نسخ الـ matrix) بتعدّي بموافقة واحدة. «أعد تشغيل الفاشل» بيبدأ جولة جديدة بطلب جديد.
- **الطلب** مربوط بالـ run (يعني الـ sha والتعريف) وبـ `"RuleVersion"`.
- **مين يوافق:** حد في `Reviewers` (مباشرة أو من فريق)، عضو، ودوره Developer أو أعلى على المشروع، وعنده خطوة تانية مسجّلة ([قرار Sabry 5](#قرارات-sabry-2026-11-03))، ومش «نفسه» لو `PreventSelfReview` شغّال (الافتراضي، وAdmin يقدر يقفله للبيئة دي بس). «نفسه» = اللي عمل التشغيل اليدوي (`"TriggeredByUserId"`)، أو المستخدم اللي حسابه على GitHub أو GitLab المربوط بداو (M2.4) هو نفس الـ actor بتاع الـ push. في Git العام الـ pusher نص من الـ webhook ومش مربوط بحد، والواجهة بتقول كده جنب الطلب.
- **الموافقة:** `POST .../runs/{runId}/approvals/{approvalId}/approve` بـ `CurrentFactor` (TOTP أو كود طوارئ أو مفتاح مرور، زي تغييرات الهوية) و`Comment` اختياري (1 KiB). بتتسجّل بنوع الخطوة (مش القيمة).
- **الرفض:** `.../reject` بنفس الشروط. الـ jobs اللي مستنية بتفشل `DEPLOYMENT_REJECTED`، واللي بعدها `skipped` بـ `NEEDS_FAILED`.
- **الانتهاء:** الطلب اللي ماحدش رد عليه في `ApprovalTimeoutHours` بيخلص `expired`، والـ jobs بتفشل `APPROVAL_EXPIRED`.
- **push جديد:** لو `SupersedePending`، أول ما تنفيذ أحدث على **نفس الـ ref** يطلب موافقة لنفس البيئة، كل طلب أقدم لسه `pending` بيبقى `superseded` والـ jobs بتاعته `canceled` بـ `SUPERSEDED`. الموافقة اللي اتاخدت خلاص مابتتلغيش (الـ run ده هيعدّي، والأحدث بعده بالتزامن).
- **القاعدة اتغيّرت:** أي طلب `pending` على `"RuleVersion"` أقدم بيبقى `superseded` وبيتعمل طلب جديد. وموافقة اتاخدت على إصدار أقدم والـ job لسه مابدأش لازم تتعاد (الفحص في البوابة وفي `Start`).
- **المراجع اتشال:** الموافقة بتتفحص تاني عند فتح البوابة: لو صاحبها ماعادش مراجع أو عضو، الطلب بيرجع `pending`.
- **الإلغاء:** إلغاء الـ run بيلغي الطلبات.
- **التخطّي الإداري:** `.../bypass` لـ Owner وAdmin بس، لو البيئة فيها `AllowAdminBypass`، بـ `Reason` (إجباري، 1 KiB) و`CurrentFactor`. بيعدّي المراجعين والمؤقت بس. قاعدة الـ ref والقواعد الثابتة والإيقاف والـ forks مافيهاش تخطّي. بيتسجّل `environment.bypassed` وبيبان على الـ run وعلى صف النشر.
- **الخطوة التانية:** `deployment.approve` و`environment.bypass` من الإجراءات الحساسة (ADR-0054)، وفوقها `CurrentFactor` مع كل موافقة ورفض وتخطّي ([قرار Sabry 5](#قرارات-sabry-2026-11-03)).

### التزامن لكل بيئة
- **run واحد في البيئة في المرة:** الـ run بيمسك البيئة من أول job بيتدرج عليها لحد ما مايبقاش فيه `RunJob` منه على البيئة دي مش منتهي (أو الـ run يخلص أو يتلغي). كده `deploy` و`migrate-db` في نفس الـ run مايتداخلش بينهم run تاني.
- **التنفيذ:** `"Environments"."LockedByRunId"` بيتغيّر في الـ transaction اللي فيها `Gate` و`JobFinished` بعد قفل صف البيئة، وترتيب الأقفال: مجموعة التزامن، والـ run، وصف البيئة، وطابور المؤسسة. والـ reconciler بيصلّح أي قفل صاحبه خلص.
- **الطابور:** الأقدم في الموافقة أو الوصول للبوابة الأول، وبعده رقم الـ run.
- **مع `concurrency` بتاع الملف:** الاتنين شغّالين. مجموعة الملف لكل المشروع، وقفل البيئة لكل البيئة.

### شكل الـ job في صفحة التنفيذ
- **متخطّي بالـ ref:** شارة «اتخطّى» وكارت: «الفرع `feature/login` مش مسموح له ينشر على `production`. المسموح: `main`.» والـ patterns بتتعرض لأي حد عنده `run.read`، ورابط لصفحة البيئة لأصحاب `environment.manage`.
- **مستني موافقة:** شارة «مستني موافقة»، والمراجعين، ومن إمتى، وإمتى الطلب بينتهي، وزرار «وافق وانشر» و«ارفض» للمراجع (شريحة فيها الخطوة التانية والتعليق)، و«تخطّى القواعد» بلون الخطر وأيقونة للأدمن لو مسموح.
- **مستني المؤقت:** «هيبدأ الساعة …» بالوقت المحلي.
- **البيئة مشغولة:** «مستني التنفيذ #12 يخلص على `production`» برابط.
- **متوقفة:** «النشر على `production` متوقف: …» بالسبب.
- **الـ run في القايمة:** شارة «مستني موافقة» لو أي job فيه `"Gate" = 'approval'`، وعدّاد «موافقات مستنياك» في الرئيسية والقايمة.

## الـ forks والـ PRs والـ cache
- **الـ forks:** الـ resolver بيرفض أي سر لـ run فيه `"IsFork"` حتى لو الفحص في `Pipelines` اتكسر (دفاع تاني)، ومفيش متغيّرات ولا registry ولا OIDC (لما M5.8 ييجي).
- **الـ cache ([قرار Sabry 4](#قرارات-sabry-2026-11-03)):**
  - تنفيذات الـ forks مابترجّعش أي cache افتراضيًا.
  - Admin يقدر يفتح «الـ forks تقرا cache الفرع الافتراضي» من إعدادات المشروع (`"ForkCacheRestore"` = `none` أو `default_branch`)، بتحذير.
  - أي cache اتكتب من job وصله سر أو بيانات registry أو توكن OIDC بيتعلّم `"HadSecrets"`، وعمره ما بيرجع لـ fork حتى لو الإعداد مفتوح.
- **الـ artifacts:** الـ forks `Untrusted` زي M4، والبوابة بترفض job على بيئة بيستخدم artifact `Untrusted` (`UNTRUSTED_ARTIFACT`)، دفاع تاني لأن الـ fork مايوصلش للبيئة أصلًا.

## مُصدِر OIDC للأحمال
**إمتى:** بعد أول نسخة مباشرة ([قرار Sabry 3](#قرارات-sabry-2026-11-03)). التصميم هنا كامل عشان يتعمل في أي وقت بعد M5.6 (المرحلة M5.8)، ومش من شرط خلاص M5. أهداف أول نسخة SSH وCompose مش بتستخدمه، واللي محتاج سحابة لحد ساعتها يستخدم سر عادي.
- **الـ endpoints** (بره `/api/v1`، بـ `policy.Public`، وrate limit، و`Cache-Control: max-age=300`): `GET /.well-known/openid-configuration` و`GET /.well-known/jwks.json`. والـ issuer = `DHOW_PUBLIC_URL`، ولازم `https` وhost عام، وإلا الميزة مقفولة برسالة في `doctor`. وقالب nginx بتاع الـ installer بيعدّي المسارين دول.
- **الطلب:** الـ job بيعلن `id_token: {audiences: [...]}` (5 بالكتير)، وإلا مفيش توكن. والخطوة بتنادي `/run/dhow/bin/id-token --audience <aud>`، فالـ agent بيبعت للـ runner على قناة التحكم، والـ runner بينادي `IdentityService.MintIdToken(LeaseRef، audience، lifetime)` بالـ fencing، والتوكن بيتضاف للـ masker وبيتطبع على stdout الـ helper.
- **السيرفر بيتأكد:** الـ lease حي والـ fencing الحالي، والـ audience في اللي اتعلن، والـ run مش fork، ولو فيه بيئة البوابة عدّت، و60 توكن بالكتير لكل attempt.
- **الـ claims:** `iss` و`sub` (`project:<org slug>/<project slug>:environment:<name>`، أو `:ref:<ref>` من غير بيئة) و`aud` و`iat` و`nbf` (قبلها بـ 30 ثانية) و`exp` (5 دقايق افتراضيًا، 60 بالكتير) و`jti`، ومعاهم `organization_id` و`project_id` و`repository` و`ref` و`ref_type` و`sha` و`run_id` و`run_number` و`job` و`environment` و`event` و`deployment_id` و`definition_digest`.
- **التوقيع:** RS256 من الـ standard library (من غير مكتبة JWT، زي القرار 16 في M4)، لأن AWS وGCP وAzure كلهم بيقبلوه. حجم المفتاح بيتأكد في أول المرحلة (3072 لو الاتلاتة قبلوه، وإلا 2048).
- **المفاتيح:** `"OidcSigningKeys"` (الـ `kid` والـ JWK العام والخاص متشفّر بـ DEK الـ `instance`) بحالات `next` و`current` و`previous`. كل 90 يوم: المفتاح الجاي بيتنشر في الـ JWKS قبلها بـ 7 أيام، والقديم بيفضل 7 أيام بعدها.
- **الوثائق:** AWS (IAM OIDC provider وشرط `sub`) وGCP (Workload Identity Federation) وAzure (federated credential) وOpenBao/Vault (JWT auth)، بالنسختين.

## الواجهة
- **أسرار المؤسسة:** في إعدادات المؤسسة. جدول بالاسم والوصف والمشاريع وآخر تعديل وآخر استخدام. شريحة «ضيف سر» فيها الاسم والقيمة (`textarea` بتتمسح بعد الحفظ وعمرها ما بتترجع) والوصف والإتاحة للـ PRs. «غيّر القيمة» بيعمل إصدار جديد، و«امسح السر» في منطقة خطر.
- **المشروع:** تبويب «الأسرار والمتغيّرات» (نفس الجدول، ومعاه «بيغطي على سر المؤسسة»)، وتبويب «البيئات» (البيئات بالمنشور دلوقتي والقفل والإيقاف)، وصفحة البيئة (القواعد كفورم، وأسرارها ومتغيّراتها، وسجل النشر)، وبيانات دخول الـ registry في إعدادات المشروع والمؤسسة.
- **صفحة التنفيذ:** الحالات اللي فوق، وشريحة الموافقة.
- **الرئيسية والقايمة:** «موافقات مستنياك» بعدّاد، وصفحة بالطلبات المفتوحة للمستخدم.
- **إعدادات السيرفر:** كارت «سلسلة المفاتيح» للقراية بس (الإصدار الـ active، والتقدّم، وآخر تدوير، وآخر تصدير)، والتدوير نفسه من الـ CLI بس.
- **القواعد:** كل نص مفتاح في `Locales/Workspace/{Ar,En}.ts`، ومصطلحات جديدة في `Plan/Glossary.md` (متغيّر، ومراجع، وقواعد الحماية، ومؤقت الانتظار، وتخطّي القواعد، وسجل النشر، وسلسلة المفاتيح)، ومفيش قيمة سر في أي state أو URL أو `localStorage`، ومفيش `<style>` اتحقن، والصفحات lazy.

## القرارات المفوَّضة (كل واحد بيبقى ADR في أول مرحلته)
| # | القرار | ليه في سطر | المرحلة |
|---|---|---|---|
| 1 | طبقتين: KEK بره القاعدة بإصدارات، وDEK لكل صاحب في `"DataKeys"`، وAES-256-GCM، والـ AAD مربوط بالصف والصاحب | زي ADR-0014 و`architecture.md` §6، ومسح بالتشفير لكل صاحب | M5.1 |
| 2 | صيغة القيمة `0x02` فيها `DataKeyId`، و`Open` بيتأكد من الصاحب | القيمة مابتتنقلش بين صفوف ولا أصحاب، والصيغة القديمة تتعرف من أول byte | M5.1 |
| 3 | `"CheckValue"` لكل إصدار، والإقلاع بيقف لو أي إصدار ناقص أو مختلف | ملف غلط مايطلّعش بيانات بايظة بصمت (fail closed) | M5.1 |
| 4 | التفعيل في القاعدة بأمر، وبيتأكد من كل instance حي بالـ heartbeat | instance من غير المفتاح الجديد مايقدرش يفك اللي اتلف بيه | M5.1 |
| 5 | التدوير بيعيد لف `"DataKeys"` بس، والـ DEK مابيتدوّرش في M5 | ثواني بدل إعادة تشفير كل القيم، وبعد التسريب الحل تغيير القيمة نفسها | M5.1 |
| 6 | `DHOW_SECRET_KEY` بيفضل للمفاتيح المشتقة (توقيع وHMAC) بس، وتدويره بعد أول نسخة | تدويره بيعيد تسجيل الـ runners ويبطّل التوكنات، وده إجراء لوحده | M5.2 |
| 7 | نقل `Sealer` بثلاث خطوات على صيغة القيمة، وhook بيمنع أي `NewSealer` جديد | النسخة السابقة لازم تقرا كل حاجة (ADR-0077) | M5.2 |
| 8 | البيئات بيعملها Admin بس، والاسم الغلط `ENVIRONMENT_NOT_FOUND` (`failed`) | مفيش بيئة تتعمل من غير حماية بسبب غلطة في YAML | M5.3 |
| 9 | النطاقات مؤسسة ومشروع وبيئة، والأولوية بيئة ثم مشروع ثم مؤسسة، والمؤسسة لكل المشاريع أو لمختارين | زي GitHub وGitLab، والأقرب يكسب | M5.4 |
| 10 | 5 إصدارات 30 يوم، والمسح بـ `NULL` مش `DELETE`، ومفيش قراية أبدًا | الرجوع لقيمة قديمة من غير ما حد يشوفها، والحذف soft | M5.4 |
| 11 | القيمة من 8 byte لـ 32 KiB، و256 KiB لكل job، والأسماء من غير فرق في الحروف | الـ masker، وحد رسالة `Start`، و`Token` و`TOKEN` مايبقوش سرين | M5.4 |
| 12 | `vars.*` بنفس النطاقات وبتتقري بالـ API، والـ forks مابتاخدهاش | الـ spec بيقبلها والبيئات محتاجاها، وهي إعدادات المؤسسة | M5.4 |
| 13 | بيانات دخول الـ registry نوع سر بنطاق، والـ runner بيستخدمها في authfile بره الـ sandbox | §12.5 وعد بيها، والـ job مايشوفهاش | M5.4 وM5.5 |
| 14 | `files:` في الخطوة والـ job، والأسرار بتتحل وقت `Start` بس، وبيتعاد فحص الـ fork والبيئة والقاعدة | ملفات مفاتيح SSH للبيئات، والـ spec مش مصدر ثقة | M5.5 |
| 15 | أسرار المؤسسة والمشروع مش للـ PRs إلا بعلامة، والـ forks ولا حاجة | تقليل اللي بيوصل لكود لسه ماتراجعش | M5.5 |
| 16 | الـ runner بيفحص الـ artifacts والـ cache بقيم الأسرار | الـ artifact بيتنزّل من Developer، وده تسريب مش بيعدّي على الـ masker | M5.5 |
| 17 | البوابة بعمود `"Gate"` على `"RunJobs"` مش حالة جديدة، وترتيب الفحوص اللي فوق | الـ binary القديم بيقفل، ومفيش تعديل على triggers الحالات | M5.6 |
| 18 | موافقة واحدة لكل (run، بيئة، جولة)، و72 ساعة، واستبدال الـ `pending` الأقدم على نفس الـ ref، ومربوطة بـ `"RuleVersion"` | المراجع بيوافق على نسخة الكود مش على كل job، والقاعدة المتغيّرة محتاجة موافقة جديدة | M5.6 |
| 19 | run واحد لكل بيئة، ماسكها لحد ما jobs البيئة بتاعته تخلص | نشر ومايجريشن من نفس الـ run مايتداخلش بينهم حد | M5.6 |
| 20 | «الصور بالـ digest» = في الملف، والإضافات ببادئة الاسم، والتخطّي للمراجعين والمؤقت بس | الـ tag بيتنقل، والتوقيع مع السجل بعد GA، والـ ref قرار Sabry مالوش تخطّي | M5.6 |
| 21 | OIDC (بعد أول نسخة): RS256 من الـ standard library، و5 دقايق، وعند الطلب من خلال الـ runner، ومفاتيح كل 90 يوم | أوسع توافق، ومن غير مكتبة JWT، والتوكن مايتولدش قبل ما الخطوة تحتاجه | M5.8 |

## الأمان
| الخطر | التخفيف | فين بيتختبر |
|---|---|---|
| القاعدة اتسرّبت (`pg_dump`) | كل القيم بـ DEKs ملفوفة بـ KEK مش في القاعدة | `leaksweep` على `pg_dump` بقيم مزروعة وبالـ DEKs بعد فكها: مالقاش |
| ملف السلسلة اتسرّب لوحده | مالوش لازمة من غير القاعدة، والتدوير بيعيد اللف | دليل الطوارئ، واختبار التدوير |
| ملف سلسلة غلط أو ناقص | `CheckValue` لكل إصدار، والإقلاع بيقف | اختبار إقلاع بملف ناقص إصدار، وبمفتاح متبدّل، وبسطر بايظ |
| قيمة متنقلة بين صفوف أو أصحاب | الـ AAD فيه الصف والغرض، و`Open` بيتأكد من صاحب الـ DEK | اختبار بينسخ قيمة لصف تاني ولمؤسسة تانية: `ErrSealed` |
| instance بيلف بمفتاح الباقيين مش معاهم | التفعيل بيتأكد من الـ heartbeat | instance وهمي من غير الإصدار: `KEK_NOT_EVERYWHERE` |
| السر بيرجع في الـ API | DTOs من غير `Value`، واختبار بيلف على كل الـ responses | اختبار انعكاس على كل نوع في `Gen/` بيدوّر على حقول القيمة، وقيمة مزروعة في كل الـ E2E |
| السر في logs السيرفر أو الـ audit أو الأحداث | `LogValue` و`Format` و`MarshalJSON` بتخفي، والـ audit بالـ allowlist، والـ access log من غير bodies | قيمة مزروعة بتتدوّر في كل log الاختبار والـ audit وأحداث الـ bus |
| السر في سجل الـ job أو الـ outputs | الـ masker في الـ runner (ADR-0045) | السر بكل الصيغ في الـ log والـ outputs: متقنّع |
| السر في artifact أو cache | فحص قبل الضغط | artifact فيه السر: `ARTIFACT_CONTAINS_SECRET`، وcache فيه السر: مااتحفظش |
| fork بياخد سر أو بيئة | الفحص في `Pipelines` وتاني في الـ resolver | PR من fork بسر وبيئة: `FORK_PR_NO_SECRETS` و`FORK_PR_NO_ENVIRONMENT`، وresolver بيترفض لو الـ run fork مباشرة |
| fork بيقرا cache فيه أسرار | الـ forks مابترجّعش، و`"HadSecrets"` عمره ما بيرجع لـ fork | cache من job بسر، والإعداد مفتوح: الـ fork مالقاهوش |
| commit بيضعّف الحماية | القواعد على السيرفر بس، ومفيش حاجة منها في YAML | push بيغيّر `.dhow.yml` لبيئة تانية أو بيشيل `environment:`: القاعدة زي ما هي، والأسرار بتاعة البيئة مابتوصلش لـ job من غير البيئة |
| فرع مش مسموح بينشر | قاعدة الـ ref في البوابة وفي `Start` | push على `feature/x` لبيئة `main` بس: `skipped` ومفيش أسرار اتفكّت |
| المراجع بيوافق على كوده | `PreventSelfReview` بالمستخدم والهوية المربوطة | اللي شغّل يدوي بيوافق: `SELF_REVIEW_NOT_ALLOWED` |
| موافقة مزوّرة أو من جلسة مسروقة | جلسة وCSRF و`CurrentFactor` مع كل موافقة | من غير خطوة، وبخطوة غلط، وبتوكن API: مرفوض |
| موافقة بتتعاد على كود تاني | الطلب مربوط بالـ run والـ sha و`"RuleVersion"` | موافقة run قديم مابتفتحش run جديد، وتغيير القاعدة بيطلب من تاني |
| تخطّي إداري بيتساء استخدامه | مقفول افتراضيًا، وسبب وخطوة تانية وaudit، وظاهر على الـ run | تخطّي من غير السماح أو من Maintainer: مرفوض، والحدث بيتسجّل |
| إنكار الموافقة (Repudiation) | audit بالفاعل ونوع الخطوة والتعليق، بسلسلة الـ hash | الحدث بالحقول في اختبار الـ audit |
| طلبات مفتوحة للأبد | انتهاء 72 ساعة افتراضيًا | ساعة وهمية: `APPROVAL_EXPIRED` |
| runner من مؤسسة تانية بياخد الأسرار | الـ lease بالمؤسسة وHPKE لمفتاح الـ runner اللي ماسك الـ lease (M3) | اختبار العزل بين المؤسسات بيتوسّع للأسرار |
| Maintainer بيطلّع سر البيئة بكود | أسرار البيئة بعد قاعدة الـ ref والمراجعة بس، والـ masker | خطر مقبول ومكتوب (§13.2) |
| بيانات دخول registry بتروح لـ host تاني | للـ host نفسه وللـ realm اللي بيعدّي قواعد §12.5 بس، و`netguard` | realm على host تاني أو IP خاص: مابيتبعتلوش |
| توكن OIDC اتسرق من الـ job (M5.8) | 5 دقايق، و`aud` محدد، والـ masker، و`sub` فيه البيئة | توكن بعد `exp` وبـ `aud` تاني: مرفوض في اختبار التحقق |
| مفتاح توقيع OIDC اتسرّب (M5.8) | متشفّر بـ DEK، والتدوير كل 90 يوم، وتدوير فوري بأمر | `leaksweep` بيدوّر على المفتاح الخاص |

**الأسرار للكتابة بس (اختبار):** نفس اختبار M4 بيتوسّع: كل DTO جديد مافيهوش قيمة سر ولا قيمة registry ولا مفتاح OIDC خاص ولا DEK، والـ access log مابيسجّلش bodies الـ routes دي.

## قاعدة البيانات
كل الجداول بالأعمدة الأربعة، والوقت `timestamptz`، والنص `TEXT`، والحذف soft، ومفيش `DELETE FROM` في `Queries/`، وكل ملف أوله `-- dhow:phase expand` (أو `contract` في M5.9)، والـ indexes على جداول موجودة `CONCURRENTLY` في ملف `NO TRANSACTION` لوحده. الأرقام الفاضية وقت الـ merge (آخر رقم حاليًا `00049`).

| الجدول | الموديول | أهم الأعمدة والقيود |
|---|---|---|
| `"KeyEncryptionKeys"` | `Core/Keys` (الـ migration في `Database`) | `"Version"` (unique، من 1) و`"CheckValue"` (bytea 32) و`"Provider"` (`file`) و`"State"` IN (`pending`, `active`, `retiring`, `retired`)، وواحد بس `active` (partial unique) و`"ActivatedAt"` و`"RetiredAt"` و`"LastExportedAt"` |
| `"DataKeys"` | `Core/Keys` | `"OwnerKind"` IN (`instance`, `organization`, `project`, `environment`, `user`) و`"OwnerId"` (NULL لـ `instance` بس، CHECK) و`"KekVersion"` و`"Wrapped"` (bytea 60، NULL بعد المسح) و`"State"` (`active` أو `destroyed`)، وunique (`"OwnerKind"`, `"OwnerId"`) WHERE `active`، وindex على `"KekVersion"` للـ worker |
| `"Environments"` | Environments | `"OrganizationId"` و`"ProjectId"` (FK مركّب) و`"Name"` (unique مع المشروع WHERE NOT deleted) و`"Description"` و`"RuleVersion"` و`"AllowedRefs"` (jsonb، 50) و`"AllowPullRequests"` و`"RequiredApprovals"` (CHECK = 1) و`"PreventSelfReview"` و`"WaitMinutes"` و`"ApprovalTimeoutHours"` و`"RequirePinnedImages"` و`"AllowedPlugins"` (`TEXT[]`، 50) و`"RequiredIsolation"` و`"AllowAdminBypass"` و`"SupersedePending"` و`"PausedAt"` و`"PausedByUserId"` و`"PausedReason"` و`"LockedByRunId"` و`"Version"` (optimistic) |
| `"EnvironmentReviewers"` | Environments | `"EnvironmentId"` و`"UserId"` أو `"TeamId"` (واحد بس، CHECK)، unique |
| `"DeploymentApprovals"` | Environments | `"RunId"` و`"EnvironmentId"` و`"Round"` (unique التلاتة) و`"RuleVersion"` و`"State"` IN (`pending`, `approved`, `rejected`, `expired`, `superseded`, `canceled`, `bypassed`) و`"ExpiresAt"` و`"DecidedByUserId"` و`"DecidedAt"` و`"FactorKind"` و`"Comment"` (1 KiB) و`"BypassReason"`، وtrigger بيقفل الانتقالات من `pending` بس |
| `"Deployments"` | Environments | `"EnvironmentId"` و`"RunId"` و`"RunJobId"` و`"JobId"` و`"Sha"` و`"Ref"` و`"ApprovalId"` و`"State"` (`queued`, `running`, `succeeded`, `failed`, `canceled`) و`"StartedAt"` و`"FinishedAt"`، وindex (`"EnvironmentId"`, `"CreatedAt"` DESC) |
| `"Secrets"` | Secrets | `"OrganizationId"` و`"ProjectId"` و`"EnvironmentId"` (NULL حسب النطاق، CHECK إن البيئة معاها مشروع) و`"Name"` (unique على `lower` مع النطاق WHERE NOT deleted) و`"Description"` (256) و`"CurrentVersion"` و`"Visibility"` و`"AvailableToPullRequests"` و`"CreatedByUserId"` و`"UpdatedByUserId"` و`"LastUsedAt"` |
| `"SecretVersions"` | Secrets | `"SecretId"` و`"Version"` (unique الاتنين) و`"ValueSealed"` (bytea، NULL بعد المسح) و`"SizeBytes"` و`"CreatedByUserId"` و`"ErasedAt"`، وCHECK (`"ValueSealed" IS NULL`) = (`"ErasedAt" IS NOT NULL`) |
| `"SecretProjectAccess"` | Secrets | `"SecretId"` و`"ProjectId"`، unique |
| `"Variables"` | Secrets | نفس نطاق `"Secrets"` و`"Name"` و`"Value"` (4 KiB) و`"UpdatedByUserId"` |
| `"RegistryCredentials"` | Secrets | `"OrganizationId"` و`"ProjectId"` و`"Host"` (unique مع النطاق) و`"Username"` و`"PasswordSealed"` |
| `"OidcSigningKeys"` (M5.8، بعد أول نسخة) | WorkloadIdentity | `"Kid"` (unique) و`"PublicJwk"` (jsonb) و`"PrivateSealed"` و`"State"` (`next`, `current`, `previous`, `retired`) و`"ActivatesAt"` و`"RetiresAt"` |

- **أعمدة جديدة (expand):**
  - `"RunJobs"`: `"EnvironmentId"` و`"Gate"` (`TEXT` CHECK IN (`approval`, `timer`, `paused`, `environment_busy`)) و`"GateOpensAt"` و`"FailureDetail"` (jsonb `{}`، 1 KiB).
  - `"CacheEntries"`: `"HadSecrets"` (`boolean NOT NULL DEFAULT false`).
  - `"Projects"`: `"ForkCacheRestore"` (`TEXT NOT NULL DEFAULT 'none'`).
  - `"ServerInstances"` (من U2): `"KekVersions"` (`integer[] NOT NULL DEFAULT '{}'`).
- **أسباب فشل جديدة** على `"RunJobs"."FailureReason"` (64 حرف بالكتير): `ENVIRONMENT_NOT_FOUND` و`ENVIRONMENT_REF_NOT_ALLOWED` و`ENVIRONMENT_REQUIRES_PINNED_IMAGES` و`ENVIRONMENT_PLUGIN_NOT_ALLOWED` و`ENVIRONMENT_ISOLATION_REQUIRED` و`ENVIRONMENT_RULE_CHANGED` و`SECRET_NOT_FOUND` و`SECRET_NOT_AVAILABLE_TO_PULL_REQUESTS` و`SECRETS_TOO_LARGE` و`DEPLOYMENT_REJECTED` و`APPROVAL_EXPIRED` و`SUPERSEDED` و`UNTRUSTED_ARTIFACT` و`ARTIFACT_CONTAINS_SECRET`. الـ binary القديم بيعرضهم نص زي ما هم (الحقل `TEXT`).
- **`"ApiTokens"`:** migration بيوسّع الـ CHECK على `"Scopes"` بـ `secret.read` بس ([قرار Sabry 6](#قرارات-sabry-2026-11-03)): الأسماء والـ metadata، عمره ما بيقرا قيمة. ومفيش scope للكتابة ولا للموافقة في M5.
- **الـ FK بين الموديولات:** مفيش (زي M4)، والتحقق في الـ service. والـ FK المركّب على `"Projects"` و`"PipelineRuns"` زي `"Repositories"`.

## الـ API

### REST (`packages/openapi/openapi.yaml`، والـ tags `secrets` و`environments`)
| Route | الإجراء | ملاحظة |
|---|---|---|
| `GET /organizations/{id}/secrets` | `secret.read` | metadata بس |
| `PUT /organizations/{id}/secrets/{secretName}` | `org.secret.manage` | `Value` للكتابة بس، و`Visibility` و`ProjectIds` |
| `POST /organizations/{id}/secrets/{secretName}/remove` | `org.secret.manage` | |
| `POST /organizations/{id}/secrets/{secretName}/restore-version` | `org.secret.manage` | `Version` |
| `GET` و`PUT .../organizations/{id}/variables[/{variableName}]` و`.../remove` | `project.read` و`org.secret.manage` | |
| `GET` و`PUT .../organizations/{id}/registry-credentials[/{credentialId}]` و`.../remove` | `secret.read` و`org.secret.manage` | `Password` للكتابة بس |
| `GET` و`PUT` و`POST .../remove` و`.../restore-version` على `/organizations/{id}/projects/{projectId}/secrets[/{secretName}]` | `secret.read` و`secret.manage` | |
| نفس الشكل لـ `.../projects/{projectId}/variables` و`.../projects/{projectId}/registry-credentials` | `project.read` و`secret.manage` | |
| `GET /organizations/{id}/projects/{projectId}/environments` | `project.read` | ومعاها المنشور دلوقتي والقفل |
| `POST /organizations/{id}/projects/{projectId}/environments` | `environment.manage` | الاسم والقواعد |
| `GET /organizations/{id}/projects/{projectId}/environments/{environmentName}` | `project.read` | القواعد والمراجعين |
| `PUT /organizations/{id}/projects/{projectId}/environments/{environmentName}` | `environment.manage` | `Version` و`CurrentFactor` |
| `POST .../environments/{environmentName}/remove` | `environment.manage` | `CurrentFactor`، وبيمسح أسرارها والـ DEK بتاعها |
| `POST .../environments/{environmentName}/pause` | `environment.pause` | `Reason` |
| `POST .../environments/{environmentName}/resume` | `environment.manage` | `CurrentFactor` |
| `GET` و`PUT` و`POST .../remove` على `.../environments/{environmentName}/secrets[/{secretName}]` و`.../variables` | `secret.read` و`environment.manage` | |
| `GET .../environments/{environmentName}/deployments` | `run.read` | صفحات |
| `GET /organizations/{id}/deployment-approvals` | `run.read` | `?State=pending&Mine=true`، مفلترة بالمشاريع اللي يقدر يشوفها |
| `GET .../projects/{projectId}/runs/{runId}/approvals` | `run.read` | |
| `POST .../runs/{runId}/approvals/{approvalId}/approve` | `deployment.approve` | `CurrentFactor` و`Comment`، والمراجع والنفس في الـ service |
| `POST .../runs/{runId}/approvals/{approvalId}/reject` | `deployment.approve` | نفس الشروط |
| `POST .../runs/{runId}/approvals/{approvalId}/bypass` | `environment.bypass` | `Reason` و`CurrentFactor` |
| `GET /system/keys` | `instance.settings.manage` | حالة السلسلة للقراية بس |

- **الـ `RunJob` في تفاصيل الـ run** بيزيد عليه `Environment` و`Gate` و`GateOpensAt` و`ApprovalId` و`FailureDetail`.
- **الـ OIDC** (M5.8 بعد أول نسخة، بره `/api/v1`): `GET /.well-known/openid-configuration` و`GET /.well-known/jwks.json` بـ `policy.Public`.
- **الأخطاء الجديدة (`UPPER_SNAKE`، مترجمة من `Accept-Language`):** `SECRET_TOO_SHORT` و`SECRET_TOO_LARGE` و`SECRET_NAME_RESERVED` و`SECRET_LIMIT_REACHED` و`ENVIRONMENT_EXISTS` و`ENVIRONMENT_VERSION_CONFLICT` و`REVIEWER_NOT_ELIGIBLE` و`REVIEWER_NEEDS_SECOND_FACTOR` و`SELF_REVIEW_NOT_ALLOWED` و`APPROVAL_NOT_PENDING` و`BYPASS_NOT_ALLOWED` و`KEK_NOT_EVERYWHERE`.

**الأدوار المقترحة** (على المشروع من `"RoleBindings"` لو موجودة، وغير كده دور المؤسسة):

| الإجراء | Owner | Admin | Maintainer | Developer | Viewer |
|---|:-:|:-:|:-:|:-:|:-:|
| `secret.read` (الأسماء والـ metadata) | ✅ | ✅ | ✅ | ✅ | — |
| `secret.manage` (أسرار ومتغيّرات وregistry المشروع) | ✅ | ✅ | ✅ | — | — |
| `org.secret.manage` (نطاق المؤسسة) | ✅ | ✅ | — | — | — |
| `environment.manage` (البيئة وقواعدها وأسرارها) | ✅ | ✅ | — | — | — |
| `environment.pause` | ✅ | ✅ | ✅ | — | — |
| `deployment.approve` (ولازم يبقى مراجع في البيئة) | ✅ | ✅ | ✅ | ✅ | — |
| `environment.bypass` (لو البيئة سامحة) | ✅ | ✅ | — | — | — |

- **التوكنات:** `secret.read` بس. أي route بيكتب سر أو متغيّر أو بيئة أو بيوافق بيرفض التوكن (قرار Sabry 6)، وصفوف الـ route-table بتتأكد.
- **حساسة (ADR-0054):** `secret.manage` و`org.secret.manage` و`environment.manage` و`deployment.approve` و`environment.bypass`.
- **الإيقاف والاستكمال مش متساويين عن قصد:** وقف النشر وقت مشكلة Maintainer يقدر عليه، والرجوع Admin بخطوة تانية.
- **كل route** ليه صف في اختبار الـ route-table لكل دور، ومعاه binding على المشروع بيرفع أو بيقلّل الدور.

### الـ proto (`packages/proto/dhow/runner/v1`، إضافات بس، و`buf breaking` أخضر)
```
JobSpec      += registry_auth[] RegistryAuth { host, secret_name }   (M5.5)
             += id_token IdTokenGrant { audiences[] }                (M5.8, after the first release)
AttemptFailureReason += ARTIFACT_CONTAINS_SECRET

IdentityService.MintIdToken(LeaseRef, audience, lifetime_seconds) → token, expires_at   (M5.8, deferred)

features: registry_auth (M5.5), id_token (M5.8)
agent frame: IdTokenRequest { audience, lifetime }  (from /run/dhow/bin/id-token, like the mask helper)
```
- `files:` بتستخدم `SecretRef.as_file` الموجود من M3، فالـ runner القديم بيدعمه.
- السيرفر مابيدّيش job فيه `registry_auth` أو `id_token` لـ runner ماعلنش الـ feature في تسجيله (زي M4)، والـ job بيستنى `NO_MATCHING_RUNNER` ظاهر.

### الإعدادات الجديدة
- **السيرفر** (في `docs/install/configuration.md` بالنسختين، ومش في «إعدادات السيرفر» لأنها مفاتيح):
  - `DHOW_KEYRING_FILE` أو الـ credential `dhow-keyring`: إجباري، والإقلاع بيقف من غيره.
  - `DHOW_OIDC_ISSUER` (M5.8، بعد أول نسخة): `on` أو `off`، والافتراضي `off` لحد ما الأدمن يفتحه.
- **الثوابت** (مش env): حدود الأسرار والمتغيّرات، و`ApprovalTimeoutHours` على البيئة، وإيقاع إعادة اللف.

## الوثائق (بالنسختين، و`make docs-check`)
| الملف | المرحلة | المحتوى |
|---|---|---|
| `docs/install/keys.md` (جديد) | M5.1 | سلسلة المفاتيح، والتدوير، والتصدير، والنسخ الاحتياطي، ودليل الطوارئ لو حاجة اتسرّبت |
| `docs/install/{configuration,binary-systemd,compose,installer}.md` | M5.1 | `DHOW_KEYRING_FILE` والـ credential و`./secrets/` في Compose |
| `docs/guides/secrets.md` (جديد) | M5.4 | النطاقات والأولوية والـ write-only والإصدارات والمتغيّرات والـ registry |
| `docs/guides/environments.md` (جديد) | M5.6 | القواعد والموافقات والتزامن والسجل والتخطّي وشكل الـ jobs |
| `docs/pipeline-spec.md` | M5.5 وM5.6 | `files:` و`vars` والـ registry (§12.5)، وشيل سطر M5 من §8، و§9 كامل، و§12.4 والـ cache |
| `docs/architecture.md` | M5.1 | §6 بالطبقات، وتصحيح سطر «إلا لو أدمن وافق» للـ forks (بيعارض ADR-0076)، و§7 بيقول إن المُصدِر بعد أول نسخة (قرار Sabry 3) |
| `docs/security-model.md` | M5.9 | §8 بالتفاصيل، و§13 مراجعة تهديدات M5 (الجدول والمخاطر المقبولة)، وإضافات لقايمة المراجعة الخارجية |
| `docs/guides/oidc-workloads.md` (جديد) | M5.8 (بعد أول نسخة) | AWS وGCP وAzure وOpenBao/Vault |
| `docs/roadmap.md` | M5.9 | M5 خلص. (OIDC اتنقل لـ «بعد أول نسخة» والتجميد لـ M8 في الخطة دي نفسها) |

## Edge cases
- **السر اتمسح أو اتغيّر اسمه وهو مستني موافقة:** `SECRET_NOT_FOUND` في `Start`، والـ attempt بيفشل من غير ما أي سر يتفك.
- **السر اتغيّر بين الموافقة والبدء:** القيمة الجديدة هي اللي بتوصل. الموافقة على الكود مش على القيم، ومكتوب في الوثائق.
- **البيئة اتمسحت والـ job مستني:** `ENVIRONMENT_NOT_FOUND`، وسجل النشر والطلبات بيفضلوا (soft)، والـ DEK بيتصفّر.
- **القاعدة اتغيّرت والـ job مستني:** بيتقيّم من الأول: ref ماعادش مسموح يبقى `skipped`، ومراجعين اتغيّروا يبقى طلب جديد.
- **push بيحرّك الـ tag** (tag اتمسح واتعمل على commit تاني): تنفيذ جديد على الـ sha الجديد وطلب جديد. حماية الـ tags نفسها عند المزوّد، والوثائق بتقول اقفلها هناك.
- **تشغيل يدوي لـ commit قديم على فرع مسموح (رجوع):** مسموح، والـ sha لازم يكون اتشاف على الفرع (`SHA_NOT_ON_REF` زي M4)، وسجل النشر بيعلّمه «أقدم من المنشور».
- **PR من نفس المستودع وبيئة مش سامحة للـ PRs:** `skipped` بـ `ENVIRONMENT_REF_NOT_ALLOWED`، مش `failed`.
- **مراجع واحد هو نفسه اللي عمل الـ push** والـ `PreventSelfReview` شغّال: مفيش حد يوافق. الطلب بيفضل `pending` لحد ما ينتهي، والواجهة بتقول «مفيش مراجع تاني»، والتخطّي لو مسموح.
- **فريق المراجعين فاضي:** إنشاء البيئة أو تعديلها بيحذّر، والطلب بيقول «مفيش مراجعين».
- **مراجع ساب المؤسسة بعد ما وافق وقبل البدء:** الموافقة بتتفحص تاني، والطلب بيرجع `pending`.
- **jobين في نفس الـ run على نفس البيئة** (`deploy` و`migrate-db`): موافقة واحدة، ومؤقت واحد، والقفل لحد ما الاتنين يخلصوا.
- **matrix على بيئة:** النسخ بتعدّي بموافقة واحدة، وبتشتغل حسب `max_parallel` جوه نفس الـ run.
- **«أعد تشغيل الفاشل» لـ job بيئة:** جولة جديدة وموافقة جديدة. والإعادة بتاعة البنية التحتية جوه نفس الـ job (قبل أي خطوة `mutating`، ADR-0047) مش محتاجة موافقة.
- **الـ run اتلغى وهو ماسك البيئة:** القفل بيتساب في نفس الـ transaction، والـ reconciler بيصلّح لو حاجة فاتت.
- **السيرفر وقع والـ run ماسك البيئة:** القفل في القاعدة، والـ reconciler بيشيل القفل اللي الـ run بتاعه خلص.
- **الإيقاف وفيه job شغّال:** الشغّال بيكمّل (الإيقاف للبدايات الجديدة)، والواجهة بتقول كده، وزرار الإلغاء موجود.
- **مؤقت 30 يوم والسيرفر اتقفل في النص:** الوقت من القاعدة (`now()`)، فالمؤقت بيكمّل صح.
- **runner قديم (M4) وjob فيه `registry_auth`:** `NO_MATCHING_RUNNER` ظاهر. وjob فيه `files:` بس بيشتغل لأن `as_file` موجود من M3.
- **سر أقصر من 8:** مرفوض وقت الكتابة. والأسرار قبل M5 مفيش.
- **سر قيمته بتتكرر في السجل كنص عادي** (زي `production`): هيتقنّع في كل مكان. الواجهة بتحذّر لو القيمة كلمة واحدة شائعة أو أقل من 12 حرف.
- **سر متعدد السطور (PEM):** كل سطر بيتقنّع لوحده (ADR-0045)، وكملف أحسن من env.
- **مجموع أسرار الـ job أكبر من 256 KiB:** `SECRETS_TOO_LARGE` قبل الإدراج، والحل ملف أصغر أو أسرار أقل.
- **ملف السلسلة صلاحياته مفتوحة:** الإقلاع بيقف (`secretValue` الموجود).
- **تفعيل KEK وinstance قديم شغّال:** `KEK_NOT_EVERYWHERE` بأسماء الـ instances، ومفيش أي تغيير.
- **إعادة اللف اتقطعت:** بتكمّل من مكانها، والقراية شغّالة طول الوقت.
- **استرجاع نسخة قديمة بعد شيل إصدار قديم من الملف:** السلسلة اللي جوه النسخة بترجع معاها، ولو اتاخدت القاعدة بس يبقى الإقلاع بيقف بـ `KEYRING_MISSING_VERSION`.
- **القاعدة اترجعت من غير السلسلة:** الأسرار ضاعت. الإقلاع بيقف، و`keys init --discard-unreadable` (بتأكيد صريح) بيعلّم كل القيم ممسوحة عشان السيرفر يقوم والناس تدخّل الأسرار تاني، وTOTP بيتطلب يتسجّل تاني. ده مكتوب في دليل الطوارئ.
- **(M5.8) `DHOW_PUBLIC_URL` اتغيّر والـ OIDC شغّال:** الـ issuer اتغيّر، وكل الثقة في السحابة هتقع. `doctor` والواجهة بيحذّروا قبل الحفظ، والقديم مش بيتنشر.
- **(M5.8) فرق الساعة في OIDC:** `nbf` قبلها بـ 30 ثانية، و`doctor` بيحذّر من فرق أكبر من 60 ثانية.

## تقسيم الـ PRs
كل PR بيعدّي `make check` وCI أخضر، وفيه اختبارات سلبية لكل مسار أمني، وبيحدّث `Plan/Progress/` في يومه، وبيكتب الـ ADR بتاع قراراته في أوله، وعليه مراجعة أمنية مستقلة قبل الـ merge. والأوامر بتفترض نفس `DHOW_TEST_DATABASE_URL` بتاع M3 وM4.

### M5.1: سلسلة المفاتيح والتشفير المغلّف (`Core/Keys`)
- **الحالة:** اتدمج في #91، والقرارات في [ADR-0081](ADR/0081-keyring-and-envelope-encryption.md).
- **الحجم:** M. **الاعتماد:** U2 (`"ServerInstances"` و`"SchemaFloor"`) 
- **المحتوى:**
  - `apps/server/internal/Core/Keys` (`package keys`): قراية السلسلة وفحصها، و`KeyProvider` بـ `file` و`fake`، و`Envelope.Seal` و`Open` بالصاحب والغرض والـ context، والـ DEK lazy (`INSERT ... ON CONFLICT DO NOTHING` وبعده `SELECT`)، و`Destroy(owner)`.
  - الـ migrations: `"KeyEncryptionKeys"` و`"DataKeys"` و`"ServerInstances"."KekVersions"`.
  - الإقلاع: التسجيل والـ check values والـ fail closed، و`keyring` في `/health/ready` و`doctor`.
  - worker إعادة اللف تحت advisory lock.
  - `dhow-server keys init|status|generate|activate|export` (بـ argv، و`export` بـ `filippo.io/age`)، و`keys init --discard-unreadable` لحالة القاعدة اللي اترجعت من غير سلسلتها (بتأكيد صريح، وبيعلّم القيم اللي مابتتفتحش ممسوحة ويسجّل في الـ audit)، و`GET /system/keys`.
  - `deploy/systemd/dhow.service` (`LoadCredential=dhow-keyring:...`)، والـ installer بيعمل الملف، و`deploy/compose/init.sh` و`compose.yaml` (`secrets:`)، وحزمة التطوير (`deploy/dev`).
  - تعديل U5 في `Upgrades-and-Help.md`: `./secrets/` في نسخة Compose، والاسترجاع بيقارن الـ check values.
  - الوثائق: `docs/install/keys.md` والإعدادات، و`architecture.md` §6.
  - ADR للقرارات 1 لـ 5، وقرار Sabry 1 (الملف، وKMS بعدين ورا نفس الـ interface).
- **قبول:**
  - **الشكل:** القيمة المختومة مافيهاش النص، ونفس القيمة مرتين بتطلع مختلفة، وقيمة صف بتتفتح لنفس الصف بس، وقيمة صاحب مابتتفتحش لصاحب تاني حتى بنفس الـ context.
  - **الإقلاع:** ملف ناقص إصدار في القاعدة، أو مفتاح متبدّل، أو سطر بايظ، أو صلاحيات مفتوحة: السيرفر بيقف برسالة فيها الإصدار، وexit code ثابت.
  - **التدوير:** 10,000 DEK على v1، و`generate` و`activate 2`، والـ worker بيتقتل 3 مرات في النص ويكمّل، وكل قيمة بتتفتح طول الوقت، وفي الآخر v1 `retired`.
  - **أكتر من instance:** instance وهمي ماعندوش v2: `KEK_NOT_EVERYWHERE` ومفيش تغيير.
  - **المسح بالتشفير:** بعد `Destroy` مفيش قيمة للصاحب ده بتتفتح.
  - **التصدير:** الملف بيتفك بمفتاح `age` الخاص في الاختبار بس، والسيرفر مالوش وصول له.
- **التحقق:**
  - `go test -race -count=1 ./apps/server/internal/Core/Keys/... ./apps/server/internal/Config/...`
  - `go test -run '^$' -fuzz FuzzKeyring -fuzztime 60s ./apps/server/internal/Core/Keys` و`FuzzOpen`.
  - `make gen && git diff --exit-code` و`make hooks-test` و`make docs-check` و`make check`.

### M5.2: نقل مستخدمي `Sealer` للتشفير المغلّف
- **ملزم (من المراجعة الأمنية لـ M5.1):** M5.2 لازم يضيف `dhow-server keys rekey`: DEK جديد لكل صاحب، وإعادة ختم كل قيمة من خلال «أماكن قيم» مسجّلة (كل عمود متشفّر بيسجّل الصاحب والـ label والـ context بتوعه)، وبعدها مسح الـ DEK القديم. لأن تدوير الـ KEK مابيحميش من سلسلة اتسرّبت مع نسخة قاعدة قديمة، و`Destroy` مابيمنعش نسخة أقدم. M5 مابيخلصش من غيره. وفيه اختبار: سلسلة قديمة ونسخة قاعدة من قبل الـ rekey مابيفتحوش أي قيمة حالية.
- **الحالة:** على `feat/sealer-to-envelope-m52` مع U4، والقرارات في [ADR-0082](ADR/0082-sealer-users-move-to-the-envelope.md). الصيغة الجديدة هي `0x03` بتاعة M5.1 (مكان `0x02` في النص تحت). `GitConnections.TokenSealed` مالوش كاتب لحد M4.6 فاتأجّل معاه، و`GitHubApps` مش موجود (M4.5)، و`deploy/upgrade-test` مستني U3.
- **الحجم:** M. **الاعتماد:** M5.1، وU4 (`Core/Backfill`) اللي بيتعمل هنا لو لسه.
- **المحتوى:**
  - قراية `0x01` و`0x02` في كل الأماكن اللي في [الجدول](#نقل-مستخدمي-sealer-m52)، والكتابة `0x02`، والـ backfills (واحد لكل عمود) بنفس الـ context.
  - الترتيب: قبل أول tag الخطوتين في PR واحد. بعد أول tag: PR للقراية، وrelease، وبعده PR للكتابة والـ backfill.
  - `Sources` و`Session` (TOTP) و`Core/Settings` (الإعدادات ومزوّدي الدخول) بياخدوا `*keys.Envelope` بدل `*appcrypto.Sealer`.
  - `internal/leaksweep` و`deploy/m4-demo/leak-sweep.sh`: بيفتحوا الصيغتين بالسلسلة، وبيدوّروا على الـ DEKs بعد الفك وعلى مفاتيح السلسلة، وبيفشلوا لو أي عمود `...Sealed` فيه قيمة مش عارفين يفتحوها.
  - الـ hook: `NewSealer` بره `Core/Crypto` وقايمة القراية القديمة مرفوض، ومعاه `test_crypto_hooks.py`.
  - `deploy/upgrade-test` (U3): base بقيم `0x01`، وhead بيقراها، وبعد الـ backfill الـ base لسه بيقوم (لو الخطوتين في releases منفصلة) أو بيقف بـ `78` (بعد الـ contract).
  - ADR للقرارين 6 و7.
- **قبول:**
  - كل عمود في الجدول: قيمة `0x01` بتتقري، والكتابة بتطلع `0x02`، والـ backfill بيحوّل كله ويكمّل بعد `kill -9`.
  - أكبر قيمة ممكنة في كل عمود بعد الصيغة الجديدة جوه الـ CHECK بتاعه.
  - TOTP بعد النقل: الدخول بنفس التطبيق شغّال. والـ deploy key بعد النقل: الـ checkout شغّال. وسر الـ webhook: التوقيع بيعدّي.
  - بعد الـ backfill وتغيير `DHOW_SECRET_KEY` في الاختبار: كل القيم المتخزّنة لسه بتتفتح (اللي بيقع هو المشتق بس: الـ runners والتوكنات، ومكتوب).
  - `grep` في `pg_dump` على كل القيم المزروعة وعلى الـ DEKs: مالقاش.
- **التحقق:** `go test -race -count=1 ./apps/server/internal/Modules/Sources/... ./apps/server/internal/Modules/Session/... ./apps/server/internal/Core/Settings/... ./apps/server/internal/Core/Backfill/... ./internal/leaksweep/...` و`make upgrade-test` و`make hooks-test` و`make check`.

### M5.3: البيئات: النموذج والقواعد والـ API
- **الحجم:** M. **الاعتماد:** M4.8 (خلص). بيمشي بالتوازي مع M5.1.
- **المحتوى:**
  - موديول `Modules/Environments`: الـ migrations لـ `"Environments"` و`"EnvironmentReviewers"`، والـ routes بتاعة البيئات (من غير الأسرار والموافقات)، والإجراءات الجديدة في `Core/Policy` (`environment.manage` و`environment.pause` و`environment.bypass` و`deployment.approve`)، وصفوف الـ route-table.
  - التحقق من القواعد: الـ patterns بـ `internal/pathglob`، والمراجعين أعضاء بالدور المطلوب، و`RequiredApprovals = 1`، والحدود.
  - `"RuleVersion"` بيزيد مع أي تغيير، والـ audit `environment.created` و`environment.updated` (القديم والجديد) و`environment.removed` و`environment.paused` و`environment.resumed`.
  - `Pipelines` لسه بيتخطّى بـ `ENVIRONMENTS_UNAVAILABLE` لحد M5.6.
  - ADR للقرار 8.
- **قبول:**
  - الـ route-table: كل route لكل دور، وbinding Admin على المشروع بيدّي `environment.manage` على المشروع ده بس.
  - Maintainer يقدر يوقف ومايقدرش يستكمل ولا يعدّل القواعد. وتعديل من غير `CurrentFactor`: مرفوض.
  - `Version` قديم: `ENVIRONMENT_VERSION_CONFLICT`. ومراجع مش عضو أو Viewer: `REVIEWER_NOT_ELIGIBLE`.
  - pattern فيه `..` أو أطول من الحد أو أكتر من 50: مرفوض.
- **التحقق:** `go test -race -count=1 ./apps/server/internal/Modules/Environments/... ./apps/server/internal/Core/Policy/...` و`make gen && git diff --exit-code` و`make check`.

### M5.4: الأسرار والمتغيّرات وبيانات دخول الـ registry: المخزن والـ API
- **الحجم:** M. **الاعتماد:** M5.1 وM5.3.
- **المحتوى:**
  - موديول `Modules/Secrets`: الـ migrations لـ `"Secrets"` و`"SecretVersions"` و`"SecretProjectAccess"` و`"Variables"` و`"RegistryCredentials"`، والـ routes كلها في [جدول الـ API](#الـ-api)، و`secret.read` و`secret.manage` و`org.secret.manage`.
  - الختم بـ `Core/Keys` بصاحب النطاق، والـ context `secret:<SecretId>:v<Version>` و`registry:<Id>`.
  - الإصدارات والاحتفاظ (worker بيمسح القيم بعد 30 يوم وبعد 5 إصدارات)، والمسح مع البيئة والمشروع.
  - الـ port `Core/SecretStore` (`Check` و`Resolve` بالأولوية) من غير حد بيناديه لسه.
  - السيرفر بيستخدم بيانات الـ registry في تثبيت الـ tag بالـ digest (§12.5).
  - `docs/guides/secrets.md` بالنسختين.
  - ADR للقرارات 9 لـ 13.
- **قبول:**
  - **write-only:** اختبار بيلف على كل نوع في `Gen/` للموديول ده ويتأكد إن مفيش حقل للقيمة في أي رد، وقيمة مزروعة مش في أي رد ولا log ولا audit ولا access log.
  - **الحدود:** 7 byte و32 KiB + 1 و`DHOW_X` واسم مكرر بحروف مختلفة والعدد فوق الحد: كلهم مرفوضين بالكود الصح.
  - **الأولوية:** نفس الاسم في التلات نطاقات بيرجع من `Resolve` بتاع البيئة، ومن غير البيئة بتاع المشروع، والمؤسسة لمشروع مش مختار: مش موجود.
  - **الإصدارات:** `restore-version` بيعمل إصدار جديد بنفس القيمة (بنفتحها في الاختبار)، والإصدار السادس بيمسح قيمة الأول بعد المدة.
  - **الـ registry:** tag في registry خاص بيتثبّت بالـ digest، والـ realm على host تاني مابيوصلوش الـ password.
- **التحقق:** `go test -race -count=1 ./apps/server/internal/Modules/Secrets/... ./apps/server/internal/Modules/Pipelines/...` و`make gen && git diff --exit-code` و`make docs-check` و`make check`.

### M5.5: التوصيل للـ runner، والـ forks، والـ cache
- **الحجم:** L. **الاعتماد:** M5.4.
- **المحتوى:**
  - **`internal/pipelinespec`:** `files:` في الخطوة والـ job، و`id_token:` بيترفض بـ `ID_TOKEN_NOT_SUPPORTED_YET` لحد M5.8 (زي `TRIGGER_NOT_SUPPORTED_YET`)، وشيل الـ warnings `SECRETS_UNAVAILABLE` و`ENVIRONMENTS_UNAVAILABLE`، و`vars` في السياق، و`packages/spec/dhow.schema.json` واختبار التطابق.
  - **`Pipelines`:** `Core/SecretStore.Check` قبل الإدراج، و`"FailureDetail"`، والمتغيّرات، والـ PR والعلامة.
  - **`App`:** الـ resolver الحقيقي بدل `jobs.NoSecrets` (جنب `checkoutSecrets`)، وفحص `Start` كله، و`job.secrets_delivered`. و`SecretRequest` بيزيد عليه `OriginKind` و`OriginID` (العقد بتاع ADR-0045، والتغيير بيعدّي على الاتنين).
  - **الـ runner:** `registry_auth` والـ authfile، وفحص الـ artifacts والـ cache، والـ feature `registry_auth`.
  - **الـ cache:** `"Projects"."ForkCacheRestore"` و`"CacheEntries"."HadSecrets"`، و`CacheService.Lookup` بيطبّق القاعدة.
  - **فحص التسريب:** E2E بقيمة مميزة في كل نطاق، وjob بيطبعها بكل الصيغ، ويكتبها في output وartifact وcache وملف.
  - `docs/pipeline-spec.md` بالنسختين.
  - ADR للقرارات 14 لـ 16، وقرار Sabry 4.
- **قبول:**
  - **التوصيل:** سر المشروع بيوصل كـ env وكملف للخطوة اللي طلبته بس، والإضافة بتاخد سر `with:` بتاعها بس (ADR-0075 زي ما هو).
  - **`Start`:** run اتعلّم fork بعد الإدراج (في الاختبار)، أو `RunJob` مش `active`: مفيش قيمة اتفكّت، والـ resolver مانداش `Open` خالص (عدّاد في الـ fake).
  - **الـ PRs:** سر من غير العلامة: `SECRET_NOT_AVAILABLE_TO_PULL_REQUESTS`.
  - **التسريب:** القيمة متقنّعة في `"LogChunks"`، ومش في `"RunJobs"."Outputs"` ولا الـ audit ولا logs السيرفر والـ runner ولا `pg_dump` ولا تخزين Podman. والـ artifact: `ARTIFACT_CONTAINS_SECRET`. والـ cache: مااتحفظش.
  - **الـ cache:** fork مابيرجّعش حاجة افتراضيًا، وبالإعداد بيرجّع الفرع الافتراضي بس، وعمره ما بيرجّع `"HadSecrets"`.
  - **الـ registry:** صورة خاصة بتتسحب، والـ authfile مش موجود بعد السحب ولا جوه الـ sandbox (probe).
- **التحقق:**
  - `go test -race -count=1 ./internal/pipelinespec/... ./internal/jobspec/... ./apps/server/internal/Modules/Pipelines/... ./apps/server/internal/Modules/Artifacts/... ./apps/server/internal/App/...`
  - `go test -race -count=1 -tags dhowtest ./apps/runner/...` و`make runner-isolation` (الـ probes الجديدة).
  - `make proto-lint` و`make check`.

### M5.6: بوابة البيئة والموافقات والتزامن وسجل النشر
- **الحجم:** L. **الاعتماد:** M5.3 وM5.5.
- **المحتوى:**
  - الـ migrations لـ `"DeploymentApprovals"` و`"Deployments"` وأعمدة `"RunJobs"`.
  - الـ port `Core/Deployments` (`Gate` و`Released`) بينفّذه `Environments`، و`Pipelines` بيناديه جوه نفس الـ transaction بالترتيب اللي في [البوابة](#البوابة-ترتيب-الفحوص)، والـ `if` بيتنقل بعد البيئة والمتغيّرات.
  - الموافقة والرفض والتخطّي بـ `CurrentFactor`، والانتهاء، والاستبدال، و`"RuleVersion"`، والفحص تاني في `Start` (`ENVIRONMENT_RULE_CHANGED`).
  - قفل البيئة وترتيب الأقفال، والـ reconciler بيصلّحه.
  - أسرار البيئة ومتغيّراتها بتوصل (الـ resolver بيقرا `"RunJobs"."EnvironmentId"`).
  - `GET /organizations/{id}/deployment-approvals` والعدّاد في `GET /organizations/{id}/overview` (ADR-0061، جزء بصلاحيته).
  - الـ audit: `deployment.approval_requested` و`deployment.approved` و`deployment.rejected` و`deployment.approval_expired` و`environment.bypassed` و`deployment.started` و`deployment.finished`.
  - `docs/guides/environments.md` و`pipeline-spec.md` §9 بالنسختين.
  - ADR للقرارات 17 لـ 20، وقراري Sabry 2 و5 (الإيقاف اليدوي، وإعداد `PreventSelfReview` اللي Admin بس يقفله، والقبول بيختبر الاتنين).
- **ملاحظات ملزمة من مراجعة M5.3 الأمنية:**
  - الموافقة مربوطة بـ `"Id"` البيئة ومعاه `"RuleVersion"`، عمرها ما بتتربط بالاسم. بيئة اتمسحت واتعملت تاني بنفس الاسم بتبدأ من الأول.
  - وقت الموافقة بيتفحص تاني دور كل موافق وعضويته الحالية وأدوار أعضاء الفريق. صفوف `"EnvironmentReviewers"` المتخزّنة عمرها ما بتتصدّق لوحدها.
  - المطابقة بنوع الـ ref (`branch` أو `tag`) ومعاه الـ pattern، مش بالـ pattern بس.
  - «نفسه» = اللي شغّل التنفيذ، وكمان صاحب التوكن اللي شغّله.
- **قبول:**
  - **قرار Sabry:** push على `feature/x` لبيئة `main` بس: `skipped` بـ `ENVIRONMENT_REF_NOT_ALLOWED`، والـ run مابيفشلش بسببه، ومفيش سر اتفك.
  - **الموافقة:** push على `main`: `"Gate" = 'approval'`. اللي شغّل بيوافق: `SELF_REVIEW_NOT_ALLOWED`. مراجع من غير خطوة تانية: `REVIEWER_NEEDS_SECOND_FACTOR`. مراجع بالخطوة: الـ job بيتدرج بأسرار البيئة، وصف النشر `succeeded`.
  - **الانتهاء والاستبدال والرفض:** بساعة وهمية `APPROVAL_EXPIRED`، وpush أحدث على نفس الـ ref بيعمل `SUPERSEDED` للـ `pending` بس، والرفض بيفشّل الـ job ويتخطّى اللي بعده.
  - **القاعدة اتغيّرت بعد الموافقة وقبل `Start`:** `ENVIRONMENT_RULE_CHANGED` ومفيش سر اتفك.
  - **التزامن:** runين على `production`: التاني `environment_busy` لحد ما الأول يخلص. وjobين في نفس الـ run على نفس البيئة: موافقة واحدة ومفيش run تاني بينهم.
  - **التخطّي:** من غير `AllowAdminBypass`: `BYPASS_NOT_ALLOWED`. وبيه: بيعدّي المراجعين والمؤقت، ومابيعدّيش قاعدة الـ ref ولا الإيقاف.
  - **السباقات:** 50 موافقة على نفس الطلب بالتوازي: واحدة بس بتنجح، والـ job بيتدرج مرة واحدة (`-count=20`).
  - **الرجوع:** binary M4 على القاعدة دي بيعلّم jobs البيئة `skipped` ومابيدرجهاش (U3).
- **التحقق:**
  - `go test -race -count=1 ./apps/server/internal/Modules/Environments/... ./apps/server/internal/Modules/Pipelines/...`
  - `go test -race -count=20 -run 'Race|Once|Lock' ./apps/server/internal/Modules/Environments/Test/Edge/...`
  - `make gen && git diff --exit-code` و`make upgrade-test` و`make check`.

### M5.7: الواجهة
- **الحجم:** L، في PRين: **(أ)** صفحات الأسرار والمتغيّرات والـ registry والبيئات بعد M5.4 وM5.3، و**(ب)** الموافقات وصفحة التنفيذ والعدّاد بعد M5.6.
- **المحتوى:** كل اللي في [الواجهة](#الواجهة)، والمفاتيح في `Locales/Workspace/{Ar,En}.ts`، والمصطلحات في `Plan/Glossary.md`، و`pnpm gen:api` بعد كل تغيير في الـ OpenAPI.
- **قبول:**
  - lint وtypecheck والاختبارات والـ build والـ size في الميزانية (JS الأولاني 128 KB gzip وكل chunk كسول 60 KB وCSS أقل من 30 KB، [ADR-0103](ADR/0103-web-size-budgets.md)).
  - **الـ write-only في الواجهة:** القيمة مش في أي state بعد الحفظ، ولا في الـ URL ولا الـ history ولا `localStorage` ولا `sessionStorage` (اختبار بعد الحفظ بيدوّر على القيمة في كل دول وفي الـ DOM).
  - Playwright على سيرفر حقيقي وForgejo: سر في البيئة، وpush على فرع مش مسموح يبان «اتخطّى» بالسبب، وpush على `main` يبان «مستني موافقة»، ومستخدم تاني يوافق بـ TOTP والـ job يخلص. بالعربي والإنجليزي وموبايل 390px، ومفيش console error ولا مخالفة CSP ولا `<style>` اتحقن.
  - `App/Requests.test.tsx`: مفيش طلب متكرر في الصفحات الجديدة.
- **التحقق:** `pnpm -C apps/web lint && pnpm -C apps/web typecheck && pnpm -C apps/web test && pnpm -C apps/web build && pnpm -C apps/web size`، و`make web` و`make check`.

### M5.8: مُصدِر OIDC للأحمال (متأجّلة لبعد أول نسخة)
- **الحالة:** مصمّمة ومتأجّلة ([قرار Sabry 3](#قرارات-sabry-2026-11-03)). مش من شرط خلاص M5، ومش شرط لـ M5.9، وبتتعمل بعد أول نسخة production مباشرة. وقتها بتعدّي مراجعة أمنية لوحدها، وبتضيف صفوفها لجدول التهديدات وفحص التسريب.
- **الحجم:** M. **الاعتماد:** M5.6 (claims البيئة)، والـ runner بعد M5.5.
- **المحتوى:**
  - **spike أول المرحلة (نص يوم):** حجم مفتاح RS256 اللي AWS وGCP وAzure بيقبلوه من الوثائق الرسمية، والنتيجة في الـ ADR. والاختبار الحقيقي في الـ CI على OpenBao في حاوية (ببلاش)، والسحابة يدوي واختياري.
  - موديول `Modules/WorkloadIdentity`: `"OidcSigningKeys"` والتدوير، و`.well-known` بـ `policy.Public` وقالب nginx في `Core/Installer`.
  - `IdentityService.MintIdToken`، والـ helper `/run/dhow/bin/id-token` وframe الـ agent، والـ feature `id_token`، والـ masker.
  - `DHOW_OIDC_ISSUER`، و`doctor` (https وhost عام وفرق الساعة).
  - `docs/guides/oidc-workloads.md` بالنسختين، و`architecture.md` §7.
  - ADR للقرار 21.
- **قبول:**
  - OpenBao بيقبل التوكن بـ role مربوط بـ `sub` البيئة، وبيرفضه بـ `aud` تاني أو بعد `exp` أو من مشروع تاني.
  - fork، وjob من غير `id_token:`، و`aud` مش معلن، وlease خلص، وfencing قديم: مفيش توكن.
  - التدوير: المفتاح الجاي في الـ JWKS قبلها بـ 7 أيام، والتوكنات القديمة بتتحقق لحد ما القديم يتشال (ساعة وهمية).
  - التوكن متقنّع في السجل، ومش في `pg_dump` ولا logs السيرفر.
- **التحقق:** `go test -race -count=1 ./apps/server/internal/Modules/... -run 'Oidc|IdToken'` و`go test -race -count=1 -tags dhowtest ./apps/runner/...` و`go test -run '^$' -fuzz FuzzIdTokenRequest -fuzztime 60s` و`make proto-lint` و`make check`.

### M5.9: التقوية وإثبات شرط الخلاص
- **الحجم:** M. **الاعتماد:** كل اللي فات.
- **المحتوى:**
  - **الـ contract لصيغة `0x01`** لو لسه مفيش tag (`-- dhow:floor 0.0.0`)، أو في أول minor بعد release فيه M5.2 كامل.
  - **fuzz targets جديدة** في `FUZZ_TARGETS` و`fuzz-nightly.yml`: `FuzzKeyring` و`FuzzOpen` و`FuzzAllowedRefs` و`FuzzSecretName` (و`FuzzIdTokenRequest` بييجي مع M5.8).
  - **`deploy/m5-demo/run.sh`** (فوق M4 demo): بيئة `production` بـ `main` بس ومراجع تاني بـ TOTP، وأسرار في التلات نطاقات، وpush على فرع تاني (متخطّي)، وpush على `main` (مستني، والموافقة على النفس مرفوضة، والمراجع بيوافق، والنشر بالسر)، وتدوير KEK في النص (`generate` وإعادة تشغيل و`activate` وإعادة اللف) وتنفيذ تاني، وفحص التسريب.
  - **فحص التسريب** بيدوّر كمان على: قيم الأسرار بكل نطاق، ومفاتيح السلسلة، والـ DEKs بعد الفك، وكلمة سر الـ registry. ومفتاح OIDC الخاص والتوكنات بشكل JWT بيتضافوا مع M5.8.
  - **مراجعة التهديدات:** `docs/security-model.md` §13 بالنسختين (الصفوف اللي فوق بالاختبار بالمسار، والمخاطر المقبولة: التسريب بتحويل القيمة، والـ pusher في Git العام، والملفات المضغوطة جوه الـ artifacts)، وإضافات لقايمة المراجعة الخارجية.
  - **مراجعة أمنية مستقلة** وإصلاح الـ Critical والـ High.
  - `docs/roadmap.md` بالنسختين: M5 خلص.
- **قبول (شرط خلاص M5، من غير OIDC):**
  - `deploy/m5-demo/run.sh` عدّى على `ubuntu-26.04` في `demo.yml`، وعلى VM نضيفة بـ Debian 13 وUbuntu 26.04.
  - فحص التسريب لقى الحالات المزروعة ومالقاش أي حاجة غيرها.
  - مفيش finding مفتوح Critical أو High.
- **التحقق:** `make check` و`make runner-isolation` و`make runner-chaos` و`make upgrade-test` و`deploy/m5-demo/run.sh`.

### الترتيب
```
U2 ──► M5.1 ─┬─► M5.2 (+U4)
             │
M4.8 ─► M5.3 ┴─► M5.4 ─┬─► M5.5 ─► M5.6 ─┬─► M5.7 (ب)
                       └─► M5.7 (أ)      └─► M5.8 (بعد أول نسخة)
                                                   M5.9 في الآخر
```
- M5.1 وM5.3 بيبدأوا مع بعض.
- M5.2 مستقل عن الأسرار، فيمشي بالتوازي مع M5.4 وM5.5.
- M5.7 (أ) بيبدأ أول ما M5.4 يخلص، و(ب) بعد M5.6.

### الترتيب مع الشغل الباقي
- **U2 قبل M5.1** (خطة الترقيات بتقول كده أصلًا): التفعيل محتاج الـ heartbeat، والـ contract محتاج الـ floor.
- **U4 جوه M5.2** لو لسه ماتعملش، لأن M5.2 أول تغيير محتاج backfill كبير.
- **U3** بيزيد حالتين: base بقيم `0x01` وhead بيقراها، وbinary M4 على قاعدة فيها بيئات بيقفل.
- **U5 (M9):** بيتعدّل في M5.1 (السلسلة في نسخة Compose، والاسترجاع بيقارن الـ check values)، والتنفيذ في M9 زي ما هو.
- **U9 (الـ updater):** الـ preflight بيتأكد إن السلسلة موجودة ومقرية وإن الـ check values متطابقة قبل أي swap، وإلا `KEYRING_NOT_READY`. والـ `Actions[]` في الـ feed بيقول «اعمل السلسلة» لأي ترقية من نسخة قبل M5 (مفيش release قبل M5 غير لو أول نسخة تجريبية طلعت قبله).
- **M4.5 وM4.6 (مستنيين حسابات الاختبار):**
  - لو اتعملوا **قبل M5.2:** بيستخدموا `Sealer` بـ label لكل نوع (القرار 12 في M4)، وM5.2 بينقلهم مع الباقي (أعمدتهم في الجدول).
  - لو اتعملوا **بعد M5.2:** لازم `Core/Keys` (الـ hook بيرفض `NewSealer`): مفتاح الـ App وclient secret وسر الـ webhook بصاحب `instance`، وتطبيق GitLab OAuth بصاحب `instance`، وأي توكن بصاحب `organization`.
  - التوكن القصير بتاع الـ checkout (GitHub) مابيتخزّنش أصلًا، فمش محتاج حاجة.
- **M6 بعد M5:** أهداف النشر بتقرا عنوان السيرفر من متغيّرات البيئة ومفتاحه من أسرارها (ملف)، والـ host key المثبّت للهدف في M6.

## الاختبارات: إيه بيشتغل فين
| الاختبار | كل PR | ليلي أو label | قبل كل release |
|---|---|---|---|
| `Core/Keys` (الشكل، والإقلاع، والتدوير بالقتل في النص) وfuzz قصير | ✅ | — | — |
| نقل `Sealer` والـ backfills على Postgres 18 | ✅ | — | — |
| `Secrets` و`Environments` و`Pipelines` (route-table، والـ write-only، والبوابة، والسباقات) | ✅ | — | — |
| فحص الـ resolver وقت `Start` (fork، وبيئة، وقاعدة اتغيّرت) | ✅ | — | — |
| الـ runner: `files:` والـ authfile وفحص الـ artifacts والـ cache، والـ probes على Podman rootless (`ubuntu-26.04`) | ✅ لو الـ PR بيلمس `apps/runner` أو `packages/proto` | ✅ | ✅ VM بـ Debian 13 وUbuntu 26.04 |
| `make upgrade-test` (`0x01` وbinary M4 على قاعدة M5) | ✅ لو الـ PR بيلمس `Database/Migrations` أو `apps/server` | ✅ | ✅ |
| Playwright (الأسرار والبيئات والموافقة) على سيرفر حقيقي وForgejo | ✅ لو الـ PR بيلمس `apps/web` | — | ✅ |
| `deploy/m5-demo/run.sh` بفحص التسريب | ✅ لو الـ PR بيلمس الكود | ✅ ليلي في `demo.yml` | ✅ |
| OIDC مع OpenBao في حاوية بالـ digest (من M5.8، بعد أول نسخة) | ✅ لو الـ PR بيلمس OIDC | ✅ | ✅ |
| OIDC مع AWS أو GCP أو Azure (M5.8) | — | — | يدوي واختياري (مش شرط، قرار Sabry «ببلاش») |
| fuzz طويل (السلسلة والصيغة والـ refs والأسماء، وطلب التوكن مع M5.8) | — | ✅ ليلي (30 دقيقة لكل target) | — |
| الفوضى: قتل السيرفر وقت إعادة اللف ووقت فتح البوابة | — | ✅ ليلي وعلى label `chaos` | ✅ |

## المخاطر
| الخطر | التخفيف |
|---|---|
| ضياع ملف السلسلة = ضياع كل الأسرار | الـ installer و`init.sh` بيعملوه ويقولوا «صدّره»، و`keys export` بـ `age`، والنسخة الاحتياطية فيها السلسلة، و`doctor` بيحذّر لو مفيش تصدير، ودليل طوارئ للاسترجاع من غيرها |
| تغيير صيغة القيم بيكسر الرجوع | قراية الصيغتين قبل الكتابة، وU3 بيختبر، والـ hook بيمنع `NewSealer` جديد |
| احتكاك الموافقات للفرق الصغيرة (شخص واحد) | المراجعين اختيارية، وقاعدة الـ ref لوحدها بتحمي، وAdmin يقدر يقفل منع الموافقة على النفس للبيئة دي بس (قرار Sabry 5)، والتخطّي اختيار |
| الـ masker مابيمسكش كل تحويل | الحماية الحقيقية قاعدة الـ ref والمراجعة، وفحص الـ artifacts، ومكتوب كخطر مقبول |
| فحص الـ artifacts الكبيرة بياخد CPU | Aho-Corasick على الـ stream مرة واحدة، و`make bench` بـ artifact 1 GiB قبل الدمج، والحد في الوثائق |
| النطاق بيكبر (المتغيّرات والـ registry وOIDC) | كل واحد في مرحلة مستقلة، وOIDC اتأجّل لبعد أول نسخة (قرار Sabry 3) |
| الحد بين Community وBusiness في قواعد الحماية | قرار Sabry 2: الجدول زي ما هو، والنموذج جاهز للتوسيع بـ expand في M8 |
| «نفسه» مش معروف في Git العام | مكتوب جنب الطلب في الواجهة، والفرق بتستخدم التشغيل اليدوي أو مراجع تاني |
| أكتر من instance والتفعيل | محتاج heartbeat U2، و`KEK_NOT_EVERYWHERE` قبل أي تغيير |
| أرقام الـ migrations بتتخبط بين المراحل والـ U | الرقم وقت الـ merge، و`Compat_test.go` |
| ميزانية الـ JS في الواجهة | الصفحات lazy، ومن غير مكتبات جديدة |

## الاعتماديات المتوقعة (بنسخ مثبّتة)
اتأكدت يوم 2026-11-03 من `Plan/Upgrades-and-Help.md` (اللي اتراجع على proxy.golang.org وOSV يوم 2026-11-02)، وبتتراجع تاني في أول المرحلة اللي بتضيفها.

| الاعتمادية | النسخة | تاريخها | ليه |
|---|---|---|---|
| Go standard library (`crypto/aes` و`crypto/cipher` و`crypto/hkdf` و`crypto/hpke` و`crypto/rsa` و`crypto/sha256`) | Go 1.27.3 (`go.mod`) | — | الختم والـ HPKE وتوقيع RS256، من غير مكتبات |
| `filippo.io/age` | v1.3.2 | 2026-08-29 | `keys export` بمفتاح عام لملف السلسلة (قرار Sabry 1). نفس نسخة U5 بالظبط، وبتتضاف هنا لو U5 لسه |
| `go.yaml.in/yaml/v3` و`github.com/google/cel-go` | الموجودين | — | `files:` و`vars` في الـ spec |
| صورة OpenBao (CI، مع M5.8 بعد أول نسخة) | بالـ digest وقت التنفيذ | — | اختبار OIDC الحقيقي، open source (MPL) وببلاش، بدل Vault (BUSL) |
| صورة Forgejo (CI) | الموجودة من M4 | — | الـ demo وPlaywright |

**مش هنضيف:**
- مكتبة JWT أو JOSE (`go-jose` و`golang-jwt` موجودين indirect بس): RS256 والـ JWKS من الـ standard library، زي القرار 16 في M4.
- SDKs بتاعة Vault أو AWS أو GCP أو Azure KMS: في `ee/` مع M8.
- مكتبة glob: `internal/pathglob` بتاع M4.
- مكتبة Aho-Corasick: الـ masker بتاعنا من M3.

## قرارات Sabry (2026-11-03)
Sabry اختار التوصية في الأسئلة الستة، والخطة كلها اتعدّلت عليها:
1. **مكان مفتاح التغليف (KEK):** ملف سلسلة مفاتيح منفصل (`dhow-keyring` بإصدارات، systemd credential أو Compose secret أو `DHOW_KEYRING_FILE`)، مش مشتق من `DHOW_SECRET_KEY`. وKMS خارجي (Vault Transit وKMS السحابة وHSM) اختياري بعدين ورا نفس الـ interface `KeyProvider` في `ee/` مع M8، وعمره ما بيبقى شرط. السبب: تدوير حقيقي من غير توقف، ومنفصل عن متغيّر بيئة ممكن يبان، ومفيش release لسه فمفيش ترقية هتتعب.
2. **نوافذ النشر والتجميد وأكتر من مراجع:** زي جدول النسخ في `docs/editions-and-licensing.md` §3. النوافذ والتجميد المجدول وأكتر من مراجع في Business مع M8 بعد أول نسخة. Community في M5 فيها مراجع واحد (من قايمة مراجعين)، وزرار «أوقف النشر» يدوي بسبب، لأن وقف النشر وقت حادثة أمان أساسي. والنموذج جاهز للباقي بـ expand.
3. **مُصدِر OIDC للأحمال:** بعد أول نسخة مباشرة. M5.8 بتفضل مصمّمة هنا ومتأجّلة، ومش من شرط خلاص M5. و`docs/roadmap.md` (بالنسختين) نقله لـ «بعد أول نسخة». وفي أول M5.8 فيه ADR صغير بيعدّل ADR-0068 في النقطة دي بس.
4. **الـ cache لتنفيذات الـ forks:** افتراضيًا الـ fork مابيرجّعش أي cache خالص. Admin يقدر يفتح cache الفرع الافتراضي للمشروع (`"ForkCacheRestore"` = `default_branch`) بتحذير. والـ cache اللي اتكتب من job وصله سر أو بيانات registry (أو توكن OIDC بعدين) عمره ما بيوصل لـ fork حتى لو الإعداد مفتوح.
5. **الموافقة:** كل موافقة (ورفض وتخطّي) محتاجة الخطوة التانية (`CurrentFactor`)، والمراجع لازم عنده خطوة تانية مسجّلة. ومنع الموافقة على النفس شغّال افتراضيًا، وAdmin يقدر يقفله لبيئة معيّنة بس (للفرق اللي فيها شخص واحد)، بخطوة تانية، والتغيير بيتسجّل في الـ audit وبيبان في صفحة البيئة.
6. **توكنات API:** في M5 كتابة الأسرار والمتغيّرات والبيئات والموافقة من جلسة بس. التوكنات بتاخد `secret.read` بس: الأسماء والـ metadata، عمرها ما بتقرا قيمة. والكتابة بتوكن (`secret.write`) بعدين بـ ADR لو حد طلبها.

## قرارات تانية اتاخدت بالتوصية (Sabry يقدر يرجع فيها)
1. **الأدوار:** الجدول اللي فوق. Maintainer بيدير أسرار المشروع ومتغيّراته، وAdmin بس بيدير البيئات وأسرارها وأسرار المؤسسة.
2. **الإيقاف والاستكمال:** Maintainer يوقف، وAdmin يستكمل بخطوة تانية.
3. **مدة الطلب:** 72 ساعة افتراضيًا، والبيئة تختار من ساعة لـ 30 يوم.
4. **الرفض:** الـ job بيبقى `failed` بـ `DEPLOYMENT_REJECTED` (زي GitHub)، مش `canceled`.
5. **الإيميل للمراجعين:** مع إشعارات M7. في M5 العدّاد والصفحة بس.
6. **حالة السلسلة في الواجهة:** للقراية بس، والتدوير من الـ CLI لأنه محتاج الملف.
7. **أسرار المؤسسة والمشروع للـ PRs من نفس المستودع:** مقفولة افتراضيًا، والسر نفسه يتفتح بعلامة.
8. **المتغيّرات للـ forks:** مفيش، زي الأسرار.
9. **البيئة اللي مش موجودة:** `failed` مش `skipped`، لأن ده غلط في الإعداد مش قاعدة.
