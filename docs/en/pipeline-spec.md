[العربية](../pipeline-spec.md)

# Pipeline spec — `.dhow.yml` v1

Status: **v1, validated.** The Go validator in `internal/pipelinespec` is the source of truth, and the server, the runner and the CLI (`dhow lint`) all use it. The JSON Schema in `packages/spec/dhow.schema.json` is for editors (VS Code), and a test keeps it in step with the validator. Runs that execute these files arrive with M4.8.

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
| `jobs.<id>.image` | string | Required when the job has a `run:` step. Should be digest-pinned: a tag-only image is a warning, and the runner refuses it unless `DHOW_RUNNER_ALLOW_UNPINNED_IMAGES` is set. A name without a registry host is completed the way Docker does it: `node:22-bookworm@sha256:…` runs as `docker.io/library/node:22-bookworm@sha256:…`. The runner never searches other registries. |
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
- **What it fetches:** the run's ref with `--depth=1`, then exactly the run's commit. If the branch moved on and the commit is no longer in its history, the runner fetches the commit by id, and if that fails too the job fails with `CHECKOUT_REF_MOVED`. Submodules, LFS files, hooks and redirects are off; a step of yours can fetch them with its own egress.
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
- **A plugin gets only what is given to it:** its inputs, `DHOW_OUTPUT`, the proxy variables of the job's egress, the `env` of its own step, and the secrets given to that step. A secret in `with:` (`token: ${{ secrets.CHAT_TOKEN }}`) arrives as `DHOW_INPUT_TOKEN` and only in that step. The job's `env`, secrets given to the whole job, and `DHOW_FENCING_TOKEN`, `DHOW_JOB_ID`, `DHOW_ATTEMPT_ID` and `DHOW_STEP_INDEX` never reach a plugin.

### Step outputs

Every step, `run:` or `uses:`, gets the variable `DHOW_OUTPUT`, a file where it may write `key=value` lines. Jobs that list this job in `needs` read the values as `needs.<job>.outputs.<key>`.

- Keys are letters, digits and `_` and do not start with a digit; a value is up to 1 KiB on one line. A job keeps at most 64 outputs, and a later step overwrites an earlier key.
- A plugin may write only the keys in its manifest's `Outputs`.
- A line that breaks these rules fails the step. Values pass through the masker, so a secret in an output is hidden. Outputs are kept only when the job succeeds.

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
