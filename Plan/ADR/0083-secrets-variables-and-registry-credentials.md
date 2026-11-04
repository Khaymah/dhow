# ADR-0083: الأسرار والمتغيّرات وبيانات دخول الـ registry: النطاقات والإصدارات والحدود

- **الحالة:** مقبول. القرارات 9 لـ 13 في [خطة M5](../M5-Secrets-Environments.md#القرارات-المفوَّضة-كل-واحد-بيبقى-adr-في-أول-مرحلته) مفوَّضة ومكتوبة بالتوصية. و«التوكنات `secret.read` بس» قرار Sabry 6.
- **التاريخ:** 2026-11-04
- **بيكمّل:** [ADR-0014](0014-secrets-management.md) (الـ write-only والتشفير المغلّف)، و[ADR-0045](0045-secrets-transport-and-masking.md) (`SecretRef` والـ masker)، و[ADR-0080](0080-environments-are-made-by-admins-only.md) (البيئات)، و[ADR-0081](0081-keyring-and-envelope-encryption.md) و[ADR-0082](0082-sealer-users-move-to-the-envelope.md) (`Core/Keys` و«أماكن القيم»)، و[ADR-0077](0077-upgrades-migrations-and-update-policy.md) (expand/contract).

## السياق
- `.dhow.yml` بيقبل `${{ secrets.NAME }}` و`vars.*` من M4، والسيرفر بيدّي الـ jobs أسرار فاضية (`jobs.NoSecrets`) ومتغيّرات فاضية.
- M5.1 وM5.2 عملوا `Core/Keys`: كل قيمة متشفّرة بـ DEK لصاحبها، وكل عمود متشفّر بيتسجّل كـ «مكان قيمة» عشان الـ rekey وفحص التسريب يلاقوه.
- M5.4 بيعمل المخزن والـ API بس. التوصيل للـ runner والـ resolver الحقيقي في M5.5، والبوابة في M5.6.

## القرار
1. **موديول `Modules/Secrets`** فيه الأسرار والمتغيّرات وبيانات دخول الـ registry، وبيسجّل عمودين في «أماكن القيم»: `"SecretVersions"."ValueSealed"` و`"RegistryCredentials"."PasswordSealed"`. التانيين بيختموا بـ `Core/Keys` (الـ hook بيرفض `NewSealer`).
2. **النطاقات والصاحب (القرار 9):**

   | النطاق | صاحب الـ DEK | مين بيستخدمه |
   |---|---|---|
   | المؤسسة | `organization` | كل مشاريع المؤسسة (`Visibility` = `all_projects`)، أو المشاريع المختارة في `"SecretProjectAccess"` (`selected_projects`) |
   | المشروع | `project` | كل jobs المشروع اللي مش من fork |
   | البيئة | `environment` | الـ jobs اللي `environment:` بتاعها البيئة دي، بعد البوابة (M5.6) |

   - **الأولوية:** البيئة، وبعدها المشروع، وبعدها المؤسسة. نفس الاسم في أكتر من نطاق مسموح.
   - **`Overrides` بدل `ShadowedBy`:** السر الأضيق هو اللي بيقول «بيغطي على» (`Overrides` = `["organization"]` أو `["project", "organization"]`)، لأن سر المؤسسة الواحد ممكن يتغطّى في مشروع ومايتغطّاش في التاني، فمالوش قيمة واحدة يحطها في `ShadowedBy`. القايمة بتتحسب للي يقدر يشوفه بس: سر مؤسسة على مشروع مش مختار مابيتحسبش.
   - **الـ PRs من نفس المستودع:** أسرار المؤسسة والمشروع مابتوصلهاش إلا بـ `AvailableToPullRequests = true` (افتراضيًا `false`). أسرار البيئة حسب قاعدة البيئة (`AllowPullRequests`، M5.6)، فالحقل ده مش موجود على سر البيئة.
   - **الـ forks:** ولا حاجة من النطاقات التلاتة، ولا متغيّرات، ولا registry.
3. **الختم:** الـ label `dhow/secret` والـ context `secret:<SecretId>:v<Version>`، والـ label `dhow/registry-credential` والـ context `registry:<CredentialId>`. فالقيمة مربوطة بالسر وبرقم الإصدار، ومنسوخة لإصدار تاني أو لسر تاني مابتتفتحش. `restore-version` بيفتح القيمة القديمة ويختمها من جديد بالـ context الجديد.
4. **الـ API للكتابة بس:**
   - `PUT .../secrets/{secretName}` بيعمل السر أو إصدار جديد. الـ body فيه `Value` (إجباري) و`Description` و`AvailableToPullRequests`، و`Visibility` و`ProjectIds` لسر المؤسسة بس. والرد هو الـ metadata.
   - الـ metadata: `Name` و`Scope` و`Description` و`Version` و`SizeBytes` و`AvailableToPullRequests` و`Visibility` و`ProjectIds` و`Overrides` و`CreatedAt` و`UpdatedAt` و`UpdatedByUserId` و`LastUsedAt`. مفيش `Value` ولا hash ولا بصمة، لأن البصمة بتكشف إن قيمتين متساويين.
   - `SizeBytes` بيفضل لأن الحد (256 KiB لكل job) لازم يتحسب قبل التشغيل، والطول مابيكشفش القيمة.
   - **مفيش طريقة تقرا قيمة**، ولا حتى للـ Owner. اللي نسي القيمة بيكتب واحدة جديدة.
5. **الإصدارات والاحتفاظ (القرار 10):**
   - كل `PUT` بيعمل إصدار جديد. القيمة بتفضل في آخر 5 إصدارات بس: الإصدار السادس بيمسح قيمة الأول في نفس الـ transaction.
   - أي إصدار مش الحالي قيمته بتتمسح بعد 30 يوم من ما إصدار أحدث اتعمل (في `Database/Retention` اللي بيشتغل كل 10 دقايق تحت advisory lock).
   - المسح `"ValueSealed" = NULL` ومعاه `"ErasedAt"`، والـ CHECK بيربطهم. مفيش `DELETE`.
   - `restore-version` على إصدار قيمته اتمسحت: `409 SECRET_VERSION_ERASED`.
   - `remove` بيعلّم السر ممسوح وبيمسح قيم كل إصداراته في نفس الـ transaction، والاسم يرجع متاح.
   - **البيئة اتمسحت:** `Secrets` مشترك في `environment.removed` في نفس الـ transaction (الـ bus متزامن و`Audited.Tx`)، فبيمسح أسرار البيئة ومتغيّراتها وبعدها DEK البيئة نفسه (`Envelope.DestroyIn`). لو أي حاجة فشلت، مسح البيئة بيرجع. المشروع بيتأرشف ومابيتمسحش، فأسراره بتفضل لحد ما يبقى فيه مسح مشروع.
6. **الأسماء والحدود (القرار 11):**
   - الاسم `^[A-Za-z_][A-Za-z0-9_]{0,63}$` زي `jobspec`، وأي اسم بيبدأ بـ `DHOW_` (من غير فرق في الحروف) مرفوض بـ `SECRET_NAME_RESERVED`. فريد في النطاق من غير فرق في الحروف (unique على `lower("Name")`)، فـ `Token` و`TOKEN` مايبقوش سرين.
   - القيمة UTF-8 صحيح من غير NUL، من 8 byte (`SECRET_TOO_SHORT`، لأن الأقصر مابيتقنّعش كويس) لـ 32 KiB (`SECRET_TOO_LARGE`). المفاتيح الثنائية بتتكتب base64.
   - العدد: 500 سر للمؤسسة، و200 للمشروع، و100 للبيئة (`SECRET_LIMIT_REACHED`). الحد بيتعدّ تحت قفل على صف النطاق، فطلبين مع بعض مايعدّوهوش.
   - الوصف 256 حرف بالكتير، ومن غير حروف غير مرئية (زي ADR-0080).
   - 256 KiB لكل job و50 سر (`jobspec.MaxSecrets`) بيتطبّقوا في `Check` و`Resolve` (M5.5 بيناديهم).
7. **المتغيّرات (القرار 12):** `"Variables"` بنفس النطاقات والأولوية وقواعد الأسماء، والقيمة نص UTF-8 من 0 لـ 4 KiB، و256 لكل نطاق. بتتقري بالـ API لأي حد عنده `project.read` (ومعاهم التوكنات)، فالواجهة بتقول إنها مش للأسرار. مفيش إصدارات، والـ audit بالاسم بس.
8. **بيانات دخول الـ registry (القرار 13):**
   - `"RegistryCredentials"` على المؤسسة (لكل مشاريعها) أو المشروع: `Host` و`Username` (256) و`Password` (1 لـ 4096 byte، للكتابة بس). الأولوية للمشروع على نفس الـ host. 20 لكل نطاق.
   - `Host` اسم بقواعد `jobspec.ParseEgress` (ASCII أو punycode، من غير wildcard ولا IP خاص)، ومعاه بورت لو موجود، وفريد في النطاق. `docker.io` بيتحوّل لـ `registry-1.docker.io` لأن ده الـ host اللي الصور بتتسحب منه.
   - **السيرفر** بيستخدمها وهو بيثبّت الـ tag بالـ digest (`Core/SecretStore.RegistryLogins`، و`Pipelines` بيناديه): بيبعت Basic للـ host نفسه بس لما يطلب Basic، وللـ token realm بس لو الـ realm `https` على نفس الـ host والبورت، أو `auth.docker.io` لـ `registry-1.docker.io`. أي realm تاني مسموح (زي `ghcr.io` لـ registry مختلف) بيتسأل من غير بيانات دخول، وكل اتصال من خلال `netguard`. اترفض «نفس الـ registrable domain» لأنه محتاج قايمة الـ public suffixes، وده اعتمادية جديدة لحالة نادرة.
   - **تنفيذات الـ forks** بتثبّت الصور من غير بيانات دخول أبدًا، عشان PR من fork مايقدرش يعرف digest صورة خاصة.
   - الـ runner بياخدها في M5.5.
9. **الصلاحيات:**

   | الإجراء | Owner | Admin | Maintainer | Developer | Viewer | التوكن |
   |---|:-:|:-:|:-:|:-:|:-:|:-:|
   | `secret.read` (الأسماء والـ metadata) | ✅ | ✅ | ✅ | ✅ | — | ✅ |
   | `secret.manage` (أسرار ومتغيّرات وregistry المشروع) | ✅ | ✅ | ✅ | — | — | — |
   | `org.secret.manage` (نطاق المؤسسة) | ✅ | ✅ | — | — | — | — |

   - `secret.read` و`secret.manage` إجراءات مشروع (الـ binding بيرفع أو بيقلّل)، و`org.secret.manage` إجراء مؤسسة. أسرار البيئة ومتغيّراتها بتتكتب بـ `environment.manage` (ADR-0080)، وبتتقري بـ `secret.read`.
   - `secret.manage` و`org.secret.manage` حساسين (ADR-0054). والكتابة مابتطلبش `CurrentFactor` مع كل طلب: كتابة قيمة جديدة مابتضعّفش أي حماية، والجلسة لازم عدّت سياسة الخطوة التانية.
   - **التوكنات `secret.read` بس** (قرار Sabry 6): migration بيوسّع الـ CHECK على `"ApiTokens"."Scopes"`، وأي route بيكتب بيرفض التوكن، وصفوف الـ route-table بتتأكد.
10. **الـ port `Core/SecretStore`:** `Check(ctx, q, Request)` بيرجّع الأسماء الناقصة واللي مش للـ PRs، و`Resolve(ctx, Request)` بيرجّع الاسم والنطاق والإصدار والقيمة بعد الفك بالأولوية، وبيحدّث `"LastUsedAt"`. الـ fork بيترفض في الاتنين قبل أي قراية (دفاع تاني جنب `Pipelines`). مفيش حد بيناديه لحد M5.5 غير الاختبارات.
11. **الـ audit** في نفس الـ transaction (زي ADR-0080)، والبيانات اسم ونطاق ومعرّف المشروع والبيئة ورقم إصدار بس، عمرها ما فيها قيمة ولا طول: `secret.created` و`secret.updated` و`secret.version_restored` و`secret.removed` و`variable.created` و`variable.updated` و`variable.removed` و`registry_credential.created` و`registry_credential.updated` و`registry_credential.removed`. المسح التلقائي بعد 30 يوم مابيتسجّلش لكل إصدار، والـ worker بيكتب العدد في الـ log.
12. **الجداول** (`00056_secrets.sql`، expand): `"Secrets"` و`"SecretVersions"` و`"SecretProjectAccess"` و`"Variables"` و`"RegistryCredentials"` بالأعمدة الأربعة، وFK مركّب على `"Projects"` و`"Environments"`، وCHECK إن البيئة معاها مشروع. و`00057_api_token_secret_read.sql` بيضيف `secret.read` للـ CHECK بـ `NOT VALID` وبعده `VALIDATE`.

## البدائل اللي اترفضت
- **قراية القيمة للـ Owner:** أي جلسة Owner مسروقة كانت هتطلّع كل الأسرار. الـ write-only بيخلّي التسريب محتاج runner وjob.
- **بصمة القيمة في الـ API:** كانت هتساعد «هل القيمة اتغيّرت؟»، بس بتكشف إن سرين متساويين وبتسمح بتخمين القيم القصيرة.
- **DEK واحد للمؤسسة لكل النطاقات:** مسح بيئة كان هيحتاج إعادة تشفير بدل مسح مفتاح.
- **الإصدارات من غير حد:** القيم القديمة بتفضل للأبد، وده عكس «بعد التسريب غيّر القيمة».
- **`CurrentFactor` مع كل كتابة:** تغيير القيمة مش بيفتح وصول جديد، والسياسة المؤسسية بتغطي الجلسة. البيئة نفسها (قواعدها) هي اللي محتاجاه.

## النتايج
- M5.5 بيبني الـ resolver على `Core/SecretStore` من غير ما يلمس الجداول.
- فحص التسريب بيلاقي العمودين الجداد لوحده لأنهم مسجّلين، والـ `rekey` بيعيد ختمهم.
- الواجهة (M5.7 أ) بتبني على نفس الـ API.
