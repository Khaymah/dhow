[English](../en/install/binary-systemd.md)

# تثبيت الملف التنفيذي مع systemd

هذه الصفحة تثبّت داو (Dhow) يدويًا كملف تنفيذي واحد (binary) تشغّله خدمة systemd محصّنة، بدون Docker. [المثبّت التفاعلي](installer.md) ينفّذ الخطوات نفسها نيابةً عنك ويتحقق منها بعد ذلك. اقرأ هذه الصفحة لتعرف ما يفعله، أو لتثبّت على جهاز لا يدعمه.

## المتطلبات

- Linux على `amd64` أو `arm64` مع systemd 247 أو أحدث (يصلح Debian 12، وUbuntu 22.04 و24.04، وRHEL وRocky وAlma 9، وFedora الحالي).
- PostgreSQL 18، على هذا الجهاز أو يمكن الوصول إليه عبر TLS.
- وكيل عكسي (reverse proxy) يتولى TLS. راجع [ضع داو خلف وكيل عكسي](reverse-proxy.md).
- `curl` و`tar` و`sha256sum` و`openssl`، و[cosign](https://docs.sigstore.dev/) 2.4 أو أحدث للتحقق من التوقيعات.

شغّل كل الأوامر التالية كـ root، مثلًا داخل `sudo -i`.

## 1. تنزيل إصدار والتحقق منه

لكل إصدار أرشيف لكل معمارية، وملف `SHA256SUMS`، وتوقيع cosign بلا مفتاح لهذا الملف، وقائمة مكوّنات SPDX SBOM لكل أرشيف:

```sh
version=1.0.0
arch=amd64
base=https://github.com/Khaymah/dhow/releases/download/v$version
mkdir -p /root/dhow-$version && cd /root/dhow-$version
curl -fsSLO "$base/dhow-server_${version}_linux_${arch}.tar.gz"
curl -fsSLO "$base/SHA256SUMS"
curl -fsSLO "$base/SHA256SUMS.sigstore.json"
cosign verify-blob --bundle SHA256SUMS.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github\.com/Khaymah/dhow/\.github/workflows/release\.yml@refs/tags/v' \
  SHA256SUMS
sha256sum --check --ignore-missing SHA256SUMS
tar -xzf "dhow-server_${version}_linux_${arch}.tar.gz"
```

يفشل `cosign verify-blob` ما لم يكن workflow الإصدار في هذا المستودع قد وقّع `SHA256SUMS` على وسم `v*`، ثم يربط `sha256sum` الأرشيف بهذا الملف. إثبات البناء (build provenance) موثّق أيضًا: `gh attestation verify dhow-server_${version}_linux_${arch}.tar.gz --repo Khaymah/dhow`. في الأرشيف `dhow-server` (والواجهة مبنية فيه)، و`dhow.service`، و`LICENSE`، و`README.md`. الملفات التنفيذية ثابتة (`CGO_ENABLED=0`) وقابلة لإعادة الإنتاج: بناء الوسم نفسه مرتين يعطي البايتات نفسها.

## 2. تثبيت الملف التنفيذي وإنشاء مستخدم الخدمة

```sh
install -o root -g root -m 0755 dhow-server /usr/local/bin/dhow-server
useradd --system --user-group --no-create-home --home-dir /var/lib/dhow \
  --shell /usr/sbin/nologin --comment "Dhow service" dhow
```

المستخدم `dhow` ليس له shell ولا مجلد منزلي. ينشئ systemd له `/var/lib/dhow` بالصلاحيات `700`.

## 3. تجهيز PostgreSQL 18

**PostgreSQL 18 محلي.** ثبّته من [مستودع مشروع PostgreSQL](https://www.postgresql.org/download/linux/)، ثم أنشئ الدور (role) وقاعدة البيانات. اتصل عبر Unix socket، حتى لا تكون قاعدة البيانات على الشبكة أبدًا:

```sh
password=$(openssl rand -hex 24)
runuser -u postgres -- psql -X -v ON_ERROR_STOP=1 -d postgres <<SQL
\set pw '$password'
CREATE ROLE dhow LOGIN PASSWORD :'pw';
CREATE DATABASE dhow OWNER dhow;
SQL
db_url="postgres://dhow:$password@/dhow?host=/var/run/postgresql&port=5432&sslmode=require"
```

فحص الإعدادات في داو يشترط `sslmode=require`، ويُتجاهل على Unix socket بالطريقة نفسها التي يتجاهله بها libpq. مع `pg_hba.conf` الافتراضي (`local all all peer`)، يدخل مستخدم النظام `dhow` كالدور `dhow` بمصادقة peer، وكلمة السر احتياط لاتصال TCP. تصل كلمة السر إلى `psql` عبر المدخل القياسي، وليس في سطر الأوامر أبدًا. يذهب المثبّت خطوة أبعد، ويرسل إلى PostgreSQL قيمة تحقق SCRAM بدل كلمة السر. هكذا لا تصل كلمة السر إلى السيرفر أبدًا، حتى مع تفعيل `log_statement`.

**سيرفر موجود.** استخدم رابطًا مع `sslmode=verify-full` ومع سلطة الشهادات (CA) التي وقّعت شهادة السيرفر، مثل `postgres://dhow:<password>@db.internal:5432/dhow?sslmode=verify-full&sslrootcert=/etc/dhow/db-ca.pem`. يجب أن يستطيع المستخدم `dhow` قراءة ملف الـ CA، وألا يكون تحت `/home` أو `/root`، لأن الخدمة تخفيهما. يحتاج داو إلى PostgreSQL 18 أو أحدث.

## 4. حفظ الأسرار

الأسرار في ملفات لا يقرؤها إلا root. يمرّرها systemd إلى الخدمة بـ `LoadCredential=`، فلا تظهر أبدًا في البيئة أو في `ps` أو في `systemctl show`:

```sh
install -d -o root -g root -m 0700 /etc/dhow /etc/dhow/credentials
umask 077
openssl rand -hex 32 > /etc/dhow/credentials/dhow-secret-key
printf '%s\n' "$db_url" > /etc/dhow/credentials/dhow-database-url
unset password db_url
```

احفظ نسخة من `/etc/dhow/credentials/dhow-secret-key` مع قاعدة البيانات، لأن نسخة قاعدة البيانات لا تُفك بدونه. راجع [الأسرار في ملفات](configuration.md#الأسرار-في-ملفات) لتعرف كيف يقرأ داو هذه الملفات وما الذي يرفضه.

## 5. كتابة الإعدادات

الإعدادات غير السرية مكانها `/etc/dhow/dhow.env`، ويقرؤه systemd كـ root قبل أن يشغّل الخدمة:

```sh
cat > /etc/dhow/dhow.env <<'EOF'
DHOW_PUBLIC_URL=https://dhow.example.com
DHOW_LISTEN_ADDR=127.0.0.1:8080
DHOW_TRUSTED_PROXIES=127.0.0.1/32,::1/128
DHOW_LOG_LEVEL=info
EOF
chmod 600 /etc/dhow/dhow.env
```

قيمة `DHOW_TRUSTED_PROXIES` السابقة تناسب وكيلًا على الجهاز نفسه. للإعدادات الأخرى راجع [الوكلاء الموثوقون](reverse-proxy.md#الوكلاء-الموثوقون). كل متغير مشروح في [الإعدادات](configuration.md). لا تضبط أبدًا `DHOW_SECRET_KEY` أو `DHOW_DATABASE_URL` في هذا الملف. مع ملفات الأسرار، يوقفان السيرفر، لأن كل سر يُضبط في مكان واحد فقط.

## 6. تثبيت الخدمة وتشغيلها

```sh
install -o root -g root -m 0644 dhow.service /etc/systemd/system/dhow.service
systemctl daemon-reload
systemctl enable --now dhow
dhow-server healthcheck && echo ready
journalctl -u dhow | grep setup_url
```

يسأل `dhow-server healthcheck` المسار `/api/v1/health/ready` على `DHOW_LISTEN_ADDR`، ويفشل حتى تصبح قاعدة البيانات متاحة ومحدّثة. الأمر الأخير يطبع الرابط الذي يعمل مرة واحدة وينشئ حساب صاحب النسخة. افتحه خلال ساعة. للحصول على رابط جديد أعد تشغيل الخدمة، كما في [العثور على رابط الإعداد](troubleshooting.md#العثور-على-رابط-الإعداد).

## ما تفعله الخدمة

[`deploy/systemd/dhow.service`](../../deploy/systemd/dhow.service) هو الملف نفسه الذي يكتبه المثبّت. يقيّمه `systemd-analyze security dhow.service` بـ **1.2 OK**، ويفشل CI إذا أصبح التقييم أسوأ من OK.

| الإعداد | الأثر |
|---|---|
| `User=dhow`، `NoNewPrivileges=yes`، `CapabilityBoundingSet=` (فارغ)، `AmbientCapabilities=` | تعمل العملية بلا صلاحيات خاصة، ولا تستطيع أبدًا الحصول على capability، ولا حتى عبر ملف setuid. المنفذ 8080 لا يحتاج إلى أي منها. |
| `ProtectSystem=strict`، `ProtectHome=yes`، `PrivateTmp=yes`، `StateDirectory=dhow dhow/blobs` | نظام الملفات كله للقراءة فقط عند داو، و`/home` و`/root` و`/run/user` مخفية، و`/tmp` خاص، و`/var/lib/dhow` وحده قابل للكتابة. وsystemd ينشئ `/var/lib/dhow/blobs` (`0700`) للملفات الناتجة والذاكرة المؤقتة. |
| `PrivateDevices=yes`، `DevicePolicy=closed`، `PrivateUsers=yes`، `PrivateIPC=yes` | لا أجهزة فعلية، وuser namespace فيه `dhow` وroot فقط، وSystem V IPC خاص. |
| `ProtectKernelTunables=yes`، `ProtectKernelModules=yes`، `ProtectKernelLogs=yes`، `ProtectControlGroups=yes`، `ProtectClock=yes`، `ProtectHostname=yes`، `ProtectProc=invisible`، `ProcSubset=pid` | لا يمكن تغيير النواة (kernel) ولا cgroups ولا الساعة ولا اسم الجهاز، والعمليات الأخرى غير مرئية. |
| `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX` | IP وUnix sockets فقط، بلا netlink ولا raw packets. |
| `SystemCallFilter=@system-service`، `SystemCallFilter=~@privileged`، `SystemCallArchitectures=native` | قائمة مسموحة بالـ system calls التي تحتاجها الخدمة العادية. |
| `MemoryDenyWriteExecute=yes`، `LockPersonality=yes`، `RestrictNamespaces=yes`، `RestrictRealtime=yes`، `RestrictSUIDSGID=yes`، `RemoveIPC=yes`، `KeyringMode=private` | لا ذاكرة قابلة للكتابة والتنفيذ معًا، ولا namespaces جديدة، ولا جدولة realtime، ولا ملفات setuid. |
| `UMask=0077` | كل ما يكتبه داو خاص به. |
| `LoadCredential=`، `EnvironmentFile=/etc/dhow/dhow.env` | الأسرار تأتي من `$CREDENTIALS_DIRECTORY`، والإعدادات من ملف البيئة، ويقرأ systemd الاثنين كـ root. |
| `Restart=on-failure`، `RestartPreventExitStatus=78` | يعيد systemd تشغيل داو بعد أي انهيار، لكن ليس بعد إيقاف سليم، ولا عندما يرفض داو قاعدة بيانات أحدث منه (الرمز `78`، راجع [الترقية والرجوع](upgrades.md)). |

ترتّب الخدمة نفسها بعد `postgresql.service` و`postgresql-18.service` إذا كانتا موجودتين، حتى تعمل قاعدة البيانات المحلية أولًا.

## الترقية

1. خذ نسخة احتياطية، كما يلي.
2. نزّل الإصدار الجديد وتحقق منه كما في الخطوة 1.
3. استبدل الملف التنفيذي وأعد التشغيل:

   ```sh
   install -o root -g root -m 0755 dhow-server /usr/local/bin/dhow-server
   cmp -s dhow.service /etc/systemd/system/dhow.service || { install -m 0644 dhow.service /etc/systemd/system/; systemctl daemon-reload; }
   systemctl restart dhow
   dhow-server healthcheck
   ```

تحديثات قاعدة البيانات تعمل عند بدء داو، تحت قفل، فلا تطبّقها إلا عملية واحدة في كل مرة. إذا فشل تحديث، لا يبدأ داو ويبقى الجدول القديم. لتجربة التحديثات قبل الترقية، وللرجوع إلى الإصدار السابق، راجع [الترقية والرجوع إلى إصدار أقدم](upgrades.md). الرجوع بالملف التنفيذي وحده يكفي في أغلب الحالات، والنسخة الاحتياطية تلزم عندما يرفع تحديث الحد الأدنى لقاعدة البيانات.

## النسخ الاحتياطي والاسترجاع

```sh
runuser -u postgres -- pg_dump -d dhow --format=custom > /root/dhow-$(date +%F).dump
tar -C /etc -czf /root/dhow-etc-$(date +%F).tar.gz dhow
chmod 600 /root/dhow-*
```

انسخ الملفين إلى مكان غير هذا الجهاز. للاسترجاع: أوقف داو، واسترجع قاعدة البيانات بـ `pg_restore --clean --if-exists --no-owner -d dhow` كمستخدم superuser، وأعد `/etc/dhow` بالمفتاح السري نفسه، ثم شغّل داو مرة أخرى.

## إزالة التثبيت

```sh
systemctl disable --now dhow
rm /etc/systemd/system/dhow.service /usr/local/bin/dhow-server /etc/dhow/dhow.env
systemctl daemon-reload
```

هذا يُبقي الأسرار في `/etc/dhow/credentials`، ويُبقي `/var/lib/dhow` وقاعدة البيانات. لحذفها أيضًا، ولا يمكن التراجع عن ذلك، شغّل كذلك `rm -r /etc/dhow /var/lib/dhow` و`userdel dhow`، ومع قاعدة بيانات محلية `runuser -u postgres -- psql -c 'DROP DATABASE dhow WITH (FORCE)' -c 'DROP ROLE dhow'`.
