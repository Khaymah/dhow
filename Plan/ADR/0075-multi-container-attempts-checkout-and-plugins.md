# ADR-0075: الـ attempt بأكتر من حاوية، والـ checkout المدمج، والإضافات (M4.7)

- **الحالة:** مقبول. القرارات 3 و4 و5 مفوَّضة في [`M4-Pipelines.md`](../M4-Pipelines.md)، ومعاها ملاحظات المراجعة الأمنية لـ M4.4 وملاحظة M4.3 عن الـ features. والباقي تفاصيل تنفيذ مكتوبة هنا.
- **التاريخ:** 2026-11-02
- **بيكمّل:** [ADR-0043](0043-job-sandbox-rootless-podman.md) (الـ sandbox بـ Podman rootless) و[ADR-0045](0045-secrets-transport-and-masking.md) (نقل الأسرار والـ masker) و[ADR-0057](0057-egress-proxy-implementation.md) (الـ egress) و[ADR-0069](0069-pipeline-spec-parser-and-validator.md) و[ADR-0072](0072-webhooks-and-git-credentials.md) (`internal/gitsource`).

## السياق
- في M3 الـ attempt كان حاوية واحدة من صورة الـ job، و`--userns=auto:size=65536` بيدّي كل حاوية نطاق uid لوحدها، والـ workspace tmpfs جوه الحاوية نفسها.
- M4 محتاج أكتر من صورة في نفس الـ job: الـ checkout في صورة بتاعتنا فيها git، وكل إضافة (`uses:`) في صورتها، وخطوات `run` في صورة الـ job. والملفات اللي خطوة بتكتبها لازم الخطوة اللي بعدها تلاقيها بنفس المالك.
- الـ credential بتاع الـ checkout مايوصلش لخطوات المستخدم، والـ runner القديم (M3) مايستلمش job فيه checkout.
- `internal/gitsource` من M4.4 بيبني أوامر git بس، ومحدش بيشغّلها لسه. والمراجعة الأمنية لـ M4.4 طلبت تقوية إلزامية قبل ما حد يشغّلها.

## الـ spike (go/no-go)
**المطلوب:** workspace مشترك بين حاويات متتابعة بنفس الـ uid map، بطريقتين، والـ escape والـ limits suites خضرا، وحد القرص بيتحسب صح، ومفيش volume فاضل بعد الهدم.

**البيئة:** الجهاز ده من غير Podman وعلى cgroup v1 (والـ controllers مربوطة بـ v1، ومش ممكن نحوّلها من غير ما نوقف الجهاز المشترك). فاشتغلنا بـ Podman 5.6.2 rootless (مستخدم عادي بمليون subuid) جوه حاوية Docker privileged. السكربتات والـ logs في `/tmp/claude-0/m47/` (`spike2.sh` و`spike2-run3.log` و`iso-full.log` و`iso-checkout1.log`).

**الطريقتين:**
- **أ. pod لكل attempt بـ `--userns=auto` على الـ pod**، والحاويات بتنضم للـ pod فبتاخد نفس الـ user namespace.
- **ب. uid map صريح من نطاق الـ runner** (`--uidmap`/`--gidmap` لكل حاوية بنفس الـ slot)، والـ workspace tmpfs بيركّبه الـ runner نفسه.

**اللي لقيناه:**
- **volume بتاع Podman من نوع tmpfs مش ينفع للاتنين:** Podman بيركّبه لما حاوية تبدأ وبيفكّه لما تقف، فالملفات بتضيع بين الحاويات المتتابعة. وكمان `volume rm` فشل بـ `device or resource busy` وساب `_data` وmount فاضلين. فالـ workspace في الاتنين بقى tmpfs بيركّبه الـ runner بـ `podman unshare mount -t tmpfs -o size=…,nosuid,nodev,mode=0755,uid=…,gid=…` جوه الـ user namespace بتاع Podman، وبيتعمله bind في كل حاوية.
- **أ:** اشتغلت بعد تعديلين: الـ workspace مايتعملهوش `:U` على الـ pod (لأن `:U` بيتطبّق تاني على كل حاوية بتنضم وبيرجّع كل الملفات لـ root، فملف uid 1000 بقى 0:0)، والـ chown بيحصل مرة واحدة بعد ما الـ infra تبدأ. بس الحاويات بقى `UsernsMode=container:<infra>` من غير annotation الـ `auto`، فالتحقق بعد الإنشاء بتاع ADR-0043 لازم يتكتب من الأول، والـ infra محتاجة `catatonit` على جهاز الـ runner، وفيه process زيادة طول الـ attempt.
- **ب:** اشتغلت من غير أي تعديل في باقي التقوية: نفس flags بتاعة M3، بس `--uidmap 0:<slot>:65536` بدل `--userns=auto`. والتحقق بعد البدء بقى أقوى: `/proc/<pid>/uid_map` لازم يبقى بالظبط نطاق الـ slot.
- **للمقارنة:** `--userns=auto` لكل حاوية من غير pod ادّى نفس النطاق للحاويتين بالصدفة بس، لأن الأولى اتمسحت قبل التانية. ومع attempts شغّالة مع بعض مفيش ضمان.

**النتيجة على Podman 5.6.2 (cgroup v1، والتحقق من حدود الـ cgroup مقفول في نسخة محلية بس لأن Podman rootless على v1 بيتجاهل الحدود):**

| الشرط | أ (pod) | ب (map صريح) |
|---|:-:|:-:|
| ملف root وملف uid 1000 من الحاوية الأولى بنفس المالك ويتكتب فيهم من التانية | ✅ | ✅ |
| نفس الـ uid map في كل حاويات الـ attempt | ✅ | ✅ |
| حد القرص بيتحسب مرة واحدة للـ attempt (`ENOSPC` عند الحد) | ✅ | ✅ |
| `nosuid,nodev`، وملف setuid ماياخدش صلاحيات، و`mknod` مرفوض | ✅ | ✅ |
| مفيش mount ولا مجلد ولا volume فاضل بعد الهدم | ✅ | ✅ |
| escape suite الحالي (21 من 23 اختبار في suite الـ isolation) | — | ✅ |
| حدود الذاكرة والـ CPU | ماتجرّبش هنا | ماتجرّبش هنا |

- اختبارات `TestMemoryAboveTheLimitIsOOMKilled` و`TestCPUIsThrottledToTheLimit` فشلت هنا لأن Podman على cgroup v1 بيتجاهل الحدود، مش بسبب التغيير. وحد الـ pids والقرص اشتغلوا.
- **القرار: go بالطريقة ب.** نفس التقوية ونفس التحقق بتاع M3 من غير إعادة كتابة، ومن غير اعتمادية `catatonit`، والتحقق من الـ map بقى بالظبط. والشرط الوحيد اللي ماتأكدش هنا (حدود الـ cgroup v2) بيتأكد في job `isolation` في الـ CI على `ubuntu-26.04` قبل الدمج، وده اللي بيقرر.

## القرار

### 1. القرار 4: الـ attempt = workspace واحد ونطاق uid واحد وحاويات بالتتابع
- **النطاق:** الـ driver بيقرا نطاق الـ subuid بتاع الـ runner من `podman unshare cat /proc/self/uid_map`، وبيقسّمه slots كل واحد 65536. كل attempt بياخد slot لحد ما الـ workspace بتاعه يتهدم. مفيش `--userns=auto` تاني، فالـ driver هو المصدر الوحيد للتوزيع، ولو الـ slots خلصت الـ attempt بيفشل `SANDBOX_FAILED`.
- **الـ workspace:** `<attempt>/workspace` tmpfs بحجم القرص بتاع الـ job و`nosuid,nodev` ومالكه root الـ slot، بيتركّب في الـ namespace بتاع Podman (مش ظاهر للـ host). كل حاوية بتاخده bind على `/workspace`، والتحقق بعد الإنشاء بيرفض أي tmpfs تاني على `/workspace` أو مصدر غير مجلد الـ attempt.
- **التركيب من غير `mount(8)` (بعد CI الـ PR):** الـ unit المقوّاة بتقفل `/usr/bin/mount` و`/usr/bin/umount` بـ `InaccessiblePaths` عن قصد (setuid root، والـ unit ماتقدرش تحط `NoNewPrivileges` بسبب `newuidmap`)، فـ `podman unshare mount` فشل تحتها بـ `exec: "mount": executable file not found`، والـ spike ماشافش ده لأنه ماكانش تحت الـ unit. القرار: الـ unit تفضل زي ما هي، والتركيب والفك بيحصلوا بإن `podman unshare` يشغّل `dhow-runner` نفسه (المسار اللي اتحدد عند البدء) بأمر داخلي مخفي: `__workspace-mount <attempts> <workspace> <size> <uid>` و`__workspace-umount <attempts> <workspace>`. الأمر بينادي `unix.Mount` بـ `MS_NOSUID|MS_NODEV` و`size,mode=0755,uid,gid` ثابتين، أو `unix.Unmount` بـ `MNT_DETACH`، وبالـ argv بس. وبيرفض أي مسار مش `<attempts>/<attempt>/workspace` بالظبط (مسارات مطلقة ونضيفة ومن غير symlinks، والـ driver بيحلّها قبل ما يبعتها)، وأي حجم أو uid أو وسيط زيادة، وبيرفض يشتغل لو مش جوه user namespace (يعني مش root الحقيقي)، فمابيدّيش أي صلاحية زيادة على المستخدم اللي نادى. `rm` و`chown` و`cat` جوه `podman unshare` زي ما هم. والحماية من الرجوع: اختبار بيفشل لو الـ driver شغّل `mount` أو `umount`، والمرور التاني لـ job `isolation` تحت `sandbox-properties.sh`، وjob التنصيب اللي بيشغّل job تحت الـ unit الحقيقية.
- **التحقق بعد البدء:** `uid_map` و`gid_map` لازم يبقوا سطر واحد `0 <بداية الـ slot على الـ host> 65536`، زيادة على الفحوص بتاعة M3 (مش الـ runner نفسه، ومش root، ومش متداخل مع sandbox تاني شغّال).
- **الهدم:** الـ workspace مابيتفكّش ولا الـ slot بيرجع طول ما فيه حاوية من الـ attempt عايشة. والـ cleanup وقت البدء بيفك أي workspace فاضل من crash قبل ما يمسح المجلدات.
- **الأسماء:** الحاوية الأولى `dhow-<attempt>` زي M3، والباقي `dhow-<attempt>-<n>`، وكل واحدة ليها control dir لوحدها.
- **الترتيب:** checkout (لو موجود)، وبعده كل مجموعة خطوات `run` ورا بعض في حاوية واحدة من صورة الـ job، وكل إضافة في حاويتها. الحاوية بتتهدم قبل اللي بعدها ما تتعمل، والإلغاء والـ fencing بيقتلوا الحاوية الحالية زي M3.
- **الذاكرة:** حد الذاكرة لكل حاوية لسه الذاكرة زائد القرص زي M3. صفحات الـ tmpfs اللي حاوية خلصت كتبتها بتتحسب على الـ cgroup الأب بعد ما الحاوية تتمسح، فأسوأ حالة للـ attempt الذاكرة زائد ضعف القرص. ده مكتوب ومقبول لحد ما نقيسه في M9.

### 2. القرار 3: الـ checkout خطوة مدمجة في صورة `dhow-git`
- **الصورة:** `deploy/dhow-git` من Alpine 3.22 متثبّتة بالـ digest، فيها `git` و`openssh-client` بس، من غير ملفات setuid ولا templates بتاعة git ولا `ssh_config` ولا `gitconfig` للنظام. `release.yml` بيبنيها لـ amd64 وarm64، ويوقّعها بـ cosign، ويتأكد من التوقيع، ويكتب الـ provenance. والـ digest بيتكتب جوه `dhow-runner` وقت الـ build (`CHECKOUT_IMAGE`)، و`DHOW_RUNNER_CHECKOUT_IMAGE` بيغيّره بالـ digest بس.
- **إعادة البناء بنفس النتيجة (بعد المراجعة الأمنية، S2):** كل حزمة apk بتضيفها الصورة متثبّتة بنسختها بالظبط (`git=2.49.1-r0` و`openssh-client-default=10.0_p1-r10` واعتمادياتهم)، على Alpine المتثبّت بالـ digest. **التحديث:** غيّر `ARG BASE` للـ digest الجديد لو فيه، وشغّل `make git-image-pins`، وهو بيطبع النسخ اللي `git` و`openssh-client` بيتحلّوا ليها على الـ base ده بصيغة `name=version`، والصقها مكان القايمة في `deploy/dhow-git/Dockerfile`، وابني بـ `make git-image`. Alpine بيشيل النسخ القديمة من الـ mirrors، فلو البناء فشل بـ `unable to select packages` يبقى لازم تحديث بنفس الطريقة، ومعناه إن فيه تحديث أمني غالبًا.
- **التشغيل:** حاوية بنفس التقوية، والـ egress بتاعها host المستودع وبورته بس (مش egress الـ job). الـ agent بيشغّل `dhow-runner __checkout` بـ `RunExec`، وهو بيشغّل git بـ argv من `internal/gitsource`: `init`، و`fetch --depth=<n> --no-tags -- <url> <ref>`، والتأكد إن `FETCH_HEAD` هو الـ sha أو إن الـ sha في تاريخه، ولو لأ (أو الفرع اتمسح) `fetch` بالـ sha، ولو فشل `CHECKOUT_REF_MOVED`، وفي الآخر `checkout --detach <sha> --`.
- **تقوية git (ملزمة من مراجعة M4.4):**
  - كل أمر بـ `cmd.Env` صريح: `PATH` ثابت، و`HOME` فاضي، و`GIT_CONFIG_NOSYSTEM=1`، و`GIT_CONFIG_GLOBAL=/dev/null`، و`GIT_TERMINAL_PROMPT=0`، و`GIT_LFS_SKIP_SMUDGE=1`، و`GIT_SSH_VARIANT=ssh`، ومفيش أي `GIT_*` أو `SSH_*` موروث.
  - `--literal-pathspecs` و`--no-replace-objects` قبل الأمر الفرعي، و`-c core.fsmonitor=false` و`-c credential.helper=` و`-c init.templateDir=` جنب الإعدادات القديمة.
  - **`core.sshCommand`:** القيمة الفاضية (`-c core.sshCommand=`) بتخلّي git 2.43 يحاول يشغّل أمر فاضي ويفشل (`cannot run : No such file or directory`)، ومن غير قيمة خالص بيوقف (`missing value`). فبقت مسار ثابت: `/dev/null` لكل الأوامر (أي محاولة SSH بتفشل)، والـ checkout بـ SSH بس بياخد مسار الـ helper `/run/dhow/bin/git-ssh`. git بيشغّله من خلال `sh -c`، بس النص ثابت ومن غير أي قيمة من المستخدم، والـ helper بيتحقق من الـ argv بنفسه.
  - **الـ helper بتاع SSH:** بيقبل بس `[-o SendEnv=GIT_PROTOCOL] [-p <بورت المستودع>] <user@host> "git-upload-pack '<path>'"` لنفس المستودع، وبيشغّل `ssh -F /dev/null -o StrictHostKeyChecking=yes -o UserKnownHostsFile=<سطر الـ host المتثبّت بس> -o GlobalKnownHostsFile=/dev/null -o IdentitiesOnly=yes -o IdentityFile=<ملف السر>` ومعاه `BatchMode` ومن غير agent ولا forwarding، و`ProxyCommand` هو الـ helper `connect` بتاع M3. ولو ssh قال إن الـ host key اتغيّر: `HOST_KEY_MISMATCH`.
  - **HTTPS:** `GIT_ASKPASS` لـ helper بيقرا التوكن من ملف السر، والـ username `x-access-token`، والـ proxy بتاع الـ egress في `HTTPS_PROXY`.
  - الاختبارات بتشغّل git حقيقي ببيئة فيها `GIT_SSH_COMMAND` و`GIT_DIR` و`GIT_CONFIG_*` متزرعين، ومستودع إعداداته فيها `core.sshCommand` و`core.fsmonitor` و`core.hooksPath`، وsubmodule بـ `file://`: ولا حاجة منهم بتتنفّذ.
- **الـ credential:** السر المحجوز `dhow.git.checkout`، كملف `git-credential` لخطوة الـ checkout بس. `jobspec` بيرفضه من غير checkout، أو كمتغيّر بيئة، أو مربوط بخطوة، والـ executor مابيدّيهوش لأي خطوة مستخدم. السيرفر بيولّده وقت `Start` (M4.5 وM4.8).
- **الأكواد:** `CHECKOUT_FAILED` و`CHECKOUT_REF_MOVED` و`HOST_KEY_MISMATCH` و`CHECKOUT_DISK_FULL` أسباب فشل جديدة في الـ proto.

### 3. القرار 5: الإضافات صور حاويات بـ manifest في label
- **الـ label:** `io.dhow.plugin.manifest` على الصورة، JSON بمفاتيح PascalCase:
  ```json
  {"Version": 1, "Entrypoint": ["/usr/local/bin/notify", "--quiet"],
   "Inputs": {"channel": {"Required": true}, "message": {"Default": "done"}}, "Outputs": ["url"]}
  ```
  `Entrypoint` لازم يبدأ بمسار مطلق، و32 عنصر بالكتير، والـ inputs بنفس شكل أسماء `with:`، و64 KiB للـ label كله.
- **التحقق:** الـ runner بيقرا الـ label بعد السحب بالـ digest وقبل أي تشغيل. input مش معلن أو input مطلوب مش موجود: `PLUGIN_INPUT_INVALID`. label مش موجود أو مش صح: `PLUGIN_MANIFEST_INVALID`. والسيرفر بيتحقق من الشكل بس (digest، و32 input، و32 KiB للقيمة).
- **التشغيل:** حاوية لوحدها بنفس التقوية وعلى نفس الـ workspace، والـ egress بتاع الـ job، والـ inputs كـ `DHOW_INPUT_<NAME>`، والـ entrypoint بـ `RunExec` من غير shell (صورة من غير `/bin/sh` بتشتغل).
- **أقل صلاحيات (بعد المراجعة الأمنية، MEDIUM):** الإضافة كود من بره، فبتاخد بس: الـ inputs، و`DHOW_OUTPUT`، ومتغيرات الـ proxy لو الـ job ليه egress، و`env` الخطوة نفسها، والأسرار اللي `step_indexes` بتاعها فيه رقم الخطوة دي. السر في `with:` بيوصل كـ SecretRef `as_env: DHOW_INPUT_<NAME>` محصور في خطوة الإضافة دي بس، و`jobspec` بيرفضه لو مش محصور فيها أو لو نفس الـ input موجود كقيمة عادية. ومابتاخدش `env` الـ job، ولا أسرار الـ job كله، ولا `DHOW_FENCING_TOKEN` و`DHOW_JOB_ID` و`DHOW_ATTEMPT_ID` و`DHOW_STEP_INDEX`. خطوات `run` بتفضل زي M3.

### 4. الـ outputs
- كل خطوة (`run` أو إضافة) بتلاقي `DHOW_OUTPUT`، وبتكتب فيه سطور `key=value`. الـ agent بيقرا الملف بعد الخطوة (ملف عادي، 64 KiB بالكتير، من غير ما يتبع links) ويمسحه.
- الـ runner بيقبل 64 مفتاح و1 KiB للقيمة، والإضافة مابتكتبش غير المفاتيح اللي في الـ manifest. أي حاجة غير كده بتفشّل الخطوة. وبيبعتها في `CompleteRequest.outputs` لو الـ job نجح بس.
- **الإخفاء على المجموعة (بعد المراجعة الأمنية، LOW):** القيم بتتخزّن زي ما هي لحد `Complete`، وساعتها الـ masker بيعدّي على كل قيمة لوحدها، وعلى كل سلسلة مفاتيح ورا بعض بترتيب الأسماء، وعلى كل زوج بأي ترتيب. سر جوه قيمة بيبقى `***`، وسر بيظهر بس لما القيم تتجمع بيشيل كل المفاتيح اللي بيعدّي عليها. الحد: سر أقصر من `masking.MinLength` (4) مابيتخفاش أصلًا، وتجميعات بترتيب تاني غير اللي فوق (تلات قيم بترتيب عشوائي مثلًا) مش متغطية.

### 5. الـ features من التسجيل والدخول الموقّع (ملاحظة M4.3)
- `RegisterRequest.features` و`AuthenticateRequest.features`، والقيم `checkout` و`plugins` (و`artifacts` و`cache` في M4.9).
- السيرفر بيخزّنها في `"Runners"."Features"` (migration `00045`). التسجيل بيحطها، وكل دخول بيحطها تاني، والـ runner بيوقّع الدخول بمفتاحه على رسالة فيها الـ features (`dhow/runner-auth/v2`). الدخول من غير features بيفضل بتوقيع M3 وبيفضّي القايمة، فـ runner رجع لنسخة M3 مابياخدش jobs جديدة فيها checkout.
- `LeaseRequest` مافيهوش features. توكن دخول مسروق مايقدرش يغيّرها، لأن مفيش طريقة غير الدخول الموقّع.
- الـ lease بياخد بس jobs `"RequiredFeatures"` بتاعتها جزء من features الـ runner. و`Enqueue` بيضيف الـ features اللي الـ spec محتاجها (checkout وplugins) على اللي اتطلب، فـ pipeline ناسي يطلبها مايقدرش يبعت checkout لـ runner قديم.
- الـ runner بيعلن `plugins` دايمًا، و`checkout` لو عنده صورة checkout.

### 6. التحقق من الـ `JobSpec` النهائي (ملاحظة L1 من مراجعة M4.1)
- `Core/JobQueue.Enqueue` بيعدّي الـ spec النهائي (بعد ما `EvalValue` يملا `env` و`with`) على `jobspec.Prepare`، يعني `Validate` كامل، قبل الإدراج. واختبارات بتتأكد إن قيمة فيها NUL أو أطول من الحد، أو إضافة مش متثبّتة، أو checkout بعنوان أو ref غلط، بتترفض.
- `jobspec` بيتحقق من الحقول الجديدة: العنوان لازم يبقى الشكل الموحّد اللي `gitsource` بيقبله، والـ transport مطابق، والـ ref والـ sha، والعمق من 1 لـ 65536، وسطر `known_hosts` للـ host ده بالظبط لو SSH، والسر المحجوز. ومنطق الـ host اتنقل لـ `internal/hostname` عشان `jobspec` يستورد `gitsource` من غير دايرة.

## اللي اترفض
- **pod بـ `--userns=auto`:** اشتغل، بس محتاج إعادة كتابة التحقق بعد الإنشاء و`catatonit` على كل جهاز، ومن غير مكسب على الـ map الصريح.
- **volumes بتاعة Podman:** بتتفك بين الحاويات وبتسيب بقايا.
- **git في صورة الـ job:** صورة المستخدم مش محتاجة git، والـ credential كان هيوصل لحاوية المستخدم.
- **`GIT_SSH` بدل `core.sshCommand`:** `-c core.sshCommand=` (المطلوب من المراجعة) بيغلب على `GIT_SSH` وبيكسره، فبقى `core.sshCommand` بمسار ثابت.
- **features في `LeaseRequest`:** أي حد معاه توكن الدخول كان يقدر يعلن ميزة ماعندوش.

## العواقب
- runner بنسخة M3 بيفضل شغّال للـ jobs القديمة، وjob فيه checkout بيستنى `NO_MATCHING_RUNNER` لحد ما runner جديد يدخل.
- الـ subuid لازم يكفي 65536 لكل slot من الـ capacity (الـ 16 × 65536 اللي `dhow-runner install` بيضبطهم بيدّوا 16 slot، أكبر capacity).
- **لسه (مش في M4.7):**
  - توليد الـ credential وقت `Start` لـ `dhow.git.checkout` (M4.5 وM4.8)، ولحد ده الـ checkout بيشتغل لمستودعات HTTPS عامة من غير credential.
  - تخزين `CompleteRequest.outputs` في السيرفر و`"RunJobs"."Outputs"` (M4.8). السيرفر دلوقتي مابيقراهاش.
  - أسماء أسباب الفشل الجديدة في الواجهة (M4.10).
  - حدود الـ cgroup v2 مع الـ workspace المشترك بتتأكد في job `isolation` في الـ CI بس.

## تعديلات من M4.8 (التفاصيل في ADR-0076)
- **§2 الـ ancestry:** الـ checkout مابقاش يجيب بالـ sha أبدًا. لو الـ sha مش رأس الـ ref، الـ runner بيعمّق تاريخ الـ ref على خطوات (64 ثم 1024 ثم 65536) بـ `--filter=tree:0` (`gitsource.DeepenArgs`) جوه ميزانية دقيقتين، وبيتأكد بـ `merge-base --is-ancestor`، وغير كده `CHECKOUT_REF_MOVED`. والـ checkout بيجيب trees وملفات الـ commit المطلوب بس، فالمستودع بيبقى partial clone: أمر git في خطوة بيحتاج كائن من تاريخ قديم هيحاول يجيبه من الشبكة.
- **§4 الـ outputs بعد إعادة التشغيل:** الـ journal بقى بيحفظ الـ outputs (بعد الـ masking، 64 مفتاح و128 KiB بالكتير) مع النتيجة، وبيبعتها في `CompleteRequest` اللي بيتبعت تاني بعد ما الـ runner يقوم. فالـ journal مابقاش فيه IDs بس.
- **§6 `JobSpec.derived_values`:** حقل جديد (رقم 11) فيه مسارات القيم اللي عملها تعبير في الـ pipeline (`Env.X` و`Steps[i].Env.X` و`Steps[i].With.X`). الـ runner مابيستخدموش، بس `jobspec.Validate` في الناحيتين بيرفض أي مسار مش بيشاور على قيمة موجودة. والسيرفر بيعرض القيم دي `[REDACTED]` في تفاصيل الـ job.
