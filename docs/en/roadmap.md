[العربية](../roadmap.md)

# Roadmap to the first production release

Status: **Draft**. Team: Sabry + Claude.

This plan builds Dhow's **first production release**: the complete free Community edition, secured to the same bar as
v1.0, running on the environments that small and mid-size companies and agencies use most. The order and scope come
from [ADR-0068](../../Plan/ADR/0068-first-production-release-scope.md). Milestones are ordered, and each one ends in a
working, tested, demoable state. There are no calendar dates. Effort is given in rough T-shirt sizes (S < M < L < XL).

Nothing deferred is cancelled: every deferred item is listed under [After the first release](#after-the-first-release)
and stays part of the complete v1.0.

## Supported in the first release

- **Runners:** Debian and Ubuntu, with rootless Podman.
- **Deploy targets:** SSH/VPS and Docker Compose.
- **Git providers:** GitHub (App), GitLab, and any generic Git host through a deploy key and a webhook, which covers Gitea and Forgejo.
- **Edition:** Community only.

## Milestones

| # | Milestone | Status | Scope | Exit criteria | Effort |
|---|---|---|---|---|---|
| M0 | **Design sign-off** | Done | Design system (brand color, Arabic and Latin type, tokens, core components, key screens in RTL and LTR, dark and light); architecture, security model, pipeline spec, editions; ADRs accepted | Sabry approves the design system and ADRs | M |
| M1 | **Foundations** | Done | Monorepo, required CI (tests, lint, vuln scanning, CodeQL, SBOM, signing), Go server skeleton, Postgres migrations with locking, config validation (fail closed), OpenAPI pipeline, web app shell (workspace, panels, ⌘K, i18n ar/en) | `docker compose up` boots an empty, secure instance | L |
| M2 | **Identity & tenancy** | Done | Owner setup flow, invitations, password + TOTP + Passkeys + OIDC, sessions with rotation and revocation, orgs, teams, fixed roles, `can()` policy + route-table tests, API tokens, audit log | All authz tests green; internal review of the auth flows with no open critical or high finding ([checklist](security/auth-review-checklist.md)); the external review moved to M9 | L |
| M3 | **Runner & isolation** | Done on Debian and Ubuntu, M3.13 included (fuzzing, the runner-plane threat review, and `deploy/m3-demo/run.sh`) | Runner binary, registration and identity, leasing over Connect, rootless container sandbox with limits and network policy, log streaming (seq + SSE), cancellation, reconciliation, systemd install. Plan: `Plan/M3-Runner.md` | A job runs isolated on Debian and Ubuntu; killing the runner mid-job reconciles correctly | XL |
| M4 | **Pipelines** | Done except M4.5 and M4.6 (GitHub and GitLab), which wait for test accounts. M4.11 added the nightly fuzzing, the threat review and `deploy/m4-demo/run.sh` | `.dhow.yml` schema + validator + CEL, DAG/matrix, artifacts, cache, container-image plugins, `dhow lint` CLI, the three Git providers (GitHub App, GitLab, generic Git with a deploy key and a webhook), fork-PR policy. Plan: `Plan/M4-Pipelines.md` | Real repos on all three providers run multi-job pipelines; a forged or replayed webhook is refused | XL |
| M5 | **Secrets & environments** | Not started | Envelope encryption with the KEK in a separate key file, KEK rotation, scoped secrets, runner-side masking, environments + protection rules (allowed branches and tags, a single reviewer, a manual deploy pause) + approvals with a second factor. Plan: `Plan/M5-Secrets-Environments.md` | Secrets never appear in API, logs or notifications (tested); approval flow works end to end | L |
| M6 | **Deploy targets** | Not started | SSH/VPS atomic releases + health checks + auto-rollback, Docker Compose, external-CI artifact ingestion (official GitHub Action) | Each target deploys, fails a health check, and auto-rolls back in an E2E test | L |
| M7 | **Product UI** | Not started | Full workspace: overview, deployment panels, virtualized log viewer, approvals, settings, notifications (Email, webhook, Slack, Telegram, Discord), the About, What's new and FAQ pages, contextual help on every page, `doctor` results in the UI, error messages that say how to fix them | Performance budgets met; WCAG 2.2 AA audit passed; Arabic copy reviewed | L |
| M9 | **Hardening** | Not started | Threat-model review, fuzzing (YAML, CEL, webhooks), load tests, chaos tests (runner and DB loss), backup and restore drill with encrypted backup commands (`dhow-server backup`), external penetration test, external review of the authentication flows (from the [M2 checklist](security/auth-review-checklist.md)), fix all findings | No open critical or high findings. This gate is never deferred before any public announcement | L |
| M10 | **Public beta** | Not started | Install methods hardened for beta (the Compose stack and the binary with the interactive installer), docs site (ar/en), Deploy Center v3 importer, community channels, release notes in Arabic and English, an update check against a signed feed (off for air-gapped installs), one-click updates with a backup and automatic rollback, compatibility rules for settings, `/api/v1` and `.dhow.yml` | First external teams in production | M |
| M11 | **First production release (GA)** | Not started | Stabilization, upgrade guarantees (security updates install themselves in the maintenance window with a backup and automatic rollback, a Docker Compose updater, runner self-update, the current release compatible with the previous one), signed releases, security advisory process | GA release; a security update that fails its health check rolls itself back in an E2E test | M |

**Order:** M4.5 and M4.6 when their test accounts exist, then M5, M6, M7, M9, M10 and M11.

**Upgrades:** from now on, every new database migration follows the expand/contract rule: the previous release runs on the new release's schema, so rolling back is safe. The plan is in [`Plan/Upgrades-and-Help.md`](../../Plan/Upgrades-and-Help.md) and [ADR-0077](../../Plan/ADR/0077-upgrades-migrations-and-update-policy.md).

**In parallel, before M1:** the Deploy Center **v3.0.1 security hotfix** for existing users (ADR-0002).

## After the first release

These items are part of the complete v1.0 plan and come after the first production release, to the same security bar:

| Item | Was in | Reference |
|---|---|---|
| Runners on SELinux-enforcing distributions (Fedora, RHEL, Rocky, Alma) | M3 | ADR-0067 |
| Kubernetes: a runner driver (one restricted Pod per job), the Helm chart for the server and runners, and Helm installs | M3 (M3.11) and M10 | ADR-0048 |
| gVisor as an optional isolation level | M3 (M3.8) | ADR-0043 |
| Bitbucket (Cloud and Data Center) | M4 | ADR-0012 |
| The Kubernetes and Static/CDN/SFTP deploy targets | M6 | ADR-0011 |
| The workload OIDC issuer (short-lived tokens for cloud deploys without static keys) | M5 | ADR-0014, and phase M5.8 in `Plan/M5-Secrets-Environments.md` |
| The GitLab CI component for artifact ingestion | M6 | ADR-0010 |
| The visual pipeline editor (writes YAML, opens a PR) | M7 | ADR-0008 |
| Microsoft Teams and WhatsApp notifications | M7 | ADR-0025 |
| **M8: Editions:** license file + entitlements, `ee/` features (SAML/SCIM, custom roles, multi-reviewer, deployment windows and freezes, HA, canary, Vault/KMS, microVM, SIEM, air-gapped bundle). Exit criteria: the paid features work behind entitlements, and expiry degrades gracefully | M8 | ADR-0017 |

**After that:** the managed control plane (ADR-0020), a runner autoscaler, a plugin registry with verified publishers, a Terraform provider, and more languages.
