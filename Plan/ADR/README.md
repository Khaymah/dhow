# سجلّ قرارات المعمارية (Architecture Decision Records)

| ADR | العنوان |
|---|---|
| [0001](0001-product-positioning.md) | تموضع المنتج: منصة CD ذاتية الاستضافة بتتكامل مع CI |
| [0002](0002-rewrite-with-v3-hotfix.md) | إعادة الكتابة من الصفر، مع إصلاح أمني عاجل لـ v3 بالتوازي |
| [0003](0003-go-for-server-and-runner.md) | Go للـ control plane والـ runner |
| [0004](0004-monorepo-layout.md) | Monorepo واحد |
| [0005](0005-api-rest-openapi-and-connect.md) | REST عام + OpenAPI؛ وConnect/gRPC للـ runners؛ وSSE للواجهة الحيّة |
| [0006](0006-postgres-default-redis-optional.md) | PostgreSQL افتراضيًا؛ وRedis اختياري |
| [0007](0007-job-isolation.md) | عزل الـ jobs: حاويات rootless افتراضيًا، وmicroVM اختياري |
| [0008](0008-pipeline-as-code-with-visual-editor.md) | Pipeline-as-code (`.dhow.yml`) مع محرّر مرئي |
| [0009](0009-container-image-plugins.md) | الـ Plugins عبارة عن خطوات (steps) على شكل container image |
| [0010](0010-build-or-ingest-artifacts.md) | البناء جوه Dhow أو استيراد الـ artifacts من CI خارجي |
| [0011](0011-deploy-targets.md) | أهداف النشر (deploy targets) في v1.0 |
| [0012](0012-git-providers.md) | مزوّدو Git عند الإطلاق |
| [0013](0013-authentication-methods.md) | طرق المصادقة |
| [0014](0014-secrets-management.md) | الأسرار: تشفير envelope مدمج؛ وVault/KMS ضمن Enterprise |
| [0015](0015-rbac-model.md) | RBAC: أدوار ثابتة + Teams؛ والأدوار المخصصة مدفوعة |
| [0016](0016-multi-org.md) | منظمات متعددة في التثبيت الواحد من اليوم الأول |
| [0017](0017-licensing-agpl-and-ee.md) | نواة AGPL-3.0 + مجلد `ee/` تجاري |
| [0018](0018-pricing-per-active-user.md) | الإصدارات المدفوعة بتتسعّر لكل مستخدم نشط |
| [0019](0019-no-community-limits.md) | مفيش حدود رقمية في Community |
| [0020](0020-managed-cloud-later.md) | control plane مُدار بعد استقرار الاستضافة الذاتية |
| [0021](0021-arabic-first-ui.md) | واجهة عربية أولًا |
| [0022](0022-navigation-model.md) | مساحة عمل واحدة + لوحات منزلقة (slide-over) + command palette |
| [0023](0023-ui-stack.md) | تقنيات الواجهة: React + Tailwind + shadcn/ui + Motion |
| [0024](0024-install-methods.md) | طرق التثبيت الرسمية |
| [0025](0025-notifications.md) | قنوات الإشعارات عند الإطلاق |
| [0026](0026-product-name.md) | اسم المنتج: Dhow |
| [0027](0027-public-repo-from-day-one.md) | مستودع عام من اليوم الأول |
| [0028](0028-naming-conventions-and-project-hooks.md) | قواعد التسمية والأسلوب، وفرضها بـ Claude Code hooks |
| [0029](0029-design-system-aurora-dusk.md) | الـ Design System على aurora-dark-ui بألوان الغروب، وغامق بس في الإصدار الأول |
| [0030](0030-server-modular-monolith.md) | هيكل السيرفر: Modular Monolith بتكييف Go |
| [0031](0031-web-structure-and-ui-components.md) | هيكل الواجهة بالـ Modules، وأسماء الـ hooks، وcomponents مكتوبة بإيدينا فوق Radix |
| [0032](0032-stacked-slides-and-themes.md) | الشرائح المتراكبة من اليمين، وثيمات النظام الأربعة، والثيم المخصص |
| [0033](0033-browser-sessions-invites-breached-passwords.md) | جلسات المتصفح بـ cookie في السيرفر، والدعوات برابط وSMTP اختياري، وفحص كلمات السر المسرّبة |
| [0034](0034-ui-direction-calm-and-organized.md) | اتجاه التصميم «هادي ومنظّم» |
| [0035](0035-identity-core-implementation.md) | تنفيذ نواة الهوية في M2.1: سلسلة الـ audit، والـ throttle، وإعلان الـ policy لكل route |
| [0036](0036-interactive-host-installer.md) | الـ installer التفاعلي على السيرفر (`dhow-server install`) |
| [0037](0037-mfa-implementation.md) | تنفيذ التحقق بخطوتين في M2.2: خطوة `mfa_pending`، وTOTP، وأكواد الطوارئ، ومفاتيح المرور |
| [0038](0038-orgs-teams-invites-implementation.md) | تنفيذ المؤسسات والفرق والدعوات في M2.3: مين يعمل مؤسسة، والدور الفعلي، وقواعد منح الأدوار، والدعوة برابط مرة واحدة |
| [0039](0039-oidc-api-tokens-preferences.md) | تنفيذ OIDC وAPI tokens والتفضيلات في M2.4: الإعداد من الـ env، وGitHub كمزوّد OAuth لوحده، والـ token اللي عمره ما يعدّي صاحبه، والتفضيلات في السيرفر |
| [0040](0040-identity-hardening.md) | تقوية الهوية في M2.5: rate limiter مختلط، والمسح الفعلي للجداول المؤقتة بس، وقايمة سماح للـ audit، والمراجعة الخارجية في M9 |
| [0041](0041-runner-identity-and-registration.md) | هوية الـ runner وتسجيله: مفاتيح Ed25519 متثبّتة، وتوكن قصير، وإثبات امتلاك عند الحاجات الحساسة |
| [0042](0042-runner-protocol-and-leasing.md) | بروتوكول الـ runner: Connect بـ RPCs unary بس، وleases بـ fencing token، وتسليم at-least-once بإنهاء idempotent |
| [0043](0043-job-sandbox-rootless-podman.md) | الـ sandbox: Podman rootless افتراضيًا، وagent هو PID 1، وحدود cgroup v2 بتفشل مقفولة، وgVisor اختياري |
| [0044](0044-job-network-egress-proxy.md) | شبكة الـ job: `none` افتراضيًا، والـ egress من خلال proxy في الـ runner بقايمة hosts مسموحة |
| [0045](0045-secrets-transport-and-masking.md) | توصيل الأسرار للـ job: HPKE لمفتاح الـ runner، والذاكرة بس، والـ masking في الـ runner قبل أي حاجة تخرج |
| [0046](0046-job-logs-storage-and-streaming.md) | الـ logs: chunks متسلسلة من غير فراغات في جدول Postgres مقسّم بالأيام، وSSE بيكمّل من آخر seq |
| [0047](0047-job-state-machine-and-reconciliation.md) | حالات الـ job والـ attempt، والإلغاء بالتدريج، والـ reconciliation بقيود في قاعدة البيانات |
| [0048](0048-runner-packaging-and-kubernetes.md) | تغليف الـ runner: binary وsystemd ووضع `runner` في الـ installer، وHelm chart واحد، وpod لكل job على Kubernetes |
| [0049](0049-web-code-splitting-and-message-catalogs.md) | تقسيم الواجهة لـ chunks بتتحمّل وقت الحاجة، وكتالوج الرسائل لجزئين |
| [0050](0050-runner-identity-implementation.md) | تنفيذ هوية الـ runner في M3.2: صيغة الرسائل الموقّعة، والـ gate، والـ throttle، وكشف النسخ |
| [0051](0051-jobs-and-leasing-implementation.md) | تنفيذ الـ jobs والـ leases في M3.3: الـ triggers، والـ claim، والرسائل الموقّعة، وقواعد الإنهاء، والـ reconciler |
| [0052](0052-job-logs-implementation.md) | تنفيذ الـ logs في M3.4: الجدول المقسّم، والـ Append بقفل الـ attempt، والـ SSE بقراية لكل مشترك، والصيانة تحت advisory lock |
| [0053](0053-runner-implementation.md) | تنفيذ الـ runner في M3.5: الطبقات، والهوية على القرص، والقفل الذاتي، والـ journal، والـ masking والـ logs، والـ driver الوهمي |
| [0054](0054-second-factor-policy-sessions-and-credential-alerts.md) | سياسة الخطوة التانية للمؤسسة، وحدود الجلسة حسب الدور، وإيميلات تنبيه تغيير بيانات الدخول (M2.6) |
| [0055](0055-podman-driver-and-agent-implementation.md) | تنفيذ الـ sandbox في M3.6: driver الـ Podman، والـ agent كـ PID 1، وقناة التحكم، والـ seccomp، والـ escape والـ limits suites |
| [0056](0056-sandbox-hardening-after-the-m3-6-review.md) | تقوية الـ sandbox بعد المراجعة الأمنية لـ M3.6: الخطوة بتخلص بكل الـ processes بتاعتها، وملفات الأسرار، وقفل وضع `directory`، والـ image بالـ ID، والـ vsock |
| [0057](0057-egress-proxy-implementation.md) | تنفيذ الـ egress proxy في M3.7: القايمة ASCII وpunycode بس، وطبقة `Egress`، والـ forwarder في الـ agent، والـ helper `connect`، ونطاقات المشغّل |
| [0058](0058-graded-kill-and-chaos-suite.md) | آخر مرحلة في القتل بالتدريج (`cgroup.kill`)، وsuite الفوضى على processes حقيقية بـ Podman وPostgres وregistry بـ TLS (M3.9) |
| [0059](0059-ui-direction-control-room.md) | اتجاه التصميم «غرفة التشغيل»: صفحات كاملة بروابط، وقايمة جانبية بمجموعات، ورئيسية بحالة النظام، وخط Plex بمقاسات عربي صريحة. بيعدّل ADR-0034 وADR-0032 |
| [0060](0060-host-size-check.md) | فحص حجم الجهاز: الحد الأدنى والموصى بيه من القياس، والتنصيب الجديد بيقف تحت الحد الأدنى، والترقية بتحذّر بس |
| [0061](0061-organization-overview-endpoint.md) | endpoint ملخّص المؤسسة: طلب واحد للرئيسية والعدّادات، وكل block بصلاحية قايمته و`null` للممنوع، و3 queries ثابتة بـ partial indexes |
| [0062](0062-simple-arabic-for-users.md) | العربي المبسّط في كل نص بيقراه المستخدم: فصحى بجمل قصيرة، لا عامية ولا فصحى تقيلة، وجدول تبديل، وhook بيرفض العلامات العامية في الكتالوجات |
| [0063](0063-instance-settings-in-the-database.md) | إعدادات السيرفر في قاعدة البيانات: الأساسيات في env، والباقي من «إعدادات السيرفر» لمالك السيرفر بخطوة تانية، وenv يكسب ويقفل، والأسرار مشفّرة وللكتابة بس. والجزء التاني: مزوّدي الدخول من اللوحة، وenv بيقفل القايمة كلها |
| [0064](0064-arabic-first-documentation.md) | الوثائق العامة بالعربي أولًا: `docs/` بالعربي المبسّط وهي المرجع، و`docs/en/` بنفس المسارات، وhook على الـ commit وفحص `make docs-check` بيمنعوا التأخّر |
| [0065](0065-runner-packaging-part-a.md) | تغليف الـ runner، الجزء أ: `rotate-keys` بمفاتيح بتتحفظ قبل ما السيرفر يعرفها والرجوع من أي وقعة، ووحدة systemd بتقوية متجرّبة مع Podman rootless (5.2 MEDIUM)، وأرشيفات الـ runner الموقّعة و`get.sh --component runner` |
| [0066](0066-runner-install-and-encrypted-identity.md) | تغليف الـ runner، الجزء ب: الـ engine المشترك في `internal/installer` بـ aliases من غير ما اختبارات السيرفر تتغيّر، و`dhow-runner install` و`doctor` و`uninstall`، والهوية مشفّرة بـ `systemd-creds` وبتتفك في الخدمة بـ `LoadCredentialEncrypted=`، و`AuthService.Revoke` الموقّع، و`doctor` مابيبدّلش جلسة الخدمة |
| [0067](0067-selinux-enforcing-not-supported-yet.md) | SELinux enforcing مش مدعوم دلوقتي: `container_t` ممنوع من `connectto` على control socket الـ runner، فـ `install` بيرفض قبل الخطوة 1 و`doctor` بيقول نفس الكلام، والـ VM test على `debian-13` بس، والقرار بيترجعله قبل ما M3 يقفل |
| [0068](0068-first-production-release-scope.md) | نطاق أول نسخة production: Debian وUbuntu بـ Podman، والنشر على SSH/VPS وDocker Compose، وGitHub وGitLab وGit عام، وCommunity بس؛ وM5 وM9 كاملين، وSELinux وKubernetes وgVisor وBitbucket وM8 بعد أول نسخة |
| [0069](0069-pipeline-spec-parser-and-validator.md) | الـ parser والـ validator بتوع `.dhow.yml` (M4.1): `${{ }}` ممنوع في `run:`، وYAML من غير anchors ولا aliases ولا merge keys ولا tags خاصة وبحدود، والـ validator بـ Go هو المصدر والـ JSON Schema للمحررات باختبار تطابق، و`schedule` و`artifact` بيترفضوا؛ وCEL بـ cel-go v0.30.0 بمتغيرات منقّطة وحد تكلفة ومهلة 10 ms، والتوسيع حتمي |
| [0070](0070-ports-in-core-and-project-permissions.md) | الـ ports في `Core` بـ interfaces بس وبتشتغل جوه transaction اللي بينادي (`Core/JobQueue` بتنفيذه في `Jobs`)، و`"Jobs"."OriginKind"` و`"OriginId"` و`"RequiredFeatures"` (job محتاج ميزة مابيروحش لـ runner قديم)، و`policy.Project` بدور على المشروع من `"RoleBindings"` بيرفع أو بيقلّل (ما عدا Owner وAdmin)، وscopes `project.read` و`project.manage`، وhook لاتجاه الاستيراد للـ ports |
| [0071](0071-deploy-center-importer-never-copies-secrets.md) | أداة النقل من Deploy Center بتنقل المشاريع وأوامر النشر وأسماء الأسرار بس، ومابتنقلش أي قيمة سر: كل سر «محتاج قيمة جديدة» والمشروع مايتنشرش قبلها، ومفاتيح SSH بتتعمل جديدة |
| [0072](0072-webhooks-and-git-credentials.md) | الـ webhooks وcredentials المزوّدين وGit عام (M4.4): `/webhooks/` بره `/api/v1` بـ `policy.Webhook` وmiddleware خاصة، والتوقيعات الأربعة على الـ body الخام بنفس `401`، والـ dedupe بالـ delivery id أو sha256 الـ body، و`"WebhookDeliveries"` مقسّم يوميًا 30 يوم، والـ deploy keys وأسرار الـ webhooks متشفّرة بـ label لكل نوع، والـ host key بيتثبّت بالتأكيد بس، و`internal/netguard` مشترك مع الـ runner |
| [0073](0073-disable-mptcp-in-all-binaries.md) | MPTCP مقفول في كل ملفات داو التنفيذية بسبب CVE-2026-98432: `godebug multipathtcp=0` في `go.mod` بيغطّي الـ listeners والـ dialers، واختبار وقت تشغيل وحارس، والـ `doctor` في السيرفر والـ runner بيحذّر لو `net.mptcp.enabled=1` من غير ما يحكم من رقم النواة ومن غير ما يغيّر الـ sysctl، والتحذير advisory مابيغيّرش رمز الخروج |
| [0074](0074-git-connection-oauth-for-setup-deploy-keys-for-access.md) | ربط Git: الـ OAuth للتجهيز بس والـ deploy keys للوصول. GitHub App بتوكنات قصيرة، وGitLab بـ OAuth2 بيعمل deploy key وwebhook للمشروع ومابيخزّنش توكن المستخدم، وتطبيق GitLab OAuth من «إعدادات السيرفر»، وGit عام بـ deploy key وwebhook موقّع |
| [0075](0075-multi-container-attempts-checkout-and-plugins.md) | الـ runner في M4.7: الـ attempt workspace tmpfs واحد ونطاق uid صريح واحد لكل الحاويات بالتتابع (الـ spike اختار الـ map الصريح على pod بـ `--userns=auto`)، والـ checkout في صورة `dhow-git` المتثبّتة والموقّعة بـ git بـ argv وتقوية M4.4، والإضافات بـ manifest في label `io.dhow.plugin.manifest` بيتحقق منه الـ runner قبل التشغيل، والـ outputs، والـ features من التسجيل والدخول الموقّع |
| [0076](0076-pipeline-runs-definitions-and-forks.md) | تنفيذ خطوات النشر (M4.8): الملف من API المزوّد أو من discovery job لـ Git عام بيسلّم الملف في السجل بـ sha256، والـ checkout المربوط بمفتاح المستودع وقت `Start` بس والـ ancestry الإجباري، والـ PR على sha الـ head، والـ forks بموافقة وتعريف من الـ base ومن غير أسرار ولا بيئات، وتثبيت الصور بالـ tag وقت البداية من خلال `netguard`، و`POST /jobs` بـ `404` من غير `DHOW_DEV`، ومرة واحدة بالأقفال في transaction واحدة، و`Core/RunEvents` و`Core/CommitStatus`، والدور على المشروع في routes الـ jobs |
| [0077](0077-upgrades-migrations-and-update-policy.md) | الترقيات: الافتراضي «إشعار + الأمني أوتوماتيك» جوه نافذة صيانة بنسخة احتياطية ورجوع، وfeed ثابت موقّع بـ TUF من غير instance id، وقاعدة expand/contract لكل migration جديد من النهارده (`-- dhow:phase` و`SchemaFloor`، والـ contract في minor أو major بس)، والسيرفر بيقرر والـ helper (root، الـ binary المتنصّب) بينفّذ وبيتحقق من تاني؛ ومن غير cosign في الـ updater، ومفاتيح offline ملفات مشفّرة على USBين، والـ feed على GitHub Pages بتاع نفس الريبو، وكله ببلاش |
| [0078](0078-artifacts-cache-and-blob-store.md) | الـ artifacts والـ cache (M4.9): رفع وتنزيل بـ HTTP على `/runner-api/blobs/{sha256}` مربوط بالـ lease والـ fencing ومنحة تنزيل HMAC للـ digest بالظبط، وblob store على القرص ورا `Core/Blob` (ملف مؤقت وتأكيد الـ SHA-256 و`fsync` و`rename`، والمسار من الـ digest بس)، والجمع والفك وhash الملفات في sandbox من صورة الـ checkout جوه user namespace الـ job، ومفتاح الـ cache يتقيّم في السيرفر و`hash_files()` يكمّلها الـ runner، والـ scope من السيرفر والـ fork مابيكتبش، وmark and sweep تحت advisory lock، واحتفاظ الـ logs لكل مؤسسة |
| [0079](0079-runner-in-the-docker-dev-stack.md) | الـ runner في حزمة التطوير بـ Docker: Podman rootless حقيقي في حاوية privileged و`cgroup: private` للتطوير بس، والـ entrypoint بيعمل اللي `Delegate=yes` بيعمله من غير أي flag بيقفل فحص، والوصول للسيرفر بـ forwarder على loopback جوه الحاوية فـ `--dev` مابيتوسّعش، والتسجيل بـ `dhow-dev-register` من stdin، وprofile `fake-runner` بـ volume منفصل، وحد 3 بدايات في 10 دقايق عشان كشف الهوية المنسوخة |
| [0080](0080-environments-are-made-by-admins-only.md) | البيئات (M5.3): بيعملها Admin بس بـ `environment.manage` والاسم الغلط في الملف `ENVIRONMENT_NOT_FOUND` (`failed`) من M5.6، والإجراءات `environment.manage` و`environment.pause` و`environment.bypass` و`deployment.approve` على المشروع، وMaintainer يوقف وAdmin يستكمل بـ `CurrentFactor`، والتعديل بـ `Version` و`CurrentFactor`، و`"RuleVersion"` بيزيد مع أي تغيير في القواعد والإيقاف، ومراجع واحد في Community و`AllowSelfApproval` مقفول افتراضيًا، والـ patterns بـ `internal/pathglob` من غير `..` |
| [0081](0081-keyring-and-envelope-encryption.md) | سلسلة المفاتيح والتشفير المغلّف (M5.1): KEK بإصدارات في ملف منفصل عن `DHOW_SECRET_KEY` (systemd credential أو Compose secret أو `DHOW_KEYRING_FILE`، بصاحب وصلاحية `0600` ومن غير symlink)، وDEK لكل صاحب في `"DataKeys"` ملفوف بـ AES-256-GCM وAAD بالصاحب والإصدار، وصيغة القيمة `0x02` بالـ `DataKeyId` و`Open` بيتأكد من الصاحب، وقيمة فحص HMAC لكل إصدار والإقلاع بيقف بـ 79، والتفعيل بأمر بعد ما كل instance حي يحمّل الإصدار (`KEK_NOT_EVERYWHERE`)، وإعادة اللف في الخلفية تحت advisory lock، و`keys export` بـ age، و`--discard-unreadable` بتأكيد صريح، وKMS بعدين ورا `KeyProvider` |
| [0082](0082-sealer-users-move-to-the-envelope.md) | نقل مستخدمي `Sealer` (M5.2) وU4: كل عمود متشفّر بيتسجّل «مكان قيمة» (الصاحب والـ label والـ context وحد الـ CHECK) من الموديول نفسه وApp بيجمعهم، والقراية بالصيغتين `0x01` و`0x03` والكتابة `0x03` بنفس الـ labels والـ contexts، وbackfill لكل عمود في `Core/Backfill` (دفعات بـ `SKIP LOCKED` وadvisory lock والتقدّم في نفس الـ transaction و`BACKFILL_NOT_DONE`)، و`keys rekey` بحالة `superseded` وإعادة ختم ومسح ويكمّل بعد أي توقف، و`DHOW_SECRET_KEY` للتوقيع والـ HMAC بس بعد الـ backfill، وhook بيرفض `NewSealer` |
| [0083](0083-secrets-variables-and-registry-credentials.md) | الأسرار والمتغيّرات وبيانات دخول الـ registry (M5.4): موديول `Secrets` بنطاقات المؤسسة والمشروع والبيئة وDEK لكل نطاق، والأولوية البيئة ثم المشروع ثم المؤسسة و`Overrides` على السر الأضيق، والـ API للكتابة بس من غير قيمة ولا بصمة، و5 إصدارات و30 يوم والمسح `NULL`، والاسم من غير فرق في الحروف و`DHOW_` محجوز والقيمة من 8 byte لـ 32 KiB، والمتغيّرات للقراية بـ `project.read`، والـ registry للـ host نفسه والـ realm الآمن بس، و`secret.read` و`secret.manage` و`org.secret.manage` والتوكنات `secret.read` بس، والـ port `Core/SecretStore` |
| [0084](0084-secret-delivery-forks-and-cache.md) | توصيل الأسرار (M5.5): `Check` قبل الإدراج و`"FailureDetail"` بالأسماء، والتوصيل وقت `Start` بس بعد فحص الـ fork والـ `RunJob`، و`job.secrets_delivered`، و`files:`، و`vars.*` من المخزن، وفحص الـ artifacts والـ cache في الـ runner (`ARTIFACT_CONTAINS_SECRET`)، و`registry_auth` بـ authfile بره الـ sandbox، و`"ForkCacheRestore"` و`"HadSecrets"` للـ forks |
| [0085](0085-environment-gate-and-deployment-lock.md) | بوابة البيئة (M5.6أ): عمود `"Gate"` على `"RunJobs"` مش حالة جديدة، والـ port `Core/Deployments` (`Lock` و`Started` و`Settle` و`ReleaseStale`)، وترتيب الفحوص (الـ fork ثم الوجود ثم `vars` و`if` ثم قاعدة الـ ref بالنوع ثم الصور بالـ digest في الملف والإضافات ثم الأسرار ثم الإيقاف والمراجعين والمؤقت والتزامن)، والـ PR بـ `AllowPullRequests` وفرع الـ base، وrun واحد ماسك البيئة بـ `"LockedByRunId"` والأصغر رقمًا بعده، و`"Deployments"`، و`ENVIRONMENT_RULE_CHANGED` وقت `Start` |
| [0086](0086-deployment-approvals.md) | موافقات النشر (M5.6ب): `"DeploymentApprovals"` بطلب حي واحد لكل (run، بيئة، جولة) وtrigger بيقفل الانتقالات من `pending`، و`"PipelineRuns"."Round"` بيزيد مع إعادة التشغيل، والموافقة بتعدّي بنفس `"RuleVersion"` ومراجع لسه مؤهّل، والاستبدال على نفس الـ ref، والانتهاء والرفض (`APPROVAL_EXPIRED` و`DEPLOYMENT_REJECTED` و`SUPERSEDED`)، والموافقة والرفض بخطوة تانية ومن غير «نفسه» (اللي شغّل أو صاحب الـ login على GitHub/GitLab اللي عمل الـ push، `"UserIdentities"."Login"`)، والتخطّي للمراجعين والمؤقت بس، و`Reopen` بعد الـ commit، والقايمة والعدّاد |
