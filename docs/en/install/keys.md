[العربية](../../install/keys.md)

# Keyring

The keyring is a small file that holds the key encryption keys (KEKs) that protect every secret Dhow stores in the database. It is separate from `DHOW_SECRET_KEY` and never enters the database. The server does not start without it. The decisions and their reasons are in [ADR-0081](../../../Plan/ADR/0081-keyring-and-envelope-encryption.md).

## How the keyring protects secrets

Encryption has two layers:

- **Data key (DEK).** Each owner has its own key of 32 random bytes: the instance itself, each organization, each project, each environment and each user. A value is encrypted with its owner's key: each value gets a fresh subkey derived with HKDF-SHA256 from the data key and 32 random bytes of salt, then AES-256-GCM. A value copied to another row, another column or another owner does not open.
- **Key encryption key (KEK).** The data keys are stored in the `"DataKeys"` table, wrapped with a key encryption key from the keyring. Each key encryption key has a version number, and the database keeps a check value (HMAC-SHA256) for each version, never the key itself.

So a database backup alone opens no secret, and the keyring file alone opens no secret. When an environment or a project is removed, its data key is destroyed in the database, and its values never open from it again. A backup taken before the removal, together with its keyring, still opens those values.

## Where the keyring lives

| Install method | File | How it reaches the server |
|---|---|---|
| [systemd](binary-systemd.md) | `/etc/dhow/credentials/dhow-keyring` (`root:root`, `0600`) | `LoadCredential=dhow-keyring:/etc/dhow/credentials/dhow-keyring` in `dhow.service` |
| [Docker Compose](compose.md) | `./secrets/dhow-keyring` next to `compose.yaml` (owner `65532`, `0600`) | `secrets:` in `compose.yaml`, at `/run/secrets/dhow-keyring` inside the container |
| Anything else | any absolute path | `DHOW_KEYRING_FILE` |

The keyring is never an environment variable, because `docker inspect` and the process environment reveal variables. The server refuses `DHOW_KEYRING`, and refuses `DHOW_KEYRING_FILE` together with the systemd credential.

The file has stricter rules than other secrets:

- A regular file, not a symbolic link, of at most 64 KiB.
- Owned by the user that runs the server, or by root.
- Nobody but its owner can read or write it (`chmod 600` or `0400`). Only with the systemd credential is the group read bit accepted, because systemd shows its ACL that way.

The format is one line per version, at most 16 versions. The key is 32 bytes in unpadded base64url, that is 43 characters:

```text
kek 1 <43 base64url characters>
kek 2 <43 base64url characters>
```

Version numbers rise, have no leading zeros, and fields are separated by exactly one space. A line that does not fit this format, a repeated number, a short key, a key that does not look random, or the same key under two numbers stops the server. The error names the line number and the version, never the key.

## Boot

At start the server reads the keyring and compares it with the database in one transaction:

- Every version in the database whose state is not `retired` must be in the file with the same check value.
- A version in the file that the database does not know is registered as `pending`.
- On a new install, the highest version in the file becomes `active`.

The server also compares the keyring with the database before any migration, so a wrong keyring applies no migration. When any check fails, the server stops with exit status `79` before it listens for any request, and its message names the reason and the version. The systemd unit does not restart on this status. In the container image (`DHOW_ON_INCOMPATIBLE=wait`) the server logs the error once and waits for a stop signal instead of exiting, so the healthcheck keeps failing and the container does not restart in a loop.

| Code | Reason |
|---|---|
| `KEYRING_MISSING` | There is no file, or its path is not set. |
| `KEYRING_INSECURE` | The file is a symbolic link, is open to other users, or belongs to another user. |
| `KEYRING_MALFORMED` | A line does not fit the format; the message names the line. |
| `KEYRING_MISSING_VERSION` | A version the database needs is not in the file. |
| `KEYRING_MISMATCH` | The key in the file does not match the check value in the database: this is not the keyring of this database. |
| `KEYRING_RETIRED` | No version is active and the newest version in the file is retired, for example after restoring an old export. Add a new version with `keys generate`. |

Each server writes the versions it loaded (the numbers only) to `"ServerInstances"."KekVersions"` with its heartbeat.

## Commands

Every command reads the keyring from `--file` when you give it, and otherwise from `DHOW_KEYRING_FILE` or the systemd credential. The commands that need the database read `DHOW_DATABASE_URL` the way the server does. No command ever prints a key.

| Command | What it does | Needs the database |
|---|---|---|
| `dhow-server keys init` | Creates a new keyring with version 1 and mode `0600`. Refuses to overwrite an existing file. | No |
| `dhow-server keys status` | The versions and their states, the number of data keys on each, what is left to re-wrap, the last rotation and the last export. | Yes |
| `dhow-server keys generate` | Appends a line with the next version to the file, keeping its owner and mode. | No |
| `dhow-server keys activate <n>` | Makes version `n` the active one, provided every running server has loaded it. | Yes |
| `dhow-server keys export --recipient age1... --out FILE` | Writes the keyring encrypted to an [age](https://age-encryption.org) public key. `--out -` writes it to standard output. | Yes, to record the export time |
| `dhow-server keys init --discard-unreadable` | Only for a database restored without its keyring. See [The keyring is lost](#the-keyring-is-lost). | Yes |
| `dhow-server keys rekey [--grace 10s]` | Gives every owner a new data key, encrypts every value again, then destroys the old keys. See [Replace the data keys](#replace-the-data-keys). | Yes |

With systemd, run the database commands with the service's own settings:

```sh
sudo systemd-run --pipe --wait --quiet -p User=dhow -p EnvironmentFile=/etc/dhow/dhow.env \
  -p LoadCredential=dhow-database-url:/etc/dhow/credentials/dhow-database-url \
  -p LoadCredential=dhow-keyring:/etc/dhow/credentials/dhow-keyring \
  /usr/local/bin/dhow-server keys status
```

With Compose, run them in a new container from the stack's directory:

```sh
docker compose run --rm --no-deps -T dhow keys status
```

## Rotate the key

Rotation does not stop the server, and reads keep working the whole time, because each data key names the version that wrapped it.

1. Add a new version to the file. The command locks the file (`flock`) so two runs never overlap, writes the line at its end, reads it back, and on any failure truncates the file back to its size before the write. With systemd: `sudo dhow-server keys generate --file /etc/dhow/credentials/dhow-keyring`. With Compose:

   ```sh
   docker compose run --rm --no-deps -T --volume "$PWD/secrets/dhow-keyring:/keyring/dhow-keyring" dhow keys generate --file /keyring/dhow-keyring
   ```

2. If you run more than one server, copy the same file to all of them.
3. Restart each server, one at a time. Each server registers the new version as `pending` and wraps nothing with it yet.
4. Run `dhow-server keys activate <n>`. If a running server (heartbeat in the last 90 seconds) has not loaded version `n`, the command refuses with `KEK_NOT_EVERYWHERE`, names that server and changes nothing. Otherwise `n` becomes `active`, the previous version becomes `retiring`, and every new data key is wrapped with `n`.
5. The server re-wraps the old data keys in the background: 500 per batch, under an advisory lock so two servers never work on them at once. If the server stops halfway, it resumes where it stopped after the restart. The values themselves do not change. A data key the keyring cannot unwrap is recorded in `keys status` and `doctor` and skipped, so it does not stop the rest, but it keeps its version from retiring.
6. When no data key is left on the old version, it becomes `retired` on its own. `keys status` says its line can be removed from the file, only when it wraps no data key at all, but remove it only after every backup taken before the rotation is gone, because each backup needs the keyring it was taken with.

### What rotation protects and what it does not

Rotation re-wraps the data keys only and does not change them, so it protects against a leaked keyring only after the old key is gone from every keyring file and every database backup taken before the rotation is gone. A leaked keyring together with a database backup from before the rotation unwraps data keys that still protect the current values. For the same reason, destroying an environment's data key does not stop a backup taken before the destroy, together with its keyring, from opening its values. The full fix after a keyring leak is a rotation followed by [Replace the data keys](#replace-the-data-keys). The values that were in the leaked copy itself are exposed anyway, so the owner of each secret should still change its value.

## Replace the data keys

`dhow-server keys rekey` gives every owner a new data key, encrypts every stored value again with the new key of its owner, then destroys the old keys. After it, the old keyring together with a database copy from before the command opens no current value. The decisions and their reasons are in [ADR-0082](../../../Plan/ADR/0082-sealer-users-move-to-the-envelope.md).

1. If the keyring leaked, the order is `keys generate`, then copy the file and restart every server, then `keys activate`, then `keys rekey`, as in [Rotate the key](#rotate-the-key). This way the new data keys are wrapped with a version the leaker does not have. If the active version was activated more than an hour before the command, the command prints a warning (`WARNING`) with this order.
2. Run the command with the settings of the service:

   ```sh
   sudo systemd-run --pipe --wait --quiet -p User=dhow -p EnvironmentFile=/etc/dhow/dhow.env \
     -p LoadCredential=dhow-database-url:/etc/dhow/credentials/dhow-database-url \
     -p LoadCredential=dhow-keyring:/etc/dhow/credentials/dhow-keyring \
     /usr/local/bin/dhow-server keys rekey
   ```

3. The command runs while the server runs. Reads never stop, because an old key stays readable in the `superseded` state until all its values have moved, and a new value is encrypted with the new key at once. After the first pass the command waits for `--grace` (10 seconds by default, and at least 5 seconds) so every write that started with an old key finishes, then counts the values left and passes over them again, then destroys the old keys.
4. The command runs under an advisory lock, so a second run at the same time stops with `REKEY_RUNNING`. If it stops halfway, run it again and it resumes where it stopped. If it finds a value on an old key that does not open, it stops with `REKEY_UNREADABLE`, names the column and the row, and does not destroy the key that value uses. Fix or clear the value, then run the command again.
5. The command writes `system.keys.rekey_started` to the audit log when it starts, then `system.keys.rekeyed` when it ends or `system.keys.rekey_failed` when it stops, with high severity and counts only.
6. The command needs a direct connection to PostgreSQL, because its lock is tied to the session. Do not run it through PgBouncer in transaction mode, where the session changes between statements.
7. The command changes the keys only, not the secret values themselves. Replacing a secret that leaked is a separate job: change it where it comes from, then save it in Dhow again.

Every encrypted column is registered in the code as a value location: the table and column, the kind of owner, the label, and the context that binds the value to its row. The command passes over these locations only, and the tests check that every encrypted column in the database is registered.

## Values saved before the keyring

Before the keyring, values were encrypted with a key derived from `DHOW_SECRET_KEY`, in a format whose first byte is `0x01`. New values use the keyring format (`0x03`). The old format is gone: the server reads and writes the keyring format only, and refuses any value whose first byte is not `0x03`. Each of the following columns also has a CHECK constraint in the database that blocks writing any other format:

| Column | Owner |
|---|---|
| `"TotpFactors"."SecretSealed"` | The user |
| `"InstanceSettings"."Secrets"` | The instance |
| `"OidcProviders"."Secret"` | The instance |
| `"Repositories"."DeployKeySealed"` and `"PreviousDeployKeySealed"` | The project |
| `"Repositories"."WebhookSecretSealed"` | The project |
| `"SecretVersions"."ValueSealed"` | The organization, project or environment |
| `"RegistryCredentials"."PasswordSealed"` | The organization or project |
| `"GitConnections"."TokenSealed"` | Not used yet: nothing writes it before GitHub and GitLab connections arrive |

Stop every server of the older release before you upgrade, so that no server writes a value in the old format while the migration runs. The migration that adds these constraints stops with `BACKFILL_NOT_DONE` if a column still holds a value in the old format whose conversion has not finished. In that case run the previous Dhow release until `dhow-server migrate status` says the conversion is done, then upgrade again. If the conversion is done and a value that cannot be opened is left, the next migration stops at that constraint, so delete that value or save it again, then restart.

Now `DHOW_SECRET_KEY` opens no stored value and only derives signing and HMAC keys. Changing it does not touch the stored secrets, but every runner must register again, current API tokens and recovery codes stop working, and OIDC sign-ins that started before it and temporary artifact download links fail. So change it only if it leaked.

## Backups and export

- **With the backup.** With systemd, the copy of `/etc/dhow` holds the keyring. With Compose, `.env` alone is not enough: take the `./secrets/` directory too. A `pg_dump` without the keyring opens no secret.
- **Export.** Keep a copy of the keyring away from the host, encrypted to an age public key whose private key the server does not have:

  ```sh
  age-keygen -o dhow-keyring-recovery.key
  dhow-server keys export --recipient "$(age-keygen -y dhow-keyring-recovery.key)" --out dhow-keyring.age
  ```

  Generate the age key on your own machine, not on the server, and pass only the public key. The server refuses a private key if you pass one by mistake. Dhow records the time of the last export, and `doctor` warns when the keyring was never exported.
- **Restore.** Restore the database and the keyring from the same backup. At start the server compares the check values in the database with the keyring and stops with `KEYRING_MISSING_VERSION` or `KEYRING_MISMATCH` when they do not match, so it never runs on broken data. To open an export: `age --decrypt -i dhow-keyring-recovery.key dhow-keyring.age > dhow-keyring`, then `chmod 600 dhow-keyring`.

## Emergency guide

### The database leaked

The values in the database are encrypted with data keys wrapped by the keyring, and the keyring is not in the database. Nothing about the keys needs to change in a hurry, but find out who reached the copy, and rotate `DHOW_SECRET_KEY` if it leaked with it.

### The keyring file leaked

The file alone opens nothing without the database, but together with any database copy the leaker has, it opens everything. Rotate the key at once as in [Rotate the key](#rotate-the-key), then run [Replace the data keys](#replace-the-data-keys), because rotation alone does not change the data keys (see [What rotation protects and what it does not](#what-rotation-protects-and-what-it-does-not)). Wait until the old version is `retired`, then remove its line from the file and from every copy, and delete every database backup taken before the rotation. Change the secret values that were stored at the time of the leak.

### The keyring is lost

If the database came back without its keyring and there is no export, the stored secrets are gone, and nothing can open them. The server refuses to start. To run it again:

```sh
dhow-server keys init --discard-unreadable
dhow-server keys init --discard-unreadable --confirm destroy-1234-keys
```

The first command shows what will be lost: the missing or different versions, the number of data keys, and the `--confirm` value that holds exactly that number, and changes nothing. When it finds no file it creates a new keyring, unless you gave `--file`, in which case it stops. The second refuses while a running server still holds those versions, because its keyring may be the right one, and refuses if the number changed. Otherwise it marks those data keys destroyed, so all their values count as erased, and writes a high-severity entry to the audit log (`system.keys.discarded`) in the same transaction, so the discard never happens without its entry. Then enter the secrets again, and ask people to set up their authenticator app again.

## Status

- **`/api/v1/health/ready`** has the `keyring` check: it is healthy while the keyring the server loaded holds every `active` and `retiring` version with the same check value. The public answer never names the reason; the reason is in the server log.
- **`GET /api/v1/system/keys`** shows the instance owner only (`instance.settings.manage`) the versions, their states, the number of data keys, the last rotation and the last export, and whether each version is loaded on the server that answered. It never shows a key or a check value. Rotation happens on the command line only, because it needs the file.
- **`dhow-server doctor`** checks the file's owner, mode and format, compares the check values with the database, and warns when the keyring was never exported, the active version is more than a year old, or some data keys cannot be unwrapped with the keyring. It never makes a new keyring, and it refuses a symbolic link, like the server.
