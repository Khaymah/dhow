[العربية](README.md)

# Dhow

> **Ship to your own servers — safely.**
> A secure, self-hosted continuous deployment platform. Arabic-first.

**Status:** 🚧 Under construction, not ready for production yet. Foundations (M1), identity and organizations (M2) and most
of runners and isolation (M3) are done: password, TOTP, passkey and OIDC sign-in, organizations, teams and roles, the audit
log, and runners that run every job in a throwaway rootless container with the network closed except to allowed hosts,
live logs, and `dhow-runner install`. Pipelines, secrets, approvals and deploy targets come in M4 to M6
([roadmap](docs/en/roadmap.md)). Feedback is welcome through issues and discussions.

---

## What Dhow is

Dhow is a **self-hosted Continuous Deployment control plane**. It takes your code or a built artifact and
delivers it to infrastructure you own (VPS, on-prem servers, Docker hosts, Kubernetes, static/CDN), with:

- **Isolated runners.** Every job runs in an ephemeral, sandboxed container (microVM optional). Nothing runs on the control plane host.
- **Pipeline-as-code.** A `.dhow.yml` file in your repo, plus a visual editor that writes the YAML and opens a PR.
- **Environments and approvals.** Protection rules, required reviewers and deployment windows.
- **Instant rollback.** Atomic releases with health checks and automatic rollback.
- **Scoped, encrypted secrets.** Secrets are injected only at runtime. Workload OIDC removes the need for long-lived cloud keys.
- **Audit by default.** An append-only, tamper-evident log of who deployed what, where, and when.

Dhow **integrates with** your CI. It does not try to replace it. Build inside Dhow, or let GitHub Actions /
GitLab CI build and hand Dhow the artifact through an official action.

## Who it is for

- Developers worldwide who want a secure, open-source way to deploy to their own infrastructure.
- Small and mid-size companies and agencies running VPS fleets or on-prem servers.
- The Arabic-speaking market. The UI is designed **Arabic-first** (RTL first, LTR second), not as a mirrored English UI.

## Editions

| Edition | License | Summary |
|---|---|---|
| **Community** | AGPL-3.0 | Free for any use, including commercial. No limits on users, projects or runners. Includes all security basics. |
| **Business** | Commercial (`ee/`) | Governance for many teams: SAML/SCIM, custom roles, multi-reviewer approvals, canary/blue-green, HA, long audit retention. Priced per active user. |
| **Enterprise** | Commercial (`ee/`) | Compliance and scale: Vault/KMS, microVM isolation, air-gapped install, SIEM export, OPA policies, SLA support. |

See [docs/editions-and-licensing.md](docs/en/editions-and-licensing.md).

## Install

Dhow installs with one Docker Compose file: `./init.sh` generates strong secrets, then `docker compose up -d` starts an empty, hardened instance with its own PostgreSQL. Images are signed with cosign and ship with an SBOM.
See [docs/install/](docs/en/install/README.md) to choose a method, then the [Compose guide](docs/en/install/compose.md) for the quick start, upgrades, backups and signature verification.
A host needs at least 1 CPU core, 2 GiB of memory and 5 GiB of free disk (2 cores, 4 GiB and 20 GiB recommended): see [Hardware requirements](docs/en/requirements.md).

## Documentation

| Doc | Contents |
|---|---|
| [Architecture](docs/en/architecture.md) | Components, data model, job lifecycle, storage, targets, install methods |
| [Security model](docs/en/security-model.md) | Threat model, secure defaults, authn/authz, isolation, lessons learned |
| [Pipeline spec](docs/en/pipeline-spec.md) | `.dhow.yml` schema v1 (draft) |
| [UI principles](docs/en/ui-principles.md) | Arabic-first design, navigation model, performance budgets |
| [Editions & licensing](docs/en/editions-and-licensing.md) | Open-core model, feature matrix, license enforcement |
| [Install](docs/en/install/README.md) | Choosing a method, Docker Compose, reverse proxies, local runs, configuration, troubleshooting |
| [Keyring](docs/en/install/keys.md) | The key encryption keys that protect every stored secret, where they live, rotation without downtime, export and the emergency guide |
| [Hardware requirements](docs/en/requirements.md) | Measured CPU, memory and disk for the server, PostgreSQL and runners |
| [Development](docs/en/development.md) | Running Dhow from source on Linux, macOS or Windows, with or without Docker, and the tests |
| [Sign in with OIDC](docs/en/guides/oidc.md) | Google, Keycloak, GitHub and any OIDC provider, account linking, allowed domains |
| [The home page](docs/en/guides/home.md) | What is running, what needs attention, the sidebar counters, and the overview API behind them |
| [Follow jobs in the web app](docs/en/guides/jobs.md) | The job list, stuck jobs without a runner, stopping a job, the live log, attempts and settings |
| [Projects and runs](docs/en/guides/projects.md) | Create a project, link a Git repository step by step, runs and their stages, manual runs and artifacts |
| [Environments](docs/en/guides/environments.md) | Environments and their server-side protection rules, who manages them, pause and resume, and the API |
| [Deploy to a server over SSH](docs/en/guides/deploy-ssh.md) | The official deploy plugin: atomic releases, a health check, an automatic rollback, and how to prepare the server |
| [Secrets and variables](docs/en/guides/secrets.md) | Write-only secrets, their places and order, versions, variables, and registry credentials |
| [Your organization and your account](docs/en/guides/organization.md) | Members and teams beside their details, invitations, API keys, organization settings, and your profile and security |
| [Server settings](docs/en/guides/instance-settings.md) | Mail, organization creation, the breached-password check, rate limits and log limits from the dashboard, and how the environment locks them |
| [Roadmap](docs/en/roadmap.md) | Milestones toward v1.0 |
| [ADRs](Plan/ADR/) | Every architectural and product decision, with alternatives |

## Relationship to Deploy Center

Dhow is the from-scratch successor to **Deploy Center v3**. We built Deploy Center after an attack on our servers, to
deploy without SSH keys spread everywhere. A later review found problems in its foundations that patching cannot fix:
pipelines run on the control plane host with its privileges, commands are built as shell strings, and sessions cannot be
revoked. So Deploy Center receives security fixes only, and a migration tool will import its projects into Dhow. See
[ADR-0002](Plan/ADR/0002-rewrite-with-v3-hotfix.md).

## License

Core: AGPL-3.0. Code under `ee/`: commercial license. See [LICENSE.md](LICENSE.md).
