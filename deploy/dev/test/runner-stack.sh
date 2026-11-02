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

ok() { printf '  ok    %s\n' "$1"; }
bad() {
  printf '  FAIL  %s\n' "$1" >&2
  failures=$((failures + 1))
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

json_field() { grep -o "\"$2\":\"[^\"]*\"" "$1" | head -1 | cut -d'"' -f4; }
api() {
  local prefix=$1 method=$2 path=$3 body=$4
  shift 4
  curl --silent --show-error --max-time 10 --dump-header "$prefix.head" --output "$prefix.body" --write-out '%{http_code}' \
    --request "$method" --header 'Content-Type: application/json' --header "Origin: $public_url" "$@" \
    ${body:+--data "$body"} -- "$base$path"
}

logged() {
  local service=$1 text=$2
  for _ in $(seq 300); do
    docker compose logs --no-color "$service" 2>/dev/null | grep -qF -- "$text" && return 0
    sleep 2
  done
  return 1
}

setup_owner() {
  local setup_token status cookie
  setup_token=$(docker compose logs --no-color server | grep -o "$public_url/setup#[A-Za-z0-9_-]*" | tail -1 | cut -d'#' -f2)
  [[ -n $setup_token ]] || return 1
  status=$(api "$work/setup" POST /api/v1/setup "$(printf '{"SetupToken":"%s","OwnerName":"Dev Stack","Email":"owner@dev-stack.test","Password":"%s","OrganizationName":"Dev Stack","OrganizationSlug":"dev-stack"}' "$setup_token" "$password")")
  [[ $status == 200 ]] || return 1
  status=$(api "$work/login" POST /api/v1/sessions "$(printf '{"Email":"owner@dev-stack.test","Password":"%s"}' "$password")")
  [[ $status == 200 ]] || return 1
  cookie=$(sed -n 's/^[Ss]et-[Cc]ookie: \([^=;]*dhow_session=[^;]*\);.*/\1/p' "$work/login.head" | tr -d '\r' | head -1)
  session=(--header "Cookie: $cookie" --header "X-CSRF-Token: $(json_field "$work/login.body" CsrfToken)")
  api "$work/me" GET /api/v1/me '' "${session[@]}" >/dev/null
  org_id=$(json_field "$work/me.body" OrganizationId)
  [[ -n $org_id ]]
}

register() {
  local service=$1 status token
  status=$(api "$work/rrt" POST "/api/v1/organizations/$org_id/runner-registration-tokens" '{"Labels":["pool=dev"]}' "${session[@]}")
  [[ $status == 201 ]] || return 1
  token=$(json_field "$work/rrt.body" Token)
  printf '%s\n' "$token" | docker compose exec -T "$service" dhow-dev-register >"$work/register.log" 2>&1 || {
    cat -- "$work/register.log" >&2
    return 1
  }
  cat -- "$work/register.log"
  runner_id=$(sed -n 's/^Registered runner \([0-9a-f-]*\) .*/\1/p' "$work/register.log")
  [[ -n $runner_id ]] && ! grep -qF -- "$token" "$work/register.log"
}

online() {
  for _ in $(seq 120); do
    api "$work/runner" GET "/api/v1/organizations/$org_id/runners/$runner_id" '' "${session[@]}" >/dev/null
    [[ $(json_field "$work/runner.body" Status) == online ]] && return 0
    sleep 1
  done
  return 1
}

delegated() {
  local pid
  if pid=$(docker compose exec -T runner pgrep -o -u dhow-runner -x dhow-runner) &&
    docker compose exec -T runner grep -q '^0::/dhow-runner[.]service' "/proc/$pid/cgroup"; then
    return 0
  fi
  printf '    runner pid: %s\n' "${pid:-none}" >&2
  docker compose exec -T runner ps -eo pid,user,comm >&2 || true
  docker compose exec -T runner find /proc -mindepth 2 -maxdepth 2 -path '/proc/[0-9]*/cgroup' -exec grep -H . {} + >&2 || true
  return 1
}

job_succeeds() {
  local name=$1 status id state
  status=$(api "$work/new-job" POST "/api/v1/organizations/$org_id/jobs" \
    "$(printf '{"Name":"%s","Spec":{"Image":"%s","Steps":[{"Name":"hello","Run":"echo hello from the dev stack"}]}}' "$name" "$image")" "${session[@]}")
  [[ $status == 201 ]] || {
    cat -- "$work/new-job.body" >&2
    return 1
  }
  id=$(json_field "$work/new-job.body" Id)
  for _ in $(seq 300); do
    api "$work/job" GET "/api/v1/organizations/$org_id/jobs/$id" '' "${session[@]}" >/dev/null
    state=$(json_field "$work/job.body" State)
    case $state in
      succeeded)
        printf '    job %s succeeded\n' "$id"
        return 0
        ;;
      failed | canceled | lost | timed_out)
        printf '    job %s ended %s\n' "$id" "$state" >&2
        cat -- "$work/job.body" >&2
        return 1
        ;;
    esac
    sleep 1
  done
  return 1
}

password='dev stack runner passphrase'
session=()
org_id=
runner_id=

printf 'The Podman runner\n'
must "the runner waits for a registration token" logged runner 'waiting for a registration token'
must "the owner and the organization are set up" setup_owner
must "the runner registers with dhow-dev-register" register runner
must "the runner is online" online
expect "the runner runs as dhow-runner in its delegated cgroup" delegated
expect "a job runs to success in rootless Podman" job_succeeds "dev stack podman"

printf 'The fake runner\n'
docker compose --profile fake-runner up --detach --scale runner=0
must "the fake runner waits for a registration token" logged fake-runner 'waiting for a registration token'
must "the fake runner registers with dhow-dev-register" register fake-runner
must "the fake runner is online" online
expect "a job runs to success on the fake driver" job_succeeds "dev stack fake"
expect "the fake container is not privileged" test "$(docker inspect --format '{{.HostConfig.Privileged}}' "$(docker compose ps -q fake-runner)")" = false

if ((failures != 0)); then
  printf '\n%d dev stack runner check(s) failed\n' "$failures" >&2
  exit 1
fi
printf '\nThe dev stack runs jobs on the Podman runner and on the fake runner\n'
