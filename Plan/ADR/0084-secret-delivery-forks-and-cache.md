# ADR-0084: توصيل الأسرار للـ runner، والـ forks، والـ cache

- **الحالة:** مقبول. القرارات 14 لـ 16 في [خطة M5](../M5-Secrets-Environments.md#القرارات-المفوَّضة-كل-واحد-بيبقى-adr-في-أول-مرحلته) مفوَّضة ومكتوبة بالتوصية، و«الـ forks من غير cache افتراضيًا» [قرار Sabry 4](../M5-Secrets-Environments.md#قرارات-sabry-2026-11-03).
- **التاريخ:** 2026-11-04
- **بيكمّل:** [ADR-0045](0045-secrets-transport-and-masking.md) (`SecretResolver` والـ HPKE والـ masker)، و[ADR-0075](0075-multi-container-attempts-checkout-and-plugins.md) (أسرار الإضافات)، و[ADR-0076](0076-pipeline-runs-definitions-and-forks.md) (الـ forks)، و[ADR-0078](0078-artifacts-cache-and-blob-store.md) (الـ cache)، و[ADR-0083](0083-secrets-variables-and-registry-credentials.md) (المخزن و`Core/SecretStore`).

## السياق
- M5.4 عمل المخزن والـ API و`Core/SecretStore`، بس الـ jobs لسه بتاخد `jobs.NoSecrets` وأي job فيه سر بيقف بـ `SECRETS_UNAVAILABLE`.
- البيئات لسه بتتخطّى بـ `ENVIRONMENTS_UNAVAILABLE` لحد البوابة في M5.6، فأسرار البيئة مابتوصلش لأي job في M5.5.
- المراجعة الأمنية لـ M5.4 قالت: أسرار البيئة مايوصلوش لتنفيذ PR قبل قاعدة البيئة. وده متحقق هنا لأن مفيش job ببيئة بيتدرج أصلًا.

## القرار
1. **فحص قبل الإدراج (`Pipelines`):** قبل `Enqueue` بيتنادى `Core/SecretStore.Check` بالأسماء اللي في الـ job (أسرار الخطوات والإضافات والملفات) ونطاق التنفيذ (المؤسسة والمشروع، والـ PR):
   - اسم مش موجود: الـ `RunJob` بيفشل `SECRET_NOT_FOUND`، والأسماء في عمود جديد `"RunJobs"."FailureDetail"` (`jsonb`، 1 KiB، `{"Names": [...]}`). أسماء بس، عمرها ما بتبقى قيم.
   - سر مش للـ PRs: `SECRET_NOT_AVAILABLE_TO_PULL_REQUESTS` بنفس الشكل.
   - أكتر من 50 سر أو أكبر من 256 KiB: `SECRETS_TOO_LARGE`.
   - الـ forks زي ما هي: `FORK_PR_NO_SECRETS` قبل أي فحص.
2. **التوصيل وقت `Start` بس (`App`):** resolver حقيقي بدل `jobs.NoSecrets`، ورا الـ checkout الموجود. بيوصل من الـ job لـ `RunJob` والتنفيذ بنفسه (`Pipelines.SecretScopeOfJob`) ومش بيصدّق الـ spec:
   - الـ job لازم أصله `pipeline`، والتنفيذ مش fork، والـ `RunJob` لسه `active` والـ job ده هو الـ `CurrentJobId`. غير كده: `SECRETS_UNAVAILABLE` من غير ما أي قيمة تتفك.
   - بعدها `SecretStore.Resolve` (اللي بيرفض الـ fork تاني)، والقيم بتتختم لمفتاح الـ runner (ADR-0045 زي ما هو).
   - الـ job اليدوي (`manual`) مالوش مشروع، فمالوش أسرار: `Accepts` بيرفضه وقت الإنشاء.
   - `Jobs.SecretRequest` و`Accepts` بياخدوا `OriginKind` و`OriginID` (العقد بتاع ADR-0045 بيتعدّل في الاتنين مع بعض).
   - الـ audit: `job.secrets_delivered` بالأسماء والنطاقات وأرقام الإصدارات بس، مرة لكل attempt، في الـ transaction بتاعة `Start`.
3. **`files:` (القرار 14):** على الخطوة أو على الـ job، `files: {deploy_key: ${{ secrets.DEPLOY_KEY }}}`:
   - كل ملف `SecretRef.as_file` باسم الملف، و`step_indexes` الخطوة دي (أو كل خطوات `run` لو على الـ job، ومش الإضافات).
   - الاسم `^[A-Za-z0-9_][A-Za-z0-9._-]{0,63}$`، و20 ملف بالكتير لكل job، والقيمة لازم `${{ secrets.NAME }}` كاملة.
   - الـ runner بيكتبها في `/run/dhow/secrets/<name>` بـ `0400` زي M3، و`DHOW_SECRETS_DIR` بيشاور على المجلد.
4. **`vars.*`:** `Pipelines` بيملا السياق من `Core/SecretStore.Variables` (المؤسسة ثم المشروع، والبيئة في M5.6) قبل `if` والـ `env`. الـ forks بتاخد سياق فاضي.
5. **الـ warnings:** `SECRETS_UNAVAILABLE` و`ENVIRONMENTS_UNAVAILABLE` مابقوش warnings في الـ validator، والأكواد بتفضل محجوزة. و`id_token:` بيترفض بـ `ID_TOKEN_NOT_SUPPORTED_YET` لحد M5.8.
6. **فحص الـ artifacts والـ cache (القرار 16):** الـ runner بيعدّي كل ملف قبل الأرشفة على نفس Aho-Corasick بقيم الـ job بكل صيغها:
   - artifact فيه قيمة: الـ attempt بيفشل `ARTIFACT_CONTAINS_SECRET` (إضافة للـ proto)، والرسالة فيها اسم الـ artifact والملف بس.
   - cache فيه قيمة: مابيتحفظش، وسطر تحذير في السجل.
   - الفحص على الملفات زي ما هي، فالملف المضغوط أو المتشفّر جوه الـ artifact مش متغطي، ومكتوب في الوثائق.
7. **بيانات دخول الـ registry (`registry_auth`):** `JobSpec.registry_auth[] {host, secret_name}` و`RunnerFeature.REGISTRY_AUTH`:
   - السيرفر بيضيفه وقت الإدراج لصور الـ job اللي ليها حساب في المشروع أو المؤسسة، ومش للـ forks أبدًا.
   - القيمة بتوصل في نفس الـ bundle باسم محجوز `dhow.registry.<n>` (اسم المستخدم وكلمة السر)، ومش بتعدّي على `Check` لأنها مش اسم في الـ spec.
   - الـ runner بيكتب authfile مؤقت في مجلد حالته الخاص (`0600`، بره أي sandbox)، ويسحب بـ `--authfile`، ويمسحه بعد السحب، وكلمة السر في الـ masker.
   - runner من غير الـ feature: الـ job بيستنى `NO_MATCHING_RUNNER` ظاهر (زي M4).
8. **الـ cache والـ forks (قرار Sabry 4):**
   - `"Projects"."ForkCacheRestore"` (`none` افتراضيًا أو `default_branch`)، بيتغيّر من تعديل المشروع بـ Admin بس.
   - `"CacheEntries"."HadSecrets"` بيتعلّم لما الـ job اللي كتب الـ cache وصله أي سر أو `registry_auth`.
   - تنفيذ fork: مابيرجّعش أي cache مع `none`، ومع `default_branch` بيرجّع نطاق الفرع الافتراضي بس، وعمره ما بيرجّع `"HadSecrets"`.
9. **الـ migrations** كلها expand: `"RunJobs"."FailureDetail"` (`jsonb NOT NULL DEFAULT '{}'`)، و`"Projects"."ForkCacheRestore"` (`TEXT NOT NULL DEFAULT 'none'`)، و`"CacheEntries"."HadSecrets"` (`boolean NOT NULL DEFAULT false`)، والـ CHECKs بـ `NOT VALID` وبعدها `VALIDATE`.

## البدائل اللي اترفضت
- **الفحص في `Start` بس:** السر الناقص كان هيبان بعد ما الـ job ياخد runner ويستنى في الطابور، بدل ما يبان أول ما التنفيذ يبدأ.
- **السيرفر بيفحص الـ artifacts:** محتاج يحتفظ بالقيم الصريحة بعد `Start`، وده عكس ADR-0045.
- **`registry_auth` كسر عادي في الـ spec:** صاحب الـ pipeline كان هيقدر يطبعه. الـ job عمره ما بيشوف الـ authfile.
- **الـ forks ترجّع cache الفرع الافتراضي افتراضيًا:** cache اتكتب بسر ممكن يطلع لكود لسه ماتراجعش.

## النتايج
- أول نسخة من التوصيل الحقيقي: سر المشروع والمؤسسة بيوصل للـ jobs اللي مالهاش بيئة.
- M5.6 بيضيف البيئة للـ resolver (`EnvironmentID`) ولـ `vars`، ويعيد فحص القاعدة في `Start`.
