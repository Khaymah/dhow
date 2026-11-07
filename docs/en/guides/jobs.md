[العربية](../../guides/jobs.md)

# Follow jobs in the web app

Open **Run › Jobs** in the side menu (or type "jobs" in the command palette). The list lives at `/jobs` and each job has its own page at `/jobs/<id>`, so you can bookmark or share a link to it. Every member of the organization sees the list. Viewers see the jobs but not their logs.

## The list

- Newest first, 20 to a page, with each job's state, when it was created, how long it waited, ran or took, its attempts and, for a job that ended badly, the reason in plain words.
- The chips above the list filter by state. Each chip turns its state on or off, so you can show several states at once, and **All** clears the filter. The filter is part of the address (`/jobs?state=queued`, or `/jobs?state=failed,timed_out` for several), so the numbers on the [home page](home.md) open the list already filtered.
- The API takes the same list: repeat `State` or separate the states with commas (`GET /api/v1/organizations/{id}/jobs?State=failed,timed_out`).
- **Jobs that no runner can take are called out at the top.** A queued job needs an active runner in the organization that has every label the job asks for and supports its isolation level. When none exists, the list shows how many jobs are stuck, marks each one, and offers to show only the waiting jobs. Register a runner with the right labels ([Register a runner](../runner/register.md)) and they start on their own. A job still waiting after 24 hours fails with `QUEUE_TIMEOUT`.
- The list refreshes every 10 seconds while any job on the page is still active.

## A job

- **State and times:** queued, started, finished, duration, the deadlines, and the reason when it failed, timed out, was lost or canceled.
- **Why is it waiting?** When no runner can take a queued job, the page lists every active and paused runner of the organization with its state, whether it supports the job's isolation level, the labels the job asks for that it lacks, and a verdict: can take it once connected, missing labels, lacks the isolation, or matches but paused. The dashboard applies the server's rule: the job's labels must all be in the runner's labels or capability labels, its isolation level must be one the runner supports, and the runner must be active. Owners, admins, maintainers and developers see the table. Under it, owners and admins get one action: open the paused runner that matches to resume it, edit the labels of the runner that lacks the fewest, or add a runner. Other roles read who can fix it, and viewers keep the short explanation without the table.
- **Stop the job:** owners, admins and maintainers can stop a job that has not finished, after a confirmation. A queued job stops at once. A running one stops when its runner sees the request, usually within 10 seconds.
- **Job log:** the log of the latest attempt, or of any earlier one. A running attempt streams live over server-sent events, and the view follows the newest line until you scroll up; "Jump to the latest line" brings it back. If the connection drops, the browser reconnects and continues after the last line it received. Colors from ANSI escape codes are shown; other terminal control codes are removed. The view keeps the last 100,000 lines in memory and fetches older ones from the server when you press "Load the older lines". A long log does not slow the page, because only the visible lines are drawn. If a proxy cuts the live stream, press "Use a fallback connection" and we read the log every two seconds instead. Developers and above can read logs.
- **Attempts:** each attempt with its state, reason, exit code, the runner that took it and its fencing token. A newer attempt always has a larger token, and the server refuses any call that carries an older one.
- **Settings:** the image, the steps (a step marked "Changes a target" stops automatic retries once it begins), the resources, the time limit, the labels, the isolation level and the hosts the job may reach. Secret values never appear.

### Search, share and download the log

- **Search:** type in "Search the log", or press `/` while you are inside the log. We find the matching lines even when they are off screen, and jump to the first one. `Enter` goes to the next result, `Shift` with `Enter` to the previous one, and `Esc` clears the search. "Match case" tells capital and small letters apart. The result count shows, and is read out, in a place apart from the list of lines. Search covers the loaded lines only, so load the older lines when you need them.
- **Link to a line:** press a line number to select the line and copy its link. The link carries `?line=<line number>` and the attempt number, and opens the page at the line, highlighted. "Copy the link of the selected line" works from the keyboard once you select a line or move to a search result. The line number is its position in the log as Dhow shows it.
- **Collapse steps:** press a step title to collapse or expand its lines. "Collapse all steps" collapses everything, and turns into "Expand all steps".
- **Download:** "Download the log" saves the whole text of the attempt as a file, from `GET /api/v1/organizations/<id>/jobs/<jobId>/logs/raw?Attempt=<number>`. The permission is the one for reading the log, and the text is the stored text the page shows.
- **Safety:** lines are shown as text only, so no HTML tag in them runs. Terminal control codes (cursor moves, title changes and links) are removed, and text direction override characters show as marks such as `‹U+202E›` and never reorder the text.

## Create a job through the API

Pipelines (milestone M4) will create jobs on their own. Until then, an owner or admin creates one through the API. When the organization has no jobs yet, the empty list links here.

```sh
curl -X POST https://dhow.example.com/api/v1/organizations/<org-id>/jobs \
  -H 'Content-Type: application/json' --cookie ... --header 'X-CSRF-Token: ...' \
  -d '{
    "Name": "build-web",
    "Spec": {
      "Image": "node:22@sha256:<digest>",
      "Steps": [{"Name": "test", "Run": "npm ci && npm test"}],
      "Labels": ["linux-builders"],
      "Isolation": "container"
    }
  }'
```

- `<org-id>` is `Memberships[].OrganizationId` in the answer of `GET /api/v1/me`. The request needs the session of an owner or admin, with a second factor when the organization asks for one.
- The image must be pinned by digest. The full spec, including resources, secrets and allowed hosts, is in [the API reference](../../../packages/openapi/openapi.yaml) under `CreateJobRequest`.
- The job waits in the queue until an active runner has every label in `Labels` and supports its `Isolation`.
