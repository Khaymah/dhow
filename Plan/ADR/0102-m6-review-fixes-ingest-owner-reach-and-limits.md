# ADR-0102: إصلاحات المراجعة المستقلة لـ M6 (M6.9هـ): صاحب توكن الاستقبال، وتأكيد البيئة المحمية، وحدود الرفع

- **الحالة:** مقبول. Sabry طلب تنفيذ الستة كلها (2026-11-07).
- **التاريخ:** 2026-11-07
- **بيكمّل:** [ADR-0098](0098-artifact-ingest-from-external-ci.md) (الاستقبال)، و[ADR-0100](0100-second-factor-for-writing-a-target-key-secret.md)، و[ADR-0077](0077-upgrades-migrations-and-update-policy.md) (الـ migration).

## السياق
مراجعة مستقلة ثابتة (من غير تشغيل) لـ `main` بعد M6 لقت واحدة Medium وخمس Low (L1 لـ L5)، وسادسة (L6) مقبولة:
- **M69e-1:** توكن الاستقبال بيفضل شغّال بعد ما صاحبه يتشال من المؤسسة أو يتنزّل دوره، لأن `FindIngestTokenByHash` بيفحص المشروع بس. توكنات الـ API مربوطة بـ `MembershipId`، لكن توكن الاستقبال لأ.
- **L1:** تأكيد البيئة المحمية (`AcknowledgeProtectedReach`) بيتطلب وقت الإنشاء بس. لو بيئة بقت محمية بعد كده (أهداف أو مراجعون أو refs مسموحة)، التوكن القديم بيوصلها بدون ما حد يعرف.
- **L2:** أخذ القفل المنتهي على السيرفر البعيد كان `RemoveAll` ثم `Mkdir`، فآخذين اتنين يقدروا يتداخلوا.
- **L3:** نص الـ hooks (`Settings.Ssh.*Command`) كان بيرجع لكل من يقرأ المشروع ولكل توكن API بنطاق قراءة.
- **L4:** الرفع المتزامن 4 عام و2 لكل مؤسسة، فمؤسستين تملّوا كل الأماكن.
- **L5:** `chargeDial` بيعدّي لما `limiter == nil`.

## القرار

### 1. صاحب التوكن (M69e-1)
1. التحقق وقت الاستعمال في `authenticate` (للرفع وللحالة): query واحد (`ListIngestCreatorAccess`) بيجيب دور العضوية المباشر وأدوار الفرق وربطات المشروع للمستخدم صاحب التوكن (`CreatedByUserId`)، وبنحسب `policy.EffectiveOf` ثم `policy.ProjectRole` ثم `policy.Allows(…, IngestManage)`. ده نفس حساب الحارس، فيغطي التخفيض وربط المشروع والفرق. المستخدم المعطّل أو المحذوف أو اللي مالوش عضوية مش مسموح.
2. الرفض: `403 INGEST_CREATOR_NOT_ALLOWED` برسالة عربي وإنجليزي (`errors.ingestCreatorNotAllowed`). بيتسجّل في `ingest.rejected` زي باقي الرفض (مرة في الدقيقة).
3. الإلغاء الفعلي عند الإزالة: موديول `Ingest` بيشترك في حدثَي `identity.member.removed` و`identity.member.left` على الـ EventBus (من غير ما يستورد `Identity`)، وبيلغي كل توكنات المستخدم في المؤسسة (`RevokeIngestTokensOfCreator`) ويسجّل `ingest_token.revoked` بسبب `creator-removed`. الحدث بيتنشر بعد الـ commit، فلو الاشتراك فشل فالفحص وقت الاستعمال هو اللي بيحمي. ومفيش إلغاء عند التخفيض: التوكن بيرجع يشتغل لو الدور رجع، وده مقصود.
4. القائمة بترجّع `CreatorAllowed` لكل توكن (query واحد لكل الصفحة)، والواجهة بتنبّه.
5. نفس الفحصين (صاحب التوكن والتأكيد) بيتعادوا جوه transaction الحفظ (`recheckToken` في `Ingest.Receive.go`، بعد قفل المشروع) بالدور الحي والتأكيد الحي، فتخفيض دور أو حماية بيئة وقت رفع بطيء بيرفض الحفظ ومفيش ملف بيتخزّن ولا تشغيل بيبدأ.
6. `RevokeCreatorTokens` بتلغي وبتسجّل التدقيق في transaction واحدة. و`ListIngestCreatorAccess` بيربط `Organizations` غير المحذوفة، زي توكنات الـ API.

### 2. تأكيد البيئة المحمية (L1)
1. عمود جديد `"IngestTokens"."AcknowledgedEnvironmentIds" uuid[] NOT NULL DEFAULT '{}'` (migration `00078`، `expand`، لسه ما اتعملش له merge فاتعدّل). بيتكتب فيه **معرّفات** البيئات المحمية اللي التوكن بيوصلها وقت الإنشاء (بعد «نعم») أو وقت التأكيد اللاحق. المفتاح هو المعرّف مش الاسم، فبيئة اتمسحت واتعملت تاني بنفس الاسم مابتورثش التأكيد.
2. وقت الرفع (`put`، بعد فحص الـ ref وقبل أي تخزين أو تشغيل) ووقت الحفظ: لو فيه بيئة محمية حية مش في المؤكَّد، الرد `403 INGEST_REACH_NOT_ACKNOWLEDGED`. **الخيار المقفول (fail closed):** الرفع بيترفض ومفيش ملف بيتخزّن ومفيش تشغيل. الرد **مايذكرش أسماء البيئات** (اسم البيئة معلومة عن المشروع مش لازم توصل لحامل توكن CI)؛ الأسماء بتظهر لمدير المشروع في القايمة وفي سجل التدقيق لحدث التأكيد.
3. نقطة `POST …/ingest-tokens/{ingestTokenId}/acknowledge-reach` بخطوة تانية وجلسة بس. الجسم فيه `Environments`: أسماء البيئات غير المؤكدة اللي الصفحة عرضتها. لو المجموعة اللي بيحسبها السيرفر دلوقتي مختلفة: `409 INGEST_REACH_CHANGED` ومفيش حاجة بتتأكد. بتكتب الوصول المحمي الحي كله (بالمعرّفات) وبتسجّل `ingest_token.reach_acknowledged` بالأسماء. توكن ملغي أو منتهي: `409`.
4. القائمة بترجّع `UnacknowledgedReach`، والواجهة بتعرضها مع زرار «أكّد» وبتبعت اللي عرضته.
5. **التوكنات الموجودة وقت الـ migration (M3):** بتبدأ بقايمة فاضية، فاللي بيوصل لبيئة محمية منها بيترفض لحد ما يتأكد. ده fail closed مقصود. مفيش تنصيبات منشورة قبل الإصدار ده (M6 لسه ماخرجش في release)، فالأثر العملي صفر. اتكتب في `Plan/Upgrades-and-Help.md` وفي `docs/install/upgrades.md` بالنسختين.

### 3. القفل (L2)
التصميم الأول (`Rename` للقفل ثم إعادته لو طلع حي) اترفض: `PosixRename` بيحل محل مجلد فاضي، فإعادة القفل ممكن تدمّر قفل حي أو تكرره. التصميم النهائي في `internal/deployrelease/preflight.go`:
1. بوابة `lock.takeover` بـ `Mkdir` (ذرية): اللي ينجح فيها بس يكمّل، والباقي `TARGET_LOCKED`.
2. جوه البوابة نعيد قراية الـ owner ونتأكد إنه منتهي فعلًا (compare-and-swap على حالة القفل). لو اتجدّد أو اختفى: ما نلمسهوش (أو نكمّل لو اختفى).
3. `Rename(lock, lock.stale-<16 hex>)` بعد ما البوابة بتمنع أي آخذ تاني (القفل الحي مابيتنقلش أبدًا لأن البوابة هي اللي بتحميه)، ثم `Mkdir` للقفل الجديد، ثم `RemoveAll` للنسخة القديمة، ثم نشيل البوابة.
4. أول `lock()` بيكنس `lock.stale-*` و`lock.takeover` الأقدم من `LockTTL` (آخذ مات في النص). ده آخر سباق متبقي: كنس بوابة ميتة بيتزامن مع آخذ جديد على نفس البوابة، ونافذته أصغر من مدة القفل بكتير.
5. الاختبارات: ثلاثة آخذين متزامنين واحد بس بيكسب، وقفل اتجدّد أثناء الأخذ مايتلمسش، وكنس القديم بس.

### 4. نص الـ hooks (L3)
`targetView` بيشيل `ReloadCommand` و`PreSwitchCommand` و`PostSwitchCommand` من الرد لو المنادي مش جلسة بـ `environment.manage` على المشروع (دور المشروع هو اللي بيحسم، زي أي صلاحية مشروع)، ويرجّع بدالها `ReloadCommandSet` و`PreSwitchCommandSet` و`PostSwitchCommandSet`. كل توكن API يتعامل كغير مدير. الحقول `readOnly` في `openapi.yaml`. الواجهة بتعرض «الأوامر موجودة لكن نصها لمسؤولي المشروع». والوثائق بتمنع أي سر في الـ hooks. نص الـ hooks لسه بيظهر في سجل الأحداث وتقارير النشر لمن يقدر يقراها، فالسر ممنوع.

### 5. حدود الرفع المتزامن (L4)
- `IngestConcurrent` من 4 إلى 8، و`IngestPerOrgInflight = IngestConcurrent / IngestOrgShareDivisor` (4) يعني 2: المؤسسة مهما عملت ماتاخدش أكتر من الربع، فمؤسستين ماتملوش الأماكن.
- جديد `IngestPerNetworkInflight = 2` لكل عنوان شبكة، بنفس دالة `ratelimit.Network(middleware.ClientIPFrom(ctx))` اللي بيستخدمها محدد الطلبات (يعني مع الوكلاء الموثوقين، و`/64` لـ IPv6، وكل العناوين غير المعروفة في حصة واحدة).
- الثوابت في `Config/Ingest.go` ومذكورة في `docs/guides/ingest.md` وأدلة الـ proxy. مفيش متغير بيئة: القيم الافتراضية آمنة وتغييرها يحتاج رؤية الذاكرة (كل رفع واقف بيمسك ملف مؤقت).

### 6. المحدد في `Environments` (L5)
`environments.NewService` بيرجّع `(*Service, error)` ويرفض `Limiter == nil` بـ `ErrNoLimiter`، و`chargeDial` لما `limiter == nil` بيرجّع `ErrNoLimiter` بدل `nil`. `App/Wire.go` بيوقف الإقلاع.

### 7. صلاحية تأكيد الوصول لبيئة محمية (قرار Sabry، مراجعة #125)
تأكيد الوصول لبيئة محمية، وقت الإنشاء (`AcknowledgeProtectedReach`) ووقت `acknowledge-reach`، بيحتاج `environment.manage` على المشروع (Owner وAdmin أو ربط Admin على المشروع). الـ Maintainer ممكن يعمل توكن بيوصل لبيئات غير محمية بس. لو طلب «نعم» من غير الصلاحية: `403 INGEST_REACH_NEEDS_ENVIRONMENT_ADMIN` (عربي وإنجليزي)، ومسار `acknowledge-reach` لسه بقاعدة `ingest.manage` في الجدول وبيفحص `environment.manage` جوه الخدمة عشان الكود يكون واضح. الواجهة بتعطّل خانة التأكيد وبتشرح، وبتخفي زرار «أكّد» وبتقول إن مسؤول المشروع هو اللي يأكد.

### 8. مقبول: L6
الـ `Sha` المعلَن في الرفع يكفي إنه في تاريخ الـ ref. صاحب توكن مسروق يقدر يعلن commit قديم من نفس الفرع. الحماية: أنماط الـ ref والتعريف من الفرع الافتراضي والبوابة والموافقة، والتحقق بالهوية في إصدار لاحق (OIDC). مكتوب في `docs/security-model.md` §14.2.

## البدائل اللي اترفضت
- **ربط التوكن بـ `MembershipId` زي توكنات الـ API:** كان هيحتاج عمود وتغيير في الـ create، ومش بيغطي تخفيض الدور أو ربط المشروع. الحساب الحي أقوى وأبسط.
- **إلغاء التوكن عند التخفيض:** كان هيعمل عدد كبير من الإلغاءات عند أي تعديل أدوار، وصاحب الدور اللي رجع محتاج توكن جديد. الرفض المؤقت أنسب.
- **تأكيد مرتبط بـ digest للإعدادات بدل الأسماء:** أعقد من اللازم، والاسم كفاية.

## النتائج
- migration `00078_ingest_token_acknowledged.sql` (expand: عمود بـ `DEFAULT`).
- أخطاء جديدة: `INGEST_CREATOR_NOT_ALLOWED` و`INGEST_REACH_NOT_ACKNOWLEDGED`، وحدث `ingest_token.reach_acknowledged`، وحقول `CreatorAllowed` و`UnacknowledgedReach` و`...CommandSet`.
- الاختبارات: `Modules/Ingest/Test/E2E/{Creator,Acknowledge,Limits}_test.go`، و`Ingest.Guard_test.go`، و`Modules/Environments/Test/E2E/Hooks_test.go`، و`Environments.Limiter_test.go`، و`internal/deployrelease/lock_takeover_test.go`، واختبارات الواجهة `IngestCreator.test.tsx` و`TargetHooks.test.tsx`.
