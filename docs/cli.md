[English](en/cli.md)

# أداة سطر الأوامر `dhow`

`dhow` ملف تنفيذي واحد ثابت لـ Linux وmacOS وWindows، لمعماريتي `amd64` و`arm64`. أول أمر فيه هو `dhow lint`: يفحص ملف `.dhow.yml` بالمدقق نفسه الذي يستخدمه سيرفر داو (`internal/pipelinespec`)، فالملف الذي يقبله يقبله السيرفر أيضًا. ويعمل دون شبكة: لا يفتح أي اتصال، ولا يتصل بأي سيرفر. والأمر الثاني هو `dhow artifact push`: يرفع ملفًا ناتجًا إلى سيرفر داو ([الشرح](guides/ingest-action.md)). وهو الأمر الوحيد الذي يستخدم الشبكة.

## التثبيت

### عبر get.sh (Linux)

نزّله، واقرأه، ثم شغّله. لا تمرّره أبدًا إلى shell مباشرة:

```sh
curl -fsSLO https://raw.githubusercontent.com/Khaymah/dhow/main/deploy/install/get.sh
less get.sh
bash get.sh --component cli
```

الخيار `--version 1.2.3` يختار إصدارًا، والافتراضي هو آخر إصدار. يفعل السكربت ما يلي:

1. ينزّل `dhow_<version>_linux_<arch>.tar.gz` و`SHA256SUMS` و`SHA256SUMS.sigstore.json` من إصدار GitHub؛
2. يفحص توقيع cosign على `SHA256SUMS` مقابل workflow الإصدار في `Khaymah/dhow` (بلا مفتاح، عبر Sigstore) إذا كان `cosign` مثبّتًا، ويحذّر بأنه يفحص الـ checksum فقط إذا لم يكن مثبّتًا؛
3. يفحص الأرشيف مقابل سطره في `SHA256SUMS`، ويتوقف إذا كان السطر ناقصًا أو لا يطابق؛
4. يثبّت `/usr/local/bin/dhow` (`0755`، ومالكه root).

ثبّت `cosign` أولًا لتحصل على فحص التوقيع: بدونه، يثبت الـ checksum فقط أن التنزيل لم يتلف، ولا يثبت من بناه. مع مرآة (`DHOW_DOWNLOAD_URL` أو `DHOW_REPO`) يصبح cosign إلزاميًا، كما في [دليل المثبّت](install/installer.md). عند تشغيله بمستخدم عادي، ينسخ السكربت الملفات إلى مجلد لا يكتب فيه إلا root، ويفحصها ويفك أرشيفها هناك قبل أن يثبّت أي شيء بـ `sudo`.

### يدويًا (macOS وWindows وLinux)

في كل إصدار أرشيف لكل نظام: `dhow_<version>_<os>_<arch>.tar.gz`، والقيمة `<os>` هي `linux` أو `darwin` أو `windows`. أرشيف Windows فيه `dhow.exe`، ويفك الأمر `tar` الأرشيف في Windows 10 وما بعده. نزّل الأرشيف و`SHA256SUMS` و`SHA256SUMS.sigstore.json` من صفحة الإصدار، ثم شغّل هذه الأوامر بالإصدار والنظام الدقيقين لأرشيفك (هنا 1.2.3 على جهاز Mac بمعالج Apple):

```sh
cosign verify-blob --bundle SHA256SUMS.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github\.com/Khaymah/dhow/\.github/workflows/release\.yml@refs/tags/v' \
  SHA256SUMS
sha256sum --check --ignore-missing SHA256SUMS
tar -xzf dhow_1.2.3_darwin_arm64.tar.gz
sudo install -m 0755 dhow /usr/local/bin/dhow
dhow version
```

على جهاز Mac ليس فيه `sha256sum`، افحص سطر أرشيفك بالأمر `shasum`: `grep dhow_1.2.3_darwin_arm64 SHA256SUMS | shasum -a 256 -c`. ملفات macOS غير موقّعة من Apple بعد (لا codesign ولا notarization). إذا رفض Gatekeeper فتح `dhow`، فافحص التوقيع والـ checksum كما سبق أولًا، ثم احذف علامة الحجر: `xattr -d com.apple.quarantine /usr/local/bin/dhow`. ولكل أرشيف أيضًا SPDX SBOM (`<archive>.spdx.json`) وإثبات بناء SLSA على صفحة الإصدار.

### من الكود المصدري

```sh
make build-cli
./bin/dhow version
```

## الأمر `dhow lint`

```sh
dhow lint
dhow lint path/to/.dhow.yml
dhow lint path/to/repo
dhow lint --format json --lang en .dhow.yml
```

دون مسار، يفحص `.dhow.yml` في المجلد الحالي. وإذا كان المسار مجلدًا، فيفحص `.dhow.yml` الذي فيه. يقرأ 256 KiB بالأكثر، وهو حد المدقق، ويرفض أي شيء ليس ملفًا عاديًا (مثل FIFO أو جهاز) دون أن يقرأه. ولا يتبع أي رابط رمزي (symlink) أبدًا: إذا كان `.dhow.yml` رابطًا، يتوقف بكود الخروج 2، فلا يستطيع pull request أن يوجّهه إلى ملف آخر على الجهاز (مثل `~/.docker/config.json`) ليظهر أسماء مفاتيحه في سجل CI عام.

| الخيار | المعنى |
|---|---|
| `--format text` | سطور مقروءة، سطر لكل مشكلة، ثم ملخص. هذا هو الافتراضي. |
| `--format json` | مستند JSON واحد للمحررات والسكربتات (انظر أدناه). |
| `--lang en` أو `--lang ar` | لغة الرسائل. نأخذها افتراضيًا من `LC_ALL` أو `LC_MESSAGES` أو `LANG`: العربية للغة `ar_*` بترميز UTF-8، والإنجليزية في غير ذلك. |
| `--help` | يعرض الأوامر وأكواد الخروج. |

يمكن كتابة الخيارات قبل المسار أو بعده. واكتب `--` قبل أي مسار يبدأ بـ `-`.

## أكواد الخروج

| الكود | المعنى |
|---|---|
| `0` | الملف صحيح. وقد تكون فيه تحذيرات، مثل صورة مثبّتة بالـ tag فقط. |
| `1` | في الملف خطأ واحد على الأقل، والسيرفر يرفضه. |
| `2` | الملف غير موجود، أو رابط رمزي، أو ليس ملفًا عاديًا، أو لا يمكن قراءته، أو الأمر غير صحيح (خيار غير معروف، أو قيمة خاطئة، أو أكثر من مسار). لا يطبع شيئًا على stdout. |

## المخرجات

في المخرجات النصية سطر لكل مشكلة: الملف، والسطر والعمود، ونوع المشكلة، والكود، والمسار داخل الملف، والرسالة. والسطر الأخير ملخص:

```text
.dhow.yml:7:5: warning IMAGE_NOT_PINNED at jobs.build.image: the image is not pinned by digest: the runner refuses it unless unpinned images are allowed
.dhow.yml:9:9: error RUN_EXPRESSION_NOT_ALLOWED at jobs.build.steps[0].run: ${{ }} is not allowed inside run: pass the value through env and read it as a variable
.dhow.yml: errors: 1, warnings: 1. The server refuses this pipeline.
```

حروف التحكم (C0 وC1) وحروف الاتجاه غير المرئية في اسم الملف أو في مفتاح تظهر كرموز مكتوبة (`\u001b` و`\u009b` و`\u202e`)، والشرطة المائلة العكسية تظهر `\\`، في المخرجات النصية وفي JSON، فلا يستطيع ملف من مستودع أن يحرّك المؤشر أو يغيّر ترتيب السطر في الـ terminal.

الخيار `--format json` يطبع مستندًا واحدًا. شكله ثابت في `SchemaVersion` رقم 1: قد نضيف حقولًا جديدة، ولا نغيّر اسم أي حقل ولا نحذفه:

```json
{
  "SchemaVersion": 1,
  "File": ".dhow.yml",
  "Lang": "en",
  "Valid": false,
  "Errors": 1,
  "Warnings": 0,
  "Issues": [
    {
      "Code": "RUN_EXPRESSION_NOT_ALLOWED",
      "Severity": "error",
      "Path": "jobs.build.steps[0].run",
      "Line": 9,
      "Column": 9,
      "Message": "${{ }} is not allowed inside run: pass the value through env and read it as a variable"
    }
  ]
}
```

| الحقل | المعنى |
|---|---|
| `Code` | الكود الثابت بصيغة `UPPER_SNAKE`، وهو نفس كود السيرفر. اعتمد عليه، ولا تعتمد على الرسالة أبدًا. |
| `Severity` | `error` أو `warning`. |
| `Path` | مكان المشكلة (`jobs.build.steps[0].run`)، أو فارغ إذا كانت المشكلة في الملف كله. |
| `Line` و`Column` | المكان في الملف، ويبدأ العد من 1، أو 0 إذا لم يوجد مكان. |
| `Message` | الرسالة بلغة `Lang`. والرسالة الإنجليزية هي رسالة السيرفر نفسها. ولا تكرر أي قيمة من الملف أبدًا. |

قيمة `Issues` هي `[]` للملف الصحيح. الأكواد موجودة في [مرجع خطوات النشر](pipeline-spec.md#11-التحقق)، وملفات golden في `apps/cli/internal/App/testdata` تعرض المخرجات بالضبط.

## دون شبكة

لا يستخدم `dhow lint` الشبكة أبدًا. لا يفحص ملفات manifest للإضافات، ولا ينزّل الصور، فيفحص `uses:` و`image:` بشكلهما فقط (والـ digest مطلوب في `uses:`). ثلاثة اختبارات تضمن ذلك:

- الأمر يربط فقط الـ packages المكتوبة في `apps/cli/cmd/dhow/testdata/deps.linux-amd64.txt`، فأي package جديد يُفشل الاختبار حتى يراجعه أحد. وبعد ADR-0099 صارت فيها حزم الشبكة (`net` و`net/http` و`crypto/tls`) لأن `dhow artifact push` يحتاجها، وحزمة واحدة فقط من الكود هي التي تستوردها: `internal/ingestpush`، ويفحص اختبار ذلك. أما `os/exec` فليس بينها، ولا يوجد في الملف التنفيذي `os.StartProcess` ولا `syscall.ForkExec` ولا `syscall.Exec`؛
- يشغّل الاختبار الملف التنفيذي على كل ملف خاطئ من اختبارات المدقق، داخل network namespace بلا أي شبكة، وتحت `strace`، فلا يظهر في السجل أي system call للشبكة، ولا أي `execve` بعد الأول، والنتيجة هي نفسها خارج الـ namespace. وبرنامج صغير يفتح socket في الإعداد نفسه يثبت أن `strace` سيراه. ويجري الاختبار نفسه على `dhow artifact push --dry-run`، فلا يتصل بالشبكة قبل الإرسال؛
- يرفض hook في المشروع داخل `apps/cli` أي استيراد لحزم الشبكة، أو لـ `syscall` و`golang.org/x/sys` و`unsafe` وcgo، وأي استدعاء لـ `os.StartProcess` أو `syscall.Exec` أو `syscall.ForkExec`. فكود الأوامر في `apps/cli` لا يستورد الشبكة بنفسه، بل عبر `internal/ingestpush`.

## الأوامر الأخرى

| الأمر | المعنى |
|---|---|
| `dhow artifact push` | يعبّئ مجلدًا أو يأخذ `.tar.gz` ويرفعه إلى سيرفر داو بتوكن استقبال. تدخل الملفات المخفية في الأرشيف، ويُرفض مجلد فيه `.git` في جذره. انظر [رفع الملفات الناتجة](guides/ingest-action.md). |
| `dhow version` | يطبع الإصدار، والـ commit، وإصدار Go. |
| `dhow help` | يعرض الأوامر وأكواد الخروج. |
