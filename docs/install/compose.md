[English](../en/install/compose.md)

# التثبيت بـ Docker Compose

هذه هي طريقة التثبيت الافتراضية ([ADR-0024](../../Plan/ADR/0024-install-methods.md)). تشغّل داو (Dhow) ومعه PostgreSQL 18 الخاص به على جهاز واحد، بإعدادات آمنة افتراضيًا، ومن غير أن تتعامل مع الأسرار بيدك. للطرق الأخرى والمتطلبات المشتركة بينها، راجع [اختر طريقة التثبيت](README.md).

## المتطلبات

- Docker Engine 24 أو أحدث، مع Compose plugin (`docker compose version`).
- `openssl` و`bash` على الجهاز، لتشغيل `init.sh`.
- وكيل عكسي (reverse proxy) أمام داو يتولى TLS. داو نفسه يتكلم HTTP العادي على المنفذ 8080، ويرفض أن يبدأ إذا كان العنوان العام `http://`. في [ضع داو خلف وكيل عكسي](reverse-proxy.md) إعدادات مجرَّبة لـ nginx وCaddy وTraefik.

## البدء السريع

```sh
git clone --depth 1 https://github.com/Khaymah/dhow.git
cd dhow/deploy/compose
./init.sh https://dhow.example.com
docker compose up -d
```

إذا كنت لا تريد نسخ المستودع (repository) كله، فنزّل `compose.yaml` و`init.sh` من مجلد `deploy/compose` في وسم إصدار (release tag) إلى مجلد فارغ، ثم شغّل `chmod +x init.sh`.

يكتب `init.sh` الملف `.env` بجوار ملف Compose، وفيه:

| المتغير | القيمة |
|---|---|
| `POSTGRES_PASSWORD` | 24 بايت عشوائية بصيغة hex. |
| `DHOW_SECRET_KEY` | 32 بايت عشوائية بصيغة hex. |
| `DHOW_PUBLIC_URL` | العنوان الذي مرّرته. قيمته الافتراضية `https://dhow.localhost`، وهي مجرد قيمة مؤقتة لإلقاء نظرة أولى. |

صلاحيات الملف `0600`، و`init.sh` يرفض الكتابة فوق ملف `.env` موجود. احفظ نسخة من `DHOW_SECRET_KEY` في مكان آمن. بدونه لا يمكن فك الأسرار المحفوظة، والمفتاح الجديد لا يقرأ البيانات القديمة.

ويكتب `init.sh` أيضًا [سلسلة المفاتيح](keys.md) في `./secrets/dhow-keyring` بصلاحية `0600`، ويجعل مالكها مستخدم الحاوية `65532`، لذلك يحتاج `chown` إلى root أو `sudo` مرة واحدة. السلسلة ليست متغير بيئة: `compose.yaml` يربطها بالحاوية كـ secret في `/run/secrets/dhow-keyring`، فلا تظهر في `docker inspect`. مع Docker بدون root، اضبط `DHOW_CONTAINER_UID` على رقم المستخدم الذي يقابل `65532` في `/etc/subuid`. صدّر السلسلة بعد أول تشغيل واحفظ النسخة بعيدًا عن الجهاز، كما في [النسخ الاحتياطي والتصدير](keys.md#النسخ-الاحتياطي-والتصدير).

ينتظر `docker compose up -d` حتى يصبح PostgreSQL جاهزًا، ثم يطبّق تحديثات قاعدة البيانات (migrations)، ثم يشغّل داو. تحقّق من ذلك:

```sh
docker compose ps
curl -s http://127.0.0.1:8080/api/v1/health/ready
```

داو منشور على واجهة loopback فقط، فلا يصل إليه أحد من الشبكة حتى تضع أمامه وكيلًا عكسيًا. إذا فتحت `http://127.0.0.1:8080` على الجهاز نفسه سترى الواجهة، لكن الإعداد وتسجيل الدخول لا يعملان إلا من عنوان `DHOW_PUBLIC_URL`. السبب أن المتصفح يرسل هذا العنوان في `Origin`، وكوكي الجلسة يحتاج إلى HTTPS.

اتبع [ضع داو خلف وكيل عكسي](reverse-proxy.md) لإعداد nginx أو Caddy أو Traefik. يشرح الدليل الترويسات (headers) التي يحتاجها داو، و`DHOW_TRUSTED_PROXIES`، وكيف تتحقق من النتيجة. يمكن تشغيل Traefik داخل مشروع Compose هذا بملف إضافي. اضبط `DHOW_PUBLIC_URL` في `.env` على عنوان `https://` العام، ثم شغّل `docker compose up -d` مرة أخرى لتطبيقه.

## أنشئ حساب صاحب النسخة الأول

السيرفر الجديد ليس فيه مستخدمون، والتسجيل العام مغلق. في كل تشغيل من غير صاحب نسخة، يكتب داو في السجل رابط إعداد يعمل مرة واحدة. صلاحيته ساعة واحدة:

```sh
docker compose logs dhow | grep setup_url
```

الرمز (token) هو الجزء الذي بعد `#`. افتح الرابط كاملًا في المتصفح. صفحة الإعداد تقرأ الرمز وتحذفه من شريط العنوان فورًا، ثم تطلب اسمك وبريدك وكلمة السر واسم المؤسسة. عند الإرسال تنشئ حساب صاحب النسخة وأول مؤسسة، وتسجّل دخولك. الصفحة تعمل من هذا الرابط فقط. إذا فتحت `/setup` من غير الرمز، فستشرح لك أين تجده.

الخطوة نفسها طلب API واحد إذا كنت تفضّل الطرفية. يجب أن تكون ترويسة `Origin` مساوية لـ `DHOW_PUBLIC_URL`:

```sh
curl -sS -X POST https://dhow.example.com/api/v1/setup \
  -H 'Content-Type: application/json' -H 'Origin: https://dhow.example.com' \
  -d '{"SetupToken":"<token>","OwnerName":"Mona","Email":"mona@example.com","Password":"<12 to 128 characters>","OrganizationName":"Acme","OrganizationSlug":"acme"}'
```

إذا انتهت الساعة أو أضعت الرابط، فأعد تشغيل داو بـ `docker compose restart dhow` لتحصل على رابط جديد. الرابط القديم يتوقف فورًا. بعد الإعداد يختفي الرابط، ويُرجع `GET /api/v1/meta` القيمة `SetupRequired: false`.

يستخدم تسجيل الدخول كوكي يعيش 7 أيام بالأكثر، وينتهي بعد 12 ساعة بلا نشاط (4 ساعات لأصحاب المؤسسات والمديرين). لذلك اجعل الوكيل العكسي يمرّر الكوكيز وترويسة `Origin` كما هي.

## الإعدادات

اضبط هذه القيم في `.env`. يقرأ Compose الملف تلقائيًا، والقيم المصدَّرة في الـ shell لها الأولوية. صفحة [الإعدادات](configuration.md) تشرح كل متغير `DHOW_*` وكيف نتحقق منه. يستطيع صاحب النسخة أيضًا تغيير إعدادات البريد والمؤسسات وكلمات السر وحدود الطلبات والسجلات من **إعدادات السيرفر**. لتفعل ذلك، اترك هذه القيم فارغة في `.env`. القيمة المضبوطة هنا هي التي تُطبَّق، وتظهر في الواجهة للقراءة فقط.

| المتغير | القيمة الافتراضية | المعنى |
|---|---|---|
| `POSTGRES_PASSWORD` | مولَّدة | كلمة سر مستخدم قاعدة البيانات `dhow`. استخدم حروفًا آمنة في الروابط فقط. |
| `DHOW_SECRET_KEY` | مولَّدة | يشفّر الأسرار المحفوظة. بصيغة hex أو base64، و32 بايت على الأقل. المفتاح الذي يشبه قيمة مؤقتة أو قليل التنوع يمنع السيرفر من البدء. |
| `DHOW_PUBLIC_URL` | مولَّدة | عنوان `https://` الذي يفتحه المستخدمون. |
| `DHOW_PUBLISH` | `127.0.0.1:8080` | عنوان الجهاز والمنفذ الذي يُنشر عليه داو. استخدم `0.0.0.0:8080` فقط إذا كان شيء آخر يتحكم في الوصول إلى المنفذ. |
| `DHOW_TRUSTED_PROXIES` | فارغة | عناوين IP أو نطاقات CIDR للوكلاء العكسيين الذين نثق بترويسة `X-Forwarded-For` منهم، مفصولة بفواصل. الوكيل على الجهاز نفسه يصل إلى الحاوية من بوابة شبكة Docker bridge، وليس من `127.0.0.1`. الأمر `docker network inspect dhow_edge --format '{{(index .IPAM.Config 0).Gateway}}'` يطبعها. راجع [الوكلاء الموثوقون](reverse-proxy.md#الوكلاء-الموثوقون). |
| `DHOW_LOG_LEVEL` | `info` | `debug` أو `info` أو `warn` أو `error`. |
| `DHOW_BREACHED_PASSWORD_CHECK` | `true` | يرفض كلمات السر الجديدة الموجودة في تسريبات معروفة، عبر range API من Have I Been Pwned. أول 5 حروف فقط من SHA-1 لكلمة السر تخرج من السيرفر. إذا لم نصل إلى الخدمة نتخطى الفحص ونكتب تحذيرًا في السجل. اضبطه على `false` في الشبكات بلا إنترنت. |
| `DHOW_IMAGE` | `ghcr.io/khaymah/dhow:latest` | الصورة التي نشغّلها. ثبّتها بالـ digest، كما في [تحقق من صورة](#تحقق-من-صورة). |

لا تحتاج إلى `DHOW_DEV`، وملف Compose لا يضبطه أبدًا. الاتصال بقاعدة البيانات يستخدم TLS (`sslmode=require`)، والعنوان العام `https://`، فتنجح الفحوص الصارمة في السيرفر بدونه. `DHOW_DEV=true` يخفّف هذه الفحوص للتطوير المحلي فقط. لا تضبطه أبدًا على جهاز يصل إليه غيرك.

## ما تحميه الحزمة

- PostgreSQL على شبكة Docker داخلية، بلا طريق إلى الخارج وبلا منفذ منشور. اتصال داو به يستخدم TLS بشهادة مولَّدة في كل تشغيل. هذا يشفّر البيانات المنقولة لكنه لا يثبت هوية السيرفر، وهذا مقبول على شبكة لا يشترك فيها غير الحاويتين.
- الحاويتان تعملان بنظام ملفات جذري للقراءة فقط، و`tmpfs` للملفات المؤقتة، وبلا أي capabilities، ومع `no-new-privileges`، وبحدود للمعالج والذاكرة وعدد العمليات.
- صورة داو مبنية من `distroless/static` وتعمل بالمستخدم `65532`. ليس فيها shell ولا مدير حزم، لذلك يفشل `docker exec dhow sh` عن قصد.
- يبدأ داو بعد أن يصبح PostgreSQL جاهزًا فقط، ويعيد Compose تشغيل الحاويتين ما لم توقفهما أنت.

متغيرات البيئة في الحاوية، ومنها المفتاح السري، يراها أي شخص يستطيع تشغيل `docker inspect`. تعامل مع الوصول إلى Docker socket كأنه وصول root إلى السيرفر.

## الترقية

1. خذ نسخة احتياطية أولًا، كما في [النسخ الاحتياطي والاسترجاع](#النسخ-الاحتياطي-والاسترجاع).
2. اقرأ ملاحظات الإصدار الجديد.
3. تحقق من الصورة الجديدة، كما في [تحقق من صورة](#تحقق-من-صورة).
4. اضبط `DHOW_IMAGE` في `.env` على الوسم أو الـ digest الجديد، ثم:

```sh
docker compose pull
docker compose up -d
```

تحديثات قاعدة البيانات تعمل تلقائيًا عند بدء داو، تحت قفل، فلا تطبّقها إلا حاوية واحدة في كل مرة. إذا فشل تحديث، لا يبدأ السيرفر ويبقى الجدول القديم كما هو. لتجربة التحديثات قبل الترقية، وللرجوع إلى نسخة أقدم، راجع [الترقية والرجوع إلى إصدار أقدم](upgrades.md). الرجوع بالصورة السابقة وحدها يكفي في أغلب الحالات، والنسخة الاحتياطية التي أخذتها في الخطوة 1 تلزم عندما يرفع تحديث الحد الأدنى لقاعدة البيانات.

## النسخ الاحتياطي والاسترجاع

خذ نسخة من قاعدة البيانات ومن `.env` ومن المجلد `./secrets/`. نسخة قاعدة البيانات لا فائدة منها بدون `DHOW_SECRET_KEY` وسلسلة المفاتيح.

```sh
docker compose exec -T postgres pg_dump -U dhow -d dhow --format=custom > dhow-$(date +%F).dump
cp .env dhow-env-$(date +%F).backup
sudo tar -czf dhow-secrets-$(date +%F).tar.gz secrets
chmod 600 dhow-env-*.backup
```

للاسترجاع في الحزمة التي تعمل: أوقف داو حتى لا يكتب شيء أثناء الاسترجاع، ثم أفرغ الـ schema حتى تطابق النتيجة النسخة تمامًا، ثم حمّل النسخة. خطوة `DROP SCHEMA` تحذف كل شيء في قاعدة البيانات:

```sh
docker compose stop dhow
docker compose exec -T postgres psql -U dhow -d dhow -c 'DROP SCHEMA public CASCADE' -c 'CREATE SCHEMA public'
docker compose exec -T postgres pg_restore -U dhow -d dhow --no-owner < dhow-2026-01-01.dump
docker compose start dhow
```

للاسترجاع على جهاز جديد: شغّل `./init.sh`، واستبدل `DHOW_SECRET_KEY` المولَّد في `.env` بالمفتاح من نسختك الاحتياطية، واستبدل `./secrets/dhow-keyring` بالسلسلة من النسخة (بالمالك `65532` والصلاحية `0600`). ثم شغّل PostgreSQL وحده بـ `docker compose up -d postgres`، وشغّل أمر `pg_restore`، ثم شغّل `docker compose up -d`. إذا لم تطابق السلسلةُ القاعدةَ المسترجعة، يتوقف داو برسالة فيها رقم الإصدار، ولا يعمل ببيانات تالفة.

## تحقق من صورة

صور الإصدارات موقّعة بـ [cosign](https://docs.sigstore.dev/) بتوقيع بلا مفتاح (keyless) من workflow الإصدار في هذا المستودع. يستطيع أي شخص التحقق من التوقيع بلا مفتاح:

```sh
cosign verify ghcr.io/khaymah/dhow:1.0.0 \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github\.com/Khaymah/dhow/\.github/workflows/release\.yml@refs/tags/v'
```

يفشل الأمر ما لم توقّع الصورة شهادةٌ صادرة لهذا الـ workflow، على وسم `v*` من هذا المستودع. قائمة المكوّنات SPDX SBOM وإثبات البناء SLSA مرفقان بالصورة كإثباتات موقّعة (attestations):

```sh
gh attestation verify oci://ghcr.io/khaymah/dhow:1.0.0 --repo Khaymah/dhow
```

بعد أن تنجح الصورة في التحقق، ثبّتها بالـ digest حتى لا يُنقل الوسم إلى صورة أخرى من غير علمك:

```sh
docker buildx imagetools inspect ghcr.io/khaymah/dhow:1.0.0 | sed -n 's/^Digest: *//p'
```

ضع النتيجة في `.env` هكذا: `DHOW_IMAGE=ghcr.io/khaymah/dhow@sha256:<digest>`.

## إزالة التثبيت

```sh
docker compose down
```

يوقف الحاويات والشبكات ويحذفها، لكنه يحتفظ بمساحة تخزين قاعدة البيانات (volume). لحذف البيانات أيضًا، ولا يمكن التراجع عن ذلك:

```sh
docker compose down --volumes
```

ثم احذف المجلد الذي فيه `.env`، واحذف الصورة بـ `docker image rm`.

## حل المشكلات

- **`run ./init.sh to create .env`**: لم يجد Compose المتغيرات المطلوبة. شغّل `./init.sh` في مجلد `compose.yaml` نفسه.
- **يتوقف داو بعد بدئه مباشرة**: الأمر `docker compose logs dhow` يطبع كل إعداد غير صالح، مثل عنوان عام ليس `https://`. يرفض السيرفر العمل إذا كان المفتاح السري ناقصًا، أو قيمة مؤقتة، أو قليل التنوع.
- **المنفذ 8080 مشغول**: اضبط `DHOW_PUBLISH=127.0.0.1:9090` في `.env`.
- **أي شيء عن `CSRF_FAILED` أو الكوكيز أو `sslmode` أو عنوان العميل الخاطئ**: راجع [حل المشكلات](troubleshooting.md).
