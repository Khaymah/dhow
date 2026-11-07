[العربية](../../guides/notifications.md)

# Notifications

Dhow tells you when something needs you: a deployment failed, a deployment waits for approval, a pipeline run failed, a runner stopped answering, or something changed in the security of an account. It sends the message by email or to a signed webhook of your own.

This page covers the first version: **email** and the **generic webhook**. Slack, Telegram and Discord come next.

## What is sent, and when

| Event | When | Status |
|---|---|---|
| `deployment.finished` | A deployment ended, or Dhow rolled it back | `succeeded`, `failed`, `canceled`, `rolled_back`, `rollback_failed` |
| `deployment.approval.requested` | A deployment waits for a reviewer | `awaiting_approval` |
| `run.finished` | A pipeline run failed (a run that succeeds is not announced) | `failed` |
| `runner.offline` | A runner did not answer for 5 minutes. One message for each outage | none |
| `account.new_device_login` | Someone signed in from a browser and system the account had not used before | none |
| `account.second_factor_changed` | A second factor or a passkey was added or removed, or recovery codes were made again | none |
| `member.removed` and `member.role_changed` | A member was removed, or their role changed | none |

The account events reach the channels of the **organization** (not the channels of a project). The first sign-in of an account is not announced. A user also gets an email about a sign-in from a new device, in their own language, whatever the channels say.

A message carries names and numbers only: the project, the environment, the branch, the run number, the status, a short reason code and a link to the page in Dhow. It never carries log lines, variable values, tokens or a full IP address. For an account event it shows the browser and the system, and the network part of the address (`203.0.113.0/24`).

## Channels

A channel belongs to a project, or to the organization. A channel of the organization receives the events of every project and the events that belong to no project, such as `runner.offline`.

| Kind | What it needs | What members see |
|---|---|---|
| `email` | Nothing. The rule chooses the member roles that get the email | The email, in the language of the channel |
| `webhook` | An https address that Dhow may reach | `Host` and `HasSecret`, never the address |

- **Who sees what.** Everyone who can read the project (or the organization) sees the name, the kind and the state of a channel. Only those who manage channels see its `Host` and who made it.
- **Who can manage them.** For a project, a Maintainer, an Admin or an Owner (the action `notification.manage`). For the organization, an Admin or an Owner. Every change asks for a fresh second factor (`CurrentFactor`). An API token cannot manage channels.
- **The address is a secret.** The address of a webhook often holds a token. Dhow keeps it sealed with the instance keys, shows only the host (`hooks.example.com`), and never writes it to a log, an audit entry or an event. To replace it, send a new address with `CurrentFactor`.
- **The signing secret** is made by Dhow when you create the webhook and is shown **once**, in the answer of the create call. Copy it then. To make a new one, update the channel with `RotateSecret: true`.
- **The language** of a channel (`ar` or `en`) is the language of its messages, not the language of the person who pressed the button.
- **A channel stops** when its creator leaves the organization or no longer holds a role that manages channels, and after 10 failed attempts in a row. A stopped channel shows `DisabledReason`: `creator-removed` or `failures`. After 10 failures Dhow also emails the creator. A manager turns the channel on again by updating it with `Enable: true`, and becomes its owner.

## Rules and mute

A channel has one rule: the events, the environments (empty means all) and the statuses it wants. A new channel starts with `deployment.finished`, `deployment.approval.requested` and `run.finished` for the statuses `failed`, `rolled_back`, `rollback_failed` and `awaiting_approval`. A successful deployment is **not** in the default, because it is noise. Add `succeeded` to the rule if you want it. The account events are in no default rule.

An email channel also lists the member roles that get the email. A member who muted the notifications in their preferences gets none of the channel emails. The security emails of the account are never muted.

## The signed webhook

Dhow sends `POST` with a JSON body. The field names start with a capital letter, like the rest of the API. These headers come with it:

| Header | Meaning |
|---|---|
| `Dhow-Event` | The event, such as `deployment.finished` |
| `Dhow-Delivery` | The id of the delivery. It is the same in every attempt |
| `Dhow-Timestamp` | The time of the attempt, in seconds since 1970 |
| `Dhow-Signature` | `v1=` followed by the hex of the signature |
| `Idempotency-Key` | The id of the event |

The signature is the HMAC-SHA256 of `timestamp + "." + body`, with the signing secret as the key. To check a message:

1. Read the timestamp. Refuse the message if it is more than 5 minutes away from your clock.
2. Compute the signature over the raw body, **before** you parse the JSON.
3. Compare it with `Dhow-Signature` in constant time.

```python
import hashlib
import hmac
import time


def verify(secret, timestamp, body, signature, now=None):
    now = time.time() if now is None else now
    if abs(now - int(timestamp)) > 300:
        return False
    message = timestamp.encode() + b"." + body
    expected = "v1=" + hmac.new(secret.encode(), message, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, signature)
```

A test vector for your code: the secret `dhow_whsec_test`, the timestamp `1700000000` and the body `{"A":1}` give `v1=63bf61c029c49a6b4f4776e06bfa67277e5f4a9f9517208b3c1ea9e845e45adb`.

## Delivery and retries

Dhow writes the message to a queue in the same database transaction as the event, so a restart between the event and the sending loses nothing. A worker sends it. The delivery is **at least once**: a message can arrive twice, for example when the server stops right after the receiver answered. Your receiver must accept a repeat. Use `Dhow-Delivery` to drop it.

| After attempt | Wait before the next |
|---|---|
| 1 | 1 minute |
| 2 | 5 minutes |
| 3 | 30 minutes |
| 4 | 2 hours |
| 5 | none: the delivery is `dead` |

- Dhow waits 5 seconds to connect and 10 seconds for the whole request, and reads at most 64 KiB of the answer. It keeps only the status code and a short error code, never the answer.
- An answer `2xx` is success. `408`, `425`, `429` and `5xx` are tried again. Another `4xx`, a redirect and a blocked address are final at once.
- At most 2 deliveries to one channel and 16 in all run at the same time. One slow receiver does not hold the others.
- The delivery log of a project or of the organization shows each delivery with its state: `pending`, `sending`, `sent` or `dead`. A `dead` delivery can be tried again by hand.
- Dhow keeps the log for 30 days (`DHOW_NOTIFY_RETENTION_DAYS`).
- **Fairness between organizations.** One organization takes at most 4 of the 16 deliveries at the same time. The account security emails have 2 slots of their own, so they go out even when every webhook slot is busy.
- **Email limits.** One event reaches at most 100 members, and no member gets more than 30 channel emails an hour, across all channels. A message reaches only those who really can read the project: their role in the project (with the roles given through teams) must be one of the roles the rule chose.
- **A failing mail server does not stop the channel.** A failed email to a member does not count toward the 10 failures. The failure shows in the delivery log with the code `mail_send` and is tried again on the same schedule.
- When the server stops, Dhow sends the account security emails that are still waiting, for 5 seconds at most, and never waits for a webhook.

## What Dhow refuses to reach

Dhow checks the address of a webhook when you save it **and again at every send**, because a name can point somewhere else later.

- Only `https`. No user name or password in the address. No redirects: an answer `3xx` is a failure.
- No loopback, private, link-local or reserved addresses, and no cloud metadata address, also when they are written as a decimal or hexadecimal number, as an IPv4-mapped IPv6 address, or hidden behind a DNS name. Dhow connects to the address it checked, so a name cannot change between the check and the connection.
- If the receiver is inside your network, the operator lists its range in `DHOW_NOTIFY_ALLOW_PRIVATE_CIDRS`, for example `10.20.0.0/16`. Loopback and cloud metadata addresses stay refused.

## Troubleshooting

| Sign | Likely cause |
|---|---|
| `422 CHANNEL_HOST_BLOCKED` | The address points to a private network. Ask the operator to add its range to `DHOW_NOTIFY_ALLOW_PRIVATE_CIDRS` |
| `422 CHANNEL_HOST_NOT_ALLOWED` | The address is not `https`, has a user name, or writes the host as a number |
| `409 CHANNEL_VERSION_CONFLICT` | Someone changed the channel after you opened it. Reload and try again |
| `409 CHANNEL_DISABLED` | The channel stopped. Check `DisabledReason`, fix the cause and turn it on again |
| `429 TEST_RATE_LIMITED` | More than 5 test messages in a minute for one channel, 10 for one user, or 20 for the organization |
| A delivery `dead` with `blocked` | The address was refused at send time. The name now points to a private address |
| A delivery `dead` with `http_status` | The receiver answered with an error. `LastStatus` shows which one |
| A delivery `dead` with `mail_disabled` | The server has no SMTP settings, so it cannot send email |
