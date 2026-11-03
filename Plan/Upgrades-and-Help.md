# خطة الترقيات والـ migrations والمساعدة جوه التطبيق (Upgrades, migrations & in-app help)

**الحالة:** مقبولة. Sabry جاوب الأسئلة الـ 11 كلها يوم 2026-11-02 ([قراراته](#قرارات-sabry-2026-11-02))، والقرارات مكتوبة في [ADR-0077](ADR/0077-upgrades-migrations-and-update-policy.md). وقاعدة الـ migrations (expand/contract) شغّالة من النهارده على أي migration جديد، حتى قبل الـ hook. الباقي مفوَّض ومكتوب بالتوصية، ومفيش سؤال مفتوح · **التاريخ:** 2026-11-02

## الهدف
Sabry عايز داو «ذكي ومساعد على قد ما نقدر». الخطة دي بتغطّي تلات حاجات:
1. **التحديث:** داو يعرف لوحده إن فيه نسخة جديدة، ويقول للمالك فيها إيه بالعربي المبسّط، ويثبّت التحديثات الأمنية لوحده جوه نافذة صيانة، بنسخة احتياطية قبلها ورجوع أوتوماتيك لو فحص الصحة فشل. ومن غير telemetry ولا instance id.
2. **الكسر:** أي release بيغيّر حاجة موجودة (الـ schema، والإعدادات، والـ API، وبروتوكول الـ runner، و`.dhow.yml`) بيعدّي بفترة توافق، والنسخة القديمة تقدر ترجع من غير ما تبوّظ البيانات.
3. **المساعدة:** صفحة «عن داو»، و«الجديد» بعد كل ترقية، ومساعدة في كل صفحة بتودّي لصفحة الوثائق الصح، وأسئلة شائعة، ونتايج `doctor` في الواجهة، ورسايل خطأ بتقول المستخدم يعمل إيه.

**شرط الخلاص للخطة كلها (مع M11):**
- تنصيب binary على Debian 13 وتنصيب Compose بالـ installer، الاتنين على النسخة N-1 بالسياسة الافتراضية: بيستقبلوا patch أمني موقّع ويثبّتوه لوحدهم جوه النافذة، بنسخة احتياطية مشفّرة قبلها.
- نفس الاختبار بنسخة فحص الصحة بتاعتها بيفشل: بترجع لوحدها للنسخة القديمة، والـ audit والإشعار بيقولوا حصل إيه.
- `kill -9` للـ helper أو قطع الكهربا في أي خطوة من الـ swap بيخلّص يا على النسخة الجديدة سليمة يا على القديمة سليمة، ومفيش حالة في النص.
- feed مزوّر، أو قديم (replay)، أو منتهي، أو بنسخة أقدم: بيترفض، ومفيش أي تغيير على الجهاز.
- تنصيب air-gapped مابيعملش ولا طلب شبكة واحد (اختبار).
- كل migration من وقت الخطة دي معلَّم `expand` أو `contract`، وCI بيثبت إن binary النسخة N-1 بيشتغل على schema النسخة N.

المراجع:
- التنصيب: [ADR-0024](ADR/0024-install-methods.md) و[ADR-0036](ADR/0036-interactive-host-installer.md) و[ADR-0060](ADR/0060-host-size-check.md) و[ADR-0066](ADR/0066-runner-install-and-encrypted-identity.md).
- الـ runner: [ADR-0042](ADR/0042-runner-protocol-and-leasing.md) و[ADR-0050](ADR/0050-runner-identity-implementation.md) و[ADR-0053](ADR/0053-runner-implementation.md) و[ADR-0065](ADR/0065-runner-packaging-part-a.md) و[ADR-0070](ADR/0070-ports-in-core-and-project-permissions.md).
- الإعدادات: [ADR-0063](ADR/0063-instance-settings-in-the-database.md). النطاق: [ADR-0068](ADR/0068-first-production-release-scope.md).
- الواجهة واللغة: [ADR-0034](ADR/0034-ui-direction-calm-and-organized.md) و[ADR-0059](ADR/0059-ui-direction-control-room.md) و[ADR-0049](ADR/0049-web-code-splitting-and-message-catalogs.md) و[ADR-0062](ADR/0062-simple-arabic-for-users.md) و[ADR-0064](ADR/0064-arabic-first-documentation.md).
- `.github/workflows/release.yml` و`deploy/install/get.sh` و`docs/install/{compose,binary-systemd,installer}.md`.

## اللي موجود النهارده (اتراجع في الكود يوم 2026-11-02)
| النقطة | اللي لقيناه | المكان |
|---|---|---|
| الـ migrations | goose v3.29.0 بـ `Provider` وقفل session في Postgres، و`Up` بيشتغل في الإقلاع قبل أي حاجة، ولو فشل السيرفر مابيقومش. كل migration في transaction لوحده إلا `-- +goose NO TRANSACTION` (`00042`). | `Database/Migrations/Migrator.go` و`App/Server.go` |
| الرجوع | فيه أقسام `Down` في كل الملفات، بس بتتستخدم في الاختبارات بس (`DownTo` في 3 اختبارات). الإنتاج مابيرجعش أبدًا | `Modules/*/Test/Integration` |
| **ثغرة: binary أقدم على schema أحدث** | goose مابيشتكيش لو القاعدة فيها نسخ أعلى من اللي يعرفها (`gooseutil.UpVersions` بيتجاهلها)، بس `Migrator.Current` بيقارن بـ `==`، فـ `/health/ready` بيرجّع 503 للأبد. يعني رجوع الـ binary من غير استرجاع القاعدة مستحيل النهارده، حتى لو التغيير إضافة بس | `Migrator.go` و`Modules/System/System.Service.go` |
| نمط expand موجود من غير اسم | `00041` بيضيف constraints بـ `NOT VALID` و`00042` بيعمل `VALIDATE` و`CREATE INDEX CONCURRENTLY` بره الـ transaction | `00041_projects.sql` و`00042_projects_validate.sql` |
| الإعدادات من env للقاعدة | **مفيش نسخ.** ADR-0063 قرر إن env بيكسب ويقفل، ورفض صراحة «env يتنسخ مرة ويتجاهل بعدها». الـ brief قال إن فيه نسخ مرة واحدة بتحذير، وده مش موجود في الكود ولا في الـ ADR | `Core/Settings/Store.go` (`SourceEnv` و`LockedBy`) |
| الـ runner | `Dhow-Protocol` لازم يساوي `"1"` بالظبط، والمختلف بياخد `RUNNER_UPGRADE_REQUIRED` والـ runner بيقف. مفيش نافذة N-1. بس فيه `"Jobs"."RequiredFeatures"` (ADR-0070): job محتاج ميزة مابيروحش لـ runner قديم | `Core/RunnerAuth/Gate.go` و`apps/runner/internal/Client` |
| الترقية | يدوي: `get.sh --version X` (الـ installer بيستبدل الـ binary ويحذّر من حجم الجهاز بس)، أو `DHOW_IMAGE` و`docker compose pull`. الوثائق بتقول «خد نسخة احتياطية الأول» وبتكتب الأوامر، ومفيش حاجة بتعملها لوحدها، ومفيش رجوع غير الاسترجاع | `docs/install/{installer,compose,binary-systemd}.md` |
| صورة Compose | الافتراضي `ghcr.io/khaymah/dhow:latest` بالـ tag مش بالـ digest | `deploy/compose/compose.yaml` |
| التوقيع | cosign keyless للصورة ولـ `SHA256SUMS` (bundle)، وSBOM SPDX وSLSA provenance كـ attestations، وفحص إن البناء التاني مطابق byte by byte. مفيش مفتاح عندنا: الهوية هي workflow الـ release. **ومفيش ولا tag لسه** | `.github/workflows/release.yml` و`deploy/release/dist.sh` |
| `get.sh` | الـ checksum دايمًا، وcosign لو موجود على الجهاز (وإجباري لو المصدر مخصّص) | `deploy/install/get.sh` |
| النسخة للعامة | `GET /meta` من غير دخول بيرجّع `Version` و`Commit`. ده بيسهّل على المهاجم يعرف أنهي instances مش متحدّثة | `Modules/System` و`openapi.yaml` |
| الواجهة | صفحة `/system` (للكل) فيها `BuildInfoCard` (النسخة والـ commit) و`ChecksCard` وتفاصيل كل فحص بخطوات الإصلاح، و`/system/settings` للمالك | `apps/web/Src/Modules/System` |
| `.dhow.yml` | `version: 1` إجباري، وغيره بيطلّع `VERSION_UNSUPPORTED` | `internal/pipelinespec` |
| M11 | «ضمانات الترقية، والإصدارات الموقّعة، والإعلانات الأمنية» من غير تفاصيل | `docs/roadmap.md` |

## القرارات اللي بدأت بيها الخطة (Sabry، 2026-11-02، ملزمة)
1. **الخطة دي بتتكتب دلوقتي بالتوازي مع M4**، وأي migration من النهارده بيمشي على قاعدة expand/contract.
2. **سياسة التحديث الافتراضية «إشعار + الأمني أوتوماتيك»:**
   - الـ patch الأمني بيتثبّت لوحده جوه نافذة الصيانة، بنسخة احتياطية أوتوماتيك قبله، ورجوع أوتوماتيك لو فحص الصحة فشل.
   - الـ minor والـ major بيطلّعوا إشعار وزرار «حدّث الآن».
   - المالك يقدر يغيّر السياسة: إشعار بس، أو الأمني أوتوماتيك، أو كله أوتوماتيك جوه النافذة.
   - التنصيب المعزول (air-gapped) بيقفل الفحص.
3. **M5:** قاعدة البيئة بتربط الـ branches أو الـ tags المسموح لها تنشر عليها، والتحقق في السيرفر. مكتوبة في branch تاني، والخطة دي مابتلمسش البيئات.
4. **كله ببلاش:** مفيش خدمة مدفوعة ولا جهاز بفلوس. يعني مفيش دومين، ولا hardware keys، ولا CDN أو خدمة مدفوعة. الـ feed على GitHub Pages والأرشيفات على GitHub Releases بتوع نفس الريبو (مجانيين للريبوهات العامة)، والمفاتيح ملفات مشفّرة على USB، والـ CI على runners GitHub المجانية.

## النطاق
1. **فحص التحديثات:** feed ثابت وموقّع (TUF)، بمفاتيح متثبّتة في الـ binary، وحماية من الرجوع لنسخة أقدم ومن التجميد، وcache وbackoff، ومفتاح قفل للمعزول.
2. **الـ updater:** helper منفصل بصلاحيات root لكل طريقة تنصيب، والسيرفر نفسه عمره ما بياخد root ولا بيكتب الـ binary بتاعه. وخطواته: تنزيل، وتحقق، وpreflight، ونسخة احتياطية، وswap، وفحص صحة، ورجوع، ونافذة صيانة.
3. **قاعدة الـ migrations (expand/contract):** القاعدة نفسها، وتوافق الـ schema في الـ binary، والـ backfills، والـ dry run، والـ hook واختباراته.
4. **توافق باقي العقود:** الإعدادات (env والقاعدة)، و`/api/v1`، وبروتوكول الـ runner وتحديثه لنفسه، و`.dhow.yml` و`dhow migrate`.
5. **جوه التطبيق:** «عن داو»، و«الجديد»، والمساعدة حسب الصفحة، والأسئلة الشائعة، و`doctor` في الواجهة، ورسايل خطأ فيها الحل.
6. **هندسة الـ releases:** الـ changelog، والملاحظات بالعربي والإنجليزي، وقواعد semver، وتعريف الـ release الأمني، واختبارات الترقية في CI.

## مش في الخطة دي
- **Kubernetes وHelm:** بعد أول نسخة (ADR-0068). على Kubernetes الفحص بيشتغل والسياسة إشعار بس، والـ GitOps هو اللي بيحدّث.
- **الحزمة الموقّعة للشبكات المعزولة:** في `ee/` مع M8 (ADR-0024). في أول نسخة، المعزول = الفحص مقفول، والترقية بـ `get.sh` من ملفات متنقولة بإيد ومتحقق منها.
- **ترقية أكتر من instance بالتدريج (rolling) أوتوماتيك:** مع HA في M8. في أول نسخة الـ updater بيشتغل على instance واحدة، ولو لقى أكتر بيرجع للإشعار بس.
- **تحديث PostgreSQL نفسه** (major) أو صورته في Compose: بره داو. الـ preflight بيتأكد إنه 18، والصفحة بتقول لو فيه digest أحدث موصى بيه.
- **telemetry وإحصاءات الاستخدام:** مش هتتعمل، ولا حتى اختيارية.
- **نسخ احتياطية بره الجهاز أوتوماتيك** (S3 وغيره): الأوامر بتطلّع ملف مشفّر (وبمفتاح `age` عام للتصدير، [قرار 7](#قرارات-sabry-2026-11-02))، ونقله مسؤولية المشغّل.
- **أي خدمة أو جهاز مدفوع** (دومين، أو CDN، أو hardware key، أو خدمة توقيع): ممنوع بقرار Sabry.

## المعمارية
```
 https://khaymah.github.io/dhow/updates/   TUF metadata on the repo's GitHub Pages: root, timestamp, snapshot, targets
 https://github.com/Khaymah/dhow/releases/download/v<X.Y.Z>/   TUF targets = the release assets
        ▲   GET only, fixed User-Agent "Dhow", no query string, no cookie, no instance id
        │
 ┌──────┴──────────────── dhow-server (User=dhow, no root, read-only binary) ────────────────────────┐
 │ Modules/Updates   check every 6h ± 30m, verify TUF, keep state in "UpdateFeedState"             │
 │                   policy + window from InstanceSettings (group "updates"), notices, audit         │
 │                   decides WHAT and WHEN ──► /var/lib/dhow/update/request.json  (dhow-owned, 0700) │
 │                   reads the result     ◄── /var/lib/dhow/update/status.json                        │
 └───────────────────────────────────────────────────────────────────────────────────────────────────┘
        │ dhow-update.path (PathChanged, TriggerLimit)      dhow-update.timer (every 15 min, catch-up)
        ▼
 dhow-update.service  Type=oneshot, root, hardened = the INSTALLED /usr/local/bin/dhow-server update apply
   0 lock /var/lib/dhow-update/lock (flock), read journal, refuse anything but a newer signed version
   1 fetch + verify the feed itself (TUF, pinned root) and the artifact (length + sha256 from targets)
   2 preflight: disk, PostgreSQL 18, pg_dump, new binary `migrate dry-run` as User=dhow
   3 backup: pg_dump -Fc + /etc/dhow  ──► AES-256-GCM  ──► /var/lib/dhow-update/backups (budget, rotation)
   4 journal (fsync) ─► rename new binary over /usr/local/bin/dhow-server ─► systemctl restart dhow
   5 health: /health/ready 200 + /meta Version == target + 60 s stable + no restart loop
   6 ok: status ok, keep previous binary 7 days | fail: rollback (binary; DB restore only if the floor moved)
 dhow-update-recover.service  Before=dhow.service: finishes or undoes a half-done journal after a crash

 Compose (installer-managed): same helper on the HOST, swap = DHOW_IMAGE=<repo>@sha256:<digest> in .env
 runner host: dhow-runner-update.{path,service}, the runner drains first, the helper swaps, same journal
```

## القرارات المفوَّضة
| # | القرار | ليه في سطر | المرحلة |
|---|---|---|---|
| 1 | **السيرفر بيقرر، والـ helper بينفّذ وبيتحقق من تاني.** السيرفر بيكتب في الطلب رقم النسخة بس، والـ helper بيجيب الـ feed بنفسه، ويرفض أي حاجة مش موقّعة أو مش أحدث من المتنصّب | سيرفر مخترَق أقصى حاجة يعملها إنه يطلب نسخة رسمية أحدث، أو إعادة تشغيل | U9 |
| 2 | الـ helper هو subcommand `dhow-server update` في الـ binary **المتنصّب** (مش اللي اتنزّل)، والكود في `internal/updater` زي `internal/installer`، ووضع السيرفر عمره ما بيناديه (hook) | artifact واحد زي الـ installer (ADR-0036)، والكود اللي بيتحقق هو كود موثوق قبل التنزيل | U9 |
| 3 | الطلب والحالة ملفات JSON في مسارات ثابتة، بـ schema صارم، 4 KiB بالكتير، وبتتقري بـ `O_NOFOLLOW` | مفيش socket ولا API بين process عادي وroot، والسطح أصغر حاجة ممكنة | U9 |
| 4 | journal بيتكتب بـ fsync قبل كل خطوة، و`dhow-update-recover.service` بيشتغل قبل `dhow.service` في كل إقلاع | قطع الكهربا في النص بيكمّل أو بيرجع، عمره ما بيسيب حالة مش معروفة | U9 |
| 5 | **الـ floor:** عمود `"Instance"."SchemaFloor"` (نسخة semver). الـ contract migration بيرفعه، والـ binary اللي القاعدة أحدث منه بيقوم لو نسخته ≥ الـ floor، وغير كده بيرفض بـ `SCHEMA_TOO_NEW` | الـ binary القديم مايعرفش الـ migrations الجديدة، فلازم القاعدة نفسها تقوله ينفع ولا لأ | U2 |
| 6 | كل migration جديد بيبدأ بـ directive `-- dhow:phase expand` أو `-- dhow:phase contract`، والـ contract معاه `-- dhow:floor X.Y.Z` | directive بيتقري بالآلة زي `-- +goose`، مش كومنت | U1 |
| 7 | الـ patch فيه `expand` بس، والـ `contract` في minor أو major بس | التحديث الأمني الأوتوماتيك عمره ما بيحتاج استرجاع القاعدة | U1 وU7 |
| 8 | الـ backfill الكبير بيشتغل في الخلفية من التطبيق على دفعات، وتقدّمه في `"Backfills"`، ويكمّل من مكانه. والـ contract بيتأكد إنه خلص قبل ما يشيل أي حاجة | migration طويل وقت الإقلاع معناه السيرفر واقف | U4 |
| 9 | الـ dry run جوه transaction واحدة بـ `lock_timeout` على القاعدة الحقيقية، جوه النافذة، وبعدها `ROLLBACK`. والـ `NO TRANSACTION` بيتعرض «مش متجرّب» | ده اللي Postgres بيسمح بيه. والأقفال هي هي اللي الـ migration الحقيقي هياخدها بعدها بدقايق | U2 |
| 10 | النسخة الاحتياطية: `pg_dump -Fc` و`/etc/dhow` كله (فيه `DHOW_SECRET_KEY`)، في ملف واحد مشفّر بـ AES-256-GCM على chunks بمفتاح للنسخ بس (`/etc/dhow/credentials/dhow-backup-key`، root و`0600`)، وبحد للحجم وتدوير | النسخة من غير المفتاح السري مالهاش لازمة، ومفتاح منفصل عشان تسريب واحد منهم مايكفيش | U5 |
| 11 | حالة الـ feed (الـ metadata الموثوقة وأرقامها، وآخر وقت اتشاف) في جدول `"UpdateFeedState"` | الحماية من الرجوع لازم تفضل بعد إعادة التشغيل والترقية | U8 |
| 12 | الفحص في السيرفر (من غير صلاحيات)، كل 6 ساعات ± 30 دقيقة، وUser-Agent ثابت `Dhow` من غير نسخة | الشبكة في الـ process الأقل صلاحية، والطلب نفسه لكل الـ instances | U8 |
| 13 | مجموعة إعدادات `updates` في القاعدة (نمط ADR-0063)، وenv يكسب ويقفل: `DHOW_UPDATE_CHECK` و`DHOW_UPDATE_POLICY` و`DHOW_UPDATE_WINDOW` و`DHOW_UPDATE_URL` (env بس) | نفس نمط الإعدادات الموجود، والـ GitOps يقدر يثبّتها | U8 |
| 14 | الـ major عمره ما بيتثبّت أوتوماتيك، حتى في سياسة «كله» | الـ major ممكن يشيل حاجات محتاجة تدخّل بشري. الإشعار بيقول كده، وSabry يقدر يرجع فيها | U10 |
| 15 | ملاحظات الإصدار في `docs/releases/X.Y.Z.md` و`docs/en/releases/X.Y.Z.md` بعناوين ثابتة، ومنها JSON بيتضمّن في الـ binary | «الجديد» بيشتغل في المعزول، وملف Markdown في الـ root ممنوع (CLAUDE.md) | U7 |
| 16 | الإيقاف من الـ API بـ `Deprecation` (RFC 9745) و`Sunset` (RFC 8594) و`Link: rel="deprecation"`، وجوه `/api/v1` إضافات بس | معايير، والـ clients تقدر تكتشفها لوحدها | U12 |
| 17 | الـ runner بيعلن `Dhow-Protocol-Max`، والسيرفر بيقبل N وN-1 | الـ runner الأحدث والأقدم الاتنين بيشتغلوا | U11 |
| 18 | الـ runner بينزّل التحديث من السيرفر (mirror)، والـ helper بتاعه بيتحقق من الـ TUF والطول والـ hash بنفسه | أجهزة كتير مالهاش internet غير للسيرفر، والسيرفر مخزن غبي مش مصدر ثقة | U11 |
| 19 | شكل قيمة الإعداد في `"InstanceSettings"` مابيتغيّرش. الشكل الجديد = مفتاح جديد، والقديم بيتشال بـ contract | الـ binary القديم بيقفل لو لقى قيمة مش فاهمها (fail closed في ADR-0063) | U12 |
| 20 | الواجهة بتعرف إن السيرفر اتحدّث من `ServerBuild` في رد `/me` (مش header عام)، وبتقول «أعد تحميل الصفحة» | المتصفح بيفضل شايل الـ JS القديم بعد الترقية، ومش عايزين نكشف النسخة لغير المسجّلين | U6 |

## 1. فحص التحديثات

### الـ feed
**الشكل:** TUF repository ([قرار 1 و2 و5](#قرارات-sabry-2026-11-02)). كله على ريبو `Khaymah/dhow` نفسه، من غير ريبو جديد ولا دومين:
- **الـ metadata** (`root` و`targets` و`snapshot` و`timestamp`، كلها صغيرة) على GitHub Pages بتاع الريبو تحت `/updates/`: `https://khaymah.github.io/dhow/updates/`. GitHub بيكتب host الـ Pages بحروف صغيرة (`<owner>.github.io`)، والـ URL ده هو اللي بيتثبّت في الـ binary (`internal/updatefeed`). الـ Pages لسه مش متفعّل على الريبو (`404` يوم 2026-11-02)، وتفعيله ببلاش للريبو العام في U7.
- **الـ targets** هي الـ release assets الموجودة النهارده: الـ base `https://github.com/Khaymah/dhow/releases/download/`، والـ target path `vX.Y.Z/<file>`، نفس اللي `get.sh` بينزّله. ومستند النسخة `vX.Y.Z/dhow-release.json` بيترفع asset مع الأرشيفات.
- **`root.json`:** مفتاحين root (threshold 1 من 2 في الأول)، والنسخة الأولى متضمّنة في الـ binary (`internal/updatefeed/root/1.root.json`). تغيير الـ root بيبقى سلسلة `N+1.root.json` موقّعة بالقديم والجديد، زي TUF بالظبط.
- **`targets.json`:** مفتاح targets offline، Sabry بيوقّع بيه مرة مع كل release على جهازه ([الـ ceremony](#مفاتيح-الـ-feed-والـ-ceremony))، وصلاحيته 180 يوم. فيه لكل ملف الطول والـ sha256.
- **`snapshot.json` و`timestamp.json`:** مفاتيح online في GitHub environment محمي (`release-feed`)، وصلاحيتهم 7 أيام، و`timestamp` بيتعاد توقيعه كل أسبوع ([النشر](#النشر-على-github-pages)).
- **`releases/<version>.json`** (target)، 64 KiB بالكتير:

| الحقل | المعنى |
|---|---|
| `Version` و`Date` و`Channel` | semver، و`stable` أو `rc` |
| `Kind` | `patch` أو `minor` أو `major` (بيتحسب في CI من النسخة اللي قبلها، مش بيتكتب بإيد) |
| `Security` | `{Severity, Advisories[]}`. `Severity` من `none` و`low` و`moderate` و`high` و`critical`. والـ advisories أرقام GHSA، وممكن تبقى فاضية لحد الإعلان (embargo) |
| `RequiresFrom` | أقل نسخة ينفع تترقّى منها مباشرة. لو المتنصّب أقدم، الـ updater بيعدّي على النسخة دي الأول |
| `Contract` و`SchemaFloor` | فيه contract migrations ولا لأ، والـ floor بعدها |
| `Artifacts[]` | `{Component, Os, Arch, Name, Sha256, Size}` لكل أرشيف، والصورة `{Repository, IndexDigest}` |
| `Runner` | بروتوكولات الـ runner المدعومة (`[1, 2]`) |
| `Spec` | نسخ `.dhow.yml` المدعومة (`[1]`) |
| `Notes` | `{Ar, En}`، نص عادي لحد 2 KiB لكل لغة، من غير HTML. والتفاصيل في `NotesPath` (صفحة الوثائق) |
| `Actions[]` | خطوات لازم المالك يعملها (مفتاح i18n ومعاه رابط)، زي «غيّر `DHOW_X` لـ `DHOW_Y`» |

**الأرشيفات نفسها** بتفضل على GitHub Releases زي النهارده. الـ feed بيربطها بالطول والـ sha256، وده التحقق الوحيد في الـ updater ([قرار 3](#قرارات-sabry-2026-11-02)): مفيش تحقق من cosign جواه. `release.yml` بيفضل يوقّع بـ cosign للناس اللي بتتحقق بإيدها و`get.sh`. وأي mirror (`DHOW_DOWNLOAD_URL`) مايقدرش يبدّل الأرشيف.

### الطلب بيكشف إيه
بالظبط ولا حاجة زيادة:
- **عنوان الـ IP** (أو عنوان الـ proxy لو `HTTPS_PROXY` متحدد)، و**وقت** الطلب، و**اسم الـ host** في الـ SNI (`khaymah.github.io`، ولما بينزّل أرشيف `github.com` والـ CDN بتاع الـ assets بتاعهم). ده بيقول لـ GitHub إن فيه instance داو على العنوان ده، وإحنا مابنشوفش حاجة (مفيش logs عندنا أصلًا). مفيش طريقة نخبّيها غير الـ proxy أو القفل.
- **مفيش:** instance id، ولا cookie، ولا query string، ولا رقم النسخة، ولا عدد المستخدمين. الـ User-Agent ثابت `Dhow` لكل الـ instances، والملفات اللي بتتطلب واحدة للكل.
- **الإيقاع:** كل 6 ساعات ± 30 دقيقة عشوائي. `snapshot` و`targets` و`releases/*` بيتطلبوا بس لما `timestamp` يتغيّر، فالنمط ممكن يقول «الـ instance دي ماشافتش 1.4.2 لسه»، ودي معلومة ضعيفة ومقبولة.
- **مكتوب للمستخدم** في `docs/install/updates.md` (بالنسختين) وفي صفحة «عن داو».

### التحقق
- **TUF كامل** بـ `go-tuf/v2`: الـ root المتضمّن، وسلسلة الـ root، والـ thresholds، والـ expiry، والـ versions، والـ hashes والأطوال.
- **الحماية من الرجوع (rollback):** أرقام الـ metadata الموثوقة متخزنة في `"UpdateFeedState"`، وأي رقم أقل بيترفض. والـ client كمان مابيقبلش `timestamp` وقت توقيعه أقدم من آخر واحد اتشاف، عشان ساعة الجهاز لو رجعت لورا ماتفتحش الباب.
- **الحماية من التجميد (freeze):** `timestamp` بيخلص بعد 7 أيام. من غير تحقق ناجح لـ 3 أيام، صفحة «عن داو» بتحذّر. وبعد ما يخلص: بانر للمالك «لم نتمكن من التأكد من التحديثات منذ ...»، وfinding في `doctor`، **وأي تثبيت أوتوماتيك بيقف**.
- **ساعة الجهاز:** الـ client بيقارن بالـ `Date` header في رد الـ feed. فرق أكتر من 5 دقايق بيطلّع finding «ساعة الجهاز غير مضبوطة» في `doctor`.
- **الحدود:** `timestamp` 16 KiB، و`snapshot` 64 KiB، و`targets` 1 MiB، وكل طلب 30 ثانية، وTLS 1.2 على الأقل، والاتصال من `internal/netguard` (مفيش عناوين داخلية إلا لو `DHOW_UPDATE_URL` على نطاق مسموح).
- **الـ redirects:** الـ metadata من غير redirects خالص. والأرشيف: GitHub بيعمل redirect من `github.com` للـ CDN بتاع الـ assets، فالمسموح قايمة ثابتة من hosts GitHub بس، والسلامة أصلًا من الطول والـ hash.

### مفاتيح الـ feed والـ ceremony
كل المفاتيح ملفات، ومفيش hardware ([قرار 2](#قرارات-sabry-2026-11-02)).

| المفتاح | فين | بيوقّع إيه |
|---|---|---|
| root (2 مفاتيح) | offline، ملفات مشفّرة على USBين | `root.json` بس، عند الإنشاء والتغيير |
| targets | offline، ملف مشفّر على نفس الـ USBين | `targets.json` مع كل release |
| snapshot وtimestamp | GitHub environment `release-feed` (secrets) | كل أسبوع ومع كل release |

**الإنشاء (مرة واحدة، في U7):**
1. Sabry بيعمل boot لجهازه من USB عليه Debian Live (ببلاش، والـ checksum والتوقيع بتوع الـ ISO متراجعين من جهاز تاني)، والشبكة مقفولة (من غير Wi-Fi ولا كابل).
2. `dhow-feed keys init` (من `deploy/release/feed`، binary متبني ومتراجع قبلها) بيعمل مفتاحين root ومفتاح targets (ed25519)، وبيشفّر كل واحد بـ passphrase بـ `age` (scrypt)، وبيطلّع `1.root.json` والمفاتيح العامة.
3. **الـ passphrase:** 7 كلمات diceware أو أكتر، Sabry حافظها، ونسخة على ورقة في مكان **غير** مكان الـ USB الاحتياطي. عمرها ما بتتكتب على أي قرص.
4. الملفات المشفّرة و`1.root.json` و`SHA256SUMS` بتتنسخ على USBين: **الأساسي** مع Sabry، و**الاحتياطي** في مكان تاني (بيت تاني أو حد موثوق). والملف المشفّر مابيتنسخش على أي جهاز متوصّل بالإنترنت.
5. `1.root.json` (عام) بيتعمله commit في `internal/updatefeed/root/` بـ PR عادي، والـ snapshot والـ timestamp بيتعملوا online في الـ environment.

**توقيع release:**
1. `release.yml` بيبني ويرفع الأرشيفات و`dhow-release.json` كـ assets، وبيطلّع `targets.json` مش موقّع كـ artifact، وبيطبع ملخّص الـ hashes في الـ log.
2. Sabry على جهازه العادي: بينزّل الـ artifact، ويتأكد بـ `cosign verify-blob` من `SHA256SUMS` (اختياري ومفيد، cosign ببلاش)، وينقله لـ USB نقل.
3. boot من Debian Live من غير شبكة، و`dhow-feed sign-targets` بيعرض النسخة وكل ملف بالـ hash. Sabry بيقارن الملخّص ده بالـ log من جهاز تاني (الموبايل)، وبعدين بيكتب الـ passphrase.
4. `targets.json` الموقّع بيرجع على USB النقل، وبيترفع لـ branch `tuf-metadata` (القسم الجاي)، والـ CI بيوقّع snapshot وtimestamp وبينشر.

**التغيير بعد فقد:**
| اللي حصل | العمل |
|---|---|
| USB ضاع أو باظ ومحدش خده | نسخة جديدة من الـ USB التاني، ومفيش rotate |
| USB اتسرق أو ممكن يكون اتنسخ | rotate لـ targets والـ root: `2.root.json` موقّع بالـ root القديم والجديد، ومفتاح targets جديد، وrelease patch عادي. الـ passphrase بيدّي وقت، والـ rotate في نفس الأسبوع |
| جهاز Sabry ممكن يكون اتخترق وقت توقيع | نفس الـ rotate، ومراجعة كل `targets.json` من وقت الشك قدّام `SHA256SUMS` الموقّع بـ cosign، وإعلان أمني لو فيه فرق |
| مفتاح snapshot أو timestamp اتسرّب | rotate في `root.json` (بالمفتاح الـ offline) وsecrets جديدة في الـ environment |
| الـ passphrase اتنست والورقة ضاعت، أو الـ USBين راحوا | مفيش طريقة نوقّع root جديد: release بـ root جديد متضمّن، وإعلان بالترقية اليدوي بـ `get.sh` (اللي بيتحقق بـ cosign). ده أسوأ حالة، وعشان كده الورقة والـ USB الاحتياطي في مكانين |

الـ ceremony كله مكتوب في `docs/security/release-keys.md` بالنسختين (U7).

### النشر على GitHub Pages
- **مصدر الـ metadata:** branch يتيم `tuf-metadata` في نفس الريبو، فيه الملفات الموقّعة بس. بيكتب فيه Sabry (`targets.json` و`root.json`) وjobs الـ environment `release-feed` (`snapshot` و`timestamp`). محمي (من غير force push ولا مسح). وده branch بيانات مش شغل، فمالوش PR، و`delete-merged-branch.yml` مابيلمسوش لأنه عمره ما بيتدمج.
- **workflow واحد للـ Pages** (`.github/workflows/pages.yml`): بيبني الموقع كله في كل مرة من مصدرين: موقع الوثائق (M10) على `/dhow/` من `main`، والـ metadata على `/dhow/updates/` من `tuf-metadata`. بيشتغل مع أي push على الاتنين، ومن `release.yml`، ومن job الـ timestamp. لأن نشر Pages بيستبدل الموقع كله، مفيش workflow تاني ينشر لوحده.
- **تجديد الـ timestamp:** job مجدول كل أسبوع بيعيد توقيع `timestamp.json` (وصلاحيته 7 أيام) وبينشر. ولو فشل: الـ job بيفتح issue بـ label `feed-expiry` (بالـ `GITHUB_TOKEN`)، وGitHub بيبعت إيميل الفشل.
- **هامش الأمان:** الأسبوع والـ 7 أيام قريبين من بعض، وGitHub ممكن يأخّر الـ jobs المجدولة ساعات أو يوقفها في الريبو اللي مافيهوش نشاط 60 يوم. فـ job تاني يومي بيفحص بس: لو الصلاحية فاضلها أقل من 48 ساعة بيعيد التوقيع على طول وبيفتح الـ issue. العملاء كمان بيستحملوا: بيحذّروا بعد 3 أيام من غير تحقق ناجح، والأوتوماتيك بيقف بس بعد ما الصلاحية تخلص.
- **تغيير الـ URL بعدين** (لو اتنقلنا لدومين في يوم): محتاج release بالـ URL الجديد، والنسخ القديمة هتفضل تقرا القديم. فالـ URL القديم لازم يفضل يتجدّد لحد ما آخر نسخة بتقراه تبطّل تبقى مدعومة.

### الـ cache والـ backoff
- `If-None-Match` و`ETag`، والـ `304` مابيغيّرش حاجة غير `LastCheckAt`.
- الفشل: backoff من 5 دقايق بيتضاعف لحد 6 ساعات، و`Retry-After` بيتحترم لحد 24 ساعة. الخطأ بيتخزّن ككود بس (`FEED_UNREACHABLE` و`FEED_SIGNATURE_INVALID` و`FEED_EXPIRED` و`FEED_ROLLBACK` و`FEED_TOO_LARGE`)، مش رسالة من بره.
- instance واحدة بس بتفحص في المرة، بـ advisory lock (`pg_try_advisory_lock`)، والباقيين بيقروا الجدول.
- زرار «تحقّق الآن» للمالك، مرة كل 5 دقايق بالكتير.

### القفل للمعزول
- `DHOW_UPDATE_CHECK=off` في env (بيقفل الإعداد زي ADR-0063)، أو الإعداد `updates.check` من اللوحة.
- **مقفول يعني مقفول:** الـ worker مابيتعملّوش start أصلًا، والـ helper مابيتصلش بالشبكة، و«عن داو» بتقول «التحقق من التحديثات مقفول» ومعاها خطوات الترقية اليدوي.
- الـ installer بيسأل «هل يصل هذا السيرفر إلى الإنترنت؟»، ولو لأ بيكتب `DHOW_UPDATE_CHECK=off` في `dhow.env` أو `.env`.
- **اختبار:** السيرفر بيقوم بـ `DHOW_UPDATE_CHECK=off` وdialer بيفشّل الاختبار لو أي اتصال خارجي اتعمل في دقيقة (ساعة وهمية)، وكمان الـ helper بنفس الإعداد.

## 2. السياسة والنافذة

### الإعدادات (مجموعة `updates`، لمالك السيرفر بس، بخطوة تانية حديثة، وكل تعديل في الـ audit)
| الإعداد | القيم | الافتراضي | env |
|---|---|---|---|
| `Check` | `on` أو `off` | `on` | `DHOW_UPDATE_CHECK` |
| `Policy` | `notify` أو `security` أو `all` | `security` | `DHOW_UPDATE_POLICY` |
| `Window` | أيام الأسبوع، والبداية `HH:MM`، والمدة 60 لـ 480 دقيقة، والمنطقة الزمنية IANA | كل يوم 03:00 لمدة 120 دقيقة بتوقيت UTC، لحد ما المالك يختار | `DHOW_UPDATE_WINDOW` (مثلًا `mon-fri 03:00/120 Africa/Cairo`) |
| `CriticalOutsideWindow` | `true` أو `false` | `false` ([قرار 6](#قرارات-sabry-2026-11-02)) | — |
| `Channel` | `stable` أو `rc` | `stable` | `DHOW_UPDATE_CHANNEL` |

`DHOW_UPDATE_URL` في env بس (mirror داخلي)، وبيتحقق بنفس الـ TUF.

### هيعمل إيه مع كل release
| الـ release | `notify` | `security` (الافتراضي) | `all` |
|---|---|---|---|
| patch أمني | إشعار | **تثبيت أوتوماتيك جوه النافذة** | تثبيت أوتوماتيك |
| patch عادي | إشعار | إشعار وزرار | تثبيت أوتوماتيك |
| minor | إشعار وزرار | إشعار وزرار | تثبيت أوتوماتيك |
| major | إشعار وزرار، وتأكيد «قرأت الملاحظات» | نفس الشيء | نفس الشيء (قرار 14) |
| `rc` | بس لو `Channel = rc`، ومن غير أوتوماتيك أبدًا | | |

- **الـ release الأمني:** نسخة patch (الرقم الأخير بس اتغيّر) في الـ feed بـ `Security.Severity` من `low` لفوق. وفيه expand migrations بس (قرار 7)، ومفيش إعداد جديد إجباري، ومفيش رفع لبروتوكول الـ runner. الـ release اللي مايعدّيش الشروط دي مايتعلّمش أمني حتى لو فيه إصلاح أمني، وبيبقى إشعار وزرار.
- **أكتر من release:** بيتثبّت آخر واحد مسموح بالسياسة. ولو `RequiresFrom` أعلى من المتنصّب، الـ updater بيعدّي على النسخة دي الأول، ويستنى الـ backfills تخلص، وبعدين يكمّل (في نفس النافذة لو الوقت كفاية، وإلا في اللي بعدها).
- **التأجيل:** المالك يقدر يأجّل تحديث أمني أوتوماتيك مرة واحدة لحد 7 أيام، ومكتوب في الـ audit.
- **الـ instance مش سليمة:** مفيش تثبيت أوتوماتيك وفحص الجاهزية فاشل، والإشعار بيقول ليه.
- **شغل شغّال:** قبل الـ swap السيرفر بيبطّل يبدأ خطوات `mutating` جديدة (نشر M6)، ويستنى الشغّال لحد 15 دقيقة. لو ماخلصش، التحديث بيتأجّل للنافذة اللي بعدها. والـ patch الأمني اللي اتأجّل 3 نوافذ بيبقى إشعار عاجل «محتاج تدخّل». الـ builds العادية بتكمّل على الـ runners، والـ runner بيستحمل وقوع السيرفر (سيناريوهات الفوضى في ADR-0058).
- **أكتر من instance:** لو جدول `"ServerInstances"` فيه instance تانية شافت القاعدة في آخر دقيقتين، التثبيت الأوتوماتيك والزرار الاتنين بيقفوا، والإشعار بيشرح الترقية اليدوي بالترتيب.

### الإشعارات
- **المالك:** بانر في `/system` وكارت في الرئيسية لما يبقى فيه تحديث، وبعد أي تثبيت أو رجوع. وإيميل (وقنوات M7 لما تبقى موجودة) للتحديث الأمني، وللرجوع، وللفشل اللي محتاج تدخّل.
- **باقي الأعضاء:** مابيشوفوش إشعارات التحديث. بيشوفوا «الجديد» بس بعد الترقية (القسم 10).
- **الـ audit:** `system.update.available` و`system.update.requested` (بالمستخدم أو `policy`) و`system.update.started` و`system.update.succeeded` و`system.update.rolled_back` (بالسبب والخطوة) و`system.update.failed` و`system.update.postponed` و`system.update.settings.changed`.

## 3. الـ updater

### فصل الصلاحيات حسب طريقة التنصيب
| الطريقة | الـ helper | بيكتب إيه | الثمن |
|---|---|---|---|
| **binary وsystemd** (`dhow-server install`) | `dhow-update.service` (oneshot، root)، بيشغّله `dhow-update.path` (على `request.json`) و`dhow-update.timer` (كل 15 دقيقة للحاجات المتأخرة)، و`dhow-update-recover.service` قبل `dhow.service` | `/usr/local/bin/dhow-server` و`/etc/systemd/system/dhow*.service` و`/var/lib/dhow-update/` | مفيش ثمن جديد: الـ installer أصلًا بيشتغل root. الـ unit نفسها بـ `ProtectHome=yes` و`PrivateTmp=yes` و`NoNewPrivileges=yes` و`ReadWritePaths=` للمسارات دي بس و`RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX` و`SystemCallFilter=@system-service` |
| **Compose بالـ installer** | نفس الـ units على الـ host (مش جوه container)، والـ swap = تغيير `DHOW_IMAGE` في `.env` لـ `ghcr.io/khaymah/dhow@sha256:<digest>` و`docker compose up -d dhow` بـ argv | `.env` بس، وبـ rename ذرّي بيحافظ على باقي السطور | الـ helper بيكلّم Docker كـ root على الـ host، زي ما المشغّل بيعمل بإيده. مفيش socket جوه أي container |
| **Compose يدوي** | إشعار بس، والصفحة بتعرض الأوامر بالـ digest بالظبط | — | [قرار 4](#قرارات-sabry-2026-11-02): مفيش sidecar بالـ Docker socket |
| **مسار الـ installer** (`get.sh --version X`) | نفس `internal/updater`: الـ installer بقى بياخد نسخة احتياطية، ويعمل journal، ويفحص الصحة، ويرجع لو فشل | زي الـ binary | الترقية اليدوي والأوتوماتيك نفس الكود ونفس الاختبارات |
| **Kubernetes** (بعد أول نسخة) | مفيش. إشعار بس | — | الـ GitOps هو اللي بيحدّث |

**السيرفر نفسه:** `User=dhow` و`ProtectSystem=strict` في systemd، و`read_only: true` في Compose، وده بيفضل زي ما هو. مفيش أي مسار يخلّي process السيرفر يكتب binary أو unit أو `.env`. والاختبار بيتأكد إن `internal/updater` مش في الـ import graph بتاع وضع السيرفر.

### الطلب (`/var/lib/dhow/update/request.json`، بيكتبه السيرفر)
```json
{"Schema": 1, "RequestId": "0193b0c2-6a1e-7c3e-9a51-3f0f6c1d2b7a", "Version": "1.4.2", "Reason": "policy", "NotBefore": "2026-11-03T03:00:00Z", "NotAfter": "2026-11-03T05:00:00Z"}
```
- `Reason` من `policy` أو `user` أو `rollback`. والـ helper بيرفض أي حقل زيادة، وأي `Version` مش semver، وأي ملف أكبر من 4 KiB أو symlink أو مش مملوك لـ `dhow`.
- **قواعد الـ helper اللي مابتتكسرش، مهما الطلب قال:** النسخة لازم تبقى في feed موقّع وصالح، وأحدث من المتنصّب. الاستثناء الوحيد `rollback` للنسخة اللي في الـ journal بتاعه هو، وبالـ binary المحفوظ عنده. وأي تحقق فاشل = مفيش تغيير.
- **الحالة** (`status.json`، بيكتبها الـ helper و`dhow` بيقراها): `RequestId` والخطوة والنتيجة والكود والأوقات والنسخة القديمة والجديدة وهل القاعدة اترجعت. السيرفر بيحوّلها لـ audit وإشعار أول ما يقوم.

### الخطوات
| # | الخطوة | الفشل هنا معناه |
|---|---|---|
| 0 | `flock` على `/var/lib/dhow-update/lock`، وقراية الـ journal. لو فيه journal ناقص: `recover` الأول | تحديث شغّال: الطلب التاني بيستنى ومابيتعملّوش تكرار |
| 1 | جلب الـ feed والتحقق (TUF)، واختيار الأرشيف حسب المعمارية | مفيش تغيير، `FEED_*` |
| 2 | تنزيل لـ `/var/lib/dhow-update/stage/` (نفس filesystem الـ binary أو الـ rename بيبقى copy+fsync+rename)، والطول والـ sha256 من `targets` بس، من غير cosign ([قرار 3](#قرارات-sabry-2026-11-02)) | مفيش تغيير، الـ stage بيتمسح |
| 3 | **preflight:** المساحة (حجم القاعدة × 1.3 + الأرشيف + 512 MiB احتياطي)، وPostgreSQL 18 و`pg_dump` بنفس الـ major، والـ binary الجديد `migrate dry-run` كـ `User=dhow` (مش root) بـ `lock_timeout = 3s`، و`SchemaFloor` الجديد ≤ النسخة الجديدة، ومفيش instance تانية | مفيش تغيير، `PREFLIGHT_*` بالسبب |
| 4 | **النسخة الاحتياطية** (U5) والتأكد منها (`pg_restore --list` على الملف بعد فك التشفير في الذاكرة) | مفيش تغيير، `BACKUP_FAILED` |
| 5 | **quiesce** (القسم 2)، وبعدين الـ journal: `{from, to, backup, binary_previous, step: "swap"}` بـ fsync | الـ journal بيتمسح، والتأجيل للنافذة اللي بعدها |
| 6 | **swap:** نسخ الـ binary القديم لـ `/var/lib/dhow-update/previous/dhow-server-<v>`، و`rename` للجديد فوق `/usr/local/bin/dhow-server` و`fsync` للمجلد، والـ unit لو اتغيّرت مع `daemon-reload`، و`systemctl restart dhow` | الـ recover بيكمّل من الـ journal |
| 7 | **فحص الصحة:** `/health/ready` بيرجّع 200 في 120 ثانية، و`/meta` بيقول النسخة الجديدة، وبيفضل ready 60 ثانية متواصلين، و`NRestarts` في systemd ماتغيّرش | رجوع (الجدول اللي تحت) |
| 8 | **النجاح:** `status.json`، والـ journal بيتقفل، والـ binary القديم بيتحفظ 7 أيام عشان «ارجع للنسخة السابقة» اليدوي | — |

### الرجوع الأوتوماتيك
| الحالة وقت الفشل | الرجوع |
|---|---|
| الـ binary الجديد وقع قبل أي migration | الـ binary القديم بس |
| migrations اتطبّقت كلها أو جزء منها، والـ floor ماتغيّرش (expand بس) | الـ binary القديم بس، والقاعدة بتفضل متقدّمة. «عن داو» بتقول «القاعدة أحدث من النسخة بـ k تحديثات» وده سليم |
| الـ floor اترفع (contract، في minor أو major بس) | وقف `dhow`، واسترجاع القاعدة من نسخة الخطوة 4، والـ binary القديم، وتشغيل، وفحص صحة. والـ audit بيقول الوقت من الـ swap للاسترجاع: **أي كتابة في الفترة دي راحت** (webhooks وصلت وruns اتعملت)، والإشعار بيقول كده صراحة |
| `NO TRANSACTION` migration وقع في النص | الرجوع زي expand، والـ migrator في المحاولة الجاية بيشيل الـ index اللي `indisvalid = false` باسمه قبل ما يعيد |
| الرجوع نفسه فشل | `dhow` بيفضل واقف (fail closed)، والحالة `NEEDS_ATTENTION`، ورسالة في `journalctl -u dhow-update` وفي `/etc/motd.d/50-dhow-update` (أول ما حد يدخل SSH يشوفها) بالأوامر بالظبط. مفيش retry أوتوماتيك |

- النسخة اللي رجعنا منها بتتعلّم «فشلت على الجهاز ده» ومابتتجرّبش تاني أوتوماتيك. أي patch أحدث منها مسموح.
- **«ارجع للنسخة السابقة» اليدوي:** للمالك لحد 7 أيام، بخطوة تانية. لو الـ floor بيسمح: الـ binary بس. ولو لأ: الزرار بيقول إن القاعدة هترجع لوقت الترقية، والبيانات من ساعتها هتروح، ومحتاج تأكيد بكتابة رقم النسخة.

### مسار الـ installer
`get.sh --version X` و`dhow-server install` على جهاز فيه داو بينادوا `internal/updater` نفس الخطوات من 3 لـ 8 (الـ installer أصلًا root). يعني الترقية اليدوي بقت بنسخة احتياطية ورجوع، و`--no-backup` موجود بتأكيد صريح. والـ installer بيركّب `dhow-update.{service,path,timer}` و`dhow-update-recover.service` ومفتاح النسخ الاحتياطي، وبيسأل عن السياسة والنافذة والاتصال بالإنترنت، وبيحترم ملف الإجابات.

### HA وأكتر من instance
- **جدول `"ServerInstances"`:** كل instance بتكتب heartbeat كل 30 ثانية (الاسم من الـ hostname والـ PID، والنسخة، و`LastSeenAt`). ده بيظهر في «عن داو» كمان.
- **أول نسخة:** لو فيه أكتر من instance شافت القاعدة في آخر دقيقتين، الـ updater إشعار بس.
- **مع HA في M8:** الترقية بالدور، instance واحدة في المرة. ده آمن بقاعدة N-1، والـ contract migration بيستنى لحد ما كل instance بقت على نسخة ≥ الـ floor (الـ migrator بيقرا `"ServerInstances"` ويرفض بـ `SCHEMA_FLOOR_BLOCKED_BY_INSTANCE`).

## 4. النسخ الاحتياطي
- **الأوامر** (root): `dhow-server backup create [--out DIR]` و`list` و`verify FILE` و`restore FILE`، وفي Compose نفس الأوامر من الـ host بتنادي `docker compose exec -T postgres pg_dump` بـ argv.
- **المحتوى:** `pg_dump --format=custom` للقاعدة كلها (فيها الـ logs والـ audit)، و`/etc/dhow` كله أو `.env` في Compose (فيه `DHOW_SECRET_KEY`)، و`manifest.json` (النسخة، والـ schema version، والـ floor، والتاريخ، والـ sha256 لكل جزء). **مش فيها:** `DHOW_BLOB_DIR` (الـ artifacts والـ cache بتاعة M4 بتتعمل تاني)، ومكتوب في الصفحة.
- **الملف:** `dhow-backup-<version>-<UTC timestamp>.dhowbak`، صيغة بسيطة: header ثابت ورقم الصيغة، وبعده chunks كل واحد 1 MiB بـ AES-256-GCM و nonce = عدّاد، وآخر chunk معلَّم (STREAM)، عشان القطع أو إعادة الترتيب يتكشف. من الـ standard library بس.
- **المفتاح:** `/etc/dhow/credentials/dhow-backup-key` (32 byte عشوائي، root، `0600`)، بيعمله الـ installer. مش `DHOW_SECRET_KEY`، عشان تسريب واحد منهم لوحده مايكفيش. والمجلد `/var/lib/dhow-update/backups` بـ `0700` لـ root.
- **الحجم والتدوير:** آخر 3 نسخ قبل الترقية، وبحد إجمالي (`DHOW_BACKUP_MAX_BYTES`، الافتراضي أقل قيمة بين 20% من المساحة الفاضية وقت التنصيب و10 GiB). نسخة النسخة المتنصّبة دلوقتي مابتتمسحش قبل 7 أيام من نجاح الترقية اللي بعدها. لو المساحة مش كفاية: الترقية بتتأجّل بإشعار «فضّي مساحة أو كبّر الحد»، ولـ الـ patch الأمني فيه زرار «ثبّت من غير نسخة احتياطية» للمالك بخطوة تانية وتأكيد.
- **قاعدة خارجية مُدارة** (RDS وغيره): الـ installer بيسأل. لو المالك اختار «المزوّد بياخد snapshots»، الـ updater بيعمل نسخة لـ `/etc/dhow` بس، ومفيش استرجاع أوتوماتيك، فالـ release اللي فيه contract مش أوتوماتيك أبدًا والإشعار بيقول خد snapshot.
- **جدولة اختيارية:** `dhow-backup.timer` يومي، مقفول افتراضيًا، والـ installer بيسأل.

## 5. قاعدة الـ migrations (expand/contract)

### القاعدة (شغّالة من النهارده، ADR-0077)
1. **schema النسخة N لازم يشتغل عليه binary النسخة N وN-1.** يعني الـ binary يرجع خطوة من غير ما القاعدة ترجع، والـ instances القديمة (HA) تفضل شغّالة وقت الترقية.
2. **`expand` (أي release):** إضافات بس:
   - جدول جديد، أو عمود nullable أو بـ `DEFAULT` ثابت (عشان الـ `INSERT` بتاع الـ binary القديم يعدّي).
   - index على جدول موجود بـ `CREATE INDEX CONCURRENTLY IF NOT EXISTS` في ملف `NO TRANSACTION` لوحده.
   - constraint على جدول موجود بـ `NOT VALID`، والـ `VALIDATE` في ملف `NO TRANSACTION` بعده (زي `00041` و`00042`).
   - function جديدة، أو `CREATE OR REPLACE` بنفس الـ signature وبسلوك يقبله الكود القديم.
   - قيمة جديدة في `CHECK ... IN (...)`: الكود القديم مابيكتبهاش، بس لازم يتحمّل يقراها (ده في الكود، والمراجعة بتسأل عليه).
3. **`contract` (minor أو major بس، وبعد release كامل من الـ expand):** أي حاجة بتشيل أو بتغيّر:
   - `DROP TABLE` أو `DROP COLUMN` أو `RENAME`.
   - `ALTER COLUMN ... TYPE` أو `SET NOT NULL` على عمود فيه بيانات.
   - `DROP FUNCTION` أو تغيير signature بيستخدمها الكود القديم (زي `EnsureLogPartitions`).
   - constraint جديد الكود القديم ممكن يكسره وهو بيكتب.
   - الـ contract بيرفع `"Instance"."SchemaFloor"` لأقل نسخة بطّلت تستخدم الحاجة القديمة.
4. **الترتيب لتغيير عمود** (مثال إعادة تسمية `A` لـ `B`):
   - **N:** expand بيضيف `B`، والكود بيكتب في الاتنين وبيقرا من `A`، والـ backfill بيبدأ.
   - **N+1:** بيقرا من `B` وبيكتب في `B` بس (`A` لازم nullable أو بـ default)، والـ backfill لازم يكون خلص.
   - **N+2:** contract بيشيل `A`، و`-- dhow:floor` = N+1.
5. **الـ backfill:** صغير (أقل من 10,000 صف ومعروف إنه صغير زي الإعدادات) ينفع جوه الـ expand. غير كده بيشتغل في الخلفية (U4): دفعات 1,000 صف بـ `FOR UPDATE SKIP LOCKED`، والتقدّم في `"Backfills"` (`"Name"` و`"LastKey"` و`"DoneAt"`)، تحت advisory lock، ويكمّل من مكانه بعد أي إعادة تشغيل. والـ contract اللي بيعتمد عليه بيبدأ بـ guard: لو `"DoneAt" IS NULL` بيفشل بـ `BACKFILL_NOT_DONE`، والـ feed بيحط `RequiresFrom` عشان الـ updater يعدّي على النسخة اللي بتعمله.
6. **الـ dry run:** `dhow-server migrate dry-run` بيطبّق الـ migrations المستنية جوه transaction واحدة، بـ `SET LOCAL lock_timeout = '3s'` و`statement_timeout = '5min'`، وبعدين `ROLLBACK`. ملفات `NO TRANSACTION` مابتتجرّبش وبتتعرض كده. والـ `--against-backup FILE` بيسترجع النسخة في قاعدة مؤقتة ويطبّق كل حاجة فيها (للقواعد الكبيرة واختبار الليل).
7. **الـ Down:** بيفضل في الملفات للاختبارات بس، والإنتاج عمره ما بينفّذه.
8. **قبل أول release:** مفيش نسخة حد يرجعلها، فالـ contract بيتكتب بـ `-- dhow:floor 0.0.0` وبيتراجع في الـ PR. من أول tag، الـ floor لازم يبقى tag موجود.

### توافق الـ schema في الـ binary (U2)
- **`Migrator.Current`** بقى بيرجّع حالة من تلاتة: `current`، و`ahead` (القاعدة أحدث والـ floor ≤ نسخة الـ binary)، و`incompatible`. الجاهزية بتنجح في `current` و`ahead`.
- **في الإقلاع:** `incompatible` بيوقف السيرفر برسالة فيها أقل نسخة مطلوبة، وبرمز خروج ثابت (`78`) و`RestartPreventExitStatus=78` في الـ unit، عشان systemd مايفضلش يعيد للأبد.
- **نسخة `dev`:** على قاعدة `ahead` بترفض إلا بـ `DHOW_DEV=true`.
- **الأوامر:** `dhow-server migrate status` (النسخة، والمستني، والـ floor، والـ backfills) و`plan` (كل ملف بالـ phase بتاعه) و`dry-run`.

### الـ hook (المهمة الأولى في الخطة، U1)
**الملفات:** `.claude/hooks/dhow_hooks/migration_rules.py`، ومتسجّل في `CHECKS` في `edit_rules.py`، والاختبارات في `.claude/hooks/tests/test_migration_hooks.py`.

**بيشتغل على:** أي ملف جديد في `apps/server/internal/Database/Migrations/*.sql`، أو ملف لسه مش على `main` (الملفات اللي اتدمجت ممنوع تتعدّل أصلًا بـ `check_migration_immutable`). والقسم اللي بيتفحص هو `-- +goose Up` بس.

**الـ parsing:** شيل الكومنتات والـ strings والـ `$$...$$` من النص قبل البحث، وقسّم الـ statements على `;` بره الـ quotes، واحترم `-- +goose StatementBegin/End`. الجداول اللي اتعملت بـ `CREATE TABLE` في نفس الملف «جديدة»، وأي حاجة عليها مسموحة.

**القواعد:**
| # | القاعدة | الرسالة بتقول |
|---|---|---|
| M1 | أول سطر غير فاضي `-- dhow:phase expand` أو `-- dhow:phase contract` (بعد `-- +goose NO TRANSACTION` لو موجود) | ضيف الـ phase، ورابط ADR-0077 |
| M2 | في `expand`: ممنوع `DROP TABLE` و`DROP COLUMN` و`RENAME` و`ALTER COLUMN ... TYPE` و`SET NOT NULL` و`DROP FUNCTION` و`DROP TYPE` و`TRUNCATE` و`DELETE` على جدول مش جديد | ده contract، استنى release |
| M3 | في `expand`: `ADD COLUMN ... NOT NULL` من غير `DEFAULT` على جدول مش جديد | الـ `INSERT` بتاع النسخة القديمة هيفشل |
| M4 | `ADD CONSTRAINT` (CHECK أو FOREIGN KEY) على جدول مش جديد من غير `NOT VALID` | `NOT VALID` هنا و`VALIDATE` في ملف `NO TRANSACTION` |
| M5 | `CREATE INDEX` على جدول مش جديد من غير `CONCURRENTLY IF NOT EXISTS`، أو `CONCURRENTLY` في ملف من غير `NO TRANSACTION` | قفل الجدول طول بناء الـ index |
| M6 | `UPDATE` في `expand` من غير `-- dhow:backfill small` | الـ backfill الكبير في الخلفية (U4) |
| M7 | `contract` من غير `-- dhow:floor X.Y.Z`، أو من غير `UPDATE "Instance" SET "SchemaFloor" = 'X.Y.Z'` بنفس القيمة (مع `0.0.0` الـ UPDATE مش مطلوب، لأن العمود مش موجود لحد U2 وقيمته الافتراضية `0.0.0`، ولو موجود لازم يبقى بنفس القيمة) | الـ floor هو اللي بيحمي الـ binary القديم |
| M8 | `-- dhow:floor` مش `0.0.0` ومفيش tag `vX.Y.Z` في الريبو | الـ floor نسخة اتعمل لها release |
| M9 | ملف `NO TRANSACTION` فيه statement مش idempotent (`IF NOT EXISTS` أو `IF EXISTS` أو `VALIDATE`) | الملف ده ممكن يقف في النص ويتعاد |
| M10 | أي statement بيتحكم في الـ transaction (`BEGIN` و`START TRANSACTION` و`COMMIT` و`END` و`ROLLBACK` و`ABORT` و`SAVEPOINT` و`RELEASE` و`PREPARE TRANSACTION`)، حتى جوه `StatementBegin/End`، إلا جسم `BEGIN ATOMIC` | goose بيشغّل كل ملف في transaction، والـ dry run معتمد على transaction واحدة بيرجع فيها (U2) |
| M11 | بلوك `StatementBegin/End` فاضي أو فيه كومنتات بس | goose بيقرا السطر اللي بعده statement لوحده، والـ splitter بتاعنا بيرفضه |

**الاختبارات (table-driven):**
- كل قاعدة بحالة مرفوضة وحالة مقبولة.
- `DROP COLUMN` على جدول اتعمل في نفس الملف مقبول.
- كلمة `DROP` جوه string أو كومنت أو `$$` body مش false positive.
- قسم `Down` مابيتفحصش.
- الحروف الكبيرة والصغيرة، والأسماء بين علامات تنصيص، والـ statement على أكتر من سطر.
- ملف على `main` مابيتفحصش تاني.
- الـ 43 migration الموجودين مابيتفحصوش (مش جداد)، وده مكتوب.

**نفس القواعد في CI:** الـ hooks بتشتغل مع الـ agents بس، والبشر بيفتحوا PRs برضه. فـ `apps/server/internal/Database/Migrations/Compat_test.go` بيطبّق نفس القواعد على كل ملف بعد `00048` (الحد اتنقل من `00043` لأن `00044` و`00045` اتدمجوا من غير directive، و`00048` قبل عمود `"SchemaFloor"`، والـ migrations لحد `00048` متثبّتة بأساميها في الاختبار). والحالات في `apps/server/internal/Database/Migrations/testdata/compat/` (اسم الملف فيه النتيجة المتوقعة، زي `reject_m3_not_null.sql`)، والـ Python والـ Go الاتنين بيمشوا على نفس المجلد، عشان مايبعدوش عن بعض.

## 6. توافق الإعدادات
- **الأنواع التلاتة، في جدول واحد في الكود** (`Config/Deprecations.go`): كل صف فيه الاسم القديم والجديد والنسخة اللي بدأ فيها والنسخة اللي هيتشال فيها.
  - **إعادة تسمية env:** القديم بس متحدد → بيتستخدم، وتحذير مرة في الـ log، وfinding في `doctor`، وسطر في «عن داو». الاتنين متحددين وبقيم مختلفة → الإقلاع بيقف ويقول الاتنين. بعد نسخة الشيل: القديم متحدد → الإقلاع بيقف ويقول «اتغيّر اسمه لـ Y»، مش تجاهل بصمت.
  - **متغيّر env بيتشال وبيتنقل للقاعدة:** ده الحالة الوحيدة اللي فيها نسخ. في النسخة اللي بتعلن الشيل: لو env متحدد والقاعدة مافيهاش قيمة، بيتنسخ مرة واحدة في transaction ومعاه `"ConfigImports"` (الاسم والوقت)، وبيفضل env يكسب ويقفل لحد نسخة الشيل. وبعدها env متحدد → تحذير بس، لأن القيمة بقت في القاعدة. **ده مايتطبّقش على إعدادات ADR-0063**، اللي env فيها بيفضل مدعوم، وأي شيل ليها محتاج ADR.
  - **شيل من غير بديل:** نفس دورة الإعادة تسمية.
- **قيم القاعدة:** شكل قيمة المفتاح مابيتغيّرش (قرار 19)، وأي حقل جديد = مفتاح جديد.
- **`docs/install/configuration.md`** فيه جدول «متغيرات اتغيّرت» بالنسختين.

## 7. توافق الـ API
- **`/api/v1` من GA:** إضافات بس: endpoint جديد، أو حقل اختياري، أو قيمة enum جديدة. والوثائق بتقول للـ clients يتحمّلوا الحقول والقيم الجديدة.
- **الإيقاف:** `deprecated: true` و`x-dhow-sunset` في `openapi.yaml`. والـ middleware بيضيف `Deprecation: @<unix>` و`Sunset: <date>` و`Link: <docs>; rel="deprecation"`. والـ audit بيعدّ الاستخدام لكل API token، و«عن داو» بتقول للمالك «3 مفاتيح بتستخدم API هيتشال».
- **الشيل:** جوه `/api/v1` مفيش. الشيل في `/api/v2` بس، و`v1` بيفضل لحد major واحد بعد ما `v2` يطلع.
- **قبل GA:** مفيش clients بره، فالتغيير ينفع في release واحد، بس بنفس الـ headers عشان الـ middleware يبقى متجرّب.
- **`/meta` العام** ([قرار 8](#قرارات-sabry-2026-11-02)): `Version` و`Commit` بيتشالوا منه في U6، ويظهروا في «عن داو» بعد الدخول بس. ده contract قبل GA، فبيمشي بالـ headers العادية لمدة release.
- **أكواد الأخطاء:** `ErrorDetail` بياخد `Fix` (اختياري، مترجَم من السيرفر) و`DocsPath`. أي كود جديد لازم يكون ليه `Fix` بالعربي والإنجليزي، أو يبقى في allowlist مكتوب فيها السبب (زي `INTERNAL`)، واختبار بيتأكد من ده.

## 8. الـ runner

### نافذة البروتوكول
- السيرفر بيقبل `Dhow-Protocol` من `{N, N-1}`، والـ runner بيبعت كمان `Dhow-Protocol-Max`. لو الـ runner أحدث من السيرفر، السيرفر بيرد بـ `Dhow-Protocol-Selected` والـ runner بينزل للنسخة دي لو يعرفها.
- أقدم من N-1 → `RUNNER_UPGRADE_REQUIRED` زي النهارده، والـ runner بيظهر في الواجهة «يحتاج تحديثًا» برقم النسخة المطلوبة.
- التغييرات في الـ proto إضافات بس (`buf breaking` موجود). ورفع رقم البروتوكول للتغيير في المعنى بس، وفي minor أو major.
- الميزات الجديدة بتمشي بـ `"RequiredFeatures"` (ADR-0070): الـ job المحتاج ميزة بيستنى runner أحدث، و`NO_MATCHING_RUNNER` بيقول «حدّث جهاز التشغيل».

### التحديث لنفسه
- **السياسة:** نفس سياسة السيرفر، والسيرفر بيقول لكل runner النسخة المطلوبة في رد `Heartbeat` (حقل جديد `TargetVersion`). والهدف = نسخة السيرفر.
- **الـ drain:** الـ runner بيبطّل ياخد leases جديدة، ويستنى الشغّال يخلص (لحد أطول timeout للـ jobs اللي معاه، أو 6 ساعات)، وبعدين بيكتب `/var/lib/dhow-runner/update/request.json`. لو مخلصش: تأجيل للنافذة اللي بعدها.
- **الـ helper:** `dhow-runner-update.path` و`dhow-runner-update.service` (root، oneshot)، بنفس `internal/updater` ونفس الـ journal. التنزيل من السيرفر (`/runner-api/updates/{version}/{file}`، mirror للأرشيفات اللي السيرفر اتحقق منها)، ولو مش متاح فمباشرة. والـ helper بيتحقق من الـ TUF والطول والـ sha256 بنفسه، فالسيرفر مجرد مخزن.
- **الصحة:** الـ runner الجديد لازم يعمل `Authenticate` وينجح في 2 دقيقة (ملف حالة محلي)، وإلا رجوع للـ binary القديم. الهوية المشفّرة بـ `systemd-creds` (ADR-0066) زي ما هي.
- **سياسة `notify`:** الواجهة بتعرض لكل runner الأمر بالظبط (`sudo dhow-runner update apply --version X`).

## 9. `.dhow.yml`
- السيرفر و`dhow lint` بيقبلوا النسخة N وN-1 (`Spec` في الـ feed). النهارده N = 1.
- **التحذير قبل الشيل:** ملف بـ N-1 بيعدّي، ومعاه warning (مش error) في `dhow lint` وفي صفحة الـ run: «نسخة الملف 1 ستتوقف في الإصدار X. شغّل `dhow migrate`». والإشعار لـ Maintainers المشروع مع M7.
- **`dhow migrate`:** offline زي `dhow lint`. بيعيد كتابة الملف بتحويل حتمي على `yaml.Node` (بيحافظ على الكومنتات والترتيب)، و`--check` بيقول محتاج ولا لأ، و`--write` بيكتب، و`--branch NAME` بيعمل commit على branch جديد بـ `git` (argv). و`--open-pr` بيشغّل `gh pr create` أو `glab mr create` بتوع المستخدم (argv) لو موجودين، بصلاحياته هو ([قرار 10](#قرارات-sabry-2026-11-02)).
- **ملف بنسخة أحدث من السيرفر:** `VERSION_UNSUPPORTED` برسالة «حدّث السيرفر إلى الإصدار X أو أحدث».

## 10. جوه التطبيق
كل النصوص في `I18n/Locales/Workspace/{Ar,En}.ts` بالعربي المبسّط (ADR-0062)، والمصطلحات من `Plan/Glossary.md` (بيتضاف: «نافذة الصيانة»، و«سياسة التحديث»، و«النسخة الاحتياطية»، و«الرجوع»). والشكل زي ADR-0059: كروت مسطّحة، وزرار أساسي واحد في كل منطقة، والخطر مرجاني بأيقونة.

| الصفحة | المالك | الأعضاء |
|---|---|---|
| **عن داو** (`/system`، بيكبّر `BuildInfoCard`) | النسخة، والـ commit، والنسخة (Community)، والترخيص (AGPL-3.0 برابط)، والصحة، وآخر تحديث ونتيجته، والسياسة والنافذة الجاية، وحالة الـ feed (آخر تحقق ناجح)، وحالة الـ schema (`current` أو `ahead`)، والـ backfills، والنسخ الاحتياطية (العدد والحجم وآخر واحدة)، والـ instances، والـ runners اللي محتاجة تحديث، والـ API المستخدم وهيتشال | النسخة، والنسخة (Community)، والترخيص، والصحة باختصار |
| **التحديثات** (`/system/updates`) | التحديث المتاح وملاحظاته، و«حدّث الآن»، و«أجّل»، و«ارجع للنسخة السابقة»، والسجل | — |
| **الجديد** | بعد الترقية: كارت في الرئيسية مرة واحدة لكل مستخدم (`"UserPreferences"."ReleaseNotesSeen"`)، والملاحظات من الـ JSON المتضمّن في الـ binary (بيشتغل في المعزول)، والمالك بيشوف كمان قسم «إجراءات مطلوبة» | نفس الكارت، من غير الأقسام الخاصة بالتشغيل |
| **المساعدة حسب الصفحة** | زرار «مساعدة» في رأس كل صفحة بيفتح صفحة الوثائق بتاعتها بلغة الواجهة. الخريطة في `Core/Help/DocLinks.ts`، واختبار بيتأكد إن كل صفحة ليها رابط وإن كل رابط موجود في `docs/` و`docs/en/` | نفس الشيء |
| **الأسئلة الشائعة** (`/help`) | أسئلة قصيرة وبحث، والنص في الكتالوج (lazy chunk)، ومصدرها `docs/install/troubleshooting.md` وأسئلة الدعم | نفس الشيء، من غير أسئلة التشغيل |
| **`doctor` في الواجهة** | كارت «فحوص الجهاز»: الفحوص اللي ينفع تتعمل من جوه الـ process (الإعدادات ومصدرها، والقاعدة، والـ schema، وMPTCP من `/proc`، ومساحة مجلد الحالة، وفرق الساعة، والـ feed، وعمر آخر نسخة احتياطية). وفحوص الـ host اللي محتاجة root بيكتبها `dhow-update.timer` في `status.json` وبتظهر بوقتها | — |
| **رسايل الخطأ** | `Fix` تحت الرسالة في كل مكان بيعرض خطأ، ورابط «اعرف أكثر» لو فيه `DocsPath` | نفس الشيء |

- **الـ SPA القديمة:** `/me` بيرجّع `ServerBuild` (hash قصير للنسخة)، ولو اختلف عن build الواجهة: شريط «حدّثنا داو. أعد تحميل الصفحة»، وفشل تحميل chunk بيعرض نفس الشريط بدل شاشة بيضا.
- **الحجم:** الصفحات الجديدة lazy، وميزانية `pnpm size` زي ما هي.

## 11. هندسة الـ releases
- **semver:**
  - **major:** شيل حاجات (`/api/v2`، أو نسخة `.dhow.yml`، أو إعداد، أو بروتوكول بره النافذة).
  - **minor:** ميزات، وexpand، وcontract لحاجات اتعلن إيقافها في minor قبلها على الأقل.
  - **patch:** إصلاحات وexpand بس.
  - **`-rc.N`:** عمره ما بيتثبّت أوتوماتيك.
- **الدعم** ([قرار 9](#قرارات-sabry-2026-11-02)): آخر minor بس بياخد patches لحد GA، ونرجع للقرار بعد 3 شهور من GA. اللي على minor أقدم بياخد الإصلاح الأمني كتحديث minor بزرار.
- **الـ changelog:** `deploy/release/changelog` (Go، من غير اعتماديات) بيقرا `git log` بين الـ tags بـ Conventional Commits، ويجمّع `feat` و`fix` و`perf` و`security` و`deprecate`، وبرقم الـ PR. ومنه body الـ GitHub Release.
- **الملخّص البشري:** `docs/releases/X.Y.Z.md` (عربي مبسّط) و`docs/en/releases/X.Y.Z.md` بعناوين ثابتة: «أهم التغييرات»، و«إصلاحات أمنية»، و«إجراءات مطلوبة»، و«ما سيتوقف لاحقًا». و`make docs-check` بيتأكد إنهم متطابقين، و`release.yml` بيفشل لو مش موجودين، ومنهم JSON بيتضمّن في الـ binary («الجديد»).
- **الـ release الأمني:** label `security` على الـ PR، وGHSA advisory، وحقل `Security` في الـ feed. والـ embargo: الـ feed بيقول الخطورة من غير تفاصيل لحد الإعلان.
- **البوابات في `release.yml`:** النوع بيتحسب من الـ tag اللي قبله، والـ patch اللي فيه migration مش `expand` بيفشل، والـ `-- dhow:floor` لازم tag أقدم، واختبار الترقية (U3) أخضر على الـ commit نفسه، وتوقيع cosign keyless للـ assets والصورة زي النهارده. وبعدين Sabry بيوقّع targets offline والـ feed بيتنشر على الـ Pages.
- **مفاتيح الـ feed:** [الـ ceremony](#مفاتيح-الـ-feed-والـ-ceremony) في القسم 1، ومكتوب للعامة في `docs/security/release-keys.md`.

## المراحل
كل PR بيعدّي `make check` وCI أخضر، وفيه اختبارات سلبية لكل مسار أمني، وبيحدّث `Plan/Progress/` في يومه، وبيكتب ADR بتاع قراراته في أوله. والمراحل اللي بتلمس الـ updater أو الـ feed أو النسخ الاحتياطية عليها مراجعة أمنية مستقلة إجبارية.

### U1: hook قاعدة الـ migrations (الآن، مع M4)
- **الحجم:** S. **الاعتماد:** ADR-0077.
- **المحتوى:** `migration_rules.py` بالقواعد M1 لـ M9، و`test_migration_hooks.py`، و`Compat_test.go` بنفس الحالات من `testdata/compat/`، وسطر في `CLAUDE.md` (قسم الـ hooks وقسم قاعدة البيانات).
- **قبول:**
  - كل حالة في `testdata/compat/` نتيجتها في الـ hook زي الـ Go test بالظبط.
  - ملف `00044_x.sql` جديد من غير phase بيترفض، وبـ `expand` و`ADD COLUMN "X" TEXT` بيعدّي.
  - الـ 43 ملف الموجودين ماتأثروش.
- **التحقق:** `make hooks-test` و`go test -count=1 ./apps/server/internal/Database/Migrations/` و`make check`.
- **ملاحظة لـ M4.7 وM4.8:** الـ migrations اللي بتتعمل قبل ما U1 يندمج بتتكتب بالقاعدة بإيد، والـ directive بيتضاف لو لسه مش على `main`.

### U2: توافق الـ schema في الـ binary
- **الحجم:** M. **الاعتماد:** U1. **التوقيت:** قبل M5، وعشان كده قبل أول release.
- **المحتوى:**
  - expand migration: `"Instance"."SchemaFloor" TEXT NOT NULL DEFAULT '0.0.0'`.
  - `Migrator.State` (`current` و`ahead` و`incompatible`)، والـ readiness، ورمز الخروج `78` و`RestartPreventExitStatus=78` في `deploy/systemd/dhow.service`.
  - `dhow-server migrate status|plan|dry-run`، وsplitter للـ statements (goose بيخبّي الـ parser بتاعه في `internal`) واختبار بيقارن نتيجة الـ splitter بنتيجة goose على كل الملفات الموجودة (`pg_dump --schema-only` لقاعدتين).
  - شيل الـ index اللي `indisvalid = false` قبل إعادة ملف `NO TRANSACTION`.
  - `"ServerInstances"` والـ heartbeat.
  - `docs/install/upgrades.md` بالنسختين (القاعدة للمشغّل: إمتى الرجوع آمن).
- **قبول:**
  - binary أقدم على قاعدة `ahead` بـ expand بس: ready.
  - binary أقدم على قاعدة floor بتاعها أعلى منه: بيقف بـ `78` ورسالة فيها النسخة.
  - الـ dry run على migration فيه خطأ بيرجّع الخطأ والقاعدة ماتغيّرتش (اختبار بيعدّ الجداول قبل وبعد).
  - `lock_timeout` قدّام transaction ماسك قفل بيفشل في 3 ثواني بـ `PREFLIGHT_LOCK_TIMEOUT`.
- **التحقق:** `go test -race -count=1 ./apps/server/internal/Database/... ./apps/server/internal/Modules/System/...` و`make gen && git diff --exit-code` و`make check`.

### U3: اختبار الترقية في CI
- **الحجم:** M. **الاعتماد:** U2. **التوقيت:** الآن لحد M5.
- **المحتوى:** `deploy/upgrade-test/run.sh` و`make upgrade-test`. بيبني الـ base (الـ merge-base مع `main` لحد أول tag، وبعدها آخر tag) والـ head، على Postgres 18:
  - base بيقوم، وseed بالـ API (owner، ومؤسسة، ومشروع، وrunner، وjob بـ logs).
  - head بيقوم ويطبّق الـ migrations، وكل حاجة اتعملت في الـ seed ظاهرة.
  - base تاني على الـ schema الجديد: ready وsmoke، إلا لو الـ floor اترفع: يبقى لازم يقف بـ `78`.
  - استرجاع نسخة (`pg_dump` العادي لحد U5) وbase بيقوم عليها.
- **قبول:** job `upgrade` في `ci.yml` بيشتغل على أي PR بيلمس `Database/Migrations` أو `apps/server` أو `Config`، بنفس شرط `!github.event.pull_request.draft`. وmigration بيعمل `DROP COLUMN` مستخدم بيفشّل الـ job في خطوة الـ base التاني (اختبار سلبي في `deploy/upgrade-test/testdata`).
- **التحقق:** `make upgrade-test` محليًا و`make lint-actions`.

### U4: الـ backfills في الخلفية
- **الحجم:** S. **الاعتماد:** U2. **التوقيت:** مع أول تغيير محتاجه، وبالكتير قبل M9.
- **المحتوى:** جدول `"Backfills"`، و`Core/Backfill` (register بالاسم، ودفعات بـ `SKIP LOCKED`، وadvisory lock، وthrottle، وتقدّم)، والـ guard للـ contract (`BACKFILL_NOT_DONE`)، والتقدّم في `migrate status` و«عن داو».
- **قبول:** backfill لـ 100,000 صف بيتقتل (`kill -9`) 3 مرات في النص ويكمّل من مكانه من غير تكرار، والسيرفر بيخدم طلبات طول الوقت (p99 للـ API أقل من 2x الطبيعي في الاختبار).
- **التحقق:** `go test -race -count=1 ./apps/server/internal/Core/Backfill/...` و`make check`.

### U5: النسخ الاحتياطي والاسترجاع
- **الحجم:** M. **الاعتماد:** U2. **التوقيت:** M9، وهو اللي بيستخدمه تمرين الاسترجاع.
- **المحتوى:** `internal/backup` (الصيغة والتشفير والـ manifest)، و`dhow-server backup create|list|verify|restore`، ومفتاح النسخ في الـ installer، والتدوير والحد، و`dhow-backup.timer` الاختياري، وتحديث `docs/install/{compose,binary-systemd}.md` بالأوامر الجديدة بالنسختين.
- **قبول:**
  - النسخة بتترجع على جهاز جديد بالمفتاح، والـ secret key بيرجع معاها.
  - byte واحد متغيّر، أو chunk ناقص، أو chunk متبدّل مكانه: `verify` بيفشل.
  - النسخة على القرص مافيهاش أي نص صريح: `grep` على المفتاح السري وكلمة سر القاعدة وأسرار M4 مالقاش حاجة.
  - المساحة الناقصة بتوقف قبل الكتابة.
  - الملفات `0600` والمجلد `0700` لـ root.
- **التحقق:** `go test -race -count=1 ./internal/backup/...` وfuzz على قارئ الصيغة (`FuzzBackupReader` 60 ثانية) و`make check` وتمرين M9.

### U6: المساعدة جوه التطبيق
- **الحجم:** L. **الاعتماد:** U2 (حالة الـ schema). **التوقيت:** M7. بيتقسم لـ 3 PRs:
  1. «عن داو» للمالك والأعضاء، والـ `ServerBuild` وشريط إعادة التحميل، و`doctor` في الواجهة.
  2. `Fix` و`DocsPath` في `ErrorDetail` لكل الأكواد الموجودة، والعرض في الواجهة.
  3. «الجديد» (مع U7 أو بملف ملاحظات تجريبي)، والمساعدة حسب الصفحة، والأسئلة الشائعة.
- **قبول:**
  - كل صفحة في `Sections.ts` ليها رابط مساعدة موجود في الوثائق بالنسختين.
  - كل كود خطأ ليه `Fix` بالعربي والإنجليزي أو في الـ allowlist.
  - عضو مش مالك مابيشوفش السياسة ولا النسخ الاحتياطية ولا الـ commit (اختبار في الـ API والواجهة).
  - الـ hook العربي المبسّط عدّى على كل النصوص.
  - `pnpm size` جوه الميزانية، ومفيش `<style>` اتحقن.
- **التحقق:** `pnpm lint` و`pnpm typecheck` و`pnpm test` و`pnpm build` و`pnpm size`، و`make gen && git diff --exit-code`، و`make docs-check`، و`make check`.

### U7: هندسة الـ releases والـ feed
- **الحجم:** L. **الاعتماد:** U3. **التوقيت:** M10، قبل أول نسخة تجريبية عامة.
- **المحتوى:**
  - `deploy/release/changelog` وملفات `docs/releases/` بالنسختين، والـ JSON المتضمّن.
  - البوابات في `release.yml`.
  - `deploy/release/feed` و`dhow-feed`: `keys init`، و`build` (بيعمل `dhow-release.json` و`targets.json` مش موقّع من الـ assets)، و`sign-targets` (offline، المفتاح ملف مشفّر بـ `age`)، و`sign-online` (snapshot وtimestamp في CI).
  - `release.yml` بيرفع `dhow-release.json` كـ asset وبيطلّع `targets.json` كـ artifact، وبيفضل يوقّع بـ cosign.
  - branch `tuf-metadata` وحمايته، وenvironment `release-feed` بالـ secrets.
  - `.github/workflows/pages.yml` (نشر واحد للوثائق والـ metadata)، وتفعيل GitHub Pages بمصدر Actions.
  - job الـ timestamp الأسبوعي، والفحص اليومي للصلاحية، والـ issue لو فشل.
  - job مراقبة يومي بيقارن كل hash في `targets.json` المنشور بـ `SHA256SUMS` الموقّع بـ cosign في نفس الـ release، ويفتح issue `feed-mismatch` لو فيه فرق.
  - `docs/security/release-keys.md` بالنسختين، والـ ceremony الأول (root v1 متضمّن في الـ binary).
- **قبول:**
  - release تجريبي (`v0.9.0-rc.1`) بيطلّع feed على `https://khaymah.github.io/dhow/updates/` بيتحقق منه `go-tuf` client من الصفر، والأرشيفات من الـ release assets.
  - تبديل byte في أي ملف بيكسر التحقق.
  - `timestamp` قديم بيترفض.
  - patch فيه contract migration بيوقف الـ release.
  - نشر الوثائق مابيمسحش `/updates/`، وتجديد الـ timestamp مابيمسحش الوثائق.
  - job الصلاحية بيفتح issue لما الصلاحية أقل من 48 ساعة (اختبار بساعة وهمية).
  - مفيش أي خطوة محتاجة خدمة أو جهاز مدفوع.
- **التحقق:** `make lint-actions` و`go test -race -count=1 ./deploy/release/...` وrelease تجريبي على fork.

### U8: فحص التحديثات والسياسة والإشعارات
- **الحجم:** M. **الاعتماد:** U7. **التوقيت:** M10.
- **المحتوى:**
  - `internal/updatefeed` (client بـ `go-tuf/v2` فوق `netguard`، والحدود).
  - موديول `Updates` (worker بـ advisory lock، و`"UpdateFeedState"`، والقرار حسب السياسة).
  - مجموعة الإعدادات `updates` وenv بتاعها.
  - الـ API (`GET /system/updates`، و`POST /system/updates/check`، و`PUT /instance/settings/updates`) بـ `can()` لمالك السيرفر، وصف لكل route في الـ route-table.
  - الـ audit والإشعارات، و«التحديثات» في الواجهة.
  - `doctor` (الـ feed والساعة).
  - سؤال «هل يصل السيرفر للإنترنت؟» والسياسة والنافذة في الـ installer.
  - إيميل عاجل للمالك مع أي release أمني طول فترة البيتا (M10)، لأن الأوتوماتيك في M11 ([قرار 0](#قرارات-sabry-2026-11-02)).
- **قبول:**
  - feed بتوقيع غلط، أو نسخة metadata أقل، أو `timestamp` منتهي، أو root مش في السلسلة، أو ملف أكبر من الحد: مرفوض بالكود الصح، والحالة القديمة ماتغيّرتش.
  - `DHOW_UPDATE_CHECK=off`: صفر اتصالات (dialer الاختبار).
  - الطلب مافيهوش أي header غير `User-Agent: Dhow` و`Accept` و`If-None-Match` (اختبار على `httptest`).
  - instance تانية مابتعملش طلب طول ما الأولى ماسكة القفل.
- **التحقق:** `go test -race -count=1 ./internal/updatefeed/... ./apps/server/internal/Modules/Updates/...` وfuzz على الـ parser بتاع `releases/<v>.json`، و`make gen && git diff --exit-code`، و`make check`.

### U9: الـ updater للـ binary ومسار الـ installer
- **الحجم:** XL. **الاعتماد:** U5 وU8. **التوقيت:** M10 (التحديث بزرار والرجوع)، والأوتوماتيك في U10.
- **المحتوى:**
  - `internal/updater`: الخطوات 0 لـ 8، والـ journal، والـ recover، وقواعد الطلب، والرجوع بالجدول.
  - `dhow-server update apply|status|rollback|recover`.
  - الـ units التلاتة في `deploy/systemd/` بتقويتها.
  - الـ installer بيركّبهم وبيستخدم `internal/updater` في الترقية.
  - الكتابة في `/etc/motd.d`.
  - «ارجع للنسخة السابقة».
  - الـ hook: `internal/updater` مش في import graph السيرفر.
  - `docs/install/upgrades.md` بالخطوات والرجوع.
- **قبول:**
  - **(VM بـ Debian 13 وUbuntu 26.04):** ترقية بالزرار بتنجح.
  - نسخة بتفشل فحص الصحة بترجع لوحدها، والـ audit والإشعار فيهم السبب.
  - `kill -9` للـ helper في كل خطوة من 3 لـ 8 وإعادة تشغيل الـ VM: بيخلّص يا على الجديدة يا على القديمة، والاتنين سليمين.
  - طلب لنسخة أقدم، أو مش في الـ feed، أو symlink، أو ملف 5 KiB، أو مش مملوك لـ `dhow`: مرفوض ومفيش تغيير.
  - process السيرفر مايقدرش يكتب في `/usr/local/bin` ولا `/etc/systemd` (اختبار من جوه الخدمة).
- **التحقق:** `go test -race -count=1 ./internal/updater/...`، و`make updater-e2e` (VMs)، و`make hooks-test`، و`make check`، ومراجعة أمنية مستقلة.

### U10: السياسة الأوتوماتيك والنافذة وCompose
- **الحجم:** L. **الاعتماد:** U9. **التوقيت:** M11، وده اللي بيخلّي الافتراضي «الأمني أوتوماتيك» حقيقي.
- **المحتوى:**
  - النافذة والتأجيل والـ quiesce.
  - المسار الأوتوماتيك والخطوات المتعددة بـ `RequiresFrom`.
  - الـ helper لـ Compose على الـ host، والملفات الاختيارية للـ Compose اليدوي.
  - الحماية من أكتر من instance.
  - `CriticalOutsideWindow` ([قرار 6](#قرارات-sabry-2026-11-02)).
- **قبول:**
  - patch أمني بيتثبّت جوه النافذة بس (ساعة وهمية حوالين الحدود والمنطقة الزمنية وتغيير التوقيت الصيفي).
  - النشر الشغّال بيأجّل.
  - Compose: `.env` فيه سطور زيادة وكومنتات بيفضل زي ما هو إلا `DHOW_IMAGE`، والرجوع بيرجّع الـ digest.
  - instance تانية → إشعار بس.
- **التحقق:** `go test -race -count=1 ./internal/updater/... ./apps/server/internal/Modules/Updates/...`، و`make updater-e2e` لـ Compose، ومراجعة أمنية مستقلة.

### U11: نافذة الـ runner وتحديثه لنفسه
- **الحجم:** L. **الاعتماد:** U9. **التوقيت:** M11.
- **المحتوى:**
  - `Dhow-Protocol-Max` و`Dhow-Protocol-Selected`، والسيرفر بيقبل N وN-1.
  - `TargetVersion` في `Heartbeat`، والـ drain.
  - `dhow-runner-update.{path,service}`، و`/runner-api/updates/...` كـ mirror.
  - `dhow-runner update`، والواجهة («يحتاج تحديثًا» والأمر).
- **قبول:**
  - runner بـ N-1 بياخد jobs، وبـ N-2 بياخد `RUNNER_UPGRADE_REQUIRED`.
  - التحديث مابيقطعش job شغّال.
  - سيرفر مخترَق بيبعت `TargetVersion` أقدم أو مش موقّع: الـ helper بيرفض.
  - الهوية المشفّرة بتفضل شغّالة بعد التحديث.
- **التحقق:** `go test -race -count=1 ./apps/runner/... ./internal/updater/...`، و`make runner-isolation`، و`make runner-chaos`، و`buf breaking`، ومراجعة أمنية مستقلة.

### U12: عقود التوافق (الإعدادات والـ API و`.dhow.yml`)
- **الحجم:** M. **الاعتماد:** U2. **التوقيت:** M10، عشان تتجرّب في النسخة التجريبية قبل ضمانات GA.
- **المحتوى:**
  - `Config/Deprecations.go` و`"ConfigImports"`.
  - الـ middleware بتاع `Deprecation` و`Sunset`، والعدّ لكل token.
  - نافذة N وN-1 في `internal/pipelinespec` والـ warnings في `dhow lint`.
  - `dhow migrate` (بيشتغل لما تبقى فيه نسخة 2، والهيكل والاختبارات بتحويل وهمي من النهارده).
  - جدول «متغيرات اتغيّرت» في `configuration.md`.
- **قبول:**
  - متغيّر قديم بس: شغّال وتحذير. الاتنين بقيم مختلفة: الإقلاع بيقف. بعد الشيل: بيقف برسالة الاسم الجديد.
  - endpoint متعلّم `deprecated` بيرجّع الـ headers التلاتة، والـ audit بيعدّ.
  - `dhow migrate --check` على ملف حديث: صفر وخروج `0`.
  - مفيش اتصال شبكة من `dhow` (اختبار M4.2).
- **التحقق:** `go test -race -count=1 ./apps/server/internal/Config/... ./internal/pipelinespec/... ./apps/cli/...`، و`make docs-check`، و`make check`.

### الترتيب
```
الآن (مع M4):   U1 ─► U2 ─┬─► U3
                          │
M5 لـ M9:                 ├─► U4 (مع أول تغيير محتاجه)
                          └─► U5 (M9، تمرين الاسترجاع)
M7:                 U2 ─────► U6
M10:          U3 ─► U7 ─► U8 ─► U9          U2 ─► U12
M11:                            U9 ─┬─► U10
                                    └─► U11
```
- **U1 وU2 أولوية دلوقتي** عشان migrations M4.7 وM4.8 والجاي من M5 تمشي على القاعدة، وعشان أول release يطلع والرجوع بالـ binary شغّال.
- **ليه مش milestone جديد:** كل جزء ليه مكان طبيعي. النسخ الاحتياطي محتاجه تمرين M9، والإشعار والتحديث بزرار محتاجينهم أول فرق بره في M10، والأوتوماتيك والـ runner هما «ضمانات الترقية» بتاعة M11. milestone لوحده كان هيأخّر GA أو يتكرر مع M10 وM11.

## الاختبارات: إيه بيشتغل فين
| الاختبار | كل PR | ليلي أو label | قبل كل release |
|---|---|---|---|
| hook الـ migrations و`Compat_test.go` | نعم | — | — |
| `upgrade-test` (base ← head ← base، والاسترجاع) | لو الـ PR بيلمس `apps/server` أو `Database/Migrations` أو `Config` | نعم | نعم، من آخر tag ومن الـ tag اللي قبله |
| `internal/updatefeed` (TUF، والرفض، والحدود، وصفر اتصالات لما مقفول) | نعم | — | — |
| `internal/updater` بـ filesystem وsystemd وهميين، و`kill` في كل خطوة | نعم | — | — |
| `internal/backup` (الصيغة، والعبث، والـ fuzz القصير) | نعم | fuzz 30 دقيقة | — |
| `make updater-e2e`: VM بـ systemd حقيقي، ترقية ورجوع وقطع كهربا | — | نعم، وlabel `updater` | نعم، Debian 13 وUbuntu 26.04 |
| `make updater-e2e` لـ Compose | — | نعم | نعم |
| الـ runner: N-1 وN-2 وتحديث لنفسه أثناء job | لو الـ PR بيلمس `apps/runner` أو `packages/proto` | نعم | نعم |
| الواجهة: «عن داو» بالأدوار، والـ `Fix`، وروابط المساعدة | لو الـ PR بيلمس `apps/web` | — | نعم |

## Edge cases
- **القاعدة أحدث من الـ binary والـ floor بيسمح:** ready، و«عن داو» بتقول `ahead`، و`doctor` advisory بس.
- **الـ floor أعلى من الـ binary:** الإقلاع بيقف بـ `78`، وsystemd مابيعيدش، والرسالة فيها النسخة المطلوبة ورابط `upgrades.md`.
- **migration وقع في release فيه أكتر من ملف:** الملفات اللي قبله اتطبّقت (كل واحد في transaction)، وكلها expand، فالـ binary القديم بيشتغل.
- **index `CONCURRENTLY` وقع في النص:** بيفضل `INVALID`، و`IF NOT EXISTS` مش هيصلّحه، فالـ migrator بيشيله الأول (U2).
- **المساحة خلصت أثناء التنزيل أو النسخة الاحتياطية:** وقف قبل الـ swap، ومسح الملف الناقص، وإشعار.
- **الـ feed قال نسخة والأرشيف مش موجود لسه** (الـ release بيتنشر): `404` أو hash غلط، فإعادة في النافذة الجاية، وإشعار بعد 3 مرات.
- **ساعة الجهاز غلط:** لقدّام كتير → كل الـ metadata منتهية، فمفيش أوتوماتيك وfinding. لورا → الحماية بآخر `timestamp` اتشاف.
- **المالك داس «حدّث الآن» والأوتوماتيك شغّال:** الـ flock، والطلب التاني بياخد «في تحديث شغّال».
- **نسختين نزلوا ورا بعض قبل النافذة** (1.4.1 و1.4.2): بيتثبّت 1.4.2 على طول.
- **النسخة اترجعت منها:** مابتتجرّبش تاني أوتوماتيك. الـ patch اللي بعدها مسموح.
- **فحص الصحة نجح بس فيه bug:** «ارجع للنسخة السابقة» لحد 7 أيام، بالقاعدة أو من غيرها حسب الـ floor.
- **الرجوع نفسه فشل:** `dhow` واقف، و`NEEDS_ATTENTION`، ورسالة في `motd` والـ journal. الواجهة واقعة ساعتها، ومفيش إيميل لأن السيرفر هو اللي بيبعت، والوثيقة بتقول كده.
- **Compose و`.env` مافيهوش `DHOW_IMAGE`** أو الملف متعدّل بشكل مش مفهوم: إشعار بس، و`doctor` بيقول السبب.
- **قاعدة PostgreSQL مش 18، أو `pg_dump` ناقص أو major تاني:** `PREFLIGHT_POSTGRES` برسالة فيها الأمر اللي ينصّبه.
- **قاعدة كبيرة النسخة بتاخد أطول من النافذة:** الـ preflight بيقدّر من حجم آخر نسخة وسرعتها، ولو مش هتلحق: تأجيل وإشعار «كبّر النافذة».
- **الـ runner offline وقت ترقية السيرفر:** لما يرجع، لو N-1 بيشتغل وبيتحدّث لوحده، ولو أقدم بياخد `RUNNER_UPGRADE_REQUIRED` والواجهة بتقول الأمر.
- **الـ runner مالوش internet:** بينزّل من السيرفر (قرار 18).
- **`.dhow.yml` بنسخة أحدث من السيرفر:** `VERSION_UNSUPPORTED` بـ «حدّث السيرفر».
- **متصفح فيه الواجهة القديمة:** شريط إعادة التحميل، والـ API إضافات بس فالقديمة مابتقعش.
- **API token بيستخدم endpoint هيتشال:** الـ headers، والعدّ، وسطر للمالك.
- **air-gapped:** «عن داو» بتقول الفحص مقفول وخطوات الترقية اليدوي، و«الجديد» شغّال من الـ binary.
- **`DHOW_UPDATE_URL` على mirror داخلي:** نفس التحقق، والـ mirror مايقدرش يزوّر.
- **التوقيت الصيفي في المنطقة الزمنية:** النافذة بالوقت المحلي، واليوم اللي فيه الساعة بتتكرر أو بتتشال مختبَر.

## جدول التهديدات
| التهديد | اللي بيحصل | التخفيف |
|---|---|---|
| **feed خبيث** (حساب GitHub أو الـ Pages اتخترق، أو DNS، أو MITM، أو mirror) | ملفات بتقول فيه نسخة غير الحقيقية | TUF بـ root متضمّن في الـ binary، وكل ملف بطول وhash، والأرشيف بـ sha256 من `targets` المتوقّع offline، وTLS، والـ redirects للأرشيف لـ hosts GitHub بس. مفيش ثقة في GitHub نفسه: اللي بيتحكّم في الريبو أو الـ Pages من غير المفتاح الـ offline أقصاه تجميد |
| **مفتاح targets اتسرّب** | المهاجم يعمل targets بأرشيف خبيث، و**ده لوحده كفاية لتحديث خبيث** لأن الـ updater مابيتحققش من cosign ([قرار 3](#قرارات-sabry-2026-11-02)) | **مقبول بقرار Sabry**: مفتاح واحد عمره ما بيخرج من الـ USBين المشفّرين، وبيتفك بس على Debian Live من غير شبكة، والـ passphrase مش على أي قرص، والبديل كان اعتمادية كبيرة (`sigstore-go`) جوه الـ helper. **الكشف:** job المراقبة اليومي بيقارن `targets.json` بـ `SHA256SUMS` الموقّع بـ cosign في نفس الـ release (الاتنين لازم يتخترقوا عشان مايبانش)، وأي حد يقدر يعمل نفس المقارنة بإيده. **الرد:** rotate بـ root جديد، والـ clients بتاخده في أول فحص، وإعلان أمني |
| **مفتاح الـ timestamp أو snapshot (في CI) اتسرّب** | تجميد أو replay | بيوقّعوا `timestamp` و`snapshot` بس، فمايقدروش يضيفوا نسخة. وصلاحية 7 أيام، والحماية بالأرقام وبآخر وقت اتشاف، والـ environment المحمي بيقلّل مين يوصلهم |
| **جهاز Sabry اتخترق وهو بيوقّع** | malware بيسرق المفتاح بعد فكّه أو بيبدّل الـ hashes قبل التوقيع. ده أضعف من hardware key، اللي كان هيمنع سرقة المفتاح نفسه | التوقيع عمره ما بيحصل على النظام العادي: boot من Debian Live متراجع، والشبكة مقفولة، والـ USB الأساسي بيتحط بس وقت التوقيع. والـ passphrase بيتكتب هناك بس وعمره ما بيتحفظ. والملخّص بيتقارن بالـ log من جهاز تاني قبل الـ passphrase. وjob المراقبة بيكشف الـ hash المبدّل. وبرضه رد الـ rotate لو فيه شك |
| **USB المفاتيح اتسرق** | المهاجم معاه الملفات المشفّرة | `age` بـ scrypt وpassphrase طويل بيدّي وقت، والـ rotate في نفس الأسبوع، والـ USBين في مكانين |
| **مفتاح الـ root اتسرّب** | تحكّم كامل في الـ feed | ملف مشفّر offline في مكانين، وthreshold بيتكبّر لـ 2 من 3 لما يبقى فيه maintainer تاني، وjob المراقبة بيكشف أي targets مش مطابق لـ cosign. والخطة في `release-keys.md`: binary جديد بـ root جديد وإعلان أمني والترقية اليدوي بـ `get.sh` |
| **الرجوع لنسخة أقدم (downgrade)** | تثبيت نسخة فيها ثغرة معروفة | الـ helper بيرفض أي نسخة ≤ المتنصّبة إلا رجوعه هو من الـ journal بالـ binary المحفوظ عنده، والـ metadata بأرقام مابتنزلش |
| **تحديث ناقص** | binary نص مكتوب، أو unit من نسخة وbinary من نسخة | التنزيل في stage والتحقق قبل أي لمسة، و`rename` ذرّي، والـ journal بيقول الخطوة، والـ recover بيكمّل أو يرجع |
| **قطع الكهربا وقت الـ swap** | الجهاز بيقوم في حالة مش معروفة | fsync للملف والمجلد والـ journal، و`dhow-update-recover.service` قبل `dhow.service`، وكل migration في transaction أو idempotent، والاختبار بـ `kill -9` وreboot في كل خطوة |
| **النسخة الاحتياطية بتسرّب الأسرار** | الملف فيه المفتاح السري والقاعدة كلها | AES-256-GCM بمفتاح منفصل على الجهاز، و`0700`/`0600` لـ root، ومش في volume بتاع container، والتدوير بيمسح. والتصدير بره الجهاز بمفتاح `age` عام، والجهاز مايقدرش يفكّه |
| **تصعيد الصلاحيات من الـ updater** | process `dhow` مخترَق بيستغل الـ helper | الطلب رقم نسخة بس بـ schema صارم و`O_NOFOLLOW`، والـ helper هو الـ binary المتنصّب مش المنزّل، ومفيش shell (argv بس)، ومسارات ثابتة، و`TriggerLimitIntervalSec` على الـ path unit، وتقوية systemd للـ helper، وأقصى ضرر = ترقية لنسخة رسمية أحدث |
| **الـ binary الجديد خبيث رغم التوقيع** (supply chain) | كود خبيث بيشتغل | البناء reproducible ومتراجع في CI، وSBOM وprovenance وcosign، والـ dry run بيشتغل كـ `dhow` مش root، والتوقيع البشري على targets بعد ما CI يخلص وبعد مقارنة الـ hashes |
| **Docker socket في container** | أي ثغرة في الـ container = root على الجهاز | مفيش sidecar ([قرار 4](#قرارات-sabry-2026-11-02))، والـ helper على الـ host، والـ Compose اليدوي إشعار بس |
| **إغراق الـ helper بطلبات** | إعادة تشغيل كتير (DoS) | `TriggerLimitBurst` على الـ path unit، ونفس النسخة بعد فشلها مابتتجرّبش، والـ audit |
| **الـ runner بيتحدّث لنسخة مزوّرة من سيرفر مخترَق** | كود خبيث على أجهزة التشغيل | الـ helper بتاع الـ runner بيتحقق من الـ TUF والطول والـ hash بنفسه، والسيرفر mirror بس |
| **الطلب للـ feed بيكشف الـ instance** | GitHub بيشوف عنوان الـ IP | مفيش ids ولا نسخة، والـ proxy مدعوم، والقفل للمعزول، ومكتوب بالظبط في الوثائق |
| **الـ timestamp خلص لأن الـ job المجدول ماشتغلش** | كل الـ instances بتقول «لم نتمكن من التأكد» والأوتوماتيك بيقف (fail closed، مش ثغرة) | الفحص اليومي بيعيد التوقيع تحت 48 ساعة، والـ issue وإيميل GitHub، والعملاء بيكملوا شغل عادي |

## الاعتماديات المتوقعة (بنسخ مثبّتة)
اتأكدت يوم 2026-11-02 من proxy.golang.org (التاريخ) وOSV (مفيش advisories)، وكلها أقدم من أسبوعين.

| الاعتمادية | النسخة | تاريخها | ليه |
|---|---|---|---|
| `github.com/theupdateframework/go-tuf/v2` | v2.4.2 | 2026-05-19 | الـ client والـ repository tooling بتوع TUF ([قرار 1](#قرارات-sabry-2026-11-02))، open source وببلاش، بدل بروتوكول بنكتبه بنفسنا |
| `filippo.io/age` | v1.3.2 | 2026-08-29 | تصدير النسخ الاحتياطية بمفتاح عام في U5 ([قرار 7](#قرارات-sabry-2026-11-02))، وتشفير مفاتيح الـ feed بـ passphrase في `dhow-feed` |
| goose | v3.29.0 (موجودة) | — | زي ما هي |

**مش هنضيف:**
- مكتبات self-update (`minio/selfupdate` وأمثالها): بتكتب الـ binary من نفس الـ process، وده عكس فصل الصلاحيات.
- `git-cliff` أو أي أداة changelog: سكربت Go صغير بيكفي.
- مكتبة Docker SDK: `docker compose` بـ argv من الـ helper.
- مكتبة cron: النافذة أيام وساعة ومدة، و`time` بيكفي.
- `sigstore-go`: الـ updater مابيتحققش من cosign ([قرار 3](#قرارات-sabry-2026-11-02))، وcosign بيفضل في `release.yml` و`get.sh` والتحقق اليدوي.
- أي خدمة توقيع أو KMS أو hardware key: ممنوع بقرار «كله ببلاش».

## قرارات Sabry (2026-11-02)
Sabry جاوب الأسئلة الـ 11 كلها، وضاف شرط ملزم: **كله ببلاش**، مفيش خدمة مدفوعة ولا جهاز بفلوس. التغييرات عن التوصية متعلّمة «اتغيّر».

0. **التوقيت:** الإشعار والتحديث بزرار في M10، والأمني الأوتوماتيك في M11 (GA). وطول البيتا، إيميل عاجل للمالك مع أي release أمني (U8). (زي التوصية.)
1. **التوقيع:** TUF حقيقي بـ `go-tuf/v2`، open source وببلاش. (زي التوصية.)
2. **المفاتيح (اتغيّر):** Sabry بيوقّع كل release offline على جهازه. مفاتيح root وtargets **ملفات مشفّرة بـ passphrase على USBين offline**، واحد معاه وواحد احتياطي في مكان تاني، ومفيش hardware keys. وsnapshot وtimestamp في CI. الـ ceremony في [مفاتيح الـ feed والـ ceremony](#مفاتيح-الـ-feed-والـ-ceremony)، والتهديد الأضعف (جهاز Sabry وقت التوقيع) في [جدول التهديدات](#جدول-التهديدات).
3. **cosign (اتغيّر):** **مفيش تحقق من cosign في الـ updater.** الطول والـ hash من TUF targets بس، و`sigstore-go` اتشال من الاعتماديات. مفتاح targets لوحده بقى كفاية لتحديث، ومقبول لأنه مفتاح واحد عمره ما بيخرج من الـ USBين المشفّرين، ومعاه job مراقبة بيكشف أي فرق. و`release.yml` بيفضل يوقّع بـ cosign للي بيتحقق بإيده.
4. **Compose:** helper على الـ host بيركّبه الـ installer، والـ Compose اليدوي إشعار بس، ومفيش sidecar بالـ Docker socket. (زي التوصية.)
5. **مكان الـ feed (اتغيّر):** نفس الريبو، من غير ريبو جديد ولا دومين. الأرشيفات release assets على `Khaymah/dhow` زي النهارده والـ targets بتشاور عليها، والـ metadata على GitHub Pages بتاع الريبو: `https://khaymah.github.io/dhow/updates/`. والـ timestamp بيتجدّد كل أسبوع بصلاحية 7 أيام وتنبيه لو فشل، ونفس نشر الـ Pages هيشيل موقع الوثائق في M10 ([النشر](#النشر-على-github-pages)).
6. **التحديث الحرج بره النافذة:** إعداد `CriticalOutsideWindow`، مقفول افتراضيًا. (زي التوصية.)
7. **النسخ الاحتياطية:** مفتاح على الجهاز لنسخة ما قبل التحديث (U9)، وتصدير بمفتاح `age` عام في U5. (زي التوصية.)
8. **`/meta` العام:** `Version` و`Commit` بيتشالوا، ويظهروا في «عن داو» بعد الدخول بس (U6). (زي التوصية.)
9. **الدعم:** آخر minor بس بياخد patches لحد GA، ونرجع للقرار بعد 3 شهور من GA. (زي التوصية.)
10. **`dhow migrate --open-pr`:** بيشغّل `gh` أو `glab` بتوع المستخدم نفسه (argv)، ومفيش كود شبكة في `dhow`. (زي التوصية.)

**مفيش سؤال مفتوح.**

## قرارات تانية اتاخدت بالتوصية (Sabry يقدر يرجع فيها)
1. **النافذة الافتراضية:** كل يوم 03:00 لمدة ساعتين بتوقيت UTC، لحد ما المالك يختار منطقته. والـ installer بيسأل.
2. **الاحتفاظ:** 3 نسخ قبل الترقية، والـ binary القديم 7 أيام.
3. **فحص الصحة:** 120 ثانية للجاهزية و60 ثانية ثبات.
4. **الـ major مش أوتوماتيك أبدًا** (قرار 14).
5. **الـ `rc`** للي بيختاره بس، ومن غير أوتوماتيك.
6. **مفيش retry أوتوماتيك** لنسخة رجعنا منها.
