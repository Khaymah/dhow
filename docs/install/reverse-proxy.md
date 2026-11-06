[English](../en/install/reverse-proxy.md)

# ضع داو خلف وكيل عكسي

يتكلم داو (Dhow) بـ HTTP العادي، ويتوقع أمامه وكيلًا عكسيًا (reverse proxy) يتولى TLS. في هذه الصفحة إعدادات كاملة لـ [nginx](#nginx-مع-certbot) و[Caddy](#caddy) و[Traefik](#traefik-داخل-حزمة-compose)، وشرح [`DHOW_TRUSTED_PROXIES`](#الوكلاء-الموثوقون)، و[طريقة التحقق من النتيجة](#تحقق-من-الإعداد).

كل إعداد في هذه الصفحة هو الملف نفسه الذي يشغّله `make proxy-check` في Docker. راجع [كيف نختبر هذه الإعدادات](#كيف-نختبر-هذه-الإعدادات). استبدل `dhow.example.com` بنطاقك في كل مكان يظهر فيه.

## ما يحتاجه داو من الوكيل

| المتطلب | السبب |
|---|---|
| اسم نطاق كامل، مثل `dhow.example.com` | كوكي الجلسة اسمه `__Host-dhow_session`. راجع [لا مسار فرعي](#لا-مسار-فرعي). |
| TLS، وضبط `DHOW_PUBLIC_URL` على عنوان `https://` الذي يفتحه المستخدمون | يرفض داو البدء بعنوان عام `http://`، والمتصفحات لا ترسل كوكي الجلسة `Secure` إلا عبر HTTPS. |
| تمرير `Host` و`Origin` بلا تغيير | كل طلب غير `GET` و`HEAD` و`OPTIONS` يجب أن يحمل `Origin` مساويًا لـ `DHOW_PUBLIC_URL`، وإلا يرد داو بـ `403 CSRF_FAILED`. الوكيل الذي يحذف `Origin` أو يغيّره يعطّل الإعداد وتسجيل الدخول وكل تعديل. |
| `X-Forwarded-For` مع إضافة عنوان العميل في آخره، و`X-Forwarded-Proto` | يسجّل داو عنوان العميل في سجل الوصول وفي سجل الأحداث، ويحدّ محاولات الدخول لكل عنوان. لا يقرأ `X-Forwarded-For` إلا من الوكلاء المذكورين في `DHOW_TRUSTED_PROXIES`. |
| حد لحجم الطلب أكبر قليلًا من 1 MiB | يرفض داو الطلب الأكبر من 1 MiB بـ `413 PAYLOAD_TOO_LARGE`. الإعدادات التالية توقف أي طلب أكبر من 2 MB عند الوكيل، وتترك الخطأ الدقيق لداو. |
| بلا تخزين مؤقت للرد (buffering) ومهلة قراءة طويلة لبث السجل | سجلات التشغيلات المباشرة تُبث من `/api/v1/organizations/<id>/jobs/<jobId>/logs/stream` كـ server-sent events. الوكيل الذي يخزّن الرد يؤخر الأحداث حتى يمتلئ المخزن، فتصل السجلات متأخرة ودفعة واحدة. يرسل داو تعليقًا كل 15 ثانية، لكن خطوة بناء هادئة قد تطول أكثر من مهلة قراءة قصيرة. لذلك يأخذ مسار البث ساعة واحدة. ويرسل داو أيضًا `X-Accel-Buffering: no` على البث، وnginx يحترمها وحده. |
| مهلة قراءة أكثر من 25 ثانية على `/runner-api/` | تنتظر أجهزة التشغيل العمل بطلبات طويلة (long polls) تبقى مفتوحة حتى 25 ثانية، وترسل دفعات سجلات حتى 1 MiB. الإعدادات التالية تعطي مسار أجهزة التشغيل 90 ثانية. |
| HSTS | يرسل داو `Strict-Transport-Security` بنفسه. الوكلاء التاليون يضبطونها أيضًا، حتى تظهر في الردود التي يصنعها الوكيل بنفسه، مثل التحويلات والأخطاء أثناء إعادة تشغيل داو. ويستبدلون نسخة داو حتى تظهر الترويسة مرة واحدة بالضبط. |

يستمع داو على `127.0.0.1:8080` افتراضيًا (`DHOW_LISTEN_ADDR`)، و[حزمة Compose](compose.md) تنشره على `127.0.0.1:8080` في الجهاز. إعدادات nginx وCaddy التالية توجّه الطلبات إلى هذا العنوان، فتعمل مع الطريقتين.

## لا مسار فرعي

لا يمكن تشغيل داو تحت مسار مثل `https://example.com/dhow/`. أعطه اسم نطاق خاصًا به، مثل نطاق فرعي.

- كوكي الجلسة يستخدم البادئة `__Host-`. المتصفحات تقبل هذا الكوكي فقط مع `Secure` و`Path=/` وبدون `Domain`، فهو دائمًا للنطاق كله.
- الواجهة وملفاتها والـ API تُخدم من `/` و`/assets/` و`/api/v1/`.
- الترويسة `Origin` التي ترسلها المتصفحات لا تحتوي مسارًا أبدًا، ويقارنها داو بـ `DHOW_PUBLIC_URL`.

## nginx مع certbot

هذا يحتاج إلى nginx 1.25.1 أو أحدث، بسبب التوجيه `http2 on;`. في nginx أقدم، مثل 1.24 في Ubuntu 24.04، احذف السطر `http2 on;` وغيّر `listen 443 ssl;` إلى `listen 443 ssl http2;`.

1. وجّه سجل DNS لـ `dhow.example.com` إلى الجهاز، واسمح بالمنفذين 80 و443 في جدار الحماية.
2. ثبّت nginx وcertbot مع إضافة nginx الخاصة به. في Debian وUbuntu:

   ```sh
   sudo apt install nginx certbot python3-certbot-nginx
   ```

3. احصل على الشهادة بينما يشغّل nginx موقعه الافتراضي. الأمر `certonly` لا يغيّر ملفات nginx:

   ```sh
   sudo certbot certonly --nginx -d dhow.example.com
   ```

4. احفظ هذا في `/etc/nginx/conf.d/dhow.conf`:

```nginx
map $http_upgrade $dhow_connection {
    default upgrade;
    ""      "";
}

server {
    listen 80;
    server_name dhow.example.com;

    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    http2 on;
    server_name dhow.example.com;

    ssl_certificate /etc/letsencrypt/live/dhow.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/dhow.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_cache shared:dhow_tls:10m;
    ssl_session_timeout 1d;

    client_max_body_size 2m;

    proxy_hide_header Strict-Transport-Security;
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains" always;

    proxy_http_version 1.1;
    proxy_set_header Host $http_host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $dhow_connection;

    location / {
        proxy_pass http://127.0.0.1:8080;
    }

    location ~ ^/api/v1/organizations/[^/]+/jobs/[^/]+/logs/stream$ {
        proxy_pass http://127.0.0.1:8080;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 1h;
        proxy_send_timeout 1h;
    }

    location /runner-api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_read_timeout 90s;
        proxy_send_timeout 90s;
    }

    location /ingest/ {
        proxy_pass http://127.0.0.1:8080;
        client_max_body_size 1g;
        proxy_request_buffering off;
        proxy_read_timeout 10m;
        proxy_send_timeout 10m;
    }
}
```

5. افحص الملف وحمّله:

   ```sh
   sudo nginx -t
   sudo systemctl reload nginx
   ```

6. أعد تحميل nginx كلما جدّد certbot الشهادة:

   ```sh
   printf '#!/bin/sh\nsystemctl reload nginx\n' | sudo tee /etc/letsencrypt/renewal-hooks/deploy/reload-nginx
   sudo chmod 755 /etc/letsencrypt/renewal-hooks/deploy/reload-nginx
   sudo certbot renew --dry-run
   ```

7. اضبط `DHOW_PUBLIC_URL=https://dhow.example.com` و`DHOW_TRUSTED_PROXIES` كما في [الوكلاء الموثوقون](#الوكلاء-الموثوقون)، ثم أعد تشغيل داو.

ماذا يفعل هذا الإعداد:

- `proxy_set_header Host $http_host` يمرّر ترويسة `Host` كما أرسلها المتصفح تمامًا، مع المنفذ. أما `$host` فيحذف المنفذ.
- يمرّر nginx الترويسة `Origin` والكوكيز وكل ترويسات الطلب الأخرى بلا تغيير افتراضيًا. لا تضف `proxy_set_header Origin` ولا `proxy_hide_header` لترويسات الطلب.
- `$proxy_add_x_forwarded_for` يضيف عنوان العميل إلى آخر أي `X-Forwarded-For` أرسله العميل. يقرأ داو القائمة من اليمين، فالقيمة المزوّرة في أولها لا تغيّر شيئًا. و`X-Forwarded-Proto` يحمل `https`.
- `client_max_body_size 2m` يوقف الطلبات الكبيرة قبل أن تصل إلى داو.
- الـ location الخاص بـ `/ingest/` يستقبل رفع الملفات الناتجة من CI خارجي. يسمح بـ 1 GB، ويمرّر الجسم أثناء وصوله (`proxy_request_buffering off`)، ويعطي الرفع 10 دقائق. أبقِ `client_max_body_size` هناك مساويًا لـ `DHOW_INGEST_MAX_BYTES` أو أكبر منه، ويرد داو بالخطأ `413` الدقيق.
- الـ location الذي يستخدم تعبيرًا نمطيًا (regular expression) يطابق بث السجل فقط. يوقف `proxy_buffering` ويرفع مهلتي القراءة والإرسال إلى ساعة. هكذا يصل كل حدث إلى المتصفح فورًا، ولا تقطع خطوة بناء هادئة البث. كل الردود الأخرى تبقى مع التخزين المؤقت في nginx.
- الـ location الخاص بـ `/runner-api/` يعطي طلبات أجهزة التشغيل الطويلة (حتى 25 ثانية) ورفع السجلات 90 ثانية. والـ `map` مع ترويستي `Upgrade` و`Connection` يسمحان بترقيات WebSocket أيضًا.
- `proxy_hide_header` و`add_header ... always` يستبدلان `Strict-Transport-Security` من داو بنسخة واحدة تُرسل أيضًا في ردود nginx نفسه.
- لقبول عملاء IPv6، أضف `listen [::]:80;` و`listen [::]:443 ssl;` بجوار أسطر `listen` الخاصة بـ IPv4. فحص الوكيل يعمل بدون IPv6، لذلك هذان السطران ليسا جزءًا من الملف المختبَر.

## Caddy

يحصل Caddy على الشهادة ويجدّدها بنفسه.

1. وجّه سجل DNS لـ `dhow.example.com` إلى الجهاز، واسمح بالمنفذين 80 و443 في جدار الحماية.
2. ثبّت Caddy من [حزمه الرسمية](https://caddyserver.com/docs/install).
3. احفظ هذا في `/etc/caddy/Caddyfile`:

```caddyfile
dhow.example.com {
	header >Strict-Transport-Security "max-age=63072000; includeSubDomains"

	@ingest path /ingest/*
	handle @ingest {
		request_body {
			max_size 1GB
		}

		reverse_proxy 127.0.0.1:8080 {
			transport http {
				read_timeout 10m
			}
		}
	}

	handle {
		request_body {
			max_size 2MB
		}

		@logstream path_regexp ^/api/v1/organizations/[^/]+/jobs/[^/]+/logs/stream$
		reverse_proxy @logstream 127.0.0.1:8080 {
			flush_interval -1
			transport http {
				read_timeout 1h
			}
		}

		reverse_proxy 127.0.0.1:8080 {
			transport http {
				read_timeout 90s
			}
		}
	}
}
```

4. حمّله بـ `sudo systemctl reload caddy`.
5. اضبط `DHOW_PUBLIC_URL=https://dhow.example.com` و`DHOW_TRUSTED_PROXIES` كما في [الوكلاء الموثوقون](#الوكلاء-الموثوقون)، ثم أعد تشغيل داو.

ماذا يفعل هذا الإعداد:

- `reverse_proxy` يمرّر `Host` و`Origin` والكوكيز بلا تغيير، ويضبط `X-Forwarded-For` و`X-Forwarded-Proto` بنفسه. ويستبدل أي `X-Forwarded-For` يرسله العميل، إلا إذا ذكرت هذا العميل في خيار `trusted_proxies` الخاص بـ Caddy.
- `request_body` يوقف الطلبات الأكبر من 2 MB.
- مسار `/ingest/` له `handle` خاص بـ `request_body` حجمه 1 GB و`read_timeout` عشر دقائق، لرفع الملفات الناتجة من CI خارجي. أبقِ هذا الحجم مساويًا لـ `DHOW_INGEST_MAX_BYTES` أو أكبر منه.
- المطابِق `@logstream` يرسل بث السجل عبر `reverse_proxy` خاص به، مع `flush_interval -1` الذي يكتب كل حدث إلى المتصفح فورًا، و`read_timeout 1h` الذي يُبقي البث الهادئ مفتوحًا.
- كل طلب آخر يمر عبر `reverse_proxy` الثاني، ومهلته `read_timeout 90s` تكفي طلبات أجهزة التشغيل الطويلة حتى 25 ثانية على `/runner-api/`.
- `header >Strict-Transport-Security` يضبط HSTS بعد أن يعود الرد من داو، فيستبدل نسخة داو بدل أن يضيف نسخة ثانية.

## Traefik داخل حزمة Compose

يعمل Traefik كخدمة إضافية في [حزمة Compose](compose.md)، ويجد داو عبر Docker labels، ويحصل على الشهادة من Let's Encrypt بتحدي TLS-ALPN.

1. وجّه سجل DNS لـ `dhow.example.com` إلى الجهاز، واسمح بالمنفذين 80 و443 في جدار الحماية. لا يجوز أن يستمع عليهما أي برنامج آخر على الجهاز.
2. احفظ هذا في `compose.traefik.yaml` بجوار `compose.yaml`، واستبدل `dhow.example.com` في قاعدة الـ router:

```yaml
services:
  traefik:
    image: traefik:v3.7.16
    restart: unless-stopped
    read_only: true
    cap_drop: [ALL]
    cap_add: [NET_BIND_SERVICE]
    security_opt: ["no-new-privileges:true"]
    command:
      - --providers.docker=true
      - --providers.docker.exposedbydefault=false
      - --providers.docker.network=dhow_proxy
      - --entrypoints.web.address=:80
      - --entrypoints.web.http.redirections.entrypoint.to=websecure
      - --entrypoints.web.http.redirections.entrypoint.scheme=https
      - --entrypoints.websecure.address=:443
      - --entrypoints.websecure.transport.respondingTimeouts.readTimeout=60s
      - --entrypoints.websecure.transport.respondingTimeouts.writeTimeout=0s
      - --certificatesresolvers.letsencrypt.acme.email=${ACME_EMAIL:?set ACME_EMAIL in .env}
      - --certificatesresolvers.letsencrypt.acme.storage=/letsencrypt/acme.json
      - --certificatesresolvers.letsencrypt.acme.tlschallenge=true
      - --certificatesresolvers.letsencrypt.acme.caserver=${ACME_CA_SERVER:-https://acme-v02.api.letsencrypt.org/directory}
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - letsencrypt:/letsencrypt
    tmpfs:
      - /tmp:mode=1777,size=16m
    networks:
      proxy:
        ipv4_address: 172.30.250.2

  dhow:
    environment:
      DHOW_TRUSTED_PROXIES: 172.30.250.2
    networks: [data, edge, proxy]
    labels:
      - traefik.enable=true
      - traefik.http.routers.dhow.rule=Host(`dhow.example.com`)
      - traefik.http.routers.dhow.entrypoints=websecure
      - traefik.http.routers.dhow.tls.certresolver=letsencrypt
      - traefik.http.routers.dhow.middlewares=dhow-hsts
      - traefik.http.middlewares.dhow-hsts.headers.stsSeconds=63072000
      - traefik.http.middlewares.dhow-hsts.headers.stsIncludeSubdomains=true
      - traefik.http.services.dhow.loadbalancer.server.port=8080

networks:
  proxy:
    name: dhow_proxy
    ipam:
      config:
        - subnet: 172.30.250.0/24

volumes:
  letsencrypt: {}
```

3. أضف هذه الأسطر إلى `.env`. المتغير `COMPOSE_FILE` يجعل كل أمر `docker compose` في المجلد يستخدم الملفين:

   ```sh
   COMPOSE_FILE=compose.yaml:compose.traefik.yaml
   ACME_EMAIL=you@example.com
   DHOW_PUBLIC_URL=https://dhow.example.com
   ```

4. طبّقه بـ `docker compose up -d`.

ماذا يفعل هذا الإعداد:

- يمرّر Traefik الترويسات `Host` و`Origin` والكوكيز بلا تغيير. ويضبط `X-Forwarded-For` و`X-Forwarded-Proto` بنفسه، ويحذف ترويسات `X-Forwarded-*` التي يرسلها العملاء.
- لـ Traefik عنوان ثابت، `172.30.250.2`، على شبكته الخاصة `dhow_proxy`، والملف يضبط `DHOW_TRUSTED_PROXIES` عليه. إذا كان `172.30.250.0/24` مستخدمًا على الجهاز، فغيّر الشبكة الفرعية و`ipv4_address` و`DHOW_TRUSTED_PROXIES` معًا.
- يبث Traefik الردود بلا تخزين مؤقت افتراضيًا، فلا يحتاج بث السجل إلى قاعدة خاصة. `writeTimeout=0s` يضمن ألا يُقطع البث الطويل أبدًا، و`readTimeout=60s` يحدّ الوقت الذي يأخذه العميل لإرسال طلبه، ومنه دفعة سجلات 1 MiB من جهاز تشغيل. لا يضع Traefik مهلة على الرد من داو، فلا تحتاج طلبات أجهزة التشغيل الطويلة على `/runner-api/` إلى شيء أيضًا.
- ليس لدى Traefik حد لحجم الطلب يترك الردود كما هي، فالـ middleware الخاص بالتخزين المؤقت سيخزّن الردود أيضًا ويعطّل بث السجلات. لذلك يعمل حد داو نفسه، 1 MiB.
- الـ middleware المسمى `dhow-hsts` يضبط HSTS، والرد يحملها مرة واحدة.
- القيمة الافتراضية لـ `ACME_CA_SERVER` هي خادم Let's Encrypt الحقيقي. لتجربة الإعداد أولًا، اضبط `ACME_CA_SERVER=https://acme-staging-v02.api.letsencrypt.org/directory` في `.env`. عندما يعمل، احذف هذا السطر، وشغّل `docker compose down`، واحذف الـ volume المسمى `dhow_letsencrypt` حتى تختفي شهادة التجربة، ثم ابدأ مرة أخرى.
- يقرأ Traefik الـ Docker socket ليجد الـ labels. الوصول إلى هذا الـ socket وصول root إلى الجهاز، فثبّت صورة Traefik وحدّثها دائمًا. ثبّتها بالـ digest بالطريقة نفسها التي تثبّت بها [صورة داو](compose.md#تحقق-من-صورة).

## الوكلاء الموثوقون

يأخذ داو عنوان العميل من اتصال TCP. عندما يأتي هذا الاتصال من عنوان في `DHOW_TRUSTED_PROXIES`، يقرأ داو `X-Forwarded-For` من اليمين إلى اليسار، ويأخذ أول عنوان غير موثوق. القيمة قائمة عناوين IP ونطاقات CIDR مفصولة بفواصل.

| الإعداد | `DHOW_TRUSTED_PROXIES` |
|---|---|
| nginx أو Caddy على الجهاز نفسه مع ملف داو التنفيذي على `127.0.0.1:8080` | `127.0.0.1` |
| nginx أو Caddy على الجهاز أمام حزمة Compose | بوابة شبكة `dhow_edge`، ويطبعها `docker network inspect dhow_edge --format '{{(index .IPAM.Config 0).Gateway}}'`، مثل `172.18.0.1` |
| Traefik من `compose.traefik.yaml` | `172.30.250.2`، وهو مضبوط في الملف بالفعل |
| وكيل على جهاز آخر | عنوان ذلك الجهاز كما يراه داو، مثل `10.0.0.5` |
| Cloudflare أمام nginx | عنوان nginx وكل نطاقات Cloudflare. راجع ما يلي |

اضبطه صحيحًا في الاتجاهين:

- **ضيّق جدًا.** إذا لم يكن الوكيل الذي أمام داو في القائمة، فسيبدو أن كل طلب يأتي من الوكيل. يعرض سجل الأحداث عنوان الوكيل لكل المستخدمين، ويحسب حد الدخول محاولات الجميع الفاشلة على هذا العنوان الواحد. بعد 20 محاولة فاشلة من أي شخص، يُقفل الدخول على الجميع، 30 ثانية في البداية وحتى 15 دقيقة.
- **واسع جدًا.** لا تثق أبدًا بنطاق يتصل منه العملاء، مثل `0.0.0.0/0` أو شبكة مكتب كاملة. العميل في نطاق موثوق يستطيع وضع أي عنوان في `X-Forwarded-For`. هذا يخفيه عن حد الدخول، ويكتب عنوانًا مزيفًا في سجل الأحداث.

### سلسلة وكلاء (Cloudflare أمام nginx)

مع CDN أو موزّع حمل أمام وكيلك، يجب أن تثق بكل خطوة بين العميل وداو. يضيف Cloudflare عنوان العميل إلى `X-Forwarded-For`، ويضيف nginx عنوان Cloudflare، ثم يرجع داو عبر nginx وCloudflare حتى يصل إلى العميل. إذا لم تكن نطاقات Cloudflare في القائمة، يتوقف داو عند عنوان من Cloudflare. عندها يشترك كل زائر يصل إليك عبر مركز بيانات Cloudflare نفسه في عنوان واحد، في سجل الأحداث وفي حد الدخول.

ينشر Cloudflare [نطاقاته](https://www.cloudflare.com/ips/). ابنِ القيمة من القوائم الحالية على جهاز داو:

```sh
printf 'DHOW_TRUSTED_PROXIES=127.0.0.1,%s,%s\n' \
  "$(curl -fsS https://www.cloudflare.com/ips-v4 | paste -sd, -)" \
  "$(curl -fsS https://www.cloudflare.com/ips-v6 | paste -sd, -)"
```

استخدم العنوان من الجدول السابق بدل `127.0.0.1` إذا كان nginx يصل إلى داو بطريقة أخرى، وحدّث القائمة عندما يغيّرها Cloudflare. إعداد nginx لا يحتاج إلى تغيير: `$proxy_add_x_forwarded_for` يحتفظ بالعناوين التي أرسلها Cloudflare. العميل الذي يتجاوز Cloudflare ويتصل بـ nginx مباشرة لا يستطيع تزوير عنوانه، لأن nginx يضيف العنوان الحقيقي، وداو لا يثق به.

يستبدل Caddy وTraefik ترويسة `X-Forwarded-For` التي تصل من مرسلين غير موثوقين. لذلك يحتاجان أيضًا إلى نطاقات Cloudflare خلفه: الخيار `trusted_proxies static ...` في كتلة `servers` العامة في Caddy، أو `--entrypoints.websecure.forwardedHeaders.trustedIPs=...` في Traefik. فحص الوكيل يختبر السلسلة مع nginx فقط.

## تحقق من الإعداد

1. **الصحة عبر HTTPS.** يجب أن يطبع هذا `"Status":"ok"`:

   ```sh
   curl -sS https://dhow.example.com/api/v1/health/ready
   ```

2. **ترويسة HSTS واحدة.** يجب أن يطبع هذا سطرًا واحدًا بالضبط:

   ```sh
   curl -sSI https://dhow.example.com/ | grep -i '^strict-transport-security'
   ```

3. **`Origin` يمر.** أرسل طلب إعداد برمز مخترَع. أي رد غير `CSRF_FAILED` يعني أن الوكيل حافظ على `Origin`: `SETUP_TOKEN_INVALID` قبل الإعداد، و`SETUP_COMPLETED` بعده. الطلب لا يغيّر شيئًا:

   ```sh
   curl -sS -X POST https://dhow.example.com/api/v1/setup \
     -H 'Content-Type: application/json' -H 'Origin: https://dhow.example.com' \
     -d '{"SetupToken":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","OwnerName":"Check","Email":"check@example.com","Password":"check-only-password","OrganizationName":"Check","OrganizationSlug":"check"}'
   ```

4. **عنوان العميل.** كل طلب يظهر في سجل داو كسطر `http request` فيه حقل `client_ip`. مع حزمة Compose:

   ```sh
   docker compose logs dhow | grep '"path":"/api/v1/health/ready"' | tail -1
   ```

   يجب أن يكون `client_ip` عنوان الجهاز الذي شغّلت عليه `curl`، كما يراه الإنترنت. إذا كان عنوان الوكيل، فأصلح `DHOW_TRUSTED_PROXIES`.

5. **سجّل الدخول من المتصفح.** افتح `https://dhow.example.com`، وأنشئ حساب صاحب النسخة برابط الإعداد (راجع [أنشئ حساب صاحب النسخة الأول](compose.md#أنشئ-حساب-صاحب-النسخة-الأول)) أو سجّل الدخول. في أدوات المطوّر في المتصفح، يجب أن يظهر الكوكي `__Host-dhow_session` مع `Secure` و`HttpOnly`. وحدث الدخول في سجل الأحداث يجب أن يعرض عنوانك.

## كيف نختبر هذه الإعدادات

`make proxy-check` يبني صورة داو ويشغّل [`deploy/proxy-check/run.sh`](../../deploy/proxy-check/run.sh). يحتاج إلى Docker و`curl` و`openssl`، وإلى المنافذ المحلية الفارغة 18443 و18080 و18081. لكل من nginx، وnginx خلف وكيل ثانٍ يقوم بدور Cloudflare، وCaddy، وTraefik، يشغّل PostgreSQL 18 مع TLS، وداو مع `DHOW_PUBLIC_URL=https://dhow.example.com:18443`، خلف ملفات الإعداد المعروضة في هذه الصفحة. يستخدم nginx شهادة موقّعة ذاتيًا، وCaddy سلطة الشهادات الداخلية الخاصة به، وTraefik شهادة يحصل عليها عبر ACME من خادم [Pebble](https://github.com/letsencrypt/pebble) محلي. ثم يتحقق، عبر الوكيل، من:

- `/api/v1/health/ready` عبر TLS موثّق، وترويسة HSTS واحدة، والتحويل من HTTP إلى HTTPS.
- `POST /api/v1/setup` من `Origin` غريب يرد بـ `CSRF_FAILED`، والإعداد الحقيقي مع `Origin` الصحيح ينجح.
- ترويسات `Host` و`Origin` و`X-Forwarded-For` و`X-Forwarded-Proto` كما تصل إلى socket داو، ونلتقطها بـ `tcpdump` بجوار داو.
- عنوان العميل في سجل الوصول في داو، وفي حدث الدخول في سجل الأحداث، مع `X-Forwarded-For` مزوّر في الطلب.
- خصائص الكوكي `__Host-dhow_session`، ووصول الكوكي إلى داو، ورفض طلب حجمه 3 MiB بـ `413`، ووصول رفع حجمه 3 MiB على `/ingest/` إلى داو.
- رد مسار أجهزة التشغيل عبر الوكيل على `/runner-api/`، ووصول مسار بث السجل إلى داو مع كوكي الجلسة.

يفشل اختبار Go في `deploy/proxy-check` إذا اختلف إعداد في هذه الصفحة عن الملف الذي يشغّله السكربت. الفحص لا يشغّل certbot ولا Let's Encrypt ولا Cloudflare، ويعمل بدون IPv6.
