# ADR-0079: الـ runner في حزمة التطوير بـ Docker

- **الحالة:** مقبول. الـ runner الحقيقي في حاوية privileged للتطوير بس، والـ profile الوهمي، دول قرار Sabry يوم 2026-11-02. طريقة الـ cgroup والوصول للسيرفر وحد إعادة التشغيل مفوَّضين ومكتوبين بالتوصية.
- **التاريخ:** 2026-11-02
- **بيكمّل:** [ADR-0043](0043-job-sandbox-rootless-podman.md) (Podman rootless)، و[ADR-0053](0053-runner-implementation.md) و[ADR-0056](0056-sandbox-hardening-after-the-m3-6-review.md) (فحوص الـ sandbox)، و[ADR-0050](0050-runner-identity-implementation.md) (كشف الهوية المنسوخة)، و[ADR-0075](0075-multi-container-attempts-checkout-and-plugins.md) (الـ spike جوه Docker).

## السياق
- Sabry شغّال على Windows بـ Docker Desktop. `docker compose -f deploy/dev/compose.yaml up` بيدّيه Postgres والسيرفر والواجهة، بس مفيش runner، فمش بيشوف job بيشتغل فعلًا.
- الـ runner محتاج Linux وPodman 5.4+ rootless وتفويض cgroup v2 (`cpu` و`io` و`memory` و`pids`) وsubuids. وWSL2 بـ Ubuntu 24.04 فيه Podman 4.9 بس.
- Docker Desktop (WSL2 أو macOS) وDocker Engine على Linux الحالي كلهم cgroup v2.

## القرار
1. **خدمة `runner` شغّالة افتراضيًا** في `deploy/dev/compose.yaml`، من `deploy/dev/runner/Dockerfile`: `golang:1.27.3-trixie` متثبّتة بالـ digest، وPodman 5.4.2 من Debian 13 (البناء بيفشل لو أقدم من 5.4)، و`crun` و`uidmap` و`socat`، ومستخدم `dhow-runner` بـ 1,048,576 subuid وsubgid (16 slot).
2. **الحاوية `privileged: true` و`cgroup: private`، للتطوير بس**، ومكتوب في الوثائق إن الإنتاج مابيشتغلش كده أبدًا. الـ entrypoint (`deploy/dev/stack/runner.sh`) بيبدأ root وبيعمل اللي `Delegate=yes` بيعمله:
   - بينقل كل عمليات الحاوية لـ cgroup فرعي (`dev-init`)، وبيفعّل الـ controllers الأربعة في `cgroup.subtree_control`، وبيعمل `dhow-runner.service` ويدّي ملكيته وملكية `cgroup.procs` و`cgroup.subtree_control` و`cgroup.threads` لـ `dhow-runner`، و`memory.swap.max` = 0 (زي `MemorySwapMax=0`).
   - بيحط الـ runner جوه الـ cgroup ده وبيشغّله بـ `setpriv` بالمستخدم `dhow-runner` وبنفس الـ bounding set بتاع الـ unit، و`cgroup.kill` وقت الإيقاف زي `KillMode=control-group`.
   - **مفيش أي flag بيقفل فحص.** فحوص الـ runner (cgroup v2 والتفويض والـ subuid وحدود الـ cgroup بعد الإنشاء) شغّالة زي ما هي. ولو Docker على cgroup v1، أو الـ controllers ناقصة، أو AppArmor بيقيّد الـ user namespaces (Ubuntu 24.04+)، الـ entrypoint بيقف برسالة واضحة.
3. **الوصول للسيرفر من غير ما `--dev` يتوسّع:** `--dev` بيسمح بـ `http://` لـ loopback بس، والـ `server` مش loopback. جوه حاوية الـ runner، `socat` بالمستخدم `nobody` على `127.0.0.1:8080` بيوصّل كل اتصال لـ `server:8080`، والـ runner بيتسجّل بـ `--dev --url http://127.0.0.1:8080` و`DHOW_RUNNER_SERVER_ORIGIN` = عنوان الواجهة (الـ origin بتاع السيرفر). اترفض:
   - توسيع `--dev` لأي hostname: بيفتح `http://` لعناوين حقيقية عند أي حد بيستخدم `--dev` بالغلط.
   - `network_mode: service:server`: الحاوية الـ privileged بتبقى في network namespace السيرفر، ولو حاوية السيرفر اتعملت تاني الـ runner بيفقد الشبكة.
4. **التسجيل من غير أي باب خلفي:** مفيش endpoint جديد ولا اختصار بـ `DHOW_DEV` بيعمل توكن. الـ runner بيستنى ويكتب في السجل إنه مستني توكن، و`docker compose -f deploy/dev/compose.yaml exec runner dhow-dev-register` بيقرا التوكن من stdin (مخفي لو terminal) وبيشغّل `dhow-runner register --dev --url http://127.0.0.1:8080 --token-file -` بالمستخدم `dhow-runner`.
5. **الحالة في volume:** `runner-state` فيه الهوية ومخزن الصور، فبيفضل متسجّل بعد `down`/`up`، و`down --volumes` بيمسحه.
6. **profile `fake-runner`:** `--profile fake-runner up --scale runner=0` بيشغّل runner مبني بـ `-tags dhowtest` و`DHOW_RUNNER_DRIVER=fake` في حاوية عادية مش privileged، وبيتسجّل بنفس الأمر. **volumes منفصلة** (`fake-runner-state`)، فكل واحد runner لوحده بتسجيله، ومحدش فيهم بياخد هوية التاني، و`--scale runner=0` بيوقف الحقيقي.
7. **حد إعادة التشغيل:** السيرفر بيعطّل الـ runner لو فتح أكتر من 3 جلسات جديدة في 10 دقايق (ADR-0050)، والـ live reload كان هيعدّي الحد ده بعد 4 حفظات. الـ entrypoint بيسجّل أوقات البدء في `/var/lib/dhow-dev/starts` (volume بتاع root بس، مش volume الـ runner، انظر القسم الأخير)، وبيتجاهل أي سطر مش رقم أو في المستقبل (ساعة رجعت لورا بعد sleep على WSL2 ماتقفلش الـ runner)، ومابيبدأش أكتر من 3 مرات في 10.5 دقيقة، والبناء الجديد بيستنى وبيقول في السجل هيبدأ إمتى. الـ binary الجديد مابيتحطّش مكان القديم إلا قبل البدء، عشان الـ runner القديم مايركّبش agent جديد في الـ jobs.
8. **Windows:** السكربتات بتتنسخ في الصورة وبتاخد صلاحياتها هناك، فمش محتاجة exec bit من Windows، و`.gitattributes` بيجبر LF، والمراقبة بالـ polling زي `server.sh`.

## بعد المراجعة الأمنية
- **(Medium) root مابيفتحش أي مسار جوه مجلد `dhow-runner` بيكتب فيه:** تاريخ البدء اتنقل من volume الـ runner لـ `/var/lib/dhow-dev` (volume تاني `runner-dev`، مجلده 0700 root). والتحقق من التسجيل (`identity`) بيتعمل بالمستخدم `dhow-runner` نفسه، و`cd` لمجلد الحالة بيحصل بعد ما الصلاحيات تنزل (`env -C`). اللي root بيكتبه في مجلد بيملكه `dhow-runner` هو ملفات الـ cgroup بتاعة `dhow-runner.service` بس، وcgroupfs مافيهوش symlinks أصلًا. الإثبات: `ln -s` جوه `/var/lib/dhow-dev` بالمستخدم `dhow-runner` اترفض، وsymlinks متزرعة في `dev-starts` و`starts` القديمة ماحدش لمسها بعد إعادة تشغيل، و`identity` متزرع بيشاور على ملف root بس طلع «مش متسجّل».
- **(Low) الوقت في المستقبل:** أي سطر مش رقم من 1 لـ 12 خانة أو أكبر من دلوقتي بيتشال. اختبار للدالة بخمس حالات (مستقبل، أرقام ضخمة ونص، قديم، 3 حديثة = استنى، و2 حديثة + مستقبل = يبدأ).
- **`/src` للقراءة بس في `runner` و`fake-runner`:** root مابيشغّلش ولا بيعمل `source` لأي حاجة من `/src`. الـ entrypoint و`runner-env.sh` و`dhow-dev-register` في `/usr/local/lib/dhow-dev` جوه الصورة بملكية root. root بيقرا `/src` بس عشان `go build`، والناتج في `/opt/dhow-dev` والـ caches. الـ live reload لكود Go شغّال زي ما هو، وتغيير السكربتات محتاج `docker compose build runner`.
- **مدى الضرر مكتوب في الوثائق:** الحاوية مافيهاش باقي تقوية الـ unit (`ProtectSystem` و`InaccessiblePaths` و`ProtectProc` و`SystemCallFilter` و`RestrictAddressFamilies`) ولا seccomp ولا AppArmor بتوع Docker، فالهروب لـ `dhow-runner` بيفضل بينه وبين root على آلة Docker ثغرة رفع صلاحيات محلية واحدة. وتجربة فرع مش موثوق بالحزمة دي بتدّيه root في حاوية privileged. والتوصية: `fake-runner` لما مش محتاج jobs حقيقية.
- **الثغرات الرخيصة اتقفلت في الصورة:** كل setuid وsetgid اتشال ما عدا `newuidmap` و`newgidmap` (Podman محتاجهم)، و`mount` و`umount` بقوا 0700 لـ root زي `InaccessiblePaths`. Podman 5.4.2 شغّل job لحد `succeeded` بعدها.
- **التوكن:** الوثائق بتقول الصقه عند الطلب، ولو اتحفظ في ملف يتمسح بعدها.

## اللي اتأكد
- **هنا (cgroup v1):** الـ entrypoint بيقف بالرسالة الصح. الـ runner الوهمي من أول التسجيل لحد job ناجح. والـ runner الحقيقي في نسخة محلية مش متعملها commit (فحوص cgroup v2 والحدود مقفولة، وملكية الـ cgroup v1 بتاعة الحاوية لـ `dhow-runner`): Podman 5.4.2 rootless بالمستخدم `dhow-runner` شغّل busybox بالـ digest لحد `succeeded`.
- **CI بس:** التفويض على cgroup v2 وحدود الـ job في job "Development stack with Docker only" في `installer.yml` (`deploy/dev/test/runner-stack.sh`).

## العواقب
- الصورة حوالي 1.5 GB على الديسك، وأول `up` بيبنيها.
- الـ job الافتراضي محتاج 6 GiB (2 ذاكرة و4 قرص في الذاكرة)، فـ Docker Desktop بذاكرة أقل محتاج `DHOW_DEV_RUNNER_MAX_MEMORY` و`DHOW_DEV_RUNNER_MAX_DISK`، وهم إعدادات الـ runner العادية مش استثناء.
- jobs الـ checkout مش شغّالة في الحزمة لأن مفيش صورة `dhow-git` متثبّتة في البناء المحلي.
