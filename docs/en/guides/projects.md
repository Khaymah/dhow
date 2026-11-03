[العربية](../../guides/projects.md)

# Projects and runs

A project links a Git repository to a pipeline file. Every push to the repository starts a run, and every run has jobs ordered in stages. The `/projects` page is in the sidebar and in the bottom bar on mobile.

## Who can do what

| Action | Role on the project |
|---|---|
| See projects and runs | Every member |
| Download artifacts, start a manual run, run failed jobs again | Developer and above |
| Create a project, change its settings, link, unlink and archive the repository, cancel a run, approve a run from a fork | Maintainer and above |
| Let runs from forks start without approval | Admin and owner |

The role on the project is your organization role after the bindings of teams and members on the project. The dashboard hides or disables what your role does not allow and says why.

## Create a project

“Create a project” asks for the name, the slug (it never changes), the pipeline file path (`.dhow.yml` by default) and what happens to pull requests from forks: wait for approval (the default) or start at once without secrets or environments.

## Link a repository

From the project page or its settings, “Link a repository” opens a series of steps:

1. **Address:** generic Git with an SSH address, or HTTPS for a public repository. The server checks the address, and any refusal shows as a translated sentence. GitHub and GitLab are listed as “coming soon”, without a button.
2. **Deploy key (SSH only):** copy the public half and add it to the Git server as read-only.
3. **Fingerprint (SSH only):** compare the host key fingerprint with the real one. It is pinned only after you confirm.
4. **Webhook:** copy the URL and the secret. The secret is shown once only, never lands in the URL, the browser history or its storage, and is cleared from the page when you continue.
5. **Status:** the page waits for the first signed webhook. Dhow has no test button: push a commit or use the test button of the Git server.

The project settings renew the deploy key and the webhook secret, and “Unlink the repository” sits in the danger zone behind a confirmation.

## Runs

The runs list shows the number, the branch, the first 7 characters of the commit, how it started, who started it and how long it took, with state filters. A run page shows:

- Why it stopped as a sentence, with the code only when it is new and we have no description yet.
- The jobs ordered in stages by `needs`, with their matrix values. A job name opens its live log.
- “Cancel the run”, “Run the failed jobs again” and “Approve the run”, by your role and the run state. Approval is session-only, and when your organization asks for a second step you do not have, the page sends you to the security settings.
- Artifacts with their size and expiry, marked “Untrusted” for runs from forks. Downloads come from the API as an attachment. Cache contents are never shown.

Pages refresh by themselves every few seconds while a run is active, and stop when it ends.

## Manual runs

“Start a manual run” asks for the branch or tag, an optional commit, and the `on.manual.inputs` with their types (text, true/false and a choice from a list). Dhow reads the inputs from the latest file it read on the default branch:

```http
GET /api/v1/organizations/{id}/projects/{projectId}/manual-inputs
```

`Known` is `false` until a run reads the file the first time, and `Enabled` says whether the file allows manual runs. The server checks the inputs again when the run starts.
