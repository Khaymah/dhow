[العربية](../../install/installer.md)

# The interactive installer

`dhow-server install` sets up a complete Dhow host: the `dhow` service user, PostgreSQL 18, the secrets, the hardened systemd unit, a reverse proxy with TLS and the firewall. It asks before it installs anything, shows the whole plan before it changes anything, and can be run again at any time. `dhow-server doctor` checks an install and proposes fixes, and `dhow-server uninstall` removes it. The decisions behind it are in [ADR-0036](../../../Plan/ADR/0036-interactive-host-installer.md).

The installer is part of the `dhow-server` binary, so there is one artifact to verify. It runs every system command as an argument list, never through a shell.

## Get it

`deploy/install/get.sh` downloads the right release for the host, verifies it and starts the installer. Download it, read it, then run it. Never pipe it into a shell:

```sh
curl -fsSLO https://raw.githubusercontent.com/Khaymah/dhow/main/deploy/install/get.sh
less get.sh
bash get.sh
```

`get.sh`:

1. Picks `amd64` or `arm64` from `uname -m`.
2. Downloads `dhow-server_<version>_linux_<arch>.tar.gz`, `SHA256SUMS` and `SHA256SUMS.sigstore.json` of the latest release, or of `--version 1.2.3`, into a temporary directory.
3. With [cosign](https://docs.sigstore.dev/) installed, checks that the release workflow of this repository signed `SHA256SUMS`, keyless. Without cosign, it warns and prints the `cosign verify-blob` command to run yourself.
4. Checks the archive against `SHA256SUMS`, extracts `dhow-server`, and runs `dhow-server install` with every other argument, through `sudo` when you are not root. The temporary directory is removed afterwards.

So `bash get.sh --version 1.2.3 --answers answers.yaml --yes` installs a pinned version without questions. `--component runner` fetches the runner archive of the same release instead, with the same checks, and installs `dhow-runner` and its unit ([Install a runner](../runner/install.md)). `DHOW_DOWNLOAD_URL` points the downloads at a mirror of the release files (`https://` only). With a mirror, or with `DHOW_REPO` set, cosign is required: the signature is the only thing that ties a mirror's files to this repository's release workflow, so without cosign `get.sh` stops. `--insecure-skip-signature` skips the check with a loud warning, for a mirror you control and trust; never use it otherwise. You can also download and verify a release by hand as in [Install the binary with systemd](binary-systemd.md#1-download-and-verify-a-release), and run `./dhow-server install` from it.

## Supported systems

| Distribution | Packages from | Notes |
|---|---|---|
| Debian 12 | apt, PGDG for PostgreSQL 18 | Caddy, nginx and certbot from Debian |
| Ubuntu 24.04 | apt, PGDG | Caddy from universe |
| Ubuntu 22.04 | apt, PGDG | No Caddy package: choose nginx, an existing proxy or none |
| Rocky Linux 9, AlmaLinux 9 | dnf, PGDG | Caddy and certbot from EPEL, which the installer enables when needed; the AppStream `postgresql` module is disabled |
| RHEL 9 | dnf, PGDG | EPEL needs a subscription, so Caddy and certbot must already be installed |
| Fedora 42, 43 and 44 (the releases PGDG publishes PostgreSQL 18 for) | dnf5, PGDG | A dnf drop-in keeps Fedora's own `postgresql` packages from replacing the PGDG ones |

On Fedora, Rocky Linux, AlmaLinux and RHEL, the installer first checks that PGDG publishes PostgreSQL 18 for the release, before it changes anything. When PGDG does not, as with Fedora 45 today, the installer stops, lists the supported releases and changes nothing: install on one of them, or use [Docker Compose](compose.md). When the check cannot reach PGDG, the installer goes on and says so in the plan.

Debian 12, Ubuntu 24.04, Rocky Linux 9 and Fedora 44 are tested end to end in CI (see [How it is tested](#how-it-is-tested)); Ubuntu 22.04, AlmaLinux and RHEL take the same code paths as their tested relatives. On any other distribution the installer runs in **manual mode**: it installs no packages, lists what is missing, and does everything else the same. With an existing PostgreSQL and an existing proxy, manual mode needs no packages at all. The installer always needs systemd.

The installer runs as root. Only `install --dry-run` works without it.

### Host size

The installer reads the CPU cores (`/sys/devices/system/cpu/online`), the memory (`MemTotal` in `/proc/meminfo`) and the free space in `/var/lib`, and compares them with the [hardware requirements](../requirements.md):

| | Minimum | Recommended |
|---|---|---|
| CPU cores | 1 | 2 |
| Memory | 2 GiB | 4 GiB |
| Free disk in `/var/lib` | 5 GiB | 20 GiB |

The kernel keeps some memory for itself, so a host passes the memory check when it reports at least 90% of the number above (a "2 GB" VPS reports about 1.9 GiB). Below the minimum, the plan stops with a message that names the measured minimum and links the requirements, and nothing is changed. Below the recommended size, the installer prints a warning and continues. A re-run on a host where Dhow is already installed, which is how upgrades work, only warns, so a security upgrade is never blocked; a new install on that host would be refused. The numbers live in one place, `apps/server/internal/Core/Installer/Requirements.go`.

## The questions

The installer speaks English or Arabic. It follows `--lang en|ar`, then `lang` in an answers file, then the locale (`LC_ALL`, `LC_MESSAGES`, `LANG`), and uses English when the terminal is not UTF-8. It first prints what it found: the distribution, systemd, root, Docker and Compose, PostgreSQL versions, nginx, Caddy and certbot, which of ports 80, 443 and 8080 are in use, ufw or firewalld, and an earlier Dhow install. On a re-run, the answers saved by the last run are the defaults.

1. **How Dhow runs.** Asked only when Docker with the Compose plugin is present:
   - **Native**, the binary with the [hardened unit](binary-systemd.md#what-the-unit-does). The default and the most secure.
   - **Docker Compose**, the [Compose stack](compose.md) in `/etc/dhow/compose`, with its own PostgreSQL. The installer warns that anyone in the `docker` group is effectively root on the host, and it never adds anyone to that group.
2. **The domain**, for example `dhow.example.com`. `DHOW_PUBLIC_URL` becomes `https://<domain>`. Dhow needs a whole host name, never a path ([why](reverse-proxy.md#no-sub-path)).
3. **PostgreSQL 18** (native only):
   - **Install it here** from the PostgreSQL project's repository (PGDG). The installer downloads the repository signing key and refuses it unless its fingerprint is the pinned one (`B97B0AFC AA1A47F0 44F244A0 7FCC7D46 ACCC4CF8` for apt, `D4BF08AE 67A0B4C7 A1DBCCD2 40BCA2B4 08B40D20` for dnf). It then creates the `dhow` role and database with a generated password. Offered only when no other PostgreSQL uses port 5432.
   - **Use an existing server.** You type its URL, which is not shown while you type. The installer checks the `sslmode` (`require`, `verify-ca` or `verify-full`; it recommends `verify-full` with `sslrootcert`), connects, and refuses servers older than 18.
4. **The reverse proxy:**
   - **Caddy** with automatic TLS from Let's Encrypt. The installer writes `/etc/caddy/dhow.caddyfile`, adds one `import` line at the top of `/etc/caddy/Caddyfile`, and validates the result before it reloads Caddy.
   - **nginx with certbot.** The installer asks for an email address for the Let's Encrypt account, serves the certificate challenge from `/var/www/dhow-acme`, gets the certificate with `certbot certonly --webroot`, then writes the TLS configuration to `/etc/nginx/conf.d/dhow.conf` and a renewal hook that reloads nginx.
   - **A proxy you already run.** The installer prints the exact nginx and Caddy configuration for your domain and asks for the proxy's address for `DHOW_TRUSTED_PROXIES` (`127.0.0.1/32,::1/128` when it runs on the same host).
   - **None**, when TLS ends somewhere else. Dhow then listens on `127.0.0.1:8080` only.

   The nginx and Caddy files are the configurations tested by `make proxy-check` in [Put Dhow behind a reverse proxy](reverse-proxy.md), with your domain filled in. For nginx, `http2 on;` is left out because the distributions' nginx is older than 1.25.1, which introduced it. With a managed proxy on SELinux, the installer turns on `httpd_can_network_connect` so the proxy can reach Dhow.
5. **The breached-password check.** Answer no on hosts without internet access; it sets `DHOW_BREACHED_PASSWORD_CHECK=false`, which locks the setting in the dashboard. Answering yes writes nothing, so the instance owner can change it later in **Server settings** ([ADR-0063](../../../Plan/ADR/0063-instance-settings-in-the-database.md)). A re-run removes a `DHOW_BREACHED_PASSWORD_CHECK=true` line an older installer wrote.
6. **The firewall.** Asked only with Caddy or nginx and an active ufw or firewalld: open ports 80 and 443, and nothing else. The default is no.
7. **Missing packages.** The installer lists every package it would install, for example `postgresql-18 caddy`, and installs nothing unless you agree.

## The plan

Before it changes anything, the installer prints every step with the files it will write, the packages it will install and the commands it will run, marks the steps that are already done, and asks once: `Apply this plan? [y/N]`. Then it runs the steps in order:

- **Every step is idempotent.** It first checks whether its result is already in place and skips itself if so. A second run on the same host changes nothing and does not even restart the service.
- **A failing step rolls back what it did**, for example restoring the file it replaced, and the installer stops. The steps that finished stay, so after fixing the cause you run the installer again and it continues where it stopped.
- **Secrets are never replaced.** An existing secret key or database URL is kept, so a re-run can never make stored data unreadable.

At the end it prints the one-time setup link from the journal, which creates the owner account within an hour, or the command that finds it: `journalctl -u dhow | grep setup_url`. The link is never written to a file.

### What it writes

| Path | Mode | Content |
|---|---|---|
| `/usr/local/bin/dhow-server` | `755` | The binary that ran the installer |
| `/etc/dhow/`, `/etc/dhow/credentials/` | `700` | |
| `/etc/dhow/credentials/dhow-secret-key` | `600` | 32 random bytes as hex, from the kernel's random generator |
| `/etc/dhow/credentials/dhow-database-url` | `600` | The database URL. For a local database: `postgres://dhow:<generated>@/dhow?host=/var/run/postgresql&port=5432&sslmode=require`, a Unix socket that never touches the network |
| `/etc/dhow/dhow.env` | `600` | `DHOW_PUBLIC_URL`, `DHOW_LISTEN_ADDR`, `DHOW_TRUSTED_PROXIES`, `DHOW_LOG_LEVEL`, and `DHOW_BREACHED_PASSWORD_CHECK=false` only when the check is turned off |
| `/etc/dhow/answers.yaml` | `600` | Your answers, without any secret, for re-runs, `doctor` and `uninstall` |
| `/etc/systemd/system/dhow.service` | `644` | [`deploy/systemd/dhow.service`](../../../deploy/systemd/dhow.service) |
| `/etc/apt/keyrings/dhow-pgdg.asc`, `/etc/apt/sources.list.d/dhow-pgdg.sources` | `644` | The PGDG key and repository on apt systems |
| `/etc/pki/rpm-gpg/RPM-GPG-KEY-dhow-pgdg`, `/etc/yum.repos.d/dhow-pgdg.repo` | `644` | The same on dnf systems, with `gpgcheck` and `repo_gpgcheck` on |
| `/etc/caddy/dhow.caddyfile` or `/etc/nginx/conf.d/dhow.conf` | `644` | The proxy site |

The Compose install writes `/etc/dhow/compose/compose.yaml` and `/etc/dhow/compose/.env` (mode `600`, with a generated `POSTGRES_PASSWORD` and `DHOW_SECRET_KEY`) instead of the unit and the credentials, pins `DHOW_IMAGE` to the installer's version, starts the stack with `docker compose up --wait`, and trusts the gateway of the `dhow_edge` network when a proxy on the host is in front of it.

## Answers file

`--answers file.yaml` reads every answer from a file and asks nothing. The file is strict: an unknown key, a second YAML document or an invalid value stops the installer before it does anything, with every problem listed. It never holds a secret.

| Key | Values | Default | Meaning |
|---|---|---|---|
| `lang` | `en`, `ar` | from the locale | Language of the output |
| `mode` | `native`, `compose` | `native` | How Dhow runs |
| `domain` | a host name | required | The domain users open |
| `public_url` | an `https://` URL | `https://<domain>` | Overrides `DHOW_PUBLIC_URL`, for example with a port |
| `listen_addr` | `host:port` | `127.0.0.1:8080` | `DHOW_LISTEN_ADDR`, or `DHOW_PUBLISH` for Compose |
| `database.mode` | `install`, `existing` | `install` | Install PostgreSQL 18 here, or use a server you have. Compose always uses `install` |
| `database.url_file` | a path | | With `existing`: a file of mode `600` that holds the `postgres://` URL. The installer copies the URL into the credentials and never prints it |
| `proxy.kind` | `caddy`, `nginx`, `existing`, `none` | `caddy` | The reverse proxy |
| `proxy.email` | an email address | | Required with `nginx`, for the Let's Encrypt account |
| `proxy.trusted_proxies` | IPs and CIDRs | | With `existing` or `none`: `DHOW_TRUSTED_PROXIES`. Caddy and nginx always get `127.0.0.1/32,::1/128` |
| `firewall` | `true`, `false` | `false` | Open ports 80 and 443 in ufw or firewalld. Only with `caddy` or `nginx` |
| `install_packages` | `true`, `false` | `false` | Consent to install the missing packages the plan lists |
| `breached_password_check` | `true`, `false` | `true` | `DHOW_BREACHED_PASSWORD_CHECK` |

An example for a fresh Debian host with Caddy:

```yaml
lang: en
mode: native
domain: dhow.example.com
database:
  mode: install
proxy:
  kind: caddy
firewall: true
install_packages: true
breached_password_check: true
```

And one for an existing database and a proxy on another machine:

```yaml
mode: native
domain: dhow.example.com
database:
  mode: existing
  url_file: /root/dhow-database-url
proxy:
  kind: existing
  trusted_proxies: 10.0.0.5
install_packages: false
```

## Flags

| Flag | Commands | Effect |
|---|---|---|
| `--answers file.yaml` | `install` | Read the answers from a file |
| `--dry-run` | `install` | Detect, ask and print the plan, then stop. Changes nothing and works without root |
| `--yes` | `install`, `uninstall` | Skip the final confirmation. `uninstall --purge` still needs the typed phrase |
| `--lang en\|ar` | all | Language |
| `--fix` | `doctor` | Apply every proposed fix without asking |
| `--purge` | `uninstall` | Also delete the data, the secrets and the local database |

## doctor

`dhow-server doctor` reads `/etc/dhow/answers.yaml` and checks:

- The binary, the unit and the configuration exist, and the unit matches the one shipped with this release.
- Ownership `root:root` and the modes in [What it writes](#what-it-writes).
- The secret key and the database URL are present and pass the same checks as at boot. Doctor never generates a new secret key, because a new key cannot read the old data.
- The service is enabled and running, and `/api/v1/health/ready` reports the database, the migrations, today's job log partition and the last settings reload as ready. It also says whether the first owner still has to be created.
- `DHOW_TRUSTED_PROXIES`: not `0.0.0.0/0` or `::/0`, no very large public range, and it includes `127.0.0.1` when the proxy runs on this host.
- Through the public URL: the proxy answers, the TLS certificate has at least 21 days left, the proxy passes `Origin` (a request with your origin is not refused with `CSRF_FAILED`, one with a foreign origin is), and the client address reaches Dhow (a probe request shows up in the log with a `client_ip` that is not the proxy's). For nginx it also checks that the site sets `Host`, `X-Forwarded-For` and `X-Forwarded-Proto`. These checks are skipped with proxy `none`.
- CPU cores and memory against the [hardware requirements](../requirements.md): below the minimum is a failure, below the recommended size a warning.
- Free disk space in `/var/lib` and in the PostgreSQL data directory: less than 1 GiB is a failure, less than 5 GiB a warning.
- Where each server setting and the sign-in providers come from: the environment, the database or the default. It names the setting, never its value. When `DHOW_OIDC_PROVIDERS` is set and some accounts are linked to providers saved in the database under an id the variable does not list, that is a warning with the number of accounts and providers, because those accounts cannot sign in with them. This check reads the database, so it is skipped with Compose.
- Multipath TCP in the kernel (`/proc/sys/net/mptcp/enabled`), with the kernel release: a warning while it is on, for [CVE-2026-98432](../security/kernel-advisories.md). Doctor never changes the setting.

Every finding comes with its fix: a command, such as restarting the service, renewing the certificate or restoring a file mode, or advice when a person must decide. In a terminal doctor asks before each fix. `--fix` applies them all, and without a terminal doctor only lists them. It exits with status `0` when everything is fine. The MPTCP warning alone does not change that: doctor then ends with "Nothing blocks Dhow, but read the warnings above."

## Upgrade

Run the installer of the newer release, for example `bash get.sh --version 1.3.0`. With the saved answers as defaults, the plan replaces the binary and the unit if they changed, keeps the secrets, the configuration and the data, and restarts the service. Migrations run automatically when Dhow starts, under a lock. If a migration fails Dhow does not start and the old schema stays in place. Take a backup first, and see [Upgrades and rolling back](upgrades.md) for when going back to the previous release is enough.

For Compose, the installer pins `DHOW_IMAGE` to its own version unless you pinned a digest in `.env`, which it keeps.

## Back up

Back up the database and `/etc/dhow` together, as shown in [Back up and restore](binary-systemd.md#back-up-and-restore). The database is useless without `/etc/dhow/credentials/dhow-secret-key`.

## Uninstall

`dhow-server uninstall` stops and disables the service, and removes the unit, the binary, `/etc/dhow/dhow.env` and the proxy site it wrote. It keeps the secrets and answers in `/etc/dhow`, the data in `/var/lib/dhow` and the database, so a later install picks up where you left off.

`dhow-server uninstall --purge` also drops the local `dhow` database and role, deletes `/etc/dhow` and `/var/lib/dhow` and removes the `dhow` user. It asks you to type `delete dhow data`, even with `--yes`. An external database is never touched. PostgreSQL itself, the PGDG repository, the proxy package and Let's Encrypt certificates stay installed, because other software may use them.

## Security notes

- **No shell.** Every command the installer runs is an argument list. Values you type are validated first: the domain against a host name pattern, the email address, IPs and CIDRs, and URLs with the server's own configuration rules.
- **Secrets.** They are generated on the host from the kernel's random generator, written with mode `600` owned by root, and read by systemd for the service. They never appear in the output, the journal, a command line or the answers file. For a local database, PostgreSQL receives a SCRAM-SHA-256 verifier, not the password. The setup link is printed once, because it is how you take ownership; create the owner right away.
- **The docker group.** The Compose path works, but membership of the `docker` group is equivalent to root, and `docker inspect` shows every container's environment, including `DHOW_SECRET_KEY`. The installer never adds a user to the group. Prefer the native path.
- **Signatures.** Releases are reproducible, the checksums are signed keyless with cosign, and each archive has an SPDX SBOM and SLSA build provenance. Install cosign so `get.sh` checks the signature, not only the checksum. Package repositories are trusted only with pinned key fingerprints.

## How it is tested

- Unit tests run the installer against a fake system: detection, plan building, the answers file, idempotency, rollback, doctor and uninstall, and that no secret reaches the output, a command line or the answers file. A test drives the interactive path through a real pseudo-terminal and checks that the database URL is not echoed.
- The [Installer workflow](../../../.github/workflows/installer.yml) runs [`deploy/install/e2e/run.sh`](../../../deploy/install/e2e/run.sh) on Debian 12, Ubuntu 24.04, Rocky Linux 9 and Fedora 44 in a container that boots systemd. It installs from an answers file with PostgreSQL 18 from PGDG, checks readiness, the setup link in the journal, the file modes, the `systemd-analyze security` rating and an idempotent re-run, then runs `doctor`, `uninstall` and `uninstall --purge`. `make installer-e2e E2E_BASE=ubuntu:24.04` runs it locally.
