# ADR-0086: موافقات النشر والتخطّي (M5.6ب)

- **الحالة:** مقبول. القرار 18 في [خطة M5](../M5-Secrets-Environments.md#القرارات-المفوَّضة-كل-واحد-بيبقى-adr-في-أول-مرحلته) مفوَّض ومكتوب بالتوصية، و[قرار Sabry 5](../M5-Secrets-Environments.md#قرارات-sabry-2026-11-03) (الخطوة التانية مع كل موافقة، ومنع الموافقة على النفس افتراضيًا)، وقرار Sabry يوم 2026-11-05: نربط اسم الحساب على GitHub وGitLab بالمستخدم عشان نعرف مين عمل الـ push.
- **التاريخ:** 2026-11-05
- **بيكمّل:** [ADR-0085](0085-environment-gate-and-deployment-lock.md) (البوابة: الخطوة 8 بقت حقيقية)، و[ADR-0080](0080-environments-are-made-by-admins-only.md) (المراجعين و`AllowSelfApproval` و`AllowAdminBypass`)، و[ADR-0054](0054-second-factor-policy-sessions-and-credential-alerts.md) (الخطوة التانية).

## السياق
- في M5.6أ الـ job اللي على بيئة فيها مراجعين بيستنى بـ `"Gate" = 'approval'` من غير أي طريق يوافق.
- الـ push بييجي من الـ webhook باسم نص (`"PipelineRuns"."Actor"`)، و`"UserIdentities"` فيها رقم الحساب بس (`Subject`). فمفيش طريقة نعرف بيها إن المراجع هو اللي عمل الـ push.

## القرار
1. **`"DeploymentApprovals"`** (موديول `Environments`): البيئة والـ run ورقمه والـ ref و`"Round"` و`"RuleVersion"` والحالة (`pending` و`approved` و`rejected` و`expired` و`superseded` و`canceled` و`bypassed`) و`"ExpiresAt"` ومين قرّر وإمتى ونوع الخطوة (مش القيمة) والتعليق (1 KiB) وسبب التخطّي (1 KiB) و`"SupersededByRunId"`.
   - trigger بيقفل الانتقالات من `pending` بس، وبيسمح بـ `approved` أو `bypassed` لـ `superseded` بس (القاعدة اتغيّرت أو المراجع ماعادش مراجع قبل ما الـ job يبدأ). والقرار نفسه وما الموافقة عليه عمرهم ما بيتغيّروا.
   - طلب «حي» واحد (`pending` أو `approved` أو `bypassed`) لكل (run، بيئة، جولة) بـ unique index جزئي، والطلبات اللي اتقفلت بتفضل للتاريخ.
2. **الجولة:** `"PipelineRuns"."Round"` (من 1)، و«أعد تشغيل الفاشل» بيزوّده. فالموافقة على جولة قديمة مابتعدّيش jobs الجولة الجديدة.
3. **الموافقة في البوابة (الخطوة 8، جوه نفس الـ transaction):** الـ port بقى فيه `Approve` اللي بيرجّع: عدّى، أو مستني، أو اترفض، أو انتهى، أو اتبدّل بتنفيذ أحدث.
   - مفيش طلب حي: طلب `pending` جديد بـ `"ExpiresAt"` = دلوقتي + `ApprovalTimeoutHours`، و`"Gate" = 'approval'`. ولو `SupersedePending`، كل طلب `pending` أقدم على **نفس البيئة ونفس الـ ref** من run رقمه أصغر بيبقى `superseded` بـ `"SupersededByRunId"`.
   - `pending` و`"ExpiresAt"` عدّى: `expired`، والـ job بيفشل `APPROVAL_EXPIRED`.
   - `pending` على `"RuleVersion"` أقدم: `superseded` وطلب جديد.
   - `approved`: بيعدّي بس لو `"RuleVersion"` هو نفسه، والموافق لسه مراجع (مباشرة أو في فريق) وعضو ودوره Developer أو أعلى على المشروع. غير كده `superseded` وطلب جديد.
   - `bypassed` على نفس `"RuleVersion"`: بيعدّي المراجعين **والمؤقت**.
   - `rejected`: الـ job بيفشل `DEPLOYMENT_REJECTED`، واللي بعده بيتخطّى بـ `NEEDS_FAILED`.
   - `superseded` بـ `"SupersededByRunId"`: الـ job بيتلغي `SUPERSEDED`.
   - فيه موافقة واحدة لكل jobs الـ run على البيئة دي (ومعاها نسخ الـ matrix).
4. **مين يوافق أو يرفض** (`POST .../runs/{runId}/approvals/{approvalId}/approve` و`/reject`، `deployment.approve`):
   - `CurrentFactor` بخطوة تانية (TOTP أو كود طوارئ أو مفتاح مرور)، وكلمة السر لوحدها مرفوضة: `REVIEWER_NEEDS_SECOND_FACTOR`.
   - المراجع لازم يكون في `Reviewers` دلوقتي، وعضو، ودوره Developer أو أعلى: وإلا `REVIEWER_NOT_ELIGIBLE`.
   - الطلب لازم `pending` ولسه ماانتهاش: وإلا `APPROVAL_NOT_PENDING`.
   - **«نفسه»** لو `AllowSelfApproval` مقفول (الافتراضي): `SELF_REVIEW_NOT_ALLOWED`. «نفسه» هو:
     - اللي شغّل التنفيذ يدوي أو بالـ API (`"TriggeredByUserId"`، وده صاحب التوكن لو بتوكن)؛
     - أو المستخدم اللي عنده identity اسمها (`"UserIdentities"."Login"`) هو `"Actor"` بتاع الـ push، من غير فرق في الحروف، و`Issuer` بتاعها على نفس الـ host بتاع المستودع.
   - **الاسم:** بيتحفظ وقت الدخول: `login` من GitHub، و`preferred_username` من OIDC (زي GitLab). المستخدم اللي عمره مادخلش بالحساب ده مش مربوط، والطلب بيقول كده جنب اسم الـ pusher. وفي Git العام الـ pusher نص من الـ webhook ومش مربوط بحد.
5. **التخطّي** (`POST .../bypass`، `environment.bypass`، Owner وAdmin بس): لازم `AllowAdminBypass` (وإلا `BYPASS_NOT_ALLOWED`)، و`Reason` إجباري، و`CurrentFactor`. بيعدّي المراجعين والمؤقت بس. قاعدة الـ ref والقواعد الثابتة والإيقاف والـ forks مافيهاش تخطّي.
6. **بعد القرار:** `Environments` بيكتب القرار والـ audit في transaction لوحدها، وبعد الـ commit بينادي `Reopen` (بيركّبه `App` من `Pipelines`) عشان الـ job يتدرج على طول. ولو ده فشل، الـ reconciler بيعيد البوابة كل 5 ثواني. ده بيتجنّب قفل صف الموافقة قبل صف الـ run (عكس ترتيب البوابة).
7. **السباقات:** القرار بيقفل صف الموافقة `FOR UPDATE` ويتأكد إنه لسه `pending`، فـ 50 موافقة مع بعض واحدة بس بتنجح. والإدراج نفسه تحت قفل الـ run زي M5.6أ، فالـ job بيتدرج مرة واحدة.
8. **القوايم والعدّاد:**
   - `GET .../runs/{runId}/approvals` بـ `run.read`.
   - `GET /organizations/{id}/deployment-approvals?State=pending&Mine=true` بـ `run.read` على مستوى المؤسسة، ومفلترة بالمشاريع اللي المستخدم يشوفها. و`Mine` = اللي هو مراجع فيها ويقدر يوافق.
   - `PendingApprovals` في `GET /organizations/{id}/overview` ([ADR-0061](0061-organization-overview-endpoint.md))، للمستخدم نفسه.
9. **الـ audit:** `deployment.approval_requested` و`deployment.approval_expired` و`deployment.started` و`deployment.finished` من `Pipelines` جوه transaction البوابة، و`deployment.approved` و`deployment.rejected` و`environment.bypassed` من `Environments` جوه transaction القرار. بالمعرّفات والحالة ونوع الخطوة، من غير التعليق ولا السبب لو أطول من حد الـ audit.
10. **`"Deployments"."ApprovalId"`** بيتملا وقت `Started`، فسجل النشر بيقول مين وافق أو تخطّى.

## البدائل اللي اترفضت
- **«نفسه» = اللي شغّل يدوي بس:** اللي عمل push على `main` كان هيقدر يوافق على نشر شغله (Sabry اختار الربط).
- **المطابقة ببريد صاحب الـ commit:** أي حد يقدر يكتب أي بريد في الـ commit.
- **إعادة البوابة جوه transaction القرار:** ترتيب الأقفال هيبقى الموافقة ثم الـ run، عكس البوابة، فممكن deadlock.
- **إرجاع الموافقة لـ `pending` لما المراجع يتشال:** الـ trigger بيقفل الانتقال من القرار، والطلب الجديد أوضح في السجل.

## العواقب
- البيئة اللي فيها مراجعين بتنشر بعد موافقة واحدة (Community، قرار Sabry 2). أكتر من مراجع مع M8.
- `UserIdentities.Login` بيتملا للمستخدمين اللي يدخلوا بعد النسخة دي بس، والقدامى بيتربطوا أول ما يدخلوا تاني.
