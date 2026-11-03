[العربية](../development.md)

# Develop Dhow on your machine

This page gets you from a fresh clone to a running server, the web app with live reload, and the full test suite. There are two ways in:

- **[Docker only](#quickest-start-docker-only):** one command, and nothing to install but Git and Docker. Start here.
- **[Native tools](#the-setup-script):** Go, Node.js, pnpm and PostgreSQL on your machine (through WSL2 on Windows), for `make check`, the runner isolation suites and your editor's Go and TypeScript tooling.

To try the released image instead, see [Run Dhow locally](install/local.md#the-compose-stack).

> [!WARNING]
> Everything here runs with `DHOW_DEV=true`: plain HTTP and a database connection without TLS. It is for your own machine only. Never set it on a server or anything other people can reach.

## Quickest start: Docker only

All you need is Git and Docker: [Docker Desktop](https://www.docker.com/products/docker-desktop/) on Windows and macOS, or Docker Engine with the Compose plugin on Linux. Go, Node.js, pnpm and PostgreSQL run in containers, so there is nothing else to install, and Windows needs no WSL setup of your own. In a terminal (PowerShell on Windows):

```sh
git clone https://github.com/Khaymah/dhow.git
cd dhow
docker compose -f deploy/dev/compose.yaml up
```

The first start downloads the images (about 1 GB with the runner's) and the Go and web dependencies, and builds the runner image, so it takes a few minutes. Later starts take seconds. When everything is up, the log shows a box like this:

```text
==================================================================
  Dhow is ready. Open this link within one hour to create the owner:
  http://localhost:5173/setup#...
==================================================================
```

Open that link in Chrome or Firefox to create the owner account. From then on, use `http://localhost:5173`.

- **Go:** save any file under `apps/server`, `internal` or `packages/proto/gen/go`, and the server rebuilds and restarts within a few seconds. If the build fails, the error appears in the log, the old server keeps running, and the next save tries again.
- **Web:** save any file under `apps/web` and the browser updates in place.
- **Runner:** save any file under `apps/runner` or `internal`, and the runner rebuilds and restarts the same way, at most 3 times in 10 minutes: the server disables a runner that opens more than 3 new sessions in 10 minutes, because that looks like a copied identity. A later save waits, and the log says when the runner restarts. To run jobs, [register it](#run-jobs-in-the-stack) once.

| Service | What it does | On your machine |
|---|---|---|
| `db` | PostgreSQL 18 with the databases `dhow` and `dhow_test` | `127.0.0.1:55432` |
| `init` | Writes `.env.dev` with a new secret key the first time, then exits | |
| `server` | Builds and runs the server from your clone, and watches the Go files (it checks every second, which works on Windows and macOS too) | `http://localhost:8080` (the API only) |
| `web` | Vite with live reload, forwarding `/api` to `server` | `http://localhost:5173` |
| `runner` | Builds and runs the runner from your clone, as the unprivileged `dhow-runner` user with real rootless Podman jobs, in a privileged container for development only (see [below](#run-jobs-in-the-stack)) | |
| `fake-runner` | Only with `--profile fake-runner`: the runner with the fake driver, which runs no process, in an ordinary container | |

Everyday commands, from the root of the clone:

| Command | What it does |
|---|---|
| `docker compose -f deploy/dev/compose.yaml up -d` | Starts in the background |
| `docker compose -f deploy/dev/compose.yaml logs -f server` | Follows the server log, including the setup link |
| `docker compose -f deploy/dev/compose.yaml restart server` | Restarts the server, which prints a new setup link while there is no owner yet |
| `docker compose -f deploy/dev/compose.yaml restart web` | Reinstalls the web dependencies after `package.json` or the lockfile changed (a `git pull`, for example) |
| `docker compose -f deploy/dev/compose.yaml exec server go test ./apps/server/...` | Runs Go tests against `dhow_test` |
| `docker compose -f deploy/dev/compose.yaml exec web pnpm test` | Runs the web tests |
| `docker compose -f deploy/dev/compose.yaml exec runner dhow-dev-register` | Registers the runner with a token you paste |
| `docker compose -f deploy/dev/compose.yaml logs -f runner` | Follows the runner log |
| `docker compose -f deploy/dev/compose.yaml down` | Stops and removes the containers, keeping the database |
| `docker compose -f deploy/dev/compose.yaml down --volumes` | Also deletes the database, the dependency caches and the runner state |

`Ctrl+C` stops a stack started without `-d`.

**`.env.dev`.** `init` writes it once with mode 600, in the same form as `make dev-env`, so the [native path](#quick-start) can use it too, and it never touches an existing one. The stack takes the secret key and any setting you add yourself (SMTP, for example) from it. The addresses, `DHOW_DEV=true` and the database URLs come from the stack itself. Keep the file: stored secrets need the same key at every start. Delete it only together with the database (`down --volumes`).

**Settings.** Set these in your shell before `up`, or in `deploy/dev/.env` (Compose reads it from there, and Git ignores it):

| Variable | Default | Use |
|---|---|---|
| `DHOW_DEV_WEB_PORT` | `5173` | The web app, and the address the setup link uses |
| `DHOW_DEV_HTTP_PORT` | `8080` | The server, for `curl` and API clients |
| `DHOW_DEV_DB_PORT` | `55432` | PostgreSQL, for `psql` or the native tools. It is not 5432, so a PostgreSQL already installed on the machine does not block it. `.env.dev` keeps the port of the first start |
| `DHOW_DEV_POLL_SECONDS` | `1` | How often the server and the runner check the Go files |
| `DHOW_DEV_RUNNER_MAX_MEMORY`, `DHOW_DEV_RUNNER_MAX_DISK` | `2GiB`, `4GiB` | The most memory and disk one job may ask for (`DHOW_RUNNER_MAX_MEMORY` and `DHOW_RUNNER_MAX_DISK`). Lower them on a small machine, and ask for no more in the job's `Resources` |
| `DHOW_DEV_POSTGRES_IMAGE`, `DHOW_DEV_GO_IMAGE`, `DHOW_DEV_NODE_IMAGE` | `postgres:18`, `golang:1.27.3-bookworm`, `node:26-bookworm-slim` | Another registry or mirror, such as `mirror.gcr.io/library/postgres:18` |

On Linux and macOS, `DHOW_DEV_DB_PORT=55433 docker compose -f deploy/dev/compose.yaml up`. In PowerShell, `$env:DHOW_DEV_DB_PORT = "55433"` first.

### Run jobs in the stack

The `runner` service is a real runner: it runs each job in rootless Podman, with the same checks as a production runner, on Windows and macOS too. It waits until you register it:

1. In the app, open **Runners** and create a registration token. Copy it.
2. From the root of the clone, run this and paste the token when it asks:

   ```sh
   docker compose -f deploy/dev/compose.yaml exec runner dhow-dev-register
   ```

   Paste the token at the prompt, which does not show it. It never goes on the command line or into a file. If you do save it in a file, pass it with `docker compose -f deploy/dev/compose.yaml exec -T runner dhow-dev-register < token.txt`, then delete the file.
3. Within a few seconds the runner shows as **online** on the Runners page. Jobs that need no checkout, such as those from `POST /api/v1/organizations/{id}/jobs`, now run on it. Watch them with `docker compose -f deploy/dev/compose.yaml logs -f runner`.

> [!WARNING]
> The `runner` container is **privileged**, because rootless Podman inside it needs to make user namespaces, mounts and cgroups. That is for development on your own machine only. A production runner never runs this way: it runs under the systemd unit, as [Install a runner](runner/install.md) describes.
>
> - **What a job escape reaches.** The container has none of the unit's other hardening: no `ProtectSystem`, `InaccessiblePaths`, `ProtectProc`, `SystemCallFilter` or `RestrictAddressFamilies`, and no Docker seccomp or AppArmor profile. A job that escapes its sandbox to the `dhow-runner` user is one local privilege escalation away from root in a privileged container, which is root on the Docker VM (Docker Desktop) or on your machine (Docker Engine on Linux).
> - **Untrusted code.** Running a branch of Dhow you do not trust with this stack gives that branch root in a privileged container, because the container builds and runs the runner from your clone.
> - **When you do not need real jobs**, use the [fake runner](#the-fake-runner) instead: it runs in an ordinary container.
>
> The image closes the cheap gaps: only `newuidmap` and `newgidmap` keep their setuid bit (Podman needs them), so `su`, `passwd`, `chsh`, `chfn`, `newgrp` and `gpasswd` give no root, and `mount` and `umount` are for root only, as the unit makes them unreachable.

How it works:

- **The user.** The image adds a `dhow-runner` user with 1,048,576 sub-ids in `/etc/subuid` and `/etc/subgid`, enough for 16 jobs at once. A small script starts as root, prepares the cgroup, then runs the runner as `dhow-runner` with the same capability bounding set as the systemd unit.
- **Root's files.** The scripts root runs are copied into the image when it is built, and your clone is mounted read-only at `/src`, so nothing inside the container can change them. Root builds the runner from `/src` into `/opt/dhow-dev`, and keeps its own notes in `/var/lib/dhow-dev`, which only root can enter. It never opens a file in a folder `dhow-runner` can write to: it checks for the identity as `dhow-runner`.
- **The cgroup.** The container has its own cgroup namespace (`cgroup: private`). The script moves its own processes into a leaf cgroup, turns on the `cpu`, `io`, `memory` and `pids` controllers, and gives the runner a cgroup of its own, owned by `dhow-runner`, with swap off. That is what `Delegate=yes` and `MemorySwapMax=0` give the runner under systemd, so the runner's checks pass as they are and it enforces every job limit.
- **The server address.** `--dev` allows plain `http://` to a loopback address only. Inside the container, a forwarder on `127.0.0.1:8080` passes each connection to `server:8080`, so the runner registers with `--dev --url http://127.0.0.1:8080` and `--dev` stays limited to loopback.
- **The state.** The identity and the image store live in the `runner-state` volume, so the runner stays registered across `down` and `up`.

#### The fake runner

On a machine that cannot run the Podman runner, or when you do not need real jobs, start the fake one instead. It is built with the `dhowtest` tag and the fake driver, runs no process (every step prints one line and succeeds), and needs no privileged container:

```sh
docker compose -f deploy/dev/compose.yaml --profile fake-runner up --scale runner=0
docker compose -f deploy/dev/compose.yaml exec fake-runner dhow-dev-register
```

`--scale runner=0` keeps the Podman runner stopped. The two use separate volumes (`runner-state` and `fake-runner-state`), so each is a runner of its own with its own registration, and one never takes over the other's identity.

#### Reset the runner

Remove its container and its volume, then start it again and register it with a new token. Delete the old runner on the Runners page too.

```sh
docker compose -f deploy/dev/compose.yaml rm --stop --force runner
docker volume rm dhow-dev_runner-state dhow-dev_runner-dev
docker compose -f deploy/dev/compose.yaml up -d runner
```

For the fake runner, use `fake-runner`, `dhow-dev_fake-runner-state` and `dhow-dev_fake-runner-dev`. The image holds the scripts under `deploy/dev/runner`, so after a change there, rebuild it with `docker compose -f deploy/dev/compose.yaml build runner`. Go changes need no rebuild.

### When the runner does not start

Read its log first: `docker compose -f deploy/dev/compose.yaml logs runner`.

| Symptom | Cause and fix |
|---|---|
| `Docker gives this container cgroup v1` | Docker runs with cgroup v1. Docker Desktop uses cgroup v2 on macOS and with the WSL2 backend on Windows: in Docker Desktop, open Settings, then General, and turn on "Use the WSL 2 based engine". On Linux, every current distribution boots with cgroup v2. Otherwise use the fake runner. |
| `did not give this container the cgroup controllers` | The Docker engine does not pass `cpu`, `io`, `memory` or `pids` to containers. Update Docker Desktop, or on Linux run Docker with the systemd cgroup driver. |
| `AppArmor restrict unprivileged user namespaces` | The host is Ubuntu 24.04 or newer. Run `sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0` on the host, then `docker compose -f deploy/dev/compose.yaml restart runner`. |
| `is more than this machine has` for `DHOW_RUNNER_MAX_MEMORY` | A job may use 2 GiB of memory and a 4 GiB disk in memory, so the runner needs 6 GiB. Give Docker Desktop more memory (Settings, then Resources; with WSL2, `memory=` in `%UserProfile%\.wslconfig`), or lower `DHOW_DEV_RUNNER_MAX_MEMORY` and `DHOW_DEV_RUNNER_MAX_DISK`. |
| The container stops at once with a message about privileged containers | Docker Desktop's Enhanced Container Isolation blocks privileged containers. Turn it off for this project, or use the fake runner. |
| `invalid internal status, try resetting the pause process with "podman system migrate"` | The container stopped while Podman had state in it, and an image from before this was fixed does not clear it. Rebuild the image once with `docker compose -f deploy/dev/compose.yaml build runner`, then run `docker compose -f deploy/dev/compose.yaml up -d runner`. The runner now clears that state at every container start, so a restart needs no cleanup. |
| `this runner is already registered` | Reset it as above. |
| The runner shows `RUNNER_DISABLED` | Someone disabled it on the Runners page, or the server did after more than 3 new sessions in 10 minutes. Enable it there, or reset it. |

### When something goes wrong

| Symptom | Cause and fix |
|---|---|
| `port is already allocated`, or `ports are not available` on Windows | Another program uses 55432, 5173 or 8080. `docker ps` shows whether an old container holds it, such as `dhow-dev-postgres` from `make dev-db`; remove it with `docker rm -f dhow-dev-postgres`. Otherwise pick another port with the variables above. When the database does not start, the `web` log shows `ENOTFOUND server`, because the server waits for it. |
| No setup box in the log | The owner already exists, so there is nothing to set up: sign in on `http://localhost:5173`. Otherwise run `docker compose -f deploy/dev/compose.yaml logs server` and look for the box or for `setup_url`. |
| `CSRF_FAILED` at setup or sign-in | Open the app on `http://localhost:5173` (or your `DHOW_DEV_WEB_PORT`), not on port 8080 and not on `127.0.0.1`. |
| The web app reports a missing package after a `git pull` | Run `docker compose -f deploy/dev/compose.yaml restart web`. |
| Builds and reloads are slow on Windows | Files under `C:\` reach the containers through a slower bridge. For more speed, clone inside WSL2 and run the same command from a WSL2 shell, with Docker Desktop's WSL integration turned on (Settings, then Resources, then WSL integration). |
| On Linux, `apps/web/node_modules` is an empty folder owned by root | Docker created it as the mount point for the dependencies volume. Before you switch to the native path, remove it with `sudo rm -rf apps/web/node_modules`. |

## The setup script

One script prepares the whole machine. It installs what is missing, asks only for the settings that are yours to choose, and generates every secret itself.

**Linux and macOS:**

```sh
git clone https://github.com/Khaymah/dhow.git
cd dhow
make dev-setup
```

**Windows**, in PowerShell from a clone or a downloaded copy of `deploy/dev/setup.ps1`:

```powershell
powershell -ExecutionPolicy Bypass -File deploy\dev\setup.ps1
```

On Windows, the script:

1. Installs WSL2 with Ubuntu 24.04 if it is missing. Create your Linux user when it opens, then run the script again.
2. Clones the repository into `~/src/dhow` inside it. If the clone is already there, the script updates it with `git pull --ff-only` instead. When that fails because of local changes or another branch, it warns and carries on with the clone as it is.
3. Runs the Linux script there. That script ignores the Windows entries WSL adds to `PATH`, so a Node.js installed on Windows does not get in the way.

For a PostgreSQL container, turn on Docker Desktop's WSL integration for Ubuntu 24.04: Settings, then Resources, then WSL integration.

`make` and the tools live inside Ubuntu, not in PowerShell. Run them from a shell there (`wsl -d Ubuntu-24.04 --cd ~`, then `cd src/dhow`), or straight from PowerShell:

```powershell
wsl -d Ubuntu-24.04 --cd ~ -- bash -lc 'cd src/dhow && make dev'
```

`bash -l` reads `~/.profile`, where the script put Go and Node.js on `PATH`.

What the script does, asking before anything that needs `sudo` or changes your system:

| Step | What happens |
|---|---|
| Tools | Checks Git, Make, curl, Python 3 and `libatomic` (which Node.js needs), and installs them with `apt` or `dnf` (on macOS, the Xcode Command Line Tools). Installs Go and Node.js LTS when missing or too old: with Homebrew on macOS, otherwise from go.dev and nodejs.org into `~/.local/share/dhow-dev`, each archive checked against the checksum its site publishes. Then pnpm at the version `apps/web/package.json` names. |
| PostgreSQL 18 | `container` runs it in Docker or Podman, `native` installs it from the PostgreSQL project's repository (Debian, Ubuntu, Fedora, RHEL family) or Homebrew (macOS), and `url` uses a database you already have. It refuses anything older than 18. |
| Questions | The server port (8080), the PostgreSQL port (5432), and whether to send email through an SMTP server (host, port, TLS mode, user, password typed hidden, sender). |
| Generated | `DHOW_SECRET_KEY` (32 random bytes) and a random password for the `dhow` database user. |
| Written | `.env.dev`, mode 600, ignored by Git. Run the script again to change a setting: it keeps the secret key and the database password unless you ask for new ones. |
| Last | Builds the web app, offers to add the new tools to `PATH` in your shell profile, and prints the next commands. |

`deploy/dev/test/fresh.sh` runs the whole script on a bare container image as a new user, with Windows shims for `node`, `npm` and `corepack` on `PATH` like WSL has. CI runs it on Ubuntu 24.04, Ubuntu 22.04, Debian 12 and Fedora:

```bash
docker run --rm -v "$PWD":/repo:ro ubuntu:24.04 /repo/deploy/dev/test/fresh.sh --yes --db=url --db-url=postgres://u:p@127.0.0.1/dhow --test-db-url=postgres://u:p@127.0.0.1/dhow_test
```

Options: `--yes` takes every default without asking, `--db=container|native|url` picks the database, `--db-url=` and `--test-db-url=` give one, `--port=` and `--db-port=` set the ports, `--skip-web` skips the web build, and `--check` only reports which tools are there. With `make`, pass them as `make dev-setup SETUP_ARGS="--db=native"`. The [quick start](#quick-start) below does the same steps by hand.

## What you need

| Tool | Why | Notes |
|---|---|---|
| Git | The repository | |
| Go | Server, runner, tests | Any recent Go. The `go` command downloads the exact version pinned in `go.mod` by itself. |
| Node.js LTS with pnpm | The web app (`apps/web`) | Run `corepack enable`, then `corepack prepare pnpm@10.28.0 --activate` (the version in `apps/web/package.json`). |
| **PostgreSQL 18** | The server and the database tests | Exactly 18 or newer: the migrations use the built-in `uuidv7()`. Docker or Podman, or a native install, see [below](#postgresql-18). |
| Python 3 | `make check` runs the tests of the Claude Code hooks | Any Python 3. |
| GNU Make and a POSIX shell | The `make` targets | Already there on Linux and macOS, and in WSL2. |

**Windows:** the [setup script](#the-setup-script) prepares WSL2 for you. By hand: work inside WSL2 with Ubuntu 24.04 or newer, and clone the repository into the Linux file system (`~/src/dhow`), not under `/mnt/c`, which is much slower. Everything on this page then works as on Linux, and the browser on Windows reaches `http://localhost:8080` in WSL2.

## Quick start

```sh
make dev-env
make dev-db
make web
make dev
```

1. `make dev-env` writes `.env.dev` once, with `DHOW_DEV=true`, a fresh secret key, `DHOW_PUBLIC_URL=http://localhost:8080` and the two database URLs. Git ignores it. Keep it: stored secrets need the same key at every start. Delete it only together with the database.
2. `make dev-db` starts PostgreSQL 18 in Docker, or Podman if Docker is missing, on `127.0.0.1:5432`, with the databases `dhow` and `dhow_test`. It is safe to run again. Without Docker or Podman, install PostgreSQL natively instead ([below](#postgresql-18)).
3. `make web` builds the web app into the server. Without it the server still runs, but `/` answers 404. For live reload, skip it and see [The web app with live reload](#the-web-app-with-live-reload).
4. `make dev` starts the server from source. It applies the migrations, logs a `setup_url` line such as `http://localhost:8080/setup#...`, and listens on `127.0.0.1:8080`.

Open the setup link in Chrome or Firefox within an hour to create the owner account. Restart the server for a new link. Check the server from another terminal with `curl -s http://localhost:8080/api/v1/health/ready`.

## PostgreSQL 18

Pick one. They all end with the same user, password and databases, so `.env.dev` works with any of them.

### Docker or Podman

`make dev-db` uses `docker` if it is installed and `podman` otherwise. Choose one with `make dev-db DEV_DB_ENGINE=podman`. If port 5432 is taken, pick another port for both the database and the URLs, before the first `make dev-env`:

```sh
make dev-env DEV_DB_PORT=5433
make dev-db DEV_DB_PORT=5433
```

`make dev-db-reset` deletes the container and its data.

### Native install

**Debian and Ubuntu (and WSL2)**, from the PostgreSQL project's own repository, because the distribution may ship an older version:

```sh
sudo apt install -y postgresql-common
sudo /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh -y
sudo apt install -y postgresql-18
sudo service postgresql start
```

**Fedora, RHEL, Rocky and Alma:** add the repository from [postgresql.org/download/linux/redhat](https://www.postgresql.org/download/linux/redhat/), then install and start `postgresql18-server` as that page shows.

**macOS:** `brew install postgresql@18`, then `brew services start postgresql@18`. [Postgres.app](https://postgresapp.com/) with PostgreSQL 18 works too.

Then create the user and the two databases once. On Linux run `sudo -u postgres psql`, on macOS run `psql postgres`, and enter:

```sql
CREATE ROLE dhow LOGIN PASSWORD 'dhow-dev-only';
CREATE DATABASE dhow OWNER dhow;
CREATE DATABASE dhow_test OWNER dhow;
```

Check it with `psql 'postgres://dhow:dhow-dev-only@127.0.0.1:5432/dhow' -c 'SELECT version()'`. It must say 18 or newer.

### A hosted database

Any PostgreSQL 18 works if you put its URL in `.env.dev` (with `sslmode=require`). It is slower, needs the internet, and the tests create and drop a schema per test, so use a database of its own for `DHOW_TEST_DATABASE_URL`.

## The web app with live reload

Run two terminals:

```sh
make dev-vite
```

```sh
pnpm -C apps/web install
pnpm -C apps/web dev
```

Then open `http://localhost:5173`. Vite serves the web app and forwards `/api` to the server on port 8080. `make dev-vite` starts the server with `DHOW_PUBLIC_URL=http://localhost:5173`, because Dhow compares the browser's `Origin` with it character for character. Open the setup link on port 5173 too.

## Tests and checks

```sh
make dev-check
```

`make dev-check` runs `make check` (lint, workflow lint, the runner protocol checks, every Go test with the race detector, the vulnerability scan, the hooks tests and the docs check) with the test database from `.env.dev`.

> [!IMPORTANT]
> Use `make dev-check`, not a bare `make check`. Without `DHOW_TEST_DATABASE_URL` the database tests **skip themselves**, so `make check` can pass on your machine while most server tests never ran. CI always sets it.

For one package, load the file first:

```sh
set -a; . ./.env.dev; set +a
go test -race -count=1 ./apps/server/internal/Modules/Session/...
```

For changes to `apps/web`, also run what its CI job runs:

```sh
pnpm -C apps/web lint
pnpm -C apps/web typecheck
pnpm -C apps/web test
pnpm -C apps/web build
pnpm -C apps/web size
```

`make gen` regenerates the code from `packages/openapi/openapi.yaml` and the SQL queries, and `pnpm -C apps/web gen:api` regenerates the web API types. Never edit generated files by hand.

## The runner

What you can do depends on your system:

| | Linux | macOS and Windows |
|---|---|---|
| Runner tests with the fake driver (`make test-runner`) | ✅ | ✅ (Windows in WSL2) |
| Register against your local server | ✅ | ✅ |
| Run real jobs in rootless Podman in the [Docker stack](#run-jobs-in-the-stack) | ✅ | ✅ with Docker Desktop |
| Run the runner natively, and `make runner-isolation` | ✅ with the requirements below | In a Linux VM only |

- **Tests.** `make test-runner` builds the runner with the `dhowtest` tag, which adds a fake sandbox driver that replays recorded output and runs no process. Most runner work is covered there.
- **Registration.** Create a registration token as in [Register a runner](runner/register.md), then register with `--dev`, which allows `http://` on `localhost` only, and a state directory of your own:

  ```sh
  DHOW_RUNNER_STATE_DIR="$HOME/.local/state/dhow-runner-dev" \
    go run ./apps/runner/cmd/dhow-runner register --dev --url http://localhost:8080 --token-file -
  ```

- **The sandbox.** Jobs run in rootless Podman. It needs Linux with Podman 5.4 or newer, `crun`, `uidmap`, cgroup v2 with the `cpu`, `io`, `memory` and `pids` controllers delegated to the runner (run it under systemd with `Delegate=yes`), at least 65536 sub-ids per job in `/etc/subuid` and `/etc/subgid`, and user namespaces (on Ubuntu 24.04 and newer, `kernel.apparmor_restrict_unprivileged_userns=0`). See [Install a runner](runner/install.md). On macOS and Windows, the [Docker stack](#run-jobs-in-the-stack) runs real jobs with nothing else to install. To run the runner natively there, use a Linux VM with systemd, such as [Lima](https://lima-vm.io/) or Multipass on macOS, or WSL2 with systemd turned on, and run the runner there against the server on your machine.
- **The isolation suites.** `make runner-isolation` builds the runner and an escape probe, runs them in real rootless containers, and fails if any escape works or a limit is not enforced. It needs the sandbox requirements above and `busybox-static`. CI runs it on `ubuntu-26.04` as a dedicated user under `systemd-run -p Delegate=yes` (see the `isolation` job in `.github/workflows/ci.yml`); copy that setup on your machine with `make runner-isolation ISOLATION_RUN="sudo systemd-run --uid=$USER --pipe --wait --collect -p Delegate=yes --"`.

## Common problems

| Symptom | Cause and fix |
|---|---|
| The migrations fail at the first start | PostgreSQL is older than 18 (no `uuidv7()`). Check `SELECT version()`. |
| `CSRF_FAILED` at setup or sign-in | The address in the browser is not exactly `DHOW_PUBLIC_URL`: `localhost` and `127.0.0.1` differ, and so do ports 8080 and 5173. |
| Sign-in seems to do nothing from another device | The session cookie is `Secure`, and only `localhost` is a secure context over HTTP. Use the machine itself, or HTTPS. See [Run Dhow locally](install/local.md#browsers-and-httplocalhost). |
| `make dev-db` says the port is in use | Another PostgreSQL listens on 5432. Stop it, or use `DEV_DB_PORT` as [above](#docker-or-podman). |
| `/` answers 404 | The web app is not built into the server. Run `make web`, or use the [live reload](#the-web-app-with-live-reload) setup. |
| The setup link no longer works | It is valid for one hour. Restart the server to get a new one. |
| Stored secrets cannot be read after a restart | `DHOW_SECRET_KEY` changed. Keep `.env.dev`, or start over with an empty database. |

## Start over

```sh
make dev-db-reset
rm .env.dev
```

Then follow the [quick start](#quick-start) again.
