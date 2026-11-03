#!/usr/bin/env bash
# shellcheck disable=SC2034

demo_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
root=$(cd -- "$demo_dir/../.." && pwd)
export DHOW_DEV_HTTP_PORT=${DHOW_DEMO_PORT:-8480}
export DHOW_DEV_WEB_PORT=$DHOW_DEV_HTTP_PORT
export DHOW_DEV_DB_PORT=${DHOW_DEMO_DB_PORT:-55480}
export DHOW_DEMO_REGISTRY_PORT=${DHOW_DEMO_REGISTRY_PORT:-55481}
export DHOW_DEMO_SUBNET=${DHOW_DEMO_SUBNET:-10.231.80.0/24}
export DHOW_DEV_POSTGRES_IMAGE=${DHOW_DEV_POSTGRES_IMAGE:-mirror.gcr.io/library/postgres:18}
export DHOW_DEV_GO_IMAGE=${DHOW_DEV_GO_IMAGE:-mirror.gcr.io/library/golang:1.27.3-bookworm}
export COMPOSE_PROJECT_NAME=${COMPOSE_PROJECT_NAME:-dhow-demo}
export COMPOSE_FILE="$root/deploy/dev/compose.yaml:$root/deploy/m4-demo/compose.yaml${DHOW_DEMO_COMPOSE_EXTRA:+:$DHOW_DEMO_COMPOSE_EXTRA}"
export COMPOSE_PROJECT_DIRECTORY=$root/deploy/dev
export DHOW_DEMO_WORK=${DHOW_DEMO_WORK:-}
image=${DHOW_DEMO_JOB_IMAGE:-mirror.gcr.io/library/busybox@sha256:a7dbc319b1438115a207e11930dc4bb3e643dcb3ec0bdd694bb36cd8fad289ec}
base="http://server:8080"
public_url="http://localhost:$DHOW_DEV_HTTP_PORT"
password='a long demo owner passphrase'
owner_name='Demo Owner'
owner_email='owner@dhow-demo.test'
org_name='Dhow Demo'
org_slug='dhow-demo'
passed=0
failed=0
results=()
keep=${DHOW_DEMO_KEEP:-0}

# shellcheck source=deploy/dev/lib/stack-api.sh
. "$root/deploy/dev/lib/stack-api.sh"
stack_curl=(docker compose exec -T tools curl)

say() { printf '\033[1m==>\033[0m %s\n' "$*"; }

record() {
  local verdict=$1 what=$2
  results+=("$verdict  $what")
  if [[ $verdict == PASS ]]; then
    passed=$((passed + 1))
    printf '  PASS  %s\n' "$what"
  else
    failed=$((failed + 1))
    printf '  FAIL  %s\n' "$what" >&2
  fi
}

check() {
  local what=$1
  shift
  if "$@"; then record PASS "$what"; else record FAIL "$what"; fi
}

need() {
  local what=$1
  shift
  if "$@"; then
    record PASS "$what"
    return 0
  fi
  record FAIL "$what"
  summary
  exit 1
}

summary() {
  local line
  printf '\n%s\n' '------------------------------------------------------------'
  for line in "${results[@]}"; do
    printf '%s\n' "$line"
  done
  printf '%s\n' '------------------------------------------------------------'
  if ((failed == 0 && passed > 0)); then
    printf 'PASS: %d of %d checks passed\n' "$passed" "$passed"
  else
    printf 'FAIL: %d of %d checks failed\n' "$failed" "$((passed + failed))"
  fi
}

prerequisites() {
  local missing=() tool
  for tool in docker grep sed cut od tr seq; do
    command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
  done
  if ((${#missing[@]} > 0)); then
    printf 'missing on this machine: %s\n' "${missing[*]}" >&2
    exit 2
  fi
  if ! docker compose version >/dev/null 2>&1; then
    printf 'Docker Compose v2 (docker compose) is required\n' >&2
    exit 2
  fi
}

prepare_work() {
  DHOW_DEMO_WORK=$(mktemp -d "${TMPDIR:-/tmp}/dhow-demo.XXXXXX")
  export DHOW_DEMO_WORK
  work=$DHOW_DEMO_WORK
  install -d -m 0700 "$work/server" "$work/certs" "$work/sweep"
  od -An -tx1 -N32 /dev/urandom | tr -d ' \n' >"$work/server/secret-key"
}

teardown() {
  local status=$?
  if [[ $keep == 1 ]]; then
    printf 'DHOW_DEMO_KEEP=1: the stack %s and %s are left for you\n' "$COMPOSE_PROJECT_NAME" "$DHOW_DEMO_WORK" >&2
    return "$status"
  fi
  docker compose --profile fake-runner down --volumes --remove-orphans >/dev/null 2>&1 || true
  if [[ -n $DHOW_DEMO_WORK && -d $DHOW_DEMO_WORK ]]; then
    docker run --rm -v "$DHOW_DEMO_WORK:/w" "$DHOW_DEV_GO_IMAGE" find /w -mindepth 1 -delete >/dev/null 2>&1 || true
    rm -rf -- "$DHOW_DEMO_WORK"
  fi
  return "$status"
}

clean_start() {
  docker compose --profile fake-runner down --volumes --remove-orphans >/dev/null 2>&1 || true
}

wait_http() {
  local url=$1 seconds=${2:-600}
  for _ in $(seq "$seconds"); do
    if docker compose exec -T tools curl -fsS -o /dev/null --max-time 3 -- "$url" 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  return 1
}

logged() {
  local service=$1 text=$2 seconds=${3:-900}
  for _ in $(seq "$seconds"); do
    docker compose logs --no-color "$service" 2>/dev/null | grep -qF -- "$text" && return 0
    sleep 1
  done
  return 1
}

server_ready() {
  wait_http "$base/api/v1/health/ready" 900 && logged server '/setup#' 60
}

runner_up() {
  logged runner 'waiting for a registration token' 900 || return 1
  register runner && online && ready runner
}
