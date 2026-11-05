#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  printf 'usage: %s\n' "$0" >&2
  printf 'Looks for the credentials of the demo stack in clear. Where: pg_dump, the server, runner and Postgres logs, the\n' >&2
  printf 'runner state with its Podman storage, the blob store with every tar.gz artifact and cache entry unpacked in\n' >&2
  printf 'memory, and the systemd journal when there is one. What: every sealed value of every registered column (the\n' >&2
  printf 'deploy keys, webhook secrets, TOTP secrets, instance setting secrets, the secrets of every scope (organization,\n' >&2
  printf 'project and environment) and the registry passwords, opened with the keyring; a value it cannot open fails the\n' >&2
  printf 'sweep, and so does a scope with no value stored), the same values from the needles file, the data keys after\n' >&2
  printf 'unwrapping, the keyring keys (KEKs), the server key, the artifact grant key, the API token, the runner registration token, the\n' >&2
  printf 'session cookie, the CSRF token, the owner password and the runner keys (allowed only in its identity file),\n' >&2
  printf 'and anything shaped like an API, runner access or registration token or a download grant (input grants are\n' >&2
  printf 'kept in the job spec of the Jobs table on purpose and allowed there only). Not covered: the\n' >&2
  printf 'setup token, which the server prints on purpose, Forgejo and its password, the container images of Docker\n' >&2
  printf 'itself, and memory. deploy/m4-demo/run.sh runs it at the end; to run it alone, keep a stack with\n' >&2
  printf 'DHOW_DEMO_KEEP=1 and set DHOW_DEMO_WORK to its folder.\n' >&2
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
  local where=$1 only=${2:-TestNoCredentialIsStoredInClear}
  docker compose exec -T \
    -e DHOW_LEAK_SWEEP_DATABASE_URL='postgres://dhow:dhow-dev-only@db:5432/dhow?sslmode=disable' \
    -e DHOW_LEAK_SWEEP_SECRET_KEY_FILE="$work/server/secret-key" \
    -e DHOW_LEAK_SWEEP_KEYRING_FILE="$work/server/keyring" \
    -e DHOW_LEAK_SWEEP_NEEDLES="$work/needles" \
    -e DHOW_LEAK_SWEEP_RUNNER_IDENTITY=/sweep/runner-state/identity \
    -e DHOW_LEAK_SWEEP_DIRS="$where" \
    tools go test -tags leaksweep -count=1 -v -run "^${only}\$" "$package" </dev/null
}

collect() {
  install -d -m 0700 "$sweep"
  install -d -m 0700 "$sweep/pg_dump"
  docker compose exec -T db pg_dump --username dhow --no-owner dhow </dev/null | awk -v dir="$sweep/pg_dump" '
    /^COPY public\./ { t = $2; sub(/^public\./, "", t); gsub(/"/, "", t); out = dir "/" t ".sql" }
    { print > (out != "" ? out : dir "/schema.sql") }
    /^\\\.$/ { close(out); out = "" }'
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
  grep -q 'CREATE TABLE public."Repositories"' "$sweep/pg_dump/schema.sql" && test -s "$sweep/pg_dump/Repositories.sql" &&
    docker compose exec -T tools test -d /sweep/runner-state/.local/share/containers/storage &&
    docker compose exec -T tools test -s /sweep/runner-state/identity
}

plant() {
  local token
  token=$(grep -m1 '^API token=' "$work/needles" | cut -d= -f2-)
  [[ -n $token ]] || return 1
  install -d -m 0700 "$canary/clear" "$canary/encoded" "$canary/artifact" "$canary/shape" "$canary/src" "$canary/secret" "$canary/registry" "$canary/kek"
  printf '%s\n' "$token" >"$canary/clear/server.log"
  printf '{"msg":"request","token":"%s"}\n' "$(printf '%s' "$token" | base64 | tr -d '\n')" >"$canary/encoded/server.log"
  printf '%s\n' "$token" >"$canary/src/token.txt"
  docker compose exec -T tools tar -czf "$canary/artifact/blob" -C "$canary/src" token.txt </dev/null
  printf '{"msg":"deploy","env":"%s"}\n' "$(grep -m1 '^environment secret=' "$work/needles" | cut -d= -f2-)" >"$canary/secret/job.log"
  printf '{"msg":"pull","auth":"%s"}\n' "$(printf 'demo:%s' "$(grep -m1 '^registry password=' "$work/needles" | cut -d= -f2-)" | base64 | tr -d '\n')" >"$canary/registry/pull.log"
  printf '{"msg":"unseal","key":"%s"}\n' "$(awk 'NR == 1 { print $3 }' "$work/server/keyring")" >"$canary/kek/server.log"
  printf '{"msg":"renewed","token":"dhow_rat_%s"}\n' "$(od -An -tx1 -N24 /dev/urandom | tr -d ' \n')" >"$canary/shape/runner.log"
}

caught() {
  local case=$1 expect=$2
  if sweep_test "$canary/$case" >"$work/canary-$case.out" 2>&1; then
    printf '    the sweep missed the %s case in %s\n' "$case" "$canary/$case" >&2
    return 1
  fi
  if ! grep -qF -- "$expect" "$work/canary-$case.out"; then
    printf '    the sweep failed for another reason in the %s case:\n' "$case" >&2
    tail -n 20 "$work/canary-$case.out" >&2
    return 1
  fi
  printf '    caught: %s\n' "$expect"
}

say "Collecting pg_dump, the logs, the journal, the runner state and the blob store"
collect
say "Checking that the sweep catches credentials planted on purpose"
plant
caught clear "found the API token in clear in $canary/clear/server.log"
caught encoded "found the API token in clear in $canary/encoded/server.log"
caught artifact "found the API token in clear in $canary/artifact/blob!token.txt"
caught secret "found the environment secret in clear in $canary/secret/job.log"
caught registry "found the registry password in clear in $canary/registry/pull.log"
caught kek "found the keyring key 1 in clear in $canary/kek/server.log"
if ! sweep_test "$work" TestEachKindOfCredentialIsCaughtWhenPlanted >"$work/canary-planted.out" 2>&1; then
  printf '    the sweep did not catch a data key, a keyring key, a secret of each scope or a registry password planted by the test:\n' >&2
  tail -n 30 "$work/canary-planted.out" >&2
  exit 1
fi
printf '    caught: a data key, a keyring key, a secret of every scope and a registry password planted by the test\n'
caught shape "found the runner access token in clear in $canary/shape/runner.log"
say "Sweeping"
sweep_test "$dirs" | tee "$work/sweep.out"
grep -q '^ok ' "$work/sweep.out"
