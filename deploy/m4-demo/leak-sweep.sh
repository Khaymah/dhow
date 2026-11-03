#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  printf 'usage: %s\n' "$0" >&2
  printf 'Looks for every credential of the demo stack in clear in pg_dump, the server and runner logs, the runner state\n' >&2
  printf 'with its Podman storage, the blob store and the systemd journal. deploy/m4-demo/run.sh runs it at the end;\n' >&2
  printf 'to run it alone, keep a stack with DHOW_DEMO_KEEP=1 and set DHOW_DEMO_WORK to its folder.\n' >&2
  exit 2
}
(($# == 0)) || usage

# shellcheck source=deploy/m4-demo/lib.sh
. "$(dirname -- "${BASH_SOURCE[0]}")/lib.sh"
work=${DHOW_DEMO_WORK:?set DHOW_DEMO_WORK to the folder of a running demo stack}
sweep=$work/sweep
canary=$work/canary
dirs="$sweep:/sweep/runner-state:/sweep/runner-dev:/sweep/blobs"
package=./apps/server/internal/App/LeakSweep

sweep_test() {
  local where=$1
  docker compose exec -T \
    -e DHOW_LEAK_SWEEP_DATABASE_URL='postgres://dhow:dhow-dev-only@db:5432/dhow?sslmode=disable' \
    -e DHOW_LEAK_SWEEP_SECRET_KEY_FILE="$work/server/secret-key" \
    -e DHOW_LEAK_SWEEP_NEEDLES="$work/needles" \
    -e DHOW_LEAK_SWEEP_DIRS="$where" \
    tools go test -tags leaksweep -count=1 -v -run '^TestNoCredentialIsStoredInClear$' "$package"
}

collect() {
  install -d -m 0700 "$sweep"
  docker compose exec -T db pg_dump --username dhow --no-owner dhow >"$sweep/pg_dump.sql"
  docker compose logs --no-color --timestamps server >"$sweep/server.log"
  docker compose logs --no-color --timestamps runner >"$sweep/runner.log"
  docker compose logs --no-color --timestamps db >"$sweep/postgres.log"
  if command -v journalctl >/dev/null 2>&1 && journalctl --no-pager --quiet --since '-3h' -o export >"$sweep/journal.export" 2>/dev/null &&
    [[ -s $sweep/journal.export ]]; then
    printf '    the systemd journal of the last 3 hours: %s bytes\n' "$(wc -c <"$sweep/journal.export")"
  else
    rm -f -- "$sweep/journal.export"
    printf '    no readable systemd journal on this machine; Docker keeps the container logs, which are swept above\n'
  fi
  grep -q 'CREATE TABLE public."Repositories"' "$sweep/pg_dump.sql" &&
    docker compose exec -T tools test -d /sweep/runner-state/.local/share/containers/storage
}

blind_spot() {
  install -d -m 0700 "$canary"
  grep -m1 '^API token=' "$work/needles" | cut -d= -f2- >"$canary/planted.log"
  if sweep_test "$canary" >"$work/canary.out" 2>&1; then
    printf '    the sweep missed a token planted in %s\n' "$canary/planted.log" >&2
    return 1
  fi
  grep -qF 'found the API token in clear in' "$work/canary.out"
}

say "Collecting pg_dump, the logs, the journal, the runner state and the blob store"
collect
say "Checking that the sweep finds a credential planted on purpose"
blind_spot
printf '    it does\n'
say "Sweeping"
sweep_test "$dirs" | tee "$work/sweep.out"
grep -q '^ok ' "$work/sweep.out"
