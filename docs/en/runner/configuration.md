[العربية](../../runner/configuration.md)

# Runner configuration

The runner reads its settings from the environment, usually `/etc/dhow-runner/runner.env` through systemd's `EnvironmentFile=`. Every value is checked when the runner starts, and any problem stops it with a message that names the variable (fail closed). Nothing here is a secret: the runner's keys live in its state directory (encrypted with `systemd-creds` when `dhow-runner install` set it up, see [the encrypted identity](keys.md#the-encrypted-identity)), not in the environment. `dhow-runner install` writes this file from its answers and keeps the lines you add by hand.

| Variable | Default | Example | Meaning |
|---|---|---|---|
| `DHOW_RUNNER_URL` | the address used at registration | `https://dhow.example.com` | The server address. `https://` with a host name and optional port only, no path. Plain `http://` is allowed only with `--dev` and only for `localhost`, `127.0.0.1` or `[::1]`. |
| `DHOW_RUNNER_CA_FILE` | system roots | `/etc/dhow-runner/ca.pem` | A PEM bundle that replaces the system roots, so the runner trusts that CA and nothing else. It must hold at least one certificate. |
| `DHOW_RUNNER_SERVER_ORIGIN` | the origin of `DHOW_RUNNER_URL` | `https://dhow.example.com` | The server's public origin (`DHOW_PUBLIC_URL`), which the runner's signatures name. Set it at registration when the runner talks to a separate runner listener. It is pinned in the identity, and later runs use the pinned value. |
| `DHOW_RUNNER_STATE_DIR` | `/var/lib/dhow-runner` | | An absolute path to the state directory. It must belong to the runner user and not be writable by others, and its `identity`, `identity.pending` and `journal` files must be regular files that only the runner user can read. It also holds `lock`, which `run` and `rotate-keys` hold while they work. With the Podman driver, every directory above it must let others enter (mode `o+x`), because crun reaches the state directory from the job's user namespace. A home directory with mode `0750` does not; the runner refuses to start there and names the directory to fix. |
| `DHOW_RUNNER_DRIVER` | `podman` | `kubernetes` | The sandbox driver: `podman` (M3.6) or `kubernetes` (M3.11). Test builds also accept `fake`. A driver missing from the build stops the runner. |
| `DHOW_RUNNER_CAPACITY` | `1` | `4` | How many jobs run at once, from 1 to 16. |
| `DHOW_RUNNER_MAX_CPU` | every CPU of the host | `2`, `0.5` | The most CPU one job may ask for, in CPUs (0.1 to 64). |
| `DHOW_RUNNER_MAX_MEMORY` | `2GiB` | `4GiB` | The most memory one job may ask for, from 64 MiB to 64 GiB. Sizes take `KiB`, `MiB`, `GiB` or `TiB` (or plain bytes). |
| `DHOW_RUNNER_MAX_DISK` | `4GiB` | `10GiB` | The most `/workspace` space one job may ask for, from 64 MiB to 100 GiB. |
| `DHOW_RUNNER_MAX_PIDS` | `1024` | `4096` | The most processes one job may ask for, from 32 to 32768. |
| `DHOW_RUNNER_MAX_TIMEOUT` | `6h` | `2h` | The longest `TimeoutSeconds` a job may ask for, from `1m` to `24h`. |
| `DHOW_RUNNER_DISK_MODE` | `tmpfs` | `tmpfs` | `tmpfs` puts `/workspace` in memory with a hard limit, so memory plus disk must fit the host's memory. `directory` is refused for now: it has no enforced disk limit yet, so one job could fill the runner's disk ([ADR-0056](../../../Plan/ADR/0056-sandbox-hardening-after-the-m3-6-review.md)). |
| `DHOW_RUNNER_CANCEL_GRACE` | `10s` | `30s` | How long a step has between `SIGTERM` and `SIGKILL` when it is canceled or times out, from `1s` to `60s`. |
| `DHOW_RUNNER_EGRESS_ALLOW_PRIVATE_CIDRS` | none | `10.20.0.0/16,fd00::/8` | Private ranges the egress proxy may reach, comma separated, for an internal registry or mirror. A range written here also opens the runner host's own addresses inside it. Cloud metadata and credential addresses stay denied even when a range covers them: AWS (`169.254.169.254`, `169.254.170.2`, `169.254.170.23`, `fd00:ec2::254`, `fd00:ec2::23`), Azure (`168.63.129.16`), Alibaba (`100.100.100.200`), Oracle (`192.0.0.192`, `fd00:c1::a9fe:a9fe`) and Google (`fd20:ce::254`). A range broader than `/8` for IPv4 or `/7` for IPv6 stops the runner. A job still reaches these hosts only by a name in its `network.egress` list, never by IP. The runner treats its interface addresses as its own; a public address that reaches the host through 1:1 NAT is not known to it, so keep services on that address behind your firewall. |
| `DHOW_RUNNER_ALLOW_UNPINNED_IMAGES` | `false` | `true` | Reserved for the Podman driver (M3.6). Jobs must pin images by digest either way. |
| `DHOW_RUNNER_RUNSC_PATH` | none | `/usr/local/bin/runsc` | An absolute path to an executable `runsc`. When set with the Podman driver, the runner offers the `gvisor` isolation level (M3.8). |
| `DHOW_RUNNER_CHECKOUT_IMAGE` | the digest built into the release | `ghcr.io/khaymah/dhow-git@sha256:…` | The image the checkout runs in. Only a full reference pinned by digest is accepted. Leave it unset unless you mirror the signed `dhow-git` image into your own registry; without a value (a build from source) the runner does not offer checkout, and the server never gives it a job with one. |
| `DHOW_RUNNER_LOG_LEVEL` | `info` | `debug` | `debug`, `info`, `warn` or `error`. |

## Limits and the machine

Every `MAX_*` value is a ceiling for one job, and the runner refuses to start if a ceiling is more than the host has: CPUs from the kernel, processes from `/proc/sys/kernel/pid_max`, memory from `sysinfo`, and free disk under the state directory. With the default `tmpfs` disk mode, memory plus disk counts against memory, because tmpfs pages are charged to the job's memory. The defaults (2 GiB of memory and a 4 GiB tmpfs disk) therefore need a host with at least 6 GiB of memory. On a smaller host, lower `DHOW_RUNNER_MAX_MEMORY` and `DHOW_RUNNER_MAX_DISK`. `dhow-runner install` picks values that fit for you when you set neither ([Memory per job](install.md#memory-per-job)).

The state directory must also have room for one 256 MiB log spill file per job slot (`DHOW_RUNNER_CAPACITY`).

With the Podman driver, every running job takes a range of 65536 user ids from the runner user's `/etc/subuid` and `/etc/subgid`, so they must give at least 65536 × `DHOW_RUNNER_CAPACITY` ids (the 16 × 65536 that `dhow-runner install` sets up covers the largest capacity, 16). A job that finds no free range fails with `SANDBOX_FAILED`.

A job that asks for more than a ceiling fails on this runner with `RESOURCE_REQUEST_EXCEEDS_RUNNER` before it starts, and the server gives it to another runner. The default job request (1 CPU, 2 GiB, 1024 processes, 4 GiB of disk, one hour) fits the default ceilings.

## Command line flags

- `dhow-runner register --url <server> --token-file <path|-> [--name <name>] [--dev]`: see [Register a runner](register.md).
- `dhow-runner run [--dev]`: see [Run a runner](run.md).
- `dhow-runner version`.

`--dev` is a flag on purpose, not a variable, so that a production `runner.env` cannot turn plain HTTP on.
