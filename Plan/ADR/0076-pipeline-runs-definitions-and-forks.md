# ADR-0076: تنفيذ خطوات النشر: منين بنقرا الملف، والـ sha بتاع الـ PR، وسياسة الـ forks (M4.8)

- **الحالة:** مقبول. القرارين 2 و10 مفوَّضين في [`M4-Pipelines.md`](../M4-Pipelines.md)، وسياسة الـ forks إجابة Sabry على السؤال 1، وتثبيت الصور بالـ tag قراره يوم 2026-10-30، ومصير `POST /jobs` إجابته على السؤال 2. والباقي تفاصيل تنفيذ مفوَّضة ومكتوبة هنا.
- **التاريخ:** 2026-11-02
- **بيكمّل:** [ADR-0047](0047-job-state-machine-and-reconciliation.md) (حالات الـ job) و[ADR-0069](0069-pipeline-spec-parser-and-validator.md) (الـ spec) و[ADR-0070](0070-ports-in-core-and-project-permissions.md) (الـ ports) و[ADR-0072](0072-webhooks-and-git-credentials.md) (الـ webhooks).

## السياق
- M4.4 بيسجّل الـ webhook وبيسلّم `Trigger` موحّد على `Core/Triggers`، وكان الـ sink مؤقت بيكتب سطر في الـ log بس.
- `internal/pipelinespec` بيعمل `Check` و`Compile` وبيقيّم CEL، بس مفيش حد بيحوّل الملف لـ jobs ولا بيمشّي الـ DAG.
- السيرفر صورة distroless من غير git، وGit العام (Gitea وForgejo وأي سيرفر) مالوش API للملفات.
- الـ checkout في الـ runner (`JobSpec.checkout`) بتاع M4.7 وشغّال بالتوازي، فلازم M4.8 يتبني حواليه من غير ما يلمسه.
- مراجعة M4.4 الأمنية: webhook موقّع من غير وقت ممكن يتعاد بعد ما سجل الـ deliveries يتمسح (30 يوم). ومراجعة M4.3: routes الـ jobs على مستوى المؤسسة كانت بتتجاهل الدور على المشروع، والـ audit بتاع أي نداء لـ `Core/JobQueue` مسؤولية اللي بينادي.

## القرار

### القرار 2: منين بنقرا `.dhow.yml`
- **GitHub وGitLab** (M4.5 وM4.6): من API المزوّد. الـ interface `Definitions.Fetch` بيتنادى بره أي transaction، والتنفيذ الحالي بيرجّع `ErrProviderUnavailable` للمزوّدين دول (الـ run بيفشل `PROVIDER_UNAVAILABLE` بدل ما يستنى).
- **Git عام:** «discovery job» على runner بيعمل checkout للـ commit ويرجّع الملف في الـ outputs (`dhow.definition` و`dhow.sha` و`dhow.found`). الـ job ده صف في `"RunJobs"` بـ `"Kind" = 'discovery'` ومفتاح `dhow.discovery` (فيه نقطة، فمايتلخبطش مع id أي job)، وبيتدرج بـ `Core/JobQueue` بالميزة `checkout` وأولوية 50.
- **الـ seam:** `CheckoutSeam.DiscoverySpec` بيبني الـ spec بتاع الـ discovery job، و`CheckoutSeam.ForJob` بيضيف الـ checkout لكل job عادي. التنفيذ الحالي (`CheckoutPending`) بيرجّع `ErrCheckoutUnavailable` للأول، فالـ run بيفشل **مقفول** بـ `CHECKOUT_UNAVAILABLE` ومن غير أي job، ومابيعملش حاجة للتاني. M4.7 بيبدّل التنفيذ ده بس، وكل اللي حواليه (الإدراج، وتحويل الـ output لتعريف، والتشغيل) موجود ومختبَر بـ seam وهمي.
- **ليه مش git في السيرفر؟** نفس سبب الخطة: مفيش parsing لـ packfiles غريبة في الـ control plane.

### القرار 10: الـ PR بيتشغّل على sha الـ head
- الـ run بياخد `"Sha"` = آخر commit في الـ PR، مش merge ref. الـ check بيظهر على الـ commit اللي اتعمله push، وثابت لو الـ base اتحرك.

### سياسة الـ forks (السؤال 1)
- **التعريف من الـ base:** `"DefinitionSha"` = `"BaseSha"` لأي PR من fork. الـ PR مايقدرش يضيف jobs ولا يفتح egress ولا يغيّر صور. والـ head بيتعمله checkout كبيانات بس.
- **الموافقة:** افتراضيًا الـ run بيبدأ `awaiting_approval` ومابياخدش runner ولا حتى بيتقري ملفه، لحد ما حد عنده `run.approve` على المشروع (Maintainer وأعلى) يوافق. و`run.approve` من الإجراءات الحساسة (خطوة تانية، ADR-0054)، ومن غير توكنات API. والـ `automatic` (من غير موافقة) إعداد للمشروع يختاره Admin بس (ADR-0070).
- **الحدود:** job فيه `environment:` بيبقى `skipped` بـ `FORK_PR_NO_ENVIRONMENT`، وjob بيطلب سر (في `env` أو `with`) بيفشل بـ `FORK_PR_NO_SECRETS` قبل الإدراج، حتى بعد M5. والـ cache للقراية بس (M4.9 بيقرا `"IsFork"`).

### تثبيت الصور بالـ tag (قرار Sabry)
- وقت بداية الـ run، السيرفر بيسأل الـ registry عن digest كل `image:` مكتوبة بالـ tag (`HEAD` على `/v2/<repo>/manifests/<tag>`، ولو مفيش `Docker-Content-Digest` بيعمل `GET` وبيحسب sha256 للـ manifest). وبيسجّل النتيجة في `"PipelineRuns"."ImagePins"`، وبيعمل compile بالصورة `name:tag@sha256:…`. كل job وكل إعادة في نفس الـ run بيستخدموا نفس الـ pins من غير ما يسألوا تاني.
- **الحماية:** من خلال `internal/netguard` بنفس `DHOW_GIT_ALLOW_PRIVATE_CIDRS` (registry داخلي على شبكة خاصة لازم يتسمح بنطاقه صراحة)، و`https` بس، ومن غير redirects، ومهلة 15 ثانية للصورة، والـ manifest 4 MiB بالكتير، ورد التوكن 64 KiB. توكنات السحب المجهولة (Bearer challenge زي Docker Hub) شغّالة، والـ realm لازم `https` ويعدّي على نفس الحماية. والـ registry اللي محتاج كلمة سر مش مدعوم لحد M5.
- **الفشل:** tag مش موجود أو registry مش متاح بيفشّل الـ run بـ `IMAGE_PIN_FAILED` بمكان الخطأ (`jobs.<id>.image`). و`Check` و`dhow lint` بيفضلوا يطلّعوا `IMAGE_NOT_PINNED` warning.

### `POST /organizations/{id}/jobs` (السؤال 2)
- الـ route موجود في الـ API والـ route-table بنفس الـ policy، بس السيرفر بيرد `404` لو `DHOW_DEV` مش مفعّل. الاختبارات بتفتحه بـ `Infra.ManualJobs`. والبديل في الإنتاج `on: manual` بـ inputs: أي حاجة بتشتغل لازم يبقى ليها ملف في Git.
- الواجهة مابتناديش الـ route ده خالص، فـ `Requests.test.tsx` مااتأثرش.

### جداول M4.8 (migration `00044`)
- `"PipelineRuns"`: الحالات `awaiting_approval` و`pending` و`queued` و`running` و`succeeded` و`failed` و`canceled` و`skipped`، وtrigger بيقفل الانتقالات، والـ `failed` و`canceled` بيرجعوا `running` بس بـ «أعد تشغيل الفاشل».
- `"RunJobs"`: `waiting` و`active` و`succeeded` و`failed` و`canceled` و`skipped`، والـ `succeeded` عمره ما بيتغيّر، و`"CurrentJobId"` unique.
- `"PipelineRunCounters"` للرقم المتسلسل لكل مشروع (صف لكل مشروع بـ `ON CONFLICT ... RETURNING`).
- **التعريف بيتخزّن نص** (`"DefinitionSource"` بحد 256 KiB) مع `"DefinitionDigest"`، مش الـ plan كـ jsonb زي الخطة. السبب: تقييم CEL محتاج الـ `RunJob` بعد الـ compile (فيه declarations داخلية)، فالسيرفر بيعمل compile تاني من النص والـ pins لما job يخلص. وكل `RunJob` شايل `"Digest"` من أول compile، ولو الـ compile الجديد طلّع digest مختلف (ترقية لداو غيّرت المعنى) الـ job بيفشل بـ `DEFINITION_CHANGED` بدل ما يشتغل بمعنى تاني.
- **`"Jobs"."CreatedByUserId"` بقى nullable:** الـ jobs اللي جاية من webhook مالهاش مستخدم (البند المؤجَّل في ADR-0070). الـ port بيقبل `CreatedByUserID` فاضي، ولو موجود لازم عضو حالي، و`Pipelines` بيتأكد من العضوية قبل الإدراج (`CREATOR_NOT_MEMBER`) عشان الرفض مايبقاش error بيرجّع transaction بتاع `Jobs`. والـ `POST /jobs` اليدوي دايمًا ليه مستخدم.
- **`"CancelRequestedBy"` بقى ممكن يبقى فاضي** مع `"CancelRequestedAt"` (إلغاء من النظام، زي `cancel_in_progress` من webhook). الـ CHECK القديم اتشال واتعمل بداله `"CancelRequestedBy" IS NULL OR "CancelRequestedAt" IS NOT NULL`، متحقق منه في نفس الـ migration لأن الجدول لسه صغير قبل أول نسخة.
- scopes جديدة للتوكنات: `run.read` و`run.trigger` و`run.cancel`. و`run.approve` من غير توكن.

### مرة واحدة بالظبط (من غير حالة في الذاكرة)
- **نفس الـ trigger مرتين:** `Deliver` بياخد advisory lock على (المستودع، الـ ref) جوه transaction بتاع `Sources`، وبيدوّر في `"PipelineRuns"` على نفس (المستودع، الـ ref، الـ sha)، ولو لقاه بيرجّع نفس الـ run بـ `Created = false`. وفيه unique index على نفس التلاتة للـ push والـ tag والـ PR كحماية أخيرة. والحكم من جدول الـ runs، مش من `"WebhookDeliveries"` اللي بيتمسح بعد 30 يوم (الملاحظة الملزمة من مراجعة M4.4).
- **`JobFinished`:** بينادي `Core/RunEvents` من نفس transaction بتاع `Jobs` اللي نقل الـ job لحالة نهائية (الـ `Complete` والـ reconciler وإلغاء job مستني وانتهاء مهلة الطابور). `Pipelines` بيعمل `SELECT ... FOR UPDATE` على الـ run الأول، وبعدين بيقرا الـ `RunJob` تاني ويكمّل بس لو `active` و`"CurrentJobId"` هو نفس الـ job. فـ 100 حدث متوازي بيتعملوا serialize على صف الـ run، والحدث المكرر بيلاقي الـ `RunJob` خلص ومابيعملش حاجة. والإدراج نفسه `UPDATE ... WHERE "State" = 'waiting'` كحماية تانية.
- **ترتيب الأقفال:** (1) مجموعة التزامن (advisory)، (2) صف الـ run، (3) قفل طابور المؤسسة بتاع `Jobs`. وصف الـ job بتاع `Jobs` (بيتقفل قبل الـ run في `Complete`) عمره ما بيتقفل وإحنا ماسكين run: الإلغاء بيتسجّل على الـ run والـ `RunJobs` الأول، وبعد الـ commit كل job بيتلغي في transaction لوحده (`afterCancel`)، والـ reconciler بيعيد اللي ضاع. و`JobFinished` مابياخدش قفل مجموعة ولا run تاني، فالـ run اللي بيخلص مابيبدأش اللي بعده، والـ worker هو اللي بيبدأه.

### الـ worker والـ reconciler
- **كل ثانية:** `StartPending` بياخد run `pending` بـ `SKIP LOCKED` وبيحجزه دقيقة (`"ClaimedUntil"`)، وبيجيب الملف ويتحقق منه ويثبّت الصور **بره أي transaction** (شبكة)، وبعدين في transaction واحدة: قفل المجموعة، وقفل الـ run، والتأكد إنه لسه `pending`، وتسجيل التعريف، والتزامن، والتوسيع، والإدراج.
- **كل 5 ثواني:** `RunJob` حالته `active` والـ job بتاعه خلص من أكتر من 5 ثواني بيتعمله `JobFinished` من جديد، وrun `pending` عدّى دقيقتين من غير تعريف ومن غير discovery job بيفشل `DEFINITION_TIMEOUT`، وrun `running` مافيهوش job شغّال بيتقيّم تاني (مثلًا بعد `JOB_QUEUE_FULL`)، والـ runs `queued` اللي مجموعتها فضيت بتبدأ، وطلبات الإلغاء اللي ماوصلتش للـ jobs بتتبعت.
- **القيام بعد وقوع السيرفر:** كل خطوة في transaction واحدة، فالوقوع قبل الـ commit معناه إن الـ job لسه شغّال والـ `RunJob` لسه `active`، والـ runner بيعيد `Complete` أو الـ lease بيخلص ويتعاد زي M3. الاختبار بيعمل sink بيرجّع error بعد ما `Pipelines` يخلص شغله (نفس أثر الوقوع قبل الـ commit) ويتأكد إن مفيش حاجة اتسجّلت، وبعدين الـ run بيكمّل.

### التزامن (`concurrency`)
- المجموعة لكل مشروع، وبتتقيّم مرة واحدة وقت البداية. من غير `cancel_in_progress` الأحدث بيستنى `queued` بالترتيب، ومعاه الجديد بيطلب إلغاء الأقدم على نفس الـ ref بس (الـ `waiting` بيتلغي على طول، والشغّال بيوصله في الـ heartbeat).
- «أعد تشغيل الفاشل» مابيستناش المجموعة: الـ run كان بدأ قبل كده.

### الأدوار والـ audit
- `run.read` لكل الأدوار، و`run.trigger` من Developer، و`run.cancel` و`run.approve` من Maintainer، وكلهم على المشروع بالـ bindings (`policy.Project`).
- **routes الـ jobs على مستوى المؤسسة** (`/organizations/{id}/jobs/{jobId}` والإلغاء والـ logs والـ stream): لو الـ job جاي من run، الـ controller بيطلب نفس الإجراء بالدور على مشروع الـ run كمان (`Guard.AuthorizeProject`). فالـ binding اللي بيقلّل الدور بيتطبّق هنا، واللي بيرفعه بيشتغل من routes الـ runs. والـ stream بيعيد الفحص ده في كل إعادة تفويض.
- **الـ audit بعد الـ commit:** `project.run_created` (manual وapi)، و`project.run_started` (لما الـ worker يدرج أول jobs، والفاعل هو اللي عمل الـ trigger أو مفيش)، و`project.run_cancel_requested`، و`project.run_rerun_requested`، و`project.run_approved`. والـ jobs اللي بتتدرج بعد كده جوه `JobFinished` جزء من نفس الـ run المسجّل (ADR-0070: «الـ run مش كل job»). والـ TargetType `run`.
- **`Core/CommitStatus`:** `Pipelines` بيسجّل الحالة المطلوبة (`pending` و`running` و`success` و`failed` و`canceled`) في نفس الـ transaction، والإرسال من worker بتاع `Sources`. Git العام مابيبعتش حاجة (القرار 14)، فمابنكتبش له صفوف أصلًا.

### الـ triggers
- `push` و`tag` و`pull_request` من الـ webhook، و`manual` من جلسة و`api` من توكن على نفس الـ route (`POST .../runs`). الـ ref بيتحقق بـ `internal/gitref` (فرع أو tag بس، و`main` بيبقى `refs/heads/main`)، والـ inputs 20 بالكتير، نص أو boolean.
- الـ run بيكمّل بس لو الـ trigger في `on:` والفلاتر طابقت، وغير كده `skipped` بـ `TRIGGER_NOT_ENABLED`. والملف مش موجود `skipped` بـ `DEFINITION_NOT_FOUND` ومن غير check. والـ inputs بتتحقق قدّام `on.manual.inputs` وقت البداية (`INPUTS_INVALID`)، والقيم الافتراضية بتتسجّل على الـ run.
- **فلاتر `paths`:** الـ `Trigger` مافيهوش الملفات اللي اتغيّرت لحد M4.5 وM4.6، فالفلتر ده مابيتطبّقش دلوقتي (بيطابق كل push). ده تشغيل زيادة مش ثغرة، ومكتوب في الوثيقة.

### المصطلح
- «Run» في الواجهة بقى «تنفيذ» (الجمع: التنفيذات)، و«Job» بيفضل «تشغيل». اتضاف في `Plan/Glossary.md`.

## بعد المراجعة الأمنية المستقلة
- **(High) run مسموم كان بيوقف بدء كل الـ runs:** matrix بـ 16 مفتاح وقيم 128 حرف كان بيعمل اسم `RunJob` حوالي 4,700 حرف، والـ CHECK (1024) بيرفضه، و`StartQueued` كان بيرجّع الـ error فكل pass بيقف كل 5 ثواني. الإصلاح على مستويين: (أ) `internal/pipelinespec` بيرفض وقت الـ validation أي تركيبة اسمها (القيم) أطول من 512 حرف بالكود الثابت `MATRIX_NAME_TOO_LONG` (فـ `dhow lint` بيقوله كمان)، و(ب) كل loop في الـ worker والـ reconciler بيعزل العنصر اللي فشل: خطأ بيانات أو constraint من Postgres (الفئات 22 و23) أو ملف مايتقريش بيخلّي الـ run ده بس `failed` بـ `RUN_EXPANSION_FAILED` (أو `RUN_STATE_INVALID`)، والخطأ العابر (شبكة أو قاعدة بيانات) بيتسجّل في الـ log ويستنى الـ pass الجاي، والـ pass عمره ما بيقف.
- **(Medium) تثبيت الصور كان بيقفل الـ worker الوحيد:** ميزانية 30 ثانية للـ run كله، و4 طلبات بالتوازي، وكل صورة بعد ما تتكمّل (`alpine:3` و`docker.io/library/alpine:3`) بتتسأل مرة واحدة، وقراية الملف من المزوّد بمهلة 30 ثانية. والـ reconciler بقى على goroutine لوحده غير اللي بيبدأ الـ runs. و`POST .../runs` محدود 60 لكل مستخدم و240 لكل مشروع في 10 دقايق، بنفس limiter بتاع اتصالات `Sources`.
- **(Medium) runs في الطابور كانت ممكن تجوع:** الـ runs `queued` المرشّحة هي أول run في كل (مشروع، مجموعة) بس، ومجموعته مافيهاش run شغّال، فطابور طويل في مجموعة واحدة مابيملاش الـ batch. والـ run `running` اللي اتقيّم ومااتحرّكش بيتعلّم `UpdatedAt` جديد، فبيروح آخر الطابور.
- **(Low) عنوان التوكن بتاع الـ registry:** لازم `https` ومن غير user، وعلى نفس host الـ registry أو `auth.docker.io` أو `ghcr.io` بس، وعلى 443 أو منفذ الـ registry نفسه، وطلب التوكن بيعدّي على `netguard` من غير `DHOW_GIT_ALLOW_PRIVATE_CIDRS` خالص.
- **(Low) الـ outputs:** بتتقص لحد 60 KiB من الـ JSON (تحت الـ CHECK بتاع 64 KiB) بترتيب ثابت، واللي بيتشال بيتسجّل `OUTPUTS_TRUNCATED` على الـ `RunJob`، ومابيفشّلش `FinishRunJob` ولا الـ transaction بتاع `Jobs`.
- **الـ sha في التشغيل اليدوي والـ API:** لازم يبقى sha وصل بـ webhook لنفس المستودع ونفس الـ ref من run موثوق (`push` أو `tag`، مش fork)، وغير كده `422 SHA_NOT_ON_REF`. ومن غير `Sha` الـ run بياخد آخر sha زي ده للـ ref، ولو مفيش `422 REF_HEAD_UNKNOWN`. كده عضو عنده `run.trigger` مايقدرش يشغّل commit من fork أو commit قديم مش على الفرع. **والـ checkout بتاع M4.7 لازم يتأكد كمان إن الـ sha موجود في تاريخ الـ ref** (القرار 11) قبل أي خطوة.
- **الـ dedupe وأصل الـ PR:** مفتاح التكرار بقى (المستودع، الـ ref، الـ sha، نوع الحدث، `IsFork`)، و`checkTrigger` بيطلب `refs/heads/*` للـ push و`refs/tags/*` للـ tag و`refs/pull/*` أو `refs/merge-requests/*` للـ PR. و`Trigger.IsFork` اتشال واتعمل بداله `Trigger.Origin` (`PullOriginUnknown` و`PullOriginSameRepository` و`PullOriginFork`)، والقيمة الافتراضية (unknown) بتتعامل كـ fork، فالمزوّد اللي ينسى يحدد الأصل بيقفل مش بيفتح.
- **`cancel_in_progress` على نفس الـ ref بس:** بيلغي الـ runs الأقدم في المجموعة اللي على نفس الـ ref، والـ refs التانية بتفضل والجديد بيستنى `queued`. كده Developer مايقدرش يلغي runs فرع تاني بمجموعة مشتركة.
- **الخطوة التانية لـ `run.approve`:** بتفضل زي ما هي، لأنها مطابقة لـ ADR-0054 (الموافقة بتشغّل كود من بره المؤسسة، فهي من الإجراءات الحساسة).
- **أخطاء الـ JSON المخزّن:** فك `"Inputs"` و`"Outputs"` و`"Issues"` و`"ImagePins"` و`"Matrix"` بقى بيرجّع error ملفوف (`errStoredState`)، والـ run بيفشل مقفول بـ `RUN_STATE_INVALID` بدل ما يكمّل بقيمة فاضية.

## الربط بالـ checkout بعد دمج M4.7
- **الـ `CheckoutSeam` اتشال**، و`Pipelines` بقى بيبني الـ checkout بنفسه من صف المستودع (`GetCheckoutRepository`): `repository_url` هو الشكل الموحّد من `gitsource.Parse`، و`ref` و`sha` من الـ run، و`transport` من العنوان، و`known_host` هو `KnownHostsPattern()` + الـ host key اللي الأدمن أكّده، و`depth` واحد. ولـ SSH `credential_secret` = `dhow.git.checkout` ومعاه SecretRef كملف `git-credential` من غير `step_indexes`. HTTPS العام من غير credential. ومستودع SSH مالوش host key متأكد يبقى `RunJob` فاشل بـ `REPOSITORY_UNAVAILABLE`.
- **الـ credential عمره ما بيتكتب في الـ spec:** الـ spec المتخزّن فيه اسم السر بس. وقت `Start`، `Jobs` بينادي `SecretResolver`، و`App` بيلفّه بـ resolver (`App/Checkout.go`) بيقبل `dhow.git.checkout` في `Accepts`، وفي `Resolve` بيسأل `Pipelines` عن مشروع ومستودع الـ job (`Jobs.OriginId` ← `RunJobs` ← `PipelineRuns`) وبعدين `Sources.CheckoutCredential` بيفك الـ deploy key المختوم بالـ label بتاعه. القيمة بتتختم للـ runner بـ HPKE على مفتاحه (ADR-0045) وبتتمسح من الذاكرة بعدها، والـ runner بيحطها في ملف لخطوة الـ checkout بس. اختبار بيفك الـ bundle ويلاقي المفتاح، وبيدوّر على نص المفتاح (والسطور بتاعته) في `"Jobs"` و`"LogChunks"` و`"AuditEvents"` و`"PipelineRuns"` و`"RunJobs"` وlog السيرفر وlog الـ runner وملفاته: مش موجود.
- **الـ discovery job:** checkout للـ sha بالظبط (للـ fork: الـ base ref والـ base sha)، وخطوة واحدة في صورة الـ checkout (`buildinfo.CheckoutImage()`، نفس الـ digest اللي الـ runner بيستخدمه) بتطبع الملف base64 في الـ stdout بسطور `dhow-definition:` وبتكتب `dhow_found` و`dhow_sha256` في الـ outputs. السيرفر بيقرا سجل الخطوة صفر في آخر محاولة (512 KiB بالكتير)، ويفك، ويتأكد إن الـ sha256 مطابق، وغير كده `DISCOVERY_FAILED`. ليه السجل؟ الـ outputs حدها 1 KiB للقيمة و64 مفتاح، والملف لحد 256 KiB. الرابط الرمزي أو المسار بره الـ workspace بيتعامل كملف مش موجود. وموارده صغيرة (0.5 CPU و256 MiB و512 MiB قرص و256 pid و5 دقايق).
- **`CHECKOUT_UNAVAILABLE`** بقى معناه سيرفر اتبنى من غير صورة الـ checkout بس. أما لو مفيش runner عنده `checkout`، فالـ job بيستنى في الطابور زي أي job فيه checkout (فلتر الـ lease بتاع M4.7) لحد مهلة الطابور، وبعدها الـ run بيفشل `DISCOVERY_FAILED`.
- **الـ ancestry (بيغيّر سطر في ADR-0075 §2):** الـ checkout بتاع M4.7 كان لو الـ sha مش في تاريخ الـ ref بيجيبه بالـ id، يعني commit من فرع تاني كان ممكن يشتغل على `main`. دلوقتي لو الـ sha مش رأس الـ ref، الـ runner بيجيب تاريخ أعمق للـ ref (`--depth=65536`) وبيتأكد بـ `git merge-base --is-ancestor` من خلال argv `gitsource` المقوّاة، ومن غير ما يجيب بالـ id أبدًا، والفرع الممسوح أو الـ sha اللي بره التاريخ بيطلّع `CHECKOUT_REF_MOVED`. والـ discovery job اللي بيفشل كده بيخلّي الـ run `failed` بـ `SHA_NOT_ON_REF` قبل أي job تاني. اختبار بـ git حقيقي: commit على فرع تاني موجود على السيرفر مابيتعملوش checkout على `main`.
- **الـ outputs:** `CompleteRequest.outputs` (بعد الـ masking بتاع الـ runner، ولو الـ job نجح بس) بيوصل `RunEvents.Finished.Outputs`، و`Pipelines` بيخزّنه بحد 60 KiB وبيدّيه لـ `needs.<job>.outputs`. ومفاتيح `dhow_found` و`dhow_sha256` مابتتخزّنش. والـ outputs مش بتظهر في أي response.
- **الإضافات:** `pipelinespec` بقى بيحط `Step.plugin` (الصورة والـ inputs الثابتة)، وكل سر في `with:` بقى SecretRef بـ `as_env: DHOW_INPUT_<NAME>` محصور في رقم خطوة الإضافة دي بس، ومن غير ما يبقى input عادي كمان. والـ inputs اللي بتعبيرات بتتملي وقت الإدراج، وبعدها `Enqueue` بيعدّي الـ spec النهائي على `jobspec.Prepare` (ملاحظة L1). والـ features: `Enqueue` بيضيف `checkout` و`plugins` من الـ spec نفسه، فكل نداء من `Pipelines` بيعدّي عليه.

## اللي كان متأجّل لـ M4.7 (اتعمل في القسم اللي فوق)
- ~~تنفيذ الـ checkout والـ discovery job~~، و~~التأكد إن الـ sha في تاريخ الـ ref~~، و~~الـ outputs من `CompleteRequest`~~.
- متغيرات `DHOW_*` (الـ sha والـ ref والـ run والمشروع) في `env` الـ job: `jobspec.Validate` لسه بيرفض البادئة دي، فبتفضل للمرحلة اللي بتحدد شكلها (M4.10 أو M5).
- ~~خطوات `uses:` بتفشل لحد ما `Step.plugin` ييجي~~ (اتعمل). والـ jobs اللي فيها `artifacts` أو `cache` بتطلب الميزة دي وبتستنى runner بيعلنها (M4.9).

## البدائل اللي اترفضت
- **نخزّن الـ plan كـ jsonb:** محتاج serialization لـ declarations الـ CEL الداخلية، أو تقييم CEL من غير الـ compile. النص والـ digest أبسط وبيثبت المعنى.
- **نبدأ الـ run اللي بعده من جوه `JobFinished`:** بيقفل run تاني وهو ماسك run، وده deadlock مع البداية اللي بتقفل المجموعة. الـ worker كل ثانية أبسط.
- **نثبّت الصور من الـ runner:** كل job وكل إعادة كان ممكن يسحب digest مختلف لو الـ tag اتحرك، وده عكس القرار.
- **نشيل `POST /jobs` من الـ spec:** كان هيكسر الـ route-table والاختبارات اللي بتعتمد عليه، و`404` في الإنتاج بيحقق نفس الهدف.

## العواقب
- Git العام مابيشغّلش pipelines لحد ما M4.7 يدمج، والـ runs بتاعته بتقول ده صراحة (`CHECKOUT_UNAVAILABLE`).
- كل `JobFinished` بيعمل compile للملف من جديد (ميلي ثواني لملف عادي، وبحد 256 KiB وميزانية CEL ثابتة).
- أي run جديد بيستنى ثانية بالكتير قبل ما يبدأ (الـ worker)، وأي إصلاح من الـ reconciler بيستنى 5 ثواني.
