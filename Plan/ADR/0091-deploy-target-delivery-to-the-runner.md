# ADR-0091: توصيل هدف النشر إلى الـ runner (M6.2أ)

- **الحالة:** مقبول. [خطة M6](../M6-Deploy-Targets.md) §M6.2 (القرارات المفوَّضة 4 إلى 7)، وهذا النصف الخاص بالسيرفر والبروتوكول.
- **التاريخ:** 2026-11-05
- **بيكمّل:** [ADR-0090](0090-deploy-targets-hosts-and-host-keys.md) (الأهداف وسيرفراتها ومفاتيحها)، و[ADR-0085](0085-environment-gate-and-deployment-lock.md) (البوابة)، و[ADR-0084](0084-secret-delivery-forks-and-cache.md) (تسليم الأسرار)، و[ADR-0057](0057-egress-proxy-implementation.md) (الـ egress)، و[ADR-0047](0047-job-state-machine-and-reconciliation.md) (لا إعادة بعد خطوة تغيّر شيئًا).

## السياق
- الـ commit يكتب اسم الهدف فقط. المطلوب أن يصل الهدف إلى الـ runner بما يكفي للنشر، وأن يصل مفتاح الدخول إلى خطوة النشر وحدها، وأن تكون الشبكة المسموحة للـ job هي نقاط الهدف بالضبط.
- قسّم Sabry M6.2 إلى طلبين. هذا الطلب (أ) هو السيرفر والبروتوكول. الطلب (ب) هو الـ runner: فرض `exact_endpoints` في الـ proxy، وفحوص `/proc` على ملف المفتاح، وتسليم الملف الناتج، والتقارير و`"DeploymentReleases"` وحالة `unknown` و`internal/archive`.

## القرار
1. **الإضافات الرسمية وحدها تستلم هدفًا** (القرار 4). الاسمان `ghcr.io/khaymah/dhow-deploy-ssh` و`ghcr.io/khaymah/dhow-deploy-compose` ثابتان في الـ binary (`jobspec.OfficialDeployImages`)، ويضيف المدير بادئات صور لهدف بعينه في `AllowedImages`. أي خطوة `uses:` فيها `with.target` وصورتها خارج القائمة تفشل بـ `TARGET_PLUGIN_NOT_ALLOWED`.
2. **الفحوص عند الإدراج** تجري في `Pipelines` بعد فحص الأسرار وقبل البوابة (فلا ينتظر أحد موافقة على job محكوم عليه بالفشل)، وكلها عبر port جديد `Core/Deployments.Targets` ينفّذه `Environments`: `TARGET_REQUIRES_ENVIRONMENT` ثم `TARGET_NOT_LITERAL` ثم `TARGET_NOT_FOUND` ثم `TARGET_PLUGIN_NOT_ALLOWED` ثم `TARGET_HOST_KEY_UNCONFIRMED` (بعناوين السيرفرات في `FailureNames`) ثم `TARGET_KEY_NOT_FOUND` (السر على نطاق البيئة بالتحديد) ثم `TARGET_ARTIFACT_REQUIRED` (اسم الملف الناتج يطابق ملفًا تعلنه job في `needs`) ثم `HEALTH_URL_INVALID` و`HEALTH_BUDGET_EXCEEDED` (10 دقائق للسيرفر) و`ROLLING_BUDGET_EXCEEDED` (60 دقيقة للنشر). تحديد الخطوة التي فيها الهدف يعتمد على `BuildWithOrigins` الجديدة في `pipelinespec`، لأن الخطوات المتخطاة بـ `if:` تغيّر الأرقام.
3. **الـ egress** (القرار 5): الـ job الذي فيه هدف يأخذ `Network.exact_endpoints` يحسبها السيرفر فقط: نقطة `host:port` لكل سيرفر، ثم نقاط فحص الصحة من `health_check_url` (و`{host}` يستبدل اسم السيرفر كاملًا فقط، فيعطي نقطة لكل سيرفر). الحقل لا يوجد في YAML، ويرفضه `jobspec` من غير هدف، ويتأكد أنه يحوي كل سيرفرات الهدف. وأي `network.egress` كتبه المستخدم يُرفض بـ `TARGET_JOB_EGRESS_NOT_ALLOWED` إلا إذا فتح المدير `AllowExtraDeployEgress` (وهي قاعدة بيئة تزيد `"RuleVersion"` من ADR-0090). يبقى فرض النقاط في الـ proxy للطلب (ب)، وإلى ذلك الوقت لا يعلن أي runner الميزة `deploy_targets` فلا يستلم أي job فيه هدف.
4. **`StepTarget`** (القرارين 4 و6): رقم الخطوة، و`target_id` و`version` (للفحص عند البدء)، والاسم والنوع والمستخدم والمسار و`release_id` (`<رقم التشغيل>-<أول 8 من الـ sha>-<الجولة>`) و`keep_releases`، والسيرفرات بالترتيب لكل منها `host_id` والعنوان والمنفذ وسطر `known_hosts` بمفتاحه المثبّت، والإعدادات (الدفعات والـ hooks و`Shared` وإعدادات Compose). لا سر فيه. يضع السيرفر `mutating` على خطوة النشر (فلا إعادة تلقائية بعد بدايتها، ADR-0047)، فلا حاجة لحقل `Mutating` في الـ manifest للإضافات الرسمية. ويبقى حقل الـ manifest للطلب (ب).
5. **المفتاح سر محجوز** `dhow.target.<n>.key` من نوع `as_file` باسم `target-key` (و`target-key-<n>`) و`step_indexes` خطوة الإضافة وحدها. يرفض `jobspec` هذا الاسم من غير هدف، وأي شكل غير هذا. و`DHOW_TARGET_*` تبنيها `jobspec.TargetEnv` (الاسم والنوع والمستخدم والمسار والإصدار والسيرفرات والإعدادات بـ JSON ومسار ملف المفتاح) ويستخدمها الـ runner في الطلب (ب).
6. **الفحص عند `Start`** على نمط `StartGuard.Admit` (القرار 6): قبل أي سر يقارن السيرفر الهدف الحالي بما في الـ spec: `Version` والسيرفرات وترتيبها وعناوينها ومنافذها وسطر المفتاح لكل سيرفر. أي فرق يعطي `TARGET_CHANGED`، وحذف سر المفتاح يعطي `TARGET_KEY_NOT_FOUND`، ولا يُفتح المفتاح ولا يتغيّر `LastUsedAt` ولا يُسجَّل `job.secrets_delivered`. يتكرر الفحص داخل `Resolve` نفسه قبل فتح أي سر. وقيمة المفتاح تُجلب من سر البيئة باسم `KeySecretName` الحالي، ويُرفض أي نطاق غير البيئة.
7. **الأمر البعيد** (التزامات ADR-0090 §11): `jobspec` يرفض أي سيرفر أو مستخدم يبدأ بشرطة أو فيه مسافة أو منطقة IPv6، فلا يصير وسيطًا لـ `ssh`، وعلى الإضافة أن تمرر القيم بعد `--`. والـ hooks أسطر واحدة بلا حروف تحكم، وتُنفَّذ كأمر بعيد واحد ولا تُدمج مع حقل آخر.
8. **الميزة `deploy_targets`:** قيمة جديدة في `RunnerFeature` وفي قيدي `"Jobs"."RequiredFeatures"` و`"Runners"."Features"` بالـ migration `00068` (expand، بـ `NOT VALID`) و`00069` (validate).
9. **الأسباب الجديدة** في `"RunJobs"."FailureReason"` وفي كتالوج الواجهة بالنسختين: ما سبق، و`TARGET_CHANGED` و`TARGET_KEY_NOT_FOUND` عند البدء.

## البدائل اللي اترفضت
- **حساب الـ egress في الـ runner من الهدف:** يضيف ثقة في الـ runner ولا يقدر السيرفر على إثبات النقاط. السيرفر يحسب والـ runner يفرض.
- **تخزين نسخة الهدف في جدول عند الإدراج:** الـ spec الموقَّع يحمل كل ما نحتاجه للمقارنة، فلا جدول جديد.
- **إعلان الميزة من الـ runner قبل فرض النقاط:** كان سيسمح بنشر بشبكة مفتوحة.

## النتايج
- لا تغيير في سلوك الـ runner الحالي: الحقول الجديدة إضافات في البروتوكول (`buf breaking` يمر).
- الطلب (ب) يبني فوق `exact_endpoints` و`TargetEnv` وفحص `/proc` وتسليم الملف الناتج.

## المراجعة الأمنية
- مفيش Critical ولا High.
- **(LOW، مقبول)** بين فحص `Start` وفتح السر لحظة قصيرة يمكن أن يتغيّر فيها الهدف. أسوأ نتيجة أن يصل مفتاح سر البيئة نفسه إلى الخطوة نفسها، ولا يتغيّر ما يصل إليه.
- **(LOW، مقبول)** كاتب الـ commit يختار عنوان فحص الصحة، فيستطيع فتح اتصال لذلك العنوان من job النشر. هذا مكتوب في الوثائق، ومن يريد منعه يضع الفحص خلف عنوان داخلي.
