[العربية](../pipeline-spec.md)

# Pipeline spec — `.dhow.yml` v1

Status: **v1, validated.** The Go validator in `internal/pipelinespec` is the source of truth, and the server, the runner and the CLI (`dhow lint`) all use it. The JSON Schema in `packages/spec/dhow.schema.json` is for editors (VS Code), and a test keeps it in step with the validator. How the server runs these files is in §12.

## 1. Principles

- **One source of truth.** The pipeline lives in the repository at the commit being deployed (ADR-0008).
  - For privileged events (deploys and PRs from forks), the definition is read from the **base/default branch**, so a PR cannot rewrite its own permissions.
- **Steps are sandboxed.** A step is either a `run:` (a script for a shell *inside the job sandbox*) or a `uses:` (a container-image plugin).
- **Expressions are CEL** inside `${{ … }}`. They are typed, non-Turing-complete and have cost limits.
  - Values from Git (commit message, author, branch) are **data** and are never re-parsed as expressions.
- **Secrets are referenced, not embedded.** `secrets.NAME` resolves only within the job's environment scope.
- **Plain `env` values are not secret.** Every member who can read the job sees them in the API and the UI, viewers included, so a token, password or key never goes in `env`: reference it from `secrets` instead.
- **The file is read exactly as written.** Anchors, aliases, merge keys (`<<`) and custom tags are refused, so the file you review in a PR is the file that runs.

## 2. Full example

```yaml
version: 1
name: web-app

on:
  push:
    branches: [main]
  tag:
    patterns: ["v*"]
  pull_request: {}            # fork PRs: no secrets, no deploy environments
  manual:
    inputs:
      dry_run:
        type: boolean
        default: false

defaults:
  runner:
    labels: [linux, amd64]
    isolation: container        # the only level in the first release
  timeout: 20m

concurrency:
  group: ${{ project.name + "-" + git.ref }}
  cancel_in_progress: true

jobs:
  test:
    image: node:22-bookworm@sha256:61bb7a95b0f3650f3039c0fcabe07674458cf0d13afa51aade6188bab8215b07
    strategy:
      matrix:
        node: ["20", "22"]
    env:
      NODE_MAJOR: ${{ matrix.node }}
    steps:
      - name: Install
        run: npm ci --cache .npm
      - name: Test
        run: npm test -- --ci
    cache:
      key: ${{ "npm-" + matrix.node + "-" + hash_files("package-lock.json") }}
      paths: [.npm]

  build:
    needs: [test]
    image: node:22-bookworm@sha256:61bb7a95b0f3650f3039c0fcabe07674458cf0d13afa51aade6188bab8215b07
    network:
      egress: [registry.npmjs.org]      # default is deny-all
    resources:
      cpu: 2
      memory: 4Gi
    steps:
      - run: npm ci && npm run build
    artifacts:
      - name: web
        paths: [dist/, package.json, package-lock.json]

  deploy-staging:
    needs: [build]
    if: ${{ git.branch == "main" && !inputs.dry_run }}
    environment: staging
    steps:
      - uses: ghcr.io/khaymah/dhow-deploy-ssh@sha256:03fddb3b486b758f338f8999afbf04ced985d74825963dd28b92e650645e6778
        with:
          artifact: web
          target: staging-vps          # defined on the environment, credentials are env secrets
          strategy: atomic-symlink
          keep_releases: 5
          health_check_url: https://staging.example.com/healthz

  deploy-production:
    needs: [deploy-staging]
    if: ${{ git.is_tag && git.tag.startsWith("v") }}
    environment: production            # protection rules enforced server-side (see §9)
    steps:
      - uses: ghcr.io/khaymah/dhow-deploy-ssh@sha256:03fddb3b486b758f338f8999afbf04ced985d74825963dd28b92e650645e6778
        with:
          artifact: web
          target: prod-fleet
          strategy: atomic-symlink
          health_check_url: https://example.com/healthz
      - uses: ghcr.io/khaymah/dhow-notify@sha256:9b0fdebd63cc544675f3d2a9b00f975ebc573d95b42602523ea996b62c3b6a68
        with:
          channel: release-room
          message: ${{ "Released " + git.tag + " to production" }}

  migrate-db:
    needs: [deploy-production]
    image: ghcr.io/example/migrate@sha256:a8cf72fa2a047a9d6be9b105fbece66e00460b8a9fa188c439d1559100330c4d
    environment: production
    steps:
      - run: ./bin/migrate up
        env:
          DATABASE_URL: ${{ secrets.DATABASE_URL }}
```

## 3. Reference (v1)

| Key | Type | Notes |
|---|---|---|
| `version` | int | Must be `1` |
| `name` | string | Optional, at most 100 characters |
| `on` | map | `push` and `pull_request` (`branches`, `paths`), `tag` (`patterns`), `manual` (typed `inputs`: `string`, `boolean` or `choice` with `options`, at most 20) and `api`. At least one trigger. `schedule` and `artifact` are refused for now (§8) |
| `on.*` filters | list | Glob patterns with `*` (inside one path segment), `**` (a whole segment, any depth) and `?` only, at most 50 per list |
| `defaults.runner` | map | `labels` (at most 32) and `isolation` (`container` only in the first release, §8) |
| `defaults.timeout` | duration | The job timeout when a job does not set one (otherwise 1h) |
| `concurrency` | map | `group` (text or CEL `string`), `cancel_in_progress` |
| `jobs.<id>` | map | The id is a lowercase letter followed by up to 62 of `a-z`, `0-9`, `_` and `-`. At most 64 jobs |
| `jobs.<id>.needs` | list | DAG edges; unknown jobs and cycles are rejected at validation time. A need on a matrix job waits for every copy |
| `jobs.<id>.if` | CEL bool | Evaluated before the job is queued; `false` marks it `skipped` |
| `jobs.<id>.strategy.matrix` | map | Cartesian product with `include` and `exclude` (§6); `strategy.max_parallel` from 1 to 256 |
| `jobs.<id>.image` | string | Required when the job has a `run:` step. Should be digest-pinned: a tag-only image is a warning, and the server pins the tag to a digest when the run starts (§12.5). A name without a registry host is completed the way Docker does it: `node:22-bookworm@sha256:…` runs as `docker.io/library/node:22-bookworm@sha256:…`. The runner never searches other registries. |
| `jobs.<id>.runner` | map | Same keys as `defaults.runner`; its `labels` replace the defaults |
| `jobs.<id>.timeout` | duration | From `1s` to `24h`, for example `90s`, `20m` or `1h30m` (units `d`, `h`, `m`, `s`, largest first) |
| `jobs.<id>.environment` | string | Binds protection rules, secrets and targets. Accepted now with a warning; until M5 the job is skipped |
| `jobs.<id>.network.egress` | list | Host allowlist; empty means deny-all, at most 64 entries. Each entry is one of `host`, `host:port` or `*.domain`. (1) **Hosts:** lowercase ASCII, with punycode (`xn--…`) for other scripts. IP literals, all-numeric names and single-label names (`localhost`, `*.com`) are refused. `*.domain` matches any subdomain but not `domain` itself. (2) **Ports:** without a port, an entry opens 443 through `CONNECT` and 80 for plain `http://`. (3) **Proxy:** traffic goes through the runner's proxy on `127.0.0.1:3128` (`HTTPS_PROXY`, `HTTP_PROXY` and `ALL_PROXY` are set for every step and override the job's own values), so clients must be proxy-aware. SSH and other TCP clients can use `ProxyCommand=/run/dhow/bin/connect %h %p`. (4) **Not supported:** UDP, QUIC and programs that ignore proxies. (5) **Always denied:** private, loopback, link-local and metadata addresses (ADR-0044, ADR-0057) |
| `jobs.<id>.resources` | map | `cpu` (cores like `2` or `1.5`, or millicores like `1500m`; 0.1 to 64), `memory` and `disk` (`Ki`, `Mi`, `Gi` or `Ti`, for example `4Gi`; memory 64Mi to 64Gi, disk 64Mi to 100Gi) and `pids` (32 to 32768). What is not set uses the server default (1 CPU, 2Gi memory, 4Gi disk, 1024 pids) |
| `jobs.<id>.env` | map | At most 100 variables. Names are letters, digits and `_`, and names that start with `DHOW_` are reserved. Each value is text or one whole CEL `string` expression |
| `steps[].id` | string | Optional, unique in the job, same rules as a job id |
| `steps[].if` | CEL bool | Evaluated before the job is queued; `false` leaves the step out |
| `steps[].run` | string | Executed by `/bin/sh -e` **inside the sandbox**; the script is passed on stdin, and the image must provide the shell. At most 64 KiB, and `${{ }}` is refused (§5) |
| `steps[].uses` | image ref | Plugin; must be pinned by digest (`name@sha256:…`), or the file is refused |
| `steps[].with` | map | Plugin inputs, only on a `uses:` step: at most 32, names in lowercase `a-z`, `0-9` and `_`, values up to 32 KiB (text or CEL `string`). The plugin manifest declares the inputs, and the runner checks them before the plugin starts ([Plugins](#plugins)) |
| `steps[].env` | map | Same rules as `jobs.<id>.env`; values may reference `secrets.*` and `vars.*` |
| `steps[].timeout` | duration | Not longer than the job timeout |
| `artifacts` | list | At most 16 per job. `name` (letters, digits, `.`, `_`, `-`, up to 64, unique in the pipeline: in a matrix job, join text and `matrix.*` values with `+`, like `${{ "web-" + matrix.os }}`), `paths` (1 to 32, relative to the workspace, without `..`, `~` or a leading `/`) and `retention` (`1d` to `30d`, default 30 days) |
| `cache` | map | `key` (text or CEL `string`, may call `hash_files()`), `paths` (same rules as artifact paths); scoped per project and branch, read-only for fork PRs |

**CEL context.** These variables are available:
- `git.{sha, ref, branch, tag, is_tag, message, author, event}` and `git.pr.{number, base_ref, head_ref, is_fork}`
- `project.{id, name}` and `env.*` (the job's `env`, in step fields and the cache key)
- `vars.*` (non-secret)
- `inputs.*` (the declared `manual` inputs, with their types)
- `secrets.*` (only as a whole `env`/`with` value; it can never appear in an `if`)
- `matrix.*` and `needs.<job>.outputs.*` (only for jobs listed in `needs`, and only when the job id has no `-`)

These functions are available: `hash_files()` (cache key only, evaluated by the runner inside the workspace), `startsWith()`, `endsWith()`, `contains()`, `size()`, and the rest of the CEL standard library, except `matches()` (regular expressions cannot be stopped in the middle) and the time functions (`timestamp()`, `duration()` and the `get…()` functions, which can read time zone files).

The values in the context have limits, and an evaluation with a larger value fails: `git.message` up to 64 KiB, the other `git.*` and `project.*` values up to 1 KiB, `vars.*` and `inputs.*` up to 4 KiB (256 variables), `env.*` up to 32 KiB (100 variables), and each `needs` output up to 1 KiB (64 outputs). An `env` name that holds a secret is never visible in `env.*`.

### The checkout

A job of a pipeline starts with a checkout of the run's commit into `/workspace`. You write nothing for it in `.dhow.yml`.

- **Where it runs:** in a sandbox of its own, from the `dhow-git` image that the runner pins by digest. That sandbox can reach the repository host and nothing else, whatever the job's `network.egress` says.
- **What it fetches:** the run's ref with `--depth=1`, then exactly the run's commit. If the branch moved on, the runner fetches more of the branch's history in steps (64, then 1024, then 65536 commits), commits only and without their files, for two minutes at most, and checks that the commit is in it (`git merge-base --is-ancestor`). A commit that is not in the history of the ref (the branch was force-pushed, deleted, or the commit only exists on another branch) is never fetched by its id: the job fails with `CHECKOUT_REF_MOVED`. Then only the files of the run's commit are fetched, so a `git` command of yours that needs an older commit's files has to fetch them. Submodules, LFS files, hooks and redirects are off; a step of yours can fetch them with its own egress.
- **The credential** (a deploy key over SSH, or a short token over HTTPS) is a file in the checkout sandbox only. Your steps and plugins never get it, not in the environment, not in a file and not in `/proc`. Over SSH only the host key an admin confirmed for the repository is accepted, and any other key fails the job with `HOST_KEY_MISMATCH`.
- **Old runners:** a job with a checkout goes only to a runner that declared the `checkout` feature when it signed in. Until such a runner is online the job waits in the queue.

### Plugins

A `uses:` step runs a container image in a sandbox of its own, on the same `/workspace` and with the job's `network.egress`. It cannot widen the egress. The image needs no shell.

The image declares its inputs, outputs and entrypoint in the label `io.dhow.plugin.manifest`:

```json
{"Version": 1, "Entrypoint": ["/usr/local/bin/notify", "--quiet"],
 "Inputs": {"channel": {"Required": true}, "message": {"Default": "done"}}, "Outputs": ["url"]}
```

- `Entrypoint` starts with an absolute path and runs as is, without a shell (at most 32 arguments).
- Every `with:` key must be in `Inputs`, and every input marked `Required` must be set or have a `Default`. Otherwise the step fails with `PLUGIN_INPUT_INVALID` before the plugin starts. An image without a valid label fails with `PLUGIN_MANIFEST_INVALID`.
- Each input reaches the plugin as an environment variable `DHOW_INPUT_<NAME>`, with the name in upper case (`DHOW_INPUT_CHANNEL`). The value is never parsed by a shell.
- **A plugin gets only what is given to it:** its inputs, `DHOW_OUTPUT`, the proxy variables of the job's egress, the `env` of its own step, and the secrets given to that step. A secret in `with:` (`token: ${{ secrets.CHAT_TOKEN }}`) arrives as `DHOW_INPUT_TOKEN` and only in that step. A step skipped by its `if:` takes its secrets with it, and no step ever gets a secret declared on another one: the server checks it before the job is queued and fails the job with `JOB_SPEC_INVALID` otherwise. The job's `env`, secrets given to the whole job, and `DHOW_FENCING_TOKEN`, `DHOW_JOB_ID`, `DHOW_ATTEMPT_ID` and `DHOW_STEP_INDEX` never reach a plugin.

### Step outputs

Every step, `run:` or `uses:`, gets the variable `DHOW_OUTPUT`, a file where it may write `key=value` lines. Jobs that list this job in `needs` read the values as `needs.<job>.outputs.<key>`.

- Keys are letters, digits and `_` and do not start with a digit; a value is up to 1 KiB on one line. A job keeps at most 64 outputs, and a later step overwrites an earlier key.
- A plugin may write only the keys in its manifest's `Outputs`.
- A line that breaks these rules fails the step. Outputs are kept only when the job succeeds, also when the runner restarts before the server took the result.
- **Who sees them:** an output, and every other value an expression made (`env: VERSION: ${{ needs.build.outputs.version }}`, a `git.*` value), reaches the job's steps as is, but the job's details in the API and the UI show `[REDACTED]` in its place, for every role.
- **Masking:** the masker checks each value and also the values joined together (every run of keys in name order, and every pair in either order). A secret inside a value becomes `***`; a secret that only appears once values are joined drops every key it spans. A secret shorter than 4 characters cannot be masked anywhere (the job log says so), so it can leave in an output.

### Artifacts and the cache

An artifact keeps files of one job for the jobs that need it, and for people to download. The cache keeps files, like a package manager's folder, from one run to the next.

- **Archives:** the runner packs each artifact and the cache as a tar.gz that is the same for the same files: sorted entries, zero times, no owners, and modes `0644` or `0755`. Symbolic links that stay inside the workspace are kept; a link that leaves it, a device, a FIFO or a socket fails the job with `ARTIFACT_UNSAFE`. The archive is named by its SHA-256, so the same files give the same digest.
- **Where it runs:** packing, unpacking and `hash_files()` run in a sandbox of their own, from the runner's checkout image, with no network, on the job's workspace and in the job's user namespace. The runner itself never reads or writes the workspace.
- **Uploads:** after every step succeeded, each artifact is packed and uploaded. A path that does not exist fails the job (`ARTIFACT_UPLOAD_FAILED`), and an archive larger than `DHOW_ARTIFACT_MAX_BYTES` stops in the middle (`ARTIFACT_TOO_LARGE`). A failed job uploads nothing.
- **`needs`:** before the first step, the artifacts of the jobs in `needs` are downloaded, their SHA-256 is checked on the runner, and only then they are unpacked into `/workspace`. A digest that does not match fails the job with `ARTIFACT_DOWNLOAD_FAILED` and nothing is unpacked. The unpacker refuses `..`, absolute paths, links that leave the workspace, devices, FIFOs and hard links to files outside the archive (`ARTIFACT_UNSAFE`).
- **Retention:** artifacts are kept for `retention` (1 to 30 days, and never longer than `DHOW_ARTIFACT_RETENTION_DAYS`), then removed. A Developer and above can list a run's artifacts and download them (`GET .../runs/{runId}/artifacts`, `GET .../artifacts/{artifactId}/download`).
- **Cache key:** the server evaluates `cache.key`; each `hash_files()` call is left for the runner, which fills it with a SHA-256 of the matching files (sorted by path, each with its own SHA-256) at the start of the job. `hash_files()` takes glob patterns (`*`, `**`, `?`) relative to the workspace; a key may use its result only as text, so `size(hash_files(...))` or a condition on it is refused. The final key is 1 to 512 characters.
- **Cache scope:** a push or a manual run of a branch writes into the scope of that branch, a tag into its tag, and a pull request from the same repository into the scope of that pull request. A lookup tries the run's own scope, then the default branch. A pull request from a fork reads the default branch and never writes. A pull request can never write into the scope of `main`.
- **Saving:** the cache is saved only when the job succeeded and the key was not found at the start. An existing key is never replaced. When a project's entries pass `DHOW_CACHE_MAX_BYTES_PER_PROJECT`, the least recently used ones are removed. A cache that cannot be read or saved is a warning in the job log, not a failure.
- **Forks:** the artifacts of a run from a fork's pull request are marked `Untrusted` and can never feed a deploy.

## 4. Expressions

- **Where:** `if` (job and step), `concurrency.group`, `cache.key`, `env` and `with` values, and an artifact `name` (only text and `matrix.*` joined with `+` there, so the names are known without running anything). Every other field is plain text, and `${{` in it is an error.
- **Whole value:** an expression is the whole value: `${{ "v" + git.tag }}`, not `v${{ git.tag }}`. Build text with `+` inside CEL.
- **`if`** may be written with or without `${{ }}`.
- **Types are checked when the file is validated, not when it runs.** `if` must be `bool`, every other place must be `string`, and a name that is not available in that place is an error. So `if: ${{ git.branch }}` is refused before anything runs.
- **Secrets:** `${{ secrets.NAME }}` is allowed only as the whole value of `env` or `with`. The value never passes through CEL: the runner gets it as a secret reference. `secrets` in an `if`, in `concurrency`, in `cache` or inside a larger expression is an error.
- **Limits:** 2 KiB per expression and at most 1,024 expressions per pipeline. The highest possible cost of each expression is computed from these value limits when the file is validated, and an expression that could cost too much is refused (`EXPRESSION_TOO_EXPENSIVE`), as is a pipeline whose expressions together could cost too much (`EXPRESSION_BUDGET_EXCEEDED`). When it runs, an evaluation also stops at the same cost limit and after 10 ms. No function reads files or the network on the server.

## 5. The `run:` script

`${{ }}` is never allowed inside `run:`. A value from Git, such as a branch name or a commit message, could otherwise become part of the script (the well-known command injection in GitHub Actions). Pass the value through `env` and read it as a shell variable:

```yaml
version: 1
on:
  push: {}
jobs:
  greet:
    image: alpine:3.22@sha256:61bb7a95b0f3650f3039c0fcabe07674458cf0d13afa51aade6188bab8215b07
    steps:
      - run: printf 'building %s\n' "$BRANCH"
        env:
          BRANCH: ${{ git.branch }}
```

- The script runs with `/bin/sh -e` and is passed on stdin, so the first failing command stops the step.
- The script is at most 64 KiB and has no NUL bytes.

## 6. Matrix

- Each key under `matrix` (except `include` and `exclude`) lists its values: strings, numbers or booleans of at most 128 characters. Keys are a letter or `_` followed by letters, digits and `_`, at most 16 keys.
- The jobs are the Cartesian product of the keys, sorted by key. Each job is named `build (node=20, os=debian)`.
- **`exclude`** removes every combination that matches all the pairs of an entry. An entry uses only matrix keys.
- **`include`** adds one more combination per entry (unlike GitHub Actions, it never extends the existing ones). An entry sets every matrix key, so every job has the same `matrix.*` values. A matrix with only `include` is allowed, and all its entries use the same keys.
- **Limits:** at most 256 jobs per matrix and 256 jobs in the whole pipeline after expansion. A matrix with no combinations left, or two combinations with the same name, is an error.
- The expansion is deterministic: the same file always gives the same jobs in the same order, and the same job spec digest for each one.

## 7. Limits

| Limit | Value |
|---|---|
| File size | 256 KiB |
| One value | 64 KiB |
| One mapping key | 256 bytes |
| YAML depth | 32 levels |
| YAML nodes | 50,000 |
| Jobs before the matrix | 64 |
| Jobs after the matrix | 256 |
| Name of one matrix job (its values) | 512 characters |
| Steps per job | 100 |
| `env` variables per job or step | 100 (32 KiB per value) |
| `with` inputs per step | 32 (32 KiB per value) |
| `manual` inputs | 20 |
| Patterns per filter | 50 |
| Egress entries | 64 |
| Artifacts per job | 16 (32 paths each) |
| Expression size | 2 KiB |
| Expressions per pipeline | 1,024 |
| Time per expression | 10 ms |
| Job timeout | 1s to 24h |
| Expanded job spec | under 252 KiB |

## 8. Not supported yet

- **The `schedule` trigger** comes after the first release, and **the `artifact` trigger** (a build from external CI) comes with M6. Both are refused with `TRIGGER_NOT_SUPPORTED_YET`, not ignored.
- **`isolation: gvisor`** is refused with `ISOLATION_NOT_SUPPORTED_YET` until gVisor returns after the first release.
- **Environments and secrets** arrive with M5. The file is accepted with a warning (`ENVIRONMENTS_UNAVAILABLE`, `SECRETS_UNAVAILABLE`). Until then a job with `environment:` is skipped, and a job that asks for a secret fails when it is queued.
- **`services:`** (containers that live for the whole job) and the visual editor come after the first release.

## 9. Environment protection (server-side, not in YAML)

Protection rules are configured on the environment in the UI or API, so a commit **cannot weaken them**. A rule can set:
- which branches or tags are allowed;
- the required reviewers (a count and which teams), with optional self-approval prevention;
- a wait timer and deployment windows, including freeze periods;
- the required isolation level, and a requirement for digest-pinned images and verified plugins;
- an admin bypass, which is recorded in the audit log.

## 10. Visual editor

The UI will offer a visual pipeline editor (a jobs graph plus step forms), intended for newcomers, after the first release. It works like this:
- It reads the current `.dhow.yml` at the default branch.
- It edits a typed model, validated live against the same JSON Schema.
- It **writes YAML and opens a pull request** through the Git provider integration. Dhow's copy never silently diverges from the repo.

## 11. Validation

`dhow lint` (CLI), the editor and the server all use the same Go validator. They check the YAML, the schema, the DAG, the matrix, CEL type errors, unpinned plugins and policy violations. To install and use `dhow lint`, see [The `dhow` CLI](cli.md).

- Every problem has a stable code (`UPPER_SNAKE`, for example `DAG_CYCLE` or `RUN_EXPRESSION_NOT_ALLOWED`), a path (`jobs.build.steps[2].run`), a line and a column. A message never repeats the value itself, in case a secret was pasted by mistake.
- `error` stops the run, and `warning` (such as a tag-only image) shows in the lint output and the UI.
- Unknown keys, duplicate keys, anchors, aliases, merge keys and custom tags are errors.

## 12. Runs

A run is one pass of the pipeline over one commit. The server makes it from a trigger, reads the file at that commit, and turns each job of the file (after the matrix) into a job for a runner.

### 12.1 What starts a run

- **`push` and `tag`:** a webhook. A commit runs once per ref: the same commit sent again for the same ref makes no new run, even after the webhook deliveries are deleted (they are kept 30 days).
- **`pull_request`:** a webhook from GitHub or GitLab (M4.5 and M4.6). The run uses the head commit, not a merge commit, so the check shows on the commit you pushed.
- **`manual`:** a person with `run.trigger` on the project, through `POST /organizations/{id}/projects/{projectId}/runs` with `Ref`, an optional `Sha` and `Inputs`.
- **`api`:** the same request with an API token that has the `run.trigger` scope.
- **Which commit a hand-started run uses:** only a commit that a webhook already brought for that ref, from the repository itself and not from a fork. A `Sha` that did not come that way is refused (`SHA_NOT_ON_REF`). Without `Sha`, the run uses the latest such commit of the ref, or is refused (`REF_HEAD_UNKNOWN`) until a first push arrives. One person starts at most 60 runs and one project 240 runs in ten minutes.
- A run goes on only when its trigger is in `on:` and the filters match. Otherwise it ends `skipped` with `TRIGGER_NOT_ENABLED`.
- A commit without the file gives a `skipped` run with `DEFINITION_NOT_FOUND`, and the provider gets no check.
- `paths` filters apply once a provider sends the changed files (M4.5 and M4.6). Until then they match every push.

### 12.2 How a run moves

| State | Meaning |
|---|---|
| `awaiting_approval` | A pull request from a fork waits for a Maintainer (§12.4) |
| `pending` | The server reads the file at the commit, checks it and pins its images (§12.5) |
| `queued` | An older run of the same concurrency group still runs (§12.3) |
| `running` | The jobs run in the order of their `needs` |
| `succeeded`, `failed`, `canceled`, `skipped` | The run ended |

- A wrong file ends the run `failed` with `PIPELINE_INVALID`. `GET /organizations/{id}/projects/{projectId}/runs/{runId}` lists every problem with its code, line and column, the same as `dhow lint`.
- A job starts when every job in its `needs` succeeded or was skipped by its own `if`. When a need fails or is canceled, the jobs after it are `skipped` with `NEEDS_FAILED`.
- `cancel` cancels the jobs that wait at once, and a running job stops at its runner's next heartbeat.
- `rerun-failed` runs again the jobs that failed or were canceled, and the jobs skipped because of them, with the same file and the same image digests. The jobs that succeeded keep their result.
- When the server cannot make the jobs of a run, the run ends `failed` with `RUN_EXPANSION_FAILED`, and the runs of every other project go on.
- The logs of each job are under `/organizations/{id}/jobs/{jobId}`. A role binding on the project applies there too: a Maintainer lowered to Viewer on a project cannot cancel its jobs from that route.

### 12.3 Concurrency

- `concurrency.group` names a group inside the project. One run of a group runs at a time, and the newer ones wait as `queued`, in order.
- With `cancel_in_progress: true`, a new run cancels the older runs of its group **on the same ref** instead of waiting for them. Runs of other refs in the group are never canceled: the new run waits for them as `queued`.

```yaml
version: 1
on:
  push:
    branches: [main, "release/*"]
concurrency:
  group: ${{ "deploy-" + git.branch }}
  cancel_in_progress: true
jobs:
  deploy:
    image: alpine:3.22@sha256:61bb7a95b0f3650f3039c0fcabe07674458cf0d13afa51aade6188bab8215b07
    steps:
      - run: ./deploy.sh
```

### 12.4 Pull requests from forks

- The run reads the file from the base commit, never from the fork, so a pull request cannot add jobs, open egress or change images.
- The fork's head commit is checked out as data, and none of its settings are read.
- By default the run waits until a Maintainer or above approves it (`POST .../runs/{runId}/approve`). An Admin can set the project's fork policy to `automatic`.
- A job with `environment:` is skipped (`FORK_PR_NO_ENVIRONMENT`), a job that asks for a secret fails (`FORK_PR_NO_SECRETS`), and the cache is read-only.

### 12.5 Images given by tag

- When a run starts, the server asks the registry for the digest of every `image:` given by tag. Every job and every rerun of the run then pulls `name:tag@sha256:…`, even if the tag moves later. The run lists these pins under `ImagePins`.
- The server reaches the registry through the same guard as Git: no loopback, link-local, metadata or private address unless `DHOW_GIT_ALLOW_PRIVATE_CIDRS` allows the range. It waits 15 seconds at most per image and 30 seconds for the whole run, asks for four images at a time and for each image once, follows no redirect and reads at most 4 MiB.
- A token address from the registry must be `https` on the registry's own host, or on `auth.docker.io` or `ghcr.io`, on port 443 (or the registry's own port). The private ranges in `DHOW_GIT_ALLOW_PRIVATE_CIDRS` never apply to it.
- Anonymous pull tokens (like Docker Hub's) work. A registry that needs a password does not work yet (M5), so pin those images by digest in the file.
- A tag the registry does not know fails the run with `IMAGE_PIN_FAILED` at `jobs.<id>.image`.
- `dhow lint` still warns about a tag (`IMAGE_NOT_PINNED`), because a digest in the file is still the best way.

### 12.6 Generic Git repositories

- Gitea, Forgejo and plain Git servers have no API for files, so a small job on a runner checks the commit out and reads the file (a discovery job, `dhow.discovery` in the run's jobs). For a pull request from a fork it checks out the base commit.
- It goes only to a runner with the `checkout` feature, like every checkout job. Until one is online it waits in the queue, and after the queue timeout it fails and the run ends `failed` with `DISCOVERY_FAILED`.
- When the commit is not in the history of its ref, the run ends `failed` with `SHA_NOT_ON_REF` before any of your jobs runs.
- The runner is not trusted with the file. The job prints the raw Git objects on the file's path (the commit, every folder on the path, the file) to its log, and the server hashes them in the commit's object format (SHA-1 or SHA-256) and follows the chain from the run's commit id down to a regular file. Only a file proven that way is read. A forged file or folder, a commit other than the run's, a symlink, a commit or folder larger than 1 MiB, or a file larger than 256 KiB ends the run `failed` with `DEFINITION_UNVERIFIED`.
- A missing file, or a symlink, a folder or a submodule at the path, gives a `skipped` run with `DEFINITION_NOT_FOUND`, and the objects must prove that too. A job that reports neither `true` nor `false` for `dhow_found` ends the run `failed` with `DISCOVERY_FAILED`.
- Every job of the run then checks out the run's commit with the repository's deploy key over SSH (only the host key an admin confirmed is accepted), or without a credential over HTTPS.
- A server built without the checkout image ends such runs `failed` with `CHECKOUT_UNAVAILABLE`.
