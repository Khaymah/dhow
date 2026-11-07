[العربية](../../install/upgrades.md)

# Upgrades and rolling back

This page is for whoever runs a Dhow server. It explains what happens to the database during an upgrade, when you can go back to the previous binary without restoring the database, and when you cannot. The decision and its details are in [ADR-0077](../../../Plan/ADR/0077-upgrades-migrations-and-update-policy.md).

## How the database changes between releases

Every release brings database migrations. They run automatically when Dhow starts, under a lock, so only one process migrates at a time. Each migration is one of two kinds:

| Kind | What it does | Does the previous release still run after it? |
|---|---|---|
| `expand` | Only adds: a table, a column with a default, or an index. | Yes. The previous release does not know the addition and ignores it. |
| `contract` | Removes or changes something the new release no longer uses. It ships only in a minor or major release, and one full release after the `expand` that replaces it. | Only if it is not older than the **floor** the migration records. |

The floor lives in the database itself, in `"Instance"."SchemaFloor"`. It is `0.0.0` until the first `contract` migration raises it. So every release knows when it starts whether it can run on this database, even a database newer than itself.

The rule: the database of release N runs both release N and release N-1. Going back one release is safe in most cases.

## When rolling back is safe

| What the upgrade did | Is going back to the previous binary enough? | What to do |
|---|---|---|
| Every migration was `expand`, or there were none | Yes | Install the previous binary or image and restart. Dhow starts and the `migrations` check is healthy. |
| A `contract` migration raised the floor above the version you go back to | No | Restore the backup you took before the upgrade, then start the previous release. Data written after the upgrade is lost. |
| A migration stopped halfway | Yes | The migrations that finished before it are all `expand`, and nothing of the one that stopped was kept. |
| You go back more than one release | Maybe | Run `dhow-server migrate status` with the release you go back to. If the state is `incompatible`, restore the backup. |

Take a backup before every upgrade anyway: [Compose](compose.md#back-up-and-restore), and [binary with systemd](binary-systemd.md#back-up-and-restore).

## What an older release does when it starts

Before anything else, Dhow compares the database with what it knows:

| State | Meaning | What happens |
|---|---|---|
| `current` | The database is at the last migration this release knows. | It runs normally. |
| `pending` | The database is older than the release. | It applies the missing migrations, then runs. |
| `ahead` | The database is newer than the release, and the floor is not above its version. | It runs, logs a warning, and `/api/v1/health/ready` answers 200. |
| `incompatible` | The database is newer, and the floor is above its version. | It refuses to start and exits with code `78`. |

In the `incompatible` state Dhow prints a message that names the version it needs:

```text
dhow-server: SCHEMA_TOO_NEW: the database needs dhow 1.4.0 or newer, and this binary is 1.3.2. The database is at migration 61 and this binary knows migrations up to 57. Install dhow 1.4.0 or newer, or restore the database backup taken before the upgrade. See https://github.com/Khaymah/dhow/blob/main/docs/install/upgrades.md
```

- **With systemd:** the unit has `RestartPreventExitStatus=78`, so systemd does not restart Dhow over and over. Read the message with `journalctl -u dhow -n 20`.
- **With Compose:** the Docker image sets `DHOW_ON_INCOMPATIBLE=wait`. So Dhow does not exit: it logs the message once and waits to be stopped, while the health check fails. `docker compose ps` shows the container as `unhealthy` instead of restarting it over and over. Read the message with `docker compose logs dhow`, then set `DHOW_IMAGE` to the release it needs or restore the backup.
- **A development build** (built from source, with the version `dev` or a commit) runs on a database newer than itself only with `DHOW_DEV=true`, because we cannot tell where it sits between releases.

If the database changes while Dhow runs, for example when another server upgrades the same database and raises the floor, the `migrations` check in `/api/v1/health/ready` turns unhealthy and it answers 503.

## Check before you upgrade

The binary has three commands that read the database and change nothing. They need only `DHOW_DATABASE_URL` (or `DHOW_DATABASE_URL_FILE`), and `DHOW_DEV` if you use it:

| Command | What it shows |
|---|---|
| `dhow-server migrate status` | The state (`current`, `pending`, `ahead` or `incompatible`), the last migration in the database and in the binary, the floor, the pending migrations, the progress of every background backfill, and the servers running on the database now. |
| `dhow-server migrate plan` | Every migration file, its kind (`expand` or `contract`), its floor, whether it runs in a transaction, and whether it was applied. |
| `dhow-server migrate dry-run [--timeout 60s]` | Tries the pending migrations on the real database in one transaction, then rolls all of them back. It stops after one overall deadline, 60 seconds by default. |

Run them **with the new release** before you replace the current one, so it knows the new migrations.

**Warning:** `dry-run` runs on the live database while Dhow uses it, and takes the same locks the real migration takes. Users may wait until it finishes or reaches its deadline. Run it at a quiet time. For large changes, try it on a restored copy of the database rather than on the live one (U5 will add the `--against-backup` option for that).

With the binary and systemd, put the new file next to the old one and run it with the service's own settings:

```sh
install -o root -g root -m 0755 dhow-server /usr/local/bin/dhow-server.new
systemd-run --pipe --wait --quiet -p User=dhow -p EnvironmentFile=/etc/dhow/dhow.env \
  -p LoadCredential=dhow-database-url:/etc/dhow/credentials/dhow-database-url \
  /usr/local/bin/dhow-server.new migrate dry-run
```

With Compose, run the new image once against the same database:

```sh
DHOW_IMAGE=ghcr.io/khaymah/dhow:1.4.0 docker compose run --rm --no-deps dhow migrate dry-run
```

What the `dry-run` output means:

- **`applied`:** the migration worked inside the transaction, and was then rolled back.
- **`not tried (NO TRANSACTION)`:** files that build indexes `CONCURRENTLY` cannot run inside a transaction, so they are not tried. They are safe to repeat.
- **`PREFLIGHT_LOCK_TIMEOUT`:** a migration waited more than 3 seconds for a lock on a table, because another process is using it. The real migration would wait for the same lock, so pick a quieter time.
- **`PREFLIGHT_STATEMENT_TIMEOUT`:** a statement took more than 5 minutes.
- **`PREFLIGHT_DEADLINE`:** the whole try reached its deadline (`--timeout`). A migration file cannot lift it with `SET statement_timeout = 0`.
- **`PREFLIGHT_TRANSACTION_CONTROL`:** a migration file contains `COMMIT`, `ROLLBACK`, `BEGIN` or the like. Migrations must not, so nothing is tried.
- **`PREFLIGHT_TRANSACTION_ENDED`:** a statement ended the transaction anyway. Only in this case may the statements before it be committed, so check the database.
- **`PREFLIGHT_MIGRATION_FAILED`:** a migration failed, and the message names the file, the statement number and the error. Do not upgrade until you understand why.

In every other case the database does not change.

## Roll back

1. Run `dhow-server migrate status` with the release you go back to, as in the examples above.
2. If the state is `ahead` or `current`, go back with the binary alone:
   - **systemd:** install the old file at `/usr/local/bin/dhow-server`, then `systemctl restart dhow`.
   - **Compose:** set `DHOW_IMAGE` in `.env` to the old release, then `docker compose up -d`.
   - **Installer:** run the installer of the old release, for example `bash get.sh --version 1.3.2`.
3. If the state is `incompatible`, stop Dhow, restore the backup you took before the upgrade, then start the old release.
4. Confirm with `dhow-server healthcheck`.

## When a migration stops halfway

- Every migration file runs in its own transaction. If it fails, nothing of it is kept, and the files before it stay applied.
- `NO TRANSACTION` files build indexes with `CREATE INDEX CONCURRENTLY`. If the build stops halfway, PostgreSQL keeps the index as `INVALID`, and `IF NOT EXISTS` does not repair it. So before Dhow runs that file again, it drops any `INVALID` index with that name, then builds it again.
- If Dhow does not start after a failed migration, the reason is in the log. Fix it and restart, or go back to the previous release as in the section above.

## Background backfills

Some changes convert many rows, such as moving sealed values to the format of the [keyring](keys.md#values-saved-before-the-keyring), which is finished and leaves no backfill pending now. This does not happen during the upgrade, so the server is not stopped for a long time. The server converts the rows in the background after it starts: 1,000 rows per batch, with a short pause between batches so requests do not slow down, and under an advisory lock per backfill, so two servers never work on it together. Each batch saves its position in the `"Backfills"` table together with the change itself, so a server stopped in the middle resumes from the last batch and never converts a row twice. A row that is held at the moment of a batch is converted on the next pass.

`dhow-server migrate status` shows every backfill: done, running with how many rows it converted and how many are left, or not started. A row that cannot be converted stays as it is, is named in the server log, and keeps the backfill unfinished until you fix it. A migration that removes something old that depends on a backfill stops with `BACKFILL_NOT_DONE` if that backfill has not finished yet. Wait until `migrate status` says it is done, then upgrade again.

## Several servers on one database

Every server records itself in the `"ServerInstances"` table when it starts: its stable name, the host name, its version, and the last migration it knows. The stable name is `DHOW_INSTANCE_ID`, or the host name when it is not set, and the Compose stack names the container `dhow`. After a restart the server reuses the same row, so the table does not grow. It updates its last-seen time every 30 seconds and marks itself stopped on a clean stop. A server not seen for 90 seconds counts as stopped. `dhow-server migrate status` lists the servers running now, so before an upgrade you know whether an old release still runs.

Because of the `expand` and `contract` rule, the old and the new release can run side by side during an upgrade: upgrade one server, check that it is healthy, then upgrade the rest.

## Notes for particular releases

Ingest tokens made before migration 00078 have no recorded confirmation for protected environments. Dhow refuses the upload of any such token that reaches a protected environment with `403 INGEST_REACH_NOT_ACKNOWLEDGED` until a project admin confirms the reach from the token list. This is intended, because it fails closed. No release was published before this one, so in practice nobody is affected.
