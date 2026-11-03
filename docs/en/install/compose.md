[العربية](../../install/compose.md)

# Install with Docker Compose

This is the default install method ([ADR-0024](../../../Plan/ADR/0024-install-methods.md)). It starts Dhow and its own PostgreSQL 18 on one host, with secure defaults and no manual secret handling. For other methods and the requirements they share, see [Choose an install method](README.md).

## Requirements

- Docker Engine 24 or newer with the Compose plugin (`docker compose version`).
- `openssl` and `bash` on the host, for `init.sh`.
- A reverse proxy that terminates TLS in front of Dhow. Dhow itself speaks plain HTTP on port 8080 and refuses to start with an `http://` public URL. [Put Dhow behind a reverse proxy](reverse-proxy.md) has tested configurations for nginx, Caddy and Traefik.

## Quick start

```sh
git clone --depth 1 https://github.com/Khaymah/dhow.git
cd dhow/deploy/compose
./init.sh https://dhow.example.com
docker compose up -d
```

To avoid cloning the repository, download `compose.yaml` and `init.sh` from the `deploy/compose` directory of a release tag into an empty directory, then run `chmod +x init.sh`.

`init.sh` writes `.env` next to the Compose file with:

| Variable | Value |
|---|---|
| `POSTGRES_PASSWORD` | 24 random bytes as hex. |
| `DHOW_SECRET_KEY` | 32 random bytes as hex. |
| `DHOW_PUBLIC_URL` | The URL you passed. It defaults to `https://dhow.localhost`, which is only a placeholder for a first look. |

The file has mode `0600`, and `init.sh` refuses to overwrite an existing `.env`. Back up `DHOW_SECRET_KEY` somewhere safe: without it, stored secrets cannot be decrypted, and a new key cannot read old data.

`init.sh` also writes the [keyring](keys.md) to `./secrets/dhow-keyring` with mode `0600` and gives it to the container user `65532`, so its `chown` needs root or `sudo` once. The keyring is not an environment variable: `compose.yaml` mounts it into the container as a secret at `/run/secrets/dhow-keyring`, so `docker inspect` never shows it. With rootless Docker, set `DHOW_CONTAINER_UID` to the user id that `65532` maps to in `/etc/subuid`. Export the keyring after the first start and keep the copy off the host, as in [Backups and export](keys.md#backups-and-export).

`docker compose up -d` waits for PostgreSQL to be healthy, applies the database migrations, and starts Dhow. Check it:

```sh
docker compose ps
curl -s http://127.0.0.1:8080/api/v1/health/ready
```

Dhow is published on the loopback interface only, so it is not reachable from the network until you put a reverse proxy in front of it. Opening `http://127.0.0.1:8080` on the same host shows the web app, but setup and sign-in only work from the `DHOW_PUBLIC_URL` address: the browser sends that address as `Origin`, and the session cookie needs HTTPS.

Follow [Put Dhow behind a reverse proxy](reverse-proxy.md) for nginx, Caddy or Traefik. It covers the headers Dhow needs, `DHOW_TRUSTED_PROXIES` and how to check the result. Traefik can run inside this Compose project as an extra file. Set `DHOW_PUBLIC_URL` in `.env` to the public `https://` address and run `docker compose up -d` again to apply it.

## Create the first owner

A new instance has no users, and public registration is closed. On every start without an owner, Dhow logs a one-time setup link. It is valid for one hour and works once:

```sh
docker compose logs dhow | grep setup_url
```

The token is the part after the `#`. Open the whole link in a browser. The setup page reads the token, removes it from the address bar at once, and asks for your name, email, password and the organization name. When you submit, it creates the owner and the first organization and signs you in. The page works only from this link: opening `/setup` without the token explains where to find it.

The same step is one API call if you prefer a terminal. The `Origin` header must equal `DHOW_PUBLIC_URL`:

```sh
curl -sS -X POST https://dhow.example.com/api/v1/setup \
  -H 'Content-Type: application/json' -H 'Origin: https://dhow.example.com' \
  -d '{"SetupToken":"<token>","OwnerName":"Mona","Email":"mona@example.com","Password":"<12 to 128 characters>","OrganizationName":"Acme","OrganizationSlug":"acme"}'
```

If the hour runs out, or you lose the link, restart Dhow with `docker compose restart dhow` to get a new one. The older link stops working at once. After setup the link is gone and `GET /api/v1/meta` reports `SetupRequired: false`.

Sign-in uses a cookie that lives for at most 7 days and ends after 12 hours without activity (4 hours for owners and admins), so keep your reverse proxy forwarding cookies and the `Origin` header unchanged.

## Configuration

Set these in `.env`. Compose reads the file automatically, and values exported in your shell take precedence. [Configuration](configuration.md) describes every `DHOW_*` variable and its validation. The mail, organization, password, rate-limit and log settings can also be changed by the instance owner in **Server settings**. Leave them empty in `.env` to do that: a value set here wins and shows read-only in the dashboard.

| Variable | Default | Meaning |
|---|---|---|
| `POSTGRES_PASSWORD` | generated | Password of the `dhow` database user. Use URL-safe characters only. |
| `DHOW_SECRET_KEY` | generated | Encrypts secrets at rest. Hex or base64, at least 32 bytes. Placeholder-looking and low-variety keys stop the server from booting. |
| `DHOW_PUBLIC_URL` | generated | The `https://` address users open. |
| `DHOW_PUBLISH` | `127.0.0.1:8080` | Host address and port that Dhow is published on. Use `0.0.0.0:8080` only when something else filters access to the port. |
| `DHOW_TRUSTED_PROXIES` | empty | Comma-separated IPs or CIDRs of reverse proxies whose `X-Forwarded-For` header is trusted. A proxy on the same host reaches the container from the Docker bridge gateway, not from `127.0.0.1`: `docker network inspect dhow_edge --format '{{(index .IPAM.Config 0).Gateway}}'` prints it. See [Trusted proxies](reverse-proxy.md#trusted-proxies). |
| `DHOW_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. |
| `DHOW_BREACHED_PASSWORD_CHECK` | `true` | Rejects new passwords found in known breaches, using the Have I Been Pwned range API. Only the first 5 characters of the password's SHA-1 leave the server, and the check is skipped with a log warning when the service cannot be reached. Set `false` on networks without internet access. |
| `DHOW_IMAGE` | `ghcr.io/khaymah/dhow:latest` | The image to run. Pin it by digest, see [Verify an image](#verify-an-image). |

`DHOW_DEV` is not needed and the Compose file never sets it. The database connection uses TLS (`sslmode=require`) and the public URL is `https://`, so the server's strict checks pass without it. `DHOW_DEV=true` relaxes those checks for local development only. Never set it on a host that other people can reach.

## What the stack hardens

- PostgreSQL sits on an internal Docker network with no route to the outside and no published port. The connection from Dhow uses TLS with a certificate generated at each start. This encrypts the traffic but does not authenticate the server, which is acceptable on a network that only the two containers share.
- Both containers run with a read-only root filesystem, `tmpfs` for temporary files, all capabilities dropped, `no-new-privileges`, and CPU, memory and process limits.
- The Dhow image is built from `distroless/static` and runs as user `65532`. It has no shell and no package manager, so `docker exec dhow sh` fails by design.
- Dhow starts only after PostgreSQL reports healthy, and Compose restarts both containers unless you stop them.

Container environment variables, including the secret key, are visible to anyone who can run `docker inspect`. Treat access to the Docker socket as root access to the instance.

## Upgrade

1. Back up first, see [Back up and restore](#back-up-and-restore).
2. Read the release notes for the new version.
3. Verify the new image, see [Verify an image](#verify-an-image).
4. Set `DHOW_IMAGE` in `.env` to the new tag or digest, then:

```sh
docker compose pull
docker compose up -d
```

Database migrations run automatically when Dhow starts, under a lock, so only one container migrates at a time. If a migration fails the server does not start and the old schema stays in place. To try the migrations before you upgrade, and to go back to an older version, see [Upgrades and rolling back](upgrades.md). Going back with the previous image alone is enough in most cases; you need the backup you took in step 1 when a migration raised the database floor.

## Back up and restore

Back up the database, `.env` and the `./secrets/` directory. The database dump is useless without `DHOW_SECRET_KEY` and the keyring.

```sh
docker compose exec -T postgres pg_dump -U dhow -d dhow --format=custom > dhow-$(date +%F).dump
cp .env dhow-env-$(date +%F).backup
sudo tar -czf dhow-secrets-$(date +%F).tar.gz secrets
chmod 600 dhow-env-*.backup
```

To restore into the running stack, stop Dhow so nothing writes during the restore, empty the schema so the result matches the dump exactly, then load the dump. The `DROP SCHEMA` step deletes everything in the database:

```sh
docker compose stop dhow
docker compose exec -T postgres psql -U dhow -d dhow -c 'DROP SCHEMA public CASCADE' -c 'CREATE SCHEMA public'
docker compose exec -T postgres pg_restore -U dhow -d dhow --no-owner < dhow-2026-01-01.dump
docker compose start dhow
```

To restore on a new host, run `./init.sh`, replace the generated `DHOW_SECRET_KEY` in `.env` with the one from your backup, and replace `./secrets/dhow-keyring` with the keyring from the backup (owner `65532`, mode `0600`). Then start only PostgreSQL with `docker compose up -d postgres`, run the `pg_restore` command, and run `docker compose up -d`. When the keyring does not fit the restored database, Dhow stops with a message that names the version, and never runs on broken data.

## Verify an image

Release images are signed with [cosign](https://docs.sigstore.dev/) keyless signing from the release workflow in this repository. Anyone can check the signature without a key:

```sh
cosign verify ghcr.io/khaymah/dhow:1.0.0 \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github\.com/Khaymah/dhow/\.github/workflows/release\.yml@refs/tags/v'
```

The command fails unless a certificate issued to that workflow, on a `v*` tag of this repository, signed the image. The SPDX SBOM and the SLSA build provenance are attached to the image as signed attestations:

```sh
gh attestation verify oci://ghcr.io/khaymah/dhow:1.0.0 --repo Khaymah/dhow
```

Once an image verifies, pin it by digest so a tag can never be moved under you:

```sh
docker buildx imagetools inspect ghcr.io/khaymah/dhow:1.0.0 | sed -n 's/^Digest: *//p'
```

Put the result in `.env` as `DHOW_IMAGE=ghcr.io/khaymah/dhow@sha256:<digest>`.

## Uninstall

```sh
docker compose down
```

stops and removes the containers and the networks but keeps the database volume. To delete the data as well, which cannot be undone:

```sh
docker compose down --volumes
```

Then remove the directory that holds `.env`, and the image with `docker image rm`.

## Troubleshooting

- **`run ./init.sh to create .env`**: Compose cannot find the required variables. Run `./init.sh` in the same directory as `compose.yaml`.
- **Dhow exits right after starting**: `docker compose logs dhow` prints every invalid setting, for example a public URL that is not `https://`. The server refuses to run with a missing, placeholder or low-variety secret key.
- **Port 8080 is taken**: set `DHOW_PUBLISH=127.0.0.1:9090` in `.env`.
- **Anything about `CSRF_FAILED`, cookies, `sslmode` or the wrong client address**: see [Troubleshooting](troubleshooting.md).
