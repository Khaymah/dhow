[العربية](../../runner/run.md)

# Run a runner

```sh
sudo systemctl enable --now dhow-runner
journalctl -u dhow-runner -f
```

or, by hand, `sudo -u dhow-runner dhow-runner run`. The runner logs JSON lines to stderr. They carry IDs, result codes and error codes, never job output, environment values or secrets.

## On start

1. It checks its [configuration](configuration.md) and refuses to start on any problem: a limit above what the machine has, a state file that other users can read, a plain `http://` address, a missing driver.
2. It signs in, removes any sandbox and attempt directory left by an earlier run, and reports every lease still in its journal. A job whose result was known is reported with that result. A job that was cut off is reported as `lost` with `RUNNER_RESTARTED`, using its old fencing token, so the server retries it at once if no step that changes a deploy target had started.
3. It starts asking for work, up to `DHOW_RUNNER_CAPACITY` jobs at a time.

## For each job

- **The grant is checked first.** A grant that is not signed by the server key pinned at registration, that names another runner or organization, that was issued more than 15 minutes from the runner's clock, or whose fencing token is not above every one the runner has seen before (a replay), is refused and never runs. Nothing about it is written to disk, and the server treats the lease as lost.
- **The job is checked again.** A spec that fails validation fails with `SPEC_REJECTED`. A job that asks for more CPU, memory, processes, disk or time than this runner allows fails with `RESOURCE_REQUEST_EXCEEDS_RUNNER` before it starts, and the server gives it to another runner.
- **One workspace, several sandboxes.** The attempt gets one `/workspace` (a `nosuid,nodev` tmpfs of the job's disk size) and one range of 65536 user ids, and its sandboxes run one after the other on it, so files keep their owners from one to the next. A job with a checkout starts with it, in its own sandbox from the `dhow-git` image pinned in the runner, which can reach the repository host and nothing else. Then every run of consecutive `run:` steps shares one sandbox of the job image, and every plugin (`uses:`) gets a sandbox of its own image with the job's egress. Each sandbox has the same hardening, and the runner checks after start that it has exactly the attempt's id range.
- **The checkout** fetches the job's commit with `git` run by argv, with no hooks, no submodules, no LFS and no redirects. Over SSH it accepts only the host key that was pinned for the repository. Its credential is a file of the checkout sandbox only, so no step of yours ever sees it, and the masker hides it from the log anyway.
- **Secrets** arrive encrypted for this runner's key and this attempt only, stay in memory, and reach only the steps that list them. No process outlives its step, but every step shares `/workspace`, so a secret scoped to one step is only as safe as the steps before it. Each step also gets `DHOW_FENCING_TOKEN`, `DHOW_JOB_ID`, `DHOW_ATTEMPT_ID`, `DHOW_STEP_INDEX` and `DHOW_OUTPUT`, the file where it may write `key=value` outputs for later jobs.
- **Output** goes through the masker before anything else: the secret itself, its base64 (at every offset, with and without padding), URL, hex and JSON forms, and each line of a multi-line secret, all become `***`. A secret shorter than 4 characters cannot be masked, and the job log says so by name. A secret longer than 4 KiB, or one that is not text, is masked as is and line by line, without the encoded forms. A secret over 64 KiB, or secrets over 1 MiB together, cannot be masked safely, so the job fails with `SPEC_REJECTED` before any step runs. Output is sent in ordered chunks of up to 64 KiB, at least every 250 ms.
- **When the server is slow or unreachable,** output waits in memory (8 MiB) and then in a spill file in the attempt directory (256 MiB, mode `0600`, masked text only). When both are full, the job blocks on its next write instead of losing output. If the disk itself fills up, the attempt fails with `RUNNER_DISK_FULL` and is retried elsewhere.
- **When the server truncates the log** at its limit, the job keeps running and later output is dropped.

## Cancel, time-outs and failures

| Event | What the runner does | Result |
|---|---|---|
| Someone cancels the job | `SIGTERM` to the step, `SIGKILL` after `DHOW_RUNNER_CANCEL_GRACE`, the whole sandbox 5 seconds later | `canceled` |
| The job runs past its `TimeoutSeconds` | The same sequence | `timed_out` |
| A step runs past its own timeout | The same sequence, then stop | `failed` (`STEP_FAILED`), the step `TIMED_OUT` |
| A step exits non-zero | Stops at that step | `failed`, never retried |
| No heartbeat gets through for 35 seconds | Kills the sandbox at once, about 10 seconds before the lease expires | `lost`, retried if nothing was changed |
| The server says the lease is lost | Kills the sandbox at once | decided by the server |
| The runner is stopped or restarted | Kills the sandboxes, reports them on the way down or on the next start | `lost` (`RUNNER_RESTARTED`) |

A job that sits idle sees a cancel within 10 seconds (the heartbeat interval). One that prints output sees it within a second.

## Troubleshooting

| Log message | Meaning |
|---|---|
| `the server revoked this runner` | An admin revoked it. The runner exits; register it again with a new token. |
| `an administrator disabled this runner` | It was disabled, possibly as a suspected clone. It keeps retrying and resumes once re-enabled. |
| `refused a lease grant and did not run it` | A grant failed the signature check. Check for a proxy rewriting traffic, or a server whose `DHOW_SECRET_KEY` changed (which needs every runner to register again). |
| `no heartbeat reached the server in time` | The network or the server was down for over 35 seconds. The job was stopped on purpose. |
| `the job log did not reach the server before the attempt closed` | The server was unreachable when the job ended. The result is still reported; the tail of the log may be missing. |
| `the server does not accept logs` | The server is older than the runner's log protocol. Jobs run, their output is not stored. |
| `... is not built into this dhow-runner yet` | This build has no sandbox driver for `DHOW_RUNNER_DRIVER`. See the [status](README.md#status). |
| `dhow: HOST_KEY_MISMATCH` (job log) | The repository's SSH server showed another host key than the one an admin confirmed. Nothing was fetched. Confirm the new fingerprint in the project only if you know why it changed. |
| `dhow: CHECKOUT_REF_MOVED` (job log) | The commit of the run is no longer on its branch or tag (a force push) and the server would not send it by id. Start a new run. |
| `dhow: PLUGIN_INPUT_INVALID` or `PLUGIN_MANIFEST_INVALID` (job log) | The `with:` values do not match the plugin's manifest, or the image has no valid manifest. The plugin never ran. |
