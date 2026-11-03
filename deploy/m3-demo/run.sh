#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  printf 'usage: %s\n' "$0" >&2
  printf 'Runs the M3 demo in Docker: Postgres, the server and one rootless Podman runner, and jobs without a pipeline:\n' >&2
  printf 'the sandbox holds, jobs run in order, and a job survives a killed runner. Needs Docker with Compose v2 and\n' >&2
  printf 'cgroup v2. It takes the same settings as deploy/m4-demo/run.sh (DHOW_DEMO_PORT, DHOW_DEMO_KEEP=1, ...).\n' >&2
  exit 2
}
(($# == 0)) || usage

# shellcheck source=deploy/m4-demo/lib.sh
. "$(dirname -- "${BASH_SOURCE[0]}")/../m4-demo/lib.sh"

# shellcheck disable=SC2016
isolation_script='set -e
test $(id -u) = 0
if touch /etc/dhow-escape 2>/dev/null; then echo the root filesystem is writable; exit 1; fi
if wget -q -T 5 -O /dev/null http://example.com 2>/dev/null; then echo the job reached the network; exit 1; fi
if grep -qE "^ +0 +0 +4294967295" /proc/self/uid_map; then echo the job runs as the host root; exit 1; fi
if ls /proc/1/root/ >/dev/null 2>&1 && test -e /proc/1/root/var/lib/dhow-runner; then echo the job sees the runner; exit 1; fi
echo dhow-demo isolation=ok'

spec() {
  local run=$1
  run=${run//\\/\\\\}
  run=${run//\"/\\\"}
  run=${run//$'\n'/\\n}
  printf '{"Image":"%s","Steps":[{"Name":"demo","Run":"%s"}]}' "$image" "$run"
}

epoch() { date -d "$(job_field "$1" "$2")" +%s%N; }

job_line() {
  printf 'job %s is %s%s (attempts: %s)' "$1" "$(job_field "$work/job.body" State)" \
    "$(job_field "$work/job.body" FailureReason | sed 's/^./ with &/')" "$(job_number "$work/job.body" AttemptCount)"
}

ends() {
  local id=$1 want=$2 seconds=${3:-600}
  if ! wait_job "$id" "$seconds"; then
    why "$(job_line "$id") and did not end within $seconds seconds"
    return 1
  fi
  printf '    %s\n' "$(job_line "$id")"
  [[ $job_state == "$want" ]] || {
    why "job $id should have ended $want"
    return 1
  }
}

isolated() {
  create_job "isolation" "$(spec "$isolation_script")" || { why "the job was not created"; return 1; }
  ends "$job_id" succeeded
}

in_order() {
  local first second
  create_job "first" "$(spec 'sleep 3; echo first')" && first=$job_id
  create_job "second" "$(spec 'echo second')" && second=$job_id
  [[ -n $first && -n $second ]] || { why "the two jobs were not created"; return 1; }
  ends "$first" succeeded && cp -- "$work/job.body" "$work/first.json" || return 1
  ends "$second" succeeded && cp -- "$work/job.body" "$work/second.json" || return 1
  if (($(epoch "$work/first.json" StartedAt) >= $(epoch "$work/second.json" StartedAt))) ||
    (($(epoch "$work/first.json" FinishedAt) > $(epoch "$work/second.json" StartedAt))); then
    why "the second job started before the first one finished"
    return 1
  fi
}

running() {
  for _ in $(seq 300); do
    api "$work/job" GET "/api/v1/organizations/$org_id/jobs/$1" '' "${session[@]}" >/dev/null
    [[ $(job_field "$work/job.body" State) == running ]] && return 0
    sleep 1
  done
  why "$(job_line "$1") and never ran within 300 seconds"
  return 1
}

survives_a_killed_runner() {
  local id status=0 runners
  create_job "killed runner" "$(spec 'sleep 20; echo done')" ',"MaxInfraRetries":1' || { why "the job was not created"; return 1; }
  id=$job_id
  running "$id" || return 1
  sleep 3
  runners=$(docker compose exec -T runner pgrep -u dhow-runner -x dhow-runner </dev/null | tr -s '\r\n' ' ') || true
  docker compose exec -T runner pkill -KILL -u dhow-runner -x dhow-runner </dev/null || status=$?
  printf '    killed the runner process %s (pkill exit code %s)\n' "${runners:-none}" "$status"
  ((status == 0)) || { why "pkill found no dhow-runner process to kill"; return 1; }
  ends "$id" succeeded 900 || return 1
  [[ $(job_number "$work/job.body" AttemptCount) == 2 ]] || { why "job $id should have needed exactly two attempts"; return 1; }
  online || { why "the runner did not come back online"; return 1; }
}

prerequisites
prepare_work
: >"$work/certs/ca.crt"
chmod 0644 "$work/certs/ca.crt"
trap teardown EXIT
clean_start

say "Postgres, the server and the Podman runner (the first build takes a few minutes)"
docker compose up --detach --build db server runner tools >/dev/null
need "the server starts and prints its setup link" server_ready
need "the owner and the organization are set up" setup_owner
need "the Podman runner registers and comes online" runner_up

say "One job in the sandbox"
check "the job runs as root of its own user namespace, with a read-only root and no network" isolated
say "Order"
check "two jobs on one runner run one after the other, in the order they were queued" in_order
say "A killed runner"
check "a job whose runner process is killed mid-step runs again once and succeeds, and the runner comes back" survives_a_killed_runner

summary
((failed == 0))
