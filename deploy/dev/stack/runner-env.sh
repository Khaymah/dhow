#!/usr/bin/env bash
# shellcheck disable=SC2034

mode=${DHOW_DEV_RUNNER_MODE:-podman}
user=dhow-runner
state=/var/lib/dhow-runner
opt=/opt/dhow-dev
bin=$opt/dhow-runner
recording=$opt/fake-recording.json
server_url=http://127.0.0.1:8080
origin="http://localhost:${DHOW_DEV_WEB_PORT:-5173}"
capabilities=-all,+setuid,+setgid,+dac_override,+sys_admin,+sys_ptrace

case $mode in
  podman) service=runner name=dev-runner ;;
  fake) service=fake-runner name=dev-fake-runner ;;
  *)
    printf '[runner] DHOW_DEV_RUNNER_MODE must be podman or fake, not %s\n' "$mode" >&2
    exit 1
    ;;
esac

say() {
  printf '\033[1;35m[%s]\033[0m %s\n' "$service" "$*"
}

die() {
  say "$*" >&2
  exit 1
}

as_runner() {
  local env=(
    HOME="$state"
    PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
    DHOW_RUNNER_STATE_DIR="$state"
    DHOW_RUNNER_SERVER_ORIGIN="$origin"
    DHOW_RUNNER_LOG_LEVEL="${DHOW_DEV_RUNNER_LOG_LEVEL:-info}"
  )
  if [[ $mode == fake ]]; then
    env+=(DHOW_RUNNER_DRIVER=fake DHOW_RUNNER_FAKE_RECORDING="$recording")
  fi
  if [[ -n ${DHOW_DEV_RUNNER_MAX_MEMORY:-} ]]; then
    env+=(DHOW_RUNNER_MAX_MEMORY="$DHOW_DEV_RUNNER_MAX_MEMORY")
  fi
  if [[ -n ${DHOW_DEV_RUNNER_MAX_DISK:-} ]]; then
    env+=(DHOW_RUNNER_MAX_DISK="$DHOW_DEV_RUNNER_MAX_DISK")
  fi
  cd "$state" || exit 1
  exec setpriv --reuid="$user" --regid="$user" --init-groups --inh-caps=-all --bounding-set="$capabilities" \
    env -i "${env[@]}" "$@"
}
