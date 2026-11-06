[العربية](../../install/reverse-proxy.md)

# Put Dhow behind a reverse proxy

Dhow speaks plain HTTP and expects a reverse proxy in front of it to terminate TLS. This page has complete configurations for [nginx](#nginx-with-certbot), [Caddy](#caddy) and [Traefik](#traefik-in-the-compose-stack), explains [`DHOW_TRUSTED_PROXIES`](#trusted-proxies), and shows [how to check the result](#verify-the-setup).

Every configuration on this page is the exact file that `make proxy-check` runs in Docker, see [How these configurations are tested](#how-these-configurations-are-tested). Replace `dhow.example.com` with your domain everywhere it appears.

## What Dhow needs from a proxy

| Requirement | Why |
|---|---|
| A whole host name, such as `dhow.example.com` | The session cookie is `__Host-dhow_session`. See [No sub-path](#no-sub-path). |
| TLS, and `DHOW_PUBLIC_URL` set to the `https://` address users open | Dhow refuses to start with an `http://` public URL, and browsers only send the `Secure` session cookie over HTTPS. |
| `Host` and `Origin` passed through unchanged | Every request that is not `GET`, `HEAD` or `OPTIONS` must carry an `Origin` equal to `DHOW_PUBLIC_URL`, or Dhow answers `403 CSRF_FAILED`. A proxy that drops or rewrites `Origin` breaks setup, sign-in and every change. |
| `X-Forwarded-For` with the client address appended, and `X-Forwarded-Proto` | Dhow records the client address in the access log and the audit log, and throttles sign-in attempts per address. It reads `X-Forwarded-For` only from the proxies listed in `DHOW_TRUSTED_PROXIES`. |
| A request body limit a little above 1 MiB | Dhow rejects bodies over 1 MiB with `413 PAYLOAD_TOO_LARGE`. The configurations below stop anything over 2 MB at the proxy and leave the exact error to Dhow. |
| No response buffering and a long read timeout on the log stream | Live job logs stream from `/api/v1/organizations/<id>/jobs/<jobId>/logs/stream` as server-sent events. A buffering proxy holds events back until its buffer fills, so logs arrive late and in bursts. Dhow sends a comment every 15 seconds, but a quiet build step can still outlast a short read timeout, so the stream path gets one hour. Dhow also sends `X-Accel-Buffering: no` on the stream, which nginx honors on its own. |
| A read timeout above 25 seconds on `/runner-api/` | Runners wait for work with long polls that hold a request open for up to 25 seconds, and send log batches of up to 1 MiB. The configurations below give the runner plane 90 seconds. |
| HSTS | Dhow sends `Strict-Transport-Security` itself. The proxies below also set it, so it is present on responses the proxy makes on its own, such as redirects and errors while Dhow restarts, and they replace Dhow's copy so the header appears exactly once. |

Dhow listens on `127.0.0.1:8080` by default (`DHOW_LISTEN_ADDR`), and the [Compose stack](compose.md) publishes it on `127.0.0.1:8080` of the host. The nginx and Caddy configurations below proxy to that address, so they work for both.

## No sub-path

Dhow cannot be served under a path such as `https://example.com/dhow/`. Give it its own host name, for example a sub-domain.

- The session cookie uses the `__Host-` prefix. Browsers accept such a cookie only with `Secure`, `Path=/` and no `Domain`, so it always belongs to the whole host.
- The web app, its assets and the API are served from `/`, `/assets/` and `/api/v1/`.
- The `Origin` that browsers send never contains a path, and Dhow compares it with `DHOW_PUBLIC_URL`.

## nginx with certbot

This needs nginx 1.25.1 or newer for the `http2 on;` directive. On an older nginx, such as 1.24 on Ubuntu 24.04, delete the `http2 on;` line and change `listen 443 ssl;` to `listen 443 ssl http2;`.

1. Point the DNS record of `dhow.example.com` at the host and allow ports 80 and 443 in the firewall.
2. Install nginx and certbot with its nginx plugin. On Debian and Ubuntu:

   ```sh
   sudo apt install nginx certbot python3-certbot-nginx
   ```

3. Get the certificate while nginx still runs its default site. `certonly` leaves your nginx files alone:

   ```sh
   sudo certbot certonly --nginx -d dhow.example.com
   ```

4. Save this as `/etc/nginx/conf.d/dhow.conf`:

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

5. Check the file and load it:

   ```sh
   sudo nginx -t
   sudo systemctl reload nginx
   ```

6. Reload nginx whenever certbot renews the certificate:

   ```sh
   printf '#!/bin/sh\nsystemctl reload nginx\n' | sudo tee /etc/letsencrypt/renewal-hooks/deploy/reload-nginx
   sudo chmod 755 /etc/letsencrypt/renewal-hooks/deploy/reload-nginx
   sudo certbot renew --dry-run
   ```

7. Set `DHOW_PUBLIC_URL=https://dhow.example.com` and `DHOW_TRUSTED_PROXIES` as described in [Trusted proxies](#trusted-proxies), then restart Dhow.

What the configuration does:

- `proxy_set_header Host $http_host` forwards the `Host` header exactly as the browser sent it, port included. `$host` would drop the port.
- nginx forwards `Origin`, cookies and every other request header unchanged by default. Do not add `proxy_set_header Origin` or `proxy_hide_header` for request headers.
- `$proxy_add_x_forwarded_for` appends the client address to any `X-Forwarded-For` the client sent. Dhow reads the list from the right, so a forged entry on the left changes nothing. `X-Forwarded-Proto` carries `https`.
- `client_max_body_size 2m` stops oversized bodies before they reach Dhow.
- The `/ingest/` location takes artifact uploads from an outside CI. It allows 1 GB, sends the body on while it arrives (`proxy_request_buffering off`) and gives the upload 10 minutes. Keep `client_max_body_size` there at or above `DHOW_INGEST_MAX_BYTES`, and Dhow gives the exact `413`.
- The regular-expression location matches only the log stream. It turns off `proxy_buffering` and raises the read and send timeouts to one hour, so every event reaches the browser at once and a quiet build step does not cut the stream. Every other response keeps nginx's buffering.
- The `/runner-api/` location gives runner long polls (up to 25 seconds) and log uploads 90 seconds. The `map` and the `Upgrade` and `Connection` headers let WebSocket upgrades through as well.
- `proxy_hide_header` and `add_header ... always` replace Dhow's `Strict-Transport-Security` with one copy that is also sent on nginx's own responses.
- To accept IPv6 clients, add `listen [::]:80;` and `listen [::]:443 ssl;` next to the IPv4 `listen` lines. The proxy check runs without IPv6, so those two lines are not part of the tested file.

## Caddy

Caddy gets and renews the certificate on its own.

1. Point the DNS record of `dhow.example.com` at the host and allow ports 80 and 443 in the firewall.
2. Install Caddy from its [official packages](https://caddyserver.com/docs/install).
3. Save this as `/etc/caddy/Caddyfile`:

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

4. Load it with `sudo systemctl reload caddy`.
5. Set `DHOW_PUBLIC_URL=https://dhow.example.com` and `DHOW_TRUSTED_PROXIES` as described in [Trusted proxies](#trusted-proxies), then restart Dhow.

What the configuration does:

- `reverse_proxy` forwards `Host`, `Origin` and cookies unchanged and sets `X-Forwarded-For` and `X-Forwarded-Proto` itself. It replaces any `X-Forwarded-For` that a client sends, unless you list that client in Caddy's own `trusted_proxies` option.
- `request_body` stops bodies over 2 MB.
- The `/ingest/` path has its own `handle` with a 1 GB `request_body` and a 10 minute `read_timeout`, for artifact uploads from an outside CI. Keep that size at or above `DHOW_INGEST_MAX_BYTES`.
- The `@logstream` matcher sends the log stream through its own `reverse_proxy` with `flush_interval -1`, which writes every event to the browser at once, and `read_timeout 1h`, which keeps a quiet stream open.
- Every other request goes through the second `reverse_proxy`, whose `read_timeout 90s` covers runner long polls of up to 25 seconds on `/runner-api/`.
- `header >Strict-Transport-Security` sets HSTS after the response comes back from Dhow, so it replaces Dhow's copy instead of adding a second one.

## Traefik in the Compose stack

Traefik runs as one more service of the [Compose stack](compose.md), finds Dhow through Docker labels, and gets the certificate from Let's Encrypt with the TLS-ALPN challenge.

1. Point the DNS record of `dhow.example.com` at the host and allow ports 80 and 443 in the firewall. Nothing else on the host may listen on them.
2. Save this as `compose.traefik.yaml` next to `compose.yaml`, and replace `dhow.example.com` in the router rule:

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

3. Add these lines to `.env`. `COMPOSE_FILE` makes every `docker compose` command in the directory use both files:

   ```sh
   COMPOSE_FILE=compose.yaml:compose.traefik.yaml
   ACME_EMAIL=you@example.com
   DHOW_PUBLIC_URL=https://dhow.example.com
   ```

4. Apply it with `docker compose up -d`.

What the configuration does:

- Traefik passes `Host`, `Origin` and cookies through unchanged. It sets `X-Forwarded-For` and `X-Forwarded-Proto` itself and drops the `X-Forwarded-*` headers that clients send.
- Traefik has a fixed address, `172.30.250.2`, on its own `dhow_proxy` network, and the file sets `DHOW_TRUSTED_PROXIES` to it. If `172.30.250.0/24` is already used on the host, change the subnet, `ipv4_address` and `DHOW_TRUSTED_PROXIES` together.
- Traefik streams responses without buffering by default, so the log stream needs no rule of its own. `writeTimeout=0s` makes sure a long stream is never cut, and `readTimeout=60s` limits how long a client may take to send its request, a 1 MiB log batch from a runner included. Traefik sets no timeout on the response from Dhow, so runner long polls on `/runner-api/` need nothing either.
- Traefik has no body limit of its own that leaves responses alone: its buffering middleware would also buffer responses and break log streams. Dhow's own 1 MiB limit applies instead.
- The `dhow-hsts` middleware sets HSTS, and the response carries it once.
- `ACME_CA_SERVER` defaults to Let's Encrypt production. To try the setup first, set `ACME_CA_SERVER=https://acme-staging-v02.api.letsencrypt.org/directory` in `.env`. When it works, remove that line, run `docker compose down`, delete the `dhow_letsencrypt` volume so the staging certificate is gone, and start again.
- Traefik reads the Docker socket to find the labels. Access to that socket is root access to the host, so keep the Traefik image pinned and up to date. Pin it by digest the same way as [the Dhow image](compose.md#verify-an-image).

## Trusted proxies

Dhow takes the client address from the TCP connection. When that connection comes from an address in `DHOW_TRUSTED_PROXIES`, Dhow reads `X-Forwarded-For` from right to left and takes the first address that is not trusted. The value is a comma-separated list of IP addresses and CIDR ranges.

| Setup | `DHOW_TRUSTED_PROXIES` |
|---|---|
| nginx or Caddy on the same host as a Dhow binary on `127.0.0.1:8080` | `127.0.0.1` |
| nginx or Caddy on the host in front of the Compose stack | The gateway of the `dhow_edge` network, printed by `docker network inspect dhow_edge --format '{{(index .IPAM.Config 0).Gateway}}'`, for example `172.18.0.1` |
| Traefik from `compose.traefik.yaml` | `172.30.250.2`, already set in the file |
| A proxy on another machine | That machine's address as Dhow sees it, for example `10.0.0.5` |
| Cloudflare in front of nginx | The nginx address and every Cloudflare range, see below |

Get it right in both directions:

- **Too narrow.** If the proxy in front of Dhow is missing from the list, every request seems to come from the proxy. The audit log shows the proxy's address for every user, and the sign-in throttle counts everyone's failed attempts against that one address: after 20 failures from anyone, sign-in is locked for everyone, for 30 seconds at first and up to 15 minutes.
- **Too wide.** Never trust a range that clients can connect from, such as `0.0.0.0/0` or a whole office network. A client in a trusted range can put any address in `X-Forwarded-For`, which hides it from the throttle and writes a false address into the audit log.

### A chain of proxies (Cloudflare in front of nginx)

With a CDN or load balancer in front of your proxy, every hop between the client and Dhow must be trusted. Cloudflare appends the client address to `X-Forwarded-For`, nginx appends Cloudflare's address, and Dhow walks back over nginx and Cloudflare to the client. If Cloudflare's ranges are missing, Dhow stops at a Cloudflare address, and every visitor who reaches you through the same Cloudflare data center shares one address in the audit log and the throttle.

Cloudflare [publishes its ranges](https://www.cloudflare.com/ips/). Build the value from the current lists on the Dhow host:

```sh
printf 'DHOW_TRUSTED_PROXIES=127.0.0.1,%s,%s\n' \
  "$(curl -fsS https://www.cloudflare.com/ips-v4 | paste -sd, -)" \
  "$(curl -fsS https://www.cloudflare.com/ips-v6 | paste -sd, -)"
```

Use the address from the table above instead of `127.0.0.1` if nginx reaches Dhow another way, and refresh the list when Cloudflare changes it. The nginx configuration needs no change: `$proxy_add_x_forwarded_for` keeps the addresses Cloudflare sent. A client that bypasses Cloudflare and connects to nginx directly cannot forge its address, because nginx appends the real one and Dhow does not trust it.

Caddy and Traefik replace the `X-Forwarded-For` they receive from untrusted senders, so behind Cloudflare they need the Cloudflare ranges too: the `trusted_proxies static ...` option in Caddy's global `servers` block, or `--entrypoints.websecure.forwardedHeaders.trustedIPs=...` in Traefik. The proxy check covers the chain with nginx only.

## Verify the setup

1. **Health over HTTPS.** This must print `"Status":"ok"`:

   ```sh
   curl -sS https://dhow.example.com/api/v1/health/ready
   ```

2. **One HSTS header.** This must print exactly one line:

   ```sh
   curl -sSI https://dhow.example.com/ | grep -i '^strict-transport-security'
   ```

3. **`Origin` gets through.** Send a setup request with a made-up token. Any answer except `CSRF_FAILED` means the proxy kept `Origin`: `SETUP_TOKEN_INVALID` before setup, `SETUP_COMPLETED` after it. The request changes nothing:

   ```sh
   curl -sS -X POST https://dhow.example.com/api/v1/setup \
     -H 'Content-Type: application/json' -H 'Origin: https://dhow.example.com' \
     -d '{"SetupToken":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","OwnerName":"Check","Email":"check@example.com","Password":"check-only-password","OrganizationName":"Check","OrganizationSlug":"check"}'
   ```

4. **The client address.** Every request appears in Dhow's log as an `http request` line with a `client_ip` field. With the Compose stack:

   ```sh
   docker compose logs dhow | grep '"path":"/api/v1/health/ready"' | tail -1
   ```

   `client_ip` must be the address of the machine you ran `curl` on, as the internet sees it. If it is the proxy's address, fix `DHOW_TRUSTED_PROXIES`.

5. **Sign in from a browser.** Open `https://dhow.example.com`, create the owner with the setup link (see [Create the first owner](compose.md#create-the-first-owner)) or sign in. In the browser's developer tools, the cookie `__Host-dhow_session` must be listed with `Secure` and `HttpOnly`. The sign-in event in the audit log must show your address.

## How these configurations are tested

`make proxy-check` builds the Dhow image and runs [`deploy/proxy-check/run.sh`](../../../deploy/proxy-check/run.sh). It needs Docker, `curl` and `openssl`, and the free local ports 18443, 18080 and 18081. For each of nginx, nginx behind a second proxy that stands in for Cloudflare, Caddy and Traefik, it starts PostgreSQL 18 with TLS and Dhow with `DHOW_PUBLIC_URL=https://dhow.example.com:18443`, behind the configuration files shown on this page. nginx uses a self-signed certificate, Caddy its internal CA, and Traefik a certificate it gets over ACME from a local [Pebble](https://github.com/letsencrypt/pebble) server. Then it checks, through the proxy:

- `/api/v1/health/ready` over verified TLS, one HSTS header, and the HTTP to HTTPS redirect.
- `POST /api/v1/setup` from a foreign `Origin` is `CSRF_FAILED`, and the real setup with the right `Origin` succeeds.
- The `Host`, `Origin`, `X-Forwarded-For` and `X-Forwarded-Proto` headers as they reach Dhow's socket, captured with `tcpdump` next to Dhow.
- The client address in Dhow's access log and in the audit event of a sign-in, with a forged `X-Forwarded-For` in the request.
- The `__Host-dhow_session` cookie attributes, the cookie reaching Dhow, a 3 MiB body refused with `413`, and a 3 MiB upload on `/ingest/` that reaches Dhow.
- The runner plane answering through the proxy on `/runner-api/`, and the log stream path reaching Dhow with the session cookie.

A Go test in `deploy/proxy-check` fails if a configuration on this page differs from the file the script runs. The check does not run certbot, Let's Encrypt or Cloudflare, and it runs without IPv6.
