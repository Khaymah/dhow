[العربية](../../guides/environments.md)

# Environments

An environment is where a project deploys, such as `staging` or `production`. Its protection rules live on the server only, so no commit can change them: a job only names its environment with `environment:` in the pipeline file.

A job with `environment:` passes [the gate](#the-gate) before it starts, and gets the secrets and variables of the environment only after it. An environment with reviewers waits for [the approval](#approvals) of one of them.

## Who can do what

| Action | Role on the project |
|---|---|
| See environments and their rules | Every member |
| Pause deploys to an environment | Maintainer and above |
| Create an environment, change its rules, resume it and remove it | Admin and owner |
| Create a deploy target, change it, pin the keys of its servers and remove it | Admin and owner |
| Approve or reject a deploy | A reviewer of the environment, Developer and above |
| Bypass the reviewers and the wait | Admin and owner, when the environment allows it |

The role on the project is your organization role after the bindings of teams and members on the project, so an Admin binding on one project lets you manage the environments of that project only. API tokens can read environments but never change them. Reviewer ids show only to people who can manage the environment; everyone else, and every API token, sees how many reviewers there are.

## Create an environment

A pipeline file never creates an environment. A job that names a missing one will fail with `ENVIRONMENT_NOT_FOUND`, so a typo never deploys without protection.

```http
POST /api/v1/organizations/{id}/projects/{projectId}/environments
{"Name": "production", "CurrentFactor": {"Method": "totp", "Code": "123456"}, "Description": "Live site", "Rules": {"WaitMinutes": 10}}
```

Creating needs your second step (`CurrentFactor`), like every change, because a new environment can open self-approval or the admin bypass. The name uses lowercase letters, digits, `_` and `-` (up to 64), is unique in the project, and never changes. Rules you leave out take their defaults.

## Protection rules

| Rule | Default | Limits |
|---|---|---|
| `AllowedRefs`: the branches and tags that may deploy | The default branch of the linked repository, or `main` | Up to 50 patterns |
| `AllowPullRequests`: pull requests from the same repository | No | Pull requests from forks never deploy |
| `Reviewers`: users and teams who approve | None | Up to 20, and only one approval is needed |
| `AllowSelfApproval`: the person who started the run may approve it | No | For a team of one |
| `WaitMinutes`: wait before the deploy starts | 0 | Up to 43200 (30 days) |
| `ApprovalTimeoutHours`: how long a request stays open | 72 | From 1 to 720 |
| `RequirePinnedImages`: every image pinned by digest in the file | No | |
| `AllowedPlugins`: image prefixes of the allowed plugins | Any plugin | Up to 50 |
| `RequiredIsolation` | `container` | The only value today |
| `AllowAdminBypass`: an Admin may skip the reviewers and the timer | No | Never skips the refs or the pause |
| `SupersedePending`: a newer run on the same ref replaces a waiting one | Yes | |
| `AllowExtraDeployEgress`: a job that deploys to a [target](#deploy-targets) may also reach what its file lists in `network.egress` | No | A job that deploys reaches the servers of its target only |

A pattern matches the short branch or tag name, and `Kind` says which of the two it is: write `main`, never `refs/heads/main`, `heads/main` or `tags/v1`. `*` and `?` match inside one part between slashes, and `**` alone matches any number of parts, none included, so `release/**` matches `release` as well as `release/1.2` (use `release/*` to leave `release` out). A pattern with `..`, `//`, a slash at either end, a space, an invisible character, `~`, `^`, `:`, brackets or braces is refused, as is one longer than 256 characters. An empty list means nothing deploys.

Names, patterns, the description and the pause reason never accept invisible characters such as bidi overrides or zero-width spaces, so what you read is what the server checks.

A reviewer must be a member of the organization with the Developer role or above on the project, and a team must belong to the same organization. Otherwise the request answers `REVIEWER_NOT_ELIGIBLE` with the reviewer that failed.

## The gate

Once every job it needs has succeeded, we check in this order:

| Check | Result when it fails |
|---|---|
| The run comes from a fork | Skipped with `FORK_PR_NO_ENVIRONMENT` |
| The environment exists | Fails with `ENVIRONMENT_NOT_FOUND` |
| The `if:` condition, with the variables of the environment, the project and the organization | Skipped with `CONDITION_FALSE` |
| The branch or tag is in `AllowedRefs` | Skipped with `ENVIRONMENT_REF_NOT_ALLOWED`, and the run does not fail for it |
| The images are written with their digest in the file, and the plugins come from `AllowedPlugins` (a prefix matches whole parts of the name, so `ghcr.io/acme` does not match `ghcr.io/acme-x`) | Fails with `ENVIRONMENT_REQUIRES_PINNED_IMAGES` or `ENVIRONMENT_PLUGIN_NOT_ALLOWED` |
| The secrets the job asks for exist | Fails with `SECRET_NOT_FOUND` |
| The environment is not paused | Waits (`Gate` = `paused`) |
| There are no reviewers | Waits for approval (`Gate` = `approval`) |
| `WaitMinutes` has passed | Waits until `GateOpensAt` (`Gate` = `timer`) |
| No other run holds the environment | Waits (`Gate` = `environment_busy`) |

- The kind is part of the match: a tag named `main` does not match the branch rule `main`.
- A pull request from the same repository needs `AllowPullRequests`, and we match the branch it merges into (such as `main`).
- A waiting job stays `waiting`, and we check again every few seconds.
- When the job starts on the runner, and before any secret, we check again: the environment exists and is not paused, the branch is still allowed, and the rules did not change since the gate. If anything changed, the job fails with `ENVIRONMENT_RULE_CHANGED` and gets no secret.

## One run at a time

The first job that passes the gate makes its run hold the environment. The run holds it until every job of the run on this environment has finished, so no other run comes in between a deploy and a database migration, for example. When the environment is free, the run with the lowest number among those waiting for it takes it.

## The deployment log

Every job that passes the gate is added to the deployment log, with its run, branch, commit and state:

```http
GET /api/v1/organizations/{id}/projects/{projectId}/environments/{environmentName}/deployments
```

Newest first, and "deployed now" is the newest deploy whose state is `succeeded`. Everyone who can see runs (`run.read`) reads it.

## Approvals

When a job reaches an environment with reviewers, we make one approval request for the run on this environment, which covers every job of the run on it. The request stays open for `ApprovalTimeoutHours` hours.

```http
GET /api/v1/organizations/{id}/projects/{projectId}/runs/{runId}/approvals
POST /api/v1/organizations/{id}/projects/{projectId}/runs/{runId}/approvals/{approvalId}/approve
{"CurrentFactor": {"Method": "totp", "Code": "123456"}, "Comment": "Checked the migration"}
```

- **Who approves:** a reviewer of the environment (directly or through a team) who is a member of the organization with the Developer role or above on the project. Anyone else gets `REVIEWER_NOT_ELIGIBLE`.
- **Second factor:** every approval and rejection needs an app code, a recovery code or a passkey. A password alone gets `REVIEWER_NEEDS_SECOND_FACTOR`.
- **No self-approval:** whoever started the run by hand or with an API key, and whoever pushed it with their GitHub or GitLab account, cannot approve it (`SELF_REVIEW_NOT_ALLOWED`), unless an Admin turned on `AllowSelfApproval`. We know who pushed from the account name, which we save when they sign in to Dhow with that account. Someone who has not signed in with it yet is not known.
- **Rejection** (`.../reject` with the same rules): the job fails with `DEPLOYMENT_REJECTED`, and the jobs that need it are skipped.
- **Expiry:** a request nobody decides in time expires, and the job fails with `APPROVAL_EXPIRED`.
- **A newer run:** with `SupersedePending`, a newer run on the same branch that asks for approval on the same environment cancels the older request that still waits, and the old job is canceled with `SUPERSEDED`. An approval already given is not canceled.
- **Rule changes:** an approval is bound to `RuleVersion`. If the rules change before the job starts, we ask for a new approval. If the person who approved is no longer a reviewer or a member, we ask for a new approval too.
- **Rerunning the failed jobs** starts a new round with a new request.
- **One decision:** if several people decide at the same moment, one decision is taken and the others get `APPROVAL_NOT_PENDING`.

### Bypass the reviewers

```http
POST /api/v1/organizations/{id}/projects/{projectId}/runs/{runId}/approvals/{approvalId}/bypass
{"CurrentFactor": {"Method": "totp", "Code": "123456"}, "Reason": "Security fix for the login page"}
```

Admins and owners only, on an environment that allows it (`AllowAdminBypass`, otherwise `BYPASS_NOT_ALLOWED`), and the reason is required. A bypass skips the reviewers and the wait only: the allowed branches, the images, the plugins and the pause still apply. It is recorded as `environment.bypassed` with the reason.

### What waits for you

`GET /api/v1/organizations/{id}/deployment-approvals?Mine=true` lists the requests you can decide, and `Approvals.Pending` on the organization home page counts the requests on environments where you are a reviewer.

## Change the rules

```http
PUT /api/v1/organizations/{id}/projects/{projectId}/environments/{environmentName}
{"Version": 3, "CurrentFactor": {"Method": "totp", "Code": "123456"}, "Rules": {"AllowSelfApproval": true}}
```

- Send the `Version` you read. If someone changed the environment since, the answer is `ENVIRONMENT_VERSION_CONFLICT` and nothing changes.
- Every change needs your second step (`CurrentFactor`), even when your session is fresh.
- Only the fields you send change.
- Any change to the rules, the reviewers or the pause raises `RuleVersion`. Approvals are bound to it, so a changed rule asks for a new approval.

## Pause and resume

A Maintainer stops new deploys during an incident with a reason, and needs no second step:

```http
POST /api/v1/organizations/{id}/projects/{projectId}/environments/{environmentName}/pause
{"Reason": "The database migration failed"}
```

Jobs that already run finish, and a job that has not reached the gate yet waits until the environment is resumed. A job that passed the gate but has not started yet fails with `ENVIRONMENT_RULE_CHANGED` and gets no secret. Only an Admin resumes, with the second step (`.../resume` with `CurrentFactor`).

## Remove an environment

`.../remove` with `CurrentFactor` removes the environment and its reviewers. The record stays in the database marked as deleted, and the name can be used again.

## Deploy targets

A target is where a deploy arrives: one server or a group of up to 10 servers, with the user, the deploy path and the access key. A target belongs to the environment, so no commit can pick another server. A job writes only the name of the target, and the address comes from the server. A job that deploys points to the target with `with.target` in a [deploy plugin](../pipeline-spec.md#plugins-that-deploy-to-a-target), and the server checks the target and its keys before it queues the job, and again before it opens the access key at start. Running the deploy on one server, or on a group of servers in batches, is described in [Deploy to a server over SSH](deploy-ssh.md), and a Docker Compose project in [Deploy with Docker Compose](deploy-compose.md).

Everyone who reads the project reads the targets. Only an Admin and an owner create, change and remove them, with the second step (`CurrentFactor`).

```http
POST /api/v1/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets
{"Name": "web", "Kind": "ssh", "Username": "deploy", "DeployPath": "/srv/shop", "Hosts": [{"Host": "app1.example.com"}, {"Host": "app2.example.com", "Port": 2222}], "CurrentFactor": {"Method": "totp", "Code": "123456"}}
```

| Field | Rule |
|---|---|
| `Name` | Like the name of an environment, unique in the environment, and never changes |
| `Kind` | `ssh` or `compose` |
| `Hosts` | From 1 to 10 servers, and their order is the deploy order |
| `Host` | A lowercase DNS name or an IP address. Local addresses and cloud metadata addresses are refused with `TARGET_HOST_BLOCKED` |
| `DeployPath` | An absolute path of two levels at least, not under a system folder such as `/etc`, `/usr` or `/var/lib/docker`, or the answer is `DEPLOY_PATH_INVALID` |
| `Username` | A dedicated deploy account. `root` is refused unless `Settings.AllowRoot` is set |
| `KeepReleases` | From 2 to 50, and 5 by default |
| `KeySecretName` | The name of a secret of the same environment that holds the private key, or leave it empty and let Dhow make the key |

The same server with the same port and path never appears in two targets of the organization: `TARGET_DUPLICATE`. A change sends the `Version` you read, and if the target changed before you the answer is `TARGET_VERSION_CONFLICT` and nothing changes. Adding a server, removing one and changing the order follow the same rule, and 10 is the limit.

### The access key

`.../targets/{targetName}/generate-key` with `CurrentFactor` makes an ed25519 key and keeps its private half as the secret `<TARGET>_SSH_KEY` of the environment, so nobody ever sees its value. The answer carries only the public key in `PublicKey`, to put in `authorized_keys` on every server. Doing it again makes a new key and a new version of the same secret. If the environment already has a secret with that name that Dhow did not make for this target, the answer is `TARGET_KEY_NAME_TAKEN` and nothing is written. Two targets that would give the same name to the secret, such as `web-1` and `web_1`, cannot exist together: `TARGET_DUPLICATE`.

### Pin the key of a server

Dhow does not trust a server whose key you did not confirm. Each server of the target has its own key, and you pin it in three steps:

1. `.../hosts/{hostId}/fetch-host-key` fetches the key from the server with an SSH handshake and no login. Or `.../hosts/{hostId}/pending-host-key`, where you paste a line of `ssh-keyscan` or the content of a `.pub` file. Pasting is the only way for servers on an internal network, because fetching refuses them with `TARGET_HOST_BLOCKED`. A server that does not answer returns `TARGET_HOST_UNREACHABLE`.
2. Compare the `SHA256:` fingerprint that the target shows in `PendingHostKeyFingerprint` with the fingerprint of the server itself.
3. `.../hosts/{hostId}/confirm-host-key` with the fingerprint and `CurrentFactor`.

```http
POST /api/v1/organizations/{id}/projects/{projectId}/environments/{environmentName}/targets/{targetName}/hosts/{hostId}/confirm-host-key
{"Fingerprint": "SHA256:...", "CurrentFactor": {"Method": "totp", "Code": "123456"}}
```

A fingerprint that does not match the waiting key returns `HOST_KEY_FINGERPRINT_MISMATCH` and pins nothing. With no waiting key the answer is `HOST_KEY_NOT_PENDING`. If the key of a server changes later, the new one waits and the pinned one keeps working until you confirm the new one. Changing the address or the port of a server drops its pinned key, and removing the server drops it too.

A target is ready (`Ready`) when every server in it has a pinned key and the target has an access key. Otherwise `NotReady` says why: `host_key_unconfirmed` or `key_missing`.

Removing a target marks it and its servers as deleted, and the secret of its key stays with the environment. Removing the environment removes its targets and its secrets.

To check a target send a `POST` to its path followed by `/check` (no body, the action `deployment.check`). The request returns `202` with the `RunId` and `RunNumber` of the run that started, and this run goes through the gate of the environment like any deploy. When it ends, read the result of each server with a `GET` on the same path: `Reason`, `CurrentReleaseId`, `VerifiedReleaseId`, `FreeBytes`, `Locked` and `Repaired`. If this build of Dhow has no official plugin image the request returns `TARGET_CHECK_UNAVAILABLE`. The reasons are explained in [Deploy to a server over SSH](deploy-ssh.md#checking-a-target).

To read the releases of a target send a `GET` to its path followed by `/releases` (the action `project.read`). It returns the last 20 releases, each with the state of every server, `CurrentReleaseId`, the release the target was last deployed or rolled back to successfully, and `Partial`, which is `true` when the servers are on different releases. To roll back to an earlier release send a `POST` to the path followed by `/rollback` with `ReleaseId` and `CurrentFactor` in its body (the action `deployment.rollback`). A rollback skips only the reviewers and the wait timer, and Dhow accepts it only for a release that succeeded on the target and for an environment that is not paused. The details are in [Rolling back to an earlier release](deploy-ssh.md#rolling-back-to-an-earlier-release).

If two jobs of one run deploy to the same target, the second waits for the first to end and shows the gate "Target busy" (`target_busy`), and it does not fail.

## The audit log

Every change is in the audit log, written in the same transaction as the change, so a change without its event never stays: `environment.created` with the initial rules, `environment.updated` with the changed fields and their old and new values, `environment.removed`, `environment.paused` with the reason, and `environment.resumed`. For targets: `target.created`, `target.updated`, `target.removed`, `target.key_generated`, `target.host_added`, `target.host_updated`, `target.host_removed`, `target.host_key_fetched` and `target.host_key_confirmed`, with names and fingerprints only, the fingerprint of the old and the new key when one is pinned, and never a private key. For deploys: `deployment.approval_requested`, `deployment.approved`, `deployment.rejected`, `deployment.approval_expired`, `environment.bypassed`, `deployment.started` and `deployment.finished`, with the kind of second factor only, never its value. For releases on a target: `deployment.release_started`, `deployment.release_switched`, `deployment.release_succeeded`, `deployment.release_rolled_back`, `deployment.release_rollback_failed` and `deployment.release_unknown`, with ids and the reason only. For checks of a target: `deployment.target_check_requested`, `deployment.rollback_requested` for a rollback to an earlier release with the number of the release, `deployment.target_checked` for each server with its reason, and `deployment.release_repaired` when a check or a deploy wrote the state of a release that was `unknown`.

Each rule has its own entry. The lists (`AllowedRefs` and `AllowedPlugins`) always carry their count and a SHA-256 of the old and new values, and the values themselves only when they fit in 256 characters. Reviewers carry the ids added and removed, their count and a digest. When a value does not fit, the event says `Truncated: true` and keeps the digest, so a hidden change still shows.
