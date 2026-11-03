[العربية](../../install/configuration.md)

# Configuration

Dhow has two kinds of settings ([ADR-0063](../../../Plan/ADR/0063-instance-settings-in-the-database.md)):

- **Server variables** are what the server needs before it reaches the database, or what belongs to one machine: the database, the secret key, the public address, listeners, proxies and per-instance capacity. They live only in the environment.
- **Dashboard settings** are mail, who may create organizations, the breached-password check, rate limits and log limits. The instance owner changes them on **Server settings** (`/system/settings`) without a restart. Each one can still be set by an environment variable, and then the variable wins and locks it. See [Who wins](#who-wins) and the guide [Server settings](../guides/instance-settings.md).

Dhow reads the environment when it starts. It checks every variable first and refuses to start if any is invalid, listing every problem at once:

```text
dhow-server: invalid configuration:
DHOW_SECRET_KEY: is required; generate one with `openssl rand -hex 32`
DHOW_PUBLIC_URL: must use https (http is allowed only with DHOW_DEV=true)
```

There is no configuration file and no fallback value for a secret. A missing or weak secret stops the server instead of starting it insecurely.

## Server variables

| Variable | Required | Default | Example | Purpose and validation |
|---|---|---|---|---|
| `DHOW_DATABASE_URL` | Yes | none | `postgres://dhow:<password>@db.internal:5432/dhow?sslmode=verify-full` | The PostgreSQL 18 database. It must be a `postgres://` or `postgresql://` URL that names a host, either in the authority or as a `host` query parameter. Outside development, `sslmode` must be `require`, `verify-ca` or `verify-full`; `verify-full` also checks the server's certificate and name. Other [libpq parameters](https://www.postgresql.org/docs/18/libpq-connect.html#LIBPQ-PARAMKEYWORDS) are accepted, such as `sslrootcert`. Dhow opens at most 20 connections and waits 10 seconds for the first one. |
| `DHOW_SECRET_KEY` | Yes | none | the output of `openssl rand -hex 32` | Encrypts stored secrets and signs CSRF tokens. It must be hex or base64 (standard or URL-safe, padded or not) and decode to at least 32 bytes with at least 16 different byte values. A value that contains `change`, `replace`, `example`, `placeholder`, `secret`, `password`, `default` or `dummy`, in any case, is refused as a placeholder. Back it up with the database: a different key cannot read the secrets stored with the old one. |
| `DHOW_SECRET_KEY_FILE` | No | none | `/run/secrets/dhow-secret-key` | A file that holds `DHOW_SECRET_KEY`, see [Secrets in files](#secrets-in-files). |
| `DHOW_DATABASE_URL_FILE` | No | none | `/run/secrets/dhow-database-url` | A file that holds `DHOW_DATABASE_URL`, see [Secrets in files](#secrets-in-files). |
| `DHOW_PUBLIC_URL` | Yes | none | `https://dhow.example.com` | The address users open in the browser. It must be an absolute `https://` URL without credentials, a query or a fragment, and a trailing `/` is removed. `http://` is accepted only with `DHOW_DEV=true`. Every request that is not `GET`, `HEAD` or `OPTIONS` must carry an `Origin` equal to this value, and the setup link is built from it. A path is accepted by the check but [not supported](reverse-proxy.md#no-sub-path). |
| `DHOW_LISTEN_ADDR` | No | `127.0.0.1:8080` | `0.0.0.0:8080`, `[::1]:8080` | The address and port Dhow listens on for plain HTTP. It must be `host:port` with a valid port, and IPv6 addresses go in brackets. The container image sets `0.0.0.0:8080`. `dhow-server healthcheck` reads it too. |
| `DHOW_TRUSTED_PROXIES` | No | empty | `127.0.0.1`, `10.0.0.5,172.18.0.1/32` | Reverse proxies whose `X-Forwarded-For` header Dhow trusts, as a comma-separated list of IP addresses and CIDR ranges. Spaces around entries are ignored, and an IPv4-mapped IPv6 address counts as IPv4. Empty means Dhow uses the address of the TCP connection. See [Trusted proxies](reverse-proxy.md#trusted-proxies). |
| `DHOW_LOG_LEVEL` | No | `info` | `debug` | `debug`, `info`, `warn` (or `warning`) or `error`, in any case. Logs are JSON lines on standard output. |
| `DHOW_DEV` | No | `false` | `true` | Development mode. It allows an `http://` public URL and a database URL without a secure `sslmode`, and Dhow logs a warning at every start. It accepts `true` or `false` and the other spellings of Go's `strconv.ParseBool` (`1`, `0`, `t`, `f`, `TRUE`, `FALSE`, ...). Never set it on a host that other people can reach, see [Run Dhow locally](local.md). |
| `DHOW_RUNNER_LISTEN_ADDR` | No | empty | `0.0.0.0:7443` | Moves the runner plane (`/runner-api/`, where runners register and sign in) off `DHOW_LISTEN_ADDR` onto a listener of its own, so a firewall or a Kubernetes NetworkPolicy can separate it. The path stays `/runner-api/`. Empty keeps the runner plane on the main listener. It must be `host:port` and differ from `DHOW_LISTEN_ADDR`. It needs both TLS files below, except with `DHOW_DEV=true`, where it serves plain HTTP and logs a warning. |
| `DHOW_RUNNER_TLS_CERT_FILE` | With `DHOW_RUNNER_LISTEN_ADDR` | none | `/etc/dhow/runner-tls.crt` | The PEM certificate chain of the separate runner listener. It is refused without `DHOW_RUNNER_LISTEN_ADDR`, without `DHOW_RUNNER_TLS_KEY_FILE`, or when the pair does not load. |
| `DHOW_RUNNER_TLS_KEY_FILE` | With `DHOW_RUNNER_LISTEN_ADDR` | none | `/etc/dhow/runner-tls.key` | The PEM private key of that certificate. Like a secret file, other users must not be able to read or write it (`chmod 600`). |
| `DHOW_RUNNER_MAX_POLLS` | No | `500` | `200` | How many `Lease` long-polls one server instance keeps open at once. A waiting poll holds no database connection, but every wake-up runs one short claim transaction, so the default is 25 polls for each of the 20 pooled database connections. Above the limit a runner gets `RESOURCE_EXHAUSTED` with `Retry-After` and backs off. A whole number from 1 to 10000. |
| `DHOW_SSE_MAX_STREAMS` | No | `2000` | `500` | How many live log streams (server-sent events) one server instance serves at once. One user may hold 10 of them on an instance, and one organization a tenth of this value (at least 20). Above either limit the browser gets `429 TOO_MANY_STREAMS` with `Retry-After`. A whole number from 1 to 100000. |
| `DHOW_LOG_READ_CONNS` | No | `4` | `8` | Database connections set aside for reading job logs: history pages and live streams. They are a pool of their own, next to the 20 connections of the API and the runner plane, so many open streams can slow each other down but never the rest of Dhow. Each statement on them stops after 5 seconds. A whole number from 1 to 20. |
| `DHOW_WEBHOOK_MAX_BYTES` | No | `5242880` | `10485760` | The largest body of a webhook request from a Git provider on `/webhooks/`. A larger one gets `413` before it is read and before its signature is checked. A whole number of bytes from 65536 (64 KiB) to 26214400 (25 MiB). |
| `DHOW_BLOB_DIR` | No | `/var/lib/dhow/blobs` | `/srv/dhow/blobs` | Where artifacts and cache archives are kept, each named by its SHA-256. It must exist, belong to the server's user and be closed to everyone else (`0700`), or the server does not start. With `DHOW_DEV=true` and no value, a folder under `/tmp` is used and created. Back this folder up with the database. |
| `DHOW_ARTIFACT_MAX_BYTES` | No | `1073741824` | `5368709120` | The largest single artifact. A larger upload stops in the middle with `413`, before it fills the disk. A whole number of bytes from 1048576 (1 MiB) to 68719476736 (64 GiB). |
| `DHOW_ARTIFACT_RETENTION_DAYS` | No | `30` | `14` | How many days artifacts are kept. A `retention` in the pipeline file can only be shorter. From 1 to 365. |
| `DHOW_CACHE_MAX_BYTES_PER_PROJECT` | No | `5368709120` | `10737418240` | The most cache space a project gets. Above it, the least recently used entries are removed. A whole number of bytes from 16777216 (16 MiB) to 1099511627776 (1 TiB). |
| `DHOW_ARTIFACT_MAX_BYTES_PER_ORG` | No | `53687091200` | `107374182400` | The most space the artifacts of one organization may take together. An upload that does not fit is refused before it is read, and a second check runs when the artifact is recorded. The artifacts of fork runs of one project get at most a tenth of it. A whole number of bytes from 16777216 (16 MiB) to 1125899906842624 (1 PiB). |
| `DHOW_GIT_ALLOW_PRIVATE_CIDRS` | No | empty | `10.20.0.0/16,fd12:3456::/32` | Private network ranges Dhow may connect to when it fetches the host key of a Git repository, such as a Gitea or Forgejo server on your internal network, or when it asks an image registry for the digest of a tag at the start of a run. Without it, loopback, private, link-local and CGNAT addresses are refused, also when they come from DNS or a redirect. A comma-separated list of CIDRs, at most 64, none wider than `/8` for IPv4 or `/16` for IPv6. Cloud metadata addresses (such as `169.254.169.254`) are always refused, even inside an allowed range. |
| `DHOW_SMTP_PASSWORD_FILE` | No | none | `/run/secrets/dhow-smtp-password` | A file that holds `DHOW_SMTP_PASSWORD`, see [Secrets in files](#secrets-in-files). |
| `DHOW_OIDC_PROVIDERS` | No | empty | `google,github` | Sets the sign-in providers in the environment. A comma-separated list of provider ids (lowercase letters and digits, starting with a letter, at most 32, at most 10 providers). Each id needs its own `DHOW_OIDC_<ID>_*` variables, see [Sign-in providers](#sign-in-providers). When it is set, the whole list comes from the environment and **Server settings** shows it read-only. When it is empty, the instance owner manages the providers on **Server settings → Sign-in providers**. |

`DHOW_DEV=true` never turns off the `Secure` flag of the session cookie, the `Origin` check or the secret key checks.

## Dashboard settings

The instance owner changes these on **Server settings** in the dashboard. Each one also has an environment variable. When the variable is set, it wins: the dashboard shows the value as read-only with the variable's name, and the API answers `409 SETTING_LOCKED_BY_ENV`. The same checks run on the variable at start-up and on every save from the dashboard, so a value that would stop the server is never saved.

| Variable | Dashboard | Default | Example | Purpose and validation |
|---|---|---|---|---|
| `DHOW_SMTP_HOST` | Mail → Host | empty | `smtp.example.com` | The mail server for invitation emails. Empty turns email off: an admin still gets the invitation link and sends it themselves. A host name or an IP address, without a port. See [Email](#email). A link-local address (`169.254.0.0/16`, `fe80::/10`) or a cloud metadata name or address (such as `metadata.google.internal` or `100.100.100.200`) is refused, from the environment and from the dashboard. Loopback and private addresses are accepted, because a local relay is common. Dhow connects to whatever host the owner sets, so treat it as trusted by the owner. |
| `DHOW_SMTP_PORT` | Mail → Port | `587`, or `465` with `DHOW_SMTP_TLS=tls` | `2525` | The mail server's port, from 1 to 65535. |
| `DHOW_SMTP_TLS` | Mail → TLS | `starttls` | `tls`, `none` | `starttls` connects in plain text and requires `STARTTLS` before anything else is sent. `tls` connects over TLS from the first byte (usually port 465). `none` sends in plain text and is accepted only without a username and password, for a relay on a private network. The server certificate is always verified. From the dashboard, `none` is accepted only for a host on the same machine (`localhost`, `127.0.0.0/8` or `::1`), because invitation links would travel in clear text. The environment variable still accepts `none` for any host, so existing installs keep working. |
| `DHOW_SMTP_USERNAME` | Mail → Username | empty | `dhow` | The SMTP login. Set it together with a password, or leave both empty for a relay that needs no login. |
| `DHOW_SMTP_PASSWORD` | Mail → Password | empty | none | The SMTP password. It is sent only over TLS (`AUTH PLAIN`) and never logged. |
| `DHOW_SMTP_FROM` | Mail → Sender | none | `Dhow <dhow@example.com>` | The sender of invitation emails, as one RFC 5322 address with an optional display name. |
| `DHOW_ORG_CREATION` | Organizations → Who may create | `instance-owner` | `any-user` | Who may create more organizations. `instance-owner` allows only the instance owner, the account made by the first-owner setup. `any-user` allows every signed-in user. Any other value stops the server. |
| `DHOW_BREACHED_PASSWORD_CHECK` | Passwords → Breached-password check | `true` | `false` | Rejects new passwords that appear in known breaches, using the Have I Been Pwned range API. Only the first 5 hex characters of the password's SHA-1 leave the server, and the check is skipped with a warning when the service cannot be reached within 3 seconds. Set `false` on networks without internet access. It accepts the same spellings as `DHOW_DEV`. |
| `DHOW_RATE_LIMIT_PUBLIC` | Rate limits → Public sign-in | `300` | `1000` | How many requests one client network may send in 10 minutes to each group of public sign-in routes: password sign-in, the second step, passkey sign-in and its options, OIDC authorize and callback, invitation preview and accept, and the first-owner setup. Only requests that pass the `Origin` check count, so a page on another site cannot spend the limit of your users. A client network is one IPv4 address or one IPv6 `/64`, found as described below. Above it the answer is `429 RATE_LIMITED` with `Retry-After`. The counters live in the database, so every instance shares them. Raise it when many people sign in from behind one NAT. A whole number from 1 to 100000, see [Rate limits](#rate-limits). |
| `DHOW_RATE_LIMIT_INVITES_USER` | Rate limits → Invitations per member | `50` | `200` | How many invitations one member may create or resend in an hour, across all organizations. A whole number from 1 to 100000. |
| `DHOW_RATE_LIMIT_INVITES_ORG` | Rate limits → Invitations per organization | `200` | `1000` | How many invitations one organization may create or resend in an hour, whoever sends them. A whole number from 1 to 100000. |
| `DHOW_LOG_RETENTION_DAYS` | Logs → Days to keep | `30` | `7` | How many days of job logs to keep. Logs are stored in one PostgreSQL partition per UTC day, and every hour each instance drops the partitions older than this, under an advisory lock so instances never race. A whole number from 1 to 3650. An organization owner can keep that organization's logs for fewer days (7 or more) with `PUT /organizations/{id}/log-retention`, with a fresh second factor; the change and its audit entry are written together. |
| `DHOW_JOB_LOG_MAX_BYTES` | Logs → Largest attempt log | `67108864` (64 MiB) | `268435456` | The most log output one job attempt may store, in bytes. The chunk that crosses it is cut at the limit, one `system` line says the log was truncated, and later output is dropped while the job keeps running. A whole number from 1048576 (1 MiB) to 1073741824 (1 GiB), without a unit. Logs live in PostgreSQL until log segments move to object storage, so this limit and the retention decide how much they add to the database and its backups. |

## Who wins

Each dashboard setting is resolved on its own, in this order:

| Set in the environment | Saved in the dashboard | In effect | Dashboard shows |
|---|---|---|---|
| Yes | Yes or no | The environment variable | The value, read-only, with the variable's name |
| No | Yes | The saved value | The value, editable |
| No | No | The documented default | The default, editable |

- **Nothing is copied.** Dhow never copies an environment value into the database. If you remove a variable, the setting unlocks and takes the saved value, or the default.
- **No restart.** A save notifies every instance on the same database (`pg_notify` on `dhow_settings_changed`), and each one reloads within a moment. Mail, rate limits, organization creation, the breached-password check and log retention take effect at once. A new largest attempt log applies to attempts that start after the save.
- **A failed reload is visible.** If an instance cannot reload (the database is unreachable, or a saved value was edited by hand and is not valid), it keeps the previous values, retries with a backoff of 1 second doubling up to 30 seconds, and fails the `settings` readiness check until a reload succeeds. `/api/v1/health/ready` answers 503 meanwhile, so a load balancer stops sending it traffic instead of letting it serve stale security settings. **Server status** and `dhow-server doctor` show the check too.
- **Fail closed.** At start-up, a saved value that is not valid (for example after a hand edit in the database) stops the server, and the error names the variable and where the value came from. A saved secret that the current `DHOW_SECRET_KEY` cannot decrypt stops it too.
- **Secrets.** A saved SMTP password is encrypted with a key derived from `DHOW_SECRET_KEY` (AES-GCM, its own label) and never leaves the server: not in a response, a log line, an audit event or an email. The dashboard only shows whether one is saved.
- **The SMTP password and its server go together.** When `DHOW_SMTP_PASSWORD` (or its file or systemd credential) is set, the mail server, port and encryption are locked in the dashboard too, so nobody can send that password to another server from the dashboard. When the password is saved in the dashboard and you change the mail server, type the password again; otherwise the save answers `400 SETTING_INVALID`.
- **After you change `DHOW_SECRET_KEY`.** A password saved in the dashboard was sealed with the old key, so the server refuses to start and says the secrets of `mail` cannot be decrypted. Either set `DHOW_SMTP_PASSWORD` in the environment and start (Dhow then does not open the old copy, and **Server settings** offers **Remove the saved copy**; remove the variable later if you want the dashboard to manage the password again), or clear the old copy before you start, then save the password again in the dashboard:

  ```sql
  UPDATE "InstanceSettings" SET "Secrets" = NULL, "UpdatedAt" = now() WHERE "Group" = 'mail' AND NOT "IsDeleted";
  ```
- **Who may change them.** Only the instance owner, the account made by the first-owner setup, with two-step verification on and a fresh second factor for each save. API tokens never can. Every save writes the audit event `system.settings.changed` with the group and the names of the changed fields, never their values.
- **`dhow-server doctor`** prints where each setting comes from: the environment, the database or the default. It never prints a value.

## Rate limits

Public routes that write rows or start sign-in flows are limited per client network, and invitations are limited per member and per organization so the server is not used to send spam. Password sign-in and the second step also keep their own lockout per account and per address (see the [security model](../security-model.md#5-sessions-and-tokens-adr-0033)).

- **Fixed windows in the database.** Each limit counts requests in a fixed window (10 minutes for the public routes, an hour for invitations) in the `RateLimits` table, with one upsert per request and the database's clock, so several instances behind a load balancer share one counter and one clock, and a restart does not reset it. Once a client is over a limit, each instance refuses it from memory until the window ends, without asking the database.
- **Client network.** The address comes from the TCP connection, or from `X-Forwarded-For` only when the connection comes from `DHOW_TRUSTED_PROXIES`. An IPv6 client is counted per `/64`, because one host usually controls a whole `/64`. Without a correct `DHOW_TRUSTED_PROXIES` behind a proxy, every user shares the proxy's address and its limit.
- **People behind one NAT** share one limit per group. An office of 100 people who all sign in with OIDC within 10 minutes sends about 200 requests to the OIDC group, under the default of 300; their password sign-ins count in a group of their own. Raise `DHOW_RATE_LIMIT_PUBLIC` for larger sites.
- **The answer** is `429` with the code `RATE_LIMITED`, a translated message and `Retry-After` in seconds until the window ends.
- **Each invitation** can get a new link at most 4 times (5 sends in all). After that, resending answers `409 INVITATION_SEND_LIMIT`: revoke it and invite again, which counts against the hourly limits.
- Counter rows are deleted an hour after their window ends, by the cleanup that runs every 10 minutes (see [Retention of temporary rows](../security-model.md#retention-of-temporary-rows)).

## Secrets in files

`DHOW_SECRET_KEY`, `DHOW_DATABASE_URL` and `DHOW_SMTP_PASSWORD` can come from a file instead of the environment, so they never show up in `ps`, `systemctl show` or `docker inspect`:

- `DHOW_SECRET_KEY_FILE`, `DHOW_DATABASE_URL_FILE` and `DHOW_SMTP_PASSWORD_FILE` name the file.
- Under systemd, `LoadCredential=dhow-secret-key:...`, `LoadCredential=dhow-database-url:...` and `LoadCredential=dhow-smtp-password:...` work without any variable: Dhow reads `$CREDENTIALS_DIRECTORY/dhow-secret-key`, `$CREDENTIALS_DIRECTORY/dhow-database-url` and `$CREDENTIALS_DIRECTORY/dhow-smtp-password`. The [unit file](binary-systemd.md) does this for the first two.
- Each sign-in provider's client secret works the same way: `DHOW_OIDC_<ID>_CLIENT_SECRET_FILE`, or the systemd credential `dhow-oidc-<id>-client-secret` (see [Sign-in providers](#sign-in-providers)).

The rules are the same for all of them:

- Set each secret in one place only. A variable together with its `_FILE`, or with a systemd credential, stops the server.
- The file must be a regular file of at most 64 KiB that other users cannot read or write (`chmod 600`). Group access is allowed.
- One trailing newline (`\n` or `\r\n`) is removed, so `openssl rand -hex 32 > file` works. An empty file is refused.
- The value is checked exactly like the variable: a placeholder key or a URL without a secure `sslmode` still stops the server, and the error names the file, never its content.

## Email

Email is optional. Without `DHOW_SMTP_HOST`, an admin who invites someone gets the invitation link (`https://<your host>/invite#<token>`) and sends it through any channel they trust.

With SMTP set up, Dhow also emails the link to the invited address in the inviter's language. Sending is fail-safe: if the mail server is down or refuses the message, the invitation still exists, the admin still gets the link, and the screen says the email was not sent. Dhow waits at most 10 seconds for the mail server.

- The link is the only secret in the email. It works once and expires after 7 days, and resending an invitation makes a new link and voids the old one.
- `DHOW_SMTP_FROM`, the organization name and the inviter's name are the only values that reach the headers, and a value that contains a line break never does.
- Test the settings with **Send a test email** on **Server settings**, or by inviting your own address.

## Sign-in providers

Sign-in providers come from one place, either the dashboard or the environment, never both:

| `DHOW_OIDC_PROVIDERS` | Where the providers come from | **Server settings → Sign-in providers** |
|---|---|---|
| Set | The `DHOW_OIDC_*` variables below | Read-only, marked **Set by environment variables**. The API answers `409 SETTING_LOCKED_BY_ENV`. Providers saved in the database are unused and only counted. |
| Empty | The database | The instance owner adds, edits, tests and removes them |

- **All or nothing.** Dhow does not mix the two lists, because a provider that exists twice, or a list from two places, surprises the people who manage it. When you set `DHOW_OIDC_PROVIDERS`, the saved providers stay in the database unused; when you remove it and restart, they come back. Nothing is copied either way. While they are unused, the accounts linked to them cannot sign in with them, unless the variable lists a provider with the same id. Dhow shows this instead of hiding it: the **Sign-in providers** card shows a notice with the number of such accounts (`IgnoredLinkedIdentities` in the API), and `dhow-server doctor` warns with the number of accounts and providers.
- **The same rules.** The dashboard runs the checks of the variables below on every save: an `https://` issuer (`http://` only with `DHOW_DEV=true`) without credentials, a query or a fragment, valid domains, a name of at most 40 characters, a real client secret and an issuer no other provider uses. An issuer on a link-local or cloud metadata address (including `0.0.0.0/8`, `168.63.129.16` and IPv4 inside NAT64 `64:ff9b::/96`) is refused in both places. Dhow also refuses to connect to such an address when a host name resolves to one, and it checks the target before an HTTP proxy from `HTTPS_PROXY` carries the request.
- **The id never changes.** It is part of the callback URL and of linked accounts, so it is chosen once when the provider is added.
- **Type and issuer stay once accounts are linked.** Accounts are linked by issuer and subject, so a new issuer would leave them behind. The dashboard refuses with `409 OIDC_PROVIDER_LINKED`; add a new provider instead.
- **The client secret** is encrypted with a key derived from `DHOW_SECRET_KEY` and never comes back, like the SMTP password. Leave it as it is, replace it, or remove it; removing it turns the provider off. A change of type or issuer must carry the secret again (`400 SETTING_INVALID` on `ClientSecret` otherwise), so a saved secret never goes to a new token endpoint without the owner typing it.
- **Removing a provider** is a soft delete. Linked accounts keep the link on their account page and can unlink it, but nobody signs in with it. Dhow refuses to remove or turn off a provider while some account can sign in with nothing else (`409 OIDC_PROVIDER_ONLY_SIGN_IN`, with the number of accounts), and the dashboard shows that number before you try. A new list of allowed domains is refused the same way when an account that can sign in with nothing else has a linked email outside the new list.
- **Test the connection** reads the discovery document, checks that its endpoints use `https` and fetches the signing keys (for GitHub, it calls the API). It never sends the client secret, and an owner may run it five times in ten minutes.
- **No restart.** A save reaches every instance through `pg_notify`. Open sessions stay. A sign-in or link that started before its provider was removed, turned off or moved to another issuer ends with `OIDC_FLOW_INVALID`, and one that started before its secret changed is refused by the provider. Linking checks the issuer saved in the database, and the last-sign-in-method check reads the providers from the database, so an instance that has not reloaded yet cannot lock an account out.
- **Fail closed.** A saved provider that is not valid, or whose secret the current `DHOW_SECRET_KEY` cannot decrypt, stops the server at start-up. `dhow-server doctor` prints how many providers there are and whether they come from the environment or the database.
- **After you change `DHOW_SECRET_KEY`.** Saved client secrets were sealed with the old key, so the server refuses to start and names the provider. Clear the old secrets before you start, then type them again on **Server settings** (each provider is off until you do):

  ```sql
  UPDATE "OidcProviders" SET "Secret" = NULL, "UpdatedAt" = now() WHERE NOT "IsDeleted";
  ```

  A running server whose stored secret became unreadable refuses every provider change except removing that provider or clearing its secret, which is the repair.

Each id in `DHOW_OIDC_PROVIDERS` reads the variables below, with the id in upper case (`google` reads `DHOW_OIDC_GOOGLE_ISSUER`). Any problem with any provider stops the server. The full guide, with Google, Keycloak and GitHub examples, is [Sign in with OIDC](../guides/oidc.md).

| Variable | Required | Default | Purpose and validation |
|---|---|---|---|
| `DHOW_OIDC_<ID>_TYPE` | No | `oidc` | `oidc` for any OpenID Connect provider (Google, Keycloak, Entra ID, Authentik, ...). `github` for GitHub, which speaks OAuth 2.0 without OpenID Connect. |
| `DHOW_OIDC_<ID>_ISSUER` | With `oidc` | `https://github.com` for `github` | The issuer URL, exactly as the provider's `/.well-known/openid-configuration` reports it. `https://` only, or `http://` with `DHOW_DEV=true`, without credentials, a query or a fragment, and never a link-local or cloud metadata address. For GitHub Enterprise Server, the server's URL. Two providers cannot share an issuer (compared without case in the host and without a trailing `/`), because linked identities are keyed by issuer. |
| `DHOW_OIDC_<ID>_CLIENT_ID` | Yes | none | The client id registered at the provider. |
| `DHOW_OIDC_<ID>_CLIENT_SECRET` | Yes | none | The client secret. Like other secrets it may come from `DHOW_OIDC_<ID>_CLIENT_SECRET_FILE` or the systemd credential `dhow-oidc-<id>-client-secret`, see [Secrets in files](#secrets-in-files), and placeholder values are refused. It is never logged or returned. |
| `DHOW_OIDC_<ID>_NAME` | No | the id, or `GitHub` | The button label on the sign-in page, at most 40 characters. |
| `DHOW_OIDC_<ID>_ALLOWED_DOMAINS` | No | empty | Comma-separated email domains. When set, an identity is accepted only if the provider reports a verified email address in one of them. Empty accepts any identity the provider signs. |

The redirect URI to register at the provider is `<DHOW_PUBLIC_URL>/oidc/callback/<id>`.

## Development and tests

| Variable | Required | Default | Example | Purpose and validation |
|---|---|---|---|---|
| `DHOW_TEST_DATABASE_URL` | Only in CI | none | `postgres://dhow:dhow@127.0.0.1:5432/dhow_test?sslmode=disable` | Used by the Go tests, never by the server. The database tests create a throwaway schema in this database for each test. Without it they are skipped, except when `CI` is set, where they fail. |
| `DHOW_TEST_PG_DUMP` | Only in CI | `pg_dump` | `docker run --rm --network host postgres:18 pg_dump` | The `pg_dump` command, with its arguments, that the test comparing the migration splitter with goose uses. Its version must not be older than PostgreSQL's. If it is older the test is skipped, except when `CI` is set, where it fails. |

## Compose stack variables

The [Compose stack](compose.md) reads a few more variables from `.env` that never reach the server: `POSTGRES_PASSWORD`, `DHOW_PUBLISH` and `DHOW_IMAGE`, described in [its configuration table](compose.md#configuration). The [Traefik file](reverse-proxy.md#traefik-in-the-compose-stack) adds `ACME_EMAIL` and `ACME_CA_SERVER`.

## Keeping this page current

A Go test in `apps/server/internal/Config` fails when the config package declares a `DHOW_*` variable that the server, dashboard and development tables on this page do not list, when a table lists one that the code no longer reads, or when the dashboard table and the settings the dashboard can change differ.
