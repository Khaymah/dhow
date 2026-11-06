[العربية](../../runner/chaos.md)

# Chaos scenarios

The chaos suite (`apps/runner/Test/Chaos`) runs the real `dhow-server` and `dhow-runner` as processes, with the Podman driver, against PostgreSQL 18. It then breaks things on purpose and checks that a job never runs twice at once and that nothing is left behind. The design is in [ADR-0058](../../../Plan/ADR/0058-graded-kill-and-chaos-suite.md).

## What it checks

| Scenario | Expected outcome |
|---|---|
| A step ignores `SIGTERM` | It dies by `SIGKILL` after `DHOW_RUNNER_CANCEL_GRACE`, and the job ends `canceled`. |
| A step forks a process out of its process group | The process dies with the sandbox, and no process of a job user namespace is left on the host. |
| `kill -9` of the runner during a step that is not mutating | The sandbox is gone within 3 seconds. The restarted runner reports the attempt, the job runs once more and succeeds, and the two attempts never overlap. |
| `kill -9` of the runner after a mutating step began | The job ends `lost` with `LOST_AFTER_MUTATION` and is never run again. |
| `kill -9` of the runner right after a deploy step switched and before it reported | The job ends `lost` with `LOST_AFTER_MUTATION`, the switch happened once, nothing reported the end, and the next deploy runs alone on the same runner. On a target the server marks the release `unknown`, and the next deploy goes back to the last verified release (`internal/deployrelease/crash_test.go`). |
| The server is down for 20 seconds | The job carries on without a retry. |
| The server is down for 90 seconds | The runner fences itself and kills the sandbox within 35 seconds. When the server comes back, it retries the job once, with no overlap. |
| The network between the runner and the server is a blackhole for 60 seconds | Same as the 90-second outage. |
| PostgreSQL restarts during a job | The job succeeds without a retry. |
| The job is canceled while its image is still being pulled | The job ends `canceled`, and no sandbox starts. |

To detect two attempts running at the same time, each job calls a small HTTP sink on the host once a second through its egress proxy. Every call carries `DHOW_ATTEMPT_ID` and `DHOW_FENCING_TOKEN`. If an older attempt makes a call after a newer one has started, that is an overlap, and the test fails.

## Running it

It needs a Linux host like the [isolation suite](README.md#the-sandbox) does: Podman 5.4 or newer, `crun`, `passt`, `uidmap`, cgroup v2 delegation and sub-ids. It also needs Internet access to pull `postgres:18` and `registry:2`, and an `/etc/hosts` entry for the sink:

```bash
echo "127.0.0.1 sink.dhow.test" | sudo tee -a /etc/hosts
```

```bash
make runner-chaos CHAOS_RUN="systemd-run --user --pipe --wait --collect -p Delegate=yes --"
```

The suite takes 10 to 15 minutes. It sets up everything it needs in a fresh directory under `/tmp`:

- a throwaway CA;
- a TLS registry on `localhost:5000`, with a slow path on `localhost:5001`;
- the server, and a runner registered through the real API.

It removes all of this when it ends. If a scenario fails, the server and runner logs are in that directory.

In CI the suite runs nightly, on demand from the Actions tab, and on any pull request that carries the `chaos` label.
