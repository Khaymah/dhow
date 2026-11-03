#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=deploy/dev/runner/runner-env.sh
. /usr/local/lib/dhow-dev/runner-env.sh

poll=${DHOW_DEV_POLL_SECONDS:-1}
cgroup=/sys/fs/cgroup
leaf=$cgroup/dev-init
slice=$cgroup/dhow-runner.service
controllers=(cpu io memory pids)
watched=(apps/runner internal packages/proto/gen/go go.mod go.sum)
pid=
forwarder=
hinted=0
history=$supervisor/starts
starts=()
start_window=630
max_starts=3
held=0
rebuilt=0

delegate() {
  if [[ $(stat -fc %T "$cgroup") != cgroup2fs ]]; then
    die "Docker gives this container cgroup v1, and the Podman runner needs cgroup v2. Docker Desktop has it with the WSL2 backend on Windows and on macOS; on Linux, use a distribution that boots with cgroup v2 (every current one does). On a machine without it, use the fake runner: docker compose -f deploy/dev/compose.yaml --profile fake-runner up --scale runner=0"
  fi
  if [[ $(cat /proc/sys/kernel/apparmor_restrict_unprivileged_userns 2>/dev/null || true) == 1 ]]; then
    die "the host lets AppArmor restrict unprivileged user namespaces (Ubuntu 24.04 and newer), so rootless Podman cannot start jobs. On the host, run: sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0"
  fi
  local c missing=()
  for c in "${controllers[@]}"; do
    grep -qw -- "$c" "$cgroup/cgroup.controllers" || missing+=("$c")
  done
  if ((${#missing[@]} > 0)); then
    die "Docker did not give this container the cgroup controllers ${missing[*]}, which the runner needs to limit jobs"
  fi
  mkdir -p "$leaf"
  local p procs tries
  for tries in 1 2 3 4 5; do
    procs=$(<"$cgroup/cgroup.procs")
    [[ -n $procs ]] || break
    if ((tries == 5)); then
      die "could not move the processes of this container out of its root cgroup"
    fi
    for p in $procs; do
      printf '%s\n' "$p" >"$leaf/cgroup.procs" 2>/dev/null || true
    done
  done
  for c in "${controllers[@]}"; do
    printf '+%s\n' "$c" >"$cgroup/cgroup.subtree_control" || die "could not enable the $c controller for the runner"
  done
  mkdir -p "$slice"
  chown "$user:$user" "$slice" "$slice/cgroup.procs" "$slice/cgroup.subtree_control" "$slice/cgroup.threads"
  if [[ -e $slice/memory.swap.max ]]; then
    printf '0\n' >"$slice/memory.swap.max"
  fi
  say "the runner gets its own cgroup with the ${controllers[*]} controllers, as systemd Delegate=yes gives it"
}

forward() {
  while true; do
    setpriv --reuid=nobody --regid=nogroup --clear-groups --inh-caps=-all \
      socat TCP-LISTEN:8080,bind=127.0.0.1,reuseaddr,fork TCP:server:8080 || true
    sleep 1
  done
}

fingerprint() {
  {
    find "${watched[@]}" -type f -name '*.go' -not -name '*_test.go' -printf '%T@ %s %p\n' 2>/dev/null || true
    find go.mod go.sum -printf '%T@ %s %p\n' 2>/dev/null || true
  } | sort | md5sum
}

build() {
  local tags=()
  if [[ $mode == fake ]]; then
    tags=(-tags dhowtest)
  fi
  say "building the runner"
  if CGO_ENABLED=0 go build "${tags[@]}" -trimpath -o "$bin.new" ./apps/runner/cmd/dhow-runner; then
    chmod 0755 "$bin.new"
    return 0
  fi
  say "the build failed; the previous runner keeps running until a save builds again"
  return 1
}

hint() {
  if ((hinted == 0 || SECONDS - hinted >= 60)); then
    say "waiting for a registration token. Create one on the Runners page, then run this from the root of your clone and paste it:"
    say "  docker compose -f deploy/dev/compose.yaml exec $service dhow-dev-register"
    hinted=$((SECONDS + 1))
  fi
}

start() {
  say "starting the runner"
  (
    if [[ $mode == podman ]]; then
      printf '%s\n' "$BASHPID" >"$slice/cgroup.procs"
    fi
    as_runner "$bin" run --dev
  ) &
  pid=$!
}

stop() {
  if [[ -n $pid ]] && kill -0 "$pid" 2>/dev/null; then
    kill -TERM "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  pid=
  if [[ $mode == podman && -e $slice/cgroup.kill ]]; then
    printf '1\n' >"$slice/cgroup.kill" 2>/dev/null || true
  fi
}

shutdown() {
  stop
  if [[ -n $forwarder ]]; then
    kill "$forwarder" 2>/dev/null || true
  fi
  exit 0
}

recent_starts() {
  local now t lines=()
  now=$(date +%s)
  starts=()
  if [[ -f $history ]]; then
    mapfile -t lines <"$history"
  fi
  for t in "${lines[@]}"; do
    if [[ $t =~ ^[0-9]{1,12}$ ]] && ((t <= now && now - t < start_window)); then
      starts+=("$t")
    fi
  done
}

may_start() {
  recent_starts
  if ((${#starts[@]} < max_starts)); then
    held=0
    return 0
  fi
  if ((held == 0)); then
    held=1
    say "the runner starts again at $(date -d "@$((starts[0] + start_window))" +%H:%M:%S): the server disables a runner that opens more than 3 new sessions in 10 minutes, because that looks like a copied identity"
  fi
  return 1
}

record_start() {
  recent_starts
  printf '%s\n' "${starts[@]}" "$(date +%s)" >"$history"
}

install_build() {
  if [[ -e $bin.new ]]; then
    mv "$bin.new" "$bin"
  fi
}

supervise() {
  if ! registered; then
    install_build
    hint
    return 0
  fi
  if [[ -n $pid ]] && kill -0 "$pid" 2>/dev/null; then
    if ((rebuilt == 0)) || ! may_start; then
      return 0
    fi
    say "restarting the runner with the new build"
    stop
  fi
  if [[ -n $pid ]]; then
    local status=0
    wait "$pid" 2>/dev/null || status=$?
    pid=
    say "the runner stopped with status $status; it starts again in 10 seconds"
    sleep 10
  fi
  install_build
  if [[ -x $bin ]] && may_start; then
    record_start
    rebuilt=0
    start
  fi
}

trap shutdown TERM INT

if [[ $mode == podman ]]; then
  delegate
else
  printf '{"Steps":[{"Output":[{"Data":"hello from the fake driver\\n"}]}]}\n' >"$recording"
  chmod 0644 "$recording"
  say "the fake driver runs no process: every step prints one line and succeeds"
fi
chown -h "$user:$user" "$state"
forward &
forwarder=$!

last=$(fingerprint)
build || true
while true; do
  supervise
  sleep "$poll"
  now=$(fingerprint)
  if [[ $now != "$last" ]]; then
    last=$now
    say "a change was saved; rebuilding"
    if build; then
      rebuilt=1
    fi
  fi
done
