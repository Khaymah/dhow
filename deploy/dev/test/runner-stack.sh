#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  printf 'usage: %s\n' "$0" >&2
  printf 'Run from the root of a clone with COMPOSE_FILE=deploy/dev/compose.yaml and the stack up (docker compose up --detach).\n' >&2
  printf 'It sets up the owner, registers the Podman runner, runs a job on it, then does the same with the fake runner.\n' >&2
  exit 2
}
(($# == 0)) || usage
[[ -n ${COMPOSE_FILE:-} ]] || usage

http_port=${DHOW_DEV_HTTP_PORT:-8080}
web_port=${DHOW_DEV_WEB_PORT:-5173}
image=${DHOW_DEV_JOB_IMAGE:-mirror.gcr.io/library/busybox@sha256:a7dbc319b1438115a207e11930dc4bb3e643dcb3ec0bdd694bb36cd8fad289ec}
base="http://127.0.0.1:$http_port"
public_url="http://localhost:$web_port"
work=$(mktemp -d)
failures=0
trap 'rm -rf -- "$work"' EXIT

# shellcheck source=deploy/dev/lib/stack-api.sh
. "$(dirname -- "${BASH_SOURCE[0]}")/../lib/stack-api.sh"

ok() { printf '  ok    %s\n' "$1"; }
evidence() {
  local service=$1
  printf '\n--- %s log ---\n' "$service" >&2
  docker compose logs --no-color "$service" >&2 || true
  if [[ -n $runner_id ]]; then
    printf -- '--- server log lines for runner %s ---\n' "$runner_id" >&2
    docker compose logs --no-color server 2>/dev/null | grep -F -- "$runner_id" >&2 || true
  fi
  printf -- '--- end ---\n\n' >&2
}
bad() {
  printf '  FAIL  %s\n' "$1" >&2
  failures=$((failures + 1))
  evidence "$service"
}
expect() {
  local what=$1
  shift
  if "$@"; then ok "$what"; else bad "$what"; fi
}
must() {
  local what=$1
  shift
  if "$@"; then
    ok "$what"
    return 0
  fi
  bad "$what"
  printf '\n%d check(s) failed; stopping here\n' "$failures" >&2
  exit 1
}

logged() {
  local service=$1 text=$2
  for _ in $(seq 300); do
    docker compose logs --no-color "$service" 2>/dev/null | grep -qF -- "$text" && return 0
    sleep 2
  done
  return 1
}

started_once() {
  local log
  log=$(docker compose logs --no-color "$service") || return 1
  [[ $(grep -cF 'starting the runner' <<<"$log") == 1 ]] && ! grep -qF 'the runner stopped' <<<"$log" &&
    [[ $(grep -cF '"msg":"the runner is ready"' <<<"$log") == 1 ]]
}

back_after() {
  local since=$1 log
  for _ in $(seq 180); do
    log=$(docker compose logs --no-color --since "$since" "$service" 2>/dev/null) || true
    if grep -F -- '"msg":"the runner is ready"' <<<"$log" | grep -qF -- "\"runner_id\":\"$runner_id\""; then
      return 0
    fi
    sleep 1
  done
  return 1
}

job_in_podman() {
  local status
  status=$(api "$work/busy-job" POST "/api/v1/organizations/$org_id/jobs" \
    "$(printf '{"Name":"%s","Spec":{"Image":"%s","Steps":[{"Name":"busy","Run":"sleep 30"}]}}' "$1" "$image")" "${session[@]}")
  [[ $status == 201 ]] || return 1
  for _ in $(seq 120); do
    docker compose exec -T runner pgrep -x conmon >/dev/null && return 0
    sleep 1
  done
  return 1
}

no_stale_podman() {
  ! docker compose logs --no-color "$service" 2>/dev/null | grep -qiE -- 'invalid internal status|system migrate'
}

delegated() {
  local pid=
  for _ in $(seq 60); do
    if pid=$(docker compose exec -T runner pgrep -o -u dhow-runner -x dhow-runner) &&
      docker compose exec -T runner grep -q '^0::/dhow-runner[.]service' "/proc/$pid/cgroup"; then
      return 0
    fi
    sleep 1
  done
  printf '    runner pid: %s\n' "${pid:-none}" >&2
  docker compose exec -T runner ps -eo pid,user,comm >&2 || true
  docker compose exec -T runner find /proc -mindepth 2 -maxdepth 2 -path '/proc/[0-9]*/cgroup' -exec grep -H . {} + >&2 || true
  return 1
}

job_succeeds() {
  local name=$1
  create_job "$name" "$(printf '{"Image":"%s","Steps":[{"Name":"hello","Run":"echo hello from the dev stack"}]}' "$image")" || return 1
  wait_job "$job_id" 300 || return 1
  if [[ $job_state == succeeded ]]; then
    printf '    job %s succeeded\n' "$job_id"
    return 0
  fi
  printf '    job %s ended %s\n' "$job_id" "$job_state" >&2
  cat -- "$work/job.body" >&2
  return 1
}

password='dev stack runner passphrase'
service=runner

printf 'The Podman runner\n'
must "the runner waits for a registration token" logged runner 'waiting for a registration token'
must "the owner and the organization are set up" setup_owner
must "the runner registers with dhow-dev-register" register runner
must "the runner is online" online
must "the runner signs in" ready runner
expect "the runner runs as dhow-runner in its delegated cgroup" delegated
expect "a job runs to success in rootless Podman" job_succeeds "dev stack podman"
expect "the runner started once after registering, with no restart" started_once

printf 'The Podman runner after its container stops in the middle of a job\n'
must "a job is running in Podman before the restart" job_in_podman "dev stack podman restarted"
since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
docker compose restart runner
must "the runner comes back after docker compose restart" back_after "$since"
must "the runner is online after the restart" online
expect "a job runs to success after the restart" job_succeeds "dev stack podman after restart"
must "a job is running in Podman before the kill" job_in_podman "dev stack podman killed"
since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
docker compose kill runner
docker compose up --detach runner
must "the runner comes back after docker compose kill and up" back_after "$since"
must "the runner is online after kill and up" online
expect "a job runs to success after kill and up" job_succeeds "dev stack podman after kill"
expect "Podman never asked for podman system migrate" no_stale_podman

printf 'The fake runner\n'
evidence runner
runner_id=
service=fake-runner
docker compose --profile fake-runner up --detach --scale runner=0
must "the fake runner waits for a registration token" logged fake-runner 'waiting for a registration token'
must "the fake runner registers with dhow-dev-register" register fake-runner
must "the fake runner is online" online
must "the fake runner signs in" ready fake-runner
expect "a job runs to success on the fake driver" job_succeeds "dev stack fake"
expect "the fake runner started once after registering, with no restart" started_once
expect "the fake container is not privileged" test "$(docker inspect --format '{{.HostConfig.Privileged}}' "$(docker compose ps -q fake-runner)")" = false

if ((failures != 0)); then
  printf '\n%d dev stack runner check(s) failed\n' "$failures" >&2
  exit 1
fi
printf '\nThe dev stack runs jobs on the Podman runner and on the fake runner\n'
