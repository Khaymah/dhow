# حالة داو: اللي اتعمل واللي لسه

**آخر تحديث:** 2026-11-02 · **main:** PR التحديث ده

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

شرط خلاص M2 اتحقق: اختبارات الصلاحيات خضرا، ومفيش finding مفتوح Critical أو High. والمراجعة الخارجية اتنقلت لـ M9 (ADR-0040).

## شغّال ومش متعمله merge

| الشغل | الـ branch | الـ PR | اللي فاضل |
|---|---|---|---|
| M4.4: موديول `Sources`: `internal/gitref` و`gitsource` و`netguard`، وربط Git عام بـ deploy key وhost key متثبّت بالتأكيد، و`/webhooks/git/{hookId}` بالتوقيعات الأربعة والـ dedupe و`Core/Triggers`، و`"WebhookDeliveries"` مقسّم، وworker الـ commit statuses (ADR-0072 وADR-0074) | `feat/sources-generic-git` | — | الـ PR (المراجعة الأمنية اتعملت وكل الـ findings اتصلّحت: Medium واحدة وخمسة Low) |
| M4.8: موديول `Pipelines`: `"PipelineRuns"` و`"RunJobs"` والـ DAG والـ matrix، و`Core/RunEvents` و`Core/CommitStatus`، والـ triggers (webhook وmanual وapi)، والتزامن، والـ forks بالموافقة، وتثبيت الصور بالـ tag من الـ registry، والإلغاء و«أعد تشغيل الفاشل»، والـ reconciler، و`POST /jobs` بـ `404` من غير `DHOW_DEV`، والدور على المشروع في routes الـ jobs، ومربوط بـ checkout بتاع M4.7: الـ discovery job لـ Git عام، وcheckout كل job بمفتاح المستودع وقت `Start` بس، والـ ancestry الإجباري في الـ runner، والـ outputs لـ `needs`، وأسرار `with:` للإضافات (ADR-0076) | `feat/pipelines-module` | #73 | المراجعتين الأمنيتين اتعملوا وكل الـ findings اتصلّحت، وآخرها: أرقام خطوات الأسرار بعد `if:`، وإثبات ملف الـ pipeline من كائنات git (`DEFINITION_UNVERIFIED`)، وإخفاء القيم المشتقة في تفاصيل الـ job، والـ outputs في الـ journal، والتعميق على خطوات بـ `--filter=tree:0`، ومفتاح الـ checkout للـ job الحالي بس. فاضل: الـ CI والدمج. ومتغيرات `DHOW_*` في `env` الـ job لسه مرفوضة من `jobspec` (مرحلة لاحقة) |
| `runner.revoked` (إلغاء اللوحة وإلغاء الـ runner لنفسه) بيتنشر على الـ event bus بعد الـ commit، فلو الـ process وقع بينهم الـ audit بيضيع. الـ bus مافيهوش outbox | — | — | outbox للأحداث اللي بتتسجّل، أو نشرها جوه نفس الـ transaction |

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
| M3.13 | fuzz إضافي، ومراجعة تهديدات الـ runner plane، و`deploy/m3-demo/run.sh` لشرط الخلاص |

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

**M4 شغّال:** خطته في [`M4-Pipelines.md`](M4-Pipelines.md) اتقبلت، وSabry جاوب أسئلتها كلها بالتوصية يوم 2026-10-30. **M4.1** (`internal/pipelinespec`، [ADR-0069](ADR/0069-pipeline-spec-parser-and-validator.md)) اتدمج في #65، و**M4.2** (`dhow lint`) في #66، و**M4.3** (`Projects` والـ ports، [ADR-0070](ADR/0070-ports-in-core-and-project-permissions.md)) في #68. **M4.4** (`Sources` وGit عام، [ADR-0072](ADR/0072-webhooks-and-git-credentials.md)) شغّال على `feat/sources-generic-git`. وقرار Sabry الجديد لربط Git (الـ OAuth للتجهيز بس والـ deploy keys للوصول) في [ADR-0074](ADR/0074-git-connection-oauth-for-setup-deploy-keys-for-access.md)، وفيه نقط لازم تتحسم في أول M4.6. **M4.7** (الـ runner بأكتر من حاوية والـ checkout والإضافات، [ADR-0075](ADR/0075-multi-container-attempts-checkout-and-plugins.md)) اتدمج في #74. **M4.8** (`Pipelines`، [ADR-0076](ADR/0076-pipeline-runs-definitions-and-forks.md)) مربوط بالـ checkout على `feat/pipelines-module` في #73.

**الترقيات والمساعدة جوه التطبيق:** الخطة في [`Upgrades-and-Help.md`](Upgrades-and-Help.md) و[ADR-0077](ADR/0077-upgrades-migrations-and-update-policy.md) (مقبولة، وSabry جاوب الأسئلة الـ 11 يوم 2026-11-02، وشرطه: كله ببلاش). قاعدة expand/contract شغّالة من 2026-11-02 على أي migration جديد، وU1 (الـ hook) وU2 (توافق الـ schema في الـ binary) أولوية مع M4.

## بنود مفتوحة اتأجلت

- **SELinux enforcing (ADR-0067):** الـ runner مش مدعوم على Fedora وRHEL وRocky وAlma بـ enforcing، لأن `container_t` ممنوع من `connectto` على الـ control socket. اتأجّل لبعد أول نسخة (ADR-0068)، ووقتها نختار: نقلب اتجاه قناة التحكم، أو policy module بـ domain للـ runner. وبعدها نرجّع fedora وrocky-10 لـ `vm-test.yml`.

- **الـ runner:** حقل سقف الموارد في `RegisterRequest`، ورفض `RESOURCE_REQUEST_EXCEEDS_RUNNER` وقت الإنشاء.
- **الحمل:** الـ token bucket لكل runner ورد «الـ pool مشغول» (ADR-0042). والإشعار بالـ labels لو الـ runners كتروا. وحدود الـ logs لكل instance مع HA.
- **التنضيف:** تنضيف `"RunnerSessions"` في M3.13. واحتفاظ الـ logs لكل مؤسسة في M4.
- **الهوية:** أول دخول بالـ IdP وقبول الدعوات بيه، وback-channel logout. وحذف مؤسسة.
- **مع M4:** scopes المشروع والبيئة في الـ tokens والـ bindings.
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
- **التنصيب:** ربط `make proxy-check` بالـ CI، وتجربة certbot وCloudflare وIPv6 الحقيقيين.
- **Fedora 45 في الـ CI:** الـ matrix الاتنين في `installer.yml` متثبّتين على `fedora:44` لأن PGDG لسه مانشرش PostgreSQL 18 لـ Fedora 45. أول ما `fedora-45-x86_64/repodata/repomd.xml` يرجع 200، نرجّعهم لـ `fedora:latest` أو نضيف 45، ونحدّث سطر Fedora في `docs/install/installer.md` و`docs/en/install/installer.md`.
- **الموارد:** قياس الـ sandbox بـ Podman على cgroup v2، وjob ليلي لـ `make bench` بعد ما نعرف ثبات الأرقام على أجهزة GitHub.

## طريقة الشغل اللي ماشيين بيها

- كل مرحلة بيشتغل عليها agent في worktree تحت `.claude/worktrees/`، والتحقق في الحلقة الرئيسية بـ `make check`.
- بعدها مراجعة أمنية مستقلة، والـ Critical والـ High بيتصلّحوا في نفس الـ PR.
- الـ hooks مابتشتغلش جوه `.claude/worktrees`، فقواعدها بتتراجع يدوي قبل الـ commit.
- كل agent ليه Postgres على port لوحده وملف env في مجلد خاص بيه، عشان مايكتبوش فوق بعض.
- الـ Docker daemon ممكن يحتاج يتشغّل تاني بعد إعادة تشغيل الـ container، والديسك بيتملي فـ `go clean -cache` لما مفيش agent بيبني.
