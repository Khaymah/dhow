[العربية](../../security/auth-review-checklist.md)

# Authentication review checklist (M2)

Status: **internal review done, external review pending (M9)** · Date: 2026-10-28 · Scope: milestone M2 (identity and tenancy)

This page is the starting point for the external review of the authentication and authorization paths, which moved to M9 together with the penetration test. It lists every route in scope, the threats we considered, the control for each threat, where the control lives, and the test that proves it. It ends with the findings of the internal review and the items we know are open. Design decisions behind the controls are in ADR-0033, ADR-0035, ADR-0037, ADR-0038, ADR-0039 and ADR-0040 (in `Plan/ADR/`, in Arabic); the public description is the [security model](../security-model.md).

Paths below are relative to `apps/server/internal/` unless they start with another top-level folder.

## 1. Method

- A fresh read of every M2 path (setup, password, second factor, sessions, organizations, invitations, OIDC, API tokens, audit) against the [security model](../security-model.md) and OWASP ASVS 4.0.3 level 2, after the M2.5 hardening was in place.
- An independent review of the branch followed and found no blocker; its findings are F16 to F24 below, all addressed in M2.5.
- Every Critical or High finding is fixed in M2.5. Every Medium finding is fixed or recorded with its reason. M2 is done when every authorization test is green and no Critical or High finding is open.
- Automated evidence: `make check` (lint, `go test -race` against PostgreSQL 18, govulncheck, hook tests), `make fuzz FUZZTIME=60s` (13 targets), `make timing` (statistical timing tests), and `deploy/smoke.sh` against the container image.

## 2. Routes in scope

All routes are under `/api/v1`. Per-network limits count only requests that passed the Origin check. "Origin" means the request must carry `Origin` equal to `DHOW_PUBLIC_URL`; "CSRF" means the `X-CSRF-Token` of the session is required as well. Every route has a row in a module's `Policies()`, and `App/RouteTable_test.go` walks the whole route table for every role.

| Route | Who | Request checks | Limits |
|---|---|---|---|
| `POST /setup` | anyone, before the first owner exists | Origin, one-time setup token in the body | per client network (`DHOW_RATE_LIMIT_PUBLIC`) |
| `POST /sessions` | anyone | Origin | lockout per account and per client network, and per client network (`DHOW_RATE_LIMIT_PUBLIC`) |
| `POST /sessions/mfa` | holder of `__Host-dhow_mfa` | Origin, challenge bound to the user and User-Agent | lockout per user and per client network, 5 codes per challenge, and per client network |
| `POST /sessions/mfa/passkey-options` | holder of `__Host-dhow_mfa` | Origin | bound to the pending challenge (the one listed exception to per-network limits) |
| `POST /sessions/passkey-options` | anyone | Origin | per client network, shared with `POST /sessions/passkey` |
| `POST /sessions/passkey` | anyone | Origin, stored single-use challenge | lockout per client network, and per client network |
| `GET /oidc/providers` | anyone | none (read only) | none |
| `POST /oidc/authorize`, `POST /oidc/callback` | anyone | Origin, `__Host-dhow_oidc`, state, PKCE, nonce | per client network, shared |
| `POST /invitations/preview`, `POST /invitations/accept` | anyone with a link | Origin, token in the body | per client network, shared |
| `GET /me`, `/sessions`, `/me/*` | signed-in user | session; CSRF for writes; current factor for factor changes, identity links and token creation | password change shares the account lockout |
| `POST /organizations` | instance owner, or any user with `DHOW_ORG_CREATION=any-user` | session and CSRF | none |
| `/organizations/{id}/**` | members, by role | session and CSRF, or a personal API token on routes that allow it | invitations per member and per organization each hour |
| `PUT /organizations/{id}/second-factor-rule` | Owner with a second factor | session, CSRF and the current factor; never a token | none |
| `/organizations/{id}/audit-events`, `.../verify` | `audit.read` | session or token | none |

## 3. Threats, controls and proof

### Setup

| Threat | Control | Code | Proof |
|---|---|---|---|
| Someone else claims the instance first | Setup needs a 256-bit token printed in the server log, single use, one hour | `Modules/Identity/Identity.Setup.go` | `Modules/Identity/Test/Integration/Setup_test.go`, `Modules/Session/Test/E2E/Flow_test.go` (`TestSetupIsOneShot`) |
| Two setup calls race | One transaction locks the instance row and the token | `Identity.Setup.go` (`create`) | `Setup_test.go` |
| CPU exhaustion through setup | The token is checked before the breach check and Argon2id; per client network limit | `Identity.Setup.go`, `Modules/Identity/Identity.Routes.go` | `App/RateLimit_test.go` |

### Password sign-in

| Threat | Control | Code | Proof |
|---|---|---|---|
| Online guessing and credential stuffing | Failures counted before verification, per account (5) and per client network (20), exponential lock from 30 s to 15 min; IPv6 counted per /64 | `Modules/Session/Session.Throttle.go`, `Core/Auth/Throttle.go` | `Modules/Session/Test/Integration/Throttle_test.go` (including `TestAnIPv6AttackerCannotSpreadOverItsSlash64`) |
| Parallel guesses beat the counter | Reservation in one transaction with `FOR UPDATE` on both counters | `Session.Throttle.go` | `Throttle_test.go` (`TestParallelGuesses...`) |
| Account enumeration by answer | One `INVALID_CREDENTIALS` for every failure, locked or not | `Modules/Session/Session.Login.go` | `Modules/Session/Test/Edge/Generic_test.go`, `Flow_test.go` (`TestLoginErrorsAreGeneric`) |
| Account enumeration by timing | Unknown and disabled accounts run a full Argon2id verification; no difference above max(10%, 300 µs) of the median, about 3.5 ms here | `Session.Login.go`, `Core/Auth/Password.go` | `App/Timing/Timing_test.go` (`make timing`), with a control that must notice 6 ms on sign-in |
| Offline cracking after a database leak | Argon2id (19 MiB, t=2), rehash on sign-in after a parameter change | `Core/Auth/Password.go` | `Core/Auth/Password_test.go` |
| Weak or breached passwords | 12 to 128 characters, Have I Been Pwned range check (k-anonymity) | `Core/Auth/PasswordPolicy.go`, `Core/Auth/Breached.go` | `Core/Auth/Breached_test.go`, `FuzzPasswordLength` |

### Second factor

| Threat | Control | Code | Proof |
|---|---|---|---|
| Skipping the second step | `mfa_pending` challenge on the server, bound to the user and the User-Agent hash; no user id from the client | `Modules/Session/Mfa.Pending.go` | `Modules/Session/Test/E2E/Totp_test.go`, `Passkey_test.go` |
| Guessing TOTP codes | Lockout per user separate from the password counter, 5 codes per challenge | `Mfa.Pending.go` | `Modules/Session/Test/Integration/MfaThrottle_test.go` |
| TOTP replay | The last accepted step is stored and updated atomically | `Modules/Session/Mfa.Factor.go`, `Core/Auth/Totp.go` | `Core/Auth/Totp_test.go`, `FuzzTotp` |
| Stolen session enrolls the attacker's factor | Every factor change needs the current factor in the same request and revokes other sessions | `Modules/Session/Mfa.Factor.go` | `Modules/Session/Test/E2E/FactorChange_test.go` |
| Passkey phishing or cloning | RP ID and origin from `DHOW_PUBLIC_URL`; user verification required for passwordless; a sign count that goes back is refused and audited | `Core/Auth/WebAuthn.go`, `Modules/Session/Mfa.Passkey*.go` | `Passkey_test.go`, `FuzzWebAuthnResponses` |

### Sessions, CSRF and tokens on the wire

| Threat | Control | Code | Proof |
|---|---|---|---|
| Cookie theft by script or sub-domain | `__Host-` cookies, `HttpOnly`, `Secure`, `SameSite=Strict`, `Path=/` | `Core/Auth/Cookie.go` | `Modules/Session/Test/E2E/Secrets_test.go`, smoke |
| Session fixation | A new 256-bit id at every sign-in; the old one is revoked | `Session.Login.go` (`openSession`) | `Modules/Session/Test/Integration/Lifecycle_test.go` |
| Stale sessions | 12 h idle (4 h for anyone who is an Owner or Admin anywhere, from the current role on every request) and 7 days absolute; revocation on sign-out, password, factor and role change | `Modules/Session/Session.Service.go`, `Session.Policy.go` | `Lifecycle_test.go`, `SessionPolicy_test.go`, `Flow_test.go` (`TestRevokeSessions`) |
| CSRF | Origin on every unsafe request, plus an HMAC token bound to the session | `Core/Auth/Guard.go`, `Core/Auth/CSRF.go` | `Modules/Session/Test/E2E/Csrf_test.go`, `Core/Auth/CSRF_test.go` |
| Token or cookie in a URL or a log | Tokens travel in bodies, cookies or the URL fragment; the access log drops the query; log keys are redacted | `Core/Middleware/AccessLog.go`, `internal/logging` | `Secrets_test.go`, `Core/Middleware/Middleware_test.go` |
| Malformed or alternative token spellings | Exact encoded length, strict base64url, then SHA-256 or HMAC; the same for runner tokens | `Core/Auth/Token.go`, `Core/Auth/ApiToken.go`, `Core/RunnerAuth/Tokens.go` | `Core/Auth/Token_test.go`, `Core/RunnerAuth/RunnerAuth_test.go`, `FuzzHashSessionToken`, `FuzzAPITokenHash` |

### External identity providers

| Threat | Control | Code | Proof |
|---|---|---|---|
| Login CSRF and flow injection | `__Host-dhow_oidc` cookie, random state, PKCE S256, nonce; the flow is consumed before the code exchange | `Modules/Session/Oidc.Flow.go` | `Modules/Session/Test/E2E/OidcNegative_test.go` |
| Account takeover by email | Identities match only `(issuer, subject)`; no linking or account creation by email | `Modules/Session/Oidc.Login.go` | `OidcNegative_test.go`, `Oidc_test.go` |
| Forged or replayed ID tokens | `go-oidc` verification, nonce in constant time, `azp`, 10 s timeout, 1 MB cap, no redirects | `Core/Oidc/Oidc.go`, `Core/Oidc/Provider.go` | `Core/Oidc/Oidc_test.go`, `Core/Oidc/Discovery_test.go` |
| Wrong domain or unverified email | Exact domain match on a verified email | `Core/Oidc/Domains.go` | `FuzzAcceptedEmail`, `FuzzEmailVerifiedClaim` |
| Unlinking the last way in | Row lock on the user and a count of every sign-in method | `Modules/Session/Oidc.Link.go` | `Modules/Session/Test/E2E/SignInMethods_test.go` |

### Personal API tokens

| Threat | Control | Code | Proof |
|---|---|---|---|
| A token above its creator | Every request needs the scope and the creator's current effective role | `Core/Policy/Policy.go`, `Modules/Tokens/Tokens.Resolve.go` | `Modules/Tokens/Test/E2E/Tokens_test.go`, `App/RouteTable_tokens_test.go` |
| A token on a browser-only route | Routes accept tokens only when their policy row says so; `openapi.yaml` declares the same set | `Core/Policy/Table.go` | `App/RouteTable_tokens_test.go`, `App/OpenAPI_tokens_test.go` |
| A token confirming a factor | `ConfirmFactor` refuses any principal from a token | `Modules/Session/Session.Service.go` | `Tokens_test.go` |
| Leaked token after offboarding | Bound to the membership row, the account and the organization | `Modules/Tokens/Queries/Tokens.sql` | `Tokens_test.go` |
| Existence by timing | Unknown, revoked and expired tokens take the same path | `Tokens.Resolve.go` | `App/Timing/Timing_test.go` |

### Organizations, roles and invitations

| Threat | Control | Code | Proof |
|---|---|---|---|
| IDOR across organizations | The guard answers 404 for non-members; loaders are scoped to the organization | `Core/Auth/Guard.go` | `App/RouteTable_test.go`, `App/RouteTable_orgs_test.go` |
| Privilege escalation through grants | Nobody grants above their role, changes someone higher or changes their own role; bindings never grant Owner | `Core/Policy/Rank.go`, `Modules/Identity/Members.Service.go` | `Core/Policy/Rank_test.go`, `Modules/Identity/Test/E2E/Members_test.go`, `Modules/Identity/Test/Integration/Bindings_test.go` |
| Owner changes without step-up | Granting, demoting and transferring Owner need the current factor; an Owner is removed only after a demotion | `Members.Service.go`, `Modules/Identity/Orgs.Service.go` | `Modules/Identity/Test/E2E/OwnerRemoval_test.go` |
| Losing the last owner in a race | Row lock on the organization for every membership change | `Orgs.Service.go` | `Modules/Identity/Test/Integration/Owners_test.go`, `Locks_test.go` |
| Admin MFA reset as a takeover path | Strictly higher rank in every organization of the member, current factor, high-severity audit | `Members.Service.go` | `Modules/Identity/Test/E2E/MfaRank_test.go`, `MfaReset_test.go` |
| Admins without a second factor | `RequireSecondFactor` per organization, 7-day grace, then `403 SECOND_FACTOR_REQUIRED` on sensitive actions for sessions and tokens | `Core/Policy/SecondFactor.go`, `Modules/Identity/Identity.SecondFactor.go` | `App/RouteTable_secondfactor_test.go`, `Modules/Identity/Test/E2E/SecondFactor_test.go`, `Core/Policy/SecondFactor_test.go` |
| A credential change the user does not notice | An email in the user's language after each of 10 credential events, in the background, with no secret; the audit in every case | `Modules/Alerts` | `Modules/Alerts/Test/E2E/Alerts_test.go`, `Modules/Alerts/Test/Unit/Message_test.go` |
| Invitation link leaks or replays | Token in the fragment, SHA-256 stored, single use, 7 days, one answer for every invalid state | `Modules/Identity/Invites.Accept.go` | `Modules/Identity/Test/E2E/Invitations_test.go`, `App/Timing/Timing_test.go` |
| The server as a spam relay | Per member and per organization invitations each hour, 5 sends per invitation | `Modules/Identity/Invites.Service.go` | `Modules/Identity/Test/E2E/InviteLimits_test.go` |

### Audit, limits and retention

| Threat | Control | Code | Proof |
|---|---|---|---|
| Tampering with the trail | Hash chain per organization, trigger refusing `UPDATE`, `DELETE` and `TRUNCATE`, verify endpoint | `Modules/Audit/Audit.Service.go` | `Modules/Audit/Test/Integration/Chain_test.go` |
| Secrets in the trail | Explicit allowlist of event fields; values with a Dhow credential prefix are dropped | `Modules/Audit/Audit.Data.go` | `Modules/Audit/Test/Integration/Sanitizer_test.go`, `Modules/Audit/Test/Unit/DataKeys_test.go` |
| Missing events | The harness fails every E2E test whose flow loses an audit field | `App/Testing/Harness.go` | every E2E test |
| Flooding public routes | Fixed windows in PostgreSQL per client network on the database clock, shared by every instance; counted only after the Origin check; keys over the limit refused from memory; a forged `X-Forwarded-For` is ignored without a trusted proxy; every public write is limited or a listed exception | `Core/RateLimit` | `Core/RateLimit/Postgres_test.go`, `App/RateLimit_test.go` (including `TestACrossSitePageCannotSpendTheLimitOfAnOffice` and `TestEveryPublicWriteIsRateLimitedOrAnExplicitException`), `FuzzClientIP`, `FuzzNetwork` |
| Tables growing with every attempt | Batched deletes of expired temporary rows under an advisory lock, on the database clock; a locked address creates no new throttle rows | `Database/Retention`, `Modules/Session/Session.Throttle.go` | `Database/Retention/Retention_test.go`, `Throttle_test.go` (`TestALockedAddressCreatesNoMoreThrottleRows`) |
| Destructive SQL slipping into queries | A project hook strips SQL comments and refuses `DELETE` and `TRUNCATE` in every queries file, except the batch template on the temporary tables in `Retention.sql` | `.claude/hooks/dhow_hooks/edit_rules.py` | `.claude/hooks/tests/test_hooks.py` (`make hooks-test`) |
| Configuration mistakes | The server refuses to start on a missing, placeholder or reused secret and on any invalid provider or limit | `Config` | `Config/Secrets_test.go`, `Config/Oidc_test.go`, `Config/RateLimit_test.go` |

## 4. ASVS 4.0.3 level 2

| Area | Status | Notes |
|---|---|---|
| V2.1 Password security | Met | 12 to 128 characters, no composition rules, breached-password check, paste allowed |
| V2.2 General authenticator | Met | Anti-automation met. 2.2.3: an email after every credential change when SMTP is configured, and the audit log in every case (F12) |
| V2.4 Credential storage | Met | Argon2id; recovery codes and API tokens under HMAC with derived keys |
| V2.5 Credential recovery | Partial | No self-service recovery. A lost second factor is reset by a higher-ranked admin with a factor; a forgotten password has no flow (finding F13) |
| V2.8 One-time passwords | Met | TOTP with replay protection, one step of skew |
| V2.9 Cryptographic authenticators | Met | Passkeys with user verification for passwordless sign-in |
| V3.2 Session binding | Met | 256-bit ids, new id at sign-in |
| V3.3 Session termination | Partial | 7 days absolute, 12 hours idle and 4 hours for Owners and Admins. 3.3.2 asks for re-authentication after 12 hours of use; the 7 days are a product decision (F10, ADR-0054) |
| V3.4 Cookies | Met | `__Host-`, `Secure`, `HttpOnly`, `SameSite=Strict` |
| V3.5 Token-based sessions | Met | API tokens scoped, expiring, revocable, HMAC stored |
| V3.7 Re-authentication | Met | Current factor in the same request for every sensitive change |
| V4.1 and V4.2 Access control | Met | One policy function, deny by default, route-table tests, 404 across tenants, Origin and CSRF |
| V4.3.1 MFA for administrative interfaces | Met when enabled | An organization can require a second factor for Owners and Admins or everyone, with a 7-day grace period (F11) |
| V7.1 to V7.3 Logging | Met | No credentials in logs or audit data, security events audited, tamper-evident chain |
| V8.3.1 Sensitive data in URLs | Met | Tokens in bodies, cookies or fragments only |
| V11.1.4 Anti-automation | Met | Per-network limits on public routes after the Origin check, invitation caps |
| V14.4 HTTP security headers | Partial | CSP with Trusted Types (finding F9 fixed in M7.10) |

## 5. Findings of the internal review

| ID | Severity | Finding | Status |
|---|---|---|---|
| F1 | High | An Owner could remove another Owner without the current factor, and so could an Owner's API token with `member.manage`, while demoting an Owner needed the factor. | Fixed: an Owner is removed only after a demotion, which needs the factor. `OwnerRemoval_test.go` |
| F2 | Medium | Address lockouts counted IPv6 per address, so one host with a /64 escaped the 20-failure address lock. | Fixed: per /64. `Throttle_test.go` |
| F3 | Medium | Invitation preview and accept, OIDC authorize and callback, and passkey options had no per-address limit, and invitations had no cap (spam relay). | Fixed in T1: `Core/RateLimit`, `App/RateLimit_test.go`, `InviteLimits_test.go` |
| F4 | Medium | The audit sanitizer dropped every field containing "session", so `RevokedSessions` was never recorded. | Fixed in T2: allowlist. `Sanitizer_test.go`, `DataKeys_test.go` |
| F5 | Low | Go's base64 decoder skips CR and LF, so a token with a line break was accepted as another spelling of the same token (found by fuzzing). | Fixed: exact length. `Token_test.go` |
| F6 | Low | A role change that read the target as non-owner could demote someone promoted to Owner in parallel without the factor. | Fixed: the change answers `409 CONFLICT` when the role moved under it. |
| F7 | Low | Throttle, challenge, flow and counter rows were never deleted. | Fixed in T3: `Database/Retention` |
| F8 | Low | `POST /setup` had no per-address limit. | Fixed: per client network. `App/RateLimit_test.go` |
| F9 | Medium | The CSP has no `require-trusted-types-for 'script'`, although the security model listed Trusted Types. | Fixed in M7.10 ([ADR-0115](../../../Plan/ADR/0115-trusted-types-enforced.md)). The CSP now carries `require-trusted-types-for 'script'; trusted-types 'none'` in every mode, after the browser tests passed over every screen with no violation. |
| F10 | Medium | Sessions last up to 14 days (12 hours idle); ASVS 3.3.2 asks for re-authentication after 12 hours. | Fixed in M2.6 (ADR-0054): 7 days absolute, also for sessions opened before, and 4 hours idle for anyone who is an Owner or Admin in any organization, from the current role. `SessionPolicy_test.go` |
| F11 | Medium | Owners and admins are not required to use a second factor (ASVS 4.3.1). | Fixed in M2.6 (ADR-0054): the `RequireSecondFactor` organization setting with a 7-day grace period, then `403 SECOND_FACTOR_REQUIRED` on sensitive actions. `RouteTable_secondfactor_test.go`, `Modules/Identity/Test/E2E/SecondFactor_test.go` |
| F12 | Medium | No email to the user after a password, factor or token change (ASVS 2.2.3). | Fixed in M2.6 (ADR-0054): `Modules/Alerts` emails the user after each of 10 credential events when SMTP is configured, in the background and without any secret. `Modules/Alerts/Test` |
| F13 | Low | No recovery for a forgotten password. | Accepted for M2: an admin resets the second factor, and passwordless passkeys and linked identities still sign in. A recovery flow is a product decision. |
| F14 | Low | An attacker who knows an email can keep that account's password sign-in locked (ADR-0035). | Accepted: the lock is short and doubling, and passkey and provider sign-in are not counted against the account. |
| F15 | Info | API token guessing has no per-address limit. | Accepted: 256-bit tokens; each attempt costs one indexed lookup. |
| F16 | Medium | Independent review: the limiter ran before the Origin check, so a cross-site page could spend the limit of an office behind one NAT with no-cors requests. | Fixed: the limiter runs inside the Guard. `TestACrossSitePageCannotSpendTheLimitOfAnOffice` |
| F17 | Low | Independent review: a locked address still created one `LoginThrottles` row per new email, and password sign-in, the second step and passkey sign-in had no per-network limit. | Fixed: the reservation stops at the first locked key, and the three routes are limited. `TestALockedAddressCreatesNoMoreThrottleRows`, `App/RateLimit_test.go` |
| F18 | Low | Independent review: one instance with a fast clock froze the rate limit window for all others. | Fixed: the window and retention use the database clock. `TestWithoutAClockTheDatabaseDecidesTheWindow`, `TestWithoutAClockRetentionUsesTheDatabaseTime` |
| F19 | Low | Independent review: `DELETE /**/ FROM`, `FROM"X"`, a comment between the words, and `WHERE true` or a top-level `OR` in `Retention.sql` bypassed the hook. | Fixed: comments stripped, whole file checked, one batch template in `Retention.sql`. Hook tests for every form |
| F20 | Low | Independent review: a resend spent the hourly limits before the existence, role and send-count checks. | Fixed: limits taken inside the transaction after them. `TestAResendThatCannotHappenSpendsNothing` |
| F21 | Low | Independent review: a client already over a limit still cost one upsert per request. | Fixed: a per-instance memory of blocked keys until the window ends. `TestAKeyKnownToBeOverTheLimitIsRefusedWithoutTheDatabase` |
| F22 | Info | Independent review: the timing control proved 1 ms only on a sub-millisecond path. | Addressed: the guarantee is now written as max(10%, 300 µs) of the median, and a second control must notice 6 ms on sign-in. |
| F23 | Info | Independent review: runner tokens accepted a line break like the other tokens did. | Fixed: exact length. `Core/RunnerAuth/RunnerAuth_test.go` (`TestTokens`) |
| F24 | Info | Independent review: nothing stopped a new public write route from shipping without a limit. | Fixed: `TestEveryPublicWriteIsRateLimitedOrAnExplicitException` |
| F25 | Medium | M2.6 review: the audit record and the credential alert ran on the request context after the commit, so a client that dropped the connection at that moment kept the change without either. | Fixed: both handlers detach from the request. `TestAnEventIsRecordedEvenWhenTheRequestIsGone`, `TestTheAlertIsSentEvenWhenTheRequestIsGone` |
| F26 | Low | M2.6 review: widening the second-factor rule from admins to everyone restarted the grace period of admins who were already overdue. | Fixed: a separate start for Owners and Admins. `TestWideningGivesOnlyNewlyCoveredRolesANewGracePeriodAndNarrowingKeepsIt` |
| F27 | Low | M2.6 review: every alert opened its own goroutine and SMTP connection with no cap. | Fixed: at most 32 pending sends; the rest are dropped with a warning. `TestPendingAlertsAreCapped` |
| F28 | Low | M2.6 review: the MFA-reset alert showed the admin's network and browser to the member. | Fixed: a separate text without them. `TestTheMfaResetAlertLeavesOutTheAdminsNetworkAndBrowser` |
| F29 | Info | M2.6 review: an alert could start while shutdown waited for the pending ones, and the session list showed sessions past the 4-hour idle window. | Fixed: the alert service closes before it waits, and the list uses the current idle window. `TestNoAlertStartsAfterShutdown`, `TestTheSessionListLeavesOutSessionsPastTheCurrentIdleWindow` |
| F30 | Info | M2.6 review: a mail server's error reply, which may name the address, reaches the server log; `token.manage` and `job.cancel` are not blocked after the grace period. | Accepted (ADR-0054): the log is the operator's, as for invitations; revoking tokens and cancelling jobs only reduce access. |

No Critical or High finding is open.

## 6. For the external reviewer

- Please look first at: the `Core/Auth` guard ordering (Origin, Bearer, session, CSRF, policy), the OIDC flow and linking, the owner and MFA-reset rules under concurrency, the audit chain, and the rate limiter's client-network logic behind proxies.
- Known gaps you will see: findings F9 to F15 above, the absence of back-channel logout from providers (ADR-0039), and project or environment scopes for tokens (M4).
- A test instance: `make compose-up` after `deploy/compose/init.sh`, or `deploy/smoke.sh <image>` for a scripted run.

## 7. Reproducing the evidence

| Check | Command |
|---|---|
| Everything CI runs | `make check` with `DHOW_TEST_DATABASE_URL` pointing at PostgreSQL 18 |
| Fuzzing | `make fuzz FUZZTIME=60s` |
| Timing | `make timing` |
| Image end to end | `make smoke` |
| Project hooks | `make hooks-test` |
