[العربية](../architecture.md)

# Architecture

Status: **Draft v0.1** (design phase). Decisions referenced as ADR-NNNN live in [`Plan/ADR/`](../../Plan/ADR/).

## 1. Overview

Dhow separates the **control plane** ("the brain") from **runners** ("the hands").
The control plane never executes user code. Runners execute jobs inside ephemeral sandboxes and talk to the
control plane over **outbound-only** connections, so they work behind NAT and firewalls and inside closed networks.

```mermaid
flowchart TB
  subgraph CP["Control Plane (Go, stateless, horizontally scalable)"]
    UI["Web UI (React)"]
    API["Public API<br/>REST + OpenAPI"]
    RPC["Runner API<br/>Connect/gRPC"]
    SCH["Scheduler"]
    WH["Webhook ingest"]
    OIDC["Workload OIDC issuer"]
  end
  PG[("PostgreSQL<br/>state + queue")]
  OBJ[("Object storage<br/>S3 / MinIO / disk<br/>logs · artifacts · cache")]
  RD[("Redis (optional)<br/>pub/sub, rate limits at scale")]

  UI --> API
  API --> PG
  SCH --> PG
  WH --> PG
  RPC --> PG
  API --> OBJ
  RPC --> OBJ
  CP -.-> RD

  subgraph R1["Runner (Go binary)"]
    J1["Job sandbox<br/>rootless container / microVM"]
  end
  subgraph R2["Runner (Go binary)"]
    J2["Job sandbox"]
  end
  R1 -- "outbound HTTPS/HTTP2" --> RPC
  R2 -- "outbound HTTPS/HTTP2" --> RPC

  J1 --> T1["SSH / VPS"]
  J1 --> T2["Docker / Compose"]
  J2 --> T3["Kubernetes"]
  J2 --> T4["Static / CDN / SFTP"]

  GIT["GitHub · GitLab · Bitbucket · Gitea/Forgejo"] -- webhooks --> WH
  CI["External CI (Actions, GitLab CI…)"] -- "artifact + trigger (official action)" --> API
```

## 2. Components

### 2.1 Control plane (`apps/server`, Go — ADR-0003)

A modular monolith shipped as a single binary. The modules are listed below; each one has its own package and owns its own tables. On disk, `apps/server/internal` holds `App` (router and bootstrap), `Config`, `Core` (errors, response envelope, i18n, middleware, event bus), `Database` (migrations and generated queries) and `Modules/<Name>` (routes, controller, service, models, queries, generated API code and tests). Only `App` imports modules, and modules talk to each other through the event bus.

| Module | Responsibility |
|---|---|
| `identity` | Users, sessions, TOTP, Passkeys (WebAuthn), OIDC login. SAML/SCIM come from `ee/`. |
| `authz` | The central `can(subject, action, resource)` policy. Every handler calls it. See the [security model](security-model.md). |
| `orgs` | Organizations, teams, memberships. Multi-org from day one (ADR-0016). |
| `projects` | Projects, repositories, environments, protection rules, targets. |
| `pipelines` | Fetching, parsing and validating `.dhow.yml` (shared Go package with the runner), and evaluating CEL expressions. |
| `scheduler` | Turns triggers into deployments, jobs and steps, and enqueues jobs. Enforces concurrency groups and per-environment locks. |
| `runners` | Runner registration, leasing, heartbeats, capability matching (labels, OS/arch, isolation level). |
| `secrets` | Envelope encryption and runtime secret delivery. |
| `artifacts` | Content-addressed artifacts, plus build cache entries. |
| `logs` | Ingest of sequenced log chunks, storage, and SSE fan-out. |
| `audit` | Append-only, hash-chained audit log. |
| `overview` | Read-only counts for the home page and the sidebar in one request: active and failed jobs, runners by status, pending invitations (ADR-0061). It reads the jobs, runners and invitations tables and owns none. |
| `notify` | A notification queue and channel adapters. |
| `integrations` | Git providers (GitHub App, GitLab, Bitbucket, Gitea/Forgejo, generic Git), commit statuses and checks. |
| `license` | Offline Ed25519 license verification and the entitlement checks. |

**API surfaces (ADR-0005):**
- **Public REST + OpenAPI 3.1** for the UI, CLI, Terraform provider and third parties. The spec is the source of truth, and the TypeScript client for the UI is generated from it.
- **Connect** between runners and the control plane, mounted at `/runner-api/` (or on a dedicated TLS listener). It carries registration, job leasing, log chunks, artifact upload and status updates, with protobuf contracts in `packages/proto`. Only unary RPCs are used, so the runner plane works over HTTP/1.1 or HTTP/2 through any reverse proxy (ADR-0042).
- **SSE** for live UI updates (logs and status). It works through proxies and is authenticated with the session cookie.

### 2.2 Runner (`apps/runner`, Go — ADR-0003, ADR-0007, ADR-0041 to ADR-0048)

- Ships as a single static binary for linux/amd64 and linux/arm64. Jobs only run on Linux; Windows and macOS runners are deferred until they have a sandbox driver.
- **Registration and identity (ADR-0041).**
  - An admin creates a registration token. It is single-use by default, scoped to one organization and its labels, and expires within 24 hours.
  - The runner generates an Ed25519 signing key and an HPKE (MLKEM768-X25519) encryption key locally. The server pins both public keys, and the private keys never leave the host.
  - The runner exchanges a signed challenge for a 15-minute access token. Starting a job needs a fresh signature, so a stolen token alone cannot fetch secrets.
  - The runner verifies the server with TLS and with a pinned Ed25519 key that signs every lease grant.
  - Admins can revoke a runner instantly. There is no mTLS and no internal CA, because TLS terminates at the reverse proxy.
- **Pull-based and outbound only (ADR-0042).**
  - The runner long-polls `Lease` (up to 25 s) and never opens an inbound port.
  - A lease lasts 45 s and is renewed by a heartbeat every 10 s. Every attempt carries a fencing token, and every mutating call is checked against it.
  - The runner kills its own sandbox before the server can consider the lease lost, so two attempts never overlap.
- **Executes each job in a fresh sandbox (ADR-0043, ADR-0044):**
  - **Default:** a rootless Podman container (crun) running our own Podman configuration. After it is created, the runner inspects the container and fails closed on any mismatch.
    - All capabilities are dropped, with `no-new-privileges`, our seccomp profile, and a separate user-namespace range per job.
    - cgroup v2 limits cover CPU, memory, PIDs and disk (`/workspace` is a sized tmpfs), plus a wall-clock limit.
    - The root filesystem is read-only except `/workspace` and `/tmp`, and no host mounts are allowed.
    - PID 1 is the runner's own agent. It runs the steps, and it exits (killing the container) if the runner goes away.
  - **Network:** always `--network=none`. Egress is opened only through a per-job proxy in the runner, reached through the agent on `127.0.0.1:3128`, with a hostname allowlist from the job definition. There is no DNS inside the sandbox, and private, loopback, link-local and metadata ranges are always denied.
  - **Hardened (optional):** gVisor (`runsc`) or a Firecracker microVM. Firecracker as the default isolation level is an Enterprise feature; gVisor is available in Community.
  - **Kubernetes:** the runner creates one restricted Pod per job (optionally with a gVisor `RuntimeClass` on a dedicated node pool) instead of running containers itself (ADR-0048).
  - **Teardown:** the sandbox, its workspace and any injected secrets are destroyed after every job. On restart, the runner removes any container it left behind.
- **Secrets (ADR-0045).**
  - Secrets arrive once per attempt, sealed with HPKE to the runner's pinned key and bound to the lease and the fencing token.
  - They live only in memory and in the step's environment or a container-private tmpfs, never in container configuration or on disk.
- **Secret masking happens in the runner.** The runner masks secrets on the line-buffered output stream *before* anything is spilled to disk or leaves the host. It also masks the common encodings of each secret (base64, URL-encoded, JSON-escaped, and each line of a multi-line secret).
- **Shared code with the server:** the job spec validator, the YAML schema, the CEL evaluator, and the protocol.

### 2.3 Storage (ADR-0006)

| Store | Use | Notes |
|---|---|---|
| **PostgreSQL 18** (required, for native `uuidv7()`) | All state, plus the job queue (`FOR UPDATE SKIP LOCKED`), advisory locks (migrations, environment locks), and `LISTEN/NOTIFY` for intra-cluster events | The only required stateful dependency. Migrations run as a separate locked step. |
| **Object storage** (required from M4, with a disk default) | Artifacts, build cache, and compacted log segments (live logs stay in Postgres) | S3-compatible (AWS S3, R2, MinIO) or local disk for single-node installs. |
| **Redis** (optional) | Cross-instance pub/sub fan-out, distributed rate limits, and hot caches at high scale | When Redis is absent, the same interfaces fall back to Postgres. |

## 3. Data model (core)

```
Organization ─┬─ Team ── Membership(User, Role)
              ├─ Runner pool / Runner (labels, isolation level)
              ├─ Org secrets
              └─ Project ─┬─ Repository link (provider, repo id, install id)
                          ├─ Project secrets
                          └─ Environment ─┬─ Protection rules (reviewers, branches, windows, wait timer)
                                          ├─ Environment secrets
                                          ├─ Targets
                                          └─ Deployment ── Job ── Step
                                                 │          └─ Log chunks (seq), Artifacts
                                                 └─ Release (artifact digest, target, status, health)
AuditEvent (org-scoped, hash-chained) · NotificationRule · LicenseState
```

The following hold across the model:
- Every row is scoped to an `organization_id`, and every query must go through tenant-scoped repository functions.
- IDs are UUIDv7 (time-ordered), so they cannot be enumerated.
- Soft-delete is used for projects and environments, and audit events are never cascade-deleted.

## 4. Deployment and job lifecycle

```
trigger (push / tag / PR / manual / API / external-CI artifact / schedule)
  → resolve pipeline (.dhow.yml at the commit) → validate schema → evaluate `if:` (CEL)
  → create Deployment (queued) → expand jobs (needs DAG, matrix)
  → environment gate: protection rules → approvals (if required) → wait timer / window
  → enqueue Job (priority, concurrency group, environment lock)
  → runner leases Job (lease with TTL + heartbeat) → sandbox → steps
  → logs streamed (seq-numbered chunks) → artifacts uploaded (digest)
  → deploy step: release to target (atomic) → health check
  → success: mark release current │ failure: auto-rollback to previous release
  → statuses → Git provider checks, notifications, audit events
```

**Reliability rules:**
- **Exactly-once deployment semantics.** A deployment job is never retried automatically after it has started mutating a target. Retries apply only to infrastructure failures that happen *before* the first mutating step. A user can still trigger a re-run explicitly.
- **Leases.** A job whose runner stops heartbeating is marked `lost` and reconciled; the system never assumes the job succeeded. Reconciliation runs at startup and periodically.
- **Environment lock.** At most one active deployment per environment and target, unless a pipeline explicitly opts into parallel deployment.
- **Cancellation** is a state transition that the runner observes. The runner kills the whole sandbox, never individual PIDs on a host.

## 5. Logs

- The runner sends **sequenced chunks** of the form `(attempt_id, seq, ts, stream, bytes)`. `seq` starts at 1 and has no gaps, so appends are idempotent and gaps are detected (ADR-0046).
- From M3, chunks are stored in a Postgres table partitioned by day. Retention drops whole partitions. With M4's blob store, finished attempts are compacted into object-storage segments with an offset index.
- The runner masks secrets before a chunk leaves it, in memory and in its spill file (ADR-0045). The server stores the chunks as it receives them and never masks again, and it never writes the content of a log into its own log.
- The UI reads history with `GET .../jobs/{jobId}/logs?FromSeq=`, then follows over SSE. Clients resume with `Last-Event-ID` from the last `seq` they saw, so there are no gaps and no duplicates. Instances fan out new chunks through `LISTEN/NOTIFY`, and every stream reads its own rows after the last `seq` it sent, so a slow client never holds the others back.
- Steps are collapsible groups, and the UI parses ANSI into styled spans. Raw output is never injected as HTML.
- Logs have a size cap per attempt (`DHOW_JOB_LOG_MAX_BYTES`) and a retention per instance (`DHOW_LOG_RETENTION_DAYS`). Retention per organization comes with the segments in M4.

## 6. Secrets

- **Envelope encryption (two layers).** Each owner has its own data key (DEK) of 32 random bytes: the instance itself, each organization, each project, each environment and each user. A value is encrypted with a subkey derived per value with HKDF-SHA256 from its owner's DEK and 32 random bytes of salt, then AES-256-GCM with a random 12-byte nonce, in the format `0x03 ‖ DataKeyId ‖ salt ‖ nonce ‖ ciphertext ‖ tag`. The AAD binds the value to its data key, its purpose and its row, and `Open` checks the owner of the data key, so a value copied to another row or another owner does not open. Removing an environment or a project destroys its key in the database, so its values never open from it again (crypto-shredding); older backups together with their keyring still open them.
- **KEK.** The DEKs in the `"DataKeys"` table are wrapped with a key encryption key (KEK) that never enters the database. In Community it comes from the [keyring file](install/keys.md), a file separate from `DHOW_SECRET_KEY`; in Enterprise it can later come from HashiCorp Vault Transit, AWS/GCP/Azure KMS or an HSM through the same `KeyProvider` interface, and none of them is ever required. KEKs are versioned, each version has a check value in the database, and the server does not start when the keyring does not match it. Rotation re-wraps the DEKs in the background without re-encrypting any value.
- **Value locations.** Every encrypted column is registered in the code with its owner, its label and the context that binds a value to its row. Values saved before the keyring (with a key derived from `DHOW_SECRET_KEY`) are read and converted in the background, and `dhow-server keys rekey` gives every owner a new data key, encrypts every value again and destroys the old one, so an old keyring with an older database copy does not open the current values ([ADR-0082](../../Plan/ADR/0082-sealer-users-move-to-the-envelope.md)).
- **API exposure.** The API never returns secret values. Secrets are write-only, and reads return metadata only: name, version, updated-by and updated-at.
- **Delivery.** Secrets are decrypted **only** at lease time, only for the job's environment scope, and sent over the runner channel. They are mounted as environment variables or tmpfs files and destroyed when the sandbox is torn down.
- **Untrusted triggers.** Secrets and environments are never available to jobs triggered by untrusted sources, such as PRs from forks, even when an admin approves the run ([ADR-0076](../../Plan/ADR/0076-pipeline-runs-definitions-and-forks.md)).

## 7. Workload identity (OIDC issuer)

After the first release, Dhow publishes a JWKS and signs short-lived job tokens. The claims include `org`, `project`, `environment`, `ref` and `deployment_id`, and the audience can be configured. With these tokens, cloud providers can trust Dhow jobs directly, and users do not need to store long-lived AWS/GCP/Azure keys.

## 8. Artifacts and releases

- An **artifact** is a content-addressed (sha256) tarball or an OCI image. Builds happen either in Dhow or in external CI, and in both cases the deployment references the digest (ADR-0010).
- **Atomic releases on hosts (SSH/VPS):**
  1. Upload the artifact to `releases/<release-id>/`.
  2. Link shared directories such as `shared/storage` and `.env`.
  3. Run pre-switch hooks.
  4. **Atomically switch the `current` symlink.**
  5. Reload the service (systemd/PM2/custom).
  6. Run the health check.
  7. If the health check fails, switch the symlink back automatically.

  The last N releases are kept (configurable).
- **Rollback** means re-deploying a previous release, or the artifact digest behind it. It **never rebuilds from source**.
- **Container targets** use image digests (never mutable tags) and a rollout status. Kubernetes targets use Helm/Kustomize/manifests with rollout watching.

## 9. Targets (ADR-0011)

| Target | Mechanism | v1.0 |
|---|---|---|
| SSH / VPS | Runner-side SSH with pinned host keys, atomic releases, systemd/PM2 reload | ✅ |
| Docker / Compose host | Remote Docker API over SSH or TLS, image digest, compose up with health wait | ✅ |
| Kubernetes | kubeconfig or workload identity, Helm/Kustomize/manifests, rollout status | ✅ |
| Static / CDN / SFTP | S3 / R2 / Cloudflare Pages, SFTP/FTPS sync for shared hosting, CDN purge | ✅ |

Target credentials are environment-scoped secrets. Host keys and TLS fingerprints are pinned when the target is created.

## 10. Git providers (ADR-0012)

| Provider | Integration |
|---|---|
| GitHub | GitHub App with fine-grained permissions, installation tokens, check runs and Deployments API |
| GitLab | GitLab.com and self-managed, through OAuth app or project/group tokens, commit statuses and environments |
| Bitbucket | Cloud and Data Center |
| Gitea / Forgejo / generic Git | Webhooks with HMAC, or polling, plus deploy keys |

Rules for every provider:
- Webhooks must carry a per-repository secret, verified in constant time over the raw body.
- A delivery-id is used for replay protection.

## 11. Pipelines and plugins

- Pipelines are defined in **`.dhow.yml` in the repository** and versioned with the code. The visual editor generates YAML and opens a PR (ADR-0008). See the [pipeline spec](pipeline-spec.md).
- Reusable steps are **container images** (`uses: registry/plugin@sha256:…`) that receive typed inputs. A plugin can be written in any language and runs inside the same sandbox. Dhow will host a community registry with verified publishers (ADR-0009).

## 12. Notifications (ADR-0025)

- **Channels:** Email (SMTP), a generic **signed webhook**, Slack, Microsoft Teams, Telegram, Discord and WhatsApp Business (Cloud API).
- **Interactive approvals:** Slack and Teams support approve/reject buttons with signed callbacks.
- Notifications are delivered from **their own queue** with timeouts and retries, so they never block deployments.
- **SSRF guard:** destination URLs are resolved and private, loopback and link-local ranges are rejected after DNS resolution. Admins can configure an allowlist.

## 13. Installation (ADR-0024)

| Method | Audience |
|---|---|
| **Docker Compose one-liner** (`curl … \| sh` that writes the compose file plus generated secrets) | Default for most users |
| **Helm chart** | Kubernetes users and HA deployments |
| **Single binary + systemd** | Small VPS and setups without Docker on the control plane |
| **Air-gapped bundle** (signed images, checksums, offline license) | Enterprise |

The first boot does three things:
- generates strong secrets if none were provided;
- **refuses to start** with known placeholder values;
- prints a one-time URL for creating the owner account (registration is closed).

## 14. Observability

- `/metrics` exposes Prometheus metrics: queue depth, lease latency, job and deployment duration and outcome, runner health and API latency.
- OpenTelemetry traces cover the path from trigger to deployment to job.
- Logs are structured JSON with request and trace IDs. Logs are redacted by default.
- Health endpoints: `/health/live` (process) and `/health/ready` (database, object store, migrations up to date, today's job log partition, and the last instance settings reload).

## 15. High availability and scale

- **Control plane.** Instances are stateless, and N replicas can run behind a load balancer. Cross-instance events (SSE fan-out) travel over Postgres `LISTEN/NOTIFY`, or over Redis when it is configured. Schedulers coordinate through advisory locks and leases.
- **Runners** scale horizontally. Runner pools carry labels and can be autoscaled; an autoscaler is planned.
- **Database.** Dhow relies on standard Postgres HA (managed service, Patroni, or a CloudNativePG operator).
- **Backups.** The backup guide covers the database, the object store and **the KEK**. Losing the KEK means losing every secret. Dhow will also ship a restore drill command.

## 16. Repository layout (ADR-0004)

```
apps/
  server/     Go control plane
  runner/     Go runner
  web/        React UI (Vite)
  cli/        Go CLI (`dhow`)
packages/
  proto/      Connect/gRPC contracts
  spec/       .dhow.yml JSON Schema + CEL function library (Go, used by server & runner)
  openapi/    OpenAPI spec → generated TS client
  ui/         Design-system components (React)
ee/           Commercial features (separate license)
deploy/       compose/, helm/, systemd/, airgap/
docs/
```
