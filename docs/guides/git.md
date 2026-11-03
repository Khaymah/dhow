[English](../en/guides/git.md)

# ربط مستودع Git

يربط داو كل مشروع بمستودع Git واحد. هذه الصفحة تشرح Git العام: Gitea وForgejo وأي سيرفر Git يدعم SSH أو https. ربط GitHub وGitLab يأتي في مراحل لاحقة. الخطوات في الواجهة في [المشاريع والتنفيذات](projects.md#ربط-مستودع)، وهذه الصفحة تشرحها بالـ API.

## العنوان

```http
PUT /api/v1/organizations/{id}/projects/{projectId}/repository
```

```json
{"CloneUrl": "ssh://git@git.example.com:2222/team/app.git", "DefaultBranch": "main"}
```

- يقبل داو `ssh://user@host[:port]/path`، و`user@host:path` (يحوّله إلى `ssh://`)، و`https://host[:port]/path` لمستودع عام بلا كلمة سر.
- يرفض داو عنوان IP، وكلمة السر في العنوان، و`..`، والمسار أو الاسم الذي يبدأ بـ `-`، وأي عنوان آخر مثل `file://` و`ext::`.
- يحتاج الربط إلى صلاحية `project.manage` على المشروع. ومع SSH يتصل داو بالسيرفر في كل `PUT`، لذلك يقبل 30 طلبًا في الساعة لكل مستخدم و120 لكل مؤسسة، وبعدها `429`.
- أول ربط يرد بـ `201` ومعه سر الإشعارات (`WebhookSecret`). **انسخه الآن، لأنه لا يظهر مرة أخرى أبدًا.** أي `PUT` بعده يرد بـ `200` بلا سر، ويحتفظ بمفتاح المستودع ورابط الإشعارات.

## مفتاح المستودع

مع SSH، ينشئ داو مفتاح Ed25519 للمستودع. يظهر النص العام في `DeployKeyPublic`، فأضفه إلى سيرفر Git كمفتاح للقراءة فقط (deploy key). المفتاح الخاص يبقى مشفّرًا على السيرفر، ولا يظهر في أي رد أو سجل.

لتجديده، استخدم `POST .../repository/rotate-deploy-key`. يبقى المفتاح السابق صالحًا 24 ساعة، أو حتى تستدعي `POST .../repository/confirm-deploy-key` بعد إضافة الجديد.

## مفتاح سيرفر المستودع

مع SSH، يتصل داو بالسيرفر ويجلب مفتاحه (host key)، ثم يعرض بصمته في `PendingHostKeyFingerprint`، مثل `SHA256:...`. قارنها بالبصمة التي يعرضها السيرفر نفسه، ثم ثبّتها:

```http
POST /api/v1/organizations/{id}/projects/{projectId}/repository/confirm-host-key
```

```json
{"Fingerprint": "SHA256:..."}
```

لا يثبّت داو مفتاحًا بنفسه أبدًا. إذا تغيّر مفتاح السيرفر، يبقى الجديد في الانتظار حتى تؤكده، ويبقى القديم هو المستخدم. وحتى التأكيد الأول، يسجّل داو الإشعارات ويتجاهلها.

لا يتصل داو بعنوان داخلي أو محجوز (loopback والشبكات الخاصة وlink-local وCGNAT وعناوين metadata السحابية)، ولو جاء من DNS. إذا كان سيرفر Git على شبكتك الداخلية، أضف نطاقه إلى `DHOW_GIT_ALLOW_PRIVATE_CIDRS`. راجع [الإعدادات](../install/configuration.md#متغيرات-السيرفر).

## الإشعارات (webhook)

الرد يحتوي على `WebhookUrl`، مثل `https://dhow.example.com/webhooks/git/<HookId>`. الجزء الأخير عشوائي، وليس رقم المستودع.

- **Gitea وForgejo:** أضف webhook من نوع Gitea أو Forgejo، بالرابط والسر، ونوع المحتوى `application/json`، وحدث push فقط. يقبل داو `X-Forgejo-Signature` و`X-Gitea-Signature` و`X-Hub-Signature-256`.
- **أي سيرفر آخر:** أرسل من سكربت (مثل `post-receive`) طلب `POST` بهذا الجسم:

```json
{"Ref": "refs/heads/main", "Sha": "<commit-sha>"}
```

ومعه الترويستان: `X-Dhow-Delivery` بمعرّف فريد للطلب، و`X-Dhow-Signature` بالوقت بثواني unix وتوقيع HMAC-SHA256 بالسر على الوقت ثم نقطة ثم الجسم، بصيغة hex:

```text
X-Dhow-Delivery: <delivery-id>
X-Dhow-Signature: t=<unix-seconds>,v1=<hex(HMAC-SHA256(secret, t + "." + body))>
```

مثال بـ `openssl`:

```sh
t=$(date +%s)
sig=$(printf '%s.%s' "$t" "$body" | openssl dgst -sha256 -hmac "$secret" -hex | sed 's/^.* //')
curl -sS -X POST "$url" -H "X-Dhow-Delivery: $(uuidgen)" -H "X-Dhow-Signature: t=$t,v1=$sig" --data "$body"
```

ما يفعله داو بكل طلب:

| الحالة | الرد |
|---|---|
| توقيع صحيح وطلب جديد | `202`، حتى لو تجاهله (حدث غير push، أو فرع محذوف، أو مشروع مؤرشف، أو مفتاح سيرفر لم يُؤكَّد) |
| الطلب نفسه مرة أخرى في 3 أيام، بالمعرّف نفسه أو بالجسم نفسه | `200` بلا أي أثر |
| توقيع خاطئ، أو سر مستودع آخر، أو جسم معدّل، أو `t` أقدم من 5 دقائق، أو رابط غير موجود | `401 WEBHOOK_REJECTED`، الرد نفسه دائمًا |
| توقيع صحيح بلا معرّف طلب، أو بفرع أو sha غير صالح | `422 VALIDATION_FAILED` |
| جسم أكبر من `DHOW_WEBHOOK_MAX_BYTES` | `413` |
| أكثر من 300 طلب في الدقيقة للرابط، أو 1200 من العنوان نفسه | `429` مع `Retry-After` |

لا يحفظ داو جسم الطلب، لأنه قد يحتوي على أسماء وعناوين بريد. يحفظ المعرّف وبصمة الجسم والفرع والـ sha والنتيجة فقط، ويحذفها بعد 30 يومًا.

لتجديد السر، استخدم `POST .../repository/rotate-webhook-secret`. يظهر السر الجديد مرة واحدة، ويتوقف القديم فورًا.

## فصل المستودع

```http
DELETE /api/v1/organizations/{id}/projects/{projectId}/repository
```

يحتاج إلى `project.manage`. يتوقف مفتاح المستودع والإشعارات فورًا: الإشعار إلى الرابط القديم يرد بـ `401`، ولا يبدأ تنفيذ جديد حتى تربط مستودعًا من جديد. الربط من جديد ينشئ مفتاحًا ورابطًا وسرًا جديدة. احذف المفتاح والإشعارات من سيرفر Git أيضًا.
