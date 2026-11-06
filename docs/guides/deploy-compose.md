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
- `shared/.env` اختياري. إذا وُجد، تمرّره الإضافة إلى Compose بـ `--env-file`. داو لا يكتب أسرار التطبيق على السيرفر: ضع الملف بنفسك.
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

اسم المشروع (`ComposeProject`) واسم الملف (`ComposeFile`) وخيار فتح السياسة (`AllowUnsafeCompose`) تأتي من الهدف، فلا يستطيع الـ commit تغييرها.

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
| `COMPOSE_UNSAFE_BIND_OUTSIDE` | ربط مسار خارج مجلد النشر، أو مجلد النشر نفسه، أو `.dhow`، أو مسار نسبي يخرج من مجلد الإصدار، أو `~` |
| `COMPOSE_UNSAFE_BIND_SYMLINK` | ربط مسار يمرّ برابط رمزي جاء من الأرشيف |
| `COMPOSE_UNSAFE_INTERPOLATION` | متغير `${...}` في مسار ربط أو ملف |
| `COMPOSE_UNSAFE_INCLUDE` | `include` في أعلى الملف |
| `COMPOSE_UNSAFE_EXTENDS` | `extends` في خدمة |
| `COMPOSE_UNSAFE_PROVIDER` | `provider` في خدمة |
| `COMPOSE_INVALID` | ملف ليس YAML صحيحًا، أو فيه أكثر من مستند، أو مفتاح مكرر، أو بلا خدمات، أو أكبر من 256 KiB، أو غير موجود في الأرشيف |

- الربط المسموح: volume مسمّى، أو `tmpfs`، أو مسار نسبي داخل مجلد الإصدار، أو مسار مطلق تحت مجلد النشر مثل `/srv/shop/shared/uploads`.
- المرجعيات `&` و`<<` في YAML تُفكّ قبل الفحص، فلا تخفي خيارًا.
- المدير يستطيع فتح قواعد `COMPOSE_UNSAFE_*` لهدف معيّن بالخيار `AllowUnsafeCompose`. هذا الخيار لا يفتح تثبيت الصور ولا منع `build`، ولا `COMPOSE_INVALID`.
- الرفض يوقف الـ job بالخروج 30 قبل أي اتصال أو كتابة على السيرفر، ويكتب الرمز واسم الخدمة في السجل. في التقرير يظهر النشر فاشلًا برسالة ثابتة.

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
4. تقرأ `compose.yaml` من السيرفر وتفحصه مرة ثانية، ثم تكتب `compose.dhow.yaml`.
5. تنفّذ `docker compose pull --quiet` للمشروع. إذا فشل السحب، لأن صورة غير موجودة في الـ registry مثلًا، يُحذف الإصدار ولا تتغيّر الخدمات التي تعمل ولا `current`، ويفشل النشر بـ `DEPLOY_FAILED`.
6. تبدّل `current` إلى الإصدار الجديد.
7. تنفّذ `docker compose up --detach --wait --no-build --wait-timeout=<ثواني>`. الأمر يعيد إنشاء الخدمات التي تغيّرت صورتها أو إعدادها فقط، وينتظر أن تصبح كل خدمة سليمة.
8. تفحص الصحة الخارجية إن وُجد `health_check_url`.
9. عند النجاح تسجّل الإصدار في `.dhow/verified` وتحذف الإصدارات الأقدم من `keep_releases` وترفع القفل.

كل أمر يُرسل إلى السيرفر سطرًا واحدًا مبنيًا من وسائط منفصلة، وكل وسيط يُقتبس. اسم المشروع ومسار الملف يصلان كـ `--project-name=` و`--file=` فلا يُقرأ أي منهما كخيار. لا يمر أي نص من المستخدم عبر shell محلي.

ما يطبعه `docker compose` يدخل في سجل الـ job. الإضافة لا تحذف الصور القديمة من السيرفر. اجعل `docker image prune` مجدولًا هناك.

## الرجوع التلقائي

بعد التبديل يسبب الرجوع أحد أمرين: فشل `docker compose up --wait` (خدمة لم تصبح سليمة في المهلة)، أو فشل فحص الصحة الخارجي. وإلغاء الـ job بعد التبديل يرجع كذلك.

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
- **`COMPOSE_UNSAFE_BIND_OUTSIDE`:** الملف يربط مسارًا خارج `/srv/shop`. انقل البيانات إلى `shared/` أو استخدم volume مسمّى.
- **`TARGET_LOCKED` و`HOST_KEY_MISMATCH`:** كما في دليل SSH.

## حدود هذا الإصدار

- الإضافة لا تحذف صورًا ولا volumes ولا حاويات يتيمة. حذف خدمة من الملف لا يوقف حاويتها.
- لا رجوع يدوي ولا زر «افحص الهدف» بعد.
- لا يكتب داو أسرار التطبيق على السيرفر. ضع `shared/.env` بنفسك.
- لا تدعم الإضافة `build` ولا `include` ولا `extends`، ولا أكثر من ملف Compose واحد للهدف.
