[العربية](../../guides/environments.md)

# Environments

An environment is where a project deploys, such as `staging` or `production`. Its protection rules live on the server only, so no commit can change them: a job only names its environment with `environment:` in the pipeline file.

Until the deploy gate arrives (M5.6), a job with `environment:` is still skipped with `ENVIRONMENTS_UNAVAILABLE`. You can already create environments and their rules so they are ready.

## Who can do what

| Action | Role on the project |
|---|---|
| See environments and their rules | Every member |
| Pause deploys to an environment | Maintainer and above |
| Create an environment, change its rules, resume it and remove it | Admin and owner |

The role on the project is your organization role after the bindings of teams and members on the project, so an Admin binding on one project lets you manage the environments of that project only. API tokens can read environments but never change them.

## Create an environment

A pipeline file never creates an environment. A job that names a missing one will fail with `ENVIRONMENT_NOT_FOUND`, so a typo never deploys without protection.

```http
POST /api/v1/organizations/{id}/projects/{projectId}/environments
{"Name": "production", "Description": "Live site", "Rules": {"WaitMinutes": 10}}
```

The name uses lowercase letters, digits, `_` and `-` (up to 64), is unique in the project, and never changes. Rules you leave out take their defaults.

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

A pattern matches the branch or tag name: `*` and `?` inside one part between slashes, and `**` alone for any number of parts, such as `release/*` or `hotfix/**`. A pattern with `..`, `//`, a slash at either end, a space, `~`, `^`, `:`, brackets or braces is refused, as is one longer than 256 characters. An empty list means nothing deploys.

A reviewer must be a member of the organization with the Developer role or above on the project, and a team must belong to the same organization. Otherwise the request answers `REVIEWER_NOT_ELIGIBLE` with the reviewer that failed.

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

Jobs that already run finish. Only an Admin resumes, with the second step (`.../resume` with `CurrentFactor`).

## Remove an environment

`.../remove` with `CurrentFactor` removes the environment and its reviewers. The record stays in the database marked as deleted, and the name can be used again.

## The audit log

Every change is in the audit log: `environment.created`, `environment.updated` with the changed fields and their old and new values, `environment.removed`, `environment.paused` with the reason, and `environment.resumed`.
