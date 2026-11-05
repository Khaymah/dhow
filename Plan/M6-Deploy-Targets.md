# خطة M6: أهداف النشر (Deploy Targets)

**الحالة:** مسودة للمراجعة. مستنية إجابة Sabry على [الأسئلة الـ 8](#أسئلة-لـ-sabry). الخطة مكتوبة كلها على التوصية، وأي إجابة مختلفة بتعدّل المرحلة اللي متعلّمة جنب كل سؤال. القرارات التقنية مفوَّضة ومكتوبة بالتوصية، وكل واحد منها بيبقى ADR في أول المرحلة بتاعته (الأرقام من `0090`، والرقم الفعلي وقت الكتابة) · **التاريخ:** 2026-11-05

## الهدف
شرط خلاص M6 في `docs/roadmap.md` (الصف M6، بعد [ADR-0068](ADR/0068-first-production-release-scope.md)): **كل هدف بينشر، وبيفشل في فحص الصحة، وبيرجع لوحده، في اختبار E2E.** ده معناه:
- **SSH/VPS:** إصدارات ذرية (`releases/<id>` وتبديل symlink واحد)، وفحص صحة HTTP، ورجوع تلقائي للإصدار اللي قبله، وكل ده بيتجرّب على sshd حقيقي في الـ CI.
- **Docker Compose:** نشر على سيرفر بعيد بصور متثبّتة بالـ digest، وانتظار الصحة، ورجوع تلقائي، على Docker حقيقي في الـ CI.
- **استقبال الملفات الناتجة من CI خارجي:** API رفع بتوكن محدود، وGitHub Action رسمية، وفحص digest، والرفع بيشغّل نشر. ([ADR-0010](ADR/0010-build-or-ingest-artifacts.md): المسارين بينتجوا artifact معنوَن بمحتواه، والنشر بيشاور له بالـ digest، والرجوع عمره ما بيعيد البناء.)
- **سجل النشر بيقول الحقيقة:** الهدف، والإصدار، وفحص الصحة، والرجوع، من غير ما أي سر يظهر.

المراجع:
- [ADR-0011](ADR/0011-deploy-targets.md) (SSH بـ symlink ذرّي وفحص صحة ورجوع تلقائي، وCompose، والـ host keys بتتثبّت لكل target، وكل target ليه E2E للفشل والرجوع) و[ADR-0068](ADR/0068-first-production-release-scope.md) (SSH وCompose بس في أول نسخة، وKubernetes وStatic/CDN/SFTP ومكوّن GitLab بعدها).
- [ADR-0009](ADR/0009-container-image-plugins.md) و[ADR-0075](ADR/0075-multi-container-attempts-checkout-and-plugins.md) (الإضافة صورة حاوية بـ manifest، وبتاخد الـ inputs وأسرار خطوتها بس).
- [ADR-0072](ADR/0072-webhooks-and-git-credentials.md) (الـ host key بيتجاب بـ handshake وبيتثبّت بالتأكيد بس، و`netguard`) و[ADR-0057](ADR/0057-egress-proxy-implementation.md) (الـ egress) و[ADR-0047](ADR/0047-job-state-machine-and-reconciliation.md) (مفيش إعادة أوتوماتيك بعد أول خطوة `mutating`).
- [ADR-0078](ADR/0078-artifacts-cache-and-blob-store.md) (الـ artifacts والـ blob store، وأن artifacts الـ forks `Untrusted`) و[ADR-0083](ADR/0083-secrets-variables-and-registry-credentials.md) و[ADR-0084](ADR/0084-secret-delivery-forks-and-cache.md) و[ADR-0085](ADR/0085-environment-gate-and-deployment-lock.md) و[ADR-0086](ADR/0086-deployment-approvals.md) (الأسرار والبوابة والقفل والموافقات).
- [`M5-Secrets-Environments.md`](M5-Secrets-Environments.md): الخطة دي بتبني عليه ("M6 بعد M5: أهداف النشر بتقرا عنوان السيرفر من متغيّرات البيئة ومفتاحه من أسرارها (ملف)، والـ host key المثبّت للهدف في M6").
- `docs/pipeline-spec.md` (§2 المثال، و§3 الإضافات، و§8 «مش مدعوم لسه»، و§9 البيئات)، و`docs/architecture.md` (§4 و§8 و§9)، و`docs/security-model.md` (§11 دروس Deploy Center v3).

**قبلها:**
- **M5.9ب** (#104، `deploy/m5-demo` ومراجعة التهديدات): الـ lib بتاع الـ demo هو اللي M6 بيمدّه.
- **U3** (اختبار الترقية في الـ CI) مش شرط، بس M6.1 وM6.6 فيهم migrations، فلو U3 اتعمل قبلهم بنضيف له حالة «binary M5 على قاعدة M6».
- M4.5 وM4.6 مش شرط (مفيش حاجة في M6 بتعتمد على GitHub App أو GitLab OAuth، والـ Action بتكلّم داو مش GitHub API).

## القرارات اللي الخطة مبنية عليها (ملزمة، من قبل M6)
1. **الأهداف في أول نسخة:** SSH/VPS وDocker Compose بس. Kubernetes وStatic/CDN/SFTP ومكوّن GitLab CI بعد أول نسخة (ADR-0068).
2. **البيئة هي اللي بتحدد الهدف:** الأهداف جزء من البيئة ومعرّفة على السيرفر. الـ commit مايقدرش يختار هدف خارج بيئة الـ job، ومفاتيح السيرفرات أسرار في كل بيئة (Status، قرار Sabry يوم 2026-11-02).
3. **الرجوع عمره ما بيعيد البناء** (ADR-0010 و`architecture.md` §8): بيرجّع إصدار موجود أو artifact بالـ digest.
4. **مفيش إعادة أوتوماتيك بعد أول خطوة بتغيّر في الهدف** (`architecture.md` §4 وADR-0047). الإعادة بتاعة البنية التحتية قبل أول خطوة `mutating` بس.
5. **الـ forks:** من غير أسرار ولا بيئات أبدًا، وartifacts الـ forks `Untrusted` وعمرها ما بتغذّي نشر (ADR-0076 وADR-0078).
6. **كله ببلاش** (ADR-0077): مفيش خدمة ولا جهاز مدفوع مطلوب.
7. **أسرار الهدف write-only وللجلسة بس:** كتابتها من جلسة، والتوكن بياخد قراية الأسماء بس (قرار Sabry 6 في M5).
8. **الموافقات بالخطوة التانية** (قرار Sabry 5 في M5)، وقاعدة الـ ref على السيرفر مالهاش تخطّي.

## النطاق
1. **الأهداف (`"DeployTargets"`):** نموذج وAPI على البيئة: النوع (`ssh` أو `compose`)، والعنوان، والمستخدم، ومسار النشر، ومفتاح الدخول (سر في البيئة)، والـ host key المتثبّت بالتأكيد، وإعدادات النوع.
2. **توصيل الهدف للـ runner:** رسالة `StepTarget` في الـ `JobSpec`، والـ egress للهدف بس، والمفتاح كملف لخطوة الإضافة دي بس، وعلامة `Mutating` من الـ manifest، وقناة تقارير للإصدار.
3. **إضافة `dhow-deploy-ssh`:** رفع لـ `releases/<id>`، وروابط `shared/`، وتبديل symlink ذرّي، وhooks، وفحص صحة HTTP، ورجوع تلقائي، وتنضيف `keep_releases`.
4. **إضافة `dhow-deploy-compose`:** نشر Compose على سيرفر بعيد بـ SSH، والصور بالـ digest، وانتظار الصحة، ورجوع تلقائي.
5. **سجل النشر:** `"DeploymentReleases"` (الهدف، ورقم الإصدار، والإصدار السابق، والـ digest، ونتيجة الفحص، وسبب الرجوع)، وحالة `running` في `"Deployments"`.
6. **الرجوع اليدوي وفحص الهدف:** تشغيلات اصطناعية (synthetic runs) بيعملها السيرفر بالإضافة الرسمية، من غير YAML.
7. **استقبال الـ artifacts من CI خارجي:** موديول `Ingest` (توكنات محدودة، وAPI رفع، وفحص digest، وسجل منشأ)، والـ trigger `on.artifact` و`ingested:` في الـ spec، و`dhow artifact push` في الـ CLI، وGitHub Action رسمية.
8. **الواجهة (الحد الأدنى):** أهداف البيئة وسجل النشر والرجوع وتوكنات الاستقبال. المنتج الكامل في M7.
9. **الأمان والاختبارات:** مراجعة تهديدات (§14 في `docs/security-model.md`)، وE2E لكل هدف على sshd وDocker حقيقيين، وفحص التسريب، و`deploy/m6-demo/run.sh`.
10. **الوثائق بالنسختين** (`make docs-check`).

## مش في M6
- **Kubernetes وStatic/CDN/SFTP:** بعد أول نسخة على نفس الـ `Kind` والـ interface (ADR-0011 وADR-0068).
- **مكوّن GitLab CI:** بعد أول نسخة. الـ API بتاعة الاستقبال عامة (curl من أي CI)، والـ Action بتاعة GitHub بس هي الرسمية.
- **مُصدِر OIDC للأحمال** (M5.8): بعد أول نسخة. الهدف بيستخدم مفتاح SSH سر عادي. وتحقق توكن GitHub OIDC عند الاستقبال [سؤال 4](#4-توثيق-الرفع-من-ci-خارجي) متأجّل كمان.
- **Canary وblue/green والنشر على أكتر من سيرفر في هدف واحد:** [سؤال 7](#7-مجموعة-سيرفرات). في M6 الهدف = سيرفر واحد، والمجموعة بـ matrix على أهداف.
- **agent على سيرفر الهدف:** [سؤال 1](#1-فين-النشر-بيشتغل) و[سؤال 5](#5-docker-compose-عن-بعد).
- **الإشعارات** (إيميل وSlack عند الفشل أو الرجوع): M7. الحدث `deployment.finished` بيتنشر على الـ EventBus وجاهز لها.
- **أسرار الخدمات على سيرفر الهدف:** Compose بيقرا `.env` من `shared/` على السيرفر نفسه. كتابة أسرار داو على قرص السيرفر البعيد مش في M6 (edge case).
- **تسجيل الدخول لـ registry على سيرفر الهدف:** `docker login` بيتعمل على السيرفر نفسه. الإضافة عمرها ما بتبعت بيانات registry لسيرفر بعيد.
- **أكتر من مراجع ونوافذ النشر:** M8.

## المعمارية
```
 browser ─► proxy ─► /api/v1 ─┬─ Modules/Environments   targets, releases, rollback, target check (on top of M5 gate/lock/deployments)
                              ├─ Modules/Ingest         ingest tokens, ingested artifacts, provenance        (new)
                              └─ Modules/Pipelines      Enqueue ──► Core/Deployments.Targets (port, same tx) ──► JobSpec.targets
 CI (GitHub Action / curl) ─► /ingest/v1  (outside /api/v1, policy.Ingest, scoped token)
                              PUT artifact ─► Core/Blob ─► "IngestedArtifacts" ─► Core/Triggers.Deliver(Event=artifact)

 dhow-runner ─► /runner-api  Start(lease, fencing) ─► SecretResolver: env secret ──HPKE──► dhow.target.<n>.key (as_file, this step only)
                                    JobSpec.targets[]: kind host port user known_hosts_line deploy_path release_id (no secret)
                                    ReleaseService.Report(lease, event) ──► "DeploymentReleases"   (fenced, progress + result)
   runner sandbox (egress = exact target endpoints + health host only):
     plugin container: dhow-deploy-ssh / dhow-deploy-compose
        ├─ artifact tar.gz (digest verified by the runner) ─► streamed to the host ─► releases/<id>/
        ├─ SSH: x/crypto/ssh, FixedHostKey(pinned), key from $DHOW_TARGET_KEY_FILE, dial through the runner proxy (CONNECT)
        └─ health check (HTTP via proxy) ─► keep new release │ switch back to the previous one
   target host:  <DeployPath>/{releases/<id>/, shared/, current -> releases/<id>, .dhow/{lock,state.json,verified}}
```

## الأهداف كجزء من البيئة

### النموذج
- **الهدف تابع لبيئة** (مش لمشروع ولا مؤسسة): اسمه بنفس قاعدة الـ spec (`^[a-z0-9][a-z0-9_-]{0,63}$`) وفريد جوه البيئة. نفس السيرفر في بيئتين = هدفين بمفتاحين مختلفين. بيعمله Admin بس (`environment.manage`، خطوة تانية)، زي البيئة نفسها.
- **الـ job بيشاور على الهدف باسمه** في `with: target:` على إضافة رسمية. الاسم لازم يطلع نص ثابت بعد توسيع الـ matrix، وغير كده `TARGET_NOT_LITERAL`. السيرفر بيحل الاسم وقت الإدراج من بيئة الـ job بس. هدف مش موجود: `TARGET_NOT_FOUND`، وjob فيه هدف من غير `environment:`: `TARGET_REQUIRES_ENVIRONMENT` (الاتنين `failed`، غلطة إعداد).
- **الإضافات الرسمية بس بتستلم هدف:** الاسم `ghcr.io/khaymah/dhow-deploy-ssh` و`ghcr.io/khaymah/dhow-deploy-compose` (ثوابت في الـ binary)، وAdmin يقدر يضيف بادئة صورة مخصوصة للهدف ده بس (`"AllowedImages"`). خطوة `uses:` ليها `with.target` وصورتها مش في القايمة: `TARGET_PLUGIN_NOT_ALLOWED`. لأن المفتاح بيتسلّم للإضافة، وإضافة طرف ثالث مايحصلش تاخد مفتاح سيرفر الإنتاج من غير ما حد يقرر كده.
- **الـ digest بيفضل في الملف** (`uses` لازم `@sha256:`)، فكاتب الـ commit بيختار نسخة الإضافة الرسمية. مقبول ومكتوب (§14).

### الحقول (`"DeployTargets"`)
| الحقل | الوصف |
|---|---|
| `Kind` | `ssh` أو `compose` |
| `Host` و`Port` | اسم host أو IP (public)، والبورت الافتراضي 22. لازم يعدّي `netguard` وقت الحفظ إلا لو الـ host خاص (الـ runner لازم يكون عنده `ALLOW_PRIVATE_CIDRS` للنطاق ده، [سؤال 8](#8-أول-تثبيت-لـ-host-key-وهدف-في-شبكة-خاصة)) |
| `Username` | حساب نشر مخصوص (مش root). `root` مرفوض إلا بـ `AllowRoot` صريح من الأدمن بتحذير |
| `KeySecretName` | اسم سر **على نفس البيئة** فيه المفتاح الخاص (ed25519). نطاق المؤسسة والمشروع مرفوض هنا (قرار Sabry 2026-11-02: مفاتيح السيرفرات أسرار البيئة) |
| `PublicKey` | المفتاح العام (مش سر)، بيتعرض عشان يتحط في `authorized_keys` |
| `HostKey` و`HostKeyFingerprint` | المفتاح المتثبّت (نوع واحد، ed25519 مفضّل) وبصمته `SHA256:`، بتتأكد بإيد إنسان بس |
| `PendingHostKey` و`PendingFingerprint` | مفتاح اتجاب أو اتلصق وماتأكدش لسه، أو مفتاح جديد ظهر والقديم لسه شغّال |
| `DeployPath` | مسار مطلق على السيرفر، `^/[A-Za-z0-9._/-]{1,200}$` من غير `..` وغير `/` ولا مسارات النظام (`/etc` و`/usr` و`/bin` و`/var/lib/docker`...) |
| `KeepReleases` | عدد الإصدارات المحفوظة، من 2 لـ 50، الافتراضي 5 |
| `Settings` (jsonb، 4 KiB) | حسب النوع: `ssh`: `ReloadCommand` و`PreSwitchCommand` و`PostSwitchCommand` ([سؤال 6](#6-الـ-hooks)) و`Shared` (قايمة أسماء ملفات ومجلدات). `compose`: `ProjectName` و`ComposeFile` والسياسة (`AllowUnsafeCompose`) |
| `AllowedImages` | بادئات صور إضافة مخصوصة (فاضية = الرسميتين بس) |
| `Version` | optimistic زي البيئة |

- **التفرّد:** (`Host`, `Port`, `DeployPath`) فريد في المؤسسة (`TARGET_DUPLICATE`)، عشان بيئتين مايشاوروش على نفس المجلد ويتعاركوا على `current`.
- **المسح:** soft، ومرفوض لو فيه إصدار `running` (`TARGET_IN_USE`). المسح بيسيب سجل الإصدارات. الـ `KeySecretName` سر البيئة بيفضل (بيتمسح مع البيئة).
- **توليد المفتاح:** `POST .../targets/{t}/generate-key` بيعمل ed25519 جديد، وبيحط الخاص سر في البيئة باسم `<TARGET>_SSH_KEY` (مش `DHOW_`)، وبيعرض العام مرة ويتخزّن. نفس فلسفة ADR-0071: داو بيعمل المفتاح، المستخدم مابيلمسش خاص. واللصق (مفتاح موجود) متاح. تدوير = توليد جديد + تأكيد + شيل القديم من `authorized_keys` (الواجهة بتفكّر).

### الـ host key (على نمط ADR-0072)
- **الجلب:** `POST .../targets/{t}/fetch-host-key`: السيرفر بيعمل handshake بس (`ssh.NewClientConn` و`HostKeyCallback` بيوقف بعد المفتاح، من غير مصادقة) فوق `netguard`، مهلة 10 ثواني، من غير lock في القاعدة. النتيجة `PendingHostKey`.
- **اللصق:** `PUT .../targets/{t}/pending-host-key` بسطر `ssh-keyscan` أو محتوى `.pub` (الأأمن: المفتاح اتجاب من السيرفر نفسه، من غير TOFU).
- **التثبيت:** `POST .../targets/{t}/confirm-host-key` بنفس البصمة `SHA256:` + `CurrentFactor`. عمرنا ما بنقبل مفتاح لوحده. لو الـ `Host` أو `Port` اتغيّر، المفتاح المثبّت بيتشال.
- **التغيير:** لو المفتاح اتغيّر على السيرفر، الإضافة بتفشل `HOST_KEY_MISMATCH` من غير ما ترفع حاجة. الأدمن بيجيب المفتاح من جديد ويأكد بصمته (والـ audit فيه القديمة والجديدة).
- **وقت النشر:** الإضافة بتستخدم `ssh.FixedHostKey` وبتحدد `HostKeyAlgorithms` على نوع المفتاح المثبّت بس. مفيش `known_hosts` ولا `ssh_config` ولا `StrictHostKeyChecking`.
- **من غير مفتاح مثبّت:** `TARGET_HOST_KEY_UNCONFIRMED` وقت الإدراج، والـ job `failed` بسبب واضح.

### الـ API (للإضافة على M5)
| Route | الإجراء | ملاحظة |
|---|---|---|
| `GET .../environments/{e}/targets` و`.../targets/{t}` | `project.read` | من غير أي مفتاح خاص. بيرجّع الحالة والمنشور دلوقتي والـ host key ثبت ولا لأ |
| `POST .../environments/{e}/targets` | `environment.manage` | `CurrentFactor` |
| `PUT .../targets/{t}` | `environment.manage` | `Version` و`CurrentFactor` |
| `POST .../targets/{t}/remove` | `environment.manage` | `CurrentFactor` |
| `POST .../targets/{t}/generate-key` | `environment.manage` | `CurrentFactor`، العام بس في الرد |
| `POST .../targets/{t}/fetch-host-key` و`PUT .../pending-host-key` و`POST .../confirm-host-key` | `environment.manage` | `CurrentFactor` على التأكيد |
| `GET .../targets/{t}/releases` | `run.read` | سجل إصدارات الهدف، صفحات |
| `POST .../targets/{t}/rollback` | `deployment.rollback` | [سؤال 3](#3-الرجوع-اليدوي)، `ReleaseId` اختياري و`CurrentFactor` |
| `POST .../targets/{t}/check` | `environment.manage` | فحص الهدف (تشغيل اصطناعي، تحت) |

- **الأدوار:** `deployment.rollback`: Owner وAdmin وMaintainer (حساس، ADR-0054). باقي الصفوف زي M5. التوكنات مافيهاش أي إجراء كتابة جديد، وتقرا الأهداف بـ `project.read` الموجود. كل route ليه صف في اختبار الـ route-table لكل دور.
- **الأخطاء الجديدة (`UPPER_SNAKE`، مترجمة):** `TARGET_EXISTS` و`TARGET_DUPLICATE` و`TARGET_IN_USE` و`TARGET_VERSION_CONFLICT` و`TARGET_HOST_BLOCKED` و`TARGET_HOST_UNREACHABLE` و`HOST_KEY_NOT_PENDING` و`HOST_KEY_FINGERPRINT_MISMATCH` و`TARGET_KEY_NOT_FOUND` و`DEPLOY_PATH_INVALID`.
- **الـ audit** (بعد الـ commit، أسماء وبصمات وأرقام بس): `target.created` و`target.updated` و`target.removed` و`target.key_generated` و`target.host_key_fetched` و`target.host_key_confirmed` (بالقديمة والجديدة) و`target.checked`.

## توصيل الهدف للـ runner (M6.2)

### وقت الإدراج (`Pipelines.Enqueue`، نفس الـ transaction)
زيادة على فحوص بوابة M5 ([ADR-0085](ADR/0085-environment-gate-and-deployment-lock.md)) بعد «الأسرار موجودة»، عبر port جديد `Core/Deployments.Targets` (interfaces وأنواع بس، زي ADR-0070):
1. **حل الهدف:** لكل خطوة `uses:` بـ `with.target`: الهدف من بيئة الـ job. الأخطاء فوق.
2. **المفتاح موجود:** سر `KeySecretName` على البيئة، وإلا `TARGET_KEY_NOT_FOUND` قبل ما حد يستنى موافقة.
3. **الـ artifact:** الإضافة بتعلن في الـ manifest إن `artifact` مطلوب. الاسم لازم يطابق artifact من `needs` أو `ingested:`، وإلا `TARGET_ARTIFACT_REQUIRED`. وartifact `Untrusted` بيترفض زي ما هو (`ARTIFACT_UNTRUSTED`).
4. **الـ egress** (القاعدة اللي في الطلب «egress للهدف بس»): الـ job اللي فيه خطوة هدف بياخد egress = **نقط الهدف بالظبط** (`host:port`، أو `ip:port` لو العنوان IP) + host الـ `health_check_url` (نص ثابت، `HEALTH_URL_INVALID` لو مش `https`/`http` صح). وأي `network.egress` تاني كاتبه المستخدم في الـ job ده مرفوض `TARGET_JOB_EGRESS_NOT_ALLOWED` إلا لو البيئة فتحت `"AllowExtraDeployEgress"` (افتراضيًا لأ). يعني job النشر لوحده، وإشعار Slack في job تاني. النقط دي بيحسبها السيرفر من الهدف، فالـ commit مايقدرش يوجّه النشر لـ host تاني.
5. **قاعدة IP:** قايمة الـ egress في الـ spec بترفض IP literal (ADR-0057). الهدف بـ IP بيتوصّل في حقل جديد `JobSpec.network.exact_endpoints[]` بيبنيه السيرفر بس (الـ `jobspec` بيرفضه لو جاي من YAML). الـ runner بيثبّت الاتصال على الـ IP والبورت بالظبط، ولسه بيرفض private/loopback/metadata إلا نطاق المشغّل (`ALLOW_PRIVATE_CIDRS`).

### وقت `Start` (SecretResolver في `App`)
- زيادة على فحوص M5 (fork، وبيئة، و`"RuleVersion"`، وإيقاف): الهدف لسه موجود، والـ host key لسه هو المثبّت، و`Version` الهدف مطابق لوقت الإدراج. غير كده `TARGET_CHANGED` من غير ما المفتاح يتفك.
- المفتاح بيتوصل كسر محجوز `dhow.target.<n>.key` من نوع `as_file` بـ `step_indexes` خطوة الإضافة دي بس، ومالوش وجود في الـ spec (زي `dhow.git.checkout`): `jobspec` بيرفضه لو جاي من YAML أو لو مفيش `targets` في الـ job.
- **`JobSpec.targets[]`** (غير سري): `step_index` و`kind` و`host` و`port` و`user` و`known_hosts_line` و`deploy_path` و`release_id` (بيولّده السيرفر: `<رقم الـ run>-<أول 8 من الـ sha>-<رقم الجولة>`، فريد للهدف بـ unique index) و`keep_releases` و`settings` (الـ hooks والسياسة، من الهدف مش من YAML).
- **الـ runner** بيدّي الإضافة: متغيّرات `DHOW_TARGET_*` (host وport وuser وknown_hosts وdeploy_path وrelease_id)، وملف المفتاح `DHOW_TARGET_KEY_FILE` (`0400` في tmpfs)، وarchive الـ artifact كملف `DHOW_INPUT_ARTIFACT_FILE` وdigest بتاعه `DHOW_INPUT_ARTIFACT_DIGEST` (الـ runner اتأكد منه). الإضافة مابتاخدش حاجة تانية (ADR-0075). والمفتاح بيتضاف للـ masker.

### علامة `Mutating` وقناة التقارير
- **`mutating`:** حقل `Mutating` في الـ manifest (Version 1، حقل اختياري جديد). الـ runner بيبعت `BeginStep(mutating=true)` للخطوة دي، فالسيرفر يكتب `"MutationStartedAt"` ومفيش إعادة بنية تحتية بعدها (ADR-0047). ده اللي `pipeline-spec` وM4 سابوه لـ M6.
- **`ReleaseService.Report(LeaseRef, DeployEvent)`** (Connect، مربوط بالـ fencing زي `ArtifactService.Commit`): الإضافة بتكتب سطور JSON في `DHOW_DEPLOY_REPORT` (64 KiB و100 حدث بالكتير)، والـ agent بيقراها ويبعتها وقت حدوثها: `release_started` و`uploaded` و`switched` و`health` (نتيجة كل محاولة، مدمجة) و`verified` و`rolled_back` (بالسبب) و`rollback_failed` و`pruned`. السيرفر بيتحقق من الشكل والحدود وأن الحدث لنفس الـ lease والهدف، ومابيصدّقش أي نص حر في الرسائل (اللي بيتخزّن enum وأرقام ورسالة 256 حرف بعد الإخفاء).
- **الإضافة بتبلّغ ومابتقررش:** نتيجة الـ job من exit code، والتقرير للسجل والواجهة. إضافة بتكدّب في تقريرها بتأثر على سجل بيئتها بس، وهي أصلًا معاها مفتاح الهدف.
- **الـ features:** `deploy_targets` في التسجيل والدخول الموقّع (زي `checkout` و`plugins`). job فيه هدف مابياخدوش runner قديم.
- **أسباب فشل جديدة** (`"RunJobs"."FailureReason"`) و`AttemptFailureReason`: `TARGET_NOT_FOUND` و`TARGET_NOT_LITERAL` و`TARGET_REQUIRES_ENVIRONMENT` و`TARGET_HOST_KEY_UNCONFIRMED` و`TARGET_KEY_NOT_FOUND` و`TARGET_PLUGIN_NOT_ALLOWED` و`TARGET_ARTIFACT_REQUIRED` و`TARGET_JOB_EGRESS_NOT_ALLOWED` و`TARGET_CHANGED` و`HEALTH_URL_INVALID` (إدراج/بدء)، و`HOST_KEY_MISMATCH` (موجود) و`DEPLOY_FAILED` و`DEPLOY_HEALTH_FAILED` و`DEPLOY_ROLLED_BACK` و`DEPLOY_ROLLBACK_FAILED` و`DEPLOY_INTERRUPTED` (الإضافة).

## إضافة SSH/VPS (`dhow-deploy-ssh`)

### فين بتشتغل
جوه الـ runner sandbox زي أي إضافة (ADR-0075)، مش على السيرفر ولا على سيرفر الهدف. أسبابها: كود النشر بيشتغل في نفس العزل (حدود الموارد والـ egress والـ masker وإلغاء الـ job)، والسيرفر مابيحتاجش يوصّل شبكيًا لحد، والمفتاح بيوصل لـ sandbox واحدة لفترة الخطوة بس. ([سؤال 1](#1-فين-النشر-بيشتغل).) الإضافة Go binary في صورة `FROM scratch` أو Alpine متثبّتة، موقّعة بـ cosign ومعاها provenance من `release.yml`، والـ digest بتاعها بيتكتب في أمثلة الوثائق عند كل release.

### المدخلات (`with:`، معلنة في الـ manifest)
| الاسم | مطلوب | الوصف |
|---|---|---|
| `target` | آه | اسم الهدف في بيئة الـ job (نص ثابت) |
| `artifact` | آه | اسم artifact من `needs` أو `ingested:`. يوصل كأرشيف مش مفكوك |
| `strategy` | لأ | `atomic-symlink` (الوحيد النهارده، وغيره `STRATEGY_NOT_SUPPORTED`) |
| `keep_releases` | لأ | يغطي على قيمة الهدف لو أقل منها (مش أكتر) |
| `health_check_url` | لأ | `http(s)` فقط |
| `health_check_status` | لأ | افتراضيًا `200-399` |
| `health_check_timeout` | لأ | مهلة الطلب، افتراضيًا 5s، أقصى 30s |
| `health_check_retries` | لأ | افتراضيًا 10، أقصى 60 |
| `health_check_interval` | لأ | افتراضيًا 5s، من 1s لـ 60s |
| `health_check_stable` | لأ | عدد النجاحات المتتالية المطلوبة، افتراضيًا 3 |
| `health_check_start_delay` | لأ | تأخير قبل أول محاولة، افتراضيًا 0، أقصى 300s |
| `on_first_failure` | لأ | `keep` (افتراضيًا) أو `fail`: لو مفيش إصدار سابق للرجوع له |

- **سقف الوقت الكلي لفحص الصحة 10 دقايق**، ومجموع `start_delay + retries × (interval + timeout)` لازم يعدّي الحد وإلا `HEALTH_BUDGET_EXCEEDED` وقت الإدراج. الـ job `timeout` لازم يغطيه.
- **الفحص بيتعمل من الإضافة** (من الـ runner عبر الـ proxy)، بـ TLS كامل التحقق، ومن غير `insecure` ولا headers مخصوصة (مفيش سر في الفحص). الـ redirects لحد 3 وعلى نفس الـ host بس، والرد بيتقري 64 KiB وبيتهمل. الأخطاء بتتسجّل بالحالة والنوع (مهلة، رفض اتصال، شهادة) من غير الجسم.
- **host الفحص لازم يكون في الـ egress** اللي السيرفر بيحسبه (§توصيل): السيرفر بيضيفه من `health_check_url` الثابت، فالـ commit مايقدرش يضيف host تاني.

### خوارزمية النشر
مسار السيرفر البعيد `<DeployPath>`: `releases/<id>/` و`shared/` و`current` (symlink) و`.dhow/{lock, state.json, verified}`.

1. **الاتصال:** `x/crypto/ssh` بـ `FixedHostKey` ومفتاح عام واحد، وبدون agent ولا forwarding ولا pty، ومهلة اتصال 15s، وkeepalive. الاتصال بيعدّي من `HTTPS_PROXY` بـ `CONNECT` (مش `ProxyCommand`، فمفيش اعتماد على helper جوه حاوية الإضافة).
2. **فحوص مسبقة:** `DeployPath` موجود ومالكه المستخدم (`test -d`)، ومساحة فاضية ≥ ضعف حجم الأرشيف، و`current` symlink أو مش موجود (لو مجلد حقيقي: `CURRENT_NOT_A_SYMLINK` ومانلمسوش، والوثائق بتشرح التحويل). **القفل البعيد:** `mkdir .dhow/lock` ذري ومعاه `owner` فيه رقم الـ run ووقت الانتهاء. قفل منتهي بيتاخد بتحذير، وقفل شغّال `TARGET_LOCKED`. (القفل الحقيقي قفل البيئة في السيرفر، ده دفاع تاني.)
3. **الرفع:** بـ SFTP (`github.com/pkg/sftp`) على `releases/.incoming-<id>/` من stream الأرشيف **من غير فك على القرص**: كل عنصر بيعدّي نفس قواعد الفك الآمن (`..` والمسارات المطلقة والـ links اللي بتخرج والـ devices والـ hard links مرفوضة، `ARTIFACT_UNSAFE`)، والصلاحيات `0755` أو `0644` بس، وحد الحجم والعدد. وبعد الرفع manifest `sha256` للملفات بيتأكد على السيرفر (`sha256sum -c`).
4. **المشاركة (`shared`):** روابط نسبية من الإصدار لـ `../../shared/<name>`، والأسماء من `Settings.Shared` بحروف آمنة وبدون `..`. لو الإصدار فيه ملف حقيقي بالاسم ده: `SHARED_PATH_CONFLICT`.
5. **hook قبل التبديل** ([سؤال 6](#6-الـ-hooks)): من الهدف، جوه مجلد الإصدار الجديد، بمهلة (افتراضيًا 120s، أقصى 600s) ومتغيّرات `DHOW_RELEASE_PATH` و`DHOW_RELEASE_ID` و`DHOW_PREVIOUS_RELEASE`. فشله = الإصدار مايتبدّلش، `failed`، من غير رجوع (لسه ماحصلش تغيير).
6. **التسمية الذرية:** `PosixRename` من `.incoming-<id>` لـ `<id>`.
7. **التبديل الذري:** `previous = readlink current`، `state.json = switching`، وبعدين symlink مؤقت `current.tmp-<id>` بيتعمل `PosixRename` فوق `current`. (rename فوق symlink ذري على POSIX، فمفيش لحظة من غير `current`.)
8. **إعادة التحميل:** `ReloadCommand` ثم hook بعد التبديل (من الهدف). فشل أي واحد = رجوع.
9. **فحص الصحة:** المحاولات حسب الإعدادات. النجاح = `health_check_stable` نجاحات متتالية.
10. **النجاح:** `.dhow/verified = <id>`، وتنضيف الإصدارات الأقدم من `keep_releases` (الإصدارات اللي اسمها `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$` بس، ومن غير `current` ولا `previous` ولا اللي لسه `.incoming`)، وفك القفل، وتقرير `verified` و`pruned`.
11. **الفشل بعد التبديل (الرجوع التلقائي):** تبديل ذري تاني لـ `previous`، وإعادة `ReloadCommand`، وفحص صحة قصير للقديم (معلومة بس)، وتقرير `rolled_back` بالسبب (`health_check_failed` أو `reload_failed` أو `hook_failed` أو `canceled`)، والـ job بيفشل `DEPLOY_ROLLED_BACK`. فشل الرجوع نفسه: `DEPLOY_ROLLBACK_FAILED` وتقرير `rollback_failed` وaudit فوري (أخطر حالة: السيرفر على إصدار مش متأكد منه). **لو مفيش `previous`** (أول نشر): `on_first_failure: keep` يسيب الإصدار ويفشل `DEPLOY_HEALTH_FAILED`، و`fail` برضه يسيبه ويفشل (الفرق في رسالة الواجهة). مانمسحش `current` ولا بنخمّن.
12. **الإلغاء:** الإضافة بتلتقط `SIGTERM`: قبل التبديل بتنضّف `.incoming` وتفك القفل. بين التبديل والنجاح بترجّع (خطوة 11 بسبب `canceled`). مهلة الإلغاء بتاعة ADR-0058 لازم تكفي تبديل واحد وإعادة تحميل (بيتأكد في M6.4).
13. **الانقطاع (الـ runner وقع):** مفيش حاجة تقدر ترجّع. الـ lease بيتحول `lost` والـ reconciler بيعلّم الإصدار `unknown` (تحت)، و`state.json` على السيرفر بيفضل `switching`. النشر الجاي بيكتشف ده (`.dhow/verified != readlink current`)، وبيطلّع تحذير، وبيستخدم `verified` كـ `previous` للرجوع.

- **الـ `Mutating`:** تحت مسؤولية الـ manifest، وأول عملية كتابة على السيرفر (خطوة 2 بعد القفل) بتبعت `BeginStep` اللي فيه `mutating`. ملحوظة: الرفع على `.incoming` مش بيغيّر حاجة شغّالة، بس بنعتبره `mutating` للبساطة.
- **ممنوع:** أي shell محلي. كل أوامر السيرفر البعيد قوالب ثابتة، وأجزاؤها المتغيّرة (`DeployPath` والـ release id وأسماء `shared`) بتعدّي على regex `[A-Za-z0-9._/-]` قبل ما تدخل أمر، فمفيش علامة بتاعة shell بتوصل لـ shell السيرفر البعيد. الاستثناء الوحيد الـ hooks اللي الأدمن كاتبها ([سؤال 6](#6-الـ-hooks)). ده الاستثناء الوحيد لقاعدة «ممنوع تشغيل نص بـ shell»، ومكتوب في ADR (shell الـ login على سيرفر العميل نفسه بقيم متحقق منها، وfuzz target عليه).

### أقل صلاحيات على السيرفر (في `docs/guides/deploy-ssh.md`)
- مستخدم نشر مخصوص (`dhow-deploy`) بكتابة على `DeployPath` بس، من غير `sudo` عام. لو الخدمة محتاجة reload: سطر `sudoers` لأمر واحد (`systemctl reload myapp`)، و`ReloadCommand` بنفس الأمر.
- `authorized_keys` بـ `restrict,from="<IP الـ runner>"` قبل المفتاح (يمنع forwarding والـ pty والمصدر التاني). الإضافة محتاجة exec وsftp subsystem بس.
- مفيش Docker group للمستخدم ده (SSH target). وCompose له وثيقة لوحده (تحت).
- تدوير المفتاح: توليد جديد، تأكيد، شيل القديم.

## إضافة Docker Compose (`dhow-deploy-compose`)
- **نفس المحرك** (`internal/deployrelease`): الاتصال والـ host key والأرشيف والقفل والإصدارات والتقارير والرجوع والـ health HTTP. اللي بيتغيّر التبديل والانتظار: بدل `current` symlink بس، الإصدار مجلد فيه `compose.yaml` وملف `compose.dhow.yaml` الإضافة بتولّده، و`current` symlink برضه (علشان الرجوع يعرف الإصدار السابق ويقدر ينفّذه).
- **الصور بالـ digest:** كل خدمة لازم صورتها `@sha256:` بعد دمج الـ override (من الملف أو من `with.images: "web=ghcr.io/acme/web@sha256:..."`)، وغير كده `COMPOSE_IMAGE_NOT_PINNED`. `build:` مرفوض (`COMPOSE_BUILD_NOT_ALLOWED`): النشر بيسحب مش بيبني (ADR-0010).
- **سياسة آمنة افتراضيًا:** الإضافة بتفحص الملف قبل ما تلمس السيرفر وبترفض `privileged` و`pid: host` و`network_mode: host` و`cap_add` و`devices` و`security_opt` اللي بتقفل seccomp أو apparmor، وbind mounts بره `DeployPath`، و`docker.sock` (`COMPOSE_UNSAFE_<RULE>`). الأدمن يقدر يفتح `AllowUnsafeCompose` على الهدف بتحذير وخطوة تانية. السبب: مستخدم Docker على السيرفر يساوي root عليه، فالملف اللي جاي من commit مايتنفّذش بحدود أوسع من المقصود. (والوثائق بتنصح بـ rootless Docker على الهدف.)
- **الأوامر:** `docker compose --project-name <p> -f <path>/compose.yaml -f <path>/compose.dhow.yaml pull --quiet` ثم `up -d --wait --wait-timeout <N> --remove-orphans=false`. اسم المشروع من الهدف. `--wait` بيستنى healthchecks الحاويات، وفحص HTTP الخارجي فوقه (نفس المدخلات).
- **الرجوع:** نفس خوارزمية SSH: إعادة `up -d --wait` بملفات الإصدار السابق (الـ digests محفوظة فيه)، ثم `current` للقديم. الصور السابقة بتتسحب بالـ digest لو اتمسحت. الإضافة مابتمسحش صور (الوثائق: `docker image prune` مجدول على السيرفر).
- **الاتصال بـ Docker:** [سؤال 5](#5-docker-compose-عن-بعد): أوامر `docker compose` على السيرفر بـ SSH exec، من غير agent ومن غير تعريض Docker API على الشبكة.
- **بيانات الـ registry:** السيرفر نفسه عمل `docker login` قبل كده. مفيش بيانات registry بتتبعت من داو للسيرفر البعيد.

## استقبال الـ artifacts من CI خارجي (`Modules/Ingest`)

### التوكنات (`"IngestTokens"`)
- **شكلها:** `dhow_ing_<43 حرف base64url>` (256 bit)، بتتعرض مرة واحدة وقت الإنشاء، والمخزّن `sha256` (مفتاح عالي الإنتروبيا فمفيش لازمة لـ KDF بطيء) وأول 8 حروف للعرض.
- **مقيّدة:** بمشروع واحد، و`RefPatterns` (1 لـ 20 pattern بـ `internal/pathglob`، إجباري، بنوع `branch` أو `tag`)، وإجراء واحد بس (`artifact.ingest`). مدتها الافتراضية 90 يوم وأقصاها 365، وبتتلغي في أي وقت. عمرها ما بتقرا حاجة ولا بتكتب في حاجة غير الرفع.
- **مين يعملها:** Owner وAdmin وMaintainer (`ingest.manage`، حساس بخطوة تانية)، من جلسة بس. الـ API token ماينفعش يعمل توكن استقبال.
- **الـ revoke:** فوري (فحص القاعدة في كل طلب). `LastUsedAt` بيتحدّث بحد أقصى مرة كل دقيقة.

### الـ API (بره `/api/v1`: `/ingest/v1`، `policy.Ingest` زي `policy.Webhook`)
| Route | الوصف |
|---|---|
| `PUT /ingest/v1/artifacts/{sha256}` | الرفع. الهيدرز: `Authorization: Bearer`، و`Content-Length` (411 من غيره)، و`Dhow-Ingest-Name`، و`Dhow-Ingest-Ref`، و`Dhow-Ingest-Ref-Kind` (`branch` أو `tag`)، و`Dhow-Ingest-Sha` (الـ commit)، و`Dhow-Ingest-Trigger` (`true` افتراضيًا)، و`Dhow-Ingest-Provenance` (JSON صغير 4 KiB) |
| `GET /ingest/v1/artifacts/{sha256}` | حالة الرفع: اتخزّن، وتشغيل اتعمل ولا لأ، ورابط الـ run |

- **ترتيب الفحوص (قبل ما نقرا أول بايت):** الـ protocol وTLS، وبعدين التوكن (وقت ثابت)، وحدود الطلبات للتوكن (30 رفع في الساعة افتراضيًا)، وhosts/ref/sha تعدّي `internal/gitref`، والـ ref يطابق `RefPatterns` (`403 REF_NOT_ALLOWED_FOR_TOKEN`)، والحجم تحت `DHOW_INGEST_MAX_BYTES` (افتراضيًا `DHOW_ARTIFACT_MAX_BYTES`) وحصة المؤسسة (`ARTIFACT_QUOTA_EXCEEDED`)، وعدد الرفع المتزامن على الـ instance (4).
- **الفحص أثناء الاستقبال (stream، من غير فك على القرص):**
  - حساب `sha256` مع الكتابة في `Core/Blob`، ومقارنته بـ `{sha256}` في المسار (`422 DIGEST_MISMATCH` والملف المؤقت بيتمسح).
  - الأرشيف **tar.gz فقط**، وكل عنصر بيتفحص بنفس قواعد الفك الآمن (حزمة `internal/archive` بتتطلّع من الـ runner، فالسيرفر والـ runner بيستخدموا نفس الكود): `..` ومسارات مطلقة وlinks خارجة وdevices وhard links، وحد العدد والحجم بعد الفك والعمق. غير كده `422 ARCHIVE_UNSAFE` ومفيش blob بيتحفظ.
- **الاسم:** نفس قاعدة اسم الـ artifact (`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`).
- **التكرار:** نفس (المشروع، digest، الاسم، ref، sha) مرتين: الرد نفسه من غير run جديد (`Created = false`).

### المنشأ (provenance) والتحقق
- **اللي السيرفر بيتحقق منه:** الـ digest (حسبه بنفسه)، وشكل الأرشيف، والـ ref ضد `RefPatterns` التوكن، و`sha` شكل صح. **اللي بيتسجّل كمُعلَن (مش مُتحقَّق):** المزوّد والمستودع والـ workflow ورقم التشغيل ورابطه والـ actor من `Dhow-Ingest-Provenance`. الواجهة بتكتب «مُعلَن من CI» جنبها، ولو الـ run محتاج موافقة المراجع بيشوفها (والموافقة على الـ digest مش على الادعاء).
- **أن الـ sha فعلًا من المستودع:** الـ trigger بيتعامل معاه زي webhook Git العام: تلميح. الـ checkout (لو الـ pipeline عمله) بيتأكد من تاريخ الـ ref وغير كده `CHECKOUT_REF_MOVED`. والبوابة بتطبّق `AllowedRefs` على الـ ref المُعلَن، فتوكن CI مسروق مقيّد بـ `RefPatterns` ومحتاج موافقة لو البيئة فيها مراجعين.
- **توكن GitHub OIDC (منشأ متحقق):** مش في M6 ([سؤال 4](#4-توثيق-الرفع-من-ci-خارجي))، والجدول فيه عمود `"ProvenanceVerified"` (boolean، false) جاهز له بـ expand.

### من الرفع للنشر
1. **تعريف الـ pipeline:** للـ event `artifact`، التعريف بيتقرا من **الفرع الافتراضي** (زي الأحداث المميّزة في `pipeline-spec.md` §1: PR مايعدّلش صلاحياته، وصاحب توكن CI مايكتبش pipeline)، مش من الـ sha المُعلَن.
2. **الـ spec:** `on.artifact` (جديد) بـ `names` (1 لـ 16، من نفس القاعدة) و`refs` اختياري يضيّق (patterns)، و`jobs.<id>.ingested: [web]` (اسم مقترح، يتثبّت في ADR): الـ artifact بيتنزّل ويتفك في `/workspace` قبل أول خطوة زي `needs`، أو بيوصل لإضافة الهدف كملف (`artifact: web`). `ingested:` من غير `on.artifact` أو باسم مش معلن: `INGESTED_NOT_DECLARED`. و`TRIGGER_NOT_SUPPORTED_YET` على `artifact` بيتشال، و`schedule` يفضل مرفوض.
3. **التشغيل:** `Core/Triggers.Deliver` بـ `Event = "artifact"` (ثابت جديد)، `Provider = "ingest"`، الـ ref والـ sha المُعلَنين، `Actor` = اسم التوكن + الـ actor المُعلَن. مفيش `on.artifact` مطابق: الـ artifact بيتخزّن والرد `Triggered = false` بالسبب `NO_ARTIFACT_TRIGGER`. بيطبّق نفس حدود الـ run (concurrency، والـ forks مش بتنطبق).
4. **الـ run واحد لكل artifact:** `"RunIngestedArtifacts"` بيربط (الـ run، الاسم) بـ `"IngestedArtifacts"`. إدراج الـ job بيعمل `ArtifactInput` بـ `download_token` مربوط بالـ `RunJob` (ADR-0078) زي `needs`.
5. **الإبقاء للرجوع:** الـ artifact اللي إصدار `succeeded` على أي هدف بيشاور له بيتعلّم `"PinnedUntil"` (لحد ما الإصدار يخرج من `keep_releases`)، والـ sweeper مابيمسحوش، والـ mark-and-sweep بتاع `"Blobs"` بيعتبره حيّ. بدون ده، الرجوع بعد 30 يوم كان هيحتاج إعادة بناء.

### `dhow artifact push` وGitHub Action
- **CLI:** `dhow artifact push --server <url> --name web --path dist/ --ref <ref> --sha <sha>`: بيعمل tar.gz حتمي بنفس قواعد M4.9 (من `internal/archive`)، ويحسب الـ digest، ويرفع، ويطبع `digest` و`run-url`. التوكن من `DHOW_INGEST_TOKEN` أو `--token-file` فقط، ومفيش `--token` في argv. أرشيف جاهز بـ `--archive file.tar.gz`.
- **الـ Action** (`Khaymah/dhow-action`، مصدرها `apps/github-action/` في الـ monorepo ومتنسخة بـ `release.yml` عشان الـ Marketplace): composite، بتنزّل `dhow` CLI بنسخة وSHA-256 مثبّتين جوه الـ Action نفسها (مش من الشبكة لآخر نسخة)، وتتحقق من الـ SHA قبل التشغيل، وتشغّل `artifact push`. مدخلات: `server` و`token` و`name` و`path` و`ref` و`sha` و`trigger`. مخرجات: `digest` و`run-url`. التوكن بيتمرّر بمتغيّر بيئة للخطوة، مش argv ولا `with` في log. والوثائق بتنصح بتثبيت الـ Action بـ commit sha وبعدم استخدام التوكن في `pull_request_target`.
- **curl لأي CI تاني** موثّق (الـ API عامة)، ومكوّن GitLab بعد أول نسخة.

## سجل النشر

### `"DeploymentReleases"` (جديد، expand)
صف لكل (نشر، خطوة هدف): `"DeploymentId"` و`"RunJobId"` و`"TargetId"` و`"StepIndex"` و`"Kind"` (`deploy` أو `rollback`) و`"ReleaseId"` (فريد مع الهدف) و`"PreviousReleaseId"` و`"ArtifactDigest"` (bytea 32) و`"State"` IN (`running`, `succeeded`, `failed`, `rolled_back`, `rollback_failed`, `unknown`) و`"Health"` (jsonb صغير: `Attempts` و`Passed` و`LastStatus` و`LastError` (نوع بس) و`DurationMs` و`StableChecks`) و`"RollbackReason"` و`"RollbackAt"` و`"StartedAt"` و`"FinishedAt"` و`"SwitchedAt"`.
- **`"Deployments"."State"`:** بتدخل قيمة `running` (محجوزة في الـ CHECK من M5.6أ) أول `release_started`. نتيجة الـ job من exit code زي ما هي: `rolled_back` يعني الـ job `failed`.
- **«المنشور دلوقتي» للهدف:** آخر إصدار `succeeded` على الهدف. بعد `rolled_back` المنشور هو السابق (لسه `succeeded` من نشر قبله). بعد `unknown`: الواجهة بتقول «الحالة مجهولة» وبتعرض زرار «افحص الهدف».
- **الـ reconciler:** إصدار `running` وlease اتحوّل `lost`/`expired`: بيبقى `unknown` وaudit `deployment.release_unknown`. وإصدار `running` وlease حي بس مفيش تقرير من 15 دقيقة: ما بنعملش حاجة (الـ lease هو الحكم).
- **الـ API:** تفاصيل الـ `RunJob` والنشر بيزيد عليها `Releases[]` (الهدف، والنوع، والإصدار، والسابق، وhex الـ digest، والحالة، والفحص، والسبب). والـ DTOs من غير أي حقل سر (اختبار انعكاس زي M5).
- **الـ audit** (بعد الـ commit): `deployment.release_started` و`deployment.release_switched` و`deployment.release_succeeded` و`deployment.release_rolled_back` (بالسبب) و`deployment.release_rollback_failed` و`deployment.release_unknown` و`artifact.ingested` و`ingest_token.created` و`ingest_token.revoked` و`ingest.rejected` (مرة كل دقيقة لكل توكن، لحد التسريب).
- **حدث الـ bus:** `deployment.finished` (الهدف والحالة والسبب) للإشعارات في M7.

### التشغيلات الاصطناعية (رجوع يدوي وفحص الهدف)
- **الفكرة:** `Pipelines` بيستقبل event جديد `rollback` أو `check` ويبني run من job واحد بإضافة رسمية بالـ digest الثابت في الـ binary (زي `CHECKOUT_IMAGE`)، من غير `.dhow.yml`. بيمر على نفس البوابة والقفل والموافقات والأسرار والـ egress، فمفيش مسار نشر تاني ليه قواعد تانية.
- **`rollback`:** الإدخال (هدف، `ReleaseId` اختياري: الافتراضي `verified` السابق). الإضافة بتتأكد إن `releases/<id>` لسه موجود وتبدّل `current` وتعيد التحميل وتفحص الصحة (بنفس إعدادات آخر نشر ناجح مخزّنة في صف الإصدار)، ولو الدليل اتنضّف وفيه artifact مثبّت (`PinnedUntil`)، بتعيد رفعه من الـ digest (`mode: redeploy`). عمره ما بيعيد البناء. [سؤال 3](#3-الرجوع-اليدوي) بيحدد الموافقة.
- **`check`:** الإضافة بتتصل وتتأكد من الـ host key والمستخدم و`DeployPath` والكتابة والمساحة (و`docker compose version` للـ Compose) وبتقرا `current` و`verified` وتبلّغ بالنتيجة، ولا بتغيّر حاجة. وبتصلّح `unknown` لو اللي على السيرفر واضح. الزرار «افحص الهدف» في الواجهة.

## الأمان
| الخطر | التخفيف | فين بيتختبر |
|---|---|---|
| commit بيوجّه النشر لسيرفر تاني | الهدف بيتحل من بيئة الـ job على السيرفر، واسم فقط من YAML، والـ egress بيحسبه السيرفر | job بيشاور على هدف بيئة تانية أو host في `with`: `TARGET_NOT_FOUND`، ومفيش اتصال |
| مفتاح الهدف بيوصل لكود غلط | سر البيئة بعد قاعدة الـ ref والمراجعة، وfile لخطوة الإضافة الرسمية دي بس، ومعمول له mask | خطوة `run` وإضافة تانية في نفس الـ job ما بتلاقيش الملف ولا القيمة (`/proc` كمان) |
| إضافة طرف ثالث تاخد المفتاح | `TARGET_PLUGIN_NOT_ALLOWED` | `with.target` على صورة مش مسموحة: بيفشل وقت الإدراج |
| MITM على الـ SSH | `FixedHostKey` وتثبيت بالتأكيد بس، ونوع المفتاح بس | host key متغيّر: `HOST_KEY_MISMATCH` ومفيش رفع |
| تسريب المفتاح أو سر في argv أو log | المفتاح ملف، ومفيش سر في argv، والـ masker، وفحص التسريب | `leak-sweep.sh` بيدوّر على المفتاح الخاص والـ known_hosts في كل log وaudit وDTO وأحداث |
| أرشيف عدائي بيكتب خارج الإصدار | قواعد الفك الآمن على الستريم قبل الكتابة في الإضافة والسيرفر | أرشيف بـ `../` ومسار مطلق وsymlink خارج: `ARCHIVE_UNSAFE`/`ARTIFACT_UNSAFE` |
| حقن أمر في shell السيرفر البعيد | قوالب ثابتة وregex `[A-Za-z0-9._/-]`، والـ hooks للأدمن بس | `FuzzRemoteCommand` و`FuzzDeployPath` و`FuzzReleaseID` |
| compose بيدّي root على السيرفر | سياسة افتراضية بترفض الخيارات الخطرة، وتوثيق rootless | compose فيه `privileged` أو bind على `/`: `COMPOSE_UNSAFE_*` |
| نشر بيسيب السيرفر نص نص | تبديل ذري، ورجوع تلقائي، وقفل، و`unknown` صريح | قتل الـ runner بعد التبديل: `unknown`، والنشر الجاي بيكتشف وبيستخدم `verified` |
| الرجوع بيرجّع حاجة مخترقة | الرجوع لإصدار نجح قبل كده على الهدف ده بس، بالـ digest، وبيتسجّل | رجوع لإصدار مش في سجل الهدف: مرفوض |
| artifact متغيّر بعد الموافقة | الموافقة على الـ run والـ digest مثبّت، والـ runner بيتأكد منه، والإضافة كمان | digest مختلف عن المسجّل: `ARTIFACT_DOWNLOAD_FAILED` ومفيش نشر |
| توكن CI مسروق | مقيّد بمشروع وref، ومدته، وقابل للإلغاء، وrate limit، وaudit، وبيعدّي البوابة والموافقة | رفع على ref بره الـ patterns: `403`. وتوكن ملغي: `401` فورًا |
| رفع artifact ضخم أو كتير (DoS) | الحد في الـ header قبل القراية، وحصة المؤسسة، و4 رفع متزامن، وrate limit | `Content-Length` أكبر من الحد: `413` من غير قراية |
| artifact من fork بيغذّي نشر | `Untrusted` ورفض الإدراج (M4.9) | موجود، ويتوسّع بحالة هدف |
| الـ proxy قدام الرفع بيقطع الملفات الكبيرة | وثائق وفحص `proxy-check` لـ `/ingest/` | `make proxy-check` |
| إنكار النشر | audit بالفاعل والهدف والإصدار والـ digest وسلسلة الـ hash | الحدث بالحقول في اختبار الـ audit |
| السر في سجل الـ job من الإضافة | الـ masker، والإضافة مابتطبعش رد السيرفر البعيد كامل (آخر 4 KiB بعد الإخفاء) | قيمة مزروعة في خرج السيرفر البعيد: `***` |

**الأسرار للكتابة بس (اختبار):** نفس اختبار M4 وM5 بيتوسّع على كل DTO جديد (أهداف، وإصدارات، وتوكنات استقبال: التوكن بيتعرض مرة واحدة وقت الإنشاء بس).

## القرارات المفوَّضة (كل واحد بيبقى ADR في أول مرحلته)
| # | القرار | ليه في سطر | المرحلة |
|---|---|---|---|
| 1 | الأهداف جزء من البيئة، والـ job يشاور بالاسم، والسيرفر يحل ويحسب الـ egress | الـ commit مايختارش الوجهة، وقاعدة M5 | M6.1 |
| 2 | الـ host key يتثبّت بالتأكيد بس، بجلب من السيرفر أو لصق، وبنوع مفتاح واحد | ADR-0072، ولصق لشبكات خاصة | M6.1 |
| 3 | مفتاح الهدف سر على البيئة، وداو بيولّده، والخاص مابيتعرضش | ADR-0071 وwrite-only | M6.1 |
| 4 | الإضافات الرسمية بس بتستلم هدف، والمدخلات والمفتاح بيتحددوا في الـ manifest و`StepTarget` | المفتاح مايروحش لإضافة طرف ثالث | M6.2 |
| 5 | egress الـ job اللي فيه هدف = نقاط الهدف + host الفحص بالظبط، و`exact_endpoints` للـ IP | شرط «egress للهدف بس»، وADR-0057 بيرفض IP في القايمة | M6.2 |
| 6 | `Mutating` في الـ manifest، وقناة `ReleaseService.Report` مربوطة بالـ fencing | ADR-0047، والتقرير حي مش بعد النهاية | M6.2 |
| 7 | الـ artifact بيوصل الإضافة أرشيف متحقق من الـ digest مش مفكوك، وفكه ستريم بقواعد الفك الآمن | مفيش فك على القرص، ونفس قواعد M4.9 | M6.2 |
| 8 | مكان الكود الجديد: `apps/plugins/deploy-ssh` و`apps/plugins/deploy-compose` و`apps/github-action` و`internal/deployrelease` و`internal/archive` | ADR-0004 بيتعدّل، والمحرك مشترك بين الإضافتين | M6.3 |
| 9 | الإصدار `releases/<id>` والتبديل بـ `PosixRename` فوق symlink، والقفل `mkdir` | ذري على POSIX، وSFTP بيدعمه | M6.3 |
| 10 | الرجوع التلقائي وفحص الصحة (أرقام ومهلة 10 دقايق) | [سؤال 2](#2-الرجوع-التلقائي-وإيه-اللي-يشغّله) | M6.3 |
| 11 | shell السيرفر البعيد: قوالب ثابتة وregex، والـ hooks من الهدف | الاستثناء الوحيد لقاعدة الـ shell | M6.3 وM6.4 |
| 12 | الإصدار `unknown` والـ reconciler والانقطاع | الصدق في السجل أهم من التخمين | M6.4 |
| 13 | التشغيلات الاصطناعية (`rollback` و`check`) بتعدّي على نفس البوابة | مفيش مسار تاني للنشر | M6.4 |
| 14 | سياسة Compose الآمنة والصور بالـ digest | docker group = root | M6.5 |
| 15 | `Modules/Ingest` منفصل، وتوكنات محدودة، و`/ingest/v1` بـ `policy.Ingest` | [سؤال 4](#4-توثيق-الرفع-من-ci-خارجي)، ومش API token | M6.6 |
| 16 | `on.artifact` و`ingested:` والتعريف من الفرع الافتراضي | صاحب توكن CI مايكتبش pipeline | M6.6 |
| 17 | الـ artifact المنشور بيتثبّت لحد ما يخرج من `keep_releases` | الرجوع من غير إعادة بناء | M6.6 |
| 18 | الـ Action composite بنسخة وSHA مثبّتين، ومنشورة من الـ monorepo | سلسلة توريد، وتتختبر في الـ CI | M6.7 |

## قاعدة البيانات
كل الجداول بالأعمدة الأربعة، والوقت `timestamptz`، والنص `TEXT`، والحذف soft، ومفيش `DELETE FROM` في `Queries/`، وكل ملف أوله `-- dhow:phase expand`، والـ indexes على جداول موجودة `CONCURRENTLY` في ملف `NO TRANSACTION` لوحده. الأرقام وقت الـ merge (آخر رقم حاليًا `00066`).

| الجدول | الموديول | أهم الأعمدة والقيود |
|---|---|---|
| `"DeployTargets"` | Environments | `"OrganizationId"` و`"ProjectId"` و`"EnvironmentId"` (FK مركّب) و`"Name"` (unique مع البيئة WHERE NOT deleted) و`"Kind"` IN (`ssh`, `compose`) و`"Host"` و`"Port"` (1..65535) و`"Username"` و`"KeySecretName"` و`"PublicKey"` و`"HostKey"` و`"HostKeyFingerprint"` و`"HostKeyConfirmedAt"` و`"HostKeyConfirmedBy"` و`"PendingHostKey"` و`"PendingFingerprint"` و`"DeployPath"` (CHECK regex) و`"KeepReleases"` (2..50) و`"Settings"` (jsonb 4 KiB) و`"AllowedImages"` (`TEXT[]`) و`"Version"`، وunique (`"OrganizationId"`, `"Host"`, `"Port"`, `"DeployPath"`) |
| `"DeploymentReleases"` | Environments | الأعمدة اللي فوق، وunique (`"TargetId"`, `"ReleaseId"`)، وindex (`"TargetId"`, `"CreatedAt"` DESC) |
| `"IngestTokens"` | Ingest | `"OrganizationId"` و`"ProjectId"` و`"Name"` و`"TokenHash"` (bytea 32، unique) و`"Prefix"` و`"RefPatterns"` (jsonb، 20) و`"ExpiresAt"` و`"LastUsedAt"` و`"RevokedAt"` و`"CreatedByUserId"` |
| `"IngestedArtifacts"` | Ingest | `"OrganizationId"` و`"ProjectId"` و`"TokenId"` و`"Name"` و`"Digest"` (bytea 32) و`"SizeBytes"` و`"Ref"` و`"RefKind"` و`"Sha"` و`"Provenance"` (jsonb 4 KiB) و`"ProvenanceVerified"` (false) و`"ExpiresAt"` و`"PinnedUntil"`، وunique (`"ProjectId"`, `"Digest"`, `"Name"`, `"Ref"`, `"Sha"`) |
| `"RunIngestedArtifacts"` | Pipelines | `"RunId"` و`"Name"` و`"IngestedArtifactId"`، unique (`"RunId"`, `"Name"`) |

- **أعمدة جديدة (expand):**
  - `"Environments"."AllowExtraDeployEgress"` (`boolean NOT NULL DEFAULT false`).
  - `"RunJobs"`: مفيش أعمدة جديدة (الأسباب الجديدة في `"FailureReason"`، `TEXT` من غير CHECK).
  - الـ mark-and-sweep بتاع `"Blobs"` بيعتبر الـ digests في `"IngestedArtifacts"` حيّة (تعديل في الـ sweeper، مش عمود).
- **الـ FK بين الموديولات:** مفيش (زي M4 وM5)، والتحقق في الـ service. `"DeployTargets"` جوه Environments فالـ FK مباشر على `"Environments"`.
- **الـ `Core`:** `Core/Deployments` بيزيد `Targets` و`Releases` (interfaces وأنواع بس). `Core/Triggers` بيزيد `EventArtifact` و`EventRollback` و`EventCheck`. و`Core/ArtifactIndex` بيزيد `PrepareIngested`.
- **الـ proto (إضافات بس، و`buf breaking` أخضر):**
  ```
  JobSpec          += targets[] StepTarget { step_index, kind, host, port, user, known_hosts_line, deploy_path, release_id, keep_releases, settings }
                   += network.exact_endpoints[] { host_or_ip, port }       (server-built only)
  ArtifactInput    += kind = file_for_plugin (the archive mounted, not unpacked)
  ReleaseService.Report(LeaseRef, DeployEvent) → ack                      (fenced)
  AttemptFailureReason += DEPLOY_FAILED, DEPLOY_HEALTH_FAILED, DEPLOY_ROLLED_BACK, DEPLOY_ROLLBACK_FAILED, DEPLOY_INTERRUPTED
  features: deploy_targets
  plugin manifest (label) += Mutating, Inputs.<n>.Kind = artifact|text
  ```
- **الإعدادات الجديدة** (`docs/install/configuration.md` بالنسختين): `DHOW_INGEST_MAX_BYTES` (افتراضيًا `DHOW_ARTIFACT_MAX_BYTES`). والثوابت (مش env): حدود الفحص والمدد، وصور الإضافات الرسمية، وحدود الاستقبال.

## الواجهة (حد أدنى، المنتج الكامل في M7)
- **صفحة البيئة:** تبويب «الأهداف»: الجدول (الاسم، النوع، العنوان، المنشور دلوقتي، آخر نتيجة)، وشريحة «ضيف هدف» (بياناته، وتوليد المفتاح وعرض العام مرة، وجلب المفتاح أو لصقه وتأكيد البصمة بخطوة تانية)، وزرار «افحص الهدف». نص تحذير لو الـ host key مش مثبّت.
- **سجل النشر والـ run:** أعمدة «الهدف» و«الإصدار» و«الصحة» و«رجع لـ…». شارة «رجع تلقائيًا» بلون الخطر وأيقونة، وسبب الرجوع، وعدد محاولات الفحص. حالة «النتيجة مجهولة» مع زرار الفحص. وزرار «رجّع لإصدار سابق» (شريحة: اختيار الإصدار وخطوة تانية).
- **توكنات الاستقبال:** في إعدادات المشروع: الجدول، وشريحة «توكن جديد» (الاسم، وpatterns الفروع، والمدة، والتوكن بيظهر مرة واحدة)، وقايمة «ملفات اتستقبلت» (الاسم والـ digest والـ ref والمنشأ المُعلَن وهل اتشغّل).
- **القواعد:** كل نص مفتاح في `Locales/Workspace/{Ar,En}.ts`، ومصطلحات في `Plan/Glossary.md` (هدف نشر، وإصدار، وتبديل ذري، وفحص صحة، ورجوع تلقائي، واستقبال artifact)، ومفيش قيمة سر في state أو URL أو `localStorage`، وCSP: مفيش `<style>`، والصفحات lazy.

## الوثائق (بالنسختين، و`make docs-check`)
| الملف | المرحلة | المحتوى |
|---|---|---|
| `docs/guides/deploy-ssh.md` (جديد) | M6.3 | تجهيز السيرفر بأقل صلاحيات، والهدف، والـ host key، والمدخلات، والرجوع، وأخطاء شائعة |
| `docs/guides/deploy-compose.md` (جديد) | M6.5 | السياسة الآمنة، والصور بالـ digest، والـ registry على السيرفر، ورجوع |
| `docs/guides/ingest.md` (جديد) | M6.6 | التوكن، وAPI الرفع بـ curl، والـ Action، و`on.artifact` و`ingested:`، وأمان التوكن |
| `docs/guides/environments.md` | M6.1 | قسم الأهداف، وقاعدة egress النشر |
| `docs/pipeline-spec.md` | M6.2 وM6.6 | مثال §2 (استبدال `prod-fleet` بـ matrix على أهداف [سؤال 7](#7-مجموعة-سيرفرات))، وقسم «إضافات النشر الرسمية»، و§8 (شيل `artifact` من «مش مدعوم»)، و`ingested:`، و§9 (الأهداف) |
| `docs/architecture.md` | M6.3 | §8 و§9 (Compose بـ SSH exec، والاستقبال) و§4 (حالة `unknown`) |
| `docs/security-model.md` | M6.9 | §14 مراجعة تهديدات M6 (الجدول والمخاطر المقبولة)، وإضافات لقايمة المراجعة الخارجية |
| `docs/install/configuration.md` وأدلة الـ reverse proxy | M6.6 | `DHOW_INGEST_MAX_BYTES`، وحد الجسم والمهلة لـ `/ingest/` |
| `docs/runner/*` | M6.2 | نطاقات المشغّل للأهداف الخاصة (`ALLOW_PRIVATE_CIDRS`) |
| `docs/roadmap.md` | M6.9 | M6 خلص |
| `CLAUDE.md` (في PR M6.3) | M6.3 | `apps/plugins` و`apps/github-action` و`internal/deployrelease` و`internal/archive` في جدول الهيكل |

## Edge cases
- **أول نشر والسيرفر مفيهوش `current`:** بيتعمل أول `current`، ومفيش رجوع (`on_first_failure`).
- **`current` مجلد حقيقي (نشر قديم بـ rsync):** `CURRENT_NOT_A_SYMLINK`، والوثائق بتشرح خطوات التحويل اليدوي. مانلمسوش.
- **السيرفر الهدف وقع في النص:** اتصال مقطوع قبل التبديل: `failed` والـ `.incoming` بيتنضّف في النشر الجاي. بعد التبديل: `DEPLOY_INTERRUPTED`، والنشر الجاي يكتشف.
- **الـ runner وقع بعد التبديل:** الإصدار `unknown` والـ job `lost`، وعمره ما بيتفترض إنه نجح (ADR-0047). «افحص الهدف» أو النشر الجاي بيصلّحوه.
- **فحص الصحة نجح للإصدار القديم مش الجديد (الـ URL ثابتة):** الفحص ممكن يعدّي بالغلط لو الخدمة لسه ماشية بالقديم. الوثائق بتقول: خلّي `/healthz` يرجّع رقم الإصدار، و`health_check_stable` يمنع أغلب الحالات. (لسه مقبول ومكتوب §14.)
- **الخدمة محتاجة وقت إعادة تحميل أطول من المهلة:** زوّد `health_check_start_delay` أو `retries`. السقف الكلي 10 دقايق.
- **`keep_releases` أقل من إصدارين:** مرفوض (الحد الأدنى 2 عشان الرجوع).
- **تنضيف إصدار هو `previous` أو `verified`:** مستحيل بالتصميم (مستثنى).
- **نشرين على نفس الهدف من بيئتين:** مستحيل (`TARGET_DUPLICATE` على host وport والمسار).
- **نشرين على نفس البيئة:** قفل البيئة بتاع M5 بيسلسلهم، وقفل `mkdir` البعيد دفاع تاني.
- **matrix على أهداف:** كل هدف إصدار لوحده ورجوع لوحده. هدف 2 فشل وهدف 1 نجح: هدف 1 يفضل على الجديد (مكتوب)، والـ job اللي فشل `failed`، و`max_parallel: 1` بيوقف الباقي. مفيش رجوع عابر للأهداف في M6 ([سؤال 7](#7-مجموعة-سيرفرات)).
- **الهدف اتمسح أو اتغيّر والـ job مستني:** `TARGET_NOT_FOUND` أو `TARGET_CHANGED` من غير ما المفتاح يتفك.
- **الـ host key اتغيّر بعد الإدراج:** نفس الشيء، والإضافة بتفشل `HOST_KEY_MISMATCH` برضه.
- **السر `KeySecretName` اتمسح أو اتغيّر:** `TARGET_KEY_NOT_FOUND` في `Start`. التغيير بين الموافقة والبدء: القيمة الجديدة هي اللي بتوصل (زي M5: الموافقة على الكود مش على القيم).
- **الـ artifact انتهى (`ExpiresAt`) وهو مستني موافقة:** `ARTIFACT_DOWNLOAD_FAILED`. الاحتفاظ الافتراضي 30 يوم، والموافقة 72 ساعة افتراضيًا.
- **رفع نفس الـ digest بـ ref تاني:** run تاني (الـ ref جزء من الهوية) بنفس الـ blob.
- **رفع على ref مش في `on.artifact.refs`:** بيتخزّن ومفيش run (`NO_ARTIFACT_TRIGGER`).
- **توكن الاستقبال اتسرّب:** الإلغاء فوري. اللي ممكن يعمله بس رفع بـ ref من الـ patterns، وبيعدّي البوابة والموافقة. وارفع artifact مش هيتنشر لو البيئة محتاجة موافقة والمراجع رفض.
- **الأرشيف المرفوع كبير ومضغوط (zip bomb):** حدود الحجم بعد الفك والعدد والعمق بتتطبّق على الستريم، و`ARCHIVE_UNSAFE`.
- **IPv6 للهدف:** مدعوم لو المشغّل سمح. العنوان IPv6 literal بيتوصّل في `exact_endpoints` زي IPv4.
- **سيرفر الهدف في شبكة خاصة:** لازم `ALLOW_PRIVATE_CIDRS` على الـ runner، وجلب الـ host key من السيرفر مش هيشتغل (`TARGET_HOST_BLOCKED`)، فاللصق. ([سؤال 8](#8-أول-تثبيت-لـ-host-key-وهدف-في-شبكة-خاصة).)
- **runner قديم (M5):** job فيه هدف بيستنى `NO_MATCHING_RUNNER` ظاهر (feature `deploy_targets`).
- **الرجوع وإصدار الدليل اتنضّف:** `mode: redeploy` من الـ artifact المثبّت، وغير كده `ROLLBACK_RELEASE_GONE` والواجهة بتقول «انشر artifact قديم تاني».
- **Compose: خدمة من غير healthcheck:** `--wait` بيعتبرها شغّالة لما تقوم، والفحص الخارجي هو الحماية. الوثائق بتنصح بـ healthcheck.
- **Compose: صورة اتمسحت من registry:** `pull` بيفشل قبل أي تغيير على الخدمات الشغّالة (`pull` الأول)، `failed` من غير رجوع.
- **خرج السيرفر البعيد فيه سر:** بيتـ mask، وبنقص آخر 4 KiB.

## تقسيم الـ PRs
كل PR بيعدّي `make check` وCI أخضر، وفيه اختبارات سلبية لكل مسار أمني، وبيحدّث `Plan/Progress/` في يومه، وبيكتب الـ ADR بتاع قراراته في أوله، وعليه مراجعة أمنية مستقلة قبل الـ merge. وبيتفتح كـ draft PR من أول commit.

### M6.1: الأهداف: النموذج والـ API وتثبيت الـ host key
- **الحجم:** M. **الاعتماد:** M5 كله (خلص ما عدا M5.9ب).
- **المحتوى:**
  - `Modules/Environments`: الـ migrations (`"DeployTargets"` و`"AllowExtraDeployEgress"`)، والـ queries، والـ service والـ controller وملفات جديدة (`Environments.Targets*.go`) تحت 500 سطر، والـ routes في `openapi.yaml` (tag `targets`)، والإجراء `deployment.rollback` في `Core/Policy`، وصفوف الـ route-table.
  - الـ host key: الجلب فوق `netguard` والصق والتأكيد بالبصمة (نفس كود `Sources` المشترك، بيتنقل لـ `internal/sshhostkey` لو لسه)، والتوليد (`crypto/ed25519` وصيغة OpenSSH بـ `x/crypto/ssh`) وكتابة السر في البيئة عبر `Core/SecretStore`.
  - الـ audit، والتحقق (المسار، والعنوان، والـ egress rules، والتفرّد).
  - ADR للقرارات 1 لـ 3.
- **قبول:**
  - هدف بحقل غلط أو مسار ممنوع أو كتابة من Developer: مرفوض. بيئة بتتمسح: أهدافها وأسرارها معاها.
  - جلب مفتاح من sshd الاختبار، ولصقه، وتأكيده ببصمة غلط: `HOST_KEY_FINGERPRINT_MISMATCH`. تغيير المفتاح: الجديد pending والقديم لسه شغّال.
  - host خاص أو metadata: `TARGET_HOST_BLOCKED`.
  - المفتاح الخاص مابيظهرش في أي رد (اختبار انعكاس) ولا audit.
- **التحقق:** `go test -race -count=1 ./apps/server/internal/Modules/Environments/... ./apps/server/internal/Core/Policy/...` و`make gen && git diff --exit-code` و`make hooks-test` و`make docs-check` و`make check`.

### M6.2: التوصيل للـ runner: `StepTarget` والـ egress والتقارير
- **الحجم:** L. **الاعتماد:** M6.1.
- **المحتوى:**
  - `Core/Deployments.Targets`، وفحوص الإدراج الجديدة في البوابة، و`exact_endpoints` في `jobspec` والـ runner (`Executor/Egress.go`)، وفحص `Start` للهدف، والمفتاح كسر محجوز `as_file`، والـ `StepTarget`، و`DHOW_TARGET_*`.
  - الـ manifest: `Mutating` و`Inputs.<n>.Kind`، وتوصيل الأرشيف ملف للإضافة (`ArtifactInput` نوع جديد، والـ runner بيحطه في `/run/dhow/inputs/` للإضافة بس).
  - `ReleaseService.Report` ومعاه الـ agent يقرا `DHOW_DEPLOY_REPORT`، و`"DeploymentReleases"`، وحالة `running`، والـ reconciler (`unknown`).
  - `internal/archive` بيتطلّع من M4.9 (pack/unpack/validate) ويستخدمه الـ runner.
  - ADR للقرارات 4 لـ 7.
- **قبول:**
  - المفتاح: الإضافة بتلاقيه، وخطوة `run` وإضافة تانية في نفس الـ job لأ (ملف و`/proc` و`env`)، وفي الـ masker.
  - egress: الـ job اتصل بنقطة الهدف وبـ host الفحص بس، وأي اتصال تاني (حتى لـ host من `network.egress` اللي YAML كاتبه) رُفض، وIP عام بيشتغل وIP خاص من غير نطاق المشغّل مرفوض.
  - `HOST_KEY`/الهدف اتغيّر بين الإدراج والبدء: `TARGET_CHANGED` والمفتاح مااتفكش.
  - تقرير بتزوير (lease تاني، أو fencing قديم، أو حقول كبيرة): مرفوض.
  - قتل الـ runner وإصدار `running`: `unknown`.
- **التحقق:** `go test -race -count=1 ./apps/server/internal/Modules/... ./apps/runner/... ./internal/jobspec/... ./internal/archive/...` و`make runner-isolation` و`buf breaking` و`make gen && git diff --exit-code` و`make check`.

### M6.3: محرك الإصدارات وإضافة SSH والـ E2E على sshd
- **الحجم:** L. **الاعتماد:** M6.2.
- **المحتوى:**
  - `internal/deployrelease` (الاتصال والرفع والقفل والتبديل والصحة والرجوع والتقارير) و`apps/plugins/deploy-ssh` (الـ binary والـ Dockerfile والـ manifest) وبناؤه وتوقيعه في `release.yml`.
  - `deploy/e2e-targets/sshd` (صورة Alpine بـ `openssh 10.0_p1-r11` وservice بسيط يخدم `current/`) و`make deploy-e2e`.
  - `docs/guides/deploy-ssh.md`، وCLAUDE.md (الهيكل).
  - ADR للقرارات 8 لـ 11.
- **قبول (جزء من شرط الخلاص):**
  - نشر v1 صحي، ثم v2 صحي: `current` v2، وv1 لسه موجود.
  - نشر v3 `healthz` بيفشل: رجوع تلقائي لـ v2 (الـ symlink والخدمة بترد بـ v2)، والـ job `DEPLOY_ROLLED_BACK`، وصف الإصدار `rolled_back` بنتيجة الفحص.
  - `keep_releases: 3` بعد 6 نشرات: 3 إصدارات بالظبط، ومنهم `current` و`previous`.
  - host key متبدّل على sshd: `HOST_KEY_MISMATCH` ومفيش ملف اتكتب.
  - أرشيف بـ `../` وsymlink خارج: `ARTIFACT_UNSAFE` ومفيش تغيير.
  - لا سر في أي log أو DTO أو audit.
- **التحقق:** `go test -race -count=1 ./internal/deployrelease/... ./apps/plugins/deploy-ssh/...` و`make deploy-e2e` و`go test -run '^$' -fuzz FuzzRemoteCommand -fuzztime 60s ./internal/deployrelease` و`make check`.

### M6.4: تقوية SSH: الـ hooks والإلغاء والانقطاع والرجوع اليدوي وفحص الهدف
- **الحجم:** M. **الاعتماد:** M6.3.
- **المحتوى:**
  - الـ hooks (حسب [سؤال 6](#6-الـ-hooks)) بمهلة وتنضيف، و`SIGTERM` (رجوع لو بين التبديل والنجاح)، واكتشاف `state.json` بايظ في النشر الجاي.
  - التشغيلات الاصطناعية `rollback` و`check` في `Pipelines` وروابطها في API الهدف، و`mode: redeploy`.
  - ADR للقرارات 12 و13.
- **قبول:**
  - hook فشل: رجوع. `ReloadCommand` فشل: رجوع. إلغاء الـ run بعد التبديل: رجوع، والـ job `canceled`.
  - قتل (`SIGKILL`) الـ runner بعد التبديل: `unknown`، ثم «افحص الهدف» أو نشر جديد بيصلّح ويكتب المنشور الصح.
  - رجوع يدوي لإصدار موجود وآخر اتنضّف (redeploy من artifact مثبّت)، وبالموافقة حسب [سؤال 3](#3-الرجوع-اليدوي).
  - فحص الهدف: مفتاح غلط ومسار مش موجود ومساحة ناقصة بيرجّعوا سبب واضح.
- **التحقق:** نفس M6.3 + `make runner-chaos` (سيناريو قتل وقت التبديل) و`make check`.

### M6.5: إضافة Docker Compose
- **الحجم:** M. **الاعتماد:** M6.3.
- **المحتوى:** `apps/plugins/deploy-compose` فوق نفس المحرك، وسياسة الملف الآمنة، والـ override بالـ digest، و`pull` ثم `up --wait`، والرجوع، و`deploy/e2e-targets/compose-host` (dind + openssh + registry محلي) و`docs/guides/deploy-compose.md`، وADR للقرار 14.
- **قبول (جزء من شرط الخلاص):**
  - نشر خدمة بـ digest صحي، ثم تحديث صحي، ثم تحديث بـ healthcheck فاشل: رجوع تلقائي للصور السابقة، والحاويات القديمة شغّالة، والـ job `DEPLOY_ROLLED_BACK`.
  - خدمة بـ tag بس: `COMPOSE_IMAGE_NOT_PINNED`. `privileged` وbind على `/` و`docker.sock`: `COMPOSE_UNSAFE_*` وقبل ما نلمس السيرفر.
  - صورة مش موجودة في registry: `pull` يفشل والخدمات القديمة ماتتغيّرتش.
- **التحقق:** `make deploy-e2e` (الجزء compose) و`go test -race -count=1 ./apps/plugins/deploy-compose/...` و`go test -run '^$' -fuzz FuzzComposePolicy -fuzztime 60s ./apps/plugins/deploy-compose` و`make check`.

### M6.6: استقبال الـ artifacts: الموديول والـ spec
- **الحجم:** L. **الاعتماد:** M4.8 وM4.9 وM5.6 (لا يعتمد على M6.3، يمشي بالتوازي معاه بعد M6.2).
- **المحتوى:**
  - `Modules/Ingest`: التوكنات والـ API والفحص والتخزين على `Core/Blob`، و`policy.Ingest`، وصفوف الـ route-table، والـ rate limit، والـ audit.
  - `pipelinespec`: `on.artifact` و`jobs.<id>.ingested`، والـ JSON Schema، و`dhow lint`. `Pipelines`: الـ trigger والتعريف من الفرع الافتراضي و`"RunIngestedArtifacts"` والـ `ArtifactInput`. التثبيت `PinnedUntil` وتعديل الـ sweeper.
  - أدلة الـ reverse proxy و`proxy-check` للمسار. `docs/guides/ingest.md`.
  - ADR للقرارات 15 لـ 17.
- **قبول:**
  - رفع صح: digest مطابق، والـ run اتعمل، والـ job نزّل الـ artifact بنفس الـ digest.
  - digest غلط، أرشيف `../`، حجم أكبر من الحد، توكن ملغي أو منتهي، ref بره الـ patterns، تكرار: كلها مرفوضة بالكود الصح ومن غير blob متبقي.
  - `ingested:` بدون `on.artifact`: خطأ validation. artifact بعد الاحتفاظ لكن مثبّت لإصدار: لسه موجود.
  - لا توكن ولا محتوى في log أو audit.
- **التحقق:** `go test -race -count=1 ./apps/server/internal/Modules/Ingest/... ./apps/server/internal/Modules/Pipelines/... ./internal/pipelinespec/...` و`go test -run '^$' -fuzz FuzzIngestHeaders -fuzztime 60s ./apps/server/internal/Modules/Ingest` و`make gen && git diff --exit-code` و`make check`.

### M6.7: `dhow artifact push` وGitHub Action
- **الحجم:** S. **الاعتماد:** M6.6.
- **المحتوى:** الأمر في `apps/cli`، و`apps/github-action/` (`action.yml` composite وسكربت التحقق من SHA)، وmirror في `release.yml` لـ `Khaymah/dhow-action` مع tag، وjob `ingest-e2e` في الـ CI بيشغّل الـ Action محليًا (`uses: ./apps/github-action`) على سيرفر dev، ووثائق الـ Action.
- **قبول:** الـ Action رفعت، والـ digest في مخرجاتها هو اللي اتسجّل، والـ run اتعمل. نسخة CLI بـ SHA غلط: الـ Action بتفشل قبل التشغيل. التوكن مش ظاهر في log الـ workflow ولا في argv.
- **التحقق:** `go test -race -count=1 ./apps/cli/...` و`actionlint` و`make check`.

### M6.8: الواجهة (حد أدنى)
- **الحجم:** M، وبيتقسم على PRين زي M5.7: **(أ)** الأهداف وسجل النشر والرجوع (بعد M6.4)، **(ب)** توكنات الاستقبال وقايمة الملفات المستقبلة (بعد M6.6).
- **المحتوى:** حسب [الواجهة](#الواجهة-حد-أدنى-المنتج-الكامل-في-m7). اختبارات الوحدة ومتصفح (ADR-0087) لتبويب الأهداف وتأكيد البصمة، وسجل فيه رجوع تلقائي، وتوكن الاستقبال يظهر مرة.
- **قبول:** `pnpm lint` و`typecheck` و`test` و`build` و`size` (ميزانية JS 180 KB وCSS 30 KB)، ومفيش `<style>` اتحقن، ولا سر في state أو URL.
- **التحقق:** `pnpm lint && pnpm typecheck && pnpm test && pnpm build && pnpm size` و`pnpm e2e` لو الـ PR بيلمس المسارات.

### M6.9: التقوية وإثبات شرط الخلاص
- **الحجم:** M. **الاعتماد:** كل اللي فات.
- **المحتوى:**
  - **fuzz targets** في `FUZZ_TARGETS` و`fuzz-nightly.yml`: `FuzzRemoteCommand` و`FuzzDeployPath` و`FuzzReleaseID` و`FuzzComposePolicy` و`FuzzIngestHeaders` و`FuzzIngestArchive` و`FuzzDeployReport`.
  - **`deploy/m6-demo/run.sh`** فوق M5 demo (تحت).
  - **فحص التسريب** بيتوسّع: المفتاح الخاص للهدف، وسطر الـ known_hosts غير المنشور، وتوكنات الاستقبال، في كل log وaudit وDTO وأحداث وكل `pg_dump`.
  - **مراجعة التهديدات:** `docs/security-model.md` §14 بالنسختين (الجدول فوق بالاختبار بالمسار، والمخاطر المقبولة)، وإضافات لقايمة المراجعة الخارجية (تثبيت الـ host key، والرجوع، وتوكنات الاستقبال).
  - **مراجعة أمنية مستقلة** وإصلاح الـ Critical والـ High. `docs/roadmap.md` بالنسختين: M6 خلص.
- **قبول (شرط خلاص M6):**
  - `deploy/m6-demo/run.sh` عدّى على `ubuntu-26.04` في `demo.yml` وعلى VM نضيفة بـ Debian 13 وUbuntu 26.04: كل هدف نشر وفشل في الفحص ورجع.
  - فحص التسريب لقى الحالات المزروعة ومالقاش أي حاجة غيرها.
  - مفيش finding مفتوح Critical أو High.
- **التحقق:** `make check` و`make runner-isolation` و`make runner-chaos` و`make upgrade-test` و`make deploy-e2e` و`deploy/m6-demo/run.sh`.

### الترتيب
```
M5.9ب ─► M6.1 ─► M6.2 ─┬─► M6.3 ─┬─► M6.4 ─► M6.8 (أ) ─┐
                       │         └─► M6.5              │
                       └─► M6.6 ─► M6.7 ──► M6.8 (ب) ───┴─► M6.9
```
- M6.6 بيبدأ بعد M6.2 (محتاج `ArtifactInput` من نوع الملف) وبيمشي بالتوازي مع M6.3.
- M6.5 بعد M6.3 (نفس المحرك)، وبالتوازي مع M6.4.
- M6.8 (أ) بعد M6.4، و(ب) بعد M6.6.

### الترتيب مع الشغل الباقي
- **M5.9ب قبل M6.1** لأن `deploy/m6-demo` بيمدّ `m5-demo/lib.sh`.
- **U3:** بيزيد حالة «binary M5 على قاعدة M6» (M6 كله expand، فالـ binary القديم بيقرا)، وjob فيه هدف على binary M5 بيقفل مش بيفتح (`TARGET_NOT_FOUND`).
- **M7:** بياخد الأهداف والسجل كما هي ويبني عليها (إشعارات، وعرض السجل، ولوحات النشر).
- **M9:** المراجعة الخارجية بتتوسّع بقايمة M6 (الـ host key، والرجوع، والاستقبال).
- **M10 (Deploy Center importer):** ADR-0071 بيعمل مفاتيح SSH جديدة، وM6 فيه التوليد، فالـ importer بينادي نفس الـ API.

## الاختبارات: إيه بيشتغل فين
| الاختبار | كل PR | ليلي أو label | قبل كل release |
|---|---|---|---|
| وحدات `Environments` (الأهداف، والبوابة، والرجوع)، و`Ingest`، و`internal/deployrelease` | ✅ | — | — |
| route-table للـ routes الجديدة لكل دور | ✅ | — | — |
| الـ runner: المفتاح للخطوة دي بس، والـ egress الدقيق، والـ probes على Podman rootless (`ubuntu-26.04`) | ✅ لو الـ PR بيلمس `apps/runner` أو `packages/proto` | ✅ | ✅ VM بـ Debian 13 وUbuntu 26.04 |
| `make deploy-e2e`: SSH على sshd حقيقي (نشر، وفحص فاشل ورجوع، وتنضيف، وhost key متبدّل، وأرشيف عدائي) | ✅ لو الـ PR بيلمس الأهداف أو الإضافات | ✅ | ✅ |
| `make deploy-e2e`: Compose على dind + registry (نشر بـ digest، وفحص فاشل ورجوع، وسياسة، وصورة ناقصة) | ✅ لو الـ PR بيلمس Compose | ✅ | ✅ |
| الاستقبال: `curl` و`dhow artifact push` (رفع صح، وكل الرفض) | ✅ لو الـ PR بيلمس `Ingest` | — | ✅ |
| `ingest-e2e`: الـ Action الحقيقية على سيرفر dev | ✅ لو الـ PR بيلمس `apps/github-action` أو الـ CLI | ✅ | ✅ |
| `make upgrade-test` (binary M5 على قاعدة M6) | ✅ لو الـ PR بيلمس migrations | ✅ | ✅ |
| Playwright (الأهداف، وتأكيد البصمة، والرجوع، والتوكن) | ✅ لو الـ PR بيلمس `apps/web` | — | ✅ |
| `deploy/m6-demo/run.sh` بفحص التسريب | ✅ لو الـ PR بيلمس الكود | ✅ ليلي في `demo.yml` | ✅ |
| fuzz طويل (الأوامر البعيدة، والمسارات، والسياسة، وهيدرز الاستقبال، والأرشيف) | — | ✅ ليلي (30 دقيقة لكل target) | — |
| الفوضى: قتل الـ runner وقت التبديل ووقت الفحص، وقتل السيرفر وقت التقرير، وsshd بيوقع في النص | — | ✅ ليلي وعلى label `chaos` | ✅ |

### `deploy/m6-demo/run.sh`
فوق M5 demo (Forgejo وPostgres والسيرفر وrunner بـ Podman rootless في Docker)، ومعاهم `sshd-target` و`compose-host` و`registry`. السيناريو من أول لآخر:
1. بيئة `staging` فيها هدفين (`ssh-1` و`compose-1`)، ومفتاح اتولّد وhost key اتجاب واتأكدت بصمته بالـ API.
2. pipeline بنشر SSH: build ← artifact ← deploy: v1 صحي، v2 صحي (v1 محفوظ)، v3 بـ `healthz` فاشل: رجوع تلقائي لـ v2. الاتنين بالـ digest، والسجل بيقول الإصدار والسابق ونتيجة الفحص.
3. نفس الحاجة على Compose: صورتين صحيتين، وتالتة بـ healthcheck فاشل: رجوع تلقائي.
4. استقبال: `dhow artifact push` من «CI» (بتوكن محدود) ← run ← نشر بنفس الـ digest ← رفض رفع على ref بره الـ patterns ← إلغاء التوكن.
5. رجوع يدوي لـ v1 بخطوة تانية، وفحص الهدف.
6. تغيير host key على sshd: النشر بيفشل `HOST_KEY_MISMATCH` ومفيش تغيير.
7. فحص التسريب على كل شيء (المفتاح الخاص، والتوكنات، وأسرار البيئة).

## المخاطر
| الخطر | التخفيف |
|---|---|
| الإضافة الرسمية معاها مفتاح الإنتاج، وسلسلة التوريد ليها | build موقّع بـ cosign وprovenance وبالـ digest في الملف، وSBOM، وفحص الاعتماديات في الـ CI، وصورة صغيرة |
| الـ shell البعيد بيفتح باب حقن | قوالب ثابتة وregex وfuzz، والـ hooks للأدمن بس، ومكتوب كاستثناء في ADR |
| فحص الصحة بيعدّي للإصدار القديم بالغلط | `stable` والوثائق بتنصح برقم الإصدار في الرد، ومكتوب كخطر مقبول |
| أكتر من سيرفر في «fleet» ممكن يتسيب نصه جديد ونصه قديم | matrix بـ `max_parallel: 1` و`fail-fast` والوثائق صريحة، والمجموعة الحقيقية بعد أول نسخة ([سؤال 7](#7-مجموعة-سيرفرات)) |
| Compose = root على السيرفر | سياسة افتراضية، ووثائق rootless، وبوابة البيئة والموافقة |
| الـ E2E بيحتاج Docker وdind privileged في الـ CI | job لوحده على label ومسارات، في حاويات اختبار بس، وmakes منفصلة |
| الإضافة بتتغير وتكسر نشر المستخدمين | الـ digest مثبّت في الملف، والإصدارات القديمة بتفضل منشورة، و`buf breaking` والـ manifest بـ `Version` |
| الـ SFTP/الرفع بطيء على أرشيف كبير | stream من غير قرص، وقياس بـ `make bench` على artifact 500 MiB قبل الدمج، والحد في الوثائق |
| رفع artifact من CI خارج بيحمّل السيرفر | حدود وrate limit وتزامن، ووثائق الـ proxy |
| أرقام الـ migrations بتتخبط | الرقم وقت الـ merge، و`Compat_test.go` |
| ميزانية الـ JS | صفحات lazy، ومن غير مكتبات جديدة |

## الاعتماديات المتوقعة (بنسخ مثبّتة)
اتأكدت يوم 2026-11-05 على proxy.golang.org، وبتتراجع تاني (وبـ OSV) في أول المرحلة اللي بتضيفها.

| الاعتمادية | النسخة | تاريخها | ليه |
|---|---|---|---|
| Go standard library (`crypto/ed25519` و`crypto/sha256` و`net/http` و`archive/tar` و`compress/gzip`) | Go 1.27.3 (`go.mod`) | — | الفحص، والمفاتيح، والأرشيف، من غير مكتبات |
| `golang.org/x/crypto` (`ssh`) | v0.58.0 (موجود) | 2026-10-09 | عميل SSH بـ `FixedHostKey`، والـ handshake بتاع جلب المفتاح (موجود من M4.4) |
| `github.com/pkg/sftp` | v1.13.12 | 2026-10-23 | رفع الملفات و`PosixRename` و`Symlink`، في binary الإضافة بس. **لو فحص الأمان أو الاعتماديات (أقل من أسبوعين عمرها) اعترض:** v1.13.10 |
| `go.yaml.in/yaml/v3` | الموجود | — | قراية Compose للسياسة |
| صورة sshd الاختبار | Alpine 3.22 بالـ digest + `openssh 10.0_p1-r11` (زي #102) | — | E2E |
| صورة `docker:dind` وregistry | بالـ digest وقت التنفيذ | — | E2E لـ Compose |
| Forgejo (CI) | الموجودة من M4 | — | الـ demo |

**مش هنضيف:**
- مكتبة JWT/JOSE (التحقق من OIDC الخاص بـ GitHub متأجّل).
- SDK بتاع Docker (`docker/docker`): الأوامر `docker compose` على السيرفر عبر SSH، فمفيش client لـ Docker API.
- مكتبة glob: `internal/pathglob`.
- مكتبة rsync أو `golang.org/x/crypto/ssh/knownhosts`: بنثبّت مفتاح واحد بالتأكيد، فمش محتاجينها.

## أسئلة لـ Sabry
8 قرارات مفتوحة. كل واحد ليه الاختيارات مع trade-offs كل اختيار، والتوصية. الخطة اتكتبت على التوصية.

### 1. فين النشر بيشتغل
الكود اللي بيتصل بسيرفر الإنتاج بمفتاحه لازم يكون محصور، ومراقب، وبيتلغي.
- **أ. إضافة في الـ runner sandbox (الخطة):** نفس عزل أي job (حدود موارد، وegress للهدف بس، وmasker، وإلغاء)، والسيرفر مابيتصلش بحد، والمفتاح بيوصل لـ sandbox لفترة الخطوة. العيب: الـ runner لازم يوصل لشبكة الهدف، وصورة الإضافة جزء من سلسلة التوريد.
- **ب. النشر من السيرفر نفسه:** مفيش runner في الطريق ومفيش sandbox، بس المفتاح بيتفك في الـ control plane، وكود SSH بيشتغل بصلاحيات السيرفر (أخطر مكان)، وبيخالف ADR-0007 (كود المستخدم والنشر في عزل).
- **ج. agent صغير على سيرفر الهدف** بيستلم أمر نشر موقّع: مفيش مفتاح SSH أصلًا ولا فتح SSH، والرجوع محلي (حتى لو الشبكة وقعت في نص النشر). العيب: تنصيب وتحديث وأمان برنامج على كل سيرفر، وبيبعد عن «SSH/VPS» اللي ADR-0011 وعد بيه، ومحتاج M6 أطول.
- **التوصية: (أ).** وـ(ج) ممكن بعد أول نسخة كنوع هدف تالت (`agent`) على نفس المحرك، خصوصًا للسيرفرات اللي مش بتفتح SSH.

### 2. الرجوع التلقائي وإيه اللي يشغّله
- **أ. فحص صحة HTTP فاشل أو reload فاشل أو hook فاشل، جوه خطوة النشر (الخطة):** أساسي ومحدد، والمهلة 10 دقايق كحد أقصى. مفيش مراقبة بعد ما الـ job يخلص.
- **ب. أ + نافذة مراقبة بعد النجاح** (`observe_for: 10m`): الـ job بيفضل مفتوح بيفحص، ولو فشل بيرجع. أأمن، بس بيحجز الـ runner والقفل البيئة أطول، ومعناه الرجوع بعد ما الـ job كان «نجح».
- **ج. أ + رجوع من خارج:** أداة المراقبة بتاعتك بتستدعي API الرجوع (توكن بـ `deployment.rollback`): مرن، بس محتاج scope كتابة بتوكن (قرار Sabry 6 في M5 كان ضده) فبيتأجّل.
- **التوصية: (أ)** بـ `health_check_stable` (3 نجاحات متتالية افتراضيًا) علشان نقلل إن الفحص يعدّي بالغلط. والمراقبة بعد النجاح (ب) بعد أول نسخة لو طلبها حد. ولو مفيش إصدار سابق (أول نشر)، الإصدار بيتساب والـ job بيفشل، ومانمسحش `current`.

### 3. الرجوع اليدوي
زرار «رجّع لإصدار سابق» وقت حادثة لازم يبقى سريع، وفي نفس الوقت مايبقاش باب بيعدّي الموافقات.
- **أ. بيعدّي على نفس البوابة زي أي نشر** (مراجع وخطوة تانية ومؤقت): أبسط وأأمن من ناحية المبدأ، بس وقت الحادثة المراجع ممكن مايبقاش موجود.
- **ب. بيتخطى المراجعين والمؤقت بس، ومحتاج Maintainer أو أعلى وخطوة تانية** (الخطة): لإصدار نجح على نفس الهدف قبل كده بس (كود اشتغل في الإنتاج بالفعل)، وبيفضل خاضع للإيقاف (`Paused`) وقفل البيئة، وبيتسجّل `Kind = rollback` في السجل والـ audit. وقاعدة الـ ref مابتتفحصش على الـ ref القديم (الكود ده اتنشر تحت قواعد وقتها).
- **ج. رجوع تلقائي بس، ومفيش يدوي:** أبسط، بس لو نشر نجح وطلع فيه مشكلة بعدين مفيش حل غير نشر جديد.
- **التوصية: (ب)**، على نفس فلسفة M5 في إن الإيقاف سريع (Maintainer) والاستكمال أبطأ. لو التخطّي ده مش مقبول فـ(أ).

### 4. توثيق الرفع من CI خارجي
- **أ. توكن استقبال محدود (الخطة):** مشروع واحد، patterns للـ refs، إجراء واحد (الرفع)، وانتهاء، وإلغاء فوري. بيشتغل مع أي CI (GitLab وJenkins وcurl) وفي التنصيبات المقفولة. العيب: سر طويل العمر في GitHub Secrets ممكن يتسرّب، والمنشأ «مُعلَن» مش «متحقَّق».
- **ب. توكن + تحقق من توكن GitHub OIDC اختياري:** بيدّي منشأ متحقق (المستودع والـ ref والـ workflow من GitHub نفسه) وبيستغنى عن السر في GitHub. العيب: السيرفر لازم يوصل لـ JWKS بتاع GitHub (egress)، وكود تحقق JWT/JWKS (من غير مكتبة)، وبيخص GitHub بس (GHES بعدين)، وأعمق مراجعة أمنية.
- **ج. OIDC بس:** بنلغي السر الطويل، بس مفيش GitLab ولا curl ولا تنصيب مقفول.
- **التوصية: (أ) في M6**، وـ(ب) بعد أول نسخة (العمود `"ProvenanceVerified"` جاهز له). لأن (أ) بيغطي كل CI، وبنفس فلسفة «OIDC بعد أول نسخة» (قرار Sabry 3 في M5).

### 5. Docker Compose عن بعد
- **أ. `docker compose` على السيرفر عبر SSH exec (الخطة):** مفيش Docker API على الشبكة ولا agent، والمفتاح SSH هو الوحيد، ونفس محرك SSH (host key والإصدارات والرجوع). العيب: الـ Docker CLI والـ Compose plugin لازم يكونوا على السيرفر، ومستخدم النشر في مجموعة `docker` (= root معنى).
- **ب. `DOCKER_HOST=ssh://` من جوه الإضافة:** مفيش حاجة ننصبها على السيرفر غير Docker نفسه، بس لازم docker CLI وssh client جوه الإضافة (صورة أكبر وسطح أكبر)، والـ host key وknown_hosts بيتعاملوا بـ OpenSSH مش بالكود بتاعنا (تعقيد في تثبيت المفتاح).
- **ج. agent على السيرفر:** [سؤال 1](#1-فين-النشر-بيشتغل).
- **التوصية: (أ)**، ومعاها سياسة الملف الآمنة، ووثيقة تنصح بـ rootless Docker لمستخدم النشر.

### 6. الـ hooks
- **أ. مفيش hooks في أول نسخة غير `ReloadCommand` من الهدف:** أبسط وأأمن، بس مشاريع محتاجة migrations أو warm-up قبل التبديل مش هتلاقي مكان.
- **ب. hooks معرّفة على الهدف من الأدمن (`PreSwitchCommand` و`PostSwitchCommand` و`ReloadCommand`)، وهوك من داخل الـ artifact مقفول (الخطة):** الـ commit مايقدرش يضيف أمر على سيرفر الإنتاج، والأدمن عارف بالظبط إيه بيتنفّذ. ومعاها اختيار `AllowArtifactHooks` على الهدف (افتراضيًا لأ) يشغّل سكربت ثابت المسار `.dhow/hooks/pre-switch` من الأرشيف.
- **ج. أوامر حرة في `with:` (زي `pre_switch: "..."`):** أمرن، بس أي commit يشغّل أمر عشوائي على الإنتاج (بعد البوابة طبعًا) ومعناه تشغيل نص جاي من المستخدم في shell السيرفر.
- **التوصية: (ب)** من غير `AllowArtifactHooks` في أول نسخة (يتضاف لو اتطلب)، يعني hooks من الهدف بس.

### 7. مجموعة سيرفرات
spec الوثائق فيه `target: prod-fleet`.
- **أ. هدف = سيرفر واحد، والمجموعة matrix على أهداف بـ `max_parallel: 1` (الخطة):** أبسط بكتير، والرجوع لكل هدف لوحده، والـ fail-fast بيوقف الباقي. العيب: لو الهدف التاني فشل، الأول يفضل على الجديد (نص المجموعة جديد ونصها قديم)، والمستخدم لازم يعرف.
- **ب. هدف بقايمة سيرفرات (`Hosts`) ونشر متدرّج (rolling)** مع رجوع كل السيرفرات اللي اتبدّلت لو واحد فشل: تجربة أقرب لـ fleet حقيقي، بس تعقيد كبير (حالات جزئية، والرجوع الجماعي نفسه ممكن يفشل، وقفل لكل سيرفر) ووقت M6 أطول.
- **التوصية: (أ)** في أول نسخة، ومثال الـ spec يتعدّل لـ matrix. (ب) بعد أول نسخة بـ ADR مخصوص.

### 8. أول تثبيت لـ host key وهدف في شبكة خاصة
السيرفر بيجيب المفتاح من برّه بـ `netguard` (public فقط)، فهدف على IP خاص (شبكة المكتب، VPN) مايتجابش.
- **أ. جلب من السيرفر للعناوين العامة + لصق المفتاح (من `ssh-keyscan` أو `.pub` من السيرفر نفسه) لأي عنوان، والتأكيد بالبصمة دايمًا (الخطة):** اللصق أأمن من الجلب (مفيش TOFU)، ومفتوح للشبكات الخاصة. العيب: خطوة يدوية للشبكات الخاصة، والـ runner لازم عنده `ALLOW_PRIVATE_CIDRS` للنطاق ده.
- **ب. الجلب من الـ runner** (تشغيل اصطناعي بوضع `scan-host-key`): بيشتغل للشبكات الخاصة من غير يدوي، بس محتاج runner أونلاين ومساره في البوابة، وأول اتصال بدون مصادقة من جوه sandbox (TOFU بنفس درجة السيرفر).
- **ج. لصق بس، ومن غير جلب:** أأمن وأبسط كود، بس أصعب على المستخدم العادي.
- **التوصية: (أ)**، و(ب) بعد أول نسخة لو الشبكات الخاصة طلعت حالة شائعة.

## قرارات تانية اتاخدت بالتوصية (Sabry يقدر يرجع فيها)
1. **من يدير الأهداف:** Admin وOwner بس، بخطوة تانية (زي البيئة).
2. **مفتاح الهدف:** داو بيولّده (ed25519)، واللصق متاح. والخاص عمره ما بيظهر.
3. **عدد الإصدارات:** الافتراضي 5 والحد الأدنى 2.
4. **مدة فحص الصحة:** سقف 10 دقايق، و3 نجاحات متتالية افتراضيًا.
5. **المشروع بيحط `strategy`:** `atomic-symlink` الوحيد، وغيره بيترفض صريح.
6. **ملف الـ artifact المقبول عند الاستقبال:** tar.gz فقط.
7. **مدة توكن الاستقبال:** 90 يوم افتراضيًا و365 أقصى.
8. **اسم الـ Action:** `Khaymah/dhow-action`، ومصدرها في الـ monorepo.
9. **أسماء الـ spec الجديدة:** `on.artifact` و`jobs.<id>.ingested` (تتثبّت في ADR M6.6).
