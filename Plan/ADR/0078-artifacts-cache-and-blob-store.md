# ADR-0078: الـ artifacts والـ cache: رفع وتنزيل بـ HTTP مربوط بالـ lease، وblob store على القرص (M4.9)

- **الحالة:** مقبول. القرار 6 مفوَّض في [`M4-Pipelines.md`](../M4-Pipelines.md)، والقرص بس ورا interface `Core/Blob` إجابة Sabry على السؤال 4. والباقي تفاصيل تنفيذ مفوَّضة ومكتوبة هنا.
- **التاريخ:** 2026-11-02
- **بيكمّل:** [ADR-0042](0042-runner-protocol-and-leasing.md) و[ADR-0053](0053-runner-implementation.md) (الـ lease والـ fencing) و[ADR-0075](0075-multi-container-attempts-checkout-and-plugins.md) (الـ workspace المشترك وصورة الـ checkout) و[ADR-0076](0076-pipeline-runs-definitions-and-forks.md) (الـ forks) و[ADR-0077](0077-upgrades-migrations-and-update-policy.md) (الـ migrations expand بس).

## السياق
- `.dhow.yml` فيه `artifacts:` و`cache:` من M4.1، والـ runner بيعلن الميزتين، بس محدش كان بيرفع ولا بينزّل حاجة.
- Connect عندنا unary بحد 1 MiB للرسالة، والـ artifact ممكن يبقى جيجات.
- الـ workspace tmpfs مركّب جوه الـ user namespace بتاع Podman (ADR-0075)، فالـ runner نفسه مايشوفهوش على الـ host، وده مقصود: أي ملف من job ماينفعش يتفك بصلاحيات مستخدم الـ runner.
- الـ fork بيقرا الـ cache بس (ADR-0076)، وشرط خلاص M4 إن PR مايقدرش يسمّم cache الـ `main`.
- M3 أجّل احتفاظ الـ logs لكل مؤسسة لـ M4 (ADR-0046 وADR-0052).

## القرار

### 1. القرار 6: HTTP عادي على `/runner-api/blobs/{sha256}` مربوط بالـ lease
- **الـ routes:** `PUT` و`GET /runner-api/blobs/{sha256}` جنب الـ Connect handlers في `Artifacts.Routes.go`، ومتعلنين في الـ policy table بـ `policy.RunnerHTTP` (GET وPUT على `/blobs/` بس) وبيعدّوا على نفس الـ gate (`Gate.HTTP`): الـ protocol header، ومن غير cookie جلسة متصفح، وتوكن الـ runner. والـ `BodyLimit` بتاع الـ runner plane (1 MiB) مابيتطبّقش على `/blobs/` بس، لأن الـ handler عنده حده بنفسه.
- **الـ lease:** كل طلب فيه `Dhow-Lease-Id` و`Dhow-Fencing-Token`، والسيرفر بيدوّر على lease `active` ماخلصش بنفس الـ fencing ولنفس الـ runner، وغير كده `409 LEASE_LOST`. الرفع بيتأكد من الـ lease قبل ما يقرا أول بايت، وتاني جوه الـ transaction اللي بتحط الملف مكانه، فـ lease خلص في النص مابيسيبش blob.
- **الرفع (`Dhow-Blob-Kind: artifact|cache`):** الـ job لازم يكون من pipeline ومعلن artifacts أو cache (وغير كده `403 NOT_DECLARED`)، والـ cache مش للقراية بس وليه scope يكتب فيه (وغير كده `403 CACHE_READ_ONLY`). `Content-Length` إجباري (`411`) ولازم يبقى تحت `DHOW_ARTIFACT_MAX_BYTES` أو حد الـ cache للمشروع (`413` قبل ما نقرا)، والجسم ورا `MaxBytesReader` بنفس الحد.
- **التنزيل:** محتاج `Dhow-Download-Token` لنفس الـ digest بالظبط. التوكن HMAC-SHA256 بمفتاح مشتق من `DHOW_SECRET_KEY` (`dhow/artifact-grant/v1`) على (النوع، وقت الانتهاء، الـ subject، الـ digest):
  - **artifact:** الـ subject هو الـ `RunJob` اللي هيستهلكه، فالتوكن مربوط بتنفيذ واحد، وبيتولّد وقت الإدراج في `ArtifactInput.download_token` ويعيش 72 ساعة (طابور 24 ساعة + أطول job + هامش). بس من غير lease حي للـ job ده مالوش أي قيمة.
  - **cache:** الـ subject هو الـ lease نفسه، وبيرجع من `CacheService.Lookup` ويعيش 15 دقيقة.
  - التوكن مابيتكتبش في أي log، ومش موجود في تفاصيل الـ job في الـ API (الـ `JobSpec` بتاع الـ API مافيهوش `inputs`).
- **الـ Connect:** `ArtifactService.Commit` و`CacheService.Lookup` و`CacheService.Commit` بنفس الـ lease ref. الـ commit بيتأكد إن الـ blob اتخزّن فعلًا بنفس الحجم، وإن اسم الـ artifact معلن في الـ spec.
- **اللي اترفض:** Connect streaming (محتاج HTTP/2 من الأول للآخر وفريمات بحد)، وروابط موقّعة من غير lease (تتسرّب وتفضل شغّالة بعد ما الـ job يخلص).

### 2. السؤال 4: القرص بس ورا `Core/Blob`
- **الـ interface:** `Stage` (اكتب وتأكد) ثم `Commit` (حطه مكانه) أو `Abort`، و`Open` و`Remove` و`SweepTemp`. S3 بعد أول نسخة على نفس الـ interface.
- **الكتابة:** ملف مؤقت `.upload-<random>` في نفس مجلد الـ shard (`O_EXCL|O_NOFOLLOW` و`0600`)، والـ SHA-256 بيتحسب وإحنا بنكتب، و`fsync`، وبعدها `rename` للاسم النهائي و`fsync` للمجلد. digest غلط أو حجم غير المعلن: `422 DIGEST_MISMATCH` والملف المؤقت بيتمسح.
- **المسار:** من الـ digest بس: `<DHOW_BLOB_DIR>/<أول حرفين>/<64 حرف hex>`، والـ digest لازم يبقى 64 حرف hex صغير. مفيش اسم من المستخدم في أي مسار.
- **القرص:** قبل الكتابة بنتأكد إن المساحة الفاضية أكبر من الحجم زائد 512 MiB احتياطي، وكل 64 MiB بنتأكد تاني، وغير كده `507 STORAGE_FULL`. فـ artifact كبير بيقف في النص من غير ما يملا القرص.
- **الإقلاع (fail closed):** `DHOW_BLOB_DIR` (افتراضيًا `/var/lib/dhow/blobs`) لازم يكون مجلد حقيقي مش symlink، ومالكه مستخدم السيرفر، ومقفول على الباقي (`0700`)، وقابل للكتابة، وغير كده السيرفر مابيقومش. مع `DHOW_DEV=true` ومن غير قيمة بنعمل مجلد في `/tmp`. والـ unit بقت `StateDirectory=dhow dhow/blobs`، وصورة Docker فيها المجلد بمالكه، والـ Compose عنده volume `blobs`.

### 3. الـ runner: sandbox للنقل جوه الـ user namespace
- **الـ sandbox:** نوع جديد `KindTransfer` من صورة الـ checkout المتثبّتة (صورتنا، مش صورة المستخدم)، من غير شبكة، وبنفس التقوية والـ workspace والـ uid map. الـ agent بيشغّل `dhow-runner` نفسه بـ `RunExec`: `__artifact-pack` و`__artifact-unpack` و`__hash-files`. فالفك والجمع وhash الملفات بيحصلوا كـ root الـ job جوه الـ namespace بتاعه، عمرهم ما بيحصلوا كمستخدم الـ runner على مسارات الـ host.
- **الترتيب:** بعد الـ checkout وقبل أول خطوة: ننزّل كل artifact من الـ `needs` لملف في مجلد الـ attempt على الـ host، ونتأكد من الـ SHA-256 والحجم هناك، وبعدها بس نبعته للفك على stdin (frame جديد `Input` في بروتوكول الـ agent، و`RunExec.stdin`). وبعد نجاح كل الخطوات: نجمع كل artifact على stdout ونحسب الـ digest ونوقف الجمع بـ `SIGKILL` لو عدّى الحد، وبعدها نرفع ونعمل commit.
- **الأرشيف الحتمي:** tar.gz بـ PAX، والأسماء مرتبة، والوقت صفر، ومن غير uid ولا gid ولا أسماء، والصلاحيات `0644` أو `0755` بس، والـ gzip من غير اسم ولا وقت. الـ symlink لازم يفضل جوه الـ workspace (مش مطلق، ومايطلعش بـ `..` من مكانه)، وأي device أو FIFO أو socket بيفشّل الـ job.
- **الفك:** بـ `os.Root` على `/workspace`، وبيرفض: `..` في أي جزء، والمسار المطلق، والـ NUL، والـ symlink اللي بيخرج، والكتابة من خلال symlink أو ملف (كل جزء في المسار لازم يبقى مجلد حقيقي)، والـ devices والـ FIFOs، والـ hard link لحاجة مش ملف عادي اتفك قبل كده. والملف الموجود بيتمسح ويتعمل من جديد بـ `O_EXCL`، فـ symlink متزرع مايتكتبش من خلاله. والحجم الكلي تحت قرص الـ job. وفيه `FuzzUnpack` بيتأكد إن مفيش حاجة اتكتبت جنب الـ workspace ولا link بيخرج ولا ملف خاص.
- **أسباب فشل جديدة:** `ARTIFACT_DOWNLOAD_FAILED` و`ARTIFACT_UNSAFE` و`ARTIFACT_UPLOAD_FAILED` و`ARTIFACT_TOO_LARGE`. والـ cache مابيفشّلش job أبدًا (تحذير في السجل)، إلا لو الأرشيف نفسه طلع unsafe.
- **الـ features:** الـ runner بيعلن `artifacts` و`cache` بس لو عنده صورة checkout، و`jobspec.Features` بيطلبهم لأي job فيه artifacts أو inputs أو cache، فـ runner قديم مابياخدوش.

### 4. مفتاح الـ cache: السيرفر بيقيّم والـ runner بيكمّل `hash_files()`
- السيرفر بيقيّم `cache.key` بـ CEL زي باقي التعبيرات، وكل نداء لـ `hash_files()` بيرجع علامة برقمها وبيتسجّل الـ patterns بتاعه. والنتيجة بتبقى template (`%` بيتكتب `%%`، و`%N` مكان الـ hash رقم N) في `Cache.key_expression` ومعاه `hashes[]`.
- لو التعبير استخدم الـ hash في أي حاجة غير النص (`size(...)` أو شرط أو قايمة)، العلامات مابتطلعش كلها مرة واحدة بالظبط، والـ job بيفشل `EXPRESSION_FAILED`. الحد 8 نداءات و32 pattern لكل نداء، والـ patterns نسبية.
- الـ runner بيحسب كل hash في sandbox النقل في بداية الـ job (SHA-256 على `المسار\0sha256(الملف)\n` للملفات العادية المطابقة مرتبة بالمسار، من غير ما يتبع links)، ويكمّل الـ template، والمفتاح النهائي من 1 لـ 512 حرف من غير control characters.
- **ليه مش CEL في الـ runner؟** cel-go كبير، والسيرفر عنده كل السياق (git وinputs وmatrix وenv)، والـ runner محتاج hash الملفات بس. والفرق الوحيد: hash مابيتستخدمش غير كنص.

### 5. الـ scope وسياسة الكتابة
- الـ scope بيتحسب في السيرفر من الـ run بتاع الـ lease، عمره ما بيجي من الـ runner: push أو manual أو api على فرع `branch:<الفرع>`، وtag `tag:<الاسم>`، وPR من نفس المستودع `pr:<الرقم>`.
- البحث: scope الـ run الأول، وبعده `branch:<الفرع الافتراضي>` من `"Repositories"`. والـ fork (وأي PR أصله مش معروف) بيقرا الفرع الافتراضي بس، ومالوش scope يكتب فيه، والـ `JobSpec` بتاعه فيه `read_only`، والرفع والـ commit بيترفضوا `CACHE_READ_ONLY` من السيرفر حتى لو الـ runner تجاهل العلامة.
- الحفظ بعد نجاح الـ job بس ولو المفتاح ماكانش موجود في البداية (الـ runner)، و`ON CONFLICT DO NOTHING` (السيرفر)، فمفتاح موجود عمره ما بيتغيّر. وبعد كل إضافة، لو مجموع المشروع عدّى `DHOW_CACHE_MAX_BYTES_PER_PROJECT` بنعلّم الأقدم استخدامًا (`"LastUsedAt"`) محذوف تحت قفل للمشروع.
- **Untrusted:** artifacts الـ run من fork بتتعلّم `"Untrusted"`. والـ port `Core/ArtifactIndex.Prepare` بياخد `FeedsDeploy` (job فيه `environment:` أو خطوة `mutating`)، ولو فيه artifact untrusted بيرجع `Refused` والـ job بيفشل `ARTIFACT_UNTRUSTED` قبل الإدراج. ده شغّال من دلوقتي حتى قبل M6.

### 6. الاحتفاظ والتنضيف
- **الـ artifacts:** `"ExpiresAt"` = أقل حاجة من `retention` في الملف و`DHOW_ARTIFACT_RETENTION_DAYS` (افتراضيًا 30). الـ sweeper كل 10 دقايق بيعلّم المنتهي محذوف.
- **mark and sweep على `"Blobs"`:** قفل advisory على مستوى الـ session (`pg_try_advisory_lock`)، وفي transaction بنعلّم الـ blobs اللي مالهاش artifact ولا cache entry ولا اتستخدمت من 6 ساعات، وبعد الـ commit بنمسح الملفات، وبعدها نفك القفل. والرفع والـ commit بياخدوا نفس القفل shared جوه الـ transaction بتاعهم وقت ما بيحطوا الملف مكانه أو بيربطوه، فمفيش blob بيتمسح وهو بيتربط. لو الـ process وقع بعد الـ commit وقبل المسح بيفضل ملف يتيم بس (مش مرجع لملف ممسوح)، والرفع الجاي لنفس الـ digest بيعمله row جديد.
- الملفات المؤقتة الأقدم من 24 ساعة بتتمسح، والـ 6 ساعات أكبر من أي lease، فـ blob لسه بيترفع أو اترفع ومستني commit مابيتمسحش.
- **الـ migration:** `00046` expand بس (جداول جديدة وعمود nullable)، وبيبدأ بـ `-- dhow:phase expand`.

### 7. احتفاظ الـ logs لكل مؤسسة (البند المتأجل من M3)
- عمود `"Organizations"."LogRetentionDays"` (nullable، من 1 لـ 3650)، و`PUT /organizations/{id}/log-retention` بصلاحية `org.update` وجلسة بس، و`null` يرجّع لإعداد السيرفر، وإعداد السيرفر بيكسب لو أقصر.
- القراية (الصفحات والـ SSE) بتخبّي الأقدم فورًا، والـ maintenance كل ساعة بيعمل `"IsDeleted" = true` و`"Data" = '\x00'` للأقدم من مدة المؤسسة، فالمحتوى بيتمسح فعلًا من غير `DELETE` (الـ partition بيتشال بعدين بمدة السيرفر).

## العواقب
- الـ backup لازم ياخد `DHOW_BLOB_DIR` مع القاعدة، وأكتر من replica للسيرفر محتاج S3 (بعد أول نسخة، مع HA في M8).
- الـ runner اللي من غير صورة checkout مابياخدش jobs فيها artifacts أو cache (`NO_MATCHING_RUNNER` ظاهر).
- كل artifact بيعدّي على قرص الـ runner مرة (ملف في مجلد الـ attempt) قبل الرفع أو الفك، فقرص الـ runner لازم يكفي أكبر artifact.
- **لسه:** أسماء أسباب الفشل الجديدة وقايمة الـ artifacts في الواجهة (M4.10)، وS3، و`hash_files()` بيتحسب مرة في بداية الـ job (ملف بيتغيّر بعدها مابيغيّرش المفتاح).
