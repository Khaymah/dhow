[العربية](../../install/binary-systemd.md)

# Install the binary with systemd

This page installs Dhow by hand as one binary run by a hardened systemd unit, without Docker. The [interactive installer](installer.md) does the same steps for you and checks them afterwards; read this page to know what it does, or to install on a host it does not support.

## Requirements

- Linux on `amd64` or `arm64` with systemd 247 or newer (Debian 12, Ubuntu 22.04 and 24.04, RHEL, Rocky and Alma 9, and current Fedora all qualify).
- PostgreSQL 18, on this host or reachable over TLS.
- A reverse proxy that terminates TLS, see [Put Dhow behind a reverse proxy](reverse-proxy.md).
- `curl`, `tar`, `sha256sum`, `openssl` and, to check signatures, [cosign](https://docs.sigstore.dev/) 2.4 or newer.

Run every command below as root, for example in `sudo -i`.

## 1. Download and verify a release

Each release has one archive per architecture, a `SHA256SUMS` file, its keyless cosign signature and an SPDX SBOM per archive:

```sh
version=1.0.0
arch=amd64
base=https://github.com/Khaymah/dhow/releases/download/v$version
mkdir -p /root/dhow-$version && cd /root/dhow-$version
curl -fsSLO "$base/dhow-server_${version}_linux_${arch}.tar.gz"
curl -fsSLO "$base/SHA256SUMS"
curl -fsSLO "$base/SHA256SUMS.sigstore.json"
cosign verify-blob --bundle SHA256SUMS.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github\.com/Khaymah/dhow/\.github/workflows/release\.yml@refs/tags/v' \
  SHA256SUMS
sha256sum --check --ignore-missing SHA256SUMS
tar -xzf "dhow-server_${version}_linux_${arch}.tar.gz"
```

`cosign verify-blob` fails unless the release workflow of this repository signed `SHA256SUMS` on a `v*` tag, and `sha256sum` then ties the archive to that file. The build provenance is attested too: `gh attestation verify dhow-server_${version}_linux_${arch}.tar.gz --repo Khaymah/dhow`. The archive holds `dhow-server` (with the web app built in), `dhow.service`, `LICENSE` and `README.md`. The binaries are static (`CGO_ENABLED=0`) and reproducible: building the same tag twice gives the same bytes.

## 2. Install the binary and create the service user

```sh
install -o root -g root -m 0755 dhow-server /usr/local/bin/dhow-server
useradd --system --user-group --no-create-home --home-dir /var/lib/dhow \
  --shell /usr/sbin/nologin --comment "Dhow service" dhow
```

The `dhow` user has no shell and no home directory. systemd creates `/var/lib/dhow` for it with mode `700`.

## 3. Prepare PostgreSQL 18

**A local PostgreSQL 18.** Install it from the [PostgreSQL project's repository](https://www.postgresql.org/download/linux/), then create the role and the database. Connect over the Unix socket, so the database is never on the network:

```sh
password=$(openssl rand -hex 24)
runuser -u postgres -- psql -X -v ON_ERROR_STOP=1 -d postgres <<SQL
\set pw '$password'
CREATE ROLE dhow LOGIN PASSWORD :'pw';
CREATE DATABASE dhow OWNER dhow;
SQL
db_url="postgres://dhow:$password@/dhow?host=/var/run/postgresql&port=5432&sslmode=require"
```

`sslmode=require` is required by Dhow's configuration check, and is ignored on a Unix socket in the same way libpq ignores it. With the default `pg_hba.conf` (`local all all peer`), the `dhow` system user signs in as the `dhow` role by peer authentication, and the password is a fallback for TCP. The password goes to `psql` on standard input, never on its command line. The installer goes one step further and sends PostgreSQL a SCRAM verifier instead of the password, so the password never reaches the server, even with `log_statement` turned on.

**An existing server.** Use a URL with `sslmode=verify-full` and the CA that signed the server's certificate, for example `postgres://dhow:<password>@db.internal:5432/dhow?sslmode=verify-full&sslrootcert=/etc/dhow/db-ca.pem`. The CA file must be readable by the `dhow` user and must not be under `/home` or `/root`, which the unit hides. Dhow needs PostgreSQL 18 or newer.

## 4. Store the secrets

Secrets live in files that only root can read. systemd passes them to the service with `LoadCredential=`, so they never appear in the environment, `ps` or `systemctl show`:

```sh
install -d -o root -g root -m 0700 /etc/dhow /etc/dhow/credentials
umask 077
openssl rand -hex 32 > /etc/dhow/credentials/dhow-secret-key
printf '%s\n' "$db_url" > /etc/dhow/credentials/dhow-database-url
unset password db_url
```

Back up `/etc/dhow/credentials/dhow-secret-key` together with the database: a database backup cannot be decrypted without it. See [Secrets in files](configuration.md#secrets-in-files) for how Dhow reads these files and what it refuses.

## 5. Write the configuration

Settings that are not secret go in `/etc/dhow/dhow.env`, which systemd reads as root before it starts the service:

```sh
cat > /etc/dhow/dhow.env <<'EOF'
DHOW_PUBLIC_URL=https://dhow.example.com
DHOW_LISTEN_ADDR=127.0.0.1:8080
DHOW_TRUSTED_PROXIES=127.0.0.1/32,::1/128
DHOW_LOG_LEVEL=info
EOF
chmod 600 /etc/dhow/dhow.env
```

`DHOW_TRUSTED_PROXIES` above fits a proxy on the same host; see [Trusted proxies](reverse-proxy.md#trusted-proxies) for other setups. Every variable is described in [Configuration](configuration.md). Never set `DHOW_SECRET_KEY` or `DHOW_DATABASE_URL` in this file: together with the credentials they stop the server, because each secret may be set in one place only.

## 6. Install and start the unit

```sh
install -o root -g root -m 0644 dhow.service /etc/systemd/system/dhow.service
systemctl daemon-reload
systemctl enable --now dhow
dhow-server healthcheck && echo ready
journalctl -u dhow | grep setup_url
```

`dhow-server healthcheck` asks `/api/v1/health/ready` on `DHOW_LISTEN_ADDR` and fails until the database is reachable and migrated. The last command prints the one-time link that creates the owner account; open it within an hour. Restart the service for a new link, as described in [Finding the setup link](troubleshooting.md#finding-the-setup-link).

## What the unit does

[`deploy/systemd/dhow.service`](../../../deploy/systemd/dhow.service) is the same file the installer writes. `systemd-analyze security dhow.service` rates it **1.2 OK**, and CI fails if the rating gets worse than OK.

| Setting | Effect |
|---|---|
| `User=dhow`, `NoNewPrivileges=yes`, `CapabilityBoundingSet=` (empty), `AmbientCapabilities=` | The process runs unprivileged and can never gain a capability, not even through a setuid binary. Port 8080 needs none. |
| `ProtectSystem=strict`, `ProtectHome=yes`, `PrivateTmp=yes`, `StateDirectory=dhow dhow/blobs` | The whole file system is read-only for Dhow, `/home`, `/root` and `/run/user` are hidden, `/tmp` is private, and only `/var/lib/dhow` is writable. systemd creates `/var/lib/dhow/blobs` (`0700`) for artifacts and the cache. |
| `PrivateDevices=yes`, `DevicePolicy=closed`, `PrivateUsers=yes`, `PrivateIPC=yes` | No physical devices, a user namespace with only `dhow` and root mapped, and private System V IPC. |
| `ProtectKernelTunables=yes`, `ProtectKernelModules=yes`, `ProtectKernelLogs=yes`, `ProtectControlGroups=yes`, `ProtectClock=yes`, `ProtectHostname=yes`, `ProtectProc=invisible`, `ProcSubset=pid` | The kernel, cgroups, the clock and the host name cannot be changed, and other processes are invisible. |
| `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX` | Only IP and Unix sockets, no netlink or raw packets. |
| `SystemCallFilter=@system-service`, `SystemCallFilter=~@privileged`, `SystemCallArchitectures=native` | An allow-list of the system calls a normal service needs. |
| `MemoryDenyWriteExecute=yes`, `LockPersonality=yes`, `RestrictNamespaces=yes`, `RestrictRealtime=yes`, `RestrictSUIDSGID=yes`, `RemoveIPC=yes`, `KeyringMode=private` | No writable and executable memory, no new namespaces, no realtime scheduling, no setuid files. |
| `UMask=0077` | Anything Dhow writes is private to it. |
| `LoadCredential=`, `EnvironmentFile=/etc/dhow/dhow.env` | Secrets come from `$CREDENTIALS_DIRECTORY`, settings from the env file, both read by systemd as root. |
| `Restart=on-failure`, `RestartPreventExitStatus=78` | systemd restarts Dhow after a crash, but not after a clean stop, and not when Dhow refuses a database newer than itself (code `78`, see [Upgrades and rolling back](upgrades.md)). |

The unit orders itself after `postgresql.service` and `postgresql-18.service` when they exist, so a local database is up first.

## Upgrade

1. Back up, see below.
2. Download and verify the new release as in step 1.
3. Replace the binary and restart:

   ```sh
   install -o root -g root -m 0755 dhow-server /usr/local/bin/dhow-server
   cmp -s dhow.service /etc/systemd/system/dhow.service || { install -m 0644 dhow.service /etc/systemd/system/; systemctl daemon-reload; }
   systemctl restart dhow
   dhow-server healthcheck
   ```

Migrations run when Dhow starts, under a lock, so only one process migrates at a time. If a migration fails Dhow does not start and the old schema stays. To try the migrations before you upgrade, and to go back to the previous release, see [Upgrades and rolling back](upgrades.md). Going back with the binary alone is enough in most cases; you need the backup when a migration raised the database floor.

## Back up and restore

```sh
runuser -u postgres -- pg_dump -d dhow --format=custom > /root/dhow-$(date +%F).dump
tar -C /etc -czf /root/dhow-etc-$(date +%F).tar.gz dhow
chmod 600 /root/dhow-*
```

Copy both files somewhere other than this host. To restore, stop Dhow, restore the database with `pg_restore --clean --if-exists --no-owner -d dhow` as a superuser, put `/etc/dhow` back with the same secret key, and start Dhow again.

## Uninstall

```sh
systemctl disable --now dhow
rm /etc/systemd/system/dhow.service /usr/local/bin/dhow-server /etc/dhow/dhow.env
systemctl daemon-reload
```

This keeps the secrets in `/etc/dhow/credentials`, `/var/lib/dhow` and the database. To delete them as well, which cannot be undone, also run `rm -r /etc/dhow /var/lib/dhow`, `userdel dhow` and, for a local database, `runuser -u postgres -- psql -c 'DROP DATABASE dhow WITH (FORCE)' -c 'DROP ROLE dhow'`.
