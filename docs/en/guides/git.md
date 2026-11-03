[العربية](../../guides/git.md)

# Link a Git repository

Dhow links each project to one Git repository. This page covers generic Git: Gitea, Forgejo and any Git server that speaks SSH or https. Linking GitHub and GitLab comes in later phases. The dashboard steps are in [Projects and runs](projects.md#link-a-repository); this page shows them with the API.

## The address

```http
PUT /api/v1/organizations/{id}/projects/{projectId}/repository
```

```json
{"CloneUrl": "ssh://git@git.example.com:2222/team/app.git", "DefaultBranch": "main"}
```

- Dhow accepts `ssh://user@host[:port]/path`, `user@host:path` (turned into `ssh://`), and `https://host[:port]/path` for a public repository without a password.
- Dhow refuses an IP address, a password in the address, `..`, a path or name that starts with `-`, and any other address such as `file://` and `ext::`.
- Linking needs `project.manage` on the project. With SSH, Dhow connects to the server on every `PUT`, so it accepts 30 such requests an hour per user and 120 per organization, then answers `429`.
- The first link answers `201` with the webhook secret (`WebhookSecret`). **Copy it now: it is never shown again.** Any later `PUT` answers `200` without a secret, and keeps the deploy key and the webhook address.

## The deploy key

For SSH, Dhow makes an Ed25519 key for the repository. Its public half is in `DeployKeyPublic`: add it to the Git server as a read-only deploy key. The private half stays encrypted on the server, and never appears in an answer or a log.

To rotate it, use `POST .../repository/rotate-deploy-key`. The previous key keeps working for 24 hours, or until you call `POST .../repository/confirm-deploy-key` after adding the new one.

## The host key

For SSH, Dhow connects to the server, fetches its host key and shows its fingerprint in `PendingHostKeyFingerprint`, such as `SHA256:...`. Compare it with the fingerprint the server itself shows, then pin it:

```http
POST /api/v1/organizations/{id}/projects/{projectId}/repository/confirm-host-key
```

```json
{"Fingerprint": "SHA256:..."}
```

Dhow never pins a key by itself. If the server's key changes, the new one waits until you confirm it, and the old one stays in use. Until the first confirmation, Dhow records webhooks and ignores them.

Dhow does not connect to an internal or reserved address (loopback, private networks, link-local, CGNAT and cloud metadata addresses), also when DNS answers with one. If your Git server is on your internal network, add its range to `DHOW_GIT_ALLOW_PRIVATE_CIDRS`. See [Configuration](../install/configuration.md#server-variables).

## Webhooks

The answer has `WebhookUrl`, such as `https://dhow.example.com/webhooks/git/<HookId>`. Its last part is random, not the repository id.

- **Gitea and Forgejo:** add a Gitea or Forgejo webhook with the address and the secret, content type `application/json`, and push events only. Dhow accepts `X-Forgejo-Signature`, `X-Gitea-Signature` and `X-Hub-Signature-256`.
- **Any other server:** send a `POST` from a script (such as `post-receive`) with this body:

```json
{"Ref": "refs/heads/main", "Sha": "<commit-sha>"}
```

and two headers: `X-Dhow-Delivery` with a unique id for the request, and `X-Dhow-Signature` with the time in unix seconds and a hex HMAC-SHA256 with the secret over the time, a dot and the body:

```text
X-Dhow-Delivery: <delivery-id>
X-Dhow-Signature: t=<unix-seconds>,v1=<hex(HMAC-SHA256(secret, t + "." + body))>
```

An example with `openssl`:

```sh
t=$(date +%s)
sig=$(printf '%s.%s' "$t" "$body" | openssl dgst -sha256 -hmac "$secret" -hex | sed 's/^.* //')
curl -sS -X POST "$url" -H "X-Dhow-Delivery: $(uuidgen)" -H "X-Dhow-Signature: t=$t,v1=$sig" --data "$body"
```

What Dhow does with each request:

| Case | Answer |
|---|---|
| A good signature and a new request | `202`, also when it is ignored (an event other than push, a deleted branch, an archived project, or a host key nobody confirmed) |
| The same request again within 3 days, with the same id or the same body | `200` without any effect |
| A wrong signature, the secret of another repository, a changed body, a `t` older than 5 minutes, or an unknown address | `401 WEBHOOK_REJECTED`, always the same answer |
| A good signature without a request id, or with an invalid branch or sha | `422 VALIDATION_FAILED` |
| A body larger than `DHOW_WEBHOOK_MAX_BYTES` | `413` |
| More than 300 requests a minute to one address, or 1200 from one client | `429` with `Retry-After` |

Dhow does not keep the request body, because it can hold names and email addresses. It keeps the id, a digest of the body, the branch, the sha and the outcome only, and deletes them after 30 days.

To rotate the secret, use `POST .../repository/rotate-webhook-secret`. The new secret is shown once, and the old one stops working at once.

## Unlink the repository

```http
DELETE /api/v1/organizations/{id}/projects/{projectId}/repository
```

It needs `project.manage`. The deploy key and the webhook stop at once, Dhow erases the stored key and secret, cancels the open runs with `REPOSITORY_UNLINKED` and abandons the commit statuses it has not sent: a webhook to the old URL answers `401`, and no new run starts until you link a repository again. Linking again makes a new key, a new URL and a new secret. Remove the key and the webhook from the Git server too.
