[العربية](../security-model.md)

# Security model

Status: **Draft v0.2**. Security is the product: every design choice below is a default, not an option.

## 1. Assets

The assets to protect are:
- Deployment targets: production servers and clusters.
- Secrets: environment secrets, target credentials and the KEK.
- Source code and artifacts.
- The audit trail.
- User accounts and sessions.

## 2. Threat model

| Actor | Capability | Primary controls |
|---|---|---|
| **Malicious or compromised developer** | Can push code and edit `.dhow.yml` in repos they can write to | Sandboxed runners (no host access); environment protection rules and approvals; secrets scoped per environment; branch restrictions per environment; audit |
| **External contributor (PR from a fork)** | Can open PRs, so attacker-controlled code reaches CI | Fork PR jobs run with **no secrets** and are restricted to non-deploy environments. Running them needs maintainer approval, and the pipeline definition always comes from the base branch for privileged events |
| **Compromised runner host** | Full control of one runner | A runner only receives secrets for jobs leased to it. A runner identity can be revoked instantly. Runners can be pinned to environments through labels and pools. Runners never get control-plane credentials or database access |
| **Network attacker** | MITM between components or against targets | TLS everywhere. Runner identity keys are pinned, and the control plane signs every lease grant with a key the runner pinned at registration, so a compromised proxy cannot inject jobs. Secrets are sealed end to end to the runner's key (HPKE). SSH host keys and TLS fingerprints are pinned per target. Webhooks are verified with HMAC and replay protection |
| **Stolen session or token** | Holds a user's cookie or API token | Server-side sessions with an idle and an absolute timeout. A session is revoked at once by sign-out, password change, MFA change, role change or an admin action. Cookies are `HttpOnly`, `Secure` and `SameSite=Strict`, and every state-changing request needs a CSRF token. High-risk actions require re-authentication. API tokens are scoped and expire |
| **Unauthenticated internet attacker** | Can reach the public endpoints | Registration is closed. Every route requires authentication by default. Rate limits apply per IP *and* per account. There is no unauthenticated realtime channel |
| **Malicious plugin author** | Publishes a step image | Plugins run inside the job sandbox with the job's privileges only. Plugins are referenced by digest. The registry marks verified publishers and requires signatures (cosign) |

## 3. Secure defaults

- **Registration.** Public registration is **closed**. The first owner account is created through a one-time setup URL. After that, users join by invitation. An external identity provider (OIDC or GitHub) only signs in an account that already exists and has linked that identity; it never creates an account (ADR-0039).
- **Fail closed on weak configuration.** The server **refuses to boot** with missing, placeholder, short or reused secrets. Real environment variables take precedence over config files.
- **TLS** is required outside localhost. HSTS is on. Cookies are `Secure`, `HttpOnly` and `SameSite=Strict`.
- **Strict CSP:** `default-src 'self'`, no inline script, `frame-ancestors 'none'`. Trusted Types (`require-trusted-types-for 'script'`) are planned with the M7 browser tests; until then a project hook forbids every HTML sink (`innerHTML`, `dangerouslySetInnerHTML`) in the web code.
- **No secret values in any API response, log line, error message or notification.**

## 4. Authentication (ADR-0013)

| Method | Edition |
|---|---|
| Password with Argon2id, breached-password check (k-anonymity, optional offline list) and max length | Community |
| TOTP. Enrolling or changing it requires the current factor. Replay protection stores the last accepted step | Community |
| **Passkeys (WebAuthn)**, the phishing-resistant method and the recommended default | Community |
| OIDC login (Google, Keycloak, any OIDC IdP) and GitHub (OAuth 2.0), linked per account | Community |
| SAML 2.0, SCIM provisioning, group→team sync | Business+ |

**Login flow.** The password step only yields a short-lived, single-use `mfa_pending` challenge that is bound to the user and the device. The second factor must present that challenge. A user id taken from the client is never trusted.

- **The challenge.** A correct password for an account with TOTP or a passkey answers `202` and sets `__Host-dhow_mfa` (`HttpOnly`, `Secure`, `SameSite=Strict`, `Path=/`, 5 minutes). It holds 32 random bytes, and the server stores only their SHA-256. It is bound to the user and to a hash of the User-Agent, not to the IP address, because the address of a real user can change within the same minute (IPv4/IPv6 switching, mobile handover, CGNAT). Only `POST /api/v1/sessions/mfa` turns it into a session, and it works once. Five wrong codes burn it.
- **Throttling.** Second-step attempts are counted per user and per client address before they are checked, like passwords ([section 5](#5-sessions-and-tokens-adr-0033)). The per-user counter is separate from the password counter, so passing the password again never buys more guesses.
- **TOTP.** The secret (160 bits) is generated on the server and stored sealed with AES-256-GCM under a key derived from `DHOW_SECRET_KEY` (HKDF-SHA256, one label per purpose) with a random nonce per record and the user id as associated data. One step of clock skew is accepted on each side, and the last accepted step is stored, so a code never works twice. The QR code is drawn in the browser. The web app starts an enrollment only when the user asks for the QR code, and a pending enrollment activates nothing until a code from the new app is confirmed.
- **Recovery codes.** Ten codes of 80 random bits each, shown once and stored as HMAC-SHA256 under a derived key. Each works once, and each use is audited.
- **Passkeys.** The RP ID and the only accepted origin come from `DHOW_PUBLIC_URL`. Challenges are stored on the server, live 5 minutes and work once. Passwordless sign-in requires user verification. A sign-count that goes backwards is refused and audited.
- **Changing a factor.** Turning TOTP on, off or replacing it, making new recovery codes, and adding or removing a passkey need a current factor in the same request: the password while the account has no second factor, and a TOTP code, recovery code or passkey afterwards. Every such change revokes the user's other sessions and is audited.
- **Credential alerts.** When email is configured (`DHOW_SMTP_*`), the user gets an email in their own language after a password change, turning TOTP on or off, adding or removing a passkey, new recovery codes, linking or unlinking an external identity, a new API token, and an admin's MFA reset. It says what changed, when (UTC), the network (`/24` for IPv4, `/48` for IPv6) and the browser family, and links to the account page with the open sessions. The MFA-reset email leaves out the network and browser, which are the admin's. It never carries a secret value, a token name or a code. The email is sent in the background after the change is committed and audited, so a slow or failing mail server neither delays nor undoes the change; a failure is only logged, and at most 32 sends wait at once. Both the audit record and the alert are detached from the request, so a client that drops the connection right after the commit cannot skip them. Without email the audit log is the only record.

**External identity providers (ADR-0039).** Providers are configured only through the environment (`DHOW_OIDC_PROVIDERS`, see [Sign in with OIDC](guides/oidc.md)), and any invalid provider stops the server. Two providers cannot share an issuer. Discovery happens on first use, so an unreachable provider disables only that provider (`503 OIDC_PROVIDER_UNAVAILABLE`); one discovery runs at a time and no caller waits past its own deadline. The authorization, token and JWKS endpoints the provider announces must be `https` (plain `http` only with `DHOW_DEV=true`), otherwise the provider counts as unavailable. Flow rows are deleted a day after they expire (see [Retention of temporary rows](#retention-of-temporary-rows)).

- **The flow.** `POST /api/v1/oidc/authorize` sets `__Host-dhow_oidc` (32 random bytes, `HttpOnly`, `Secure`, `SameSite=Strict`, 10 minutes) and returns the provider URL with a random `state`, a PKCE `S256` challenge and, for OIDC, a `nonce`. The PKCE verifier and the nonce are not stored: they are HMACs of the cookie under a key derived from `DHOW_SECRET_KEY`. The server stores only SHA-256 hashes of the cookie, the state and the User-Agent, so a database copy cannot finish a flow.
- **The callback.** The provider returns to the web page `<DHOW_PUBLIC_URL>/oidc/callback/<id>` (one URL per provider, against mix-up attacks), which strips `code` and `state` from the address and posts them once to `POST /api/v1/oidc/callback`. The flow row is consumed before the code is exchanged; a missing or forged cookie, a wrong state, another browser, another provider, or a used or expired flow burns it without contacting the provider and answers `400 OIDC_FLOW_INVALID`.
- **The ID token** is verified with `go-oidc`: signature against the provider's JWKS, issuer, audience, expiry, `azp` when present or when there are several audiences, and the nonce in constant time. Provider responses are capped at 1 MB, redirects are refused, and calls time out after 10 seconds. GitHub has no ID token: the subject is the numeric account `id` from `GET /user`, never the login, which can be renamed and reused.
- **Linking.** An identity is matched only by `(issuer, subject)`, under a unique index. The email from the provider is shown but never used to find an account, so an identity whose email equals an existing account's email cannot sign in to it (`401 OIDC_NOT_LINKED`). Linking and unlinking happen from the account page, need the current factor and a browser session, revoke the user's other sessions and are audited. The last sign-in method (password, passkey or linked identity) can never be unlinked (`409 LAST_SIGN_IN_METHOD`), checked under a row lock on the user.
- **Allowed domains.** `DHOW_OIDC_<ID>_ALLOWED_DOMAINS` accepts an identity only if it has a verified email whose domain is exactly one of the list; otherwise `403 OIDC_DOMAIN_NOT_ALLOWED`, both at sign-in and when linking.
- **Second step and session.** An account with TOTP or a passkey gets the same `mfa_pending` challenge as after a password; the provider's own MFA does not replace Dhow's. The challenge keeps the first step, so the `auth.login` event records both: `Method` is the second step and `FirstMethod` is `oidc:<id>` or `password`. A successful sign-in rotates the session exactly like a password sign-in. A Dhow session is independent of the provider session: signing out of the provider does not sign out of Dhow.

**Lockout.** Lockout counters are per account and per IP, with exponential backoff (details in [section 5](#5-sessions-and-tokens-adr-0033)). Error messages are generic, and `make timing` shows no difference between the failure cases above max(10%, 300 µs) of the median (about 3.5 ms on password sign-in with the production Argon2id parameters, ADR-0040).

**Rate limits (ADR-0040).** The public routes that start a flow or write a row (setup, password sign-in, the second step, passkey sign-in and its options, OIDC authorize and callback, invitation preview and accept) are limited per client network, an IPv4 address or an IPv6 `/64`, with fixed windows counted in PostgreSQL on the database's clock so every instance shares them. Only requests that pass the `Origin` check are counted, so a cross-site page cannot spend the limit of an office behind one NAT. A route-table test fails when a public write route is neither limited nor on a short exception list with its reason. Invitations are limited per member and per organization each hour, and one invitation is sent at most 5 times. Above a limit the answer is `429 RATE_LIMITED` with `Retry-After`. The defaults and their variables are in [Rate limits](install/configuration.md#rate-limits).

## 5. Sessions and tokens (ADR-0033)

The web UI and the API are served from the same origin, so browsers use **server-side sessions** with a cookie. There is no JWT for browsers: a JWT would still need a server-side lookup on every request to be revocable, and it would live in JavaScript.

- **Cookie.** `__Host-dhow_session` with `HttpOnly`, `Secure`, `SameSite=Strict` and `Path=/`, and no `Domain`. The `__Host-` prefix makes browsers refuse a cookie set by a sub-domain or over plain HTTP. `Secure` is always set, because the public URL is HTTPS by design.
- **Session id.** 32 random bytes. Only its SHA-256 hash is stored in `"Sessions"`, so a database leak does not yield usable cookies.
- **Lifetime.** A session ends after **12 hours without activity** (idle timeout), **4 hours** for anyone who is an owner or admin in any organization, or **7 days** after sign-in (absolute timeout), whichever comes first. Both limits are checked on every request against the current role, so a promotion shortens a running session at once. `LastSeenAt` is written at most once a minute, so reads do not turn into writes.
- **Rotation.** Signing in always issues a new session id and revokes the session the request carried. When a member's effective role in any organization changes (a direct role change, a team binding, joining or leaving a team, removal), every session of that member is revoked, so the next request needs a fresh sign-in and a new id. The person making the change keeps only the session they are using. Accepting an invitation while signed in keeps the current session: session ids are only ever minted at sign-in, so there is nothing to fixate (ADR-0038).
- **Revocation is immediate.** Every request checks the session row, with no cache. Sign-out, password change, MFA change, role change, an admin "sign out everywhere" and "sign out of every other device" all revoke sessions server-side. A disabled or deleted user loses every session at once.
- **CSRF.**
  - Every request that is not `GET`, `HEAD` or `OPTIONS` needs an `X-CSRF-Token` header and an `Origin` header equal to `DHOW_PUBLIC_URL`. Otherwise the answer is `403 CSRF_FAILED`.
  - The token is an HMAC of the session id under a key derived from `DHOW_SECRET_KEY`. It is bound to one session, `GET /api/v1/me` returns it, and the server stores only its hash.
  - Sign-in and setup have no session yet, so they skip the token but still check `Origin`.
  - Responses of the API are never cacheable (`Cache-Control: no-store`).
- **Passwords.** Argon2id (m=19 MiB, t=2, p=1, 16-byte salt, 32-byte key), stored as a PHC string and compared in constant time. A hash made with older parameters is replaced after the next successful sign-in. The length is 12 to 128 characters.
  - **Breached passwords.** When a password is set or changed, the server asks the Have I Been Pwned range API. Only the first 5 hex characters of the password's SHA-1 leave the server (k-anonymity), padding is requested, and the call times out after 3 seconds. If the service cannot be reached, the change goes through and a warning is logged. `DHOW_BREACHED_PASSWORD_CHECK=false` turns the check off for isolated networks.
- **Sign-in throttling.** Failures are counted per account and per client address, where an IPv6 client counts per `/64`. After 5 failures for an account, or 20 for an address, the key is locked for 30 seconds, and every further failure doubles the lock up to 15 minutes. A key forgets its failures after an hour without one, and a successful sign-in resets the account. Attempts are counted before the password is checked, in one transaction that locks both counters, so a burst of parallel guesses gets no more attempts than a slow one. Attempts made during a lock are refused without being counted, so nobody can extend a lock. A locked key answers with the same generic `INVALID_CREDENTIALS` error plus `Retry-After`. Unknown accounts are throttled exactly like real ones, and the unknown-account path still runs a full Argon2id verification, so neither the answer nor its timing shows whether an account exists.
- **CLI, CI and integrations** use **personal API tokens**, never a session (ADR-0039).
  - **Format and storage.** `dhow_pat_` followed by 32 random bytes in base64url, so secret scanners can detect them. Only an HMAC-SHA256 under a key derived from `DHOW_SECRET_KEY` is stored, and the value is shown once, in the creation response.
  - **Scope.** A token belongs to one organization and to the creator's membership row, and carries explicit scopes that are policy actions (`org.read`, `org.update`, `member.read`, `member.manage`, `audit.read`, `runner.read`, `runner.manage`, `job.read`, `job.cancel`, `project.read`, `project.manage`). `job.create` is not a scope: a job runs any image on any runner of the organization, so creating jobs needs a browser session. Every scope must be allowed by the creator's role at creation (`403 SCOPE_NOT_ALLOWED`). Expiry is mandatory, 1 to 366 days, and a user can hold at most 50 active tokens.
  - **Never above the creator.** On every request a token is allowed an action only if the action is in its scopes **and** the creator's current effective role allows it. A demotion narrows the token at once, a promotion never widens it, and leaving or being removed from the organization, a disabled account or a deleted organization stops it for good.
  - **Creating** a token needs a browser session and the current factor. The owner lists and revokes their tokens from `/api/v1/me/api-tokens`; Owners and Admins (`token.manage`) can list and revoke every token of the organization. Revoking needs no factor. In the web app they do it from **Organization → API tokens** (the organization footer, the Settings tab or the command palette), which shows each token's creator, scopes, creation, last use and expiry but never a value.
  - **Which routes.** Only `Authorization: Bearer dhow_pat_…` counts as a token; other schemes are ignored. A route accepts tokens only if its policy row says so explicitly (`AllowTokens`), and `openapi.yaml` declares the `ApiToken` scheme on exactly those routes, which a test enforces. A token on any other route, public or not, answers `403 SESSION_REQUIRED`: password, MFA, passkeys, linked identities, preferences, sessions, token management, leaving an organization (including removing your own membership), transferring ownership and creating jobs are browser-only. A token can never confirm a current factor.
  - **No mixing.** A Bearer token together with a Dhow cookie answers `401`. Bearer requests skip the `Origin` and CSRF checks, because browsers never attach an `Authorization` header on their own and the server sends no CORS headers. Runner tokens (`dhow_rat_`) are refused on the REST API, and personal tokens are refused on `/runner-api/`.
  - `LastUsedAt` and `LastUsedIp` are written at most once a minute.
  - **Credential events.** An admin's MFA reset revokes every token of the member together with their sessions, because it is the path for recovering a compromised account whose owner may not hold it. Changing the password or a factor and "sign out of every other device" do not revoke tokens: a token is a separate credential for automation, bound to the membership and role and limited by its scopes and expiry, and revoking it on a routine password change would silently stop CI. Someone who suspects a compromise revokes their tokens from the account page, and disabling the account stops all of them.

### Retention of temporary rows

Rows that only exist to run a sign-in, a throttle or a rate limit are deleted for real once they can no longer matter, so the tables do not grow with every attempt (ADR-0040). Every 10 minutes one instance, chosen by a PostgreSQL advisory lock, deletes them in batches of 1000 with `FOR UPDATE SKIP LOCKED`, so a row a request is using is never touched and no table stays locked for long:

| Table | Deleted |
|---|---|
| `LoginThrottles`, `RunnerThrottles` | a day after the last failure, once no lock is active |
| `AuthChallenges`, `OidcFlows`, `SetupTokens` | a day after they expire |
| `Invitations` | 30 days after they expire or are revoked; accepted invitations stay |
| `RateLimits` | an hour after their window ends |

These are the only deletes in the code base, all in one query file that a project hook checks: every other table, including the audit log, memberships, sessions and tokens, is only ever marked deleted.

## 6. Authorization (ADR-0015)

- A **single policy function**, `can(subject, action, resource)`, is the only path to authorization.
  - Handlers declare the action and resource, and middleware enforces them.
  - Resource loaders are tenant-scoped, so a missing check results in "not found", not a data leak.
- **Fixed roles** apply per scope (Organization → Project → Environment):

| Role | Summary |
|---|---|
| Owner | Billing/license, org deletion, everything |
| Admin | Manage org, members, runners, all projects |
| Maintainer | Manage project settings, environments, protection rules, secrets |
| Developer | Trigger deployments to environments their rules allow; read logs |
| Viewer | Read-only on assigned projects |

- **Teams** group users, and roles can be granted to teams through role bindings. A binding has a scope (organization, project or environment); M2.3 implements the organization scope. A member's **effective role** is the highest of their direct role and the roles of every team they belong to, and it is computed in one place (`Core/Policy`). A binding can never grant Owner.
- **Granting rules.** Nobody can grant a role above their own, members with a higher role can only be changed by someone at least as high, and only an Owner can make or unmake an Owner. Granting or removing Owner, transferring ownership and resetting another member's MFA all need the actor's current factor (password, TOTP or passkey). An Owner is never removed from an organization directly: their role is changed first, which needs the factor, so neither a stolen session nor an Owner's API token can remove a co-owner. Nobody changes their own role; they leave or transfer instead. The last Owner of an organization can never be removed, demoted or leave, and membership changes take a row lock on the organization so two parallel requests cannot both pass that check. Disabled or deleted accounts do not count as owners, so ownership never rests on them alone. Team writes and invitation acceptance re-check the actor's or inviter's role under the same lock, so a demotion that commits meanwhile is seen.
- **Invitations.** An invitation names an email and a role no higher than the inviter's, and never Owner. The token is 32 random bytes, only its SHA-256 hash is stored, it works once and expires after 7 days, and a resend replaces it. The link carries the token in the URL fragment (`/invite#…`), which browsers never send to the server, and the page removes it from the address at once; the API takes it only in a request body. Accepting checks again that the inviter is still an active member with a role that may grant it; a resend makes its actor the inviter. A used, revoked, expired or unknown token answers with the same `410 INVITATION_INVALID`. A signed-in account joins only when its email matches the invitation, and nobody joins without a token. Email is optional (`DHOW_SMTP_*`): when sending fails, the invitation still exists, shows that the email failed, and the inviter can copy the link.
- **Organizations.** By default only the instance owner (the account created at setup) can create more organizations; `DHOW_ORG_CREATION=any-user` opens it to every signed-in user. A request about an organization the caller is not a member of answers `404`.
- **Project roles.** A role binding on a project (for a member or a team) changes the member's role inside that project only, up or down: a Developer bound as Maintainer changes that one project, and a Maintainer bound as Viewer only reads it. Owners and Admins are never lowered, and a binding never grants Owner. Creating a project needs the role in the organization itself; a binding on another project is not enough. An API token follows the same binding, within its scopes. A project of another organization answers `404` (ADR-0070).
- **Admin MFA reset.** An Owner or Admin can reset another member's two-step sign-in after confirming their own current factor, only if their role is strictly higher than the member's effective role in every organization the member belongs to: an Owner resets an Admin and below, an Admin resets a Maintainer and below, and nobody resets someone of the same rank. The instance owner counts as an Owner everywhere. The member is signed out everywhere and asked to enroll again, and the audit event is marked high severity.
- **Required second factor.** An Owner can require a second factor (TOTP or a passkey) in an organization: `off` (the default), `admins` (Owners and Admins) or `everyone`. Changing the rule needs a browser session, the Owner's current factor, and a second factor on the Owner's own account. A member the rule covers and who has no second factor gets 7 days from the later of the rule change and their joining, shown in the organization banner and in `GET /me` (`SecondFactorDue`). After that, the sensitive actions in that organization (`org.update`, ownership transfer, the security setting, `member.manage`, `runner.manage`, `job.create`, `project.manage` and creating API tokens) answer `403 SECOND_FACTOR_REQUIRED`, for the session and for the member's API tokens alike, while reading stays allowed. Widening the rule gives a new grace period only to the roles it newly covers, so Owners and Admins who are already overdue stay blocked; narrowing it keeps the running one, and lowering it is audited with high severity. Each organization's rule applies only inside that organization. A member made Admin on a project by a role binding is covered by the `admins` rule on that project. The check runs in the central `can()` policy from the current state on every request.
- **Custom roles and OPA policies** come from `ee/`.
- **Testing:** a test walks the **entire route table** for every role and scope and asserts the expected allow or deny result. A route without a declared policy fails the build.

## 7. Execution safety

- **Isolation (ADR-0007, ADR-0043).** Jobs only run inside a rootless container or microVM, with limits, a read-only root filesystem, network egress denied by default, and no host mounts. The control plane never executes user code.
  - The default sandbox is rootless Podman with all capabilities dropped, `no-new-privileges`, a seccomp profile that also blocks nested user namespaces, `io_uring`, `bpf`, `ptrace` and `mount`, and a separate user-namespace range per job.
  - A runner refuses to start if cgroup v2 delegation for CPU, memory and PIDs is missing, and it fails the job if the created container does not match the policy.
  - An escape test suite (host files, runner credentials, other jobs, metadata endpoints, container sockets, privilege escalation, network) is a required CI check.
- **Network egress (ADR-0044).** A sandbox has no network interface besides loopback. Allowed hosts are reached only through a per-job proxy in the runner. The proxy resolves names itself, connects to the address it resolved, and always denies private, loopback, link-local, CGNAT and cloud metadata ranges. Only the runner operator can open a private range, and cloud metadata addresses stay denied even then. The proxy speaks `CONNECT` and plain `http://` only, never decrypts TLS, caps each job at 256 connections, and closes a connection after 5 minutes without traffic (ADR-0057).
- **Runner plane (ADR-0041, ADR-0042).**
  - Runners authenticate with pinned Ed25519 keys and 15-minute access tokens.
  - Starting a job needs a fresh signature.
  - Every mutating call carries the attempt's fencing token.
  - Session cookies are rejected on the runner plane, and runner tokens are rejected on the public API.
- **No shell strings in platform code.**
  - Every process the platform starts (git, ssh, rsync, docker, kubectl) uses argv arrays.
  - User-controlled values are validated against strict grammars: refs go through `git check-ref-format`, SHAs must match `^[0-9a-f]{40}$`, and URLs go through a parser with a scheme allowlist.
  - Those values are passed after `--`.
- **Expressions use CEL.** CEL is non-Turing-complete, evaluates against typed variables, has cost limits, and never performs string-concatenation evaluation.
- **No regex "WAF" as a security control.** Correctness comes from parameterized queries, typed validation and output encoding.

## 8. Data protection

- Secrets use envelope encryption (see [architecture §6](architecture.md#6-secrets)): AES-256-GCM with a random 12-byte nonce, AAD bound to the record, and versioned keys with rotation.
- Webhook secrets, target credentials and API-token peppers are all treated as secrets.
- Masking happens in the runner, on the stream, before any output leaves the host.
- Retention policies apply to logs, artifacts and audit events. Export and deletion tooling supports GDPR-style requests.

## 9. Audit

- The audit log is append-only, and each event includes `PrevHash`, which makes the log **tamper-evident**.
  - Every event stores `Hash = SHA-256(PrevHash ‖ canonical JSON of the event)`, and events form one chain per organization (plus one for instance-level events). Writers take a per-chain advisory lock, so the chain never forks.
  - A database trigger rejects `UPDATE`, `DELETE` and `TRUNCATE` on the table. `GET /api/v1/organizations/{id}/audit-events/verify` walks a chain and reports the first event whose hash or link is wrong.
  - Event data never carries a secret: passwords, tokens and cookies are not recorded. Only fields on an explicit allowlist are written, and anything else is dropped with a warning; a value that contains a Dhow credential prefix (`dhow_pat_`, `dhow_rat_`, `dhow_rrt_`) is dropped too. A test scans the modules for every field they write and fails if one is missing from the allowlist, and every end-to-end test fails if a real event loses a field.
  - Reading needs `audit.read` (Owners and Admins). `GET /api/v1/organizations/{id}/audit-events` filters by `Category` (`auth`, `identity`, `runner`, `job`, `system`), exact `Action`, `ActorUserId` and a `From`/`To` time window, all combined with AND; an unknown category or a window that ends before it starts is `422 VALIDATION_FAILED`. `GET /api/v1/organizations/{id}/audit-events/{eventId}` reads one event of that organization only, so the web app can link to an event's details.
  - An action made with an API token records `Via=api_token` and `ViaId` (the token id) next to the actor. Creating and revoking a token, linking and unlinking an identity, and failed provider sign-ins after a valid flow (with the reason and a digest of the issuer and subject) are audited.
- Every security-relevant action is recorded, including auth events, role changes, secret writes, approvals, deployments and runner registration.
- **Retention:** Community keeps 90 days. Business and Enterprise get longer retention, export, and SIEM streaming.

## 10. Supply chain

- Releases are reproducible builds, **signed with cosign** (keyless), and published with an **SBOM** (SPDX) and SLSA provenance.
- Container images are distroless and non-root, and are referenced by digest in the Helm and Compose manifests.
- **CI:**
  - It is **required** for merge: tests, lint with zero warnings allowed, type checks, `govulncheck`/`npm audit` blocking on high and critical, CodeQL, secret scanning and license checks.
  - Dependabot updates are merged only on green CI.
- An external penetration test is required before GA, and again yearly.
- The disclosure policy is in [`SECURITY.md`](../../SECURITY.en.md).

## 11. Lessons from the Deploy Center v3 audit

Each class of vulnerability found in Deploy Center v3, and the design rule in Dhow that removes it:

| v3 issue class | Dhow design rule |
|---|---|
| Public registration accepted a client-supplied role (anyone could become admin) | Registration is closed by default. Request bodies are decoded into typed DTOs with **no role field**. Roles change only through an authorized, audited endpoint |
| The realtime socket had no authentication and broadcast full records containing secrets | Realtime runs over **SSE behind the same session auth**. Every stream is authorized per resource. Payloads are explicit DTOs and never ORM objects. There are no global broadcasts |
| The 2FA step trusted a client-supplied user id, so the password step could be skipped | Server-held `mfa_pending` challenge, bound to the password step; lockout per account |
| Pipelines executed directly on the server host | The control plane never executes user code. Jobs run only in sandboxed runners |
| Commands were built by string interpolation and run through a shell | argv-only process execution, with strict validation of refs, SHAs, URLs and options |
| Conditions were evaluated with dynamic code construction | CEL, with typed inputs and cost limits |
| The API returned webhook secrets and credentials in plaintext | Secrets are write-only. Response DTOs contain no secret fields. Everything is encrypted at rest |
| Routes lacked object-level checks (IDOR) | A central `can()` policy, tenant-scoped loaders, non-enumerable UUIDv7 ids, and route-table authorization tests |
| Hardcoded fallback secrets were accepted in production | Boot fails on missing or placeholder secrets. The installer generates strong ones |
| Stateless long-lived JWTs could not be revoked | Server-side sessions stored as hashes, checked on every request, revoked at once, with idle and absolute timeouts and CSRF tokens |
| In-place `rsync --delete` deploys with fragile backups | Atomic releases (symlink switch), health checks, automatic rollback to the previous release |
| Queue retries re-ran whole, non-idempotent deployments | No automatic retry after the first mutating step. Leases and reconciliation handle crashed jobs, and an environment lock prevents concurrent deployments |
| Notification URLs allowed SSRF and blocked the single worker | A separate notification queue with timeouts, plus an SSRF guard that checks the resolved IP |
| An idempotency cache was not bound to the user and replayed cookies | Idempotency keys are scoped to the authenticated principal. Auth endpoints and `Set-Cookie` are never cached |
| Coverage gates excluded files that were never loaded, and integration tests bypassed the real middleware stack | Coverage is measured over all packages. E2E tests run against the real server binary, and CI fails if a required service is missing |

## 12. Threat review of the runner plane and pipelines (M3.13, M4.11)

The review of 2026-11-03 covers what M3 and M4 added. Each row is one STRIDE threat (**S**poofing, **T**ampering, **R**epudiation, **I**nformation disclosure, **D**enial of service, **E**levation of privilege), the control that stops it, and the test that proves the control. GitHub and GitLab (M4.5, M4.6) are not built yet, so they are not in the review.

### 12.1 Threats, controls and proofs

| Area | STRIDE | Threat | Control | Proof |
|---|---|---|---|---|
| Runner plane | S | A stolen or copied runner identity takes jobs | Pinned Ed25519 key, 15-minute access tokens, and a runner that opens more than 3 new sessions in 10 minutes is disabled | `apps/server/internal/Modules/Runners/Test/E2E/Clone_test.go`, `Auth_test.go` |
| Runner plane | T | A proxy or a fake server injects a job | Every lease grant is signed with the key the runner pinned at registration | `apps/runner/internal/Executor/Grant_test.go`, `FuzzVerifyGrant` |
| Runner plane | T | A runner that lost its lease keeps writing results | Fencing tokens only grow and every mutating call carries the current one | `apps/server/internal/Modules/Jobs/Test/E2E/Fencing_test.go`, `apps/runner/internal/Executor/Fence_test.go` |
| Runner plane | D | Killing the runner, the server or Postgres mid-job loses or doubles work | Leases, the reconciler and the runner journal; a job retries at most once, never after a mutating step began | `apps/runner/Test/Chaos/Scenarios_test.go` (`make runner-chaos`) |
| Runner plane | E | A job escapes to the host, the runner or another job | Rootless Podman, a user-namespace range per attempt, seccomp, no capabilities, read-only root, cgroup v2 limits | `apps/runner/Test/Isolation/Escape_test.go`, `Limits_test.go`, `Leftovers_test.go` (`make runner-isolation`) |
| Runner plane | I | A secret reaches Podman, its storage or another step | Secrets go to the agent over its control socket, as a file only during their step, and the files are swept before the exit is reported | `apps/runner/Test/Isolation/Escape_test.go` (`TestSecretsNeverReachPodman`), `apps/runner/internal/Agent/Secrets_test.go` |
| Runner plane | T | A malformed frame from the sandbox crashes or confuses the runner | Length-prefixed protobuf frames with a size cap, decoded by one function | `apps/runner/internal/Sandbox/Wire/Wire_test.go` (`FuzzDecode`) |
| Egress | E | A job reaches the metadata service, the LAN or loopback | No network interface but loopback; the runner's proxy resolves, checks every address and refuses private, link-local and metadata ranges | `apps/runner/Test/Isolation/Network_test.go`, `apps/runner/internal/Egress/Egress_test.go`, `FuzzProxyRequest`, `FuzzAllowlist` |
| Pipelines | T | A YAML or CEL bomb, or a huge matrix, stalls the server | Limits on size, depth, jobs and matrix copies; CEL cost and time limits and no `matches()` | `internal/pipelinespec/fuzz_test.go` (`FuzzParse`, `FuzzCEL`, `FuzzMatrix`) |
| Pipelines | E | A job spec from the server asks for more than the runner allows | The runner checks the spec again with `jobspec.Prepare` before anything starts | `internal/jobspec/jobspec_test.go`, `FuzzPrepare` |
| Pipelines | T | A runner hands over a forged `.dhow.yml` for a generic repository | The server hashes the raw Git objects and follows them from the run's commit id | `apps/server/internal/Modules/Pipelines/Test/E2E/Proof_test.go`, `FuzzDefinitionProof` |
| Pipelines | I | An output or a computed value shows a secret in the API | Derived values are `[REDACTED]` for every role, and the masker drops outputs that join into a secret | `apps/server/internal/Modules/Pipelines/Test/E2E/Redact_test.go`, `apps/runner/internal/Masking/Masker_test.go` |
| Pipelines | D | One organization's broken runs starve the others | Per-organization queue caps and a poison run that never blocks other organizations | `apps/server/internal/Modules/Pipelines/Test/Edge/Poison_test.go`, `Starvation_test.go` |
| Providers (generic Git) | S | A link points at a private or metadata address (SSRF) | `netguard` dials only public addresses unless `DHOW_GIT_ALLOW_PRIVATE_CIDRS` allows a range | `apps/server/internal/Modules/Sources/Test/E2E/Repository_test.go` (`TestLinkRefusesPrivateAndMetadataAddresses`) |
| Providers (generic Git) | S | A changed host key lets a MITM serve the code | The host key is pinned only after a person confirms its fingerprint, and never automatically | `Repository_test.go` (`TestLinkAnSSHRepositoryPinsTheHostKeyOnlyAfterConfirmation`), `apps/runner/internal/Agent/Checkout/Checkout_test.go` |
| Providers (generic Git) | I | The API shows a deploy key or a webhook secret | Both are sealed with AES-256-GCM, the secret is shown once, and the private key never | `Repository_test.go` (`TestLinkAnHTTPSRepositoryShowsTheWebhookSecretOnce`), `apps/server/internal/Modules/Sources/Test/Unit/Credential_test.go` |
| Webhooks | S | A forged delivery starts a run | HMAC on the body in four formats, checked in constant time, the same `401` for every failure | `apps/server/internal/Modules/Sources/Test/Unit/Webhook_test.go`, `apps/server/internal/Modules/Sources/Test/E2E/Webhook_test.go`, `FuzzWebhook` |
| Webhooks | R | A captured delivery is sent again later | Dedupe on the delivery id for 72 hours and on the body digest for as long as deliveries are kept (30 days); one run per commit | `Webhook_test.go` (`TestASignedPushTriggersOnceAndADuplicateDoesNothing`), `apps/server/internal/Modules/Pipelines/Test/E2E/Triggers_test.go` |
| Webhooks | D | A flood of large or bad deliveries, or of signed pushes of new commits | Body cap, a shared byte budget, rate limits per address, per hook and per failing hook, and 600 new runs per project an hour | `Webhook_test.go` (`TestWebhookRequestLimits`, `TestFailedSignaturesAreLimitedPerHookWithoutStarvingRealDeliveries`), `Triggers_test.go` (`TestWebhookRunsAreLimitedPerProject`) |
| Checkout | I | A step or a plugin reads the checkout credential | The credential lives only in the checkout sandbox, and only for the current job of a live run on the same repository | `apps/runner/Test/Isolation/Workspace_test.go` (`TestTheCheckoutCredentialNeverReachesAUserStep`), `apps/server/internal/Modules/Pipelines/Test/E2E/Credential_test.go` |
| Checkout | T | A force push or a ref trick checks out another commit | The commit must be in the history of the ref (`CHECKOUT_REF_MOVED` otherwise), refs and URLs are validated and passed after `--` | `Checkout_test.go`, `internal/gitref/gitref_test.go` (`FuzzRef`), `internal/gitsource/gitsource_test.go` (`FuzzCloneUrl`) |
| Plugins | E | A plugin gets secrets or settings of other steps | A plugin gets only its inputs, `DHOW_OUTPUT`, its egress and the secrets of its own step; the manifest is checked before it starts | `apps/runner/internal/App/Checkout_test.go`, `internal/pipelinespec/build_test.go`, `apps/runner/internal/Executor/Plugin` (`FuzzManifest`, `FuzzOutputs`) |
| Plugins | T | A plugin image changes after review | `uses:` must be pinned by digest, or the file is refused (`USES_NOT_PINNED`), and the runner refuses a tag again | `internal/pipelinespec/pipelinespectest/cases.go`, `internal/jobspec/jobspec_test.go` |
| Artifacts | T | A tampered artifact reaches the next job | SHA-256 checked on the server and again on the runner before unpacking | `apps/server/internal/Modules/Artifacts/Test/E2E/Artifacts_test.go` |
| Artifacts | E | An archive writes outside the workspace | The unpacker refuses `..`, absolute paths, escaping links, devices and FIFOs, and runs in the job's user namespace | `apps/runner/internal/Sandbox/Archive/Archive_test.go` (`FuzzUnpack`), `apps/runner/Test/Isolation/Transfer_test.go` |
| Artifacts | I | A runner reads a blob of another organization | A download needs a grant for that exact digest, a live lease and the current fencing token | `Artifacts_test.go`, `apps/server/internal/Modules/Artifacts/Test/E2E/Review_test.go` |
| Cache | T | A branch or a pull request poisons the cache of `main` | Scopes per branch, tag and pull request; a pull request never writes `main`, a fork never writes | `apps/server/internal/Modules/Artifacts/Test/E2E/Cache_test.go` |
| Cache | D | Cache entries fill the disk | A cap per project with least-recently-used removal that never removes the default branch for another scope | `Cache_test.go`, `internal/cachekey` (`FuzzRender`), `internal/pathglob` (`FuzzMatchPattern`) |
| Forks | E | A fork pull request changes the pipeline, the images or the egress | The file comes from the base commit, the run waits for approval, and it gets no secrets or environments | `apps/server/internal/Modules/Pipelines/Test/E2E/Control_test.go`, `Checkout_test.go` (`TestAForkRunReadsTheFileOfTheBase`) |
| Forks | T | A fork's artifact feeds a deploy | Artifacts of a fork run are `Untrusted` and never feed a deploy | `Artifacts_test.go` (`TestArtifactsOfAForkRunAreUntrustedAndNeverFeedADeploy`) |
| Forks | D | Fork runs, under the `automatic` policy, fill the artifact quota of the organization | The artifacts of fork runs of one project get at most a tenth of the quota | `apps/server/internal/Modules/Artifacts/Test/E2E/Review_test.go` (`TestForkRunsShareATenthOfTheQuotaOfTheirProject`) |
| Dev stack runner | E | The privileged development container is used in production | ADR-0079: the Podman runner runs `privileged` for development only, and the docs say production never runs that way | Accepted risk (12.2) |
| Update plan | T | A malicious update reaches every server | ADR-0077: a TUF feed with keys pinned in the binary, rollback and freeze protection, and keys kept offline | Not built yet (12.2) |
| All of the above | I | A credential is written in clear to the database, a log or a container store | Write-only secrets, sealed at rest, masked on the stream | `deploy/m4-demo/leak-sweep.sh` (12.3), `apps/server/internal/Modules/Pipelines/Test/E2E/Checkout_test.go` (`TestTheKeyCheckFindsAKeyPlantedInALogChunk`) |

### 12.2 Accepted risks and gaps

- **The update targets key alone can ship an update (ADR-0077).** Sabry accepted it: one key that never leaves two encrypted USB drives and is unlocked only on an offline system. The updater is not built yet, so nothing tests it.
- **The dev stack runner (ADR-0079)** runs in a `privileged` container without the unit's hardening. One local privilege escalation separates a job that escapes to `dhow-runner` from root on the Docker machine. It is for development only.
- **No secret store until M5.** A spec that references secrets fails with `SECRETS_UNAVAILABLE`, so the secret paths of plugins and steps are proven with the runner's tests and fakes, not end to end.
- **GitHub and GitLab (M4.5, M4.6) are not built.** The `providers` CI job is a skeleton, skipped until its environment is set up.
- **The sandbox:** no I/O limit, 32-bit `socketcall` still allowed, cgroup v1 and SELinux enforcing not supported (ADR-0067).
- **Masking:** a secret shorter than 4 characters cannot be masked, and the job log says so.
- **Read access of a runner:** a runner that runs a checkout job gets the repository's read-only deploy key for that job. A compromised runner host can keep it until the key is rotated.
- **Cache trust:** whoever can push to a branch can fill that branch's cache; this is the same trust as the branch's code.
- **Audit:** `runner.revoked` is published after the commit, so a crash between them loses the audit event (no outbox yet).
- **External review:** moved to M9 (ADR-0040). The checklist below is what it starts from.

### 12.3 Credential leak sweep

`deploy/m4-demo/leak-sweep.sh` runs at the end of `deploy/m4-demo/run.sh`, after a full flow (a linked repository, a webhook, a checkout, an artifact through `needs` and a cache hit). The shared code is `internal/leaksweep`, and the demo test has the `leaksweep` build tag.

- **Where it looks:** `pg_dump`, the server, runner and Postgres logs, the runner's state directory with its Podman storage, the server's blob store with every `.tar.gz` artifact and cache entry unpacked in memory (at most 100,000 entries and 4 GiB per archive), and the systemd journal when there is one.
- **What it looks for:** the deploy keys and webhook secrets, unsealed with the server key; the server key; the artifact grant key; the API token; the runner registration token; the session cookie; the CSRF token; the owner password; and the runner's signing and encryption keys, which may appear only in its identity file. Each value is searched raw, in hex, in JSON escaping and in standard and URL base64 at every alignment, and a deploy key also through its private seed. Anything shaped like an API token, a runner access or registration token, or a download grant is a finding too, so tokens the demo never sees are covered. The input grants of a job are kept in its spec in the `"Jobs"` table on purpose (valid 72 hours, and useless without that job's live lease and fencing token), so `pg_dump` is split per table and grants are allowed in that table only.
- **Fails closed:** a missing, empty or unreadable folder, an archive past its limits, an empty value, or a sealed column the sweep cannot open stops it with a failure, not a clean result.
- **Proves it can see:** before the real sweep, it must catch a token planted in clear, in base64 inside a JSON log line, inside a `.tar.gz` blob, and a made-up runner access token.
- **Not covered:** the setup token, which the server prints on purpose so the first owner can be created; Forgejo and its password; Docker's own image store; and process memory.

It prints only the name of a credential and the file it was found in, never the value.

### 12.4 Checklist for the external review

- Run `make runner-isolation` and `make runner-chaos` on a cgroup v2 host, and read the escape probes in `apps/runner/Test/Isolation/Probe`.
- Try to escape the egress proxy: IPv6 forms, DNS rebinding, `CONNECT` to an address, and redirects.
- Forge, replay and resize webhooks against the four signature formats.
- Try to get the checkout credential from a step, a plugin, an artifact or the cache.
- Hand the server forged Git objects for `.dhow.yml`, and tar files that leave the workspace.
- Check that fork pull requests never get secrets, environments or a write to the cache.
- Run `deploy/m4-demo/run.sh` and read its leak sweep.
- Read the accepted risks above and say which ones should not be accepted.
