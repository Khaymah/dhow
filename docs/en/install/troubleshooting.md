[العربية](../../install/troubleshooting.md)

# Troubleshooting

Dhow writes JSON lines to standard output. With the Compose stack, read them with `docker compose logs dhow`. Every request appears as an `http request` line with its `path`, `status` and `client_ip`, and every error answer has a `Code` such as `CSRF_FAILED` in its body.

## Dhow does not start

Dhow checks its whole configuration before it starts and prints every problem, then exits:

```text
dhow-server: invalid configuration:
DHOW_SECRET_KEY: is required; generate one with `openssl rand -hex 32`
DHOW_PUBLIC_URL: must use https (http is allowed only with DHOW_DEV=true)
```

[Configuration](configuration.md) lists the rules for each variable.

### `DHOW_PUBLIC_URL: must use https`

The public URL is the address users open, and it must be `https://`. Dhow itself speaks plain HTTP, so TLS belongs to the reverse proxy in front of it: set `DHOW_PUBLIC_URL=https://dhow.example.com` and follow [Put Dhow behind a reverse proxy](reverse-proxy.md). Do not set `DHOW_DEV=true` to get around this on a server. That mode is only for [running Dhow on your own machine](local.md).

### `DHOW_DATABASE_URL: must set sslmode=require, verify-ca or verify-full`

Dhow refuses database connections that may fall back to plain text. Add `sslmode` to the URL:

- `sslmode=require` encrypts the connection without checking the server's certificate. The Compose stack uses it on its private network.
- `sslmode=verify-full` also checks the certificate and the host name. Use it for a database on another machine or a managed service, with `sslrootcert=/path/to/ca.pem` when the CA is not in the system trust store.

If the URL already contains a `?`, add `&sslmode=verify-full` instead.

### `connect to database: ... server refused TLS connection`

The URL asks for TLS but PostgreSQL has it turned off. Enable `ssl = on` with a certificate and key in `postgresql.conf` and restart PostgreSQL. Managed PostgreSQL services have TLS on already.

### `connect to database: ... tls: failed to verify certificate`

With `verify-ca` or `verify-full`, Dhow could not verify the server's certificate:

- `certificate signed by unknown authority`: point `sslrootcert` at the CA that signed the server's certificate.
- `certificate is not valid for any names, but wanted to match ...` or `... is not valid for ...`: the host name in the URL is not in the certificate. Connect with the name the certificate was issued for, or use `verify-ca`, which checks the CA but not the name.

### `SCHEMA_TOO_NEW`

A newer release upgraded the database, and one of its migrations raised the floor above this binary's version. Dhow exits with code `78`, and the message names the version it needs. Install that version or a newer one, or restore the backup you took before the upgrade. The details are in [Upgrades and rolling back](upgrades.md).

## Setup or sign-in fails with `CSRF_FAILED`

Every request that is not `GET`, `HEAD` or `OPTIONS` must have an `Origin` header equal to `DHOW_PUBLIC_URL`. When it does not, Dhow answers `403` with `CSRF_FAILED` and "The request could not be verified. Reload the page and try again."

Compare the address in the browser's address bar with `DHOW_PUBLIC_URL`, character for character:

| In the browser | `DHOW_PUBLIC_URL` | Result |
|---|---|---|
| `https://dhow.example.com` | `https://dhow.example.com` | Works. A trailing `/` and the default port `:443` do not matter. |
| `https://www.dhow.example.com` | `https://dhow.example.com` | `CSRF_FAILED`: another host name. |
| `https://dhow.example.com:8443` | `https://dhow.example.com` | `CSRF_FAILED`: another port. |
| `http://127.0.0.1:8080` | `https://dhow.example.com` | `CSRF_FAILED`: you opened Dhow directly, past the proxy. |
| `http://127.0.0.1:8080` | `http://localhost:8080` | `CSRF_FAILED`: `localhost` and `127.0.0.1` are different origins. |
| `https://example.com/dhow/` | `https://example.com/dhow` | Does not work: Dhow cannot run [under a path](reverse-proxy.md#no-sub-path). |

If the addresses match, the reverse proxy is dropping or rewriting `Origin`. The configurations in [Put Dhow behind a reverse proxy](reverse-proxy.md) pass it through unchanged. Look for a `proxy_set_header Origin` in nginx, a `header_up Origin` in Caddy, or a headers middleware in Traefik. Step 3 of [Verify the setup](reverse-proxy.md#verify-the-setup) tests this with `curl`.

After sign-in, the same error also comes back when a request lacks the `X-CSRF-Token` header that the web app sends. That happens when a proxy strips the header, or with a script that sends the session cookie without the token from `GET /api/v1/me`.

## Sign-in succeeds but you are not signed in

The sign-in request answers `200`, yet the next page asks you to sign in again. The browser did not store the session cookie `__Host-dhow_session`. The browser's developer tools show the `Set-Cookie` header with a warning next to it.

- **Plain HTTP.** The cookie is always `Secure`, so browsers store it only over HTTPS, or on `http://localhost` and `http://127.0.0.1`, which Chrome and Firefox treat as secure. It is never stored over plain HTTP on another address, such as `http://192.168.1.20:8080`. Use the HTTPS address through the proxy.
- **A sub-path.** The `__Host-` prefix requires `Path=/`, so Dhow cannot work under `https://example.com/dhow/`. Give it its own host name.
- **A shared domain.** The cookie has no `Domain`, so it belongs to exactly the host that set it and is never shared with other sub-domains. That is by design. Open Dhow on the host name of `DHOW_PUBLIC_URL`.
- **A proxy that changes cookies.** Remove any `proxy_cookie_path`, `proxy_cookie_domain` or `proxy_cookie_flags` in nginx, or any rule that rewrites `Set-Cookie` in other proxies, for the Dhow site.

## The audit log shows the proxy's address for everyone

The sign-in events in the audit log and the `client_ip` field in the server log show the same address, such as `127.0.0.1` or `172.18.0.1`, for every user. Dhow does not trust the proxy in front of it, so it uses the address of the connection, which is the proxy.

Add the proxy's address, as Dhow sees it, to `DHOW_TRUSTED_PROXIES` and restart Dhow. [Trusted proxies](reverse-proxy.md#trusted-proxies) has the value for each setup. With the Compose stack and a proxy on the host, it is the Docker bridge gateway, not `127.0.0.1`.

If the address shown belongs to a CDN such as Cloudflare, add the CDN's ranges too, see [A chain of proxies](reverse-proxy.md#a-chain-of-proxies-cloudflare-in-front-of-nginx).

Events written before the fix keep the address they were recorded with: the audit log cannot be changed.

## Nobody can sign in, even with the right password

Every sign-in answers `INVALID_CREDENTIALS` with a `Retry-After` header, for every user at the same time. Dhow throttles failed sign-ins per account and per client address: after 20 failures from one address, that address is locked for 30 seconds, and each further failure doubles the lock up to 15 minutes.

When `DHOW_TRUSTED_PROXIES` is missing the proxy, all users share the proxy's address, so a few people mistyping their passwords lock out everybody. Check the server log: if every `POST /api/v1/sessions` line has the same `client_ip`, fix `DHOW_TRUSTED_PROXIES` as in [the previous section](#the-audit-log-shows-the-proxys-address-for-everyone) and restart Dhow. The lock on the proxy's address then no longer applies to anyone. Locks on single accounts, after 5 failures for the same account, stay until they expire. Every lock expires within 15 minutes, and attempts made while a lock is active never extend it.

## `429 RATE_LIMITED` on sign-in pages or invitations

The answer carries `Retry-After`, the seconds until the counter starts again. Dhow limits the public sign-in routes per client network and invitations per member and per organization, see [Rate limits](configuration.md#rate-limits).

- **Everyone gets it at once.** All users probably share one address: fix `DHOW_TRUSTED_PROXIES` as in [The audit log shows the proxy's address for everyone](#the-audit-log-shows-the-proxys-address-for-everyone).
- **A large office behind one NAT.** Raise `DHOW_RATE_LIMIT_PUBLIC` and restart Dhow.
- **Inviting a whole team.** Raise `DHOW_RATE_LIMIT_INVITES_USER` and `DHOW_RATE_LIMIT_INVITES_ORG`, or wait for the next hour. An invitation that answers `INVITATION_SEND_LIMIT` was sent 5 times: revoke it and invite again.

## Finding the setup link

A new instance has no users. At every start without an owner, Dhow logs a one-time link that is valid for one hour:

```sh
docker compose logs dhow | grep setup_url
```

Without Compose, look for the `setup_url` field in the server's output. The link is `DHOW_PUBLIC_URL` followed by `/setup#` and the token, so it only works if the public URL is right and the proxy is in place.

- **The hour is over, or the link is lost.** Restart Dhow, for example with `docker compose restart dhow`, to get a new link. The old one stops working at once.
- **`SETUP_TOKEN_INVALID`.** The link has expired, was replaced by a newer one, or was copied without the part after `#`.
- **`SETUP_COMPLETED`.** Setup is done and the link is gone for good. Sign in with the owner account.
- **No `setup_url` line at all.** The instance already has an owner, so there is nothing to set up.

[Create the first owner](compose.md#create-the-first-owner) describes the setup page and the equivalent API call.
