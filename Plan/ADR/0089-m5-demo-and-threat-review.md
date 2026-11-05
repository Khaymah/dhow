# ADR-0089: الـ demo الأخير لـ M5 ومراجعة التهديدات (M5.9ب)

- **الحالة:** مقبول. [خطة M5](../M5-Secrets-Environments.md) §M5.9 (شرط الخلاص بدون OIDC).
- **التاريخ:** 2026-11-05
- **بيكمّل:** [ADR-0087](0087-browser-end-to-end-tests-in-ci.md) (نفس ملفات `m4-demo` والـ job بتاع `demo.yml`)، و[ADR-0088](0088-contract-the-old-sealed-format.md) (فحص التسريب بأسرار كل النطاقات)، و[ADR-0081](0081-keyring-and-envelope-encryption.md) (التدوير) و[ADR-0086](0086-deployment-approvals.md) (الموافقات).

## السياق
- M5.1 لـ M5.7 خلصوا، وM5.9أ قفل صيغة `0x01`. الباقي من M5.9 إثبات إن المسار كله شغّال على stack حقيقي، ومراجعة التهديدات اللي الخطة طلبتها، وقفل المرحلة.
- شرط خلاص M5 في الخطة: `deploy/m5-demo/run.sh` يعدّي في `demo.yml` على `ubuntu-26.04`، وفحص التسريب يلاقي المزروع ومايلاقيش غيره، ومفيش finding مفتوح Critical أو High.
- الخطة كتبت «إعادة اللف» كأنها أمر (`rewrap`)، لكن في ADR-0081 هي worker في الخلفية كل 30 ثانية، ومفيش أمر بالاسم ده. الـ demo بيستنى `keys status`.

## القرار
1. **`deploy/m5-demo/run.sh`** (مع `lib.sh`) فوق `deploy/m4-demo/lib.sh` و`project.sh`، بنفس طريقة `deploy/e2e/run.sh`: نفس ملفات الـ compose ونفس صورة السحب والـ runner الـ Podman، وبورتات وشبكة لوحدهم (8500 و55500 و55501 و`10.231.100.0/24` واسم المشروع `dhow-m5`). كله بالـ API وبنفس `api` و`forgejo_api`، والأسرار على stdin أو في ملفات مش في الـ argv.
2. **المسار:**
   - مراجع بدعوة (`POST /invitations` ثم `/invitations/accept`) بدور Developer، وبيشغّل TOTP (`/me/totp` ثم `/me/totp/confirm`) ويدخل بالخطوة التانية (`/sessions` ثم `/sessions/mfa`).
   - بيئة `production` بـ `main` بس والمراجع ده، وسر في كل نطاق (`sealed_values` بقت بتاخد اسم البيئة ومابتعملهاش لو الـ demo عملها)، وكلمة سر registry.
   - pipeline فيه job على `production` بياخد الأسرار التلاتة وبيحتفظ بـ artifact.
   - push على `feature/demo`: الـ job `skipped` بـ `ENVIRONMENT_REF_NOT_ALLOWED` ومن غير بوابة.
   - push على `main`: الـ job عند بوابة `approval` والـ run `running` (مش `awaiting_approval` بتاع الـ forks). الـ Owner مش مراجع فبيترفض `REVIEWER_NOT_ELIGIBLE` (422)، والمراجع بيوافق بكود TOTP، والـ run بيعدّي، والـ job شاف الأسرار التلاتة (أطوالها بس في اللوج)، والـ artifact اتحفظ، وصف `"Deployments"` `succeeded`.
   - الموافقة الذاتية: الـ Owner بيتضاف للمراجعين، ويشغّل run يدوي على `main`، وموافقته بتترفض `SELF_REVIEW_NOT_ALLOWED` (403)، وبعدها بيلغي الـ run. في Git العام الـ pusher نص مش مربوط بحد (ADR-0086)، فالتشغيل اليدوي هو اللي بيثبّت الرفض.
   - تدوير الـ KEK في النص: `keys generate` في حاوية السيرفر على نسخة الملف بتاعها، والناتج بيترجّع لـ `$work/server/keyring` (الملف اللي السيرفر بيتنسخ منه كل إقلاع)، وإعادة تشغيل السيرفر، و`keys activate 2` بإعادة محاولة لحد ما الـ heartbeat بتاع النسخة القديمة يعدّي، وانتظار `keys status` لحد «Data keys to re-wrap: 0» والإصدار 1 `retired` والإصدار 2 `active` وعليه مفاتيح.
   - push تاني على `main` بعد التدوير: بوابة، وموافقة المراجع بكود جديد، ونجاح بنفس الأسرار.
   - فحص التسريب في الآخر (`leak-sweep.sh`)، والـ needles فيها أسرار TOTP وكلمة سر المراجع وتوكن الدعوة والكوكيز.
3. **`deploy/m5-demo/totp`:** برنامج Go صغير (زي `m4-demo/hmac`) بيشتغل في حاوية `tools`: السر base32 على stdin، وملف حالة بآخر step اتستخدم. كل نداء بيطلّع كود بـ step أكبر من اللي قبله (السيرفر بيرفض إعادة استخدام step)، وبيستنى لو الـ step اللي بعده برّه نافذة ±1، فمفيش sleep ثابت في الـ script. مفيش سر في الـ argv.
4. **الـ CI:** `m5-demo` في مصفوفة `demo` في `demo.yml`، و`deploy/m5-demo/**` في الـ `paths`، و`deploy/ci/demo-areas.sh` بيشغّل الـ demos لما ملفاته تتغيّر (والـ e2e لأ، لأنه مابيستخدمهاش). وفحص الـ argv (`secrets-argv-test.sh`) بقى بيقرا `m5-demo/run.sh` و`lib.sh`.
5. **مراجعة التهديدات:** `docs/security-model.md` §13 بالنسختين: جدول التهديد والضابط والاختبار بالمسار (15 صف)، والمخاطر المقبولة (التسريب بتحويل القيمة، والـ pusher في Git العام، والملفات المضغوطة جوه الـ artifacts، وtraces الـ e2e، و`00066` من غير transaction، والتدوير والنسخ القديمة، وOIDC)، وفحص التسريب بعد التدوير، وإضافات لقايمة المراجعة الخارجية. كل مسار اختبار في الجدول اتأكد إنه موجود. و`docs/roadmap.md` بالنسختين: M5 خلصت ما عدا M5.8 (OIDC) بعد أول نسخة.

## البدائل اللي اترفضت
- **الموافقة الذاتية بـ push بدل التشغيل اليدوي:** في Git العام مفيش ربط بين الـ pusher وحساب في داو، فالرفض مايتجربش. التشغيل اليدوي بيثبّت `"TriggeredByUserId"`.
- **أمر `keys rewrap` جديد:** الـ worker موجود وبيكمّل بعد أي إعادة تشغيل. أمر تاني يعمل نفس الشغل من الـ CLI كان هيتعارض مع القفل (advisory lock) من غير فايدة.
- **توليد الـ KEK الجديد من الـ host:** الملف عند السيرفر نسخة (`install -m 600`) من `$work/server/keyring`. توليده بنفس الـ binary في حاوية السيرفر وبعدين ترجيعه للمصدر بيجرّب نفس الكود اللي هيشتغل عند العميل.
- **sleep ثابت بين أكواد TOTP:** بطيء ومش مضمون. ملف الحالة بيخلّي كل كود بـ step أكبر ويستنى بس لما محتاج.
- **تشغيل الـ demo على Debian 13 ويدوي:** شرط الخلاص بيطلب VM نضيفة بـ Debian 13 وUbuntu 26.04 (`demo.yml` على `ubuntu-26.04`). VM الـ Debian بتتعمل من الحزمة اللي في `deploy/test-vm`، وخارج الـ PR ده.

## النتايج
- كل PR بيلمس السيرفر أو الـ runner أو ملفات الـ demos بيشغّل `m5-demo` كمان (حوالي 25 دقيقة)، والليلي برضه.
- `sealed_values` بقت بتاخد `sealed_environment` و`sealed_creates_environment` (الافتراضي القديم: `staging` وبيتعمل).
- أي تغيير في أوامر `keys` أو في رسالة `keys status` بيتصلّح مع `lib.sh` (`data_keys_rewrapped` بتقرا نص الجدول).

## المراجعة الأمنية
- مفيش Critical ولا High.
- **(LOW، مقبول)** الـ demo بيمرّر `DHOW_DATABASE_URL` بكلمة سر قاعدة الـ dev (`dhow-dev-only`، ثابتة ومعروفة وبتظهر في `leak-sweep.sh` كمان) في `docker exec -e`. ده stack بيتمسح في نفس التشغيل، ومفيش سر حقيقي.
- **(LOW، مقبول)** `$work/server/keyring` بيتبدّل بملف جديد (mode 600) بعد `generate`. لو الـ demo وقف بين الكتابة والنقل فيه `keyring.next` متروك في مجلد مؤقت بيتمسح مع الـ stack.
- **(LOW، مقبول)** أسرار TOTP بتفضل في متغيرات الـ shell وفي ملف needles بمجلد `0700`، ومابتتكتبش في أي argv ولا log. الفحص بيدوّر عليها في كل مكان.
