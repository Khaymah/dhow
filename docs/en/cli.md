[العربية](../cli.md)

# The `dhow` CLI

`dhow` is a single static binary for Linux, macOS and Windows, on `amd64` and `arm64`. Its first command is `dhow lint`: it checks a `.dhow.yml` file with the same validator as the Dhow server (`internal/pipelinespec`), so a file it accepts is a file the server accepts. It works offline: it never opens a network connection and never contacts a server. The second command is `dhow artifact push`: it uploads a build to a Dhow server ([guide](guides/ingest-action.md)). It is the only command that uses the network.

## Install

### With get.sh (Linux)

Download it, read it, then run it. Never pipe it straight into a shell:

```sh
curl -fsSLO https://raw.githubusercontent.com/Khaymah/dhow/main/deploy/install/get.sh
less get.sh
bash get.sh --component cli
```

`--version 1.2.3` picks a release, and the latest is the default. The script:

1. downloads `dhow_<version>_linux_<arch>.tar.gz`, `SHA256SUMS` and `SHA256SUMS.sigstore.json` from the GitHub release;
2. checks the cosign signature on `SHA256SUMS` against the release workflow of `Khaymah/dhow` (keyless, through Sigstore) when `cosign` is installed, and warns that it checks the checksum only when it is not;
3. checks the archive against its line in `SHA256SUMS`, and stops when the line is missing or does not match;
4. installs `/usr/local/bin/dhow` (`0755`, owned by root).

Install `cosign` first to get the signature check: without it, the checksum only proves the download is not corrupt, not who built it. With a mirror (`DHOW_DOWNLOAD_URL` or `DHOW_REPO`), cosign is required, as in the [installer guide](install/installer.md). Run as a normal user, the script copies the files to a folder only root can write, and checks and unpacks them there before it installs anything with `sudo`.

### By hand (macOS, Windows and Linux)

Every release has one archive per system: `dhow_<version>_<os>_<arch>.tar.gz`, where `<os>` is `linux`, `darwin` or `windows`. The Windows archive holds `dhow.exe`, and `tar` on Windows 10 and later unpacks it. Download the archive, `SHA256SUMS` and `SHA256SUMS.sigstore.json` from the release page, then run these commands with the exact version and system of your archive (here 1.2.3 on an Apple silicon Mac):

```sh
cosign verify-blob --bundle SHA256SUMS.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github\.com/Khaymah/dhow/\.github/workflows/release\.yml@refs/tags/v' \
  SHA256SUMS
sha256sum --check --ignore-missing SHA256SUMS
tar -xzf dhow_1.2.3_darwin_arm64.tar.gz
sudo install -m 0755 dhow /usr/local/bin/dhow
dhow version
```

On a Mac without `sha256sum`, check the line of your archive with `shasum`: `grep dhow_1.2.3_darwin_arm64 SHA256SUMS | shasum -a 256 -c`. The macOS binaries are not codesigned or notarized by Apple yet. If Gatekeeper refuses to open `dhow`, check the signature and the checksum as above first, then remove the quarantine flag: `xattr -d com.apple.quarantine /usr/local/bin/dhow`. Each archive also has an SPDX SBOM (`<archive>.spdx.json`) and a SLSA build provenance attestation on the release page.

### From source

```sh
make build-cli
./bin/dhow version
```

## `dhow lint`

```sh
dhow lint
dhow lint path/to/.dhow.yml
dhow lint path/to/repo
dhow lint --format json --lang en .dhow.yml
```

Without a path, it checks `.dhow.yml` in the current folder. A folder means the `.dhow.yml` in it. It reads at most 256 KiB, the limit of the validator, and refuses anything that is not a regular file (a FIFO or a device, for example) without reading it. It never follows a symbolic link: if `.dhow.yml` is a link, it stops with exit code 2, so a pull request cannot point it at another file on the machine (such as `~/.docker/config.json`) and get its key names into a public CI log.

| Option | Meaning |
|---|---|
| `--format text` | Readable lines, one per problem, then a summary. The default. |
| `--format json` | One JSON document for editors and scripts (see below). |
| `--lang en` or `--lang ar` | The language of the messages. By default it comes from `LC_ALL`, `LC_MESSAGES` or `LANG`: Arabic for an `ar_*` UTF-8 locale, English otherwise. |
| `--help` | Shows the commands and the exit codes. |

Options may come before or after the path. Write `--` before a path that starts with `-`.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | The file is valid. It may have warnings, such as an image pinned by tag only. |
| `1` | The file has at least one error, and the server refuses it. |
| `2` | The file does not exist, is a symbolic link, is not a regular file or cannot be read, or the command is wrong (an unknown option, a bad value, more than one path). Nothing is printed on stdout. |

## Output

The text output has one line per problem: the file, the line and the column, the severity, the code, the path in the file and the message. The last line is a summary:

```text
.dhow.yml:7:5: warning IMAGE_NOT_PINNED at jobs.build.image: the image is not pinned by digest: the runner refuses it unless unpinned images are allowed
.dhow.yml:9:9: error RUN_EXPRESSION_NOT_ALLOWED at jobs.build.steps[0].run: ${{ }} is not allowed inside run: pass the value through env and read it as a variable
.dhow.yml: errors: 1, warnings: 1. The server refuses this pipeline.
```

Control characters (C0 and C1) and invisible direction characters in a file name or a key are printed as escapes (`\u001b`, `\u009b`, `\u202e`), and a backslash as `\\`, in the text and in the JSON output, so a file from a repository cannot move the cursor or reorder the line in your terminal.

`--format json` prints one document. Its shape is stable within `SchemaVersion` 1: new fields may be added, and none is renamed or removed:

```json
{
  "SchemaVersion": 1,
  "File": ".dhow.yml",
  "Lang": "en",
  "Valid": false,
  "Errors": 1,
  "Warnings": 0,
  "Issues": [
    {
      "Code": "RUN_EXPRESSION_NOT_ALLOWED",
      "Severity": "error",
      "Path": "jobs.build.steps[0].run",
      "Line": 9,
      "Column": 9,
      "Message": "${{ }} is not allowed inside run: pass the value through env and read it as a variable"
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `Code` | The stable `UPPER_SNAKE` code, the same as the server's. Match on this, never on the message. |
| `Severity` | `error` or `warning`. |
| `Path` | Where the problem is (`jobs.build.steps[0].run`), or empty for the whole file. |
| `Line`, `Column` | The position in the file, from 1, or 0 when there is none. |
| `Message` | The message in `Lang`. The English one is the server's own. It never repeats a value from the file. |

`Issues` is `[]` for a valid file. The codes are listed in the [pipeline reference](pipeline-spec.md#11-validation), and the golden files in `apps/cli/internal/App/testdata` show the exact output.

## Offline

`dhow lint` never uses the network. It does not check plugin manifests and does not fetch images, so `uses:` and `image:` are checked by their form only (a digest is required for `uses:`). Three tests keep it that way:

- the command links only the packages listed in `apps/cli/cmd/dhow/testdata/deps.linux-amd64.txt`, so any new package fails the test until someone reviews it. Since ADR-0099 the network packages (`net`, `net/http` and `crypto/tls`) are in that list because `dhow artifact push` needs them, and only one package of our code imports them: `internal/ingestpush`, which a test checks. `os/exec` is not in the list, and the binary has no `os.StartProcess`, `syscall.ForkExec` or `syscall.Exec`;
- the binary runs every invalid file of the validator tests in a network namespace without any network, under `strace`, and the trace has no network system call and no `execve` after the first, while the result is the same as outside. A probe that opens a socket in the same setup shows that `strace` would see one. The same test runs on `dhow artifact push --dry-run`, so it makes no connection before it sends;
- a project hook refuses any import of the network packages, of `syscall`, `golang.org/x/sys`, `unsafe` and cgo, and any call to `os.StartProcess`, `syscall.Exec` or `syscall.ForkExec` under `apps/cli`. So the command code in `apps/cli` never imports the network itself, only through `internal/ingestpush`.

## Other commands

| Command | Meaning |
|---|---|
| `dhow artifact push` | Packs a folder or takes a `.tar.gz` and uploads it to a Dhow server with an ingest token. See [uploading artifacts](guides/ingest-action.md). |
| `dhow version` | Prints the version, the commit and the Go version. |
| `dhow help` | Shows the commands and the exit codes. |
