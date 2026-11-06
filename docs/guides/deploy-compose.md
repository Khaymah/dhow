[English](../en/guides/deploy-compose.md)

# النشر بـ Docker Compose

إضافة النشر الرسمية `dhow-deploy-compose` تنشر مشروع Docker Compose على سيرفر بعيد عبر SSH. ترفع الإصدار إلى مجلد خاص به، وتفحص ملف Compose قبل أن تلمس السيرفر، وتسحب الصور، ثم تشغّل الخدمات وتنتظر أن تصبح سليمة. إذا فشلت خدمة في أن تصبح سليمة، أو فشل فحص الصحة، ترجع الخدمات إلى صور الإصدار السابق.

كل صورة في الملف يجب أن تكون مثبّتة بالـ digest. الإضافة لا تبني صورًا ولا تفتح منفذ Docker على الشبكة: تنفّذ أوامر `docker compose` على السيرفر عبر SSH، ولا تحتاج إلى أي وكيل (agent) هناك.

الهدف قد يكون سيرفرًا واحدًا أو مجموعة من حتى 10 سيرفرات. على المجموعة تنشر الإضافة على دفعات وترجع كل سيرفر تبدّل إذا فشل سيرفر، بالقواعد نفسها الموصوفة في [النشر على مجموعة سيرفرات](deploy-ssh.md#النشر-على-مجموعة-سيرفرات).

## كيف تظهر الملفات على السيرفر

```text
/srv/shop/
  releases/
    41-3fa9c1d2-1/
      compose.yaml
      compose.dhow.yaml
    42-8be07c55-1/
  shared/
    .env
  current -> releases/42-8be07c55-1
  .dhow/
```

- `compose.yaml` هو ملف المشروع كما جاء من الـ commit. اسمه في الهدف (`ComposeFile`) ويمكن أن يكون في مجلد فرعي.
- `compose.dhow.yaml` تكتبه الإضافة. فيه صورة كل خدمة بالـ digest، فهو سجل الصور التي نُشرت في هذا الإصدار، ومنه يعرف الرجوع صور الإصدار السابق.
- `shared/.env` اختياري. إذا وُجد، تمرّره الإضافة إلى Compose بـ `--env-file`، وهو المصدر الوحيد لمتغيرات `${...}` في الملف. داو لا يكتب أسرار التطبيق على السيرفر: ضع الملف بنفسك.
- `.dhow/compose-<الإصدار>.json` تكتبه الإضافة. فيه بصمة `compose.yaml` وبصمة `compose.dhow.yaml` وقت الفحص، وتقارنهما الإضافة قبل كل تشغيل.
- `current` و`.dhow` كما في [النشر على سيرفر بـ SSH](deploy-ssh.md#كيف-تظهر-الملفات-على-السيرفر).

مجلد الإصدار هو مجلد المشروع. المسارات النسبية في الملف، مثل `./conf`، تُحسب منه.

## تجهيز السيرفر

على السيرفر يلزم Docker مع إضافة Compose (الأمر `docker compose`)، و`sshd` مع `sftp`، والأمر `sha256sum`. أنشئ حسابًا للنشر وأعطه مجلد النشر:

```bash
sudo adduser --disabled-password --gecos "" dhow-deploy
sudo install -d -o dhow-deploy -g dhow-deploy -m 0755 /srv/shop
```

ضع المفتاح العام الذي أنشأه داو في `~dhow-deploy/.ssh/authorized_keys` بالخيارات `restrict` و`from`، كما في [النشر على سيرفر بـ SSH](deploy-ssh.md#تجهيز-السيرفر).

مستخدم Docker على السيرفر يملك فعليًا صلاحيات root عليه. لهذا:

- يُفضَّل تشغيل Docker بنمط rootless لحساب النشر، فيبقى أثر أي خطأ داخل هذا الحساب.
- إذا استخدمت Docker العادي، فحساب النشر في مجموعة `docker` هو root عمليًا. اقصر المفتاح بـ `from` وبهدف واحد، ولا تستخدمه لشيء آخر.
- تفحص الإضافة ملف Compose قبل أن تتصل بالسيرفر، وترفض الخيارات التي توسّع هذه الصلاحيات. القواعد في [قواعد ملف Compose](#قواعد-ملف-compose).

سجّل الدخول إلى الـ registry على السيرفر نفسه (`docker login`) بحساب النشر، إذا كانت الصور خاصة. داو لا يرسل بيانات الـ registry إلى السيرفر.

## الخطوة في ملف الـ pipeline

```yaml
deploy:
  needs: [build]
  environment: production
  steps:
    - uses: ghcr.io/khaymah/dhow-deploy-compose@sha256:<digest>
      with:
        artifact: stack
        target: shop
        wait_timeout: 180s
        health_check_url: https://{host}/healthz
```

الرمز `<digest>` هو بصمة الإضافة المنشورة مع كل إصدار من داو. `artifact` أرشيف فيه ملف Compose، ويصل إلى الإضافة بالشروط نفسها التي في [النشر على سيرفر بـ SSH](deploy-ssh.md#الخطوة-في-ملف-الـ-pipeline): تفحصه الإضافة قبل أي اتصال وترفض أي عنصر غير آمن.

اسم المشروع (`ComposeProject`) واسم الملف (`ComposeFile`) وخيار فتح السياسة (`AllowUnsafeCompose`) تأتي من الهدف، فلا يستطيع الـ commit تغييرها. اسم المشروع أحرف صغيرة وأرقام وشرطات (`-`) فقط، بلا شرطة سفلية، حتى لا يتطابق اسم مورد في مشروع مع مورد في مشروع آخر. إذا كتبت `name:` في أعلى الملف فيجب أن يساوي اسم المشروع.

## الصور بالـ digest

بعد دمج المدخل `images` مع الملف، يجب أن تكون صورة كل خدمة بالشكل `اسم@sha256:...`. غير ذلك يوقف النشر بـ `COMPOSE_IMAGE_NOT_PINNED` قبل أي اتصال.

يمكن تثبيت الصور من الـ pipeline بدل الملف. هذا مفيد عندما يبني job سابق الصورة ويعرف الـ digest:

```yaml
with:
  images: web=ghcr.io/acme/shop-web@sha256:<digest>,worker=ghcr.io/acme/shop-worker@sha256:<digest>
```

- كل عنصر `خدمة=صورة`، ويفصل بينها فاصلة أو سطر جديد.
- المدخل يغلب `image` في الملف. إذا ذكر خدمة غير موجودة في الملف يفشل النشر بـ `COMPOSE_IMAGE_UNKNOWN_SERVICE`.
- الخدمة التي فيها `build` مرفوضة بـ `COMPOSE_BUILD_NOT_ALLOWED`. النشر يسحب الصور ولا يبنيها.
- المتغيرات في اسم الصورة (`${TAG}`) غير مقبولة، لأن الـ digest يجب أن يكون معروفًا قبل النشر.

## قواعد ملف Compose

لأن ملف Compose يأتي من commit ويعمل بصلاحيات Docker، تفحصه الإضافة مرتين: على الأرشيف قبل أي اتصال، ثم على الملف نفسه كما وصل إلى السيرفر بعد التحقق من بصمته. الفحص الثاني يضمن أن ما يعمل هو ما فُحص.

| الرمز | ما يرفضه |
|---|---|
| `COMPOSE_UNSAFE_PRIVILEGED` | `privileged: true`، وأي أمر `post_start` أو `pre_stop` بـ `privileged` |
| `COMPOSE_UNSAFE_PID_HOST` | `pid: host` |
| `COMPOSE_UNSAFE_IPC_HOST` | `ipc: host` |
| `COMPOSE_UNSAFE_UTS_HOST` | `uts: host` |
| `COMPOSE_UNSAFE_CGROUP_HOST` | `cgroup: host` |
| `COMPOSE_UNSAFE_USERNS_HOST` | `userns_mode: host` |
| `COMPOSE_UNSAFE_NETWORK_HOST` | `network_mode: host` |
| `COMPOSE_UNSAFE_CAP_ADD` | أي `cap_add` |
| `COMPOSE_UNSAFE_DEVICES` | `devices` و`device_cgroup_rules` و`gpus` وحجز الأجهزة في `deploy.resources` |
| `COMPOSE_UNSAFE_SECURITY_OPT` | أي `security_opt` غير `no-new-privileges` |
| `COMPOSE_UNSAFE_DOCKER_SOCKET` | ربط `docker.sock` بأي مسار، و`use_api_socket` |
| `COMPOSE_UNSAFE_BIND_ROOT` | ربط `/` |
| `COMPOSE_UNSAFE_BIND_OUTSIDE` | ربط مسار مطلق، أو `~`. وقراءة `env_file` أو ملف `secrets` أو `configs` من مكان غير مجلد الإصدار و`shared/.env` |
| `COMPOSE_UNSAFE_BIND_WRITABLE` | ربط مجلد من الإصدار بصلاحية كتابة. الربط المسموح للقراءة فقط (`:ro`) |
| `COMPOSE_UNSAFE_BIND_SHARED_DATA` | ربط مجلد تحت `shared` |
| `COMPOSE_UNSAFE_BIND_STATE` | ربط `.dhow` |
| `COMPOSE_UNSAFE_VOLUME_OPTS` | `driver_opts` في volume |
| `COMPOSE_UNSAFE_NETWORK_DRIVER` و`_NETWORK_OPTS` | شبكة في أعلى الملف بـ `driver` غير `bridge`، أو فيها `driver_opts` أو `ipam` |
| `COMPOSE_UNSAFE_LOGGING` | `logging` بـ driver غير `json-file` و`local`، أو بخيار غير `max-size` و`max-file` و`compress` |
| `COMPOSE_BIND_SOURCE` | مصدر ربط أو ملف مقروء غير موجود على السيرفر قبل التشغيل، أو هو رابط رمزي أو ليس مجلدًا أو ملفًا عاديًا |
| `COMPOSE_UNSAFE_BIND_SYMLINK` | ربط مسار يمرّ برابط رمزي جاء من الأرشيف |
| `COMPOSE_UNSAFE_INTERPOLATION` | متغير `${...}` في أي مكان خارج `environment` و`command` و`entrypoint` و`labels` و`healthcheck.test` |
| `COMPOSE_UNSAFE_INCLUDE` | `include` في أعلى الملف |
| `COMPOSE_UNSAFE_EXTENDS` | `extends` في خدمة |
| `COMPOSE_UNSAFE_PROVIDER` | `provider` في خدمة |
| `COMPOSE_UNSAFE_UNKNOWN_KEY` | مفتاح ليس في قائمة المفاتيح المعروفة، في خدمة أو في أعلى الملف أو في `deploy`. المفاتيح الجديدة في Compose تُرفض حتى تُضاف إلى القائمة |
| `COMPOSE_UNSAFE_VOLUMES_FROM` | `volumes_from` |
| `COMPOSE_UNSAFE_NAMESPACE_SHARED` | `pid` أو `network_mode` أو `ipc` بالصيغة `container:` أو `service:`، و`ipc: shareable` |
| `COMPOSE_UNSAFE_EXTERNAL` | شبكة أو volume في أعلى الملف يكون `external` أو له `name` |
| `COMPOSE_UNSAFE_VOLUME_DRIVER` | `driver` لـ volume غير `local` |
| `COMPOSE_UNSAFE_SECRET_SOURCE` | `secrets` أو `configs` مصدرها غير `file`، مثل `environment` |
| `COMPOSE_UNSAFE_BIND_RELEASE` | ربط مجلد الإصدار نفسه (`.`)، أو `releases`، أو مجلد إصدار كامل، أو مسار نسبي يخرج من مجلد الإصدار (`..`) |
| `COMPOSE_UNSAFE_BIND_COMPOSE` | ربط ملف Compose أو `compose.dhow.yaml` أو مجلدًا يحتويهما |
| `COMPOSE_UNSAFE_BIND_SHARED` | ربط `shared` نفسه أو `shared/.env` |
| `COMPOSE_UNSAFE_CGROUP_PARENT` و`_RUNTIME` و`_SYSCTLS` و`_GROUP_ADD` و`_EXTRA_HOSTS` و`_EXTERNAL_LINKS` و`_MODELS` | المفاتيح `cgroup_parent` و`runtime` و`sysctls` و`group_add` و`extra_hosts` و`external_links` و`models` |
| `COMPOSE_FILE_CHANGED` | ملف Compose أو `compose.dhow.yaml` على السيرفر لا يطابق ما فُحص، أو لا يوجد سجل بصماتهما |
| `COMPOSE_ENV_FORBIDDEN` | متغير يبدأ بـ `COMPOSE_` أو `DOCKER_` أو `LD_` في `shared/.env`، أو سطر فيه لا تستطيع الإضافة قراءته |
| `COMPOSE_UNSAFE_NETWORK_MODE` | `network_mode` غير `none` (عدا `host` الذي يتبع قاعدته). لربط خدمة بشبكة استخدم `networks` |
| `COMPOSE_UNSAFE_PROJECT_NAME` | `name:` في أعلى الملف لا يساوي اسم المشروع في الهدف |
| `COMPOSE_UNSAFE_LABEL` | وسم يبدأ بـ `com.docker.` |
| `COMPOSE_UNSAFE_BIND_OPTIONS` | في صيغة الربط الطويلة: `bind.propagation` أو `bind.selinux` أو `bind.recursive` أو `bind.create_host_path: true` |
| `COMPOSE_RESOLVED_UNSAFE` | الملف بعد حساب Compose للمتغيرات والدمج يخالف ما فُحص: مشروع أو صورة أو ربط أو خيار أمان أو volume |
| `COMPOSE_INVALID` | ملف ليس YAML صحيحًا، أو فيه وسم YAML غير أساسي مثل `!reset` و`!override`، أو فيه أكثر من مستند، أو مفتاح مكرر، أو بلا خدمات، أو أكبر من 256 KiB، أو غير موجود في الأرشيف، أو فيه نص حيث يجب أن تكون القيمة `true` أو `false` (مثل `read_only: "true"`)، أو وضع ربط غير معروف في `volumes` |

- الربط المسموح: volume مسمّى، أو volume بلا اسم مثل `/data`، أو `tmpfs`، أو مجلد فرعي داخل مجلد الإصدار للقراءة فقط مثل `./conf:/conf:ro`. لا يُسمح بربط مجلد الإصدار نفسه ولا ملفات Compose ولا أي شيء تحت `shared` ولا إصدار آخر ولا مسار مطلق.
- `env_file` وملفات `secrets` و`configs` تقرأ من داخل مجلد الإصدار أو من `shared/.env` بالضبط، وتكون ملفات عادية وليست روابط.
- في الـ volumes المسماة: `driver` هو `local` فقط، بلا `driver_opts`.
- الشبكات في أعلى الملف بالـ driver الافتراضي (`bridge`) بلا `driver_opts` ولا `ipam`، و`logging` بـ `json-file` أو `local` وبالخيارات `max-size` و`max-file` و`compress` فقط. المتغيرات `${...}` في أي إدخال من `volumes`، وفي `network_mode` و`pid` و`ipc` و`uts`، مرفوضة دائمًا.
- المرجعيات `&` و`<<` في YAML تُفكّ قبل الفحص، فلا تخفي خيارًا.
- فحص ثانٍ للنموذج النهائي: قبل كل تشغيل، ومنه الرجوع، تنفّذ الإضافة `docker compose config --format json` بالبيئة المعزولة نفسها وبالملفين نفسيهما، وتفحص ما حسبه Compose فعلًا. كل مصدر ربط مسار مطلق داخل مجلد الإصدار وللقراءة فقط، ولا `privileged` ولا `cap_add` ولا `devices` ولا `security_opt` غير `no-new-privileges` ولا مساحة أسماء مشتركة، والـ volumes `local` بلا خيارات وغير خارجية، وصورة كل خدمة هي المثبّتة في `compose.dhow.yaml`، واسم المشروع هو اسم الهدف. أي اختلاف يوقف النشر بـ `COMPOSE_RESOLVED_UNSAFE`. هذا يغلق كل ثغرة تفوت فيها قراءة الملف شيئًا يحسبه Compose بطريقة أخرى. يعمل الفحص أيضًا قبل سحب الصور. النموذج الذي يزيد حجمه على 64 KiB يُرفض.
- المفاتيح المسموحة قائمة محددة: `image` و`command` و`entrypoint` و`environment` و`env_file` و`ports` و`expose` و`volumes` و`networks` و`depends_on` و`healthcheck` و`restart` و`labels` و`logging` و`deploy` (`resources` و`restart_policy` فقط) و`user` و`working_dir` و`read_only` و`tmpfs` و`stop_signal` و`stop_grace_period` و`init` و`hostname` و`domainname` و`mem_limit` و`cpus` و`ulimits` و`shm_size` و`secrets` و`configs` و`profiles` وما يبدأ بـ `x-`. أي مفتاح آخر يفشل بـ `COMPOSE_UNSAFE_UNKNOWN_KEY`.
- المدير يستطيع فتح قواعد `COMPOSE_UNSAFE_*` لهدف معيّن بالخيار `AllowUnsafeCompose`، وهي المفاتيح التي تتطلب صلاحيات أوسع مثل `privileged` و`cap_add` و`devices` و`sysctls`. ويعيد هذا الخيار فتح الربط القابل للكتابة تحت مجلد النشر. تحذير: حاوية تستطيع الكتابة في مجلد مربوط تستطيع أن تضع فيه رابطًا رمزيًا إلى `/`، ثم يتبعه Docker بصلاحيات root عند أي ربط لاحق من هذا المجلد. لا تفتح الخيار إلا لتطبيق تثق به كما تثق بالسيرفر نفسه. الخيار لا يفتح ربط `.dhow` ولا ملفات Compose ولا مجلد الإصدار، ولا يفتح تثبيت الصور ولا منع `build` ولا `COMPOSE_INVALID`، ولا القواعد التي تجلب ملفات أخرى أو تشارك مساحات الأسماء أو تستخدم موارد خارجية: `include` و`extends` و`provider` و`volumes_from` و`container:` و`service:` وما يتعلق بـ `external` وبمصادر `secrets`.
- الرفض يوقف الـ job بالخروج 30 قبل أي اتصال أو كتابة على السيرفر، ويكتب الرمز واسم الخدمة في السجل. في التقرير يظهر النشر فاشلًا برسالة ثابتة.

## البيانات الدائمة: استخدم volumes مسمّاة

الحاوية التي تكتب في مجلد من السيرفر مربوط بها تستطيع أن تضع فيه رابطًا رمزيًا إلى `/` أو إلى `.dhow`، ثم يتبع Docker الرابط بصلاحيات root في ربط لاحق. لذلك لا تسمح الإضافة بالكتابة في أي مجلد من السيرفر: البيانات التي تكتبها الحاويات توضع في volumes مسمّاة يديرها Docker.

```yaml
services:
  web:
    image: ghcr.io/acme/shop-web@sha256:<digest>
    volumes:
      - uploads:/app/uploads
      - ./conf:/app/conf:ro
volumes:
  uploads: {}
```

- الـ volume المسمّى يُعلن في أعلى الملف بلا `driver` أو بـ `driver: local`، وبلا `driver_opts`.
- الإعدادات التي تأتي مع الإصدار تُربط للقراءة فقط (`:ro`) من مجلد داخل الإصدار.
- نقل البيانات من `./data:/data`: غيّر الإدخال إلى `data:/data` وأضف `data: {}` تحت `volumes` في أعلى الملف. لينتقل المحتوى القديم، أنشئ الـ volume على السيرفر وانسخ إليه الملفات مرة واحدة قبل النشر، مثلًا بتشغيل حاوية مؤقتة تربط المجلد القديم للقراءة والـ volume الجديد للكتابة. اسم الـ volume على السيرفر هو `<اسم المشروع>_data`.
- النسخ الاحتياطي يتم بنفس الطريقة: حاوية مؤقتة تربط الـ volume وتنسخ منه.

## المتغيرات في ملف Compose

متغيرات `${...}` في الملف تأتي من `shared/.env` فقط. تشغّل الإضافة Compose بأمر `env -i` وبمتغيرين فقط: `PATH` ثابت، و`DOCKER_CONFIG` يشير إلى `.docker` في مجلد حساب النشر ليجد Docker تسجيل الدخول إلى الـ registry. فلا يصل إلى الملف شيء من بيئة السيرفر، مثل `${HOME}`.

- إذا لم يوجد `shared/.env` تمرّر الإضافة `/dev/null`، فلا يُحمَّل ملف `.env` الموجود داخل الأرشيف أبدًا.
- المتغير غير المعرّف يصبح فارغًا، ويحذّر Compose.
- قيم `shared/.env` يستطيع التطبيق أن يقرأها، وهذا مقصود: ضع فيه ما يجوز للتطبيق أن يراه فقط. المتغيرات التي تستخدم في `environment` تظهر داخل الحاوية.
- ملف `shared/.env` فيه متغير يبدأ بـ `COMPOSE_` أو `DOCKER_` أو `LD_` يوقف النشر بـ `COMPOSE_ENV_FORBIDDEN`، لأن هذه المتغيرات تغيّر ما يقرؤه Compose. والإضافة تقرأ الملف بقواعد Compose نفسها (`export` ومسافات وأسطر Windows)، وأي سطر لا تستطيع قراءته يوقف النشر بالرمز نفسه. لا تستخدم قيمًا من عدة أسطر في هذا الملف.
- أين يُسمح بـ `${...}`: في قيم `environment` (قائمة أو خريطة)، وفي `command` و`entrypoint` و`labels` و`healthcheck.test` فقط. هذه نصوص تخص الحاوية ولا تصبح مسارات على السيرفر ولا إعدادات أمان. في أي مكان آخر، مثل `image` و`ports` و`networks` و`volumes` و`read_only` و`privileged`، يفشل النشر بـ `COMPOSE_UNSAFE_INTERPOLATION` حتى مع `AllowUnsafeCompose`.
- السبب: تفحص الإضافة الملف كما هو، أما Compose فيحسب المتغيرات بعد ذلك. قيمة مثل `read_only: "${RO:-false}"` تبدو للفاحص نصًا، وتصبح `false` عند Compose، فيُربط المجلد للكتابة. لذلك لا تُقبل المتغيرات حيث تؤثر القيمة على السيرفر.
- لكتابة علامة `$` حرفيًا اكتب `$$`. الكتابة `$$$X` تعني `$` حرفيًا ثم متغير، فتُرفض خارج الأماكن المسموحة.
- القيم المنطقية والأرقام التي تقرؤها الإضافة يجب أن تكون من نوعها في YAML (`true` و`false` بلا علامات اقتباس). وقيمة الوضع في `volumes` مثل `:ro` تقبل الرموز المعروفة فقط (`ro` و`rw` و`private` و`rprivate` و`nocopy` و`delegated` و`cached` و`consistent`)، ولا يزيد الإدخال على ثلاثة أجزاء. رموز SELinux والانتشار الأخرى (`z` و`Z` و`shared` و`slave`) لا تُقبل.

## مدخلات الإضافة

| المدخل | مطلوب | الافتراضي | المعنى |
|---|---|---|---|
| `target` | نعم | | اسم الهدف في بيئة الـ job |
| `artifact` | نعم | | اسم الملف الناتج الذي فيه ملف Compose |
| `images` | لا | | تثبيت صور من الـ pipeline، بالشكل `خدمة=صورة@sha256:...` |
| `wait_timeout` | لا | `120s` | أقصى انتظار لتصبح الخدمات سليمة، من `5s` إلى `10m` |
| `pull_timeout` | لا | `10m` | أقصى وقت لسحب الصور، من `10s` إلى `30m` |
| `keep_releases` | لا | قيمة الهدف | عدد الإصدارات المحفوظة |
| `health_check_url` | لا | | عنوان `http` أو `https` يُفحص بعد أن تصبح الخدمات سليمة |
| `health_check_status` | لا | `200-399` | الرد المقبول، رقم أو مدى |
| `health_check_timeout` | لا | `5s` | مهلة الطلب الواحد |
| `health_check_retries` | لا | `10` | أقصى عدد للمحاولات |
| `health_check_interval` | لا | `5s` | الفاصل بين المحاولات |
| `health_check_stable` | لا | `3` | عدد النجاحات المتتالية المطلوبة |
| `health_check_start_delay` | لا | `0s` | انتظار قبل أول محاولة |
| `on_first_failure` | لا | `keep` | `keep` أو `fail`: ماذا يُكتب في الرسالة إذا فشل أول نشر |

قواعد فحص الصحة هي نفسها في [النشر على سيرفر بـ SSH](deploy-ssh.md#مدخلات-الإضافة)، بما فيها `{host}` والميزانية القصوى. فحص `--wait` الداخلي يتحقق من `healthcheck` الحاويات، والفحص الخارجي يتحقق من وصول المستخدم إلى الخدمة.

## ما يحدث في النشر

هذه الخطوات لسيرفر واحد. على المجموعة تتكرر على كل سيرفر بالترتيب نفسه الموصوف في دليل SSH.

1. تفحص الإضافة الأرشيف وبصمته وملف Compose، قبل أن تتصل بأي سيرفر.
2. تتصل بالمفتاح المثبّت فقط، وتفحص المسار والمساحة و`current`، وتأخذ قفلًا على الهدف.
3. ترفع الملفات إلى `releases/<الإصدار>` وتتحقق من بصماتها على السيرفر.
4. تقرأ `compose.yaml` من السيرفر وتفحصه مرة ثانية، ثم تكتب `compose.dhow.yaml` وسجل البصمات.
5. تنفّذ `docker compose pull --quiet` للمشروع. إذا فشل السحب، لأن صورة غير موجودة في الـ registry مثلًا، يُحذف الإصدار ولا تتغيّر الخدمات التي تعمل ولا `current`، ويفشل النشر بـ `DEPLOY_FAILED`.
6. تبدّل `current` إلى الإصدار الجديد.
7. تنفّذ `docker compose up --detach --wait --no-build --wait-timeout=<ثواني>`. الأمر يعيد إنشاء الخدمات التي تغيّرت صورتها أو إعدادها فقط، وينتظر أن تصبح كل خدمة سليمة.
8. تفحص الصحة الخارجية إن وُجد `health_check_url`.
9. عند النجاح تسجّل الإصدار في `.dhow/verified` وتحذف الإصدارات الأقدم من `keep_releases` وترفع القفل.

كل أمر يُرسل إلى السيرفر سطرًا واحدًا مبنيًا من وسائط منفصلة، وكل وسيط يُقتبس. اسم المشروع ومسار الملف يصلان كـ `--project-name=` و`--file=` فلا يُقرأ أي منهما كخيار. لا يمر أي نص من المستخدم عبر shell محلي.

ما يطبعه `docker compose` يدخل في سجل الـ job. الإضافة لا تحذف الصور القديمة من السيرفر. اجعل `docker image prune` مجدولًا هناك.

## الرجوع التلقائي

بعد التبديل يسبب الرجوع أحد أمرين: فشل `docker compose up --wait` (خدمة لم تصبح سليمة في المهلة)، أو فشل فحص الصحة الخارجي. وإلغاء الـ job بعد التبديل يرجع كذلك.

- قبل كل تشغيل، ومنه الرجوع واستئناف نشر انقطع، تقرأ الإضافة ملفي الإصدار من السيرفر من جديد وتقارن بصمتيهما بالسجل وتفحصهما بكل القواعد. إذا تغيّر أحد الملفين بعد فحصه لا تشغّل الإضافة شيئًا، ويفشل الرجوع بـ `DEPLOY_ROLLBACK_FAILED` والرمز `COMPOSE_FILE_CHANGED` في السجل.
- الرجوع يبدّل `current` إلى آخر إصدار فحصته داو ونجح، ثم ينفّذ `docker compose up --wait` بملفات ذلك الإصدار. الصور في `compose.dhow.yaml` مثبّتة بالـ digest، فإذا حُذفت من السيرفر سحبها Compose من جديد.
- الخدمات التي لم تتغيّر لا يعاد إنشاؤها. الخدمات التي بدأت بالصور الجديدة تعود إلى صور الإصدار السابق.
- أول نشر على هدف لا إصدار سابق له لا يستطيع الرجوع. تبقى الحاويات والإصدار كما هما، ويفشل الـ job. أصلح الخدمة وانشر من جديد.
- إذا فشل الرجوع نفسه يفشل الـ job بـ `DEPLOY_ROLLBACK_FAILED`، ويسجّل داو أن السيرفر على حالة غير مؤكدة.

حاويات الإصدار الجديد التي فشلت تبقى على السيرفر حتى يستبدلها الرجوع. لا يتراجع داو عن تغييرات خارج الخدمات، مثل تحديث قاعدة البيانات.

## النشر على مجموعة سيرفرات

نفس المراحل والرجوع الجماعي الموصوفين في دليل SSH: فحص مسبق على كل السيرفرات، ثم رفع للكل، ثم تبديل على دفعات (`Settings.Rolling.BatchSize` و`BatchPauseSeconds`) مع بوابة صحة لكل دفعة. أول فشل يوقف المجموعة، ويرجع كل سيرفر تبدّل إلى صور إصداره السابق، وتكون حالة المجموعة `rolled_back`.

سحب الصور يجري على كل السيرفرات قبل أول تبديل، فصورة ناقصة على سيرفر واحد توقف النشر قبل أن يتغيّر أي سيرفر. أما فشل `--wait` في سيرفر من دفعة متأخرة فيرجع السيرفرات التي تبدّلت قبله. استخدم `{host}` في `health_check_url` ليُفحص كل سيرفر على عنوانه.

## رموز النتيجة

رموز SSH نفسها لـ `DEPLOY_ROLLED_BACK` (10) و`DEPLOY_HEALTH_FAILED` (11) و`DEPLOY_ROLLBACK_FAILED` (12) و`HOST_KEY_MISMATCH` (13) و`ARTIFACT_UNSAFE` (14) و`TARGET_LOCKED` (15) و`CURRENT_NOT_A_SYMLINK` (16) و`TARGET_HOST_UNREACHABLE` (17) و`PLUGIN_INPUT_INVALID` (2) و`DEPLOY_FAILED` (1)، مع رمز واحد جديد:

| الرمز | الرقم | المعنى |
|---|---|---|
| `COMPOSE_*` | 30 | رُفض ملف Compose قبل أي اتصال. الرمز الكامل (مثل `COMPOSE_UNSAFE_PRIVILEGED`) في سجل الخطوة |

## أخطاء شائعة

- **`COMPOSE_IMAGE_NOT_PINNED`:** خدمة صورتها باسم أو وسم فقط. ثبّت الـ digest في الملف أو بالمدخل `images`.
- **`DEPLOY_FAILED` وسجل فيه `manifest unknown`:** الصورة غير موجودة في الـ registry. تأكد من أن الـ job الذي بناها دفعها قبل النشر.
- **`DEPLOY_FAILED` وسجل فيه `unauthorized`:** حساب النشر لم يسجّل الدخول إلى الـ registry على السيرفر. نفّذ `docker login` بحسابه.
- **`DEPLOY_ROLLED_BACK` وخدمة `unhealthy`:** `healthcheck` الخدمة لم ينجح في `wait_timeout`. زد المهلة أو أصلح الفحص. الخدمة بلا `healthcheck` تُعدّ سليمة بمجرد أن تعمل، ويبقى فحص `health_check_url` الخارجي هو الحماية، فاكتب `healthcheck` لكل خدمة.
- **`COMPOSE_UNSAFE_BIND_OUTSIDE` أو `COMPOSE_UNSAFE_BIND_WRITABLE`:** الملف يربط مسارًا مطلقًا أو مجلدًا قابلًا للكتابة. استخدم volume مسمّى للبيانات، كما في [البيانات الدائمة](#البيانات-الدائمة-استخدم-volumes-مسمّاة).
- **`TARGET_LOCKED` و`HOST_KEY_MISMATCH`:** كما في دليل SSH.

## حدود هذا الإصدار

- الإضافة لا تحذف صورًا ولا volumes ولا حاويات يتيمة. حذف خدمة من الملف لا يوقف حاويتها.
- لا رجوع يدوي ولا زر «افحص الهدف» بعد.
- لا يكتب داو أسرار التطبيق على السيرفر. ضع `shared/.env` بنفسك.
- لا تدعم الإضافة `build` ولا `include` ولا `extends`، ولا أكثر من ملف Compose واحد للهدف.
