# ADR-0085: بوابة البيئة وقفلها وسجل النشر (M5.6أ)

- **الحالة:** مقبول. القرارات 17 و19 و20 في [خطة M5](../M5-Secrets-Environments.md#القرارات-المفوَّضة-كل-واحد-بيبقى-adr-في-أول-مرحلته) مفوَّضة ومكتوبة بالتوصية، وقسمة M5.6 على PRين قرار Sabry يوم 2026-11-04. القرار 18 (الموافقات) وقرار Sabry 5 في ADR الـ PR التاني (M5.6ب).
- **التاريخ:** 2026-11-04
- **بيكمّل:** [ADR-0070](0070-ports-in-core-and-project-permissions.md) (الـ ports جوه transaction اللي بينادي)، و[ADR-0076](0076-pipeline-runs-definitions-and-forks.md) (الـ forks)، و[ADR-0080](0080-environments-are-made-by-admins-only.md) (البيئات والقواعد)، و[ADR-0084](0084-secret-delivery-forks-and-cache.md) (التوصيل وقت `Start`).

## السياق
- M5.3 عمل البيئات وقواعدها، وM5.5 وصّل أسرار المؤسسة والمشروع. بس أي job فيه `environment:` لسه بيتخطّى بـ `ENVIRONMENTS_UNAVAILABLE`، فأسرار البيئة مابتوصلش لأي حد.
- M5.6 في الخطة حجمه L. Sabry اختار يتقسم: **(أ)** البوابة نفسها والقفل والسجل وأسرار البيئة، و**(ب)** الموافقات والرفض والانتهاء والاستبدال والتخطّي والعدّاد.

## القرار
1. **البوابة عمود مش حالة (القرار 17):** `"RunJobs"."Gate"` (`approval` أو `timer` أو `paused` أو `environment_busy`) و`"GateOpensAt"`، والـ `RunJob` بيفضل `waiting` والتنفيذ `running`. مفيش تعديل على triggers الحالات، فالـ binary القديم لو رجعنا له بيشوف job فيه بيئة ويتخطّاه بـ `ENVIRONMENTS_UNAVAILABLE`: بيقفل مش بيفتح.
2. **الـ port `Core/Deployments`** (زي ADR-0070: interfaces وأنواع بس، جوه transaction اللي بينادي، ومن غير نشر على الـ EventBus). `Environments` بينفّذه و`App` بيركّبه في `Pipelines`:
   - `Lock` بيقفل صف البيئة `FOR UPDATE` ويرجّع اللي البوابة محتاجاه (ومنه `RefAllowed` محسوب في `Environments`، عشان المطابقة في مكان واحد)، أو «مش موجودة».
   - `Started` بيمسك البيئة للـ run ويكتب صف `"Deployments"` بعد ما الـ job يتدرج.
   - `Settle` بيقفل صفوف النشر للـ `RunJobs` اللي خلصت، ويفك أي بيئة الـ run ماسكها ومش في قايمة `keep` (البيئات اللي لسه ليها jobs مش منتهية في الـ plan).
   - `ReleaseStale` للـ reconciler: بيفك أي قفل صاحبه خلص أو اتمسح.
   - الفحص تاني وقت `Start` مش في الـ port: `App` بينادي `Environments.CheckStart` مباشرة.
3. **ترتيب الفحوص** زي [الخطة](../M5-Secrets-Environments.md#البوابة-ترتيب-الفحوص) بالظبط:
   1. الـ fork: `skipped` بـ `FORK_PR_NO_ENVIRONMENT`.
   2. البيئة موجودة: وإلا `failed` بـ `ENVIRONMENT_NOT_FOUND`.
   3. المتغيّرات من البيئة والمشروع والمؤسسة، وبعدها `if`. فالـ job اللي شرطه `false` مابيستناش.
   4. قاعدة الـ ref: `skipped` بـ `ENVIRONMENT_REF_NOT_ALLOWED`، ومعاه `{"Ref", "Environment"}` في `"FailureDetail"` من غير الـ patterns.
   5. القواعد الثابتة على الـ spec: `ENVIRONMENT_REQUIRES_PINNED_IMAGES` و`ENVIRONMENT_PLUGIN_NOT_ALLOWED` (`failed`). و`ENVIRONMENT_ISOLATION_REQUIRED` محجوز، لأن `container` هو المستوى الوحيد النهارده وكل job بيحققه.
   6. الأسرار موجودة بنطاق البيئة: `SECRET_NOT_FOUND` قبل ما حد يستنى.
   7. متوقفة: `"Gate" = 'paused'`.
   8. فيها مراجعين: `"Gate" = 'approval'`. في (أ) مفيش طريق للموافقة، فالـ job بيستنى لحد (ب) أو لحد ما حد يلغي التنفيذ أو يشيل المراجعين. ده fail closed مقصود.
   9. المؤقت: `"GateOpensAt"` بيتحدد أول مرة الـ job يوصل للخطوة دي، و`"Gate" = 'timer'` لحد ما الوقت ييجي.
   10. التزامن: `"Gate" = 'environment_busy'` لو run تاني ماسك البيئة.
   11. الإدراج: بعدها `registry_auth` والـ transfers والـ `Enqueue` زي M5.5، وبعد ما الإدراج ينجح بس `Started` بيمسك البيئة ويكتب صف النشر. فلو الطابور اتملا مفيش قفل ولا صف بيفضل من غير job.
4. **قاعدة الـ ref:**
   - push وtag ويدوي: الـ ref بنوعه (`branch` أو `tag`) لازم يطابق `Pattern` بنفس النوع بـ `internal/pathglob`. النوع جزء من المطابقة، فـ tag اسمه `main` مايعدّيش قاعدة الفرع `main` (من مراجعة M5.3).
   - PR من نفس المستودع: لازم `AllowPullRequests`، والمطابقة على **فرع الـ base** (الـ ref بتاع الـ PR `refs/pull/N/head` ومالوش اسم فرع). يعني PR على `main` يقدر ينشر على بيئة معاينة قاعدتها `main` لو الـ PRs مسموحة.
   - `AllowedRefs` فاضية: مفيش حاجة تنشر.
5. **«الصور بالـ digest» والإضافات (القرار 20):** الـ digest لازم يبقى **مكتوب في الملف** (`@sha256:`) لصورة الـ job وكل صورة إضافة. التثبيت اللي السيرفر بيعمله للـ tag (§12.5) مابيتحسبش. و`AllowedPlugins` بادئة اسم الصورة بعد `QualifyImage`، والفاضية = أي إضافة.
6. **التزامن (القرار 19):**
   - `"Environments"."LockedByRunId"`: الـ run بيمسك البيئة من أول job بيتدرج عليها لحد ما مايبقاش فيه `RunJob` منه على البيئة دي مش منتهي (حسب الـ plan، فالـ job اللي لسه ماوصلش البوابة بيتحسب). كده `deploy` و`migrate-db` في نفس الـ run مايتداخلش بينهم run تاني.
   - الفك في آخر كل `advance` (بعد ما الـ jobs تتحرك) وفي `abandon`، و`ReleaseStale` في الـ reconciler بيفك أي قفل صاحبه خلص.
   - ترتيب الأقفال: الـ run (`LockRun`) ثم صف البيئة ثم طابور المؤسسة، زي الخطة.
   - **الطابور:** لما البيئة تفضى، الـ run صاحب **أصغر رقم** بين اللي مستنيين عليها بياخدها. الخطة قالت «الأقدم في الوصول للبوابة»، والرقم أبسط ومن غير عمود جديد، وفي الغالب نفس الترتيب. `"RunJobs"."EnvironmentId"` بيتكتب وهو مستني عشان الطابور يتحسب.
7. **إعادة التقييم:** الـ reconciler كل 5 ثواني بيعدّي على التنفيذات اللي فيها `RunJob` مستني ببوابة (`ListGatedRuns`) ويعيد `advance`. و`ListUnsettledRuns` بقى مابيرجّعش تنفيذ كل اللي مستني فيه ببوابة، عشان مايتعدّش «unsettled» كل 5 ثواني لمدة أيام.
8. **سجل النشر `"Deployments"`** (`queued` ثم `succeeded` أو `failed` أو `canceled`): صف لكل job عدّى البوابة، بالبيئة والـ run والـ `RunJob` والـ job والـ sha والـ ref و`"RuleVersion"` وقت البوابة. `running` محجوز في الـ CHECK لـ M6، لأن `Pipelines` مابيعرفش إمتى الـ attempt بدأ. «المنشور دلوقتي» = آخر `succeeded`.
9. **الفحص تاني وقت `Start`:** لو الـ `RunJob` ليه `"EnvironmentId"`، الـ resolver بيتأكد إن البيئة لسه موجودة ومش متوقفة، وإن الـ ref لسه مسموح، وإن `"RuleVersion"` هو نفسه اللي في صف النشر. غير كده المحاولة بتفشل `ENVIRONMENT_RULE_CHANGED` من غير ما أي قيمة تتفك. وأسرار البيئة ومتغيّراتها بتوصل بنطاق `"RunJobs"."EnvironmentId"` (مش من الـ spec).
   - الفحص ده جوه الـ resolver، فبيحصل بس لو الـ job طلب سر. job على بيئة من غير أي سر اتدرج قبل الإيقاف بيشتغل، وده مقبول: مفيش حاجة تتحمى، وأغلب jobs النشر بتطلب مفتاح. وقفه هو كمان محتاج فحص في `Start` لكل job، ودي نقطة نرجعلها مع (ب) لو احتجناها.
10. **الـ API:** الـ `RunJob` بيزيد عليه `Environment` و`Gate` و`GateOpensAt`، و`GET .../environments/{environmentName}/deployments` بـ `run.read` وصفحات.
11. **الأسباب الجديدة** على `"RunJobs"."FailureReason"` (`TEXT` من غير CHECK على القايمة): `ENVIRONMENT_NOT_FOUND` و`ENVIRONMENT_REF_NOT_ALLOWED` و`ENVIRONMENT_REQUIRES_PINNED_IMAGES` و`ENVIRONMENT_PLUGIN_NOT_ALLOWED` و`ENVIRONMENT_ISOLATION_REQUIRED` و`ENVIRONMENT_RULE_CHANGED`.

## البدائل اللي اترفضت
- **حالة جديدة `gated` على `"RunJobs"`:** الـ binary القديم كان هيشوف حالة مايعرفهاش، وتعديل الـ triggers بيكسر الرجوع (U3).
- **PR واحد لكل M5.6:** الـ diff كان هيبقى كبير جدًا على مراجعة أمنية واحدة (قرار Sabry).
- **البيئة اللي فيها مراجعين تعدّي لحد (ب):** بيفتح نشر من غير مراجعة لو (ب) اتأخر. الانتظار أأمن.
- **قفل البيئة بالـ advisory lock:** بيتفك مع الـ connection مش مع الـ run، والـ run ماسك البيئة عبر transactions كتير.

## العواقب
- الـ jobs اللي على بيئة بتشتغل بقواعدها، وأسرار البيئة بتوصل بعد قاعدة الـ ref بس.
- بيئة فيها مراجعين بتوقف النشر لحد M5.6ب. ده مكتوب في `docs/guides/environments.md`.
- `ENVIRONMENTS_UNAVAILABLE` بيفضل للـ binary القديم بس.
