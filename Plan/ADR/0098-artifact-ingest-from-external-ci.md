# ADR-0098: استقبال الملفات الناتجة من CI خارجي (M6.6)

- **الحالة:** مقبول. [خطة M6](../M6-Deploy-Targets.md) §M6.6، القرارات 15 و16 و17 في جدول القرارات المفوَّضة، وقرار Sabry 4 (توكن استقبال محدود).
- **التاريخ:** 2026-11-06
- **بيكمّل:** [ADR-0010](0010-build-or-ingest-artifacts.md) (البناء أو الاستقبال ينتجان artifact معنوَن بمحتواه)، و[ADR-0078](0078-artifacts-cache-and-blob-store.md) (الـ blob store والاحتفاظ)، و[ADR-0097](0097-manual-group-rollback-and-redeploy.md) (الحفظ المحسوب للرجوع)، و[ADR-0076](0076-pipeline-runs-definitions-and-forks.md) (التعريف من مرجع موثوق).

## السياق
- فرق كثيرة تبني في CI آخر (GitHub Actions أو غيره) وتريد من داو أن ينشر ما بُني بالـ digest نفسه، من غير إعادة بناء.
- الرفع يحمل ملفًا كبيرًا من طرف غير موثوق بتوكن قابل للسرقة، فلا بد من حدود على ما يقدر التوكن أن يفعله وعلى ما يُقبل من الأرشيف.

## القرار

### 1. موديول منفصل وتوكنات محدودة (القرار 15)
- موديول `Modules/Ingest` له جداول `"IngestTokens"` و`"IngestedArtifacts"`. الهجرة `00076` (expand، والقيد الجديد على `"Trigger"` بـ `NOT VALID`) و`00077` (التحقق في ملف `NO TRANSACTION`).
- التوكن `dhow_ing_` ومعه 43 حرفًا عشوائيًا (256 بت). يُعرض مرة واحدة في رد الإنشاء، ويُخزَّن `sha256` فقط. أول 8 حروف من الجزء العشوائي تُحفظ للتمييز بين التوكنات. لا يدخل التوكن في أي log أو حدث audit.
- التوكن مربوط بمشروع واحد، وله من 1 إلى 20 نمط ref (`branch` أو `tag` مع نمط `pathglob`)، ومدة 90 يومًا افتراضيًا وأقصاها 365. حد أقصى 50 توكنًا فعّالًا للمشروع.
- الإنشاء والإلغاء بالإجراء الجديد `ingest.manage` (Owner وAdmin وMaintainer، حساس: يحتاج `CurrentFactor`) ومن جلسة فقط. الإجراء ليس من إجراءات توكنات الـ API، فلا يقدر توكن API أن يصنع توكن استقبال. وقراءة قائمة الملفات المستقبلة بـ `artifact.read`.
- الرفع بره `/api/v1`: `PUT /ingest/v1/artifacts/{sha256}` وحالته بـ `GET` على نفس المسار. قاعدته `policy.Ingest` (نطاق `ScopeIngest`) ومكانها `IngestPolicies()`، ولها اختبار route-table يثبت أن الجلسة وتوكن الـ API وكل الأدوار تُرفض بـ `INGEST_TOKEN_INVALID`.
- ترتيب الفحوص قبل قراءة أول بايت: حد الطلبات للشبكة (120 في الدقيقة)، ثم التوكن (مبطل، منتهٍ، مشروع مؤرشف)، ثم حد التوكن (30 رفعًا في الساعة)، ثم الـ digest في المسار، ثم الترويسات (`ParseHeaders`، وعليها `FuzzIngestHeaders`)، ثم أنماط الـ ref، ثم `Content-Length` (411) والحد `DHOW_INGEST_MAX_BYTES` (413)، ثم الرفع المتكرر، ثم الحصة (`ARTIFACT_QUOTA_EXCEEDED`)، ثم 4 رفعات متزامنة على الـ instance (وإلا 503 مع `Retry-After`).
- أثناء الاستقبال: الكتابة في `Core/Blob` (الـ digest يُحسب أثناء الكتابة ويُقارن بالمسار، وإلا `DIGEST_MISMATCH`)، وبالتوازي `archive.Walk` على نفس الستريم بقواعد الفك الآمن نفسها التي يستخدمها الـ runner (وإلا `ARCHIVE_UNSAFE`). ما فشل لا يُحفظ منه شيء: لا صف ولا ملف. التسجيل وربط الـ blob وإنشاء الـ run في transaction واحدة، والـ `rename` للملف آخر خطوة فيها.
- **التكرار** (نفس المشروع والـ digest والاسم والـ ref والـ sha) يرجع `200` بـ `Created = false` وبنفس الـ run، من غير قراءة الجسم ومن غير run جديد. اخترنا هذا على رفض بكود لأن الخطة تقول «الرد نفسه»، ولأن إعادة رفع بعد انقطاع الشبكة يجب أن تنجح.
- الـ audit: `ingest_token.created` و`ingest_token.revoked` و`artifact.ingested` و`ingest.rejected` (مرة كل دقيقة لكل توكن). الحقول من قائمة مسموحة، ولا توكن ولا محتوى. و`dhow_ing_` من علامات الاعتماد التي يحذفها الـ audit، ويبحث عنها فحص التسريب.
- الـ sha والمنشأ **مُعلَنان وغير متحقَّق منهما**. `"ProvenanceVerified"` يبقى `false` لحين دعم توكن GitHub OIDC.

### 2. `on.artifact` و`ingested:` والتعريف من الفرع الافتراضي (القرار 16)
- `on.artifact: {names: [...], refs: [...]}` (الأسماء من 1 إلى 16، والـ refs أنماط اختيارية)، و`jobs.<id>.ingested: [name]`. الأكواد: `ARTIFACT_TRIGGER_INVALID` و`INGESTED_NOT_DECLARED` (يشمل `ingested:` من غير `on.artifact`) و`INGESTED_INVALID` (اسم سيئ أو مكرر أو ينتجه job آخر في الملف). `TRIGGER_NOT_SUPPORTED_YET` بقي لـ `schedule` فقط. الـ digest للـ job يتغير فقط لو فيه `ingested` (الحقل `omitempty`)، فلا تتأثر runs جارية.
- الـ run: `"Trigger" = 'artifact'`، و`Ref` و`Sha` المعلنان، لكن التعريف يُقرأ من رأس الفرع الافتراضي (آخر `push` موثوق عليه، ويوضع في `BaseRef` و`DefinitionSha`)، فصاحب توكن CI لا يكتب pipeline. وإن لم يُعرف الرأس: يُحفظ الملف والرد `Triggered = false` بالسبب `DEFAULT_BRANCH_UNKNOWN`. ومثله `REPOSITORY_NOT_LINKED` و`PROJECT_ARCHIVED`.
- لو آخر تعريف معروف للفرع الافتراضي لا يعلن `on.artifact` مطابقًا: `NO_ARTIFACT_TRIGGER` ولا run. وإن لم يُعرف تعريف بعد، يُنشأ run ويُتجاوز بـ `TRIGGER_NOT_ENABLED` (انحراف صغير عن الخطة، لأن قراءة التعريف وقت الرفع تعني اتصالًا بالمزوّد داخل transaction).
- `"RunIngestedArtifacts"` يربط (run، اسم) بالملف. عند الإدراج يصير `ArtifactInput` بـ `download_token` مربوط بالـ `RunJob` (نفس `needs`)، فالـ job ينزّل الـ digest نفسه. job يريد `ingested` لم يأت في هذا الـ run يُتجاوز بـ `INGESTED_ARTIFACT_NOT_IN_RUN` (مُرضٍ للتابعين)، وملف ذهب قبل الإدراج يفشل بـ `INGESTED_ARTIFACT_GONE`.
- run من هذا النوع لا يرسل حالة commit للمزوّد أبدًا، لأن الـ sha مُعلَن وغير متحقَّق. والبوابة تطبق `AllowedRefs` على الـ ref المُعلَن، والـ checkout يتحقق من تاريخ الـ ref (`CHECKOUT_REF_MOVED`).

### 3. التثبيت للرجوع (القرار 17)
- العمود `"PinnedUntil"` على `"IngestedArtifacts"` فقط. الـ sweeper (كل 10 دقايق) يضعه أول مرة (الآن + 90 يومًا، `IngestPinMax`) لكل ملف يشاور له إصدار `deploy` ناجح ما زال داخل آخر `keep_releases` على هدف في نفس المشروع. والانتهاء لا يحذف ملفًا إلا إذا كان `ExpiresAt` فات **وليس** (`PinnedUntil` لم يفت **و** الإصدار ما زال داخل النافذة)، فخروج الإصدار من `keep_releases` ينهي التثبيت. الشرط نفسه الذي في ADR-0097 للملفات العادية.
- مسح الـ blobs (mark-and-sweep) يعتبر digests `"IngestedArtifacts"` حيّة. و`PrepareDigest` (مصدر ملف الرجوع) يبحث في `"Artifacts"` ثم `"IngestedArtifacts"`.

## البدائل اللي اترفضت
- **توكن الـ API نفسه للرفع:** نطاقه المؤسسة ولا يقيّد الـ ref، وسرقته تعني نشرًا. توكن الاستقبال لا يقرأ ولا يكتب غير الرفع.
- **فك الأرشيف على القرص للفحص:** لا حاجة، `archive.Walk` يفحص الستريم.
- **قراءة التعريف من الـ sha المُعلَن:** يسمح لصاحب التوكن أن يكتب pipeline. 
- **تثبيت يكتبه مسار نجاح الإصدار:** يربط `Environments` بـ `Ingest` ويضيف كتابة في مسار حساس. الحساب في الـ sweeper أبسط وأمتن.

## النتايج
- حصة المؤسسة تُحسب بمجموع `"Artifacts"` و`"IngestedArtifacts"` على digests مميزة، بالاستعلام نفسه `OrganizationArtifactBytes` لرفع الـ runner ولرفع الاستقبال، فنفس البايتات تحت اسمين تُعدّ مرة واحدة.
- إلغاء التوكن لا يحرّر الحصة: الملفات تبقى حتى تنتهي مدتها. لا يوجد أمر حذف بعد، وهو متابعة لـ M6.8(ب).
- حد الـ runs: كل رفع جديد قد يبدأ run، فعدد الـ runs من الاستقبال محدود بحدود الطلبات (30 رفعًا في الساعة لكل توكن، و120 طلبًا في الدقيقة لكل شبكة، ورفع واحد متزامن لكل توكن)، وبحد الـ concurrency في الـ pipeline. لا حد إضافي.

## تقوية بعد المراجعة المستقلة
- **الرفع البطيء (M1):** مهلة مطلقة للرفع من الحجم المعلن بسرعة دنيا 256 KiB في الثانية، بحد أدنى دقيقتان وأقصى 30 دقيقة (`IngestMinRate` و`IngestDeadlineFloor` و`IngestDeadlineCap`)، وفحص سرعة دنيا في `idleReader` بعد 15 ثانية (`UPLOAD_TOO_SLOW`)، ورفع واحد متزامن لكل توكن ورفعان لكل مؤسسة (والأماكن الأربعة على الـ instance كما هي)، وتمديد مهلة الكتابة للرد بـ `http.ResponseController` فلا يقطع `WriteTimeout` رفعًا طويلًا. بقي `proxy_request_buffering off` عمدًا: الثمن أن nginx لا يعيد الطلب ولا يحمي وحده من عميل بطيء، والحماية في داو.
- **توكن يُلغى أثناء الرفع (M2):** transaction التسجيل تعيد قراءة التوكن بـ `FOR SHARE` وتفحص الإلغاء والانتهاء وأرشفة المشروع قبل الإدراج. إن فشل: `INGEST_TOKEN_REVOKED` أو `INGEST_TOKEN_EXPIRED` ولا صف ولا blob ولا run.
- **الـ ref ادعاء (M3):** الـ ref والـ sha يعلنهما الرافع، و`ProvenanceVerified` تبقى `false` حتى OIDC. وعند إنشاء التوكن نحسب `Reaches`: البيئات التي قد تتقاطع أنماط `AllowedRefs` فيها مع أنماط التوكن (فحص حذر: نمط حرفي يُطابق بـ `pathglob`، وبين نمطين فيهما رموز نقارن البادئة الحرفية قبل أول رمز). إن وصل التوكن إلى بيئة فيها أهداف نشر أو مراجعون يلزم `AcknowledgeProtectedReach: true` وإلا `INGEST_TOKEN_REACH_UNCONFIRMED`. وتظهر `Reaches` في القائمة محسوبة وقت القراءة.
- **سقف للتوكن (L1):** 10 GiB لكل توكن (`IngestTokenMaxBytes`، الكود `INGEST_TOKEN_QUOTA`) على digests مميزة.
- **التوكن المرفوض (L3):** الإلغاء والانتهاء وأرشفة المشروع تسجَّل في `ingest.rejected` (مرة في الدقيقة لكل توكن) برقم التوكن والكود فقط.
- **الترويسات (L4):** تُرفض محارف `unicode.Cf` (مثل الصفر العريض وعلامات الاتجاه) كما تُرفض محارف التحكم.
- **الإقلاع (L5):** لا يقوم الموديول من غير limiter. و`DHOW_TRUSTED_PROXIES` شرط لحدود الطلبات لكل عنوان، وهو مكتوب في الدليل بالنسختين.
- **ترتيب الأخطاء (L6):** خطأ الحجم أو القرص يسبق `ARCHIVE_UNSAFE` (`ClassifyUpload`).
- **المفتاح الأجنبي (L7):** `("IngestedArtifactId","OrganizationId")` من `"RunIngestedArtifacts"` إلى `"IngestedArtifacts"` بـ `NOT VALID` في `00076` والتحقق في `00077`، وجرى تعديل `00076` مكانه لأنه لم يُدمج بعد.
- واجهة إدارة التوكنات في M6.8(ب)، و`dhow artifact push` والـ Action في M6.7.
- التحقق من منشأ GitHub OIDC (والعمود `"ProvenanceVerified"` جاهز) بعد أول نسخة.
