[العربية](../../guides/secrets.md)

# Secrets and variables

A secret is a value nobody should see, like an SSH key or an API token. A variable is a plain value your pipeline needs, like the address of a server. A registry credential lets Dhow read a private image.

Dhow stores a secret encrypted and never shows it again once it is saved. Neither the web app nor the API has any way to read a secret value, not even for the organization owner. If you forget the value, write a new one.

Organization and project secrets reach a job only when it starts, sealed for the runner that took it. Environment secrets arrive with the deploy gate (M5.6). A job that asks for a missing secret fails before it is queued with `SECRET_NOT_FOUND`, and `FailureNames` lists the missing names (never values).

## Where a secret lives

| Place | Who uses it |
|---|---|
| Organization | Every project of the organization, or the projects you pick |
| Project | Every job of the project |
| Environment | Only jobs that name this environment in `environment:`, once the environment lets them deploy |

The same name can exist in several places. The closest one wins: the environment, then the project, then the organization. The list shows that a secret covers the organization or project secret (`Overrides`).

Runs of pull requests from a fork never get any secret or variable. Runs of pull requests from the same repository do not get organization or project secrets unless you turn on `AvailableToPullRequests` for the secret. Environment secrets follow the rules of the environment.

## Who can do what

| Action | Role |
|---|---|
| See secret names and details (never the value) | Developer and above on the project, and API keys with `secret.read` |
| Write project secrets, variables and registry credentials | Maintainer and above on the project |
| Write organization secrets, variables and registry credentials | Admin and owner |
| Write environment secrets and variables | Whoever manages the environment (admin and owner) |
| See variables and their values | Everyone who can see the project |

API keys never write a secret. Writing works from a browser session only, and needs a second factor when your organization's policy asks for one.

A secret that a deploy target uses as the key to log in to its servers has a stricter rule. Writing its value, restoring an older version of it or removing it always needs `CurrentFactor`, whether or not the organization's policy asks for one. Without it the answer is `TARGET_KEY_NEEDS_FACTOR` (403) and nothing is saved, and with a wrong proof it is `INVALID_CURRENT_FACTOR`. The web UI shows the confirm step then and keeps the value you typed. Removing it stops deploys to the target until the key is made again, which is why it needs the same step: send `CurrentFactor` in the body of `.../remove`. Without it the answer is `TARGET_KEY_NEEDS_FACTOR` and nothing is removed.

## Write a secret

```http
PUT /api/v1/organizations/{id}/projects/{projectId}/secrets/DEPLOY_TOKEN
{"Value": "the secret value", "Description": "Deploy token for the API"}
```

The same request creates the secret or writes a new version of it. The answer has the name, the version and the size, never the value.

- **Name:** English letters, digits and `_`, not starting with a digit, up to 64 characters. Case does not matter: `Token` and `TOKEN` are one secret. Names that start with `DHOW_` are reserved.
- **Value:** text from 8 bytes to 32 KiB. A shorter value is hard to hide in logs. Write binary keys as base64.
- **Count:** 500 secrets for the organization, 200 for a project, and 100 for an environment.

An organization secret takes `Visibility`: `all_projects` for every project, or `selected_projects` with `ProjectIds` for the projects you pick only.

## Versions

Every write makes a new version. We keep the value of the last 5 versions only, and erase the value of an older version 30 days after a newer one was written. The current version is never erased.

```http
POST /api/v1/organizations/{id}/projects/{projectId}/secrets/DEPLOY_TOKEN/restore-version
{"Version": 3}
```

A restore writes the value of the earlier version as a new version. If its value was erased, it answers `SECRET_VERSION_ERASED`.

Removing a secret (`.../remove`) erases the values of all its versions at once. Removing an environment erases its secrets, its variables and its encryption key.

## Variables

```http
PUT /api/v1/organizations/{id}/projects/{projectId}/variables/DEPLOY_HOST
{"Value": "app.example.com"}
```

A variable has the same places and order, and the same name rules. The value is text up to 4 KiB, with 256 variables in each place. A variable is not a secret: everyone who can see the project reads its value, so never put a password in one.

## Registry credentials

```http
POST /api/v1/organizations/{id}/projects/{projectId}/registry-credentials
{"Host": "ghcr.io", "Username": "deploy-bot", "Password": "the registry token"}
```

- **Registry:** a name only, like `ghcr.io` or `registry.example.com:5000`, without `https` or a path. IP addresses are refused. `docker.io` is stored as `registry-1.docker.io`, because images are pulled from there.
- **Place:** the organization for all its projects, or a project. A project credential wins over an organization one for the same registry. One credential per registry, and 20 in each place.
- **Password:** never shown after it is saved. An update (`PUT .../registry-credentials/{credentialId}`) changes the username and the password together.

The server uses the credential when it pins an image to its digest. It sends it to the registry itself only, and to the token service when it is on the same registry over `https` (or `auth.docker.io` for Docker Hub). Runs of pull requests (from the same repository or a fork) use no credential, neither to pin nor to pull.

## The audit log

Every write, removal and restore is recorded with the name of the secret, its place and the version number only: `secret.created`, `secret.updated`, `secret.version_restored` and `secret.removed`, and the same for variables (`variable.*`) and registry credentials (`registry_credential.*`). No event holds a value or its length.
