[العربية](../../guides/ingest.md)

# Receive artifacts from an outside CI

If you build in another CI, for example GitHub Actions, you can hand the finished build to Dhow and let Dhow deploy it. Dhow stores the archive by its SHA-256 digest, starts a run, and the deploy job gets the same digest. A deploy or a rollback never builds again.

## How it works

1. A Maintainer (or Owner or Admin) makes an **ingest token** in the project settings. The token reaches one project, only the refs you list, and ends after 90 days unless you choose another time (up to 365 days).
2. The outside CI packs its output as a `tar.gz` and uploads it with the token.
3. Dhow checks the digest and the archive while it receives it. Nothing is kept if a check fails.
4. Dhow starts a run from the **default branch** pipeline, because it must not let an outside CI write the pipeline. The run has the ref and commit the uploader declared.
5. A job with `ingested: [web]` gets the archive unpacked in `/workspace` before its first step.

## The token

- It looks like `dhow_ing_` and 43 more characters. It is shown **once**, when you make it. Dhow keeps only a hash, so a lost token is replaced, not read again.
- Only a session can make or revoke a token, with a fresh second factor. An API token cannot.
- The token allows nothing but the upload and the status of an upload. It reads no project data.
- A ref pattern has a kind (`branch` or `tag`) and a name pattern: `main`, `release/*` (one part), `release/**` (all parts) or `v*`.
- **Protected reach:** when you make a token, Dhow shows `Reaches`: the environments whose allowed refs could match the token's patterns. The check is conservative, so a pattern such as `**` reaches every environment that allows a branch. If the token reaches an environment that has deploy targets or reviewers, the request needs `AcknowledgeProtectedReach: true`, or the answer is `422 INGEST_TOKEN_REACH_UNCONFIRMED`. Narrow the patterns when you can.
- **The ref and the commit are claims.** The uploader says which ref and commit it built, and Dhow checks only that the ref fits the token's patterns. `ProvenanceVerified` stays `false` until a later release verifies the CI identity (GitHub OIDC). So a stolen token can start a run, with the environment's rules, as any ref its patterns allow. That is why patterns, the acknowledgement above, and approvals on protected environments matter.
- A token uploads at most 10 GiB in total. Revoking it does not free the quota of the organization: the files stay until they expire. There is no purge call yet.
- Revoke a token at once from the same page. The next upload with it is `401 INGEST_TOKEN_REVOKED`.
- Keep the token in your CI secret store. Never write it in a file of the repository, never print it, and never give it to a job that runs code from pull requests (for example `pull_request_target` in GitHub).

## Upload with curl

In GitHub, use the ready-made Action. In any other CI, use the `dhow artifact push` command: it packs the folder, computes the digest, retries safely and explains every refusal. Both are described in [uploading with the command and the Action](ingest-action.md). The example below shows the protocol itself, for anyone who wants to write their own tool.

```sh
digest=$(sha256sum build.tar.gz | cut -d' ' -f1)
curl --fail-with-body --request PUT \
  --header "Authorization: Bearer $DHOW_INGEST_TOKEN" \
  --header "Content-Type: application/gzip" \
  --header "Dhow-Ingest-Name: web" \
  --header "Dhow-Ingest-Ref: main" \
  --header "Dhow-Ingest-Sha: $GIT_SHA" \
  --data-binary @build.tar.gz \
  "https://dhow.example.com/ingest/v1/artifacts/$digest"
```

Put the token in a header file or an environment variable your CI hides, not in a command line that its log prints.

| Header | Needed | Meaning |
|---|---|---|
| `Authorization` | yes | `Bearer` and the ingest token |
| `Content-Length` | yes | The size of the archive. Without it the answer is `411` |
| `Dhow-Ingest-Name` | yes | The artifact name: letters, digits, `.`, `_` and `-`, up to 64 |
| `Dhow-Ingest-Ref` | yes | A branch or tag name, or the full ref |
| `Dhow-Ingest-Ref-Kind` | no | `branch` (the default) or `tag` |
| `Dhow-Ingest-Sha` | yes | The commit that was built. It is a hint: Dhow does not check it against the repository |
| `Dhow-Ingest-Trigger` | no | `false` stores the file without starting a run |
| `Dhow-Ingest-Provenance` | no | A small JSON object of text values (up to 16 fields, 4 KiB) about the CI, such as `Provider`, `RunUrl` and `Actor`. Dhow shows it as declared by the CI |

The answer is `201` for a new file and `200` when the same file (same project, digest, name, ref and commit) was already stored. A repeat creates no second run. `GET /ingest/v1/artifacts/{sha256}` with the same token tells where the file went.

| Code | Status | Why |
|---|---|---|
| `INGEST_TOKEN_INVALID` | 401 | No token, or a token Dhow does not know |
| `INGEST_TOKEN_REVOKED` | 401 | The token was revoked |
| `INGEST_TOKEN_EXPIRED` | 401 | The token expired |
| `REF_NOT_ALLOWED_FOR_TOKEN` | 403 | The ref matches none of the token's patterns |
| `DIGEST_MISMATCH` | 422 | The bytes do not match the digest in the address |
| `ARCHIVE_UNSAFE` | 422 | Not a `tar.gz`, or it holds a path with `..`, an absolute path, a link that leaves the folder, or a device |
| `ARTIFACT_TOO_LARGE` | 413 | Larger than `DHOW_INGEST_MAX_BYTES` |
| `ARTIFACT_QUOTA_EXCEEDED` | 413 | The artifact quota of the organization is full |
| `LENGTH_REQUIRED` | 411 | No `Content-Length` |
| `UPLOAD_TOO_SLOW` | 408 | The upload took longer than its deadline (at least 2 minutes, and 256 KiB per second of its size, at most 30 minutes) or fell below 256 KiB per second |
| `INGEST_TOKEN_QUOTA` | 413 | This token already uploaded 10 GiB. Revoke it and make a new one |
| `RATE_LIMITED` | 429 | More than 30 uploads in an hour for this token |
| `SERVICE_UNAVAILABLE` | 503 | This token already has an upload running, the organization has two, or the server has four. Try again after `Retry-After` |

## Start a run

Add `artifact` to `on`, and `ingested` to the job that needs the files:

```yaml
version: 1
on:
  artifact:
    names: [web]
    refs: [main, "release/**"]
jobs:
  ship:
    image: alpine@sha256:61bb7a95b0f3650f3039c0fcabe07674458cf0d13afa51aade6188bab8215b07
    ingested: [web]
    steps:
      - run: ls
```

- Dhow reads this file from the default branch, so the file must be there before the first upload. If that file has no matching `on.artifact`, the file is stored and the answer says `NO_ARTIFACT_TRIGGER`.
- If Dhow does not know the head of the default branch yet (no push reached it), the file is stored and no run starts (`DEFAULT_BRANCH_UNKNOWN`).
- A job whose `ingested` name did not arrive in this run is skipped.
- A run of an ingested file goes through the same gate as any run: the `AllowedRefs` of an environment use the ref the uploader declared, and an environment with reviewers waits for approval.
- A run from an upload never posts a commit status to your Git host, because the commit is only declared.

## Rollback

An ingested file that a deploy used stays while that release is among the last `keep_releases` of the target, up to 90 days after its normal time. So a rollback can use it without a new build.

## Reverse proxy and size

**Set `DHOW_TRUSTED_PROXIES` first.** The limits per address use the client address Dhow reads from the proxy. Without the right trusted proxies every upload looks like it comes from the proxy, and one noisy client uses the limit of all ([trusted proxies](../install/reverse-proxy.md#trusted-proxies)).

Dhow cuts an upload that is slower than 256 KiB per second after 15 seconds, or that takes longer than its deadline. Uploads go to `/ingest/`. Put the larger body limit on that path in your reverse proxy ([reverse proxy](../install/reverse-proxy.md)) and set `DHOW_INGEST_MAX_BYTES` ([configuration](../install/configuration.md)).
