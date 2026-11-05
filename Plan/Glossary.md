# مصطلحات الواجهة

المصطلح العربي هو اللي بيظهر في الواجهة، وبالعربي المبسّط ([ADR-0062](ADR/0062-simple-arabic-for-users.md)). الإنجليزي بيظهر في التلميح أو بين قوسين أول مرة بس ([ADR-0034](ADR/0034-ui-direction-calm-and-organized.md)).

| English | في الواجهة |
|---|---|
| Passkey | مفتاح المرور |
| TOTP / authenticator | كود من تطبيق المصادقة |
| Recovery codes | أكواد الطوارئ |
| API token | مفتاح API |
| Runner | جهاز تشغيل |
| Registration token | رمز التسجيل |
| Label | وسم |
| Capacity | السعة |
| Pause / Resume (runner) | إيقاف مؤقت / إعادة التشغيل |
| Revoke (runner) | إلغاء نهائي |
| Session | جهاز متصل / جلسة |
| Sign out everywhere | اخرج من كل الأجهزة الأخرى |
| Organization | المؤسسة |
| Team | الفريق |
| Member | عضو |
| Invitation | دعوة |
| Owner | صاحب المؤسسة |
| Admin | مدير |
| Maintainer | مسؤول مشاريع |
| Developer | مطوّر |
| Viewer | مشاهد |
| Audit log | سجل الأحداث |
| Job | تشغيل (الجمع: التشغيلات). وتشغيل النظام اللي بيقرا الملف اسمه «قراءة ملف خطوات النشر» |
| Delivery (sidebar group) | النشر: المجموعة اللي فيها المشاريع والتشغيلات والأجهزة، عشان «التشغيل» مايتلخبطش مع «التنفيذ» |
| Attempt | محاولة |
| Fencing token | رقم الحجز |
| Label | وسم (الجمع: الأوسمة) |
| Job log | سجل التشغيل |
| Deployment | نشر |
| Environment | بيئة |
| Pipeline | خطوات النشر (pipeline) |
| Run (pipeline run) | تنفيذ (الجمع: التنفيذات). التنفيذ الواحد لخطوات النشر، وكل job جواه «تشغيل» |
| Repository | مستودع |
| Project | مشروع (الجمع: المشاريع) |
| Link / Unlink (repository) | اربط / افصل المستودع |
| Deploy key | مفتاح المستودع |
| Host key | مفتاح سيرفر المستودع (البصمة: fingerprint) |
| Webhook | الإشعارات (webhook)، والرابط «رابط الإشعارات» والسر «سر الإشعارات» |
| Secret | سر (القيمة لا تظهر بعد الحفظ أبدًا) |
| Secret version | إصدار (السر) |
| Variable | متغيّر (قيمته ظاهرة لكل من يرى المشروع) |
| Environment rules | قواعد الحماية (للبيئة): الفروع المسموحة، والمراجعون، ومؤقت الانتظار |
| Reviewer | مراجع (يوافق على النشر في بيئة، موافقة واحدة تكفي) |
| Wait timer | مؤقت الانتظار (بالدقائق، قبل أن يبدأ النشر) |
| Deploy approval | موافقة النشر (طلب واحد لكل تنفيذ وبيئة، ويُحسم بالموافقة أو الرفض أو التخطي أو انتهاء المهلة) |
| Approve and deploy | وافق وانشر |
| Bypass the rules | تخطي القواعد (للمدير وصاحب المؤسسة، بسبب وخطوة ثانية، ويتخطى المراجعين والمؤقت فقط) |
| Environment busy | البيئة مشغولة (ينتظر تنفيذًا آخر ينهي عمله عليها) |
| Admin bypass | تخطي القواعد (المدير يتخطى المراجعين والمؤقت بسبب وخطوة ثانية، ولا يتخطى الفروع ولا الإيقاف) |
| Deployment log | سجل النشر (كل تشغيل عبر بوابة البيئة، من الأحدث) |
| Pause / Resume deploys | إيقاف النشر / استئناف النشر (للبيئة، غير إيقاف جهاز التشغيل) |
| Registry credential | حساب سجل الصور (اسم السجل واسم المستخدم وكلمة المرور) |
| Override (secret) | «يغطي على سر المؤسسة» |
| Available to pull requests | متاح لطلبات الدمج |
| Selected projects | مشاريع مختارة |
| Rollback | ارجع إلى النسخة السابقة |
| Approval | موافقة |
| Workspace / Home | الرئيسية |
| Settings | الإعدادات |
| Appearance | المظهر والألوان |
| Instance owner | صاحب النسخة |
| Control plane | لوحة التحكم (control plane) |
| Sandbox | البيئة المعزولة |
| Container | حاوية |
| Reverse proxy | وكيل عكسي |
| Trusted proxies | الوكلاء الموثوقون |
| Rate limits | حدود الطلبات |
| Artifact | الملف الناتج (الجمع: الملفات الناتجة)، وغير الموثوق «غير موثوق» |
| Cache | الذاكرة المؤقتة، ومحتواها لا يظهر في الواجهة |
| Fork | نسخة منفصلة |
| Fork approval | الموافقة على تنفيذ من نسخة منفصلة («وافق على التنفيذ») |
| Stage (run) | مرحلة |
| Manual run | تنفيذ يدوي («ابدأ تنفيذًا يدويًا») |
| Rerun failed | «أعد تشغيل ما فشل» |
| Push / Tag (trigger) | دفع تعديل / وسم جديد |
| Log retention | مدة حفظ سجلات التشغيل |
| Lease (job) | حجز |
| Heartbeat | نبضة |
| Issuer (OIDC) | المُصدِر |
| Migrations | تحديثات قاعدة البيانات |
