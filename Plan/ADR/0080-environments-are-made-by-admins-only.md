# ADR-0080: البيئات بيعملها Admin بس، والاسم الغلط بيفشّل الـ job

- **الحالة:** مقبول. القرار 8 في [خطة M5](../M5-Secrets-Environments.md#القرارات-المفوَّضة-كل-واحد-بيبقى-adr-في-أول-مرحلته) مفوَّض ومكتوب بالتوصية. قاعدة الـ ref على السيرفر، والمراجع الواحد في Community، والإيقاف اليدوي، ومنع الموافقة على النفس افتراضيًا، دول قرارات Sabry.
- **التاريخ:** 2026-11-03
- **بيكمّل:** [ADR-0070](0070-ports-in-core-and-project-permissions.md) (الدور على المشروع)، و[ADR-0054](0054-second-factor-policy-sessions-and-credential-alerts.md) (الإجراءات الحساسة)، و[ADR-0076](0076-pipeline-runs-definitions-and-forks.md) (الـ forks من غير بيئات)، و[ADR-0077](0077-upgrades-migrations-and-update-policy.md) (expand/contract).

## السياق
- `.dhow.yml` بيقبل `environment:` على الـ job من M4، والسيرفر بيتخطّاه بـ `ENVIRONMENTS_UNAVAILABLE` لحد M5.6.
- لو البيئة بتتعمل لوحدها أول ما حد يكتب اسمها في الملف، يبقى أي غلطة في الاسم (`prodution`) بتعمل بيئة جديدة من غير قواعد ولا مراجعين، والـ job بينشر.
- القواعد (الفروع المسموحة والمراجعين والإيقاف) لازم تفضل على السيرفر، ومفيش commit يقدر يغيّرها (قرار Sabry يوم 2026-11-02).

## القرار
1. **البيئة بيعملها Admin بس**، من الواجهة أو الـ API، بالإجراء `environment.manage` على المشروع. الـ YAML بيشاور على بيئة موجودة، ومابيعملش بيئة أبدًا.
2. **الاسم الغلط:** الـ job اللي بيشاور على بيئة مش موجودة بيخلص `failed` بـ `ENVIRONMENT_NOT_FOUND` (من M5.6)، مش `skipped`، لأنها غلطة إعداد لازم تبان. و`Pipelines` بيفضل بيتخطّى بـ `ENVIRONMENTS_UNAVAILABLE` لحد M5.6.
3. **الإجراءات الجديدة** في `Core/Policy`، كلها إجراءات مشروع (الـ binding على المشروع بيرفع أو بيقلّل، ما عدا Owner وAdmin):

   | الإجراء | Owner | Admin | Maintainer | Developer | Viewer |
   |---|:-:|:-:|:-:|:-:|:-:|
   | `environment.manage` | ✅ | ✅ | — | — | — |
   | `environment.pause` | ✅ | ✅ | ✅ | — | — |
   | `environment.bypass` | ✅ | ✅ | — | — | — |
   | `deployment.approve` | ✅ | ✅ | ✅ | ✅ | — |

   - binding بدور Admin على مشروع بيدّي `environment.manage` على المشروع ده بس.
   - `environment.manage` و`environment.bypass` و`deployment.approve` حساسين (ADR-0054). و`environment.pause` مش حساس، عشان الإيقاف وقت مشكلة مايستناش.
   - `environment.bypass` و`deployment.approve` بيتعرّفوا دلوقتي ومالهمش route لحد M5.6. اختبار الـ route-table بيستثنيهم بالاسم، وبيفشل لو route استخدمهم والاستثناء لسه موجود.
   - ولا واحد منهم scope لتوكنات API. الـ GET بتاع البيئات بـ `project.read` وبيقبل التوكنات، وكل التغييرات بالجلسة بس.
4. **الـ routes** تحت `/organizations/{id}/projects/{projectId}/environments`: القايمة والقراية بـ `project.read`، والإنشاء والتعديل (`PUT`) و`remove` و`resume` بـ `environment.manage`، و`pause` بـ `environment.pause`. الاسم في المسار، ومايتغيّرش بعد الإنشاء لأن الملفات بتشاور عليه.
5. **الإيقاف والاستكمال مش متساويين عن قصد:** Maintainer يوقف بسبب إجباري (500 حرف بالكتير)، والاستكمال Admin بـ `CurrentFactor`. الإيقاف بيمنع البدايات الجديدة بس (M5.6)، والإيقاف مرتين مابيعملش حاجة.
6. **الخطوة التانية:** التعديل والحذف والاستكمال محتاجين `CurrentFactor` مع كل طلب (زي تغييرات الهوية). الإنشاء لأ، لأن البيئة الجديدة مافيهاش أسرار ولا حاجة تتحمي لسه.
7. **التعديل بنسخة (optimistic):** `"Version"` بيزيد مع أي تغيير في الصف، والـ `PUT` لازم يبعته، والقديم بيرجع `409 ENVIRONMENT_VERSION_CONFLICT` من غير أي تغيير. والـ `PUT` بيغيّر الحقول اللي اتبعتت بس.
8. **`"RuleVersion"`** بيزيد مع أي تغيير في القواعد، ومعاها المراجعين والإيقاف والاستكمال، ومابيزيدش مع الوصف. الموافقات في M5.6 مربوطة بيه، فأي تغيير في القواعد بيطلب موافقة جديدة.
9. **القواعد في Community:**
   - `AllowedRefs`: من 0 لـ 50 (`Kind` = `branch` أو `tag`، و`Pattern`)، والافتراضي الفرع الافتراضي للمستودع المربوط (أو `main` لو مفيش). الـ pattern بيعدّي `internal/pathglob.ValidPattern` (حد 256 والـ `**` لوحده في الجزء)، وبيترفض لو فيه `..` أو `//` أو بيبدأ أو بيخلص بـ `/` أو فيه مسافة أو `~` أو `^` أو `:`، لأن Git مابيقبلش الحاجات دي في اسم ref. القايمة الفاضية معناها مفيش حاجة تنشر.
   - `Reviewers`: 20 بالكتير بين المستخدمين والفرق. المستخدم لازم عضو في المؤسسة ودوره على المشروع Developer أو أعلى بعد الـ bindings، والفريق لازم من نفس المؤسسة، وغير كده `422 REVIEWER_NOT_ELIGIBLE`. والتحقق من كل عضو في الفريق وقت الموافقة (M5.6).
   - `RequiredApprovals` = 1 بس، والـ CHECK في القاعدة `= 1`، والتوسيع expand في M8.
   - `AllowSelfApproval` افتراضيًا `false` ([قرار Sabry 5](../M5-Secrets-Environments.md#قرارات-sabry-2026-11-03)): ده نفس `PreventSelfReview` في الخطة بالعكس، والاسم الموجب بيخلّي الحقل الغايب يقفل مش يفتح. والـ Admin بس اللي يقدر يفتحه لبيئة واحدة، بـ `CurrentFactor` زي أي تعديل، والتغيير في الـ audit وبيبان في رد البيئة.
   - `WaitMinutes` من 0 لـ 43200، و`ApprovalTimeoutHours` من 1 لـ 720 (الافتراضي 72)، و`AllowedPlugins` 50 بادئة بالكتير، و`RequiredIsolation` = `container` بس، و`AllowAdminBypass` افتراضيًا `false`، و`SupersedePending` افتراضيًا `true`. التنفيذ في M5.6، والتحقق دلوقتي.
   - النوافذ والتجميد المجدول وأكتر من مراجع في M8.
10. **الحذف soft:** `remove` بيعلّم البيئة ومراجعينها `"IsDeleted"`، والاسم يرجع متاح. أسرار البيئة والـ DEK بتاعها بيتمسحوا مع M5.4.
11. **الـ audit بعد الـ commit:** `environment.created` و`environment.updated` و`environment.removed` و`environment.paused` و`environment.resumed`، والـ TargetType `environment`. التعديل فيه `Fields` و`From` و`To` (JSON مضغوط للحقول اللي اتغيّرت بس، و256 حرف بالكتير زي باقي الـ audit) و`Version` (الـ `RuleVersion` الجديد). مفيش أسرار في البيئة لسه، ولما تيجي في M5.4 مابتدخلش الـ audit أبدًا.
12. **الجداول** (`00051_environments.sql`، expand): `"Environments"` و`"EnvironmentReviewers"` بالأعمدة الأربعة، وFK مركّب على `"Projects"`، والاسم unique مع المشروع `WHERE NOT "IsDeleted"`. `"LockedByRunId"` بيتضاف مع M5.6 اللي بيستخدمه.

## البدائل اللي اترفضت
- **البيئة بتتعمل من أول job:** زي GitHub. غلطة في الاسم بتبقى بيئة إنتاج من غير حماية، وده عكس قاعدة «الأمان مايتقفلش ورا إعداد».
- **الاسم الغلط `skipped`:** الـ run بيخلص أخضر والنشر ماحصلش، وماحدش بياخد باله.
- **Maintainer يعمل البيئات:** الـ Maintainer بيعدّل `.dhow.yml` ويدير أسرار المشروع، فلو كمان بيعمل البيئة وقواعدها يبقى مفيش فصل بين اللي بيكتب الكود واللي بيحمي الإنتاج.
- **الإيقاف بـ Admin بس:** وقت الحادثة الـ Maintainer هو اللي موجود غالبًا، والإيقاف مابيضعّفش أي حماية.
- **`PUT` بيستبدل كل القواعد:** عميل نسي حقل يرجّعه للافتراضي من غير ما يقصد. التعديل الجزئي مع `Version` أوضح.
- **`RuleVersion` مابيزيدش مع الإيقاف:** موافقة اتاخدت قبل حادثة كانت هتعدّي بعد الاستكمال من غير ما حد يبص تاني.

## النتايج
- مفيش بيئة من غير صاحب. اللي بينسى يعمل البيئة بيشوف `ENVIRONMENT_NOT_FOUND` واضح من M5.6.
- الـ route-table فيه صف لكل route لكل دور، ومعاه binding Admin على مشروع واحد.
- الواجهة (M5.7) بتبني على نفس الـ API.
