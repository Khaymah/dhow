[العربية](../../guides/deploy-ssh.md)

# Deploy to a server over SSH

The official `dhow-deploy-ssh` plugin uploads the new release to its own folder on the server, switches the `current` link to it in one step, and then checks that the application works. If the check fails, or the reload command or one of the commands fails, the plugin goes back to the release that succeeded before it. The server needs nothing but `sshd` and an ordinary file system.

This version of the plugin deploys to one server. A target with more than one server fails with a clear reason, and the rolling deploy across a group comes in a later release.

## How the files look on the server

```text
/srv/shop/
  releases/
    41-3fa9c1d2-1/
    42-8be07c55-1/
  shared/
  current -> releases/42-8be07c55-1
  .dhow/
```

- A release id is made of the run number, the first 8 characters of the commit and the round number, so it tells you which commit was deployed.
- `current` is a symbolic link to the folder of the current release. Make your web server (nginx, systemd or any other) read from `current`.
- `shared` holds the files that stay between releases, such as `.env` and the upload folder.
- `.dhow` keeps the state of Dhow: the deploy lock, the last release that passed its check, and the order of the releases.

`current` changes only by an atomic switch: a new link is made under a temporary name and then moved over `current` with one `rename`, so there is no moment without `current`. If `current` is a real folder, for example from an old rsync deploy, the plugin refuses to deploy with `CURRENT_NOT_A_SYMLINK` and does not touch it. Convert the folder yourself once: move it to `releases/<name>` and then make `current` a link to it.

## Prepare the server

Create an account used only for deploys, without more rights than it needs:

```bash
sudo adduser --disabled-password --gecos "" dhow-deploy
sudo install -d -o dhow-deploy -g dhow-deploy -m 0755 /srv/shop
```

Put the public key that Dhow made in `~dhow-deploy/.ssh/authorized_keys` with options that restrict the key:

```text
restrict,from="203.0.113.10" ssh-ed25519 AAAA... dhow
```

- `restrict` turns off the terminal and port forwarding. The plugin needs to run commands and to use the file transfer (`sftp`) only.
- `from` limits the login to the address of the runner.
- Do not add this account to the `docker` group or to `sudo`. If the service needs a reload, add one `sudoers` line for one command:

```text
dhow-deploy ALL=(root) NOPASSWD: /usr/bin/systemctl reload shop
```

and write the same command in the `ReloadCommand` of the target.

The server needs `sshd` with the `sftp` file transfer, the `sha256sum` command, and an existing deploy folder that the deploy account owns. Before any change, the plugin checks that the folder has twice the size of the unpacked release free.

## The target and the key

The target, the key and pinning the host key are described in [Deploy targets](environments.md#deploy-targets). In short, you need this here:

1. Create the target in the environment with the deploy path, the deploy account and one server.
2. Generate the access key and put its public part in `authorized_keys`.
3. Fetch the host key of the server, compare its fingerprint and confirm it. The plugin connects only with the pinned key: it never accepts a new key by itself, and any difference fails with `HOST_KEY_MISMATCH` before any file is written.

## The step in the pipeline file

```yaml
deploy:
  needs: [build]
  environment: production
  steps:
    - uses: ghcr.io/khaymah/dhow-deploy-ssh@sha256:03fddb3b486b758f338f8999afbf04ced985d74825963dd28b92e650645e6778
      with:
        artifact: web
        target: web
        keep_releases: 5
        health_check_url: https://{host}/healthz
```

`artifact` is the file that a run in `needs` uploaded. It reaches the plugin as an archive, as it is, after the runner checks its digest, and the plugin checks the digest again before it sends anything. The plugin does not unpack the archive on the disk of the runner: it reads it as a stream and sends each file to the server directly, and it refuses every unsafe entry: a path with `..`, an absolute path, a link that leaves the release, a device file or a hard link. A refused archive changes nothing on the server.

## Plugin inputs

| Input | Required | Default | Meaning |
|---|---|---|---|
| `target` | Yes | | The name of the target in the environment of the job |
| `artifact` | Yes | | The name of the file to deploy |
| `strategy` | No | `atomic-symlink` | The only supported way |
| `keep_releases` | No | The value of the target | How many releases to keep. It can be lower than the value of the target and never higher |
| `health_check_url` | No | | An `http` or `https` address that is checked after the switch. Without an address there is no check |
| `health_check_status` | No | `200-399` | The accepted answer, a number or a range |
| `health_check_timeout` | No | `5s` | The timeout of one request, up to `30s` |
| `health_check_retries` | No | `10` | The most attempts, up to 60 |
| `health_check_interval` | No | `5s` | The pause between attempts, from `1s` to `60s` |
| `health_check_stable` | No | `3` | How many successes in a row are needed. Any failure starts the count again |
| `health_check_start_delay` | No | `0s` | A wait before the first attempt, up to `300s` |
| `on_first_failure` | No | `keep` | `keep` or `fail`: what the message says if the first deploy fails and there is no earlier release |

- The sum of `start_delay + retries × (interval + timeout)` does not exceed 10 minutes, or the job fails before it starts with `HEALTH_BUDGET_EXCEEDED`.
- The check request leaves the runner through the proxy that allows the check address only, and it verifies the certificate fully. There is no option to skip the certificate and no custom headers, so no secret goes through the check. The check follows up to 3 redirects to the same host, reads 64 KiB of the answer and ignores the rest.
- In `health_check_url` you can write `{host}` in place of the whole server name, and the server is checked on its own address. The rule for the check address is the one in the [pipeline specification](../pipeline-spec.md#plugins-that-deploy-to-a-target): the server reads it the same way the plugin does.

## What happens in a deploy

1. The plugin checks the archive and its digest before it connects to any server.
2. It connects with the pinned key only, checks the deploy path, the space and `current`, and takes a lock on the target. A lock that another deploy took and whose time has not ended refuses this deploy with `TARGET_LOCKED`. An expired lock is taken with a warning.
3. It uploads the files to `releases/.incoming-<release>` and computes the digest of each file, and then the server verifies the digests with `sha256sum -c`.
4. It links the shared paths (`Shared`) from the release to `shared/`.
5. It moves the folder to `releases/<release>` with `rename`, and then runs the pre-switch command if there is one. If it fails there is no switch and the release is deleted.
6. It switches `current`, and then runs `ReloadCommand` and the post-switch command.
7. It checks the health.
8. On success it records the release in `.dhow/verified`, deletes the releases older than `keep_releases`, and releases the lock.

Pruning never touches `current` or the previous release, and it orders the releases by when they were deployed to this target. A release that failed is deleted before the releases that succeeded.

## Commands

`ReloadCommand`, the pre-switch command and the post-switch command are lines that the Admin writes in the target, and a commit cannot change them. Each command runs on the server inside the folder of the new release, with these variables:

| Variable | Value |
|---|---|
| `DHOW_DEPLOY_PATH` | The deploy path |
| `DHOW_RELEASE_PATH` | The path of the release folder |
| `DHOW_RELEASE_ID` | The release id |
| `DHOW_PREVIOUS_RELEASE` | The id of the release before it, or empty |

The commands are sent to the server exactly as written, as one argument, and they are never joined with the path or the release id. The values reach the command above as environment variables, so read them inside the command (`"$DHOW_RELEASE_PATH"`). Each command has a limit of two minutes, which the server also enforces with the `timeout` command when it has one (`timeout -s KILL`); otherwise Dhow cuts the session from its side only. The pre-switch command runs before any change, and its failure stops the deploy without a rollback because nothing changed. A failure of any other command after the switch brings the previous release back.

What a command prints goes into the log of the job, and Dhow masks only the secrets that are registered. Do not print in a command the value of a secret that you did not register in Dhow.

On a rollback `ReloadCommand` runs again, and the variables of the command describe the release we went back to.

## Automatic rollback

After the switch a rollback has one of three causes: the health check failed, `ReloadCommand` failed, or the post-switch command failed. Canceling the job after the switch also rolls back. The rollback is a second atomic switch to the last release that Dhow checked and that succeeded, and then `ReloadCommand` again.

- The release it goes back to is the one recorded in `.dhow/verified`, not whatever `current` was. So if a deploy is cut off before its check ends, what stays on the server is not trusted, and the next deploy goes back to the trusted release.
- The first deploy to a target that has no earlier release cannot go back. The plugin leaves `current` as it is and fails with `DEPLOY_HEALTH_FAILED` and guesses nothing.
- If the rollback itself fails, the plugin fails with `DEPLOY_ROLLBACK_FAILED` and Dhow records that the server is on a release that is not confirmed. Fix it by hand and deploy again.

Dhow does not undo changes outside the files, such as a database migration. Design the releases so that the previous one works with the new database.

## Result codes

The plugin writes the code in the log of the step and exits with the matching number.

| Code | Number | Meaning |
|---|---|---|
| `DEPLOY_ROLLED_BACK` | 10 | It failed after the switch and the previous release came back |
| `DEPLOY_HEALTH_FAILED` | 11 | The check failed and there is no earlier release, so `current` stayed as it is |
| `DEPLOY_ROLLBACK_FAILED` | 12 | The rollback itself failed |
| `HOST_KEY_MISMATCH` | 13 | The key of the server is not the pinned one, and nothing was written |
| `ARTIFACT_UNSAFE` | 14 | The archive has an unsafe entry or its digest does not match, and nothing was written |
| `TARGET_LOCKED` | 15 | Another deploy holds the lock of the target |
| `CURRENT_NOT_A_SYMLINK` | 16 | `current` is not a symbolic link and we did not touch it |
| `TARGET_HOST_UNREACHABLE` | 17 | The connection to the server failed |
| `PLUGIN_INPUT_INVALID` | 2 | An input or a setting is not valid |
| `DEPLOY_FAILED` | 1 | Another failure before the switch |

Dhow keeps, for each release, the target, the release id, the previous release, the result of the check and the reason for the rollback. It keeps no key and no text of the answer of the server.

## Common errors

- **`HOST_KEY_MISMATCH`:** the key of the server changed, either because the server was rebuilt or because someone intercepts the connection. Compare the fingerprint in a safe way before you pin the new key.
- **`TARGET_LOCKED`:** another deploy is running. If the runner crashed during it, the lock ends after 30 minutes, or delete the `.dhow/lock` folder by hand after you make sure no deploy is running.
- **The check fails with `status`:** the address answers outside the accepted range. Open it from another machine and make sure the web server reads from `current` and the file exists in the release.
- **The check fails with `refused`:** the service has not started yet. Raise `health_check_start_delay` or `health_check_retries`.
- **The check fails with `certificate`:** the certificate of the address is not valid or does not match the name. The check never skips the certificate.

## Limits of this version

- One server in the target.
- No manual rollback and no "check the target" button yet. The only rollback is the automatic one during a deploy.
- Dhow does not write application secrets on the server. Put the `.env` file in `shared/` yourself.
- The commands written by the Admin run in the login shell of the deploy account on the server. This is the only exception to the rule "no text goes through a shell", and it is written in the architecture decision.
