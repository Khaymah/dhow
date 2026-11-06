[العربية](../../guides/deploy-compose.md)

# Deploy with Docker Compose

The official `dhow-deploy-compose` plugin deploys a Docker Compose project to a remote server over SSH. It uploads the release to its own folder, checks the Compose file before it touches the server, pulls the images, then starts the services and waits for them to become healthy. If a service does not become healthy, or the health check fails, the plugin puts the services back on the images of the previous release.

Every image in the file must be pinned by digest. The plugin does not build images and does not open the Docker port on the network: it runs `docker compose` commands on the server over SSH, and needs no agent there.

A target can be one server or a group of up to 10 servers. On a group the plugin deploys in batches and puts back every server that was switched if one server fails, by the same rules described in [Deploy to a group of servers](deploy-ssh.md#deploy-to-a-group-of-servers).

## How the files look on the server

```text
/srv/shop/
  releases/
    41-3fa9c1d2-1/
      compose.yaml
      compose.dhow.yaml
    42-8be07c55-1/
  shared/
    .env
  current -> releases/42-8be07c55-1
  .dhow/
```

- `compose.yaml` is the project file as it came from the commit. Its name is set in the target (`ComposeFile`) and it can be in a subfolder.
- `compose.dhow.yaml` is written by the plugin. It holds the image of every service by digest, so it is the record of the images that were deployed in this release, and the rollback reads the images of the previous release from it.
- `shared/.env` is optional. If it exists, the plugin passes it to Compose with `--env-file`, and it is the only source of `${...}` variables in the file. Dhow does not write application secrets on the server: put the file there yourself.
- `.dhow/compose-<release>.json` is written by the plugin. It holds the checksum of `compose.yaml` and of `compose.dhow.yaml` at the time of the check, and the plugin compares them before every start.
- `current` and `.dhow` are as in [Deploy to a server over SSH](deploy-ssh.md#how-the-files-look-on-the-server).

The release folder is the project folder. Relative paths in the file, such as `./conf`, are resolved from it.

## Prepare the server

The server needs Docker with the Compose plugin (the `docker compose` command), `sshd` with `sftp`, and the `sha256sum` command. Create an account for deployment and give it the deploy folder:

```bash
sudo adduser --disabled-password --gecos "" dhow-deploy
sudo install -d -o dhow-deploy -g dhow-deploy -m 0755 /srv/shop
```

Put the public key that Dhow generated in `~dhow-deploy/.ssh/authorized_keys` with the `restrict` and `from` options, as in [Deploy to a server over SSH](deploy-ssh.md#prepare-the-server).

A Docker user on the server effectively has root on it. For this reason:

- Run Docker in rootless mode for the deploy account if you can, so the effect of a mistake stays inside that account.
- If you use ordinary Docker, the deploy account in the `docker` group is root in practice. Limit its key with `from` and to one target, and do not use it for anything else.
- The plugin checks the Compose file before it connects to the server, and refuses the options that widen these rights. The rules are in [Compose file rules](#compose-file-rules).

Log in to the registry on the server itself (`docker login`) with the deploy account, if the images are private. Dhow does not send registry credentials to the server.

## The step in the pipeline file

```yaml
deploy:
  needs: [build]
  environment: production
  steps:
    - uses: ghcr.io/khaymah/dhow-deploy-compose@sha256:<digest>
      with:
        artifact: stack
        target: shop
        wait_timeout: 180s
        health_check_url: https://{host}/healthz
```

`<digest>` is the digest of the plugin that is published with every Dhow release. `artifact` is an archive that holds the Compose file. It reaches the plugin under the same conditions as in [Deploy to a server over SSH](deploy-ssh.md#the-step-in-the-pipeline-file): the plugin checks it before any connection and refuses every element that is not safe.

The project name (`ComposeProject`), the file name (`ComposeFile`) and the option that opens the policy (`AllowUnsafeCompose`) come from the target, so a commit cannot change them. The project name has lower case letters, digits and dashes (`-`) only, with no underscore, so that the name of a resource in one project can never match a resource of another project. If you write `name:` at the top of the file it must equal the project name.

## Images by digest

After the `images` input is merged with the file, the image of every service must have the form `name@sha256:...`. Anything else stops the deploy with `COMPOSE_IMAGE_NOT_PINNED` before any connection.

Images can be pinned from the pipeline instead of the file. This helps when an earlier job builds the image and knows the digest:

```yaml
with:
  images: web=ghcr.io/acme/shop-web@sha256:<digest>,worker=ghcr.io/acme/shop-worker@sha256:<digest>
```

- Every item is `service=image`, separated by a comma or a new line.
- The input wins over `image` in the file. If it names a service that is not in the file, the deploy fails with `COMPOSE_IMAGE_UNKNOWN_SERVICE`.
- A service with `build` is refused with `COMPOSE_BUILD_NOT_ALLOWED`. The deploy pulls images, it does not build them.
- Variables in the image name (`${TAG}`) are not accepted, because the digest must be known before the deploy.

## Compose file rules

Because the Compose file comes from a commit and runs with Docker rights, the plugin checks it twice: on the archive before any connection, and then on the file itself as it reached the server, after its checksum is verified. The second check makes sure that what runs is what was checked.

| Code | What it refuses |
|---|---|
| `COMPOSE_UNSAFE_PRIVILEGED` | `privileged: true`, and any `post_start` or `pre_stop` command with `privileged` |
| `COMPOSE_UNSAFE_PID_HOST` | `pid: host` |
| `COMPOSE_UNSAFE_IPC_HOST` | `ipc: host` |
| `COMPOSE_UNSAFE_UTS_HOST` | `uts: host` |
| `COMPOSE_UNSAFE_CGROUP_HOST` | `cgroup: host` |
| `COMPOSE_UNSAFE_USERNS_HOST` | `userns_mode: host` |
| `COMPOSE_UNSAFE_NETWORK_HOST` | `network_mode: host` |
| `COMPOSE_UNSAFE_CAP_ADD` | any `cap_add` |
| `COMPOSE_UNSAFE_DEVICES` | `devices`, `device_cgroup_rules`, `gpus` and device reservations in `deploy.resources` |
| `COMPOSE_UNSAFE_SECURITY_OPT` | any `security_opt` other than `no-new-privileges` |
| `COMPOSE_UNSAFE_DOCKER_SOCKET` | a bind of `docker.sock` at any path, and `use_api_socket` |
| `COMPOSE_UNSAFE_BIND_ROOT` | a bind of `/` |
| `COMPOSE_UNSAFE_BIND_OUTSIDE` | a bind of an absolute path or of `~`; and reading an `env_file` or a `secrets` or `configs` file from anywhere other than the release folder and `shared/.env` |
| `COMPOSE_UNSAFE_BIND_WRITABLE` | a bind of a folder of the release with write access. Binds are allowed only read only (`:ro`) |
| `COMPOSE_UNSAFE_BIND_SHARED_DATA` | a bind of a folder under `shared` |
| `COMPOSE_UNSAFE_BIND_STATE` | a bind of `.dhow` |
| `COMPOSE_UNSAFE_VOLUME_OPTS` | `driver_opts` in a volume |
| `COMPOSE_UNSAFE_NETWORK_DRIVER` and `_NETWORK_OPTS` | a top-level network with a `driver` other than `bridge`, or with `driver_opts` or `ipam` |
| `COMPOSE_UNSAFE_LOGGING` | `logging` with a driver other than `json-file` and `local`, or with an option other than `max-size`, `max-file` and `compress` |
| `COMPOSE_BIND_SOURCE` | a bind source or a file that is read is not on the server before the start, or is a symbolic link, or is not an ordinary folder or file |
| `COMPOSE_UNSAFE_BIND_SYMLINK` | a bind of a path that passes through a symbolic link that came from the archive |
| `COMPOSE_UNSAFE_INTERPOLATION` | a `${...}` variable anywhere other than `environment`, `command`, `entrypoint`, `labels` and `healthcheck.test` |
| `COMPOSE_UNSAFE_INCLUDE` | `include` at the top of the file |
| `COMPOSE_UNSAFE_EXTENDS` | `extends` in a service |
| `COMPOSE_UNSAFE_PROVIDER` | `provider` in a service |
| `COMPOSE_UNSAFE_UNKNOWN_KEY` | a key that is not in the list of known keys, in a service, at the top of the file or in `deploy`. New Compose keys are refused until they are added to the list |
| `COMPOSE_UNSAFE_VOLUMES_FROM` | `volumes_from` |
| `COMPOSE_UNSAFE_NAMESPACE_SHARED` | `pid`, `network_mode` or `ipc` in the `container:` or `service:` form, and `ipc: shareable` |
| `COMPOSE_UNSAFE_EXTERNAL` | a top-level network or volume that is `external` or has a `name` |
| `COMPOSE_UNSAFE_VOLUME_DRIVER` | a volume `driver` other than `local` |
| `COMPOSE_UNSAFE_SECRET_SOURCE` | `secrets` or `configs` with a source other than `file`, such as `environment` |
| `COMPOSE_UNSAFE_BIND_RELEASE` | a bind of the release folder itself (`.`), of `releases`, of a whole release folder, or of a relative path that leaves the release folder (`..`) |
| `COMPOSE_UNSAFE_BIND_COMPOSE` | a bind of the Compose file, of `compose.dhow.yaml`, or of a folder that holds them |
| `COMPOSE_UNSAFE_BIND_SHARED` | a bind of `shared` itself or of `shared/.env` |
| `COMPOSE_UNSAFE_CGROUP_PARENT`, `_RUNTIME`, `_SYSCTLS`, `_GROUP_ADD`, `_EXTRA_HOSTS`, `_EXTERNAL_LINKS` and `_MODELS` | the keys `cgroup_parent`, `runtime`, `sysctls`, `group_add`, `extra_hosts`, `external_links` and `models` |
| `COMPOSE_FILE_CHANGED` | the Compose file or `compose.dhow.yaml` on the server does not match what was checked, or the record of their checksums is missing |
| `COMPOSE_ENV_FORBIDDEN` | a variable that starts with `COMPOSE_`, `DOCKER_` or `LD_` in `shared/.env`, or a line in it that the plugin cannot read |
| `COMPOSE_UNSAFE_NETWORK_MODE` | a `network_mode` other than `none` (except `host`, which follows its own rule). To attach a service to a network use `networks` |
| `COMPOSE_UNSAFE_PROJECT_NAME` | a `name:` at the top of the file that is not the project name of the target |
| `COMPOSE_UNSAFE_LABEL` | a label that starts with `com.docker.` |
| `COMPOSE_UNSAFE_BIND_OPTIONS` | in the long bind form: `bind.propagation`, `bind.selinux`, `bind.recursive` or `bind.create_host_path: true` |
| `COMPOSE_RESOLVED_UNSAFE` | the file after Compose computed the variables and merged it disagrees with what was checked: a project, an image, a bind, a security option or a volume |
| `COMPOSE_INVALID` | a file that is not valid YAML, or uses a YAML tag that is not a core tag such as `!reset` and `!override`, has more than one document, has a repeated key, has no services, is larger than 256 KiB, is not in the archive, has text where the value must be `true` or `false` (such as `read_only: "true"`), or has a mount mode in `volumes` that is not known |

- Allowed mounts: a named volume, an unnamed volume such as `/data`, `tmpfs`, or a subfolder inside the release folder, read only, such as `./conf:/conf:ro`. A bind of the release folder itself, of the Compose files, of anything under `shared`, of another release, or of an absolute path is not allowed.
- `env_file` and the files of `secrets` and `configs` are read from inside the release folder or from exactly `shared/.env`, and they are ordinary files, not links.
- In named volumes: `driver` is `local` only, with no `driver_opts`.
- Top-level networks use the default driver (`bridge`) with no `driver_opts` or `ipam`, and `logging` uses `json-file` or `local` with the options `max-size`, `max-file` and `compress` only. `${...}` variables in any `volumes` entry, and in `network_mode`, `pid`, `ipc` and `uts`, are always refused.
- YAML references `&` and `<<` are expanded before the check, so they cannot hide an option.
- A second check of the final model: before every start, including a rollback, the plugin runs `docker compose config --format json` under the same isolated environment and with the same two files, and checks what Compose actually computed. Every bind source is an absolute path inside the release folder and read only; there is no `privileged`, `cap_add`, `devices`, `security_opt` other than `no-new-privileges`, or shared namespace; volumes are `local` with no options and not external; the image of every service is the one pinned in `compose.dhow.yaml`; and the project name is the name of the target. Any difference stops the deploy with `COMPOSE_RESOLVED_UNSAFE`. This closes every gap where reading the file misses something that Compose computes in another way. The check also runs before the images are pulled. A model larger than 64 KiB is refused.
- The keys that are allowed are a fixed list: `image`, `command`, `entrypoint`, `environment`, `env_file`, `ports`, `expose`, `volumes`, `networks`, `depends_on`, `healthcheck`, `restart`, `labels`, `logging`, `deploy` (`resources` and `restart_policy` only), `user`, `working_dir`, `read_only`, `tmpfs`, `stop_signal`, `stop_grace_period`, `init`, `hostname`, `domainname`, `mem_limit`, `cpus`, `ulimits`, `shm_size`, `secrets`, `configs`, `profiles` and anything that starts with `x-`. Any other key fails with `COMPOSE_UNSAFE_UNKNOWN_KEY`.
- The administrator can open the `COMPOSE_UNSAFE_*` rules for one target with the `AllowUnsafeCompose` option; these are the keys that need wider rights, such as `privileged`, `cap_add`, `devices` and `sysctls`. The option also reopens writable binds under the deploy folder. Warning: a container that can write in a bound folder can put a symbolic link to `/` in it, and Docker then follows it as root in any later bind from that folder. Open the option only for an application that you trust as much as the server itself. The option never opens a bind of `.dhow`, of the Compose files or of the release folder, and it does not open image pinning, the ban on `build`, `COMPOSE_INVALID`, or the rules about pulling in other files, sharing namespaces or using outside resources: `include`, `extends`, `provider`, `volumes_from`, `container:`, `service:`, anything about `external`, and the sources of `secrets`.
- A refusal stops the job with exit 30 before any connection or write on the server, and writes the code and the service name in the log. In the report the deploy appears as failed with a fixed message.

## Persistent data: use named volumes

A container that writes in a server folder bound into it can put a symbolic link to `/` or to `.dhow` there, and Docker then follows the link as root in a later bind. For this reason the plugin does not allow writing in any server folder: the data that containers write goes in named volumes that Docker manages.

```yaml
services:
  web:
    image: ghcr.io/acme/shop-web@sha256:<digest>
    volumes:
      - uploads:/app/uploads
      - ./conf:/app/conf:ro
volumes:
  uploads: {}
```

- A named volume is declared at the top of the file with no `driver` or with `driver: local`, and with no `driver_opts`.
- Settings that come with the release are bound read only (`:ro`) from a folder inside the release.
- Moving data from `./data:/data`: change the entry to `data:/data` and add `data: {}` under `volumes` at the top of the file. To carry the old content over, create the volume on the server and copy the files into it once before the deploy, for example by running a temporary container that binds the old folder read only and the new volume for writing. The name of the volume on the server is `<project name>_data`.
- Backups work the same way: a temporary container that binds the volume and copies from it.

## Variables in the Compose file

`${...}` variables in the file come from `shared/.env` only. The plugin runs Compose under `env -i` with two variables: a fixed `PATH`, and `DOCKER_CONFIG` that points to `.docker` in the home folder of the deploy account so that Docker finds the registry login. Nothing from the environment of the server, such as `${HOME}`, reaches the file.

- If `shared/.env` does not exist, the plugin passes `/dev/null`, so a `.env` file inside the archive is never loaded.
- A variable that is not defined becomes empty, and Compose warns.
- The values in `shared/.env` can be read by the application, by design: put in it only what the application may see. Variables that are used in `environment` appear inside the container.
- A `shared/.env` with a variable that starts with `COMPOSE_`, `DOCKER_` or `LD_` stops the deploy with `COMPOSE_ENV_FORBIDDEN`, because those variables change what Compose reads. The plugin reads the file by the same rules as Compose (`export`, spaces and Windows line ends), and a line that it cannot read stops the deploy with the same code. Do not use multi-line values in this file.
- Where `${...}` is allowed: in the values of `environment` (list or map), and in `command`, `entrypoint`, `labels` and `healthcheck.test` only. These are strings that belong to the container, and they never become paths on the server or security settings. Anywhere else, such as `image`, `ports`, `networks`, `volumes`, `read_only` and `privileged`, the deploy fails with `COMPOSE_UNSAFE_INTERPOLATION`, even with `AllowUnsafeCompose`.
- The reason: the plugin checks the file as it is, while Compose computes the variables after that. A value such as `read_only: "${RO:-false}"` looks like text to the checker and becomes `false` for Compose, so the folder is bound writable. For this reason variables are not accepted where the value affects the server.
- To write a literal `$`, write `$$`. Writing `$$$X` means a literal `$` followed by a variable, so it is refused outside the allowed places.
- Booleans and numbers that the plugin reads must be of their YAML type (`true` and `false` without quotes). The mode in a `volumes` entry such as `:ro` accepts the known flags only (`ro`, `rw`, `private`, `rprivate`, `nocopy`, `delegated`, `cached` and `consistent`), and an entry has at most three parts. The other SELinux and propagation flags (`z`, `Z`, `shared` and `slave`) are not accepted.

## Plugin inputs

| Input | Required | Default | Meaning |
|---|---|---|---|
| `target` | yes | | The name of the target in the environment of the job |
| `artifact` | yes | | The name of the output file that holds the Compose file |
| `images` | no | | Pin images from the pipeline, as `service=image@sha256:...` |
| `wait_timeout` | no | `120s` | The longest wait for the services to become healthy, from `5s` to `10m` |
| `pull_timeout` | no | `10m` | The longest time to pull the images, from `10s` to `30m` |
| `keep_releases` | no | The value of the target | The number of releases kept |
| `health_check_url` | no | | An `http` or `https` address checked after the services are healthy |
| `health_check_status` | no | `200-399` | The accepted reply, a number or a range |
| `health_check_timeout` | no | `5s` | The timeout of one request |
| `health_check_retries` | no | `10` | The largest number of attempts |
| `health_check_interval` | no | `5s` | The gap between attempts |
| `health_check_stable` | no | `3` | The number of successes in a row that is required |
| `health_check_start_delay` | no | `0s` | A wait before the first attempt |
| `on_first_failure` | no | `keep` | `keep` or `fail`: what is written in the message if the first deploy fails |

The health check rules are the same as in [Deploy to a server over SSH](deploy-ssh.md#plugin-inputs), including `{host}` and the largest budget. The internal `--wait` check verifies the `healthcheck` of the containers, and the external check verifies that a user can reach the service.

## What happens in a deploy

These steps are for one server. On a group they repeat on every server in the same order described in the SSH guide.

1. The plugin checks the archive, its digest and the Compose file, before it connects to any server.
2. It connects with the pinned key only, checks the path, the space and `current`, and takes a lock on the target.
3. It uploads the files to `releases/<release>` and verifies their checksums on the server.
4. It reads `compose.yaml` from the server and checks it a second time, then writes `compose.dhow.yaml` and the record of checksums.
5. It runs `docker compose pull --quiet` for the project. If the pull fails, for example because an image is not in the registry, the release is deleted, the services that run and `current` do not change, and the deploy fails with `DEPLOY_FAILED`.
6. It switches `current` to the new release.
7. It runs `docker compose up --detach --wait --no-build --wait-timeout=<seconds>`. The command recreates only the services whose image or settings changed, and waits for every service to become healthy.
8. It runs the external health check if there is a `health_check_url`.
9. On success it records the release in `.dhow/verified`, deletes the releases older than `keep_releases`, and releases the lock.

Every command is sent to the server as one line built from separate arguments, and every argument is quoted. The project name and the file path arrive as `--project-name=` and `--file=`, so neither is read as an option. No text from a user passes through a local shell.

What `docker compose` prints goes into the job log. The plugin does not delete old images from the server. Schedule `docker image prune` there.

## Automatic rollback

After the switch, a rollback has one of two causes: `docker compose up --wait` fails (a service did not become healthy in time), or the external health check fails. A cancel of the job after the switch also rolls back.

- Before every start, including a rollback and the resume of a deploy that was cut off, the plugin reads the two files of the release from the server again, compares their checksums with the record, and checks them with all the rules. If one of the files changed after it was checked, the plugin starts nothing, and the rollback fails with `DEPLOY_ROLLBACK_FAILED` and the code `COMPOSE_FILE_CHANGED` in the log.
- The rollback switches `current` to the last release that Dhow checked and that succeeded, then runs `docker compose up --wait` with the files of that release. The images in `compose.dhow.yaml` are pinned by digest, so if they were deleted from the server Compose pulls them again.
- Services that did not change are not recreated. Services that started with the new images go back to the images of the previous release.
- A first deploy on a target with no earlier release cannot roll back. The containers and the release stay as they are, and the job fails. Fix the service and deploy again.
- If the rollback itself fails, the job fails with `DEPLOY_ROLLBACK_FAILED`, and Dhow records that the server is in an unconfirmed state.

The containers of the new release that failed stay on the server until the rollback replaces them. Dhow does not undo changes outside the services, such as a database update.

## Deploy to a group of servers

The same stages and the group rollback described in the SSH guide: a pre-check on all servers, then an upload to all, then a switch in batches (`Settings.Rolling.BatchSize` and `BatchPauseSeconds`) with a health gate for every batch. The first failure stops the group, puts every switched server back on the images of its previous release, and the state of the group is `rolled_back`.

Images are pulled on all servers before the first switch, so a missing image on one server stops the deploy before any server changes. A `--wait` failure on a server in a later batch rolls back the servers that were switched before it. Use `{host}` in `health_check_url` so that every server is checked on its own address.

## Result codes

The same SSH codes for `DEPLOY_ROLLED_BACK` (10), `DEPLOY_HEALTH_FAILED` (11), `DEPLOY_ROLLBACK_FAILED` (12), `HOST_KEY_MISMATCH` (13), `ARTIFACT_UNSAFE` (14), `TARGET_LOCKED` (15), `CURRENT_NOT_A_SYMLINK` (16), `TARGET_HOST_UNREACHABLE` (17), `PLUGIN_INPUT_INVALID` (2) and `DEPLOY_FAILED` (1), with one new code:

| Code | Number | Meaning |
|---|---|---|
| `COMPOSE_*` | 30 | The Compose file was refused before any connection. The full code (such as `COMPOSE_UNSAFE_PRIVILEGED`) is in the step log |

## Common errors

- **`COMPOSE_IMAGE_NOT_PINNED`:** a service has an image with a name or a tag only. Pin the digest in the file or with the `images` input.
- **`DEPLOY_FAILED` with `manifest unknown` in the log:** the image is not in the registry. Make sure the job that built it pushed it before the deploy.
- **`DEPLOY_FAILED` with `unauthorized` in the log:** the deploy account did not log in to the registry on the server. Run `docker login` as that account.
- **`DEPLOY_ROLLED_BACK` and an `unhealthy` service:** the `healthcheck` of the service did not succeed within `wait_timeout`. Raise the timeout or fix the check. A service without a `healthcheck` counts as healthy as soon as it runs, and the external `health_check_url` check is then the only protection, so write a `healthcheck` for every service.
- **`COMPOSE_UNSAFE_BIND_OUTSIDE` or `COMPOSE_UNSAFE_BIND_WRITABLE`:** the file binds an absolute path or a writable folder. Use a named volume for data, as in [Persistent data](#persistent-data-use-named-volumes).
- **`TARGET_LOCKED` and `HOST_KEY_MISMATCH`:** as in the SSH guide.

## Limits of this version

- The plugin does not delete images, volumes or orphan containers. Removing a service from the file does not stop its container.
- There is no manual rollback and no "check the target" button yet.
- Dhow does not write application secrets on the server. Put `shared/.env` there yourself.
- The plugin does not support `build`, `include` or `extends`, nor more than one Compose file for a target.
