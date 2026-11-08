[العربية](../../guides/notifications.md)

# Notifications

Dhow tells you when something needs you: a deployment failed, a deployment waits for approval, a pipeline run failed, a runner stopped answering, or something changed in the security of an account. It sends the message by email, to a signed webhook of your own, to a chat in Slack, Discord or Telegram, or to the notification inbox inside Dhow itself.

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

The account events reach the channels of the **organization** (not the channels of a project). The first sign-in of an account is not announced. A user also gets an email about a sign-in from a new device, in their own language, whatever the channels say. The account events also reach the inbox of the member they concern (see "The notification inbox").

A message carries names and numbers only: the project, the environment, the branch, the run number, the status, a short reason code and a link to the page in Dhow. It never carries log lines, variable values, tokens or a full IP address. For an account event it shows the browser and the system, and the network part of the address (`203.0.113.0/24`).

## Channels

A channel belongs to a project, or to the organization. A channel of the organization receives the events of every project and the events that belong to no project, such as `runner.offline`.

| Kind | What it needs | What members see |
|---|---|---|
| `email` | Nothing. The rule chooses the member roles that get the email | The email, in the language of the channel |
| `inapp` | Nothing. The rule chooses the member roles that get the notification | The notification, in the inbox inside Dhow |
| `webhook` | An https address that Dhow may reach | `Host` and `HasSecret`, never the address |
| `slack` | `Url`: the address of an Incoming Webhook | `Host` and `HasSecret`, never the address |
| `discord` | `Url`: the address of a Webhook | `Host` and `HasSecret`, never the address |
| `telegram` | `BotToken` and `ChatId` | `Host` and `HasSecret`, never the token |

- **Who sees what.** Everyone who can read the project (or the organization) sees the name, the kind and the state of a channel. Only those who manage channels see its `Host` and who made it.
- **Who can manage them.** For a project, a Maintainer, an Admin or an Owner (the action `notification.manage`). For the organization, an Admin or an Owner. Every change asks for a fresh second factor (`CurrentFactor`). An API token cannot manage channels.
- **The address is a secret.** The address of a webhook often holds a token. Dhow keeps it sealed with the instance keys, shows only the host (`hooks.example.com`), and never writes it to a log, an audit entry or an event. To replace it, send a new address with `CurrentFactor`.
- **The signing secret** is made by Dhow when you create the webhook and is shown **once**, in the answer of the create call. Copy it then. To make a new one, update the channel with `RotateSecret: true`.
- **The language** of a channel (`ar` or `en`) is the language of its messages, not the language of the person who pressed the button.
- **A channel stops** when its creator leaves the organization or no longer holds a role that manages channels, and after 10 failed attempts in a row. A stopped channel shows `DisabledReason`: `creator-removed` or `failures`. After 10 failures Dhow also emails the creator. A manager turns the channel on again by updating it with `Enable: true`, and becomes its owner.

## Slack, Discord and Telegram channels

The message goes to a chat in the service itself. Each service has a fixed host and Dhow accepts no other, and the sending passes the same protection as the generic webhook.

| Kind | What it needs | Accepted host | Shape of the address |
|---|---|---|---|
| `slack` | `Url` | `hooks.slack.com` | `/services/T…/B…/…` |
| `discord` | `Url` | `discord.com` and `discordapp.com` | `/api/webhooks/<id>/<token>` |
| `telegram` | `BotToken` from BotFather and `ChatId` (the number of the chat, or `@channelname`) | `api.telegram.org` | Dhow builds it |

- An address on another host, on a port other than 443, with a path that is not the path of the service, or with a `?`: Dhow answers `422 CHANNEL_HOST_NOT_ALLOWED`. Look-alike hosts such as `hooks.slack.com.example.org` are refused too.
- These channels have no signing secret, so no `Secret` is shown in the answer. The webhook address, the bot token and the chat id are secrets that Dhow keeps sealed, and they appear in no answer, log, audit entry or event. To replace one, send the new value with `CurrentFactor`. For Telegram you can send `BotToken` alone or `ChatId` alone.
- The message is one text in the language of the channel: the title, then the status, project, environment, branch, run number, time and link.
- **Text that someone else writes** (such as a branch name) shows as text and never acts as a command:
  - Slack: Dhow escapes `&`, `<` and `>`, so `<!channel>`, `<!here>` and `<@…>` never arrive alive.
  - Discord: Dhow sends an empty `allowed_mentions` (`"parse": []`), so `@everyone` and `@here` show as text and notify nobody. Dhow also escapes the formatting characters.
  - Telegram: Dhow sends HTML after it escapes `&`, `<` and `>`. It does not use Markdown.
  - In all three, Dhow removes control characters and text-direction override characters from the text.
- **A token error does not leave.** The Telegram token is part of the request path. When the connection fails, Dhow logs the failure code and the error text after it removes the address and the token from it.
- Answers of the service: `429` and `5xx` are tried again on the schedule below. `401`, `403` and `404` (a revoked token or a deleted webhook) are final at once, and the channel stops after 10 failures.

## The notification inbox

The notification inbox is a page in Dhow (`/notifications`) that gathers your notifications. The number of unread ones shows on the bell in the top bar and refreshes every minute.

- **What reaches it.** A channel of the kind `inapp` puts the notification in the inbox of every member who holds a role the rule chose, the same way an email reaches them. The events of your own account reach your own inbox without a channel: a sign-in from a new device, a change of the second factor or of the passkeys, and a change of your role in the organization.
- **What is in it.** A title that Dhow translates into the language of the interface, simple values such as the project, the environment, the branch and the run number, and a link to a page inside Dhow. It never carries a token, a secret, an IP address or log lines. These values are shown as text only, and Dhow never reads any of them as HTML.
- **The mute does not touch it.** Muting the notifications in the preferences stops the channel emails only. The inbox stays as it is.
- **Limits.** The inbox keeps the last 200 notifications of each user and drops the ones older than the retention days (30 days, `DHOW_NOTIFY_RETENTION_DAYS`). A member sees only the notifications of the organizations they still belong to, and the ones about their own account.
- **The API.** `GET /me/notifications` reads the page, `GET /me/notifications/unread-count` counts the unread ones, `POST /me/notifications/{notificationId}/read` marks one as read, and `POST /me/notifications/read-all` marks them all. All of them belong to the owner of the session only.

## Using the interface

- **Where.** On the page of a project, the tab "Notifications" (`/projects/<id>/notifications`).
- **A new channel.** The button "New channel" opens a panel: the kind, the name, the language and the address, then your second step. Only whoever manages the project sees the button (a Maintainer or above), and the server refuses the request from anyone else.
- **The signing secret** shows once after you create the webhook, or after "New secret". Copy it before you press "I am done". After that it stays nowhere: not in the page, not in the address, not in the memory of the browser.
- **The rules.** The button "Rules" in the row of a channel: the events, the statuses and the environments, and the member roles for the email and inbox channels.
- **A test message.** The button "Send a test message" queues it, and it shows in the delivery log. After 5 messages in a minute the page shows the limit and a countdown.
- **The delivery log.** The state of each message, how many times it tried and the last code, and a button "Try again" for the messages that failed for good. Only whoever manages channels sees it.
- **The personal mute.** In the preferences page: "Stop channel emails to my address".
- **Read only.** Whoever does not manage channels (a Developer or a Viewer) reads the channels and their rules and does not see the buttons that create, test or delete.

## Rules and mute

A channel has one rule: the events, the environments (empty means all) and the statuses it wants. A new channel starts with `deployment.finished`, `deployment.approval.requested` and `run.finished` for the statuses `failed`, `rolled_back`, `rollback_failed` and `awaiting_approval`. A successful deployment is **not** in the default, because it is noise. Add `succeeded` to the rule if you want it. The account events are in no default rule.

An email channel and an inbox channel also list the member roles that get the email or the notification. A member who muted the notifications in their preferences gets none of the channel emails. The security emails of the account are never muted. The mute is personal and concerns email only, and it does not touch the inbox: a Slack, Discord or Telegram message goes to a shared chat, not to a person, so nobody mutes it.

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
- Slack, Discord and Telegram channels accept the host of their own service only (the table above). The host is checked when you save and at every send.
- No loopback, private, link-local or reserved addresses, and no cloud metadata address, also when they are written as a decimal or hexadecimal number, as an IPv4-mapped IPv6 address, or hidden behind a DNS name. Dhow connects to the address it checked, so a name cannot change between the check and the connection.
- If the receiver is inside your network, the operator lists its range in `DHOW_NOTIFY_ALLOW_PRIVATE_CIDRS`, for example `10.20.0.0/16`. Loopback and cloud metadata addresses stay refused.

## Troubleshooting

| Sign | Likely cause |
|---|---|
| `422 CHANNEL_HOST_BLOCKED` | The address points to a private network. Ask the operator to add its range to `DHOW_NOTIFY_ALLOW_PRIVATE_CIDRS` |
| `422 CHANNEL_HOST_NOT_ALLOWED` | The address is not `https`, has a user name, or writes the host as a number. For Slack and Discord: the address is not an address of the service itself |
| `409 CHANNEL_VERSION_CONFLICT` | Someone changed the channel after you opened it. Reload and try again |
| `409 CHANNEL_DISABLED` | The channel stopped. Check `DisabledReason`, fix the cause and turn it on again |
| `429 TEST_RATE_LIMITED` | More than 5 test messages in a minute for one channel, 10 for one user, or 20 for the organization |
| A delivery `dead` with `blocked` | The address was refused at send time. The name now points to a private address |
| A delivery `dead` with `http_status` | The receiver answered with an error. `LastStatus` shows which one. For chat channels: `404` means a deleted webhook or a wrong token, and `401` a revoked bot token |
| A delivery `dead` with `mail_disabled` | The server has no SMTP settings, so it cannot send email |
