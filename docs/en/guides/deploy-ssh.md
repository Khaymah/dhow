[العربية](../../guides/deploy-ssh.md)

# Deploy to a server over SSH

The official `dhow-deploy-ssh` plugin uploads the new release to its own folder on the server, switches the `current` link to it in one step, and then checks that the application works. If the check fails, or the reload command or one of the commands fails, the plugin goes back to the release that succeeded before it. The server needs nothing but `sshd` and an ordinary file system.

A target can be one server or a group of up to 10 servers. On a group the plugin deploys in batches, and if one server fails, every server that was switched in this deploy goes back. The details are in [Deploy to a group of servers](#deploy-to-a-group-of-servers).

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

1. Create the target in the environment with the deploy path, the deploy account and one or more servers (up to 10).
2. Generate the access key and put its public part in `authorized_keys` on every server. The key is one for the target.
3. Fetch the host key of each server, compare its fingerprint and confirm it. The plugin connects only with the pinned key: it never accepts a new key by itself, and any difference fails with `HOST_KEY_MISMATCH` before any file is written.

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
| `artifact` | Yes | | The name of the file to deploy. Not written for a check or a rollback: in a rollback the server sets it |
| `mode` | No | `deploy` | `deploy`, `check` (check of the target) or `rollback`. A check and a rollback start from the server only, and the server does not accept `rollback` in a pipeline file (`ROLLBACK_MODE_NOT_ALLOWED`) |
| `release` | Only in `rollback` | | The number of the release the target goes back to, set by the server |
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

These steps are for one server. On a group they repeat on every server in the order described in [Deploy to a group of servers](#deploy-to-a-group-of-servers).

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

`ReloadCommand`, the pre-switch command and the post-switch command are lines that the Admin writes in the target, and a commit cannot change them. **Never put a secret in a command** (a password, a token or a key): the command text appears in the audit log and in deploy reports, and everyone who holds the admin permission on the project reads it. Use a deploy secret instead. In API answers and on the page, the text of the commands is shown only to whoever holds `environment.manage`. Everyone else, including every API token, sees that a command exists without its text, in the fields `ReloadCommandSet`, `PreSwitchCommandSet` and `PostSwitchCommandSet`. Each command runs on the server inside the folder of the new release, with these variables:

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

## Cancel and interruption

If you cancel the job during the deploy step after the switch happened, the plugin goes back to the last trusted release, and then the job is recorded as `canceled`. Dhow gives the deploy step a cancel grace longer than the other steps, about 4 minutes. It is computed from the rollback budget (3 minutes) plus closing the connections and releasing the locks, so Dhow does not cut the plugin in the middle of the rollback. The other steps keep the normal cancel grace. The same long grace applies when the job times out (not only when it is canceled), because going back is right in both cases. So a job may run about 4 minutes past its timeout before it ends as `timed_out`.

If the runner itself dies (`SIGKILL`) after the switch, nobody can go back. The job ends `lost` with `DEPLOY_INTERRUPTED` and is never run again by itself. Dhow marks the release and its servers that did not finish as `unknown`, and never assumes they succeeded. On the server, `.dhow/state.json` stays in the state `switching` and `.dhow/verified` stays on the previous release. One of two things fixes this:

- **Check the target:** it reads the state of each server. If the server is ready (`ok`) and `current` matches `.dhow/verified`, Dhow writes the real state: `verified` if the server is on the interrupted release, and `rolled_back` if it is on another one. If the two differ the state is not clear, so the server stays `unknown` and you see `diverged` in the result of the check.
- **A new successful deploy:** if it fails it goes back to `.dhow/verified`. If it succeeds, Dhow closes every old release that stayed `unknown` on its servers.

The remote lock that a dead runner left ends after 30 minutes, and then the next deploy takes it with a warning.

## Checking a target

A check reads the state of the servers and changes nothing on them. Start it with a `POST` on the path of the target followed by `/check` (the action `deployment.check`, for the owner, the admin and the maintainer), and read its result with a `GET` on the same path. The request goes through the gate of the environment like any deploy: its rules, its lock and its approvals, and it runs the official plugin image that is pinned in the server. A check needs no artifact. If you ask for a check of a target that already has a check that has not ended, the request returns `202` with the number of the run that exists and does not start another. Checks have a limit of their own, 10 for each user and 30 for each project in every 10 minutes, apart from the limit of normal runs. A check run is not run again (it returns `CHECK_RUN_NOT_RERUNNABLE`): ask for a new check. A check does not appear in the list of deploys of the environment.

The plugin connects to each server with the pinned key and reads `current`, `.dhow/verified`, the state in `.dhow/state.json`, the free disk space and the lock. Then it writes one reason for each server:

| Reason | Meaning |
|---|---|
| `ok` | The server is ready for a deploy |
| `host_key_mismatch` | The key of the server is not the pinned one, and nothing was read from it |
| `unreachable` | The server could not be reached |
| `auth_failed` | The server did not accept the key of the target |
| `path_missing` | The deploy path is not a directory on the server |
| `current_not_symlink` | `current` is not a link to a release |
| `state_corrupt` | A file of Dhow inside `.dhow` is damaged |
| `disk_low` | The free space is under 64 megabytes |
| `locked` | Another deploy holds the lock and has not ended |
| `diverged` | `current` is not the last release that passed its check, or a deploy was interrupted. This is not a failure |

The job fails with `TARGET_CHECK_FAILED` if the reason of any server is something other than `ok` and `diverged`. The result of each server stays saved, with `current`, the last trusted release, the free space, and whether the check wrote the state of the server in the records of Dhow (`Repaired`).

## Deploy to a group of servers

If the target has more than one server, the plugin deploys one release (one number and one digest) to all of them, from one step on one runner. Each server has its own record in Dhow with its state and the result of its check.

The batches are set in `Settings.Rolling` in the settings of the target, and the order of the servers in the target is the order of the batches.

| Field | Meaning |
|---|---|
| `BatchSize` | The number of servers that switch together in a batch. From 1 to the number of servers, and the default is 1 |
| `BatchPauseSeconds` | The wait between two batches in seconds. From 0 to 600, and the default is 0 |

The stages in order:

1. **A check on all servers first**, one after the other: the connection with the pinned key, the deploy path, the space and `current`, and taking the lock of the server. Any server that fails stops the deploy before any change, and the locks that were taken are released.
2. **Upload the release to all servers**, check its digests and run the pre-switch command, up to 4 servers at a time. Nothing that is running changes in this stage.
3. **Switch batch by batch.** In each batch every server switches, reloads and then checks its health, and the servers of the batch work together. If `health_check_url` has the value `{host}`, each server is checked on its own address. If it does not, the address is checked once for the batch and does not tell one server from another (like the address of a load balancer). After a batch passes, the plugin waits `BatchPauseSeconds` and then starts the next one.
4. **Success:** after the last batch passes, the plugin records the release in `.dhow/verified` on all servers, deletes the older releases, and releases the locks.

The total of the health budgets of the batches and the waits between them is at most 60 minutes, or the job fails before it starts with `ROLLING_BUDGET_EXCEEDED`. This figure covers the health checks and the waits only. Uploads and commands add to it, and the job timeout in Dhow is what limits the whole job.

### When it fails

The first failure on any server stops the deploy at once. The failure can be in the upload, the switch, the reload, one of the commands, the health check, or a cancel of the job.

- No new batches start. The servers that have not switched yet do not change, their upload files are deleted, and they appear in the log as `skipped`.
- Every server that switched in this deploy goes back to the last release that Dhow checked on it. The rollback starts from the newest batch to the oldest, and the servers of one batch go back together.
- The rollback does not stop if it fails on a server. The plugin goes on with the rest of the servers. The budget of the whole rollback is 3 minutes, and a cancel of the job does not cut it.
- There is no third automatic try.

| State of the group | Meaning | Code |
|---|---|---|
| `failed` | It failed before any server switched, and nothing changed | By reason, such as `HOST_KEY_MISMATCH`, `TARGET_HOST_UNREACHABLE` or `DEPLOY_FAILED` |
| `rolled_back` | Every server that switched went back | `DEPLOY_ROLLED_BACK` |
| `rollback_failed` | At least one server stayed on the new release or it is not known which release it is on | `DEPLOY_ROLLBACK_FAILED` |
| `unknown` | The runner went down during the deploy | Dhow sets it after the job ends |

The state of each server in the record of the release is one of: `pending`, `uploaded`, `switched`, `verified`, `failed`, `rolled_back`, `rollback_failed`, `unknown` and `skipped`.

- **A server with no earlier release.** If all the servers that switched have no earlier release (the first deploy to the target), they stay as they are and the group fails as `failed` with `DEPLOY_HEALTH_FAILED`, as with one server. If some have an earlier release and some do not (a server that was added to the target recently), the one with none stays on the new release and is recorded as `rollback_failed`, and so is the group.
- **If the rollback fails on a server**, the group is on different releases. Dhow records the reason for each server and writes an audit record with high severity at once. Fix it with a new successful deploy to the target. The new deploy starts by reading the release of each server and writes a warning, and it fixes what is left.
- **A lock for each server.** It is taken in the order of the servers and stays for the whole deploy, and the plugin releases it on every path. Another deploy that holds the lock of one server stops this deploy with `TARGET_LOCKED` before any change.

## Rolling back to an earlier release

A manual rollback puts the whole target, every one of its servers, back on a release that succeeded on it before. It never rebuilds. It starts with a `POST` to the path of the target followed by `/rollback`, with `ReleaseId` and `CurrentFactor` in its body. The action is `deployment.rollback` for owners, admins and maintainers, and it needs a second factor. The request returns `202` with `RunId` and `RunNumber`. The page of the environment has the list of releases and a button "Roll back to this release", with the second factor and a confirmation question, next to a button "Check the target", the result of the last check of each server, and a badge "The group is on different releases".

Dhow refuses the request with `ROLLBACK_RELEASE_NOT_ELIGIBLE` when the release is not a deploy that succeeded on this target: a release that failed, was rolled back or is `unknown`, a release that is itself a rollback, or a number the target does not know. It refuses it with `ENVIRONMENT_PAUSED` when deploys are paused in the environment. If this build of Dhow has no official plugin image it returns `ROLLBACK_UNAVAILABLE`. In this version a rollback is for SSH targets only, and any other target returns `ROLLBACK_NOT_SUPPORTED`.

The request starts a run of one job (its `Trigger` is `rollback`) that runs the official plugin image in `rollback` mode on every server of the target. The run goes through the gate of the environment like any deploy, and it skips only the reviewers and the wait timer. So the lock of the environment, the rules of the branch, the secrets and the network limits that the server computes all still apply. The release is recorded with `Kind = rollback` and the release it went back to (`RestoredReleaseId`), and the audit entry `deployment.rollback_requested` is written with a high severity. This run is never run again by `rerun-failed` (it returns `ROLLBACK_RUN_NOT_RERUNNABLE`): ask for a new rollback.

On each server the plugin uses the same batches, the same order and the same health gate as a deploy:

- **A server that still has the directory of the release** switches `current` to it, reloads and checks the health. It uploads nothing.
- **A server that removed the directory** (because of `keep_releases`) gets the release again from the artifact that Dhow kept for it, after the plugin checks the digest, and then switches. This is the redeploy. The pre-switch command does not run in a rollback.
- If a server of the target lost the directory of the release and the artifact is no longer kept, the plugin fails before any server changes.

Dhow keeps the artifact of every release that succeeded for as long as the release is among the last `keep_releases` successful releases of the target, even after its normal retention ended, as long as the artifact belongs to the project of the target. The longest it is kept this way is 90 days past its normal expiry, and then it is removed even if the release is still in the list. The kept artifact counts against the artifact quota of the organization. The health check of the rollback is the one of the release you chose, as it was stored when that release was deployed. If what was stored is damaged, Dhow refuses the rollback with `ROLLBACK_SETTINGS_INVALID` and does not roll back without a check. Releases deployed before this version of Dhow have no stored health check, so a rollback to them runs without a health check.

If the rollback fails on a server it stops at once: no new batch starts, and the group becomes `rollback_failed` with the same alert and the high severity audit entry (`DEPLOY_ROLLBACK_FAILED`). Dhow does not put the servers that switched back by itself, and it does not try again on its own. The servers that switched and whose health was not confirmed are marked `unknown`. Check the target to see which release each server is on, then roll back again or deploy.

## Result codes

The plugin writes the code in the log of the step and exits with the matching number.

| Code | Number | Meaning |
|---|---|---|
| `DEPLOY_ROLLED_BACK` | 10 | It failed after the switch and the previous release came back, and on a group every server that switched went back |
| `DEPLOY_HEALTH_FAILED` | 11 | The check failed and there is no earlier release, so `current` stayed as it is |
| `DEPLOY_ROLLBACK_FAILED` | 12 | The rollback itself failed, and on a group on at least one server |
| `HOST_KEY_MISMATCH` | 13 | The key of the server is not the pinned one, and nothing was written |
| `ARTIFACT_UNSAFE` | 14 | The archive has an unsafe entry or its digest does not match, and nothing was written |
| `TARGET_LOCKED` | 15 | Another deploy holds the lock of the target |
| `CURRENT_NOT_A_SYMLINK` | 16 | `current` is not a symbolic link and we did not touch it |
| `TARGET_HOST_UNREACHABLE` | 17 | The connection to the server failed |
| `TARGET_STATE_CORRUPT` | 18 | A file of Dhow on the server is damaged, and nothing was written |
| `TARGET_CHECK_FAILED` | 19 | The check of the target found a server that is not ready |
| `PLUGIN_INPUT_INVALID` | 2 | An input or a setting is not valid |
| `DEPLOY_FAILED` | 1 | Another failure before the switch |

Dhow shows these same codes as the reason a job failed, without opening the log, in Arabic and English. A cancel stays `canceled` even if the plugin went back and exited with its own number.

Dhow keeps, for each release, the target, the release id, the previous release, the result of the check and the reason for the rollback. It keeps no key and no text of the answer of the server.

## Common errors

- **`HOST_KEY_MISMATCH`:** the key of the server changed, either because the server was rebuilt or because someone intercepts the connection. Compare the fingerprint in a safe way before you pin the new key.
- **`TARGET_LOCKED`:** another deploy is running. If the runner crashed during it, the lock ends after 30 minutes, or delete the `.dhow/lock` folder by hand after you make sure no deploy is running.
- **The check fails with `status`:** the address answers outside the accepted range. Open it from another machine and make sure the web server reads from `current` and the file exists in the release.
- **The check fails with `refused`:** the service has not started yet. Raise `health_check_start_delay` or `health_check_retries`.
- **The check fails with `certificate`:** the certificate of the address is not valid or does not match the name. The check never skips the certificate.
- **A server of the group does not connect (`TARGET_HOST_UNREACHABLE`):** the deploy fails before any server switches and nothing changes. Fix the connection or start the server and deploy again.
- **`TARGET_STATE_CORRUPT`:** the plugin checks `.dhow/state.json`, `.dhow/verified` and `.dhow/history` before any change. If one of them is damaged (not written the way Dhow writes it) it stops and changes nothing, because it does not guess which release to go back to. Check the target to find the server, open the file on the server, and then delete it or fix it. The next deploy writes it again.
- **A server that does not accept the key (`auth_failed` in the check):** put the public half of the key of the target in `authorized_keys` on the server, and make sure the deploy account is the one in the target.
- **The group is on different releases after `DEPLOY_ROLLBACK_FAILED`:** open the record of the release to see the server that did not go back and the reason, fix the reason, and then deploy a new release. The new deploy brings the group to one release.

## Limits of this version

- The first deploy to a group goes back to nothing if it fails in the middle: the servers that switched stay on the new release.
- If a run has a second job that deploys to the same target, it waits for the first to end and shows "Target busy", and it does not fail. Between two different runs the lock of the environment stops them.
- A manual rollback puts back files and reloads only. It does not undo changes outside the files, such as database migrations. And a manual rollback that failed is not rolled back by itself.
- Dhow does not write application secrets on the server. Put the `.env` file in `shared/` yourself.
- The commands written by the Admin run in the login shell of the deploy account on the server. This is the only exception to the rule "no text goes through a shell", and it is written in the architecture decision.
- The plugin keeps the last 64 KiB of what a command prints on the server, in memory only. It writes only the last 4 KiB of it to the step log, after the secrets are masked. A report event larger than 4 KiB is dropped whole, not cut.

## Try it on a clean machine

`deploy/m6-demo/run.sh` runs the whole path in Docker on a machine that has Docker with Compose v2 and cgroup v2, and nothing else: a real `sshd` and a Docker host as targets, the host keys pinned after a fingerprint check, a deploy that succeeds, a deploy that fails its health check and goes back by itself, a rollback by hand, an artifact pushed with `dhow artifact push` that starts a deploy, a server whose host key changed (`HOST_KEY_MISMATCH`, with nothing written), a Docker Compose project that deploys and goes back, and at the end the leak sweep with the private key of the target and the value of the ingest token added. The same demo runs in the `Demo` workflow when any file it depends on changes.
