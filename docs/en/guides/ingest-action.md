[العربية](../../guides/ingest-action.md)

# Upload artifacts with `dhow` and the GitHub Action

The [ingest guide](ingest.md) explains how Dhow receives a build from an outside CI. This page covers two tools that do the upload for you: the `dhow artifact push` command, which works in any CI, and the GitHub Action, which uses it for you in GitHub.

## The `dhow artifact push` command

```sh
dhow artifact push \
  --server https://dhow.example.com \
  --name web --ref main --sha "$GIT_SHA" \
  --provenance Provider=jenkins --provenance BuildUrl="$BUILD_URL" \
  -- dist/web
```

The command packs the folder into a `tar.gz` archive with Dhow's own safe rules, or takes a `.tar.gz` file you already built. It then computes the SHA-256, checks the archive with the same rules the server uses, and uploads it to `PUT /ingest/v1/artifacts/{sha256}` with the required headers. Packing gives the same archive, byte for byte, whenever the folder is the same.

### Options

| Option | Meaning |
|---|---|
| `--server URL` | The address of the Dhow server. The variable `DHOW_SERVER` works too. It must start with `https://`. `http://` is accepted for `localhost` only. |
| `--name NAME` | The artifact name, as the pipeline lists it under `ingested`. Required. |
| `--ref REF` | The branch or tag that was built. Required. |
| `--ref-kind KIND` | `branch` (the default) or `tag`. |
| `--sha COMMIT` | The commit that was built, in full lowercase hex. Required. |
| `--provenance K=V` | A text fact about the CI. Repeat the option for each fact. Up to 16 facts, and 256 characters for a value. |
| `--no-trigger` | Stores the file and does not start a run. |
| `--token-stdin` | Reads the token from standard input instead of the variable. |
| `--ca-file FILE` | A PEM file of extra certificate authorities. The variable `DHOW_CA_FILE` works too. |
| `--retries N` | Retries after a temporary failure, from 0 to 10. The default is 3. |
| `--dry-run` | Packs, prints the digest and the headers, and sends nothing. |
| `--format text\|json` | How to print the result. The default is `text`. |
| `--lang en\|ar` | The language of the messages. The default comes from `LC_ALL` or `LANG`. |

### The token

- The token comes from the variable `DHOW_INGEST_TOKEN`, or from standard input with `--token-stdin`. There is no `--token` option, and the command never takes the token on the command line: options show in process lists and in the logs of many CI systems.
- If you type `--token` by mistake, the command refuses and does not print the value.
- The command never prints the token in a message or an error. Even `--dry-run` writes `Authorization: Bearer (hidden)`.
- Keep the token in the secret store of your CI, as in the [ingest guide](ingest.md).

### Try it without sending

```sh
dhow artifact push --dry-run --lang en --name web --ref main --sha 1111111111111111111111111111111111111111 -- dist/web
```

```text
Dry run: nothing was sent.
Digest: sha256:bcf1b360142a05fd4c67a88c9a5ed236bea9b3d8ea8a6c01027d36fb1ccee9df
Size: 100 bytes
  Authorization: Bearer (hidden)
  Content-Length: 100
  Content-Type: application/gzip
  Dhow-Ingest-Name: web
  Dhow-Ingest-Ref: main
  Dhow-Ingest-Ref-Kind: branch
  Dhow-Ingest-Sha: 1111111111111111111111111111111111111111
```

`--dry-run` needs no token and no connection. If you give it `--server`, it also prints the address of the request.

### Repeating and retrying

- An upload that succeeded a second ago and succeeds again is a `200`, not an error, and a repeat never creates a second run. So the command is safe to repeat.
- The command retries on a network break, a timeout, `408`, `429`, `503` and any `5xx`. It waits one second, then two, then four, up to 30 seconds, or it follows the `Retry-After` header when the server sends one.
- If the server asks for a wait longer than two minutes, the command stops, exits with code 4 and prints the wait it was asked for.
- The command does not retry any other `4xx`, because a retry would not change the answer.
- The command never follows a redirect (`3xx`), so the token cannot reach a server you did not name.

### Exit codes

| Code | Meaning |
|---|---|
| `0` | The file is stored (`201`), or it was already stored (`200`). |
| `1` | The server refused the file or its details: `DIGEST_MISMATCH`, `ARCHIVE_UNSAFE`, `ARTIFACT_TOO_LARGE`, `ARTIFACT_QUOTA_EXCEEDED`, `INGEST_TOKEN_QUOTA`, `LENGTH_REQUIRED`, `VALIDATION_FAILED`, `PROJECT_ARCHIVED`, or any unknown `4xx`. |
| `2` | The command or its input is wrong, the folder cannot be read, or the command does not trust the server certificate. Nothing was sent. |
| `3` | The server does not accept the token: `INGEST_TOKEN_INVALID`, `INGEST_TOKEN_REVOKED`, `INGEST_TOKEN_EXPIRED`, or it does not allow it for this ref: `REF_NOT_ALLOWED_FOR_TOKEN`. |
| `4` | A temporary failure was still there after the retries: `NETWORK`, `TIMEOUT`, `UPLOAD_TOO_SLOW`, `RATE_LIMITED`, `SERVICE_UNAVAILABLE`, or any `5xx`. |
| `5` | The answer is not what a Dhow server sends: a page that is not JSON, a redirect, or a different digest in the reply. The command does not retry, because the server may have stored the file. Check `GET /ingest/v1/artifacts/{sha256}`. |

Messages go to `stderr` in your language, with the code in brackets, such as `(REF_NOT_ALLOWED_FOR_TOKEN)`. A successful result goes to `stdout` only. With `--format json`, `stdout` holds one object with `Digest`, `SizeBytes`, `Created`, `Triggered`, `RunId`, `RunNumber`, `RunUrl` and `Attempts`.

### Network and certificates

- The command honours `HTTPS_PROXY` and `NO_PROXY` from its environment.
- The command uses the certificate authorities your system trusts. If your server is behind your company's own authority, pass its PEM file with `--ca-file` or `DHOW_CA_FILE`. It is added to the system authorities and does not replace them.
- There is no option to turn off certificate checking.
- `dhow lint` never uses the network, and that does not change. See the [command line tool](../cli.md).

## The GitHub Action

The Action downloads the `dhow` binary at the version pinned inside it, checks the SHA-256 of its archive before it runs anything, and then calls `dhow artifact push` with the GitHub context.

```yaml
name: Ship
on:
  push:
    branches: [main]
permissions:
  contents: read
jobs:
  build:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1
        with:
          persist-credentials: false
      - run: make build
      - id: dhow
        uses: Khaymah/dhow-action@v1.0.0
        with:
          server: https://dhow.example.com
          token: ${{ secrets.DHOW_INGEST_TOKEN }}
          path: dist/web
          name: web
      - run: echo "Dhow stored sha256:${{ steps.dhow.outputs.digest }}"
```

Pin the Action to a full commit SHA instead of the tag if you want the highest safety. A tag points to one release and never moves, but the commit is the clearest thing to review.

### Inputs

| Input | Meaning |
|---|---|
| `server` | The address of the Dhow server. Required. |
| `token` | The ingest token, from a secret. Required. |
| `path` | A folder to pack, or a ready `.tar.gz` file, relative to the working folder. Required. |
| `name` | The artifact name. Required. |
| `ref` | The branch or tag. The default is `github.ref_name`. |
| `ref-kind` | `branch` or `tag`. The default is `github.ref_type`. |
| `sha` | The commit. The default is `github.sha`. |
| `trigger` | `false` stores the file without a run. The default is `true`. |
| `retries` | Retries after a temporary failure. The default is `3`. |
| `ca-file` | A PEM file of extra certificate authorities. |
| `release-base-url` | Leave it empty. Where to download `dhow` from in tests. The pinned SHA-256 check still runs with it. |

The Action adds these text fields from the GitHub context to `Dhow-Ingest-Provenance`: `Provider` (its value is `github-actions`), `Repository`, `RunId`, `RunAttempt`, `Workflow`, `Actor`, `Event` and `RunUrl`. Hidden characters are removed from them, and each is cut at 200 bytes. Remember that Dhow shows them as declared by the CI and not verified.

### Outputs

| Output | Meaning |
|---|---|
| `digest` | The SHA-256 of the archive in hex. It is the digest Dhow recorded. |
| `size` | The size of the archive in bytes. |
| `created` | `true` for a new file, `false` if Dhow already had this exact upload. |
| `triggered` | `true` if a run was started or already exists for this upload. |
| `run-id` | The id of the run in Dhow, when there is one. |
| `run-number` | The number of the run, when there is one. |
| `run-url` | The address of the run in Dhow, when there is one. |

The Action checks every output before it writes it: the digest is 64 hex characters, the id is a UUID, and the address is `http` or `https` with no control characters. So a strange reply cannot write other outputs into your workflow.

### How the binary is pinned

- The Action carries a `dhow.pin` file with the version and the SHA-256 of the archive for each platform (`linux_amd64`, `linux_arm64`, `darwin_amd64` and `darwin_arm64`). The release workflow writes it from the `SHA256SUMS` it built.
- `scripts/install.sh` downloads the archive from the GitHub release of `Khaymah/dhow` over `https` only, computes its SHA-256, and unpacks it only if it matches the pinned value. If it does not match, the step fails with a message that nothing was run, and the token never reaches any program.
- It then unpacks the `dhow` file alone, checks that it is a regular file and not a link, and checks that `dhow version` reports the pinned version.
- The copy of the Action inside the `Khaymah/dhow` repository has `version=unreleased`, so it fails on purpose. Use `Khaymah/dhow-action` at a release tag.
- The Action runs on Linux and macOS runners, on x64 or ARM64. It needs `curl`, `tar` and `jq`, all of which GitHub-hosted runners have.
- The Action does not check the cosign signature. If you want that, download the archive and check `SHA256SUMS.sigstore.json` as shown in the [command line tool](../cli.md).

### Security

- Run the Action from a `push` event on a branch or tag, or from `workflow_dispatch` or `release`. These events run your own repository's code.
- **Do not use `pull_request_target`.** That event gives repository secrets to a workflow that may run code from a pull request written by a stranger. The Action refuses to run under it and fails with a clear message.
- Repository secrets do not reach pull requests from forks on the `pull_request` event. So `token` is empty and the Action fails, which is what you want.
- Do not upload from `workflow_run` unless you are sure the first workflow built a trusted branch and not a pull request.
- Give the token the narrowest patterns you can (`main` or `release/*`) and a short life, and replace it from time to time. Keeping it in an environment secret protected by a branch rule protects it further.
- The token reaches the command in an environment variable of the upload step alone. It never appears in the command line, and neither the command nor the script prints it.
- Set `permissions: contents: read`. The Action needs nothing more.

### Publishing the Action (for maintainers)

The release workflow copies `apps/github-action` to the `Khaymah/dhow-action` repository on every release tag, after the binaries are published. It writes `dhow.pin` with the SHA-256 of that release, installs the pinned release the way a user's workflow would, and then pushes a commit and a tag of the same name. A tag is never moved, and if the tag exists the job fails.

One-time setup:

1. Create the public repository `Khaymah/dhow-action`.
2. Make a key pair for this purpose alone: `ssh-keygen -t ed25519 -N '' -C dhow-action-mirror -f dhow-action-mirror`.
3. Add `dhow-action-mirror.pub` to `Khaymah/dhow-action` under Settings, then Deploy keys, with Allow write access turned on.
4. Add the content of `dhow-action-mirror` as a secret named `DHOW_ACTION_DEPLOY_KEY` in `Khaymah/dhow`, then delete both files from your machine.

If the secret does not exist, the job is skipped with an `Action not mirrored` notice in the run summary, and the release goes on. The job runs on tags only, with `contents: read`, and the secret reaches only the step that checks it exists and the step that pushes, through an environment variable.
