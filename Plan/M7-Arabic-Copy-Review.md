# مراجعة النصوص العربية في M7

> قائمة لـ Sabry تعلّم عليها. كل نص عربي أُضيف في M7 (من M7.0 إلى M7.9، بعد دمج خطة M7 في [#126](https://github.com/Khaymah/dhow/pull/126)) مع أصله الإنجليزي والمصطلح الذي يجب أن يستخدمه من [`Glossary.md`](Glossary.md). المرجع: [ADR-0062](ADR/0062-simple-arabic-for-users.md).

**الحالة:** مفتوحة، تنتظر مراجعة إنسان.

| | |
|---|---|
| نصوص الواجهة المضافة | 353 (347 مضافة و6 معدّلة) |
| رسائل السيرفر المضافة | 174 |
| المراجع | ____________ |
| التاريخ | ____________ |
| الاستنتاج | ☐ مقبولة ☐ مقبولة بعد التعديلات المعلّمة |

## كيف تعلّم

- في عمود «قرارك»: علّم «سليم»، أو علّم «يُعدَّل» واكتب البديل. لا تعدّل الجدول نفسه.
- عمود «فحص» فيه ✔ إذا لم يجد الفحص الآلي شيئًا. الفحص الآلي يقارن مصطلحات المعجم وطول الجملة (أكثر من 15 كلمة)، والعامية تمنعها الـ hook أصلًا. هو لا يحل محل قراءتك.
- الأمثلة في المعجم لها الأولوية. إذا رأيت أن المعجم نفسه يحتاج تعديلًا، اكتبه في جدول «مصطلحات ناقصة» أسفل.

## تصحيحات أجريناها (بثقة، لأنها تخالف المعجم صراحة)

| الموضع | قبل | بعد | السبب |
|---|---|---|---|
| `notifications.event.run.finished، notifications.inbox.run.finished، notify.title.run.finished` | فشل تشغيل خط النشر | فشل تنفيذ خطوات النشر | المعجم: Pipeline = خطوات النشر، وRun = تنفيذ |
| `notifications.inbox.field.run، notify.field.run` | رقم التشغيل | رقم التنفيذ | المعجم: التنفيذ (Run) للتنفيذ الواحد لخطوات النشر، والتشغيل (Job) لوحدة داخله |
| `notify.title.runner.offline` | المنفّذ لا يستجيب | جهاز تشغيل لا يستجيب | المعجم: Runner = جهاز تشغيل، ونص الواجهة المقابل كان صحيحًا |
| `notify.field.runner` | المنفّذ | جهاز التشغيل | المعجم: Runner = جهاز تشغيل |
| `notifications.inbox.change.recovery_codes_regenerated` | صُنعت رموز استرداد جديدة | أُنشئت أكواد طوارئ جديدة | المعجم: Recovery codes = أكواد الطوارئ |
| `help.hint.projects` | مشاريعك وبيئاتها وتشغيلاتها. | مشاريعك وبيئاتها وتنفيذاتها. | المعجم: التنفيذ لا التشغيل (الأصل: runs) |
| `errors.channelHostNotAllowed` | واكتب الخادم باسمه | واكتب السيرفر باسمه | ADR-0062: «السيرفر» بدل «الخادم» |
| `INVALID_MFA_CODE، REVIEWER_NEEDS_SECOND_FACTOR (Fix)` | رمز استرداد | كود طوارئ | المعجم: Recovery codes = أكواد الطوارئ |
| `SESSION_REQUIRED، API_TOKEN_LIMIT (Fix)` | رمز الـ API، رمز API | مفتاح API | المعجم: API token = مفتاح API |
| `RUNNER_REVOKED (Fix)` | سجّل runner جديدًا برمز تسجيل جديد. الـ runner الملغى لا يعود. | سجّل جهاز تشغيل جديدًا برمز تسجيل جديد. جهاز التشغيل الملغى نهائيًا لا يعود. | المعجم: Runner = جهاز تشغيل |
| `ATTEMPT_NOT_FOUND (Fix)` | حتى يأخذ runner المهمة ... تأكد أن runner متصل | حتى يأخذ جهاز تشغيل هذا التشغيل ... تأكد أن جهاز التشغيل متصل | المعجم: Runner = جهاز تشغيل، والمعجم: Job = تشغيل (وليس مهمة) |
| `SECRETS_UNAVAILABLE، JOB_QUEUE_FULL (Fix)` | مواصفة المهمة، المهام | مواصفة التشغيل، التشغيلات | المعجم: Job = تشغيل (وليس مهمة) |
| `FORBIDDEN، ROLE_NOT_ALLOWED، LAST_OWNER، FORK_POLICY_NOT_ALLOWED (Fix)` | مالك المؤسسة، مالك، المالك | صاحب المؤسسة | المعجم: Owner = صاحب المؤسسة، وInstance owner = صاحب النسخة |
| `SETUP_COMPLETED (Fix)` | بحساب المالك | بحساب صاحب النسخة | المعجم: Owner = صاحب المؤسسة، وInstance owner = صاحب النسخة |
| `RUN_NOT_RERUNNABLE، RUN_NOT_AWAITING_APPROVAL، ARTIFACT_GONE (Fix)` | التشغيل | التنفيذ | المعجم: التنفيذ (Run) للتنفيذ الواحد لخطوات النشر، والتشغيل (Job) لوحدة داخله |
| `OIDC_PROVIDER_LINKED (Fix)` | بالنوع أو الجهة الجديدة | بالنوع أو المُصدِر الجديد | المعجم: Issuer (OIDC) = المُصدِر |
| `DEPLOY_KEY_NOT_APPLICABLE (Fix)` | لاستخدام مفتاح نشر | لاستخدام مفتاح المستودع | المعجم: Deploy key = مفتاح المستودع |

بقية النصوص في الجداول أدناه كما هي بعد هذه التصحيحات.

## مصطلحات ناقصة في المعجم

هذه نصوص جديدة لا مقابل لها في `Glossary.md`. القرار لك: نضيفها بالصيغة المستعملة، أو نغيّر الصيغة.

- Channel (قناة إشعارات): غير موجودة في المعجم.
- Theme (الثيم): مستعملة في كل الواجهة لكنها غير موجودة في المعجم.
- Ingest token (رمز استقبال): غير موجودة في المعجم، وتظهر في رسائل الحل.
- Bot token (توكن البوت): غير موجودة في المعجم.
- Delivery log (سجل التسليم): غير موجودة في المعجم.
- Rotate secret (سر جديد): غير موجودة في المعجم.

## نصوص الواجهة

المفاتيح من `apps/web/Src/I18n/Locales/`. الإنجليزي هو النص المقابل بنفس المفتاح.

### الإشعارات (M7.2 وM7.3)

الملف: `Workspace/NotificationsAr.ts`

| # | المفتاح | العربي | English | المصطلح الواجب | فحص | قرارك |
|---|---|---|---|---|---|---|
| 1 | `projects.nav.notifications` | الإشعارات | Notifications | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 2 | `shell.notifications.unread` | افتح الإشعارات، وغير المقروء: | Open notifications, unread: | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 3 | `notifications.project.lead` | قنوات تصل بها أحداث هذا المشروع إلى فريقك: النشر والموافقات وأحداث الحساب. لكل قناة قواعد تحدد ما يصل إليها. | Channels that carry the events of this project to your team: deployments, approvals and account events. Each channel has rules that decide what reaches it. | Deployment ← نشر، Project ← مشروع (الجمع: المشاريع)، Team ← الفريق، Approval ← موافقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 4 | `notifications.readOnly` | تقرأ هنا القنوات وقواعدها فقط. من يدير المشروع ينشئ القنوات ويغيّرها. | You can read the channels and their rules here. Whoever manages the project creates and changes channels. | Project ← مشروع (الجمع: المشاريع) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 5 | `notifications.forbidden.manage` | من يدير المشروع وحده ينشئ قنوات الإشعارات. | Only whoever manages the project creates notification channels. | Project ← مشروع (الجمع: المشاريع) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 6 | `notifications.channels.group` | القنوات | Channels | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 7 | `notifications.channels.title` | قنوات الإشعارات | Notification channels | — | «القناة» مصطلح جديد غير موجود في المعجم. اقتراح: نضيف Channel = قناة (إشعارات). | ☐ سليم ☐ يُعدَّل: |
| 8 | `notifications.channels.lead` | كل قناة تحمل رسائل المشروع إلى مكان واحد. عنوان القناة وسرّها لا يظهران بعد الحفظ. | Each channel carries the messages of the project to one place. The address and the secret of a channel are never shown after saving. | Project ← مشروع (الجمع: المشاريع)، Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 9 | `notifications.channels.loading` | جارٍ تحميل القنوات | Loading the channels | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 10 | `notifications.channels.error` | لم نتمكن من تحميل القنوات | We could not load the channels | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 11 | `notifications.channels.empty` | لا توجد قنوات بعد. أنشئ قناة ليصلك إشعار عند فشل النشر. | There are no channels yet. Create one to be told when a deployment fails. | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 12 | `notifications.channels.new` | قناة جديدة | New channel | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 13 | `notifications.kind.webhook` | رابط إشعارات | Notification URL | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 14 | `notifications.kind.email` | بريد إلكتروني | Email | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 15 | `notifications.kind.inapp` | صندوق الإشعارات | Notification inbox | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 16 | `notifications.kind.slack` | قناة Slack | Slack | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 17 | `notifications.kind.discord` | قناة Discord | Discord | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 18 | `notifications.kind.telegram` | قناة Telegram | Telegram | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 19 | `notifications.kind.webhook.about` | يرسل داو الحدث إلى رابطك موقّعًا بسر. تظهر قيمة السر مرة واحدة. | Dhow sends the event to your URL, signed with a secret. The secret is shown once. | Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 20 | `notifications.kind.email.about` | يصل البريد إلى الأعضاء الذين يحملون الأدوار التي تختارها. | The email goes to the members who hold the roles you choose. | Member ← عضو | ✔ | ☐ سليم ☐ يُعدَّل: |
| 21 | `notifications.kind.inapp.about` | يصل الإشعار إلى صندوق الإشعارات داخل داو للأعضاء الذين يحملون الأدوار التي تختارها. | The notification reaches the inbox inside Dhow of the members who hold the roles you choose. | Member ← عضو | ✔ | ☐ سليم ☐ يُعدَّل: |
| 22 | `notifications.kind.slack.about` | رابط الإشعارات الوارد من Slack لقناة واحدة. | The Slack incoming webhook URL of one channel. | Webhook ← الإشعارات (webhook) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 23 | `notifications.kind.discord.about` | رابط الإشعارات من Discord للقناة التي تريدها. | The Discord webhook URL of the channel you want. | Webhook ← الإشعارات (webhook) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 24 | `notifications.kind.telegram.about` | توكن بوت من Telegram ومعرّف المحادثة. | The token of a Telegram bot and the id of the chat. | — | توكن: نقل حرفي. الأقرب للمعجم «رمز البوت» (رمز = token كما في «رمز التسجيل»). | ☐ سليم ☐ يُعدَّل: |
| 25 | `notifications.new.title` | قناة إشعارات جديدة | New notification channel | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 26 | `notifications.new.subtitle` | اختر النوع والاسم، ثم أكّد هويتك. | Choose the kind and the name, then confirm who you are. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 27 | `notifications.form.kind` | نوع القناة | Kind of channel | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 28 | `notifications.form.name` | اسم القناة | Channel name | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 29 | `notifications.form.locale` | لغة الرسائل | Language of the messages | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 30 | `notifications.form.url` | رابط القناة | Channel URL | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 31 | `notifications.form.bot` | توكن البوت | Bot token | — | توكن: نقل حرفي. الأقرب للمعجم «رمز البوت». | ☐ سليم ☐ يُعدَّل: |
| 32 | `notifications.form.chat` | معرّف المحادثة | Chat id | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 33 | `notifications.form.submit` | أنشئ القناة | Create the channel | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 34 | `notifications.form.submitting` | جارٍ إنشاء القناة | Creating the channel | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 35 | `notifications.form.noProject` | افتح هذه الشريحة من صفحة إشعارات مشروع. | Open this panel from the notifications page of a project. | Project ← مشروع (الجمع: المشاريع) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 36 | `notifications.form.nameInvalid` | اكتب اسمًا للقناة لا يزيد على 64 حرفًا. | Write a channel name of at most 64 characters. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 37 | `notifications.form.urlInvalid` | اكتب رابطًا يبدأ بـ HTTPS. | Write a URL that starts with https. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 38 | `notifications.form.botInvalid` | اكتب توكن البوت. | Write the bot token. | — | توكن: نقل حرفي. الأقرب للمعجم «رمز البوت». | ☐ سليم ☐ يُعدَّل: |
| 39 | `notifications.form.chatInvalid` | اكتب معرّف المحادثة. | Write the chat id. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 40 | `notifications.factor.about` | إنشاء قناة يغيّر وجهة بيانات المشروع. اكتب خطوتك الثانية لتأكيده. | A new channel changes where project data goes. Type your second step to confirm it. | Project ← مشروع (الجمع: المشاريع) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 41 | `notifications.secret.title` | انسخ سر التوقيع الآن | Copy the signing secret now | Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 42 | `notifications.secret.lead` | يظهر هذا السر مرة واحدة فقط. لا يستطيع داو عرضه مرة أخرى. إن فقدته، اصنع سرًا جديدًا. | This secret is shown only once. Dhow cannot show it again. If you lose it, make a new one. | Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 43 | `notifications.secret.value` | سر التوقيع | Signing secret | Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 44 | `notifications.secret.copy` | انسخ السر | Copy the secret | Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 45 | `notifications.secret.copied` | نُسخ السر | Secret copied | Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 46 | `notifications.secret.copyFailed` | لم ينجح النسخ. حدّد السر وانسخه يدويًا. | Copy failed. Select the secret and copy it by hand. | Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 47 | `notifications.secret.done` | انتهيت | I am done | — | زر بكلمة حالة. اقتراح: «أغلق» أو «حفظت السر». | ☐ سليم ☐ يُعدَّل: |
| 48 | `notifications.secret.usage` | يتحقق المستقبل من توقيع كل رسالة بهذا السر. اقرأ دليل الإشعارات للتفاصيل. | The receiver checks the Dhow-Signature header with this secret. Read the notifications guide for details. | Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 49 | `notifications.state.active` | تعمل | Working | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 50 | `notifications.state.failures` | توقفت بعد فشل متكرر | Stopped after repeated failures | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 51 | `notifications.state.creatorRemoved` | توقفت لأن منشئها لم يعد يديرها | Stopped because its creator no longer manages it | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 52 | `notifications.rules.button` | القواعد | Rules | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 53 | `notifications.rules.open` | القواعد: {name} | Rules of channel {name} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 54 | `notifications.rules.title` | قواعد القناة {name} | Rules of channel {name} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 55 | `notifications.rules.readOnly` | لا تملك صلاحية تغيير القواعد. | You cannot change the rules. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 56 | `notifications.rules.events` | الأحداث | Events | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 57 | `notifications.rules.statuses` | الحالات | Statuses | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 58 | `notifications.rules.environments` | البيئات | Environments | Environment ← بيئة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 59 | `notifications.rules.environments.hint` | افصل الأسماء بفاصلة. اتركه فارغًا لكل البيئات. | Separate the names with a comma. Leave it empty for every environment. | Environment ← بيئة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 60 | `notifications.rules.roles` | الأدوار التي تستلم | Roles that receive it | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 61 | `notifications.rules.save` | احفظ القواعد | Save the rules | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 62 | `notifications.rules.saving` | جارٍ الحفظ | Saving | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 63 | `notifications.rules.close` | أغلق | Close | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 64 | `notifications.rules.saved` | حُفظت قواعد القناة {name} | Saved the rules of channel {name} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 65 | `notifications.rules.eventsEmpty` | اختر حدثًا واحدًا على الأقل. | Choose at least one event. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 66 | `notifications.rules.statusesEmpty` | اختر حالة واحدة على الأقل. | Choose at least one status. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 67 | `notifications.rules.rolesEmpty` | اختر دورًا واحدًا على الأقل. | Choose at least one role. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 68 | `notifications.rules.environmentsInvalid` | اسم بيئة غير صالح. استخدم حروفًا صغيرة وأرقامًا وشرطات. | Not a valid environment name. Use lowercase letters, digits and dashes. | Environment ← بيئة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 69 | `notifications.event.deployment.finished` | انتهاء النشر | A deployment finished | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 70 | `notifications.event.deployment.approval.requested` | نشر ينتظر موافقة | A deployment waits for approval | Deployment ← نشر، Approval ← موافقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 71 | `notifications.event.run.finished` | فشل تنفيذ خطوات النشر | A pipeline run failed | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 72 | `notifications.event.runner.offline` | جهاز تشغيل لا يستجيب | A runner stopped answering | Runner ← جهاز تشغيل | ✔ | ☐ سليم ☐ يُعدَّل: |
| 73 | `notifications.event.account.new_device_login` | دخول من جهاز جديد | Sign-in from a new device | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 74 | `notifications.event.account.second_factor_changed` | تغيّر الخطوة الثانية لحساب | The second step of an account changed | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 75 | `notifications.event.member.removed` | إزالة عضو | A member was removed | Member ← عضو | ✔ | ☐ سليم ☐ يُعدَّل: |
| 76 | `notifications.event.member.role_changed` | تغيّر دور عضو | The role of a member changed | Member ← عضو | ✔ | ☐ سليم ☐ يُعدَّل: |
| 77 | `notifications.event.test` | رسالة اختبار | Test message | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 78 | `notifications.status.succeeded` | نجح | Succeeded | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 79 | `notifications.status.failed` | فشل | Failed | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 80 | `notifications.status.canceled` | أُلغي | Canceled | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 81 | `notifications.status.rolled_back` | رجع إلى الإصدار السابق | Rolled back | Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 82 | `notifications.status.rollback_failed` | فشل الرجوع | Rollback failed | Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 83 | `notifications.status.awaiting_approval` | ينتظر الموافقة | Waiting for approval | Approval ← موافقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 84 | `notifications.test.send` | أرسل رسالة اختبار | Send a test message | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 85 | `notifications.test.queued` | أُرسلت رسالة اختبار إلى {name}. ستجدها في سجل التسليم. | A test message was sent to {name}. Look for it in the delivery log. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 86 | `notifications.test.failed` | لم نرسل رسالة الاختبار إلى {name} | We did not send the test message to {name} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 87 | `notifications.remove.button` | احذف | Delete | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 88 | `notifications.remove.aria` | احذف القناة {name} | Delete channel {name} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 89 | `notifications.remove.question` | سيتوقف داو عن الإرسال عبر القناة {name}، وتنتهي الرسائل المنتظرة. هل تريد حذفها؟ | Dhow stops sending through channel {name}, and the waiting messages end. Do you want to delete it? | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 90 | `notifications.remove.confirm` | نعم، احذف القناة | Yes, delete the channel | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 91 | `notifications.remove.done` | حُذفت القناة {name} | Deleted channel {name} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 92 | `notifications.enable.button` | فعّل | Turn on | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 93 | `notifications.enable.aria` | فعّل القناة {name} | Turn on channel {name} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 94 | `notifications.enable.question` | ستعمل القناة {name} من جديد وتصبح أنت صاحبها. أصلح سبب التوقف قبل ذلك. | Channel {name} works again and you become its owner. Fix the reason it stopped first. | Owner ← صاحب المؤسسة / صاحب النسخة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 95 | `notifications.enable.confirm` | نعم، فعّل القناة | Yes, turn on the channel | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 96 | `notifications.enable.done` | عادت القناة {name} للعمل | Channel {name} works again | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 97 | `notifications.rotate.button` | سر جديد | New secret | Secret ← سر | اسم لا فعل. اقتراح: «أنشئ سرًا جديدًا». | ☐ سليم ☐ يُعدَّل: |
| 98 | `notifications.rotate.aria` | سر جديد للقناة {name} | New secret for channel {name} | Secret ← سر | يتبع نص الزر. اقتراح: «أنشئ سرًا جديدًا للقناة {name}». | ☐ سليم ☐ يُعدَّل: |
| 99 | `notifications.rotate.question` | سيصنع داو سر توقيع جديدًا للقناة {name}، ويتوقف السر القديم عن العمل. حدّث مستقبلك به. | Dhow makes a new signing secret for channel {name}, and the old secret stops working. Update your receiver with it. | Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 100 | `notifications.rotate.confirm` | نعم، اصنع سرًا جديدًا | Yes, make a new secret | Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 101 | `notifications.rotate.done` | صُنع سر جديد للقناة {name} | Made a new secret for channel {name} | Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 102 | `notifications.deliveries.group` | سجل التسليم | Delivery log | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 103 | `notifications.deliveries.title` | سجل التسليم | Delivery log | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 104 | `notifications.deliveries.lead` | كل رسالة أرسلها داو، وحالتها وعدد محاولاتها. لا يحفظ السجل عناوين القنوات ولا نص الرسائل. | Every message Dhow sent, its state and how many times it tried. The log keeps neither channel addresses nor message text. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 105 | `notifications.deliveries.loading` | جارٍ تحميل سجل التسليم | Loading the delivery log | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 106 | `notifications.deliveries.error` | لم نتمكن من تحميل سجل التسليم | We could not load the delivery log | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 107 | `notifications.deliveries.empty` | لا توجد رسائل في السجل بعد. | There are no messages in the log yet. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 108 | `notifications.deliveries.filter` | الحالة | State | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 109 | `notifications.deliveries.state.all` | كل الحالات | Every state | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 110 | `notifications.deliveries.state.pending` | تنتظر | Waiting | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 111 | `notifications.deliveries.state.sending` | تُرسل | Sending | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 112 | `notifications.deliveries.state.sent` | وصلت | Delivered | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 113 | `notifications.deliveries.state.dead` | فشلت نهائيًا | Failed for good | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 114 | `notifications.deliveries.attempts` | المحاولات: {count} | Attempts: {count} | Attempt ← محاولة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 115 | `notifications.deliveries.retry` | أعد المحاولة | Try again | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 116 | `notifications.deliveries.retry.aria` | أعد المحاولة: {name} | Try again: {name} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 117 | `notifications.deliveries.retried` | أُعيدت جدولة الرسالة إلى {name} | The message to {name} is queued again | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 118 | `notifications.code.blocked` | العنوان محظور | The address is blocked | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 119 | `notifications.code.redirect` | ردّ المستقبل بتحويل ولا يتبعه داو | The receiver redirected and Dhow does not follow | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 120 | `notifications.code.timeout` | انتهت مهلة الانتظار | The wait timed out | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 121 | `notifications.code.connect` | لم ينجح الاتصال | Could not connect | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 122 | `notifications.code.http_status` | رفض المستقبل الرسالة | The receiver refused the message | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 123 | `notifications.code.config` | إعداد القناة غير صالح | The channel settings are not valid | Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 124 | `notifications.code.channel_removed` | حُذفت القناة | The channel was deleted | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 125 | `notifications.code.channel_disabled` | القناة متوقفة | The channel is stopped | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 126 | `notifications.code.mail_disabled` | البريد غير مفعّل على هذا السيرفر | Email is not turned on on this server | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 127 | `notifications.code.mail_send` | لم ينجح إرسال البريد | Could not send the email | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 128 | `notifications.code.no_recipient` | لا يوجد مستلم | There is no recipient | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 129 | `notifications.code.lease_expired` | توقف السيرفر أثناء الإرسال | The server stopped while sending | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 130 | `notifications.mute.title` | رسائل القنوات بالبريد | Channel emails | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 131 | `notifications.mute.lead` | القنوات التي ترسل بريدًا إلى الأعضاء حسب أدوارهم قد تصلك منها رسائل كثيرة. يمكنك إيقافها عن بريدك فقط. | Channels that email members by role can send you many messages. You can stop them for your own email only. | Member ← عضو | ✔ | ☐ سليم ☐ يُعدَّل: |
| 132 | `notifications.mute.error` | لم نتمكن من قراءة إعداد الكتم | We could not read the mute setting | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 133 | `notifications.mute.option` | أوقف رسائل القنوات عن بريدي | Stop channel emails to my address | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 134 | `notifications.mute.about` | تبقى رسائل أمان حسابك تصل دائمًا، ويبقى صندوق الإشعارات كما هو. | The security emails of your account always arrive, and the notification inbox stays as it is. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 135 | `notifications.inbox.loading` | جارٍ تحميل الإشعارات | Loading the notifications | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 136 | `notifications.inbox.error` | لم نتمكن من تحميل الإشعارات | We could not load the notifications | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 137 | `notifications.inbox.filter` | عرض الإشعارات | Show notifications | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 138 | `notifications.inbox.filter.all` | الكل | All | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 139 | `notifications.inbox.filter.unread` | غير المقروء | Unread | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 140 | `notifications.inbox.markAll` | اجعل الكل مقروءًا | Mark all as read | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 141 | `notifications.inbox.emptyUnread` | لا توجد إشعارات غير مقروءة. | There are no unread notifications. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 142 | `notifications.inbox.unread` | جديد | New | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 143 | `notifications.inbox.markRead` | مقروء | Mark as read | — | زر بكلمة حالة. ADR-0062: الزر يقول ماذا سيفعل. اقتراح: «علّمه مقروءًا». | ☐ سليم ☐ يُعدَّل: |
| 144 | `notifications.inbox.markRead.aria` | مقروء: {title} | Mark as read: {title} | — | يتبع نص الزر أعلاه. اقتراح: «علّمه مقروءًا: {title}». | ☐ سليم ☐ يُعدَّل: |
| 145 | `notifications.inbox.unknown` | إشعار من داو | A notification from Dhow | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 146 | `notifications.inbox.field.project` | المشروع | Project | Project ← مشروع (الجمع: المشاريع) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 147 | `notifications.inbox.field.environment` | البيئة | Environment | Environment ← بيئة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 148 | `notifications.inbox.field.ref` | الفرع | Branch | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 149 | `notifications.inbox.field.run` | رقم التنفيذ | Run number | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 150 | `notifications.inbox.field.runner` | جهاز التشغيل | Runner | Runner ← جهاز تشغيل | ✔ | ☐ سليم ☐ يُعدَّل: |
| 151 | `notifications.inbox.field.browser` | المتصفح | Browser | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 152 | `notifications.inbox.field.release` | الإصدار | Release | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 153 | `notifications.inbox.field.change` | التغيير | Change | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 154 | `notifications.inbox.field.role` | الدور | Role | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 155 | `notifications.inbox.change.totp_enabled` | فُعّل تطبيق المصادقة | An authenticator app was turned on | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 156 | `notifications.inbox.change.totp_disabled` | أُوقف تطبيق المصادقة | An authenticator app was turned off | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 157 | `notifications.inbox.change.passkey_added` | أُضيف مفتاح مرور | A passkey was added | Passkey ← مفتاح المرور | ✔ | ☐ سليم ☐ يُعدَّل: |
| 158 | `notifications.inbox.change.passkey_removed` | أُزيل مفتاح مرور | A passkey was removed | Passkey ← مفتاح المرور | ✔ | ☐ سليم ☐ يُعدَّل: |
| 159 | `notifications.inbox.change.recovery_codes_regenerated` | أُنشئت أكواد طوارئ جديدة | New recovery codes were made | Recovery codes ← أكواد الطوارئ | ✔ | ☐ سليم ☐ يُعدَّل: |
| 160 | `notifications.inbox.deployment.finished.succeeded` | اكتمل النشر بنجاح | A deployment succeeded | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 161 | `notifications.inbox.deployment.finished.failed` | فشل النشر | A deployment failed | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 162 | `notifications.inbox.deployment.finished.canceled` | أُلغي النشر | A deployment was canceled | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 163 | `notifications.inbox.deployment.finished.rolled_back` | رجع النشر إلى الإصدار السابق | A deployment rolled back to the previous release | Deployment ← نشر، Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 164 | `notifications.inbox.deployment.finished.rollback_failed` | فشل الرجوع إلى الإصدار السابق | A rollback to the previous release failed | Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 165 | `notifications.inbox.deployment.approval.requested` | نشر ينتظر موافقتك | A deployment waits for your approval | Deployment ← نشر، Approval ← موافقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 166 | `notifications.inbox.run.finished` | فشل تنفيذ خطوات النشر | A pipeline run failed | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 167 | `notifications.inbox.runner.offline` | جهاز تشغيل لا يستجيب | A runner stopped answering | Runner ← جهاز تشغيل | ✔ | ☐ سليم ☐ يُعدَّل: |
| 168 | `notifications.inbox.account.new_device_login` | دخل عضو من جهاز جديد | A member signed in from a new device | Member ← عضو | ✔ | ☐ سليم ☐ يُعدَّل: |
| 169 | `notifications.inbox.account.new_device_login.self` | سُجّل دخول إلى حسابك من جهاز جديد | Your account was signed in from a new device | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 170 | `notifications.inbox.account.second_factor_changed` | تغيّرت الخطوة الثانية لحساب عضو | The second step of a member account changed | Member ← عضو | ✔ | ☐ سليم ☐ يُعدَّل: |
| 171 | `notifications.inbox.account.second_factor_changed.self` | تغيّرت الخطوة الثانية لحسابك | The second step of your account changed | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 172 | `notifications.inbox.member.removed` | أُزيل عضو من المؤسسة | A member was removed from the organization | Organization ← المؤسسة، Member ← عضو | ✔ | ☐ سليم ☐ يُعدَّل: |
| 173 | `notifications.inbox.member.role_changed` | تغيّر دور عضو في المؤسسة | The role of a member changed | Member ← عضو | ✔ | ☐ سليم ☐ يُعدَّل: |
| 174 | `notifications.inbox.member.role_changed.self` | تغيّر دورك في المؤسسة | Your role in the organization changed | Organization ← المؤسسة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 175 | `notifications.inbox.notification.test` | رسالة اختبار من داو | A test message from Dhow | — | ✔ | ☐ سليم ☐ يُعدَّل: |

### لوحة النشر (M7.5)

الملف: `Workspace/BoardsAr.ts`

| # | المفتاح | العربي | English | المصطلح الواجب | فحص | قرارك |
|---|---|---|---|---|---|---|
| 1 | `projects.nav.deploy` | النشر | Deploy | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 2 | `boards.title` | لوحة النشر | Deploy board | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 3 | `boards.lead` | لكل بيئة في المشروع: الهدف الذي ننشر عليه، وما المنشور الآن، وكيف انتهى آخر نشر. تتحدث اللوحة وحدها. | For each environment of the project: the target we deploy to, what is deployed now, and how the last deploy ended. The board updates by itself. | Environment ← بيئة، Deployment ← نشر، Project ← مشروع (الجمع: المشاريع) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 4 | `boards.list` | بيئات النشر | Deploy environments | Environment ← بيئة، Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 5 | `boards.loading` | جارٍ تحميل لوحة النشر | Loading the deploy board | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 6 | `boards.error` | لم نتمكن من تحميل لوحة النشر | We could not load the deploy board | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 7 | `boards.empty` | لا توجد بيئات في هذا المشروع بعد. | This project has no environments yet. | Environment ← بيئة، Project ← مشروع (الجمع: المشاريع) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 8 | `boards.empty.hint` | أنشئ بيئة وأضف إليها هدف نشر، ثم تظهر هنا. | Create an environment and add a deploy target to it. It then shows here. | Environment ← بيئة، Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 9 | `boards.truncated` | في المشروع أكثر من 500 هدف. تظهر أول 500 فقط. | The project has more than 500 targets. Only the first 500 show. | Project ← مشروع (الجمع: المشاريع) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 10 | `boards.count` | الأهداف: {count} | Targets: {count} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 11 | `boards.noTargets` | لا يوجد هدف نشر في هذه البيئة بعد. | This environment has no deploy target yet. | Environment ← بيئة، Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 12 | `boards.env.paused` | النشر متوقف | Deploys paused | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 13 | `boards.env.approvals` | ينتظر موافقة: {count} | Waiting for approval: {count} | Approval ← موافقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 14 | `boards.target.hosts` | عدد السيرفرات: {count} | Servers: {count} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 15 | `boards.deployed` | المنشور الآن | Deployed now | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 16 | `boards.deployed.none` | لم يُنشر شيء بعد | Nothing deployed yet | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 17 | `boards.last` | آخر نشر | Last deploy | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 18 | `boards.last.none` | لا نشر سابق | No earlier deploy | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 19 | `boards.last.at` | بدأ {time} | Started {time} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 20 | `boards.status.running` | يعمل الآن | Running now | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 21 | `boards.status.succeeded` | نجح | Succeeded | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 22 | `boards.status.failed` | فشل | Failed | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 23 | `boards.status.rolled_back` | رجع تلقائيًا | Rolled back automatically | Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 24 | `boards.status.rollback_failed` | فشل الرجوع | Rollback failed | Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 25 | `boards.status.unknown` | حالة غير معروفة | Unknown state | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 26 | `boards.badge.partial` | إصدارات مختلفة | Mixed versions | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 27 | `boards.badge.rollbackFailed` | فشل الرجوع، يلزم تدخلك | Rollback failed, needs you | Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 28 | `boards.note.rollbackFailed` | فشل الرجوع بعد نشر لم ينجح. قد تعمل السيرفرات على إصدارات مختلفة. افحص الهدف ثم ارجع إلى إصدار سليم. | The rollback failed after a deploy that did not succeed. The servers may run different versions. Check the target, then roll back to a good version. | Deployment ← نشر، Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 29 | `boards.note.partial` | ليست كل السيرفرات على نفس الإصدار. افحص الهدف. | Not every server runs the same version. Check the target. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 30 | `boards.progress.label` | تقدّم النشر إلى الإصدار {release} | Deploy progress to release {release} | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 31 | `boards.progress.batch` | الدفعة {batch} من {batches} | Batch {batch} of {batches} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 32 | `boards.progress.hosts` | {done} من {hosts} سيرفرات | {done} of {hosts} servers | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 33 | `boards.progress.starting` | يبدأ النشر على {hosts} سيرفرات | Starting the deploy on {hosts} servers | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 34 | `boards.segment.done` | دفعة اكتملت | Batch finished | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 35 | `boards.segment.current` | الدفعة الحالية | Current batch | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 36 | `boards.segment.waiting` | دفعة تنتظر | Batch waiting | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 37 | `boards.action.check` | افحص الهدف | Check the target | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 38 | `boards.action.rollback` | الرجوع إلى إصدار سابق | Roll back to an earlier release | Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 39 | `boards.action.details` | تفاصيل الهدف | Target details | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 40 | `boards.action.hide` | أخفِ التفاصيل | Hide the details | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 41 | `boards.action.open` | افتح صفحة الهدف | Open the target page | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 42 | `boards.action.forbidden` | الفحص والرجوع لمسؤول المشاريع وما فوقه. | Checking and rolling back are for project maintainers and above. | Project ← مشروع (الجمع: المشاريع)، Maintainer ← مسؤول مشاريع | المعجم: «مسؤول مشاريع» بلا «ال» في الدور. المقصود الدور، فقد يكون الأفضل «دور مسؤول مشاريع». | ☐ سليم ☐ يُعدَّل: |
| 43 | `boards.action.paused` | النشر متوقف في هذه البيئة، فلا يمكن الرجوع الآن. | Deploys are paused in this environment, so you cannot roll back now. | Environment ← بيئة، Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 44 | `boards.action.aria` | إجراءات الهدف {target} في البيئة {environment} | Actions for target {target} in environment {environment} | Environment ← بيئة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 45 | `boards.approvals.title` | بانتظار موافقتك | Waiting for your approval | Approval ← موافقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 46 | `boards.approvals.empty` | لا يوجد نشر ينتظر موافقتك في هذا المشروع. | No deploy in this project is waiting for your approval. | Deployment ← نشر، Project ← مشروع (الجمع: المشاريع)، Approval ← موافقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 47 | `live.connecting` | جارٍ الاتصال بالتحديث المباشر | Connecting to live updates | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 48 | `live.polling` | التحديث المباشر متوقف، والصفحة تتحدث كل 5 ثوانٍ. | Live updates are off. The page refreshes every 5 seconds. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 49 | `live.on` | التحديث مباشر | Updates are live | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 50 | `home.stat.deploys` | نشر يعمل الآن | Deploys running now | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 51 | `home.stat.deploys.failed` | {count} فشل في آخر 24 ساعة | {count} failed in the last 24 hours | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 52 | `home.attention.rollbackFailed.title` | أهداف فشل الرجوع فيها | Targets where the rollback failed | Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 53 | `home.attention.rollbackFailed.sub` | قد تعمل سيرفراتها على إصدارات مختلفة. افتح المشاريع وافحص الهدف. | Their servers may run different versions. Open the projects and check the target. | Project ← مشروع (الجمع: المشاريع) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 54 | `home.attention.failedDeploys.title` | نشر فشل في آخر 24 ساعة | Deploys that failed in the last 24 hours | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 55 | `home.attention.failedDeploys.sub` | افتح لوحة النشر في المشروع لترى أي بيئة فشلت. | Open the deploy board of the project to see which environment failed. | Environment ← بيئة، Deployment ← نشر، Project ← مشروع (الجمع: المشاريع) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 56 | `home.attention.deadNotifications.title` | إشعارات لم تصل | Notifications that did not arrive | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 57 | `home.attention.deadNotifications.sub` | تخلّى داو عنها بعد كل المحاولات. افحص قنوات الإشعارات في إعدادات المؤسسة. | Dhow gave up on them after every try. Check the notification channels in the organization settings. | Organization ← المؤسسة، Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |

### المساعدة والأسئلة الشائعة (M7.8)

الملف: `Workspace/HelpAr.ts`

| # | المفتاح | العربي | English | المصطلح الواجب | فحص | قرارك |
|---|---|---|---|---|---|---|
| 1 | `shell.nav.help` | المساعدة | Help | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 2 | `palette.command.help` | افتح المساعدة | Open help | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 3 | `help.title` | المساعدة | Help | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 4 | `help.lead` | أسئلة شائعة وحلول لمشكلات معروفة وما الجديد في داو. | Frequently asked questions, fixes for known problems, and what is new in Dhow. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 5 | `help.offline` | تعمل هذه الصفحة بدون اتصال بالإنترنت. الروابط إلى الوثائق الكاملة تحتاج إلى اتصال. | This page works without an internet connection. Links to the full documentation need a connection. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 6 | `help.loading` | جارٍ تحميل المساعدة | Loading help | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 7 | `help.error.title` | لم نتمكن من تحميل المساعدة | We could not load help | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 8 | `help.error.body` | لم نتمكن من تحميل محتوى المساعدة. أعد تحميل الصفحة وحاول مرة أخرى. | The help content did not load. Reload the page and try again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 9 | `help.error.retry` | حاول مرة أخرى | Try again | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 10 | `help.search.label` | ابحث في المساعدة | Search help | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 11 | `help.search.placeholder` | اكتب كلمة أو جملة قصيرة | Type a word or a short phrase | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 12 | `help.search.clear` | امسح البحث | Clear the search | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 13 | `help.search.results` | {count, plural, zero {لا توجد نتائج} one {نتيجة واحدة} two {نتيجتان} few {{count} نتائج} many {{count} نتيجة} other {{count} نتيجة}} | {count, plural, =0 {No results} one {{count} result} other {{count} results}} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 14 | `help.search.empty` | لا توجد نتائج للبحث «{query}». جرّب كلمة أخرى، أو اقرأ الأسئلة الشائعة في الوثائق. | No results for "{query}". Try another word, or read the FAQ in the docs. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 15 | `help.faq.title` | الأسئلة الشائعة | Frequently asked questions | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 16 | `help.troubleshooting.title` | حل المشكلات | Troubleshooting | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 17 | `help.releases.title` | الجديد في داو | What's new in Dhow | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 18 | `help.releases.version` | النسخة {version} | Version {version} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 19 | `help.pages.title` | مساعدة كل صفحة | Help for every page | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 20 | `help.pages.open` | افتح وثائق «{page}» | Open the docs for "{page}" | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 21 | `help.docs.all` | افتح الأسئلة الشائعة في الوثائق | Open the FAQ in the docs | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 22 | `help.button` | مساعدة | Help | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 23 | `help.hint.home` | ملخص ما يعمل وما يحتاج إلى انتباهك. | A summary of what runs and what needs your attention. | — | كلمة runs فعل في الأصل، فلا مصطلح مطلوب. | ☐ سليم ☐ يُعدَّل: |
| 24 | `help.hint.projects` | مشاريعك وبيئاتها وتنفيذاتها. | Your projects, their environments, and their runs. | Run ← تنفيذ (الجمع: التنفيذات)، Environment ← بيئة، Project ← مشروع (الجمع: المشاريع) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 25 | `help.hint.jobs` | تابع التشغيلات وسجلاتها. | Follow runs and their logs. | Run ← تنفيذ (الجمع: التنفيذات) | الصفحة تعرض التشغيلات (Job)، فالمصطلح «تشغيل» صحيح هنا رغم أن الأصل runs. | ☐ سليم ☐ يُعدَّل: |
| 26 | `help.hint.approvals` | وافق على النشر أو ارفضه. | Approve or reject a deployment. | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 27 | `help.hint.runners` | أجهزة التشغيل المسجلة وحالتها. | The registered runners and their state. | Runner ← جهاز تشغيل | ✔ | ☐ سليم ☐ يُعدَّل: |
| 28 | `help.hint.runnerTokens` | أنشئ رموز تسجيل أجهزة التشغيل. | Create runner registration tokens. | Runner ← جهاز تشغيل، Registration token ← رمز التسجيل | ✔ | ☐ سليم ☐ يُعدَّل: |
| 29 | `help.hint.members` | أعضاء المؤسسة وأدوارهم. | The organization members and their roles. | Organization ← المؤسسة، Member ← عضو | ✔ | ☐ سليم ☐ يُعدَّل: |
| 30 | `help.hint.teams` | مجموعات الأعضاء في المؤسسة. | Groups of members in the organization. | Organization ← المؤسسة، Member ← عضو | المعجم: «الفريق». الأصل الإنجليزي يقول «Groups». اقتراح: «فرق الأعضاء في المؤسسة». | ☐ سليم ☐ يُعدَّل: |
| 31 | `help.hint.invitations` | ادعُ أعضاء جددًا إلى المؤسسة. | Invite new members to the organization. | Organization ← المؤسسة، Member ← عضو، Invitation ← دعوة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 32 | `help.hint.apiKeys` | مفاتيح API للمؤسسة. | API keys for the organization. | Organization ← المؤسسة، API token ← مفتاح API | ✔ | ☐ سليم ☐ يُعدَّل: |
| 33 | `help.hint.secrets` | أسرار المؤسسة ومتغيّراتها. | The organization's secrets and variables. | Secret ← سر، Variable ← متغيّر، Organization ← المؤسسة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 34 | `help.hint.orgSettings` | اسم المؤسسة وقواعدها. | The organization's name and rules. | Organization ← المؤسسة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 35 | `help.hint.audit` | سجل الأحداث الذي لا يمكن تغييره. | The audit log, which nobody can change. | Audit log ← سجل الأحداث | ✔ | ☐ سليم ☐ يُعدَّل: |
| 36 | `help.hint.system` | حالة السيرفر وفحوصاته. | The server's health and checks. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 37 | `help.hint.systemSettings` | إعدادات السيرفر لصاحب النسخة. | The server's settings, for the server owner. | Owner ← صاحب المؤسسة / صاحب النسخة، Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 38 | `help.hint.about` | النسخة والترخيص وروابط الوثائق. | The version, the license, and links to the docs. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 39 | `help.hint.profile` | بيانات حسابك ومفاتيحك. | Your account details and keys. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 40 | `help.hint.security` | كلمة السر والخطوة الثانية ومفاتيح المرور. | Password, second step, and passkeys. | Passkey ← مفتاح المرور | ✔ | ☐ سليم ☐ يُعدَّل: |
| 41 | `help.hint.preferences` | اللغة والثيم. | Language and theme. | — | «الثيم» غير موجودة في المعجم، لكنها مستعملة في كل الواجهة. اقتراح: نضيفها للمعجم. | ☐ سليم ☐ يُعدَّل: |
| 42 | `help.hint.organizations` | المؤسسات التي تنتمي إليها. | The organizations you belong to. | Organization ← المؤسسة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 43 | `help.hint.notifications` | إشعارات حسابك. | Your account's notifications. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 44 | `help.hint.help` | أسئلة شائعة وبحث بدون اتصال بالإنترنت. | Frequently asked questions, with search that works offline. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 45 | `whatsNew.title` | الجديد في داو {version} | What's new in Dhow {version} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 46 | `whatsNew.all` | كل الملاحظات | All release notes | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 47 | `whatsNew.dismiss` | أغلق البطاقة | Close the card | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 48 | `whatsNew.saveFailed` | لم نتمكن من حفظ أنك قرأت الجديد. ستظهر البطاقة مرة أخرى. | We could not save that you read the news. The card will show again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |

### عن داو وفحوصات السيرفر (M7.9)

الملف: `Workspace/AboutAr.ts`

| # | المفتاح | العربي | English | المصطلح الواجب | فحص | قرارك |
|---|---|---|---|---|---|---|
| 1 | `shell.nav.about` | عن داو | About Dhow | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 2 | `palette.command.about` | افتح صفحة عن داو | Open About Dhow | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 3 | `about.lead` | النسخة التي تعمل الآن، وروابط الوثائق، والترخيص. | The version that runs now, the links to the docs, and the license. | — | كلمة runs فعل في الأصل، فلا مصطلح مطلوب. | ☐ سليم ☐ يُعدَّل: |
| 4 | `about.loading` | جارٍ تحميل معلومات السيرفر | Loading the server information | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 5 | `about.error.title` | لم نتمكن من تحميل معلومات السيرفر | We could not get the server information | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 6 | `about.reload.title` | يوجد إصدار أحدث من داو | A newer version of Dhow is running | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 7 | `about.reload.body` | تحدّث السيرفر بعد أن فتحت هذه الصفحة. أعد التحميل لتعمل بالنسخة الجديدة. | The server was updated after you opened this page. Reload to use the new version. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 8 | `about.reload.action` | أعد تحميل الصفحة | Reload the page | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 9 | `about.build.title` | النسخة والبناء | Version and build | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 10 | `about.build.date` | تاريخ البناء | Build date | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 11 | `about.build.health` | حالة السيرفر | Server health | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 12 | `about.build.unknown` | غير معروف | Unknown | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 13 | `about.status.ok` | سليم | Healthy | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 14 | `about.status.info` | معلومة | Note | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 15 | `about.status.warn` | تحذير | Warning | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 16 | `about.status.fail` | فشل | Failed | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 17 | `about.checks.title` | فحوصات السيرفر | Server checks | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 18 | `about.checks.hostNote` | بعض الفحوصات تحتاج صلاحيات المدير على الجهاز، مثل صلاحيات الملفات ومؤقت التحديث. شغّل أمر الفحص من سطر الأوامر على السيرفر لتنفيذها. | Some checks need root on the host, such as file permissions and the update timer. Run dhow-server doctor on the server to run them. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 19 | `about.links.title` | الوثائق | Documentation | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 20 | `about.link.install` | دليل التنصيب | Install guide | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 21 | `about.link.doctor` | عن داو وفحوصات السيرفر | About Dhow and the server checks | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 22 | `about.link.cli` | أوامر سطر الأوامر | Command line reference | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 23 | `about.link.security` | نموذج الأمان | Security model | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 24 | `about.link.editions` | الإصدارات والترخيص | Editions and licensing | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 25 | `about.link.license` | نص الترخيص | License text | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 26 | `about.license.title` | الترخيص | License | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 27 | `about.license.body` | داو برنامج حر بترخيص AGPL-3.0. المميزات التجارية لها ترخيص منفصل، وهو ترخيص داو للمؤسسات. | Dhow is free software under AGPL-3.0. The files in the ee/ folder have a commercial license, the Dhow Enterprise License. | — | ✔ | ☐ سليم ☐ يُعدَّل: |

### عارض السجل (M7.4)

الملف: `Workspace/LogViewerAr.ts`

| # | المفتاح | العربي | English | المصطلح الواجب | فحص | قرارك |
|---|---|---|---|---|---|---|
| 1 | `jobs.log.polling` | تحديث دوري | Periodic refresh | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 2 | `jobs.log.fallback` | استخدم اتصالًا بديلًا | Use a fallback connection | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 3 | `jobs.log.fallbackAbout` | إذا كان الوسيط يقطع البث المباشر، نقرأ السجل كل ثانيتين بدلًا منه. | If a proxy cuts the live stream, we read the log every two seconds instead. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 4 | `jobs.log.toolbar` | أدوات السجل | Log tools | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 5 | `jobs.log.search.label` | ابحث في السجل | Search the log | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 6 | `jobs.log.search.case` | مطابقة حالة الأحرف | Match case | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 7 | `jobs.log.search.next` | التالي | Next | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 8 | `jobs.log.search.prev` | السابق | Previous | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 9 | `jobs.log.search.status` | النتيجة {n} من {total} | Result {n} of {total} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 10 | `jobs.log.search.empty` | لا توجد نتائج | No results | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 11 | `jobs.log.search.partial` | البحث يشمل الأسطر المحمّلة فقط. حمّل الأسطر الأقدم لتشملها. | Search covers the loaded lines only. Load the older lines to include them. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 12 | `jobs.log.collapse.all` | اطوِ كل الخطوات | Collapse all steps | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 13 | `jobs.log.collapse.none` | افتح كل الخطوات | Expand all steps | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 14 | `jobs.log.step.collapsed` | الخطوة مطوية | Step collapsed | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 15 | `jobs.log.step.expanded` | الخطوة مفتوحة | Step expanded | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 16 | `jobs.log.download` | نزّل السجل | Download the log | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 17 | `jobs.log.link.copy` | انسخ رابط السطر المحدد | Copy the link of the selected line | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 18 | `jobs.log.link.copied` | نسخنا رابط السطر. | We copied the line link. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 19 | `jobs.log.link.failed` | لم نتمكن من نسخ الرابط. | We could not copy the link. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 20 | `jobs.log.line.select` | السطر {n}: انسخ رابطه | Line {n}: copy its link | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 21 | `jobs.log.line.missing` | لم نجد السطر {n} في هذا السجل. | We could not find line {n} in this log. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 22 | `jobs.log.older.load` | حمّل الأسطر الأقدم | Load the older lines | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 23 | `jobs.log.older.loading` | جارٍ تحميل الأسطر الأقدم | Loading the older lines | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 24 | `jobs.log.older.failed` | لم نتمكن من تحميل الأسطر الأقدم. | We could not load the older lines. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 25 | `jobs.log.newLines` | {count, plural, one {سطر جديد واحد} two {سطران جديدان} few {{count} أسطر جديدة} many {{count} سطرًا جديدًا} other {{count} سطر جديد}} | {count, plural, one {1 new line} other {{count} new lines}} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 26 | `jobs.log.shortcuts` | اضغط / للبحث، وزر الإدخال للنتيجة التالية، وزر النهاية لآخر سطر. | Press / to search, Enter for the next result, and End for the last line. | — | ✔ | ☐ سليم ☐ يُعدَّل: |

### سجل الأحداث

الملف: `Workspace/AuditAr.ts`

| # | المفتاح | العربي | English | المصطلح الواجب | فحص | قرارك |
|---|---|---|---|---|---|---|
| 1 | `audit.action.notification.channel_created` | إنشاء قناة إشعارات | Notification channel created | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 2 | `audit.action.notification.channel_updated` | تغيير قناة إشعارات | Notification channel changed | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 3 | `audit.action.notification.channel_removed` | حذف قناة إشعارات | Notification channel removed | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 4 | `audit.action.notification.channel_disabled` | إيقاف قناة إشعارات | Notification channel stopped | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 5 | `audit.action.notification.rule_updated` | تغيير قاعدة إشعارات | Notification rule changed | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 6 | `audit.action.notification.test_sent` | إرسال رسالة اختبار من قناة | Test message sent through a channel | — | ✔ | ☐ سليم ☐ يُعدَّل: |

### المؤسسة

الملف: `Workspace/OrganizationAr.ts`

| # | المفتاح | العربي | English | المصطلح الواجب | فحص | قرارك |
|---|---|---|---|---|---|---|
| 1 | `shell.organization.switch` | غيّر المؤسسة، والحالية: | Switch organization, now: | Organization ← المؤسسة | تغيّر عن: غيّر المؤسسة (الحالية: {name}) | ☐ سليم ☐ يُعدَّل: |

### الأسرار

الملف: `Workspace/SecretsAr.ts`

| # | المفتاح | العربي | English | المصطلح الواجب | فحص | قرارك |
|---|---|---|---|---|---|---|
| 1 | `secrets.change.aria` | غيّر القيمة: السر {name} | Change value: secret {name} | Secret ← سر | تغيّر عن: غيّر قيمة السر {name} | ☐ سليم ☐ يُعدَّل: |
| 2 | `registry.change.aria` | غيّر الحساب: السجل {host} | Change account: registry {host} | — | تغيّر عن: غيّر حساب السجل {host} | ☐ سليم ☐ يُعدَّل: |

### الإعدادات (M7.6)

الملف: `Workspace/SettingsAr.ts`

| # | المفتاح | العربي | English | المصطلح الواجب | فحص | قرارك |
|---|---|---|---|---|---|---|
| 1 | `settings.toc` | في هذه الصفحة | On this page | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 2 | `settings.page.projectNotifications` | إشعارات المشروع | Project notifications | Project ← مشروع (الجمع: المشاريع) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 3 | `palette.setting` | {section} في {page} | {section} in {page} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 4 | `palette.kind.setting` | إعداد | Setting | — | ✔ | ☐ سليم ☐ يُعدَّل: |

### إطار التطبيق (Boot)

الملف: `Boot/Ar.ts`

| # | المفتاح | العربي | English | المصطلح الواجب | فحص | قرارك |
|---|---|---|---|---|---|---|
| 1 | `i18n.switchFailed` | لم نتمكن من تغيير اللغة. حاول مرة أخرى. | We could not change the language. Try again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 2 | `api.error.fix` | الحل: | Fix: | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 3 | `api.error.newTab` | يفتح في تبويب جديد | opens in a new tab | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 4 | `api.error.help` | اعرف أكثر | Learn more | — | ✔ | ☐ سليم ☐ يُعدَّل: |

### الصفحات العامة (Workspace/Ar.ts)

الملف: `Workspace/Ar.ts`

| # | المفتاح | العربي | English | المصطلح الواجب | فحص | قرارك |
|---|---|---|---|---|---|---|
| 1 | `sessions.revoke.aria` | أخرِجه: {device} | Sign it out: {device} | — | تغيّر عن: أخرِج {device} | ☐ سليم ☐ يُعدَّل: |
| 2 | `mfa.passkeys.rename.aria` | غيّر الاسم: {name} | Rename {name} | — | تغيّر عن: غيّر اسم {name} | ☐ سليم ☐ يُعدَّل: |
| 3 | `mfa.passkeys.remove.aria` | أزِله: {name} | Remove {name} | — | تغيّر عن: أزِل {name} | ☐ سليم ☐ يُعدَّل: |

## رسائل السيرفر

من `apps/server/internal/Core`. الإنجليزي بنفس المفتاح في ملف `.En.go`.

### رسائل الأخطاء والإشعارات والبريد (`I18n.Ar.go`)

| # | المفتاح | العربي | English | المصطلح الواجب | فحص | قرارك |
|---|---|---|---|---|---|---|
| 1 | `errors.releaseVersion` | رقم النسخة غير صالح. اكتبه مثل 1.2.3. | The version is not valid. Write it like 1.2.3. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 2 | `preferences.releaseNotesSeen` | سجّلنا أنك قرأت الجديد. | Release notes marked as seen. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 3 | `errors.channelExists` | توجد قناة إشعارات بهذا الاسم في هذا النطاق. اختر اسمًا آخر. | A notification channel with this name already exists here. Choose another name. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 4 | `errors.channelHostNotAllowed` | عنوان القناة غير مقبول. للـ webhook استخدم عنوان https بدون اسم مستخدم ولا كلمة سر، واكتب السيرفر باسمه أو بعنوان IP عادي. وقناة Slack أو Discord تقبل عنوان خدمتها فقط. | The channel address is not accepted. For a webhook, use an https address without a user name or a password, and write the server as a name or a plain IP address. A Slack or Discord channel accepts only the address of its own service. | Webhook ← الإشعارات (webhook) | جملة طويلة (18 كلمة) | ☐ سليم ☐ يُعدَّل: |
| 5 | `errors.channelHostBlocked` | عنوان الـ webhook يشير إلى شبكة داخلية أو محجوزة، فلا يتصل به داو. إذا كان على شبكتك الداخلية، أضف نطاقه إلى DHOW_NOTIFY_ALLOW_PRIVATE_CIDRS. | The webhook address points to an internal or reserved network, so Dhow does not connect to it. If it is on your internal network, add its range to DHOW_NOTIFY_ALLOW_PRIVATE_CIDRS. | Webhook ← الإشعارات (webhook) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 6 | `errors.channelSecretInvalid` | لم نتمكن من قراءة إعدادات القناة المحفوظة. أدخل عنوان الـ webhook من جديد. | The saved settings of the channel cannot be read. Enter the webhook address again. | Webhook ← الإشعارات (webhook)، Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 7 | `errors.channelVersionConflict` | غيّر شخص آخر هذه القناة منذ أن فتحتها. حدّث الصفحة وراجع التغيير ثم حاول مرة أخرى. | Someone else changed this channel since you opened it. Reload the page, check the change and try again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 8 | `errors.channelDisabled` | القناة متوقفة. راجع سبب التوقف، ثم فعّلها من إعدادات الإشعارات. | The channel is stopped. Check why it stopped, then turn it on again in the notification settings. | Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 9 | `errors.deliveryNotRetryable` | لا يمكن إعادة محاولة هذا الإرسال إلا إذا فشل نهائيًا. | A delivery can be retried only after it failed for good. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 10 | `errors.testRateLimited` | أرسلت رسائل اختبار كثيرة لهذه القناة. انتظر دقيقة ثم حاول مرة أخرى. | You sent many test messages to this channel. Wait a minute and try again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 11 | `mail.alert.change.new_device_login` | سُجّل دخول إلى حسابك من جهاز جديد | Someone signed in to your account from a new device | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 12 | `notify.mail.subject` | [داو] {title} | [Dhow] {title} | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 13 | `notify.disabled.subject` | [داو] توقفت قناة الإشعارات {channel} | [Dhow] The notification channel {channel} stopped | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 14 | `notify.disabled.body` | مرحبًا {name}،\n\nأوقف داو قناة الإشعارات «{channel}» بعد {count} محاولات متتالية فاشلة. أصلح عنوان الاستقبال أو الخدمة، ثم فعّل القناة من إعدادات الإشعارات:\n{link}\n | Hello {name},\n\nDhow stopped the notification channel \"{channel}\" after {count} failed attempts in a row. Fix the receiving address or the service, then turn the channel on again in the notification settings:\n{link}\n | Attempt ← محاولة، Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 15 | `notify.title.deployment.finished.succeeded` | اكتمل النشر بنجاح | Deployment finished | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 16 | `notify.title.deployment.finished.failed` | فشل النشر | Deployment failed | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 17 | `notify.title.deployment.finished.canceled` | أُلغي النشر | Deployment canceled | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 18 | `notify.title.deployment.finished.rolled_back` | رجع النشر إلى الإصدار السابق | Deployment rolled back to the previous release | Deployment ← نشر، Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 19 | `notify.title.deployment.finished.rollback_failed` | فشل الرجوع إلى الإصدار السابق | Rollback to the previous release failed | Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 20 | `notify.title.deployment.approval.requested` | نشر ينتظر موافقتك | A deployment waits for your approval | Deployment ← نشر، Approval ← موافقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 21 | `notify.title.run.finished` | فشل تنفيذ خطوات النشر | A pipeline run failed | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 22 | `notify.title.runner.offline` | جهاز تشغيل لا يستجيب | A runner stopped answering | Runner ← جهاز تشغيل | ✔ | ☐ سليم ☐ يُعدَّل: |
| 23 | `notify.title.account.new_device_login` | دخول من جهاز جديد | Sign-in from a new device | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 24 | `notify.title.account.second_factor_changed` | تغيّرت الخطوة الثانية لحساب | The second factor of an account changed | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 25 | `notify.title.member.removed` | أُزيل عضو من المؤسسة | A member was removed from the organization | Organization ← المؤسسة، Member ← عضو | ✔ | ☐ سليم ☐ يُعدَّل: |
| 26 | `notify.title.member.role_changed` | تغيّر دور عضو في المؤسسة | The role of a member changed | Member ← عضو | ✔ | ☐ سليم ☐ يُعدَّل: |
| 27 | `notify.title.notification.test` | رسالة اختبار من داو | Test message from Dhow | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 28 | `notify.status.succeeded` | نجح | Succeeded | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 29 | `notify.status.failed` | فشل | Failed | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 30 | `notify.status.canceled` | أُلغي | Canceled | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 31 | `notify.status.rolled_back` | رجع إلى الإصدار السابق | Rolled back | Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 32 | `notify.status.rollback_failed` | فشل الرجوع | Rollback failed | Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 33 | `notify.status.awaiting_approval` | ينتظر الموافقة | Awaiting approval | Approval ← موافقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 34 | `notify.status.test` | اختبار | Test | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 35 | `notify.field.status` | الحالة | Status | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 36 | `notify.field.project` | المشروع | Project | Project ← مشروع (الجمع: المشاريع) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 37 | `notify.field.environment` | البيئة | Environment | Environment ← بيئة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 38 | `notify.field.ref` | الفرع | Branch | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 39 | `notify.field.run` | رقم التنفيذ | Run number | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 40 | `notify.field.runner` | جهاز التشغيل | Runner | Runner ← جهاز تشغيل | ✔ | ☐ سليم ☐ يُعدَّل: |
| 41 | `notify.field.browser` | المتصفح | Browser | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 42 | `notify.field.network` | الشبكة | Network | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 43 | `notify.field.time` | الوقت | Time | — | ✔ | ☐ سليم ☐ يُعدَّل: |

### جمل الحل لكل كود خطأ (`I18n.Fix.Ar.go`، M7.7)

| # | المفتاح | العربي | English | المصطلح الواجب | فحص | قرارك |
|---|---|---|---|---|---|---|
| 1 | `ARTIFACT_GONE` | افتح التنفيذ واضغط زر إعادة التشغيل لإنشاء الملف من جديد. | Open the run and press the run-again button to build the file once more. | Run ← تنفيذ (الجمع: التنفيذات) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 2 | `INTERNAL` | حاول مرة أخرى بعد دقيقة. إذا تكرر الخطأ، أخبر مسؤول السيرفر بوقته ليراجع سجل السيرفر. | Try again in a minute. If it keeps happening, tell your server admin the time of the error so they can check the server log. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 3 | `NOT_FOUND` | تأكد من العنوان، أو ارجع إلى القائمة وافتح العنصر من جديد. ربما حُذف. | Check the address, or go back to the list and open the item again. It may have been removed. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 4 | `BAD_REQUEST` | أعد تحميل الصفحة وحاول مرة أخرى. إذا استمر الخطأ، راجع الحقول في النموذج. | Reload the page and try again. If it repeats, check the fields in the form. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 5 | `VALIDATION_FAILED` | صحح الحقول المعلّمة باللون الأحمر، ثم أرسل النموذج من جديد. | Fix the fields marked in red, then send the form again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 6 | `PAYLOAD_TOO_LARGE` | أرسل بيانات أقل في الطلب الواحد، مثل ملف أصغر أو عناصر أقل. | Send less data in one request, for example a smaller file or fewer items. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 7 | `SERVICE_UNAVAILABLE` | انتظر دقيقة ثم أعد تحميل الصفحة. إذا استمر التوقف، اطلب من مسؤول السيرفر مراجعة قاعدة البيانات وسجل السيرفر. | Wait a minute and reload the page. If it stays down, ask your server admin to check the database and the server log. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 8 | `UNAUTHENTICATED` | سجّل الدخول من جديد، ثم كرر ما كنت تفعله. | Sign in again, then repeat what you were doing. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 9 | `FORBIDDEN` | اطلب من صاحب المؤسسة أو مديرها أن يعطيك الدور الذي يحتاجه هذا الإجراء. | Ask an owner or admin of the organization to give you the role this action needs. | Organization ← المؤسسة، Owner ← صاحب المؤسسة / صاحب النسخة، Admin ← مدير | ✔ | ☐ سليم ☐ يُعدَّل: |
| 10 | `CSRF_FAILED` | أعد تحميل الصفحة وحاول مرة أخرى. إذا تكرر الخطأ، اطلب من مسؤول السيرفر مراجعة العنوان العام والوكيل العكسي. | Reload the page and try again. If it repeats, ask your server admin to check the public address and the reverse proxy. | Reverse proxy ← وكيل عكسي | ✔ | ☐ سليم ☐ يُعدَّل: |
| 11 | `INVALID_CREDENTIALS` | راجع البريد وكلمة السر. إذا نسيت كلمة السر، اطلب المساعدة من مسؤول السيرفر. | Check the email and the password. If you forgot the password, ask your server admin for help. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 12 | `INVALID_CURRENT_PASSWORD` | اكتب كلمة السر الحالية من جديد، بنفس الحروف التي تدخل بها. | Type your current password again, with the same letters and case you use to sign in. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 13 | `PASSWORD_BREACHED` | اختر كلمة سر طويلة لم تستخدمها في موقع آخر. | Choose a long password that you have never used on another site. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 14 | `SETUP_COMPLETED` | اذهب إلى صفحة الدخول وسجّل بحساب صاحب النسخة. | Go to the sign-in page and sign in with the owner account. | Owner ← صاحب المؤسسة / صاحب النسخة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 15 | `SETUP_TOKEN_INVALID` | أعد تشغيل السيرفر وافتح رابط الإعداد الجديد من سجله. | Restart the server and open the new setup link from its log. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 16 | `CONFLICT` | غيّر القيمة، مثل الاسم أو البريد، إلى قيمة غير مستخدمة. | Change the value, for example the name or the email, to one that is not used yet. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 17 | `MFA_CHALLENGE_EXPIRED` | ارجع إلى صفحة الدخول، وأدخل كلمة السر من جديد، ثم اكتب الرمز فورًا. | Go back to the sign-in page, enter your password again, then enter the code at once. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 18 | `INVALID_MFA_CODE` | انتظر الرمز التالي في تطبيقك واكتبه. يمكنك أيضًا استخدام كود طوارئ. | Wait for the next code in your app and type it. You can also use a recovery code. | Recovery codes ← أكواد الطوارئ | ✔ | ☐ سليم ☐ يُعدَّل: |
| 19 | `INVALID_CURRENT_FACTOR` | أدخل كلمة السر الحالية أو رمزًا جديدًا من تطبيقك، ثم حاول مرة أخرى. | Enter your current password or a fresh code from your app, then try again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 20 | `ENROLLMENT_EXPIRED` | ابدأ الإعداد من البداية، وأنهِه خلال دقائق. | Start the setup again from the beginning, and finish it within a few minutes. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 21 | `PASSKEY_REJECTED` | حاول مرة أخرى بنفس الجهاز، أو استخدم جهازًا آخر أو مفتاح أمان. | Try again with the same device, or use another device or a security key. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 22 | `RUNNER_REVOKED` | سجّل جهاز تشغيل جديدًا برمز تسجيل جديد. جهاز التشغيل الملغى نهائيًا لا يعود. | Register a new runner with a new registration token. A revoked runner cannot come back. | Runner ← جهاز تشغيل، Registration token ← رمز التسجيل | ✔ | ☐ سليم ☐ يُعدَّل: |
| 23 | `SECRETS_UNAVAILABLE` | احذف الأسرار من مواصفة التشغيل، أو اطلب من مسؤول السيرفر إعداد مخزن الأسرار. | Remove the secrets from the job spec, or ask your server admin to set up the secret store. | Job ← تشغيل (الجمع: التشغيلات)، Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 24 | `JOB_FINISHED` | لا يلزم شيء. لتشغيلها مرة أخرى، ابدأ تشغيلًا جديدًا. | Nothing to do. To run it again, start a new run. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 25 | `JOB_QUEUE_FULL` | انتظر حتى تعمل بعض التشغيلات، أو ألغِ تشغيلات لا تحتاجها، ثم ابدأ التشغيل من جديد. | Wait until some jobs run, or cancel jobs you do not need, then start the run again. | Job ← تشغيل (الجمع: التشغيلات)، Run ← تنفيذ (الجمع: التنفيذات) | السياق عن تشغيلات (Job)، فالمصطلح «تشغيل» صحيح رغم كلمة run في الأصل. | ☐ سليم ☐ يُعدَّل: |
| 26 | `ATTEMPT_NOT_FOUND` | انتظر حتى يأخذ جهاز تشغيل هذا التشغيل، ثم افتح السجل من جديد. إذا لم يأخذه أحد، تأكد أن جهاز التشغيل متصل. | Wait until a runner takes the job, then open the log again. If no runner takes it, check that a runner is online. | Runner ← جهاز تشغيل، Job ← تشغيل (الجمع: التشغيلات) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 27 | `TOO_MANY_STREAMS` | أغلق السجلات الحية التي لا تقرؤها، ثم افتح هذا السجل من جديد. | Close the live logs you are not reading, then open this one again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 28 | `RATE_LIMITED` | انتظر المدة الظاهرة ثم حاول مرة أخرى. لا ترسل الطلب نفسه في حلقة. | Wait for the time shown, then try again. Do not send the same request in a loop. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 29 | `LAST_OWNER` | اجعل عضوًا آخر صاحبًا للمؤسسة أولًا، ثم غيّر دور صاحبها الحالي أو احذفه. | Make another member an owner first, then change or remove this owner. | Member ← عضو، Owner ← صاحب المؤسسة / صاحب النسخة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 30 | `ROLE_NOT_ALLOWED` | اختر دورًا لا يزيد على دورك، أو اطلب من صاحب المؤسسة أن ينفذ ذلك. | Choose a role that is not above yours, or ask an owner to do it. | Owner ← صاحب المؤسسة / صاحب النسخة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 31 | `ALREADY_MEMBER` | لا يلزم شيء. لتغيير دور هذا الشخص، افتح قائمة الأعضاء. | Nothing to do. To change the role of this person, open the members list. | Member ← عضو | ✔ | ☐ سليم ☐ يُعدَّل: |
| 32 | `INVITATION_PENDING` | افتح قائمة الدعوات وأعد إرسال الدعوة المفتوحة. | Open the invitations list and resend the open invitation. | Invitation ← دعوة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 33 | `INVITATION_INVALID` | اطلب ممن دعاك أن يرسل دعوة جديدة. | Ask the person who invited you to send a new invitation. | Invitation ← دعوة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 34 | `INVITATION_EMAIL_MISMATCH` | سجّل الخروج، ثم افتح رابط الدعوة من جديد بالحساب الذي فيه البريد المدعو. | Sign out, then open the invitation link again with the account that has the invited email. | Invitation ← دعوة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 35 | `INVITATION_ACCOUNT_EXISTS` | سجّل الدخول بهذا الحساب، ثم افتح رابط الدعوة من جديد. | Sign in with that account, then open the invitation link again. | Invitation ← دعوة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 36 | `INVITATION_SEND_LIMIT` | ألغِ هذه الدعوة من قائمة الدعوات، ثم ادعُ الشخص من جديد. | Revoke this invitation in the invitations list, then invite the person again. | Invitation ← دعوة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 37 | `OIDC_FLOW_INVALID` | ابدأ الدخول من جديد من صفحة الدخول، في المتصفح نفسه، وأنهِه دون تأخير. | Start the sign-in again from the sign-in page, in the same browser, and finish it without delay. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 38 | `OIDC_SIGN_IN_FAILED` | حاول مرة أخرى، أو سجّل الدخول بكلمة السر. إذا تكرر الخطأ، اطلب من مسؤول السيرفر مراجعة إعدادات المزود. | Try again, or sign in with your password. If it repeats, tell your server admin to check the provider settings. | Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 39 | `OIDC_NOT_LINKED` | سجّل الدخول بكلمة السر، ثم اربط هذا المزود من إعدادات حسابك. | Sign in with your password, then link this provider in your account settings. | Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 40 | `OIDC_DOMAIN_NOT_ALLOWED` | ادخل ببريد من نطاق مسموح، أو اطلب من مسؤول السيرفر السماح بهذا النطاق. | Sign in with an email from an allowed domain, or ask your server admin to allow this domain. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 41 | `OIDC_PROVIDER_UNAVAILABLE` | انتظر دقيقة وحاول مرة أخرى، أو ادخل بكلمة السر. إذا استمر التوقف، اطلب من مسؤول السيرفر مراجعة المزود. | Wait a minute and try again, or sign in with your password. If it stays down, ask your server admin to check the provider. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 42 | `OIDC_IDENTITY_IN_USE` | ادخل إلى الحساب المرتبط بهذه الهوية، أو افصلها هناك أولًا. | Sign in to the account that has this identity, or unlink the identity there first. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 43 | `OIDC_ALREADY_LINKED` | افصل الهوية الحالية من إعدادات حسابك، ثم اربط الجديدة. | Unlink the current identity in your account settings, then link the new one. | Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 44 | `LAST_SIGN_IN_METHOD` | أضف طريقة دخول أخرى، مثل كلمة سر أو مزود آخر، ثم احذف هذه. | Add another way to sign in, such as a password or another provider, then remove this one. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 45 | `SESSION_REQUIRED` | نفّذ هذا الإجراء من تطبيق الويب بعد تسجيل الدخول. مفتاح API لا يستطيع ذلك. | Do this action from the web app after you sign in. An API token cannot do it. | API token ← مفتاح API | ✔ | ☐ سليم ☐ يُعدَّل: |
| 46 | `API_TOKEN_LIMIT` | ألغِ مفتاح API لا تستخدمه من قائمة مفاتيح API، ثم أنشئ الجديد. | Revoke an API token you do not use in the API keys list, then create the new one. | API token ← مفتاح API | ✔ | ☐ سليم ☐ يُعدَّل: |
| 47 | `SCOPE_NOT_ALLOWED` | احذف الصلاحية التي لا يملكها دورك، أو اطلب من مدير رفع دورك. | Remove the permission your role does not have, or ask an admin to raise your role. | Admin ← مدير | ✔ | ☐ سليم ☐ يُعدَّل: |
| 48 | `SECOND_FACTOR_REQUIRED` | فعّل التحقق بخطوتين من إعدادات حسابك، ثم كرر الإجراء. | Turn on two-step verification in your account settings, then repeat the action. | Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 49 | `PROJECT_ARCHIVED` | المشروع المؤرشف للقراءة فقط. نفّذ التغيير في مشروع آخر، أو أنشئ مشروعًا جديدًا. | An archived project is read-only. Make the change in another project, or create a new one. | Project ← مشروع (الجمع: المشاريع) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 50 | `FORK_POLICY_NOT_ALLOWED` | اطلب من مدير المؤسسة أو صاحبها تغيير هذا الإعداد. | Ask an organization admin or owner to change this setting. | Organization ← المؤسسة، Owner ← صاحب المؤسسة / صاحب النسخة، Admin ← مدير | ✔ | ☐ سليم ☐ يُعدَّل: |
| 51 | `FORK_CACHE_NOT_ALLOWED` | اطلب من مدير المشروع تغيير هذا الإعداد. | Ask an admin of the project to change this setting. | Project ← مشروع (الجمع: المشاريع)، Admin ← مدير | ✔ | ☐ سليم ☐ يُعدَّل: |
| 52 | `ENVIRONMENT_EXISTS` | اكتب اسمًا آخر للبيئة. | Type another name for the environment. | Environment ← بيئة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 53 | `ENVIRONMENT_VERSION_CONFLICT` | أعد تحميل البيئة، وراجع الإعدادات الجديدة، ثم كرر تغييرك. | Reload the environment, check the new settings, then make your change again. | Environment ← بيئة، Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 54 | `REVIEWER_NOT_ELIGIBLE` | احذف المراجعين الذين ليسوا أعضاء، أو أعطهم دور مطوّر في المشروع. | Remove the reviewers who are not members, or give them the Developer role on the project. | Project ← مشروع (الجمع: المشاريع)، Member ← عضو، Developer ← مطوّر، Reviewer ← مراجع | ✔ | ☐ سليم ☐ يُعدَّل: |
| 55 | `SELF_REVIEW_NOT_ALLOWED` | اطلب من مراجع آخر أن يوافق على هذا النشر أو يرفضه. | Ask another reviewer to approve or reject this deploy. | Deployment ← نشر، Reviewer ← مراجع | ✔ | ☐ سليم ☐ يُعدَّل: |
| 56 | `REVIEWER_NEEDS_SECOND_FACTOR` | أدخل رمزًا من تطبيقك أو كود طوارئ أو مفتاح مرور. كلمة السر لا تكفي. | Enter a code from your app, a recovery code or a passkey. A password is not enough. | Passkey ← مفتاح المرور، Recovery codes ← أكواد الطوارئ | ✔ | ☐ سليم ☐ يُعدَّل: |
| 57 | `APPROVAL_NOT_PENDING` | أعد تحميل الصفحة لترى الحالة الحالية. ابدأ نشرًا جديدًا إن كنت ما زلت تحتاجه. | Reload the page to see the current state. Start a new deploy if you still need one. | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 58 | `BYPASS_NOT_ALLOWED` | انتظر قرار المراجعين، أو اطلب من مدير السماح بالتخطي في قواعد البيئة. | Wait for the reviewers to decide, or ask an admin to allow bypass in the environment rules. | Environment ← بيئة، Admin ← مدير، Reviewer ← مراجع | ✔ | ☐ سليم ☐ يُعدَّل: |
| 59 | `INGEST_TOKEN_INVALID` | انسخ الرمز من جديد من إعدادات المشروع، أو أنشئ رمزًا جديدًا. تأكد أنه بلا مسافة زائدة. | Copy the token again from the project settings, or make a new token. Check that no space was added. | Project ← مشروع (الجمع: المشاريع)، Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 60 | `INGEST_TOKEN_REVOKED` | أنشئ رمز استقبال جديدًا من إعدادات المشروع واستخدمه في CI. | Make a new ingest token in the project settings and use it in your CI. | Project ← مشروع (الجمع: المشاريع)، Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 61 | `INGEST_TOKEN_EXPIRED` | أنشئ رمز استقبال جديدًا من إعدادات المشروع واستخدمه في CI. | Make a new ingest token in the project settings and use it in your CI. | Project ← مشروع (الجمع: المشاريع)، Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 62 | `INGEST_TOKEN_LIMIT` | ألغِ رمز استقبال لا تستخدمه من إعدادات المشروع، ثم أنشئ الجديد. | Revoke an ingest token you do not use in the project settings, then make the new one. | Project ← مشروع (الجمع: المشاريع)، Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 63 | `INGEST_TOKEN_NAME_TAKEN` | اختر اسمًا آخر للرمز، أو ألغِ الرمز القديم الذي يحمل هذا الاسم. | Choose another name for the token, or revoke the old token that has this name. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 64 | `REF_NOT_ALLOWED_FOR_TOKEN` | ارفع من فرع أو وسم يسمح به الرمز، أو أضف هذا المرجع إلى أنماط الرمز. | Upload from a branch or tag the token allows, or add this ref to the token patterns. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 65 | `DIGEST_MISMATCH` | احسب البصمة من جديد من الملف نفسه الذي ترفعه، وضعها في العنوان وارفع مرة أخرى. | Compute the digest again from the exact file you upload, put it in the address and upload again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 66 | `ARCHIVE_UNSAFE` | اضغط الملفات من جديد في tar.gz عادي من غير روابط أو أجهزة أو مسارات تخرج من المجلد. | Pack the files again as a plain tar.gz without links, devices or paths that leave the folder. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 67 | `ARTIFACT_TOO_LARGE` | ارفع أرشيفًا أصغر، أو اطلب من مسؤول السيرفر رفع حد الاستقبال. | Upload a smaller archive, or ask your server admin to raise the ingest limit. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 68 | `ARTIFACT_QUOTA_EXCEEDED` | احذف ملفات ناتجة قديمة لا تحتاجها، أو اطلب من مسؤول السيرفر رفع الحصة. | Remove old artifacts you do not need, or ask your server admin to raise the quota. | Artifact ← الملف الناتج (الجمع: الملفات الناتجة) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 69 | `STORAGE_FULL` | اطلب من مسؤول السيرفر تفريغ مساحة على القرص أو إضافة تخزين. | Tell your server admin to free disk space or add storage. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 70 | `UPLOAD_TOO_SLOW` | راجع شبكتك ثم ارفع من جديد. ارفع من مكان اتصاله بالسيرفر أسرع. | Check your network, then upload again. Upload from a place with a faster link to the server. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 71 | `INGEST_TOKEN_QUOTA` | ألغِ هذا الرمز وأنشئ رمزًا جديدًا من إعدادات المشروع. | Revoke this token and make a new one in the project settings. | Project ← مشروع (الجمع: المشاريع)، Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 72 | `INGEST_TOKEN_REACH_UNCONFIRMED` | أكّد البيئات التي يصل إليها الرمز، أو ضيّق أنماط المراجع. | Confirm the environments the token reaches, or narrow its ref patterns. | Environment ← بيئة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 73 | `INGEST_CREATOR_NOT_ALLOWED` | أنشئ رمزًا جديدًا بحساب يملك إدارة الرفع في هذا المشروع. | Make a new token with an account that can manage uploads in this project. | Project ← مشروع (الجمع: المشاريع) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 74 | `INGEST_REACH_NOT_ACKNOWLEDGED` | اطلب من مدير المشروع أن يؤكد الرمز من قائمة الرموز. | Ask a project admin to confirm the token from the token list. | Project ← مشروع (الجمع: المشاريع)، Admin ← مدير | ✔ | ☐ سليم ☐ يُعدَّل: |
| 75 | `INGEST_REACH_NEEDS_ENVIRONMENT_ADMIN` | اطلب من مدير المشروع أن يؤكده، أو ضيّق أنماط المراجع في الرمز. | Ask a project admin to confirm it, or narrow the ref patterns of the token. | Project ← مشروع (الجمع: المشاريع)، Admin ← مدير | ✔ | ☐ سليم ☐ يُعدَّل: |
| 76 | `INGEST_REACH_CHANGED` | أعد تحميل الصفحة، وراجع البيئات التي يصل إليها الرمز، ثم أكّد من جديد. | Reload the page, review the environments the token reaches, then confirm again. | Environment ← بيئة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 77 | `SECRET_TOO_SHORT` | اكتب قيمة لا تقل عن 8 بايت. | Type a value of at least 8 bytes. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 78 | `SECRET_TOO_LARGE` | استخدم قيمة أصغر من 32 KiB. احفظ الملفات الكبيرة في مخزن واحفظ هنا مفتاحها فقط. | Use a value under 32 KiB. Keep big files in storage and keep only a key here. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 79 | `SECRET_NAME_RESERVED` | اختر اسمًا لا يبدأ بـ DHOW_. | Choose a name that does not start with DHOW_. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 80 | `SECRET_LIMIT_REACHED` | احذف سرًّا لا تستخدمه، ثم أضف الجديد. | Remove a secret you do not use, then add the new one. | Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 81 | `SECRET_VERSION_ERASED` | اكتب قيمة جديدة للسر. | Write a new value for the secret. | Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 82 | `VARIABLE_LIMIT_REACHED` | احذف متغيّرًا لا تستخدمه، ثم أضف الجديد. | Remove a variable you do not use, then add the new one. | Variable ← متغيّر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 83 | `REGISTRY_CREDENTIAL_EXISTS` | افتح الحساب الموجود لهذا السجل وعدّله. | Open the existing credential for this registry and change it. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 84 | `REGISTRY_CREDENTIAL_LIMIT_REACHED` | احذف حساب سجل لا تستخدمه، ثم أضف الجديد. | Remove a registry credential you do not use, then add the new one. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 85 | `SETTING_LOCKED_BY_ENV` | غيّر متغيّر البيئة على السيرفر، ثم أعد تشغيل السيرفر. | Change the environment variable on the server, then restart the server. | Environment ← بيئة، Variable ← متغيّر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 86 | `SETTING_CHANGED` | أعد تحميل الصفحة، وراجع القيم الجديدة، ثم كرر تغييرك. | Reload the page, check the new values, then make your change again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 87 | `SETTING_INVALID` | صحح القيم المعلّمة باللون الأحمر، ثم احفظ من جديد. | Fix the values marked in red, then save again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 88 | `MAIL_NOT_CONFIGURED` | املأ بطاقة سيرفر البريد في إعدادات السيرفر واحفظها أولًا. | Fill in the mail server card in the server settings and save it first. | Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 89 | `MAIL_TEST_FAILED` | راجع عنوان سيرفر البريد ومنفذه والمستخدم وكلمة السر، ثم أرسل الاختبار من جديد. | Check the host, port, user and password of the mail server, then send the test again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 90 | `OIDC_PROVIDER_EXISTS` | اكتب معرّفًا آخر للمزود. | Type another id for the provider. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 91 | `OIDC_PROVIDER_LIMIT` | احذف مزودًا لا تستخدمه، ثم أضف الجديد. | Remove a provider you do not use, then add the new one. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 92 | `OIDC_PROVIDER_LINKED` | أضف مزودًا جديدًا بالنوع أو المُصدِر الجديد، ثم انقل الحسابات إليه. | Add a new provider with the new type or issuer, then move the accounts to it. | Issuer ← المُصدِر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 93 | `OIDC_PROVIDER_ONLY_SIGN_IN` | اطلب من هؤلاء المستخدمين إضافة كلمة سر أو مزود آخر، ثم حاول مرة أخرى. | Ask those users to add a password or another provider, then try again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 94 | `REPOSITORY_NOT_LINKED` | افتح المشروع واربط مستودع Git، ثم كرر هذا الإجراء. | Open the project, link a Git repository, then repeat this action. | Project ← مشروع (الجمع: المشاريع)، Repository ← مستودع | ✔ | ☐ سليم ☐ يُعدَّل: |
| 95 | `GIT_HOST_BLOCKED` | إذا كان سيرفر Git على شبكتك الداخلية، اطلب من مسؤول السيرفر السماح بنطاق عنوانه. | If the Git server is on your internal network, ask your server admin to allow its address range. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 96 | `GIT_HOST_UNREACHABLE` | راجع عنوان سيرفر Git ومنفذه، ثم حاول مرة أخرى. | Check the address and the port of the Git server, then try again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 97 | `HOST_KEY_NOT_PENDING` | لا يوجد مفتاح ينتظر التأكيد. حمّل مفتاح السيرفر من جديد إذا احتجت مفتاحًا جديدًا. | Nothing to confirm. Fetch the key of the server again if you need a new one. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 98 | `HOST_KEY_FINGERPRINT_MISMATCH` | حمّل المفتاح من جديد وقارن البصمة بالتي أعطاك إياها المسؤول. | Fetch the key again and compare the fingerprint with the one your admin gave you. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 99 | `DEPLOY_KEY_NOT_APPLICABLE` | لا يلزم شيء. لاستخدام مفتاح المستودع، غيّر عنوان المستودع إلى صيغة ssh. | Nothing to do. To use a deploy key, change the repository address to the ssh form. | Repository ← مستودع، Deploy key ← مفتاح المستودع | ✔ | ☐ سليم ☐ يُعدَّل: |
| 100 | `RUN_NOT_RERUNNABLE` | ابدأ تنفيذًا جديدًا بدلًا منه. لا يُعاد إلا التنفيذ الفاشل أو الملغى. | Start a new run instead. Only a failed or canceled run can run again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 101 | `RUN_NOT_AWAITING_APPROVAL` | أعد تحميل الصفحة لترى الحالة الحالية للتنفيذ. | Reload the page to see the current state of the run. | Run ← تنفيذ (الجمع: التنفيذات) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 102 | `SHA_NOT_ON_REF` | ادفع الـ commit إلى الفرع أو الوسم، وانتظر الإشعار، ثم شغّل من جديد. | Push the commit to the branch or tag, wait for the webhook, then run again. | Webhook ← الإشعارات (webhook) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 103 | `REF_HEAD_UNKNOWN` | ادفع مرة واحدة إلى هذا الفرع أو الوسم ليصل الإشعار، ثم شغّل من جديد. | Push once to this branch or tag so the webhook arrives, then run again. | Webhook ← الإشعارات (webhook) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 104 | `TARGET_EXISTS` | اكتب اسمًا آخر للهدف. | Type another name for the target. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 105 | `TARGET_DUPLICATE` | اختر مسار نشر آخر، أو استخدم الهدف الذي فيه هذا السيرفر. | Choose another deploy path, or use the target that already has this server. | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 106 | `TARGET_VERSION_CONFLICT` | افتح الهدف من جديد، وراجع الإعدادات الجديدة، ثم كرر تغييرك. | Open the target again, check the new settings, then make your change again. | Settings ← الإعدادات | ✔ | ☐ سليم ☐ يُعدَّل: |
| 107 | `TARGET_HOST_BLOCKED` | الصق مفتاح السيرفر بيدك بدل جلبه، أو اطلب من مسؤول السيرفر السماح بهذا النطاق. | Paste the server key by hand instead of fetching it, or ask your server admin to allow the range. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 108 | `TARGET_HOST_UNREACHABLE` | تأكد أن السيرفر يعمل، وأن العنوان والمنفذ صحيحان، وأن الجدار الناري يسمح لداو. | Check that the server is on, the address and port are right, and the firewall allows Dhow. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 109 | `TARGET_KEY_NOT_FOUND` | أنشئ السر بهذا الاسم في البيئة أولًا، أو دع داو ينشئ المفتاح. | Create the secret with this name in the environment first, or let Dhow make the key. | Environment ← بيئة، Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 110 | `DEPLOY_PATH_INVALID` | استخدم مسارًا مطلقًا للتطبيق، مثل /srv/app، وليس مجلدًا للنظام. | Use an absolute path for the app, such as /srv/app, and not a system folder. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 111 | `TARGET_KEY_NAME_TAKEN` | غيّر اسم السر القديم، أو اربط الهدف بسر آخر. | Rename the old secret, or link the target to another secret. | Secret ← سر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 112 | `TARGET_CHECK_UNAVAILABLE` | استخدم إصدارًا رسميًا من داو، أو اطلب من مسؤول السيرفر تثبيت واحد. | Use an official release of Dhow, or ask your server admin to install one. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 113 | `CHECK_RUN_NOT_RERUNNABLE` | افتح الهدف واضغط زر الفحص لبدء فحص جديد. | Open the target and press the check button to start a new check. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 114 | `ROLLBACK_RUN_NOT_RERUNNABLE` | افتح الهدف وابدأ رجوعًا جديدًا من هناك. | Open the target and start a new rollback from there. | Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 115 | `ROLLBACK_RELEASE_NOT_ELIGIBLE` | اختر إصدارًا نجح على هذا الهدف من قبل. | Choose a release that succeeded on this target before. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 116 | `ROLLBACK_UNAVAILABLE` | استخدم إصدارًا رسميًا من داو، أو اطلب من مسؤول السيرفر تثبيت واحد. | Use an official release of Dhow, or ask your server admin to install one. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 117 | `ROLLBACK_NOT_SUPPORTED` | انشر النسخة السابقة من جديد كنشر جديد بدلًا من الرجوع. | Deploy the earlier version again as a new deploy instead. | Deployment ← نشر | ✔ | ☐ سليم ☐ يُعدَّل: |
| 118 | `ROLLBACK_SETTINGS_INVALID` | انشر إصدارًا جديدًا. لا ترجع إلى هذا الإصدار. | Deploy a new release. Do not roll back to this one. | Deployment ← نشر، Rollback ← ارجع إلى النسخة السابقة | ✔ | ☐ سليم ☐ يُعدَّل: |
| 119 | `ENVIRONMENT_PAUSED` | اطلب من مدير استئناف البيئة، ثم حاول مرة أخرى. | Ask an admin to resume the environment, then try again. | Environment ← بيئة، Admin ← مدير | ✔ | ☐ سليم ☐ يُعدَّل: |
| 120 | `TARGET_KEY_NEEDS_FACTOR` | أدخل كلمة السر أو رمزًا من تطبيقك عندما يطلب داو، ثم احفظ من جديد. | Enter your password or a code from your app when Dhow asks, then save again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 121 | `CHANNEL_EXISTS` | اختر اسمًا آخر للقناة، أو افتح القناة الموجودة وعدّلها. | Choose another name for the channel, or open the existing channel and edit it. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 122 | `CHANNEL_HOST_NOT_ALLOWED` | للـ webhook اكتب عنوان https بدون اسم مستخدم. ولـ Slack استخدم عنوانًا من hooks.slack.com، ولـ Discord عنوانًا من discord.com أو discordapp.com. | Use an https address with no user name. For Slack use an address on hooks.slack.com, for Discord one on discord.com or discordapp.com. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 123 | `CHANNEL_HOST_BLOCKED` | اطلب من مسؤول السيرفر إضافة نطاق الشبكة إلى DHOW_NOTIFY_ALLOW_PRIVATE_CIDRS، أو استخدم عنوانًا عامًا. | Ask your server admin to add the network range to DHOW_NOTIFY_ALLOW_PRIVATE_CIDRS, or use a public address. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 124 | `CHANNEL_SECRET_INVALID` | أدخل عنوان الـ webhook من جديد، ثم احفظ القناة. | Enter the webhook address again, then save the channel. | Webhook ← الإشعارات (webhook) | ✔ | ☐ سليم ☐ يُعدَّل: |
| 125 | `CHANNEL_VERSION_CONFLICT` | أعد تحميل الصفحة وراجع التغيير الذي أجراه غيرك، ثم احفظ من جديد. | Reload the page, check the change someone else made, then save again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 126 | `CHANNEL_DISABLED` | راجع سبب التوقف في صفحة القناة وأصلحه، ثم شغّل القناة من جديد. | Check why the channel stopped on its page, fix it, then turn the channel on again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 127 | `DELIVERY_NOT_RETRYABLE` | انتظر حتى يفشل الإرسال نهائيًا (الحالة dead)، ثم اضغط إعادة المحاولة. | Wait until the delivery fails for good (state dead), then press retry. | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 128 | `TEST_RATE_LIMITED` | انتظر دقيقة، ثم أرسل رسالة الاختبار مرة أخرى. | Wait a minute, then send the test message again. | — | ✔ | ☐ سليم ☐ يُعدَّل: |

### المثبّت و`doctor` (`Messages.Ar.go`)

| # | المفتاح | العربي | English | المصطلح الواجب | فحص | قرارك |
|---|---|---|---|---|---|---|
| 1 | `KeyAdviceClock` | بين ساعة السيرفر وساعة قاعدة البيانات فرق أكبر من بضع ثوانٍ. هذا يفسد الرموز المؤقتة ومواعيد الانتهاء. فعّل مزامنة الوقت على الجهازين: timedatectl set-ntp true | The clock of the server and the clock of the database differ by more than a few seconds. That breaks one-time codes and expiry times. Turn on time sync on both hosts: timedatectl set-ntp true | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 2 | `KeyCheckClock` | ساعة السيرفر مقارنة بساعة قاعدة البيانات | Server clock against the database clock | — | ✔ | ☐ سليم ☐ يُعدَّل: |
| 3 | `KeyCheckStateDisk` | المساحة الفارغة لبيانات السيرفر | Free disk space for the server data | — | ✔ | ☐ سليم ☐ يُعدَّل: |
