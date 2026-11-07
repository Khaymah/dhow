# حالة داو: اللي اتعمل واللي لسه

**آخر تحديث:** 2026-11-05 · **main:** `1bddb8a` (#105)

الوثيقة دي نقطة البداية لأي جلسة جديدة. التفاصيل في [سجل التقدّم](Progress/README.md) والقرارات في [`ADR/`](ADR/README.md) والخطة الكاملة في [`docs/roadmap.md`](../docs/roadmap.md).

## الريبو والوصول

- الريبو `Khaymah/dhow`، واسمه القديم `Khaymah/dhow` بيعمل redirect.
- أحداث الـ PRs (الـ CI والدمج) بتوصل للجلسة، و`get_check_runs` لسه مفيد لما الحدث يتأخر.
- الـ proxy بيرفض مسح الـ branches ورفع الـ tags. الـ branches بتتمسح بعد الدمج بـ `delete-merged-branch.yml`، والـ tags بتتعمل من GitHub نفسه.

## اللي اتنفّذ واتعمله merge

| المرحلة | المحتوى | الـ PR |
|---|---|---|
| M0 | الـ design system والمعمارية ونموذج الأمان والـ ADRs | قبل الـ PRs |
| M1 | أساس الريبو والـ CI، وهيكل السيرفر، وواجهة الـ shell، والـ Docker والـ Compose، والـ binary الموقّع والمنصّب التفاعلي، وأدلة الـ reverse proxy، وتقسيم الصفحات | #1 #2 #3 #4 #11 #12 #14 |
| M2.1 | إعداد أول owner، وكلمة السر، والجلسات، والـ audit، وشاشات الهوية | #10 |
| M2.2 | TOTP وrecovery codes وpasskeys | #16 |
| M2.3 | المؤسسات والفرق والأدوار والدعوات | #18 |
| M2.4 | OIDC ودخول GitHub والـ API tokens والتفضيلات | #20 |
| M2.5 | التقوية والمراجعة الداخلية ([قايمة الفحص](../docs/security/auth-review-checklist.md)) | #23 |
| M3.1 | عقد الـ proto والـ codegen | #15 |
| M3.2 | تسجيل الـ runners وهويتهم والـ runner plane | #17 |
| M3.3 | الـ jobs والـ leases والـ fencing والـ reconciler | #19 |
| M3.4 | تخزين الـ logs والـ SSE | #21 |
| M3.5 | الـ runner binary والـ lease loop والـ masking (بـ driver وهمي) | #22 |
| M2.6 | سياسة الخطوة التانية للمؤسسة، والجلسة 7 أيام وخمول 4 ساعات للأدمن، وإيميلات تنبيه بيانات الدخول (ADR-0054) | #26 |
| M3.6 | driver الـ Podman rootless، والـ agent كـ PID 1، والـ seccomp، والحدود متحقق منها من الـ cgroup نفسه، والـ escape والـ limits suites في job `isolation` على `ubuntu-26.04` (ADR-0055) | #25 |
| تطوير محلي | `docs/development.md`، وسكربتات التجهيز `deploy/dev/setup.sh` و`setup.ps1`، وأهداف `make dev-*` | #27 |
| مراجعة M3.6 | إصلاحات المراجعة الأمنية المستقلة لـ M3.6 (ADR-0056): الخطوة بتخلص بكل الـ processes بتاعتها، وملفات الأسرار، ووضع `directory` مقفول، والـ image بالـ ID، والـ vsock مقفول، والـ uid map بيتفحص. وكمان السيرفر بيكمّل أسماء الـ images القصيرة، وتغيير الاسم لـ Khaymah بالكامل | #28 |
| M3.7 | الـ egress proxy: القايمة ASCII وpunycode، وطبقة `Egress` بتحل وتفحص في كل اتصال، والـ forwarder في الـ agent، والـ helper `connect`، ونطاقات المشغّل من غير metadata (ADR-0057) | #31 |
| M3.9 | آخر مرحلة في الإلغاء بالتدريج: `cgroup.kill` وفحص الـ processes الفاضلة وقت شيل الـ sandbox، والـ runner بيبطّل ياخد شغل لو فضل process عايش، وبيرفض مجلد حالة مقفول قدّام الـ user namespace. وsuite الفوضى بالتسع سيناريوهات على processes حقيقية، كل ليلة وعلى label `chaos` (ADR-0058) | #32 |
| تجهيز Windows | `setup.sh` بيتجاهل الـ PATH بتاع Windows جوه WSL، و`setup.ps1` بيحدّث الـ clone، و`libatomic`، وفحص كل رد، وشريط تحميل، وbash 3.2. وCI بيشغّل السكربت على 4 توزيعات نضيفة | #33، #34 |
| تغيير الاسم لـ Khaymah | `193f80c` غيّر الاسم كنص جوه ملفات `.pb.go`، فكل binary كان بيعمل panic، والـ image بقى اسمها بحرف كبير. والإصلاح: `make gen` واسم الـ image `ghcr.io/khaymah/dhow` | #29 |
| خطة السهولة | `Plan/Usability-Plan.md` و`Plan/UX-Review.md`: ملاحظات Sabry الخمسة ومراجعة الداشبورد كاملة | #36 |
| الطلبات المتكررة | الواجهة بطّلت تبعت نفس الطلب أكتر من مرة، واختبار `App/Requests.test.tsx` بيعدّ الطلبات | #35 |
| اتجاه التصميم | «غرفة التشغيل» مع حاجتين من الاتجاه ب (ADR-0059) | #37 |
| تطوير بـ Docker بس | `deploy/dev/compose.yaml` بيشغّل Postgres والسيرفر والواجهة، وقاعدة البيانات على `55432` | #38، #43 |
| M3.12 (الواجهة) | أجهزة التشغيل ورموز التسجيل، والتشغيلات وتفاصيلها والسجل الحي بـ SSE، و`FencingToken` و`QueuedReason` في الـ API | #39، #44 |
| أعطال الداشبورد | شريط تبويبات المؤسسة، وأسماء كل أحداث السجل، وشارة الخطوة التانية، وTOTP مابيبدأش غير بزرار | #40 |
| مفاتيح API المؤسسة | شريحة المفاتيح، وفلاتر سجل الأحداث وتفاصيل الحدث (`GET /audit-events/{eventId}`) | #41 |
| ADR-0059 الخطوة 1 | خطوط IBM Plex، ومقاسات عربي صريحة، وtokens المسافات، وثيمات أغمق، واختبار بيرفض أي `text-*` مش معرّف | #42 |
| متطلبات الموارد | `make bench`، و`docs/requirements.md` (الحد الأدنى 1 CPU و2 GiB و5 GiB)، والتنصيب الجديد بيقف تحته والترقية بتحذّر بس (ADR-0060) | #46 |
| ADR-0059 الخطوة 2 | كل قسم صفحة برابط، وقايمة جانبية بمجموعات 248px، وشريط فوق 60px، وشريط موبايل بأسماء، والشرائح للإنشاء والتأكيد بس (`?slide=`)، والروابط القديمة بتتحوّل | #48 |
| ADR-0059 الخطوة 3 | الرئيسية بالحالة، والعدّادات في القايمة، و`GET /organizations/{id}/overview` (ADR-0061) بصلاحية وخطوة تانية لكل جزء | #49 |
| ADR-0059 الخطوة 4 | جدول «لماذا ينتظر؟»، والجهاز جنب القايمة (`MasterDetail`)، وصفحة التشغيلات الفاضية، وأمر التسجيل `sudo -u dhow-runner` و`--dev` | #50 |
| ADR-0059 الخطوة 5 | العضو والفريق جنب القايمة، والدعوات، وإعدادات المؤسسة بمنطقة خطر، وصفحات الحساب، و`<bdi>` للأسماء | #51 |
| العربي المبسّط | كل نص بيقراه المستخدم (الواجهة ورسايل السيرفر والإيميلات والـ installer) بالعربي المبسّط، ودليل أسلوب وhook (ADR-0062) | #53 |
| M3.10 الجزء أ | أرشيفات الـ runner، ووحدة systemd (5.2 MEDIUM)، و`rotate-keys`، و`get.sh --component runner` (ADR-0065) | #57 |
| الوثائق بالعربي أولًا | `docs/` بالعربي المبسّط و`docs/en/` بالإنجليزي، و`make docs-check`، وhook الـ commit (ADR-0064) | #58 |
| متابعات الجولة التانية | hook الـ commit مع `git -c ... commit`، والاختبار المتقلّب بتاع الإيميل التجريبي، و`LastActiveAt` للعضو، والدعوات المقبولة في الـ API، وتنبيه حسابات مزوّدي القاعدة لما env يحدد القايمة | #59 |
| إعدادات السيرفر، الجزء 1 | البريد والمؤسسات وكلمات السر المسرّبة وحدود الطلبات والسجلات من «إعدادات السيرفر»، وenv بيكسب ويقفل، و`pg_notify` لكل instance (ADR-0063) | #54 |
| إعدادات السيرفر، الجزء 2 | مزوّدي الدخول من اللوحة بجدول `"OidcProviders"`، و«جرّب الاتصال»، والشيل اللي مايقفلش حد برّه (ADR-0063) | #55 |
| متابعات بعد خطة السهولة | فشل إعادة تحميل الإعدادات ظاهر في `/health/ready`، واسم العنصر في مسار الصفحة، وصيغ الجمع، وفلتر التشغيلات بأكتر من حالة، وفرق العضو في قايمة الأعضاء | #56 |
| M3.10 الجزء ب | الـ engine في `internal/installer`، و`dhow-runner install` و`doctor` و`uninstall`، والهوية مشفّرة بـ `systemd-creds`، و`AuthService.Revoke` (ADR-0066) | #60 |
| بعد اختبار الـ VMs | حدود ذاكرة الـ job على مقاس الأجهزة الصغيرة قبل التنصيب، ورفض SELinux enforcing (ADR-0067)، وطباعة مخرجات الـ VM | #61 |
| الـ README | الحالة الحقيقية وقصة Deploy Center | #62 |
| M4.1 | `internal/pipelinespec`: الـ parser والـ validator وCEL والـ DAG والـ matrix والـ JSON Schema (ADR-0069) | #65 |
| M4.2 | `dhow lint` في `apps/cli`، والأرشيفات الموقّعة، و`get.sh --component cli` | #66 |
| M4.3 | موديول `Projects` و`policy.Project` بالـ bindings على المشروع، و`Core/JobQueue` جوه transaction اللي بينادي (ADR-0070) | #68 |
| إصلاح CVE-2026-98432 | MPTCP مقفول في كل ملفات داو التنفيذية بـ `godebug multipathtcp=0`، وتحذير في `doctor`، وصفحة تنبيهات النواة (ADR-0073) | #70 |
| M4.4 | موديول `Sources`: ربط Git عام بـ deploy key وhost key متثبّت، وwebhooks موقّعة، و`netguard` ([ADR-0072](ADR/0072-webhooks-and-git-credentials.md)) | #71 |
| خطة الترقيات والمساعدة | `Plan/Upgrades-and-Help.md` وقاعدة expand/contract للـ migrations وسياسة التحديث ([ADR-0077](ADR/0077-upgrades-migrations-and-update-policy.md)) | #72 |
| M4.8 | موديول `Pipelines`: الـ runs والـ DAG والـ triggers والـ forks وتثبيت الصور بالـ digest، والربط بالـ checkout ([ADR-0076](ADR/0076-pipeline-runs-definitions-and-forks.md)) | #73 |
| M4.7 | الـ runner: أكتر من حاوية بمساحة شغل مشتركة، والـ checkout المحصّن، والإضافات، والـ outputs، وقدرات الـ runner ([ADR-0075](ADR/0075-multi-container-attempts-checkout-and-plugins.md)) | #74 |
| إصلاحات مراجعة الربط بالـ checkout | ترقيم أسرار الخطوات، وإثبات ملف الـ pipeline بكائنات git، وإخفاء القيم المحسوبة، وجلب التاريخ على مراحل | #75 |
| الـ installer على Fedora 45 | وقفة واضحة قبل أي تغيير لو PGDG مش داعم الإصدار، والـ CI على `fedora:44` | #76 |
| minimatch | `minimatch@^10` لـ 11.0.4 في أدوات الواجهة | #77 |
| الـ runner في حزمة التطوير بـ Docker | Podman rootless في حاوية privileged للتطوير بس، وprofile `fake-runner`، و`dhow-dev-register` ([ADR-0079](ADR/0079-runner-in-the-docker-dev-stack.md)) | #78 |
| M4.9 | الـ artifacts والـ cache ومخزن الملفات على القرص ([ADR-0078](ADR/0078-artifacts-cache-and-blob-store.md)) | #79 |
| M4.10 | واجهة المشاريع وGit العام والتنفيذات والملفات الناتجة، ومدة حفظ السجلات، وفصل المستودع بيمسح المفاتيح ويلغي التنفيذات المفتوحة | #81 |
| U1 | hook قاعدة الـ migrations (`migration_rules.py`) و`Compat_test.go` بنفس الحالات من `testdata/compat/` (ADR-0077) | #85 |
| M4.11 وM3.13 | الـ fuzz الليلي، ومراجعة التهديدات STRIDE، و`deploy/m3-demo` و`deploy/m4-demo` و`leak-sweep.sh`، وهيكل `providers.yml` | #86 |
| الـ runner في حزمة التطوير | مسح حالة Podman rootless القديمة لما الحاوية تقوم | #87 |
| M5.3 | البيئات وقواعد الحماية والـ API ([ADR-0080](ADR/0080-environments-are-made-by-admins-only.md)) | #89 |
| U2 | توافق الـ schema في الـ binary، و`"SchemaFloor"` و`"ServerInstances"`، والرمز `78`، و`dhow-server migrate status\|plan\|dry-run` (ADR-0077) | #90 |
| M5.1 | سلسلة المفاتيح والتشفير المغلّف ([ADR-0081](ADR/0081-keyring-and-envelope-encryption.md)) | #91 |
| M5.2 وU4 | نقل كل مستخدمي `Sealer` للتشفير المغلّف، و`Core/Backfill`، و`dhow-server keys rekey` ([ADR-0082](ADR/0082-sealer-users-move-to-the-envelope.md)) | #92 |
| M5.4 | الأسرار والمتغيّرات وبيانات دخول الـ registry: المخزن والـ API، و`Core/SecretStore`، والتثبيت بالـ digest ببيانات الدخول ([ADR-0083](ADR/0083-secrets-variables-and-registry-credentials.md)) | #93 |
| M5.5 | توصيل الأسرار للـ runner، والـ forks، والـ cache ([ADR-0084](ADR/0084-secret-delivery-forks-and-cache.md)) | #94 |
| M5.6أ | بوابة البيئة وقفلها وسجل النشر ([ADR-0085](ADR/0085-environment-gate-and-deployment-lock.md)) | #95 |
| M5.6ب | موافقات النشر والتخطّي بخطوة تانية، وربط الـ login ([ADR-0086](ADR/0086-deployment-approvals.md)) | #96 |
| M5.7أ1 | صفحات الأسرار والمتغيّرات والـ registry للمؤسسة والمشروع | #97 |
| M5.7أ2 | صفحات البيئات: القواعد والإيقاف وأسرار البيئة وسجل النشر | #98 |
| M5.7ب | موافقات النشر في الواجهة، وحالات البوابة في صفحة التنفيذ، والعدّاد | #99 |
| M5.7 | اختبارات المتصفح من أول لآخر في الـ CI ([ADR-0087](ADR/0087-browser-end-to-end-tests-in-ci.md)) | #101 |
| صورة السحب | openssh `10.0_p1-r11` بعد تحديث Alpine الأمني | #102 |
| M5.9أ | الـ contract لصيغة `0x01` على 9 أعمدة، و`FuzzAllowedRefs` و`FuzzSecretName`، وفحص تسريب لكل نطاق ([ADR-0088](ADR/0088-contract-the-old-sealed-format.md)) | #103 |
| M5.9ب | `deploy/m5-demo` (موافقة بـ TOTP، ورفض الموافقة على النفس، وتدوير KEK في النص، وفحص التسريب)، وجدول التهديدات في `docs/security-model.md` §13، والـ roadmap ([ADR-0089](ADR/0089-m5-demo-and-threat-review.md)) | #104 |
| عزل الأسرار | 3 اختبارات بتثبت إن runner بياخد أسرار الـ job المحجوز بس (§13.1) | #105 |
| خطة M6 | أهداف النشر: SSH وCompose والنشر المتدرّج على مجموعة سيرفرات واستقبال artifacts ([`M6-Deploy-Targets.md`](M6-Deploy-Targets.md)) | #106 |
| M6.1 | أهداف النشر وسيرفراتها وبصمات السيرفرات المثبّتة ([ADR-0090](ADR/0090-deploy-targets-hosts-and-host-keys.md)) | #107 |
| M6.2أ | توصيل الهدف للـ runner: `StepTarget` في البروتوكول، ومفتاح الهدف، وحدود الـ egress وفحص الصحة ([ADR-0091](ADR/0091-deploy-target-delivery-to-the-runner.md)) | #108 |
| M6.2ب | جانب الـ runner: egress للعناوين المحددة بالظبط، وملف المفتاح لخطوة النشر بس، و`internal/archive`، وتقارير الإصدارات ([ADR-0092](ADR/0092-deploy-target-runner-side-and-release-reports.md)) | #109 |
| M6.3أ | محرّك الإصدارات `internal/deployrelease` وإضافة `dhow-deploy-ssh` على سيرفر واحد، وE2E على sshd حقيقي ([ADR-0093](ADR/0093-release-engine-and-ssh-deploy-plugin.md)) | #110 |
| M6.3ب | النشر المتدرّج على مجموعة سيرفرات والرجوع الجماعي ([ADR-0094](ADR/0094-rolling-deploy-and-group-rollback.md)) | #111 |
| اختبار الـ chaos | الـ harness بيعمل keyring للسيرفر، والتشغيل الليلي رجع أخضر | #114 |
| M6.4أ | أسباب فشل `DEPLOY_*`، ومهلة الإلغاء، وحالة `unknown` وإصلاحها، و«افحص الهدف»، وانتظار الهدف المشغول ([ADR-0095](ADR/0095-deploy-failure-reasons-unknown-state-and-target-check.md)) | #113 |
| M6.5 | إضافة Docker Compose بسياسة ملف آمنة، والبيانات في named volumes، وفحص تاني على النموذج النهائي (ADR-0096) | #112 |
| M6.4ب | الرجوع اليدوي للمجموعة، وإعادة رفع الإصدار من الـ artifact، وقسم أهداف النشر في الواجهة (ADR-0097) | #115 |
| M6.6 | استقبال الـ artifacts من CI خارجي بتوكن استقبال، و`on.artifact`، وتثبيت الـ artifacts المستخدمة في إصدارات (ADR-0098) | #116 |
| M6.7 | `dhow artifact push` وGitHub Action، والـ CLI يقدر يوصل للشبكة من `internal/ingestpush` بس (ADR-0099) | #117 |
| M6.8أ | واجهة أهداف النشر وسيرفراتها وتأكيد البصمة وسجل النشر، والصف بيلف على الموبايل | #118 |
| M6.8ب | توكنات الاستقبال والملفات المستقبلة في الواجهة، والتوكن بيظهر مرة واحدة بس | #119 |
| تقليل الـ CI | CodeQL لكل لغة لما تتغيّر، والعروض حسب الـ package | #120 |
| متابعتان من M6.8 | الخطوة الثانية لكتابة سر هو مفتاح هدف نشر (ADR-0100)، وبوابات الـ slides الأربعة (`secret-new` و`variable-new` و`registry-new` و`environment-new`) بالصلاحية الحقيقية | #122 |
| M6.9ب | `deploy/m6-demo/run.sh` عدّى في الـ CI على Ubuntu 26.04 (sshd وCompose حقيقيين، رجوع تلقائي ويدوي، `dhow artifact push`، مفتاح سيرفر اتغيّر، فحص التسريب)، وسيناريو chaos لقتل الـ runner بعد التبديل، وسبب `DEPLOY_INTERRUPTED` (ADR-0101) | #121 |
| M6.9د | سدّ ثغرات المراجعة: حد الرفع المتزامن العام، و413 قبل قراءة الجسم، وحدود خرج السيرفر البعيد، ومفتاح الهدف خارج الـ argv، وأحداث `deployment.release_*` بالـ digest والمنفّذ، و`make proxy-check` في الـ CI (job `proxy`) | #124 |
| M6.9ج | مراجعة تهديدات M6 (§14 في `docs/security-model.md`) بكل صف ومعاه مسار اختباره، والمخاطر المقبولة، وM6 خلص في `docs/roadmap.md`. اللي فاضل: تشغيل الـ demo على جهاز نظيف بـ Debian 13 | #123 |
| M6.9هـ | إصلاحات المراجعة المستقلة لـ M6 ([ADR-0102](ADR/0102-m6-review-fixes-ingest-owner-reach-and-limits.md)): توكن الاستقبال بيتحقق من صاحبه وقت الاستعمال وبيتلغى عند إزالته، وتأكيد البيئة المحمية بيتحفظ والرفع بيترفض لحد ما يتأكد، وأخذ القفل المنتهي بالـ rename، ونص الـ hooks لمن يملك `environment.manage` بس، و8 رفعات متزامنة و2 لكل مؤسسة و2 لكل عنوان شبكة، ومحدد الطلبات إلزامي في `Environments`. L6 مقبولة | #125 |

شرط خلاص M2 اتحقق: اختبارات الصلاحيات خضرا، ومفيش finding مفتوح Critical أو High. والمراجعة الخارجية اتنقلت لـ M9 (ADR-0040).

## شغّال ومش متعمله merge

| الشغل | الـ branch | الـ PR | اللي فاضل |
|---|---|---|---|
| صفحات `secret-edit-*` و`variable-edit-*` و`registry-edit-*` مالهاش بوابة دور في `Panels.ts`، والسيرفر بس اللي بيرفض | — | — | نفس بوابات `*-new` |
| ~~`runner.revoked` بيتنشر بعد الـ commit فلو الـ process وقع الـ audit بيضيع~~ | `feat/notifications-m72a` | — | اتقفل في M7.2أ ([ADR-0108](ADR/0108-notifications-foundation.md)): النشر بقى جوه transaction الإلغاء (`Audited.Tx`) والإشعار بيتكتب في الـ outbox معاه. نفس الشيء لـ `auth.*` لسه بعد الـ commit (مذكور في الـ ADR) |
| خطة M7: واجهة المنتج ([`M7-Product-UI.md`](M7-Product-UI.md)) | `plan/m7-product-ui` | #126 | اتجاوبت كلها (قرار 3: SSE لكل حاجة، وأحداث الحساب في الإشعارات)، وبعد الدمج M7.0 |
| M7.0: القياس والميزانيات ([ADR-0103](ADR/0103-web-size-budgets.md) لـ [0107](ADR/0107-trusted-types-report-only-then-enforce.md)) | `feat/m7-baseline-m70` | #127 (draft) | اتعمل: `pnpm size` بيفرض سقف لكل chunk كسول (60 KB و35 KB للعارض)، والأولاني 127.7 لـ 123.8 KB، وأدوات قياس الأداء و`LogPerf.spec.ts`، وترويسة Trusted Types report-only في `DHOW_DEV`، وحارس الـ e2e على كل مخالفة CSP. لسه: الهدف 120 KB ما اتحققش، وأرقام الـ CI بتتسجّل بعد أول تشغيل لـ `make e2e` |
| M7.1: أساس سهولة الوصول ([ADR-0106](ADR/0106-accessibility-verification.md)) | `feat/a11y-m71` | #128 (اتدمج) | اتعمل: `@axe-core/playwright` 4.14.0 و`E2E/Accessibility.spec.ts` (9 صفحات × عربي وإنجليزي × 4 ثيمات، بيفشل على serious/critical) وحارس تركيز مخفي، وخانات الاختيار 24px، وحجم هدف شريحة الفتات، و`scroll-pb` للـ main، وخانة المساعدة الثابتة في `PageHeader` (فاضية لحد M7.8)، واختبارات `A11yRules` و`Keyboard`، و`docs/guides/accessibility.md` بالنسختين و`Plan/A11y-Manual-Checklist.md`. لسه: الـ spec ما اتشغّلش على stack حقيقي (CI بس)، والقايمة اليدوية لسه من غير توقيع، و`ChoiceList` من غير أسهم |
| M7.7: رسايل خطأ بتذكر الحل (`Fix` و`DocsPath`) | `feat/error-fix-m77` | #129 | اتعمل: `Fix` و`DocsPath` في `ErrorDetail`، و120 كود من 123 ليهم جملة حل بالعربي والإنجليزي (`Core/I18n/I18n.Fix.{Ar,En}.go`) و3 في allowlist مكتوب سببها (`METHOD_NOT_ALLOWED` و`WEBHOOK_REJECTED` و`LENGTH_REQUIRED`)، وكل `DocsPath` اختبار بيتأكد إنه ملف وanchor موجودين في `docs/` و`docs/en/`. الواجهة: `ErrorAlert` و`Toast` بيعرضوا الحل ورابط «اعرف أكثر» (`role="alert"` و`aria-describedby`)، و`useApiErrorText` بيضيف الحل للنصوص العادية. لسه: الـ toast بتاع الأخطاء مابيستخدمش `Fix` لحد ما فيه مكان بيعمل Push لخطأ API، والـ anchors مكتوبة يدوي في `I18n.Docs.go` (لو عنوان في الوثائق اتغيّر، الاختبار بيفشل) |
| M7.2أ: الإشعارات: الأساس ([ADR-0108](ADR/0108-notifications-foundation.md)) | `feat/notifications-m72a` | draft | اتعمل: `Core/Outbox` (طابور في Postgres بقفل advisory و`SKIP LOCKED` وتأجير وإعادة محاولة 1د/5د/30د/2س وقاطع دايرة)، و`Modules/Notifications` (قنوات بريد وwebhook موقّع، قواعد، سجل تسليم، كتم شخصي، قنوات المؤسسة)، و`netguard` على كل إرسال، و`ConfigSealed` في `keys rekey` والـ backfill، و`notification.manage`، ونقل `Alerts` على الـ outbox، ونشر `runner.revoked` جوه الـ transaction، وأحداث الحساب (جهاز جديد والخطوة التانية والأعضاء). لسه: Slack وTelegram وDiscord (M7.2ب)، والواجهة وصندوق `inapp` (M7.3)، ونشر `auth.*` جوه الـ transaction، وبريد القنوات بدور المؤسسة بس (من غير ربط المشروع) |
| M7.2ب: الإشعارات: Slack وDiscord وTelegram ([ADR-0110](ADR/0110-notifications-chat-channels.md)) | `feat/notifications-m72b` | #132 (draft) | اتعمل: ثلاث adapters بمضيف وشكل عنوان ثابت (يتفحص وقت الحفظ ووقت كل إرسال، و`CHANNEL_HOST_NOT_ALLOWED` بنص مترجم وجملة حل)، وتهريب حسب الخدمة (`&<>` في Slack، و`allowed_mentions` فاضية في Discord، وHTML مهرَّب في Telegram)، وإعدادات كل قناة مختومة في `ConfigSealed` ومافيش سر في أي رد أو log، و`RedactError` لتوكن Telegram اللي في المسار، و`deploy/e2e-targets/notify-receiver` (بيحاكي الثلاث بقواعدهم، باختباراته بـ `-race`)، و`FuzzChatURL`، و`make notify-live` (اختبار حي بوسم `live`). القواعد والكتم كانوا كاملين من M7.2أ. لسه: Discord webhook البيئة رد 404 في الاختبار الحي (Discord نفسه بيقول مجهول، مش من كودنا)، وتركيب الـ receiver في `make e2e`، والواجهة (M7.3) |

M2.6 وM3.6 والتطوير المحلي وإصلاحات مراجعة M3.6 وتغيير الاسم وM3.7 وتجهيز Windows وM3.9 اتدمجوا في 2026-10-28، ومن #35 لـ #59 اتدمجوا في 2026-10-28 و2026-10-29.

**الخطوات قبل الـ merge لأي مرحلة:** الشغل، وبعدين `make check`، ومراجعة أمنية مستقلة، وإصلاح الـ findings، وPR، والـ CI أخضر، وsquash merge.

## اللي لسه في M3 (بالترتيب)

```
(M3.6 وM3.7 وM3.9 خلصوا) ─┬─► M3.8
                          ├─► M3.10
                          └─► M3.11
M3.12 (الشرائح خلصت)          M3.13 في الآخر
```

| المرحلة | المحتوى |
|---|---|
| M3.8 | gVisor كـ spike بشروط go/no-go، ولو فشل يتأجّل على الـ host بـ ADR |
| M3.10 | خلص على Debian وUbuntu (#57، #60، #61). SELinux بعد أول نسخة (ADR-0067، ADR-0068) |
| M3.11 | driver الـ Kubernetes والـ Helm chart، و`make helm-e2e` على kind |
| M3.12 | خلص (#39، #44، #48، #50) |
| M3.13 | خلص في M4.11 (`feat/m4-hardening-demo`): الـ fuzz الإضافي (`Wire` و`jobspec` والـ CONNECT)، ومراجعة تهديدات الـ runner plane وقايمة المراجعة الخارجية في `docs/security-model.md`، و`deploy/m3-demo/run.sh`. فاضل: `make helm-e2e` مع M3.11 بعد أول نسخة، وتنضيف `"RunnerSessions"` |

## خطة السهولة (ملاحظات Sabry، بالترتيب)

1. ~~الطلبات المتكررة~~ (#35)، و~~الموارد~~ (#46).
2. ~~Docker Compose~~ (#38، #43).
3. ~~العربي المبسّط في الواجهة ورسايل السيرفر والـ CLI~~ (#53، ADR-0062).
4. ~~الإعدادات من env لقاعدة البيانات~~ (#54، #55، ADR-0063).
5. ~~شكل الداشبورد (ADR-0059)~~: الخطوات الخمسة (#42، #48، #49، #50، #51).

الملاحظات الخمسة خلصت.

**فاضل:**
- **الدعوات المقبولة في الواجهة:** الـ API بيرجّعها (`?Status=accepted`)، بس صفحة الأعضاء مافيهاش مكان ليها، فهي من الـ API بس لحد ما نقرر مكانها.

## الطريق لأول نسخة production (ADR-0068)

[ADR-0068](ADR/0068-first-production-release-scope.md) حدّد نطاق أول نسخة production: الـ runners على Debian وUbuntu بـ Podman rootless بس، والنشر على SSH/VPS وDocker Compose، وGitHub وGitLab وGit عام، ونسخة Community بس. الترتيب: M3.13 ← M4 ← M5 ← M6 ← M7 ← M9 ← M10 ← M11، واللي اتأجّل (SELinux وKubernetes وHelm وgVisor وBitbucket وStatic/CDN/SFTP والمحرر المرئي وTeams وWhatsApp وM8 ومكوّن GitLab CI) في قسم «بعد أول نسخة» في [`docs/roadmap.md`](../docs/roadmap.md).

**M4 شغّال:** خطته في [`M4-Pipelines.md`](M4-Pipelines.md) اتقبلت، وSabry جاوب أسئلتها كلها بالتوصية يوم 2026-10-30. **M4.1** (`internal/pipelinespec`، [ADR-0069](ADR/0069-pipeline-spec-parser-and-validator.md)) اتدمج في #65، و**M4.2** (`dhow lint`) في #66، و**M4.3** (`Projects` والـ ports، [ADR-0070](ADR/0070-ports-in-core-and-project-permissions.md)) في #68. **M4.4** (`Sources` وGit عام، [ADR-0072](ADR/0072-webhooks-and-git-credentials.md)) اتدمج في #71. وقرار Sabry الجديد لربط Git (الـ OAuth للتجهيز بس والـ deploy keys للوصول) في [ADR-0074](ADR/0074-git-connection-oauth-for-setup-deploy-keys-for-access.md)، وفيه نقط لازم تتحسم في أول M4.6. **M4.7** (الـ runner بأكتر من حاوية والـ checkout والإضافات، [ADR-0075](ADR/0075-multi-container-attempts-checkout-and-plugins.md)) اتدمج في #74. **M4.8** (`Pipelines`، [ADR-0076](ADR/0076-pipeline-runs-definitions-and-forks.md)) اتدمج في #73، والربط بالـ checkout وإصلاحات مراجعته في #75. **M4.9** (الـ artifacts والـ cache، [ADR-0078](ADR/0078-artifacts-cache-and-blob-store.md)) اتدمج في #79، و**M4.10** (الواجهة) في #81، و**M4.11** (الـ fuzz الليلي ومراجعة التهديدات والـ demos وفحص تسريب بيانات الدخول) اتدمج في #86. **M4 خلص ما عدا M4.5 وM4.6** (GitHub وGitLab)، مستنيين حسابات الاختبار، ومعاهم job `providers` في الـ CI اللي متعمل هيكل بس.

**M5 (الأسرار والبيئات):** الخطة في [`M5-Secrets-Environments.md`](M5-Secrets-Environments.md) (مقبولة، وSabry اختار التوصية في الأسئلة الستة يوم 2026-11-03: KEK في ملف منفصل، والنوافذ والتجميد وتعدد المراجعين في M8، وOIDC بعد أول نسخة، والـ forks من غير cache افتراضيًا، والخطوة التانية مع كل موافقة، والتوكنات `secret.read` بس)، وبتبدأ بعد U2. **M5.3** (البيئات، [ADR-0080](ADR/0080-environments-are-made-by-admins-only.md)) اتدمج في #89. **M5.1** (سلسلة المفاتيح والتشفير المغلّف، [ADR-0081](ADR/0081-keyring-and-envelope-encryption.md)) اتدمج في #91: `Core/Keys` والـ migration `00052`، والسيرفر مايبدأش من غير السلسلة (يقف بـ `79`)، و`dhow-server keys`، و`GET /system/keys`، والـ installer والـ Compose والـ systemd وحزمة التطوير. **M5.2** (نقل مستخدمي `Sealer`، [ADR-0082](ADR/0082-sealer-users-move-to-the-envelope.md)) اتدمج في #92: كل الأعمدة المتشفّرة بقت بتقرا `0x01` و`0x03` وتكتب `0x03` بنفس الـ labels والـ contexts، وbackfill لكل عمود، و`keys rekey`، وhook بيرفض `NewSealer`. `GitConnections.TokenSealed` مع M4.6، والـ contract لـ `0x01` في M5.9. **M5.4** (الأسرار والمتغيّرات وبيانات دخول الـ registry، [ADR-0083](ADR/0083-secrets-variables-and-registry-credentials.md)) اتدمج في #93. **M5.5** (التوصيل للـ runner، [ADR-0084](ADR/0084-secret-delivery-forks-and-cache.md)) اتدمج في #94. **M5.6** اتقسم على PRين (قرار Sabry): (أ) البوابة والقفل والسجل ([ADR-0085](ADR/0085-environment-gate-and-deployment-lock.md)) اتدمج في #95، و(ب) الموافقات والتخطّي ([ADR-0086](ADR/0086-deployment-approvals.md)) اتدمج في #96. **M5.7** (الواجهة) اتقسم على تلات PRs (قرار Sabry) واتدمجوا: الأسرار والمتغيّرات والـ registry في #97، والبيئات في #98، والموافقات وحالات البوابة في #99. واختبارات Playwright من أول لآخر في الـ CI ([ADR-0087](ADR/0087-browser-end-to-end-tests-in-ci.md)) اتدمجت في #101. **M5.9** اتقسم على PRين (قرار Sabry): (أ) الـ contract لصيغة `0x01` والـ fuzz وفحص التسريب ([ADR-0088](ADR/0088-contract-the-old-sealed-format.md)) اتدمج في #103، و(ب) `deploy/m5-demo` ومراجعة التهديدات والـ roadmap ([ADR-0089](ADR/0089-m5-demo-and-threat-review.md)) اتدمج في #104. **M5 خلص** ما عدا M5.8 (OIDC) بعد أول نسخة، واختبار عزل الأسرار اتدمج في #105.

**M6 (أهداف النشر):** الخطة في [`M6-Deploy-Targets.md`](M6-Deploy-Targets.md)، وSabry جاوب أسئلتها التمانية يوم 2026-11-05 (سبعة بالتوصية، والمجموعة بقايمة سيرفرات ونشر متدرّج ورجوع جماعي). المراحل M6.1 لـ M6.9 ومعاها M6.3ب. M6.1 لـ M6.8ب اتدمجوا، وM6.9 مقسومة تلاتة: (أ) في #122 و(ب) في #121، والجاي (ج) مراجعة التهديدات والمراجعة المستقلة والـ roadmap. وقبل أول tag لازم يتعمل `Khaymah/dhow-action` والسر `DHOW_ACTION_DEPLOY_KEY`. وM5.8 (OIDC) بعد أول نسخة، والجاي M5.9.

**الترقيات والمساعدة جوه التطبيق:** الخطة في [`Upgrades-and-Help.md`](Upgrades-and-Help.md) و[ADR-0077](ADR/0077-upgrades-migrations-and-update-policy.md) (مقبولة، وSabry جاوب الأسئلة الـ 11 يوم 2026-11-02، وشرطه: كله ببلاش). قاعدة expand/contract شغّالة من 2026-11-02 على أي migration جديد. **U1** اتدمج في #85: الـ hook `migration_rules.py` بالقواعد M1 لـ M9، و`Compat_test.go` بيطبّقها في الـ CI على أي ملف بعد `00048`، والحالات المشتركة في `Database/Migrations/testdata/compat/`. **U2** اتدمج في #90: الـ binary الأقدم بيشتغل على قاعدة أحدث طول ما الـ floor (`"Instance"."SchemaFloor"`) مش أعلى من نسخته، وغير كده بيقف بالرمز `78`، و`dhow-server migrate dry-run` بيجرّب المستني ويرجع فيه، والقاعدة للمشغّل في `docs/install/upgrades.md`. **U4** اتدمج مع M5.2 في #92: `Core/Backfill` (جدول `"Backfills"`، ودفعات بـ `SKIP LOCKED` تحت advisory lock والتقدّم في نفس الـ transaction، وthrottle، و`BACKFILL_NOT_DONE` للـ contract)، والتقدّم في `migrate status`، واختبار 100,000 صف بيتقتل 3 مرات. و«عن داو» مستنية U6. وU3 (اختبار الترقية في CI) لسه ماتعملش، فسطر `make upgrade-test` في قبول M5.2 مستنيه.

## بنود مفتوحة اتأجلت

- **قبل أول tag:** تشغيل `deploy/m6-demo/run.sh` يدويًا على VPS نضيف بـ Debian 13 قبل أول tag (شرط إغلاق M6 الباقي).
- **SELinux enforcing (ADR-0067):** الـ runner مش مدعوم على Fedora وRHEL وRocky وAlma بـ enforcing، لأن `container_t` ممنوع من `connectto` على الـ control socket. اتأجّل لبعد أول نسخة (ADR-0068)، ووقتها نختار: نقلب اتجاه قناة التحكم، أو policy module بـ domain للـ runner. وبعدها نرجّع fedora وrocky-10 لـ `vm-test.yml`.

- **الـ runner:** حقل سقف الموارد في `RegisterRequest`، ورفض `RESOURCE_REQUEST_EXCEEDS_RUNNER` وقت الإنشاء.
- **الحمل:** الـ token bucket لكل runner ورد «الـ pool مشغول» (ADR-0042). والإشعار بالـ labels لو الـ runners كتروا. وحدود الـ logs لكل instance مع HA.
- **التنضيف:** تنضيف `"RunnerSessions"` (كان في M3.13 واتأجّل). (احتفاظ الـ logs لكل مؤسسة اتعمل في M4.9.)
- **الهوية:** أول دخول بالـ IdP وقبول الدعوات بيه، وback-channel logout. وحذف مؤسسة.
- **مع M4:** scopes المشروع والبيئة في الـ tokens والـ bindings.
- **مع M5، cache الـ forks (مراجعة M4.11):** تنفيذ fork بيقدر يرجّع cache الفرع الأساسي النهارده، وده مقبول لأن مفيش أسرار. أول ما الأسرار تيجي، تنفيذات الـ forks لازم ماترجّعش أي نطاق موثوق (الفرع الأساسي والـ tags)، أو ترجّع قايمة صريحة بس. مافيش تغيير في السلوك لحد M5.
- **خطوة يدوية لـ Sabry قبل أسرار `providers`:** في GitHub (Settings ← Environments ← `providers`) يقصر البيئة على `main` بس، ويطلب reviewer. `providers.yml` بقى ليلي ويدوي بس، على `main` وبشرط `DHOW_PROVIDERS_ENABLED`، ومن غير `pull_request` (الشرح في `docs/development.md`).
- **مع M5، قرار Sabry (2026-11-02):** قاعدة البيئة نفسها بتحدد الـ branches أو الـ tags اللي بتنشر عليها (مثلًا production من `main` بس، وstaging من `stage`)، والربط ده على السيرفر ومفيش commit يقدر يغيّره. فالـ job بيكتب `environment:` واحد، والـ job اللي الـ branch بتاعه مش في قاعدة البيئة بيتعلّم `skipped` بسبب واضح بدل ما يفشل. والمستودع الواحد بيفضل بـ deploy key واحد للـ checkout لكل الـ branches، ومفاتيح السيرفرات أسرار في كل بيئة.
- **الـ sandbox بعد ADR-0056:**
  - مفيش حد للـ I/O.
  - `socketcall` على 32-bit لسه مسموح.
  - وضع القرص `directory` مقفول لحد ما يبقى ليه quota حقيقية.
  - الخدمات اللي بتعيش طول الـ job (`services`) محتاجة ميزة لوحدها، لأن مفيش process بيعيش بعد خطوته.
- **`setup.ps1`** ماتجرّبش على Windows حقيقي (مفيش WSL2 على runners بتاعة GitHub).
- **الأمان:**
  - حد إيميلات التنبيه لكل مستخدم (فيه بس سقف 32 إرسال معلّق للسيرفر كله)، والتنبيهات تتنقل على queue الـ M7.
  - Trusted Types في الـ CSP مع M7 (F9).
  - المراجعة الخارجية في M9.
  - احتفاظ الـ audit 90 يوم.
  - GO-2026-5932 في `golang.org/x/crypto` لحد ما يطلع إصلاح.
- **فرق عن خطة M6:** الخطة بتقول إن الإضافة بتطبع آخر 4 KiB من رد السيرفر البعيد. الفعلي: `internal/deployrelease/sshremote` بيحتفظ بآخر 64 KiB في الذاكرة، والسجل بياخد آخر 4 KiB بس، وحدث التقرير اللي فوق 4 KiB بيتسيب كله (مش بيتقص). مكتوب في `docs/guides/deploy-ssh.md`.
- **التنصيب:** تجربة certbot وCloudflare وIPv6 الحقيقيين.
- **Fedora 45 في الـ CI:** الـ matrix الاتنين في `installer.yml` متثبّتين على `fedora:44` لأن PGDG لسه مانشرش PostgreSQL 18 لـ Fedora 45. أول ما `fedora-45-x86_64/repodata/repomd.xml` يرجع 200، نرجّعهم لـ `fedora:latest` أو نضيف 45، ونحدّث سطر Fedora في `docs/install/installer.md` و`docs/en/install/installer.md`.
- **الموارد:** قياس الـ sandbox بـ Podman على cgroup v2، وjob ليلي لـ `make bench` بعد ما نعرف ثبات الأرقام على أجهزة GitHub.

## طريقة الشغل اللي ماشيين بيها

- كل مرحلة بيشتغل عليها agent في worktree تحت `.claude/worktrees/`، والتحقق في الحلقة الرئيسية بـ `make check`.
- بعدها مراجعة أمنية مستقلة، والـ Critical والـ High بيتصلّحوا في نفس الـ PR.
- الـ hooks مابتشتغلش جوه `.claude/worktrees`، فقواعدها بتتراجع يدوي قبل الـ commit.
- كل agent ليه Postgres على port لوحده وملف env في مجلد خاص بيه، عشان مايكتبوش فوق بعض.
- الـ Docker daemon ممكن يحتاج يتشغّل تاني بعد إعادة تشغيل الـ container، والديسك بيتملي فـ `go clean -cache` لما مفيش agent بيبني.
