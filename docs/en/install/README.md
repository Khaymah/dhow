[العربية](../../install/README.md)

# Install Dhow

Dhow is one server process with the web app built in, and a PostgreSQL 18 database. Its install methods are set in [ADR-0024](../../../Plan/ADR/0024-install-methods.md) and [ADR-0036](../../../Plan/ADR/0036-interactive-host-installer.md).

## Choose a method

| Method | Status | Best for | You manage |
|---|---|---|---|
| [Docker Compose](compose.md) | Available, the default | One host with Docker, from a small VPS up | Docker, the reverse proxy, backups |
| [Run Dhow locally](local.md) | Available | Trying Dhow out, or working on its code | Nothing that others rely on |
| [Binary with systemd](binary-systemd.md) | Available | Hosts without Docker, set up by hand | PostgreSQL, the reverse proxy, the service |
| [Interactive installer](installer.md) (`dhow-server install`) | Available | Setting up the binary, PostgreSQL and a proxy on a fresh Linux host | Answering its questions, backups |
| Helm chart for Kubernetes | With M3, together with the runners | Clusters with an external PostgreSQL | The cluster, the database, ingress |
| Signed air-gapped bundle | Enterprise, later | Networks without internet access | The offline registry and updates |

Jobs run on separate runner hosts: see [Dhow runners](../runner/README.md) to install and register one. The same bootstrap script installs either component: `get.sh` installs the server (`--component server`, the default), and `get.sh --component runner` installs the runner binary and its systemd unit from the same signed release, after which `sudo dhow-runner install` sets up, registers and starts the runner ([Install a runner](../runner/install.md)).

If you are not sure, use Docker Compose. Every method runs the same server with the same [configuration](configuration.md), so you can move between them by restoring a database backup and keeping the same `DHOW_SECRET_KEY`.

## Requirements for every method

- **A Linux host** on `amd64` or `arm64`. The release images are built for both.
- **PostgreSQL 18.** The Compose stack brings its own. Elsewhere, Dhow needs a database and a user that owns it, reached over TLS (`sslmode=require` or stronger).
- **At least 1 CPU core, 2 GiB of memory and 5 GiB of free disk** for Dhow and PostgreSQL on one host; 2 cores, 4 GiB and 20 GiB are recommended. The installer refuses a smaller host and `dhow-server doctor` reports one. See [Hardware requirements](../requirements.md) for the measured numbers and what happens below them.
- **A host name and a TLS certificate.** Users reach Dhow at an `https://` address on its own host name, such as `dhow.example.com`, never under a path like `example.com/dhow`. A reverse proxy in front of Dhow terminates TLS: see [Put Dhow behind a reverse proxy](reverse-proxy.md) for nginx, Caddy and Traefik.
- **Ports 80 and 443** open to users on the proxy. Dhow's own port, 8080, stays on the loopback interface or a private network.
- **Outbound HTTPS, optionally.** It is used to pull images and for the breached-password check, which `DHOW_BREACHED_PASSWORD_CHECK=false` turns off.

## Security checklist

Go through this list before other people use the instance.

**TLS**

- [ ] `DHOW_PUBLIC_URL` is the `https://` address users open, and the browser shows a valid certificate for it.
- [ ] Plain HTTP on port 80 only redirects to HTTPS.
- [ ] `curl -sSI https://dhow.example.com/ | grep -i strict-transport-security` prints exactly one line.
- [ ] Every step of [Verify the setup](reverse-proxy.md#verify-the-setup) passes, including the client address in the log.
- [ ] A database on another machine uses `sslmode=verify-full`, so Dhow checks the server's certificate.

**Secrets and backups**

- [ ] `DHOW_SECRET_KEY` was generated randomly, for example by `init.sh` or `openssl rand -hex 32`, and never reused from another instance.
- [ ] `DHOW_SECRET_KEY` is backed up together with the database backups, somewhere other than the host. A database backup cannot be decrypted without it.
- [ ] The artifacts folder (`DHOW_BLOB_DIR`, the `blobs` volume with Docker Compose) is backed up with the database. Without it, runs keep their history but their artifacts and cache are gone.
- [ ] A restore was tried once, as in [Back up and restore](compose.md#back-up-and-restore).
- [ ] The file that holds the secrets, such as `.env`, has mode `600`. Only administrators can use the Docker socket, because it reveals every container's environment.

**Development mode**

- [ ] `DHOW_DEV` is not set, or is `false`. It allows plain HTTP and unencrypted database connections and is only for [your own machine](local.md).
- [ ] The server log has no `DHOW_DEV=true` warning. With Compose, `docker compose logs dhow | grep DHOW_DEV` prints nothing.

**Network and accounts**

- [ ] `DHOW_TRUSTED_PROXIES` lists exactly the proxies in front of Dhow, as described in [Trusted proxies](reverse-proxy.md#trusted-proxies), and no range that clients connect from.
- [ ] Dhow's port 8080 and PostgreSQL's port are not reachable from the internet.
- [ ] The owner account was created right after the install. Until then, anyone who can read the server log can use the setup link.
- [ ] The image is pinned by digest after [verifying its signature](compose.md#verify-an-image).

**The host kernel**

- [ ] The kernel has the fix for every advisory in [Kernel security advisories](../security/kernel-advisories.md), or its mitigation is in place. `dhow-server doctor` warns while MPTCP is on (CVE-2026-98432).

## More

- [Hardware requirements](../requirements.md): the CPU, memory and disk Dhow and its runners need, and how they were measured.
- [Kernel security advisories](../security/kernel-advisories.md): kernel flaws that affect Dhow, how to check the host, and how to mitigate them.
- [Configuration](configuration.md): every `DHOW_*` variable, its default and its validation.
- [Sign in with OIDC](../guides/oidc.md): sign-in with Google, Keycloak, GitHub or any OIDC provider.
- [Troubleshooting](troubleshooting.md): `CSRF_FAILED`, cookies that are not stored, the public URL and `sslmode` errors, the wrong client address, locked-out sign-ins, and the setup link.
