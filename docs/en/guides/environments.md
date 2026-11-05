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

## The audit log

Every change is in the audit log, written in the same transaction as the change, so a change without its event never stays: `environment.created` with the initial rules, `environment.updated` with the changed fields and their old and new values, `environment.removed`, `environment.paused` with the reason, and `environment.resumed`. For deploys: `deployment.approval_requested`, `deployment.approved`, `deployment.rejected`, `deployment.approval_expired`, `environment.bypassed`, `deployment.started` and `deployment.finished`, with the kind of second factor only, never its value.

Each rule has its own entry. The lists (`AllowedRefs` and `AllowedPlugins`) always carry their count and a SHA-256 of the old and new values, and the values themselves only when they fit in 256 characters. Reviewers carry the ids added and removed, their count and a digest. When a value does not fit, the event says `Truncated: true` and keeps the digest, so a hidden change still shows.
