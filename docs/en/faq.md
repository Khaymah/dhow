[العربية](../faq.md)

# Frequently asked questions

This page collects the questions people ask most. You see the same content inside Dhow on the **Help** page, and it works there without an internet connection. For the full details, read [Troubleshooting](install/troubleshooting.md) and [Help inside Dhow](guides/help-and-faq.md).

## Getting started and signing in

### Where do I find the first setup link?

A new server has no users. Dhow writes a one-time link to its log, and the link is valid for one hour. Look for the `setup_url` field:

```sh
docker compose logs dhow | grep setup_url
```

If the hour has passed or you lost the link, restart Dhow to get a new one. The details are in [Finding the setup link](install/troubleshooting.md#finding-the-setup-link).

### Why does sign-in succeed and then ask me to sign in again?

The browser did not keep the session cookie. The usual causes:

- You open Dhow over HTTP on an address other than `localhost`. The cookie needs HTTPS.
- Dhow runs under a sub-path such as `/dhow/`. Give it its own domain name.
- A reverse proxy rewrites the `Set-Cookie` header.

The full explanation is in [Sign-in succeeds but you are not signed in](install/troubleshooting.md#sign-in-succeeds-but-you-are-not-signed-in).

### Why does the audit log show the proxy's address for every user?

Dhow does not trust the proxy in front of it, so it records the address of the connection, which is the proxy. Add the proxy's address to `DHOW_TRUSTED_PROXIES` and restart Dhow. See [Trusted proxies](install/reverse-proxy.md).

### How do I change the interface language or theme?

Language: press the language button in the top bar. Theme: open **Preferences** from your account menu. Dhow saves the language and theme for your account, so you see them on any device.

## Runs and deployments

### Why do my runs stay in the queue?

A queued run needs an active runner in the organization. The runner must have every label the run asks for, and it must support the run's isolation level. Open the run page to see why each runner cannot take it. If no runner fits, register one with the right labels. See [Follow runs](guides/jobs.md) and [Register a runner](runner/register.md).

### What happens if a run waits for a long time?

A run that is still waiting after 24 hours fails with the code `QUEUE_TIMEOUT`.

### Why is a deployment waiting for approval?

An environment with reviewers asks for approval before it deploys. A reviewer can approve or reject from the **Deploy approvals** page, and every decision needs a second verification step. The person who started the run cannot approve it, unless an admin allows that. A request that nobody decides expires, and the deployment fails. See [Environments](guides/environments.md).

### Where do I find the log of a failed run?

Open the run from the **Jobs** page. The run page shows why it failed in plain words, and the log of every attempt. You can search the log and download it. See [Follow runs](guides/jobs.md).

## Secrets and notifications

### Can I read a secret's value after I save it?

No. Dhow stores a secret encrypted and never shows it again, even to the organization owner. If you forget the value, write a new one. See [Secrets and variables](guides/secrets.md).

### How do I fix a notification channel that stopped?

Dhow stops a channel after 10 failed attempts in a row, or when its owner leaves the organization. Read `DisabledReason` on the channel and fix the cause. Then an admin turns the channel on again by updating it with `Enable: true`. See [Notifications](guides/notifications.md).

## About Dhow

### How do I know which version I am using?

Open the **About Dhow** page. It shows the version, the server's health and checks, and the license. See [About Dhow and server checks](guides/about-and-doctor.md).
