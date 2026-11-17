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
  printf 'kept in the job spec of the Jobs table on purpose and allowed there only). A deploy target adds the private key of its\n' >&2
  printf 'key secret (in every form, even on one line), a pending host key and its known_hosts line (allowed in the\n' >&2
  printf 'DeployTargetHosts row only), and any ingest token by its shape or from the needles file (ingest token=...).\n' >&2
  printf 'DHOW_DEMO_SWEEP_EXTRA is a colon separated list of folders (API responses, event captures) swept as well, and\n' >&2
  printf 'DHOW_DEMO_EXPECT_TARGETS=1 fails the sweep when the stack has no deploy target with a key. DHOW_DEMO_EXPECT_NOTIFY=1 adds\n' >&2
  printf 'the secrets of the notification channels (a Slack or Discord address, a Telegram bot token, a webhook signing secret: every\n' >&2
  printf 'one opened from the sealed settings, and any one by its shape), fails when the stack has no such channel, and plants each\n' >&2
  printf 'in a log, an API answer, an event stream, a database dump and an inbox to see that exactly that file is found. Not covered: the\n' >&2
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
dirs="$sweep:/sweep/runner-state:/sweep/runner-dev:/sweep/blobs${DHOW_DEMO_SWEEP_EXTRA:+:$DHOW_DEMO_SWEEP_EXTRA}"
package=./apps/server/internal/App/LeakSweep

sweep_test() {
  local where=$1 only=${2:-TestNoCredentialIsStoredInClear}
  docker compose exec -T \
    -e DHOW_LEAK_SWEEP_DATABASE_URL='postgres://dhow:dhow-dev-only@db:5432/dhow?sslmode=disable' \
    -e DHOW_LEAK_SWEEP_SECRET_KEY_FILE="$work/server/secret-key" \
    -e DHOW_LEAK_SWEEP_KEYRING_FILE="$work/server/keyring" \
    -e DHOW_LEAK_SWEEP_NEEDLES="$work/needles" \
    -e DHOW_LEAK_SWEEP_RUNNER_IDENTITY=/sweep/runner-state/identity \
    -e DHOW_LEAK_SWEEP_EXPECT_TARGETS="${DHOW_DEMO_EXPECT_TARGETS:-}" \
    -e DHOW_LEAK_SWEEP_EXPECT_NOTIFY="${DHOW_DEMO_EXPECT_NOTIFY:-}" \
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
  install -d -m 0700 "$canary/ingest"
  printf '{"msg":"upload","Authorization":"Bearer dhow_ing_%s"}\n' "$(od -An -tx1 -N24 /dev/urandom | tr -d ' \n')" >"$canary/ingest/upload.log"
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

needle_value() { grep -m1 "^$1=" "$work/needles" | cut -d= -f2-; }

plant_notify() {
  local slack token hook place
  slack=$(needle_value 'slack webhook url')
  token=$(needle_value 'telegram bot token')
  hook=$(needle_value 'webhook signing secret')
  [[ -n $slack && -n $token && -n $hook ]] || return 1
  for place in n-log n-api n-events n-dump/pg_dump n-inbox; do
    install -d -m 0700 "$canary/$place"
    printf '{"msg":"delivered","kind":"slack","host":"hooks.slack.com","state":"sent"}\n' >"$canary/$place/clean.txt"
  done
  printf '{"msg":"sent","url":"%s"}\n' "$slack" >"$canary/n-log/server.log"
  printf '{"Data":{"BotToken":"%s"}}\n' "$token" >"$canary/n-api/channels.json"
  printf 'id: 1-9\nevent: change\ndata: {"Secret":"%s"}\n\n' "$hook" >"$canary/n-events/events.txt"
  printf '0198f5d2\t%s\n' "$slack" >"$canary/n-dump/pg_dump/NotificationDeliveries.sql"
  printf '{"Data":[{"TitleKey":"inbox.deployment.finished.failed","Params":{"Project":"%s"}}]}\n' "$token" >"$canary/n-inbox/notifications.json"
}

caught_only() {
  local case=$1 file=$2 expect=$3 files
  caught "$case" "$expect"
  files=$(sed -n 's/^.*found the .* in clear in //p' "$work/canary-$case.out" | sort -u)
  if [[ $files != "$file" ]]; then
    printf '    the sweep found more than the one planted file in %s: %s\n' "$case" "$files" >&2
    return 1
  fi
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
caught ingest "found the ingest token in clear in $canary/ingest/upload.log"
if ! sweep_test "$work" '(TestAPrivateKeyOfATargetIsCaughtWhenPlanted|TestAPendingHostKeyIsCaughtWhenPlantedAndAllowedInItsRowOnly)' >"$work/canary-targets.out" 2>&1; then
  printf '    the sweep did not catch a deploy target key or a pending host key planted by the test:\n' >&2
  tail -n 30 "$work/canary-targets.out" >&2
  exit 1
fi
if grep -q -- '--- SKIP' "$work/canary-targets.out"; then
  grep -o -- '--- SKIP: Test[A-Za-z]*' "$work/canary-targets.out" | sed 's/^--- SKIP: /    skipped, nothing of that kind in this demo: /'
else
  printf '    caught: the private key of a deploy target and a pending host key planted by the test\n'
fi
caught shape "found the runner access token in clear in $canary/shape/runner.log"
if [[ -n ${DHOW_DEMO_EXPECT_NOTIFY:-} ]]; then
  say "Checking that each channel secret is found in the place it was planted, and in no other"
  plant_notify
  caught_only n-log "$canary/n-log/server.log" "found the slack webhook url in clear in $canary/n-log/server.log"
  caught_only n-api "$canary/n-api/channels.json" "found the telegram bot token in clear in $canary/n-api/channels.json"
  caught_only n-events "$canary/n-events/events.txt" "found the webhook signing secret in clear in $canary/n-events/events.txt"
  caught_only n-dump "$canary/n-dump/pg_dump/NotificationDeliveries.sql" "found the slack webhook url in clear in $canary/n-dump/pg_dump/NotificationDeliveries.sql"
  caught_only n-inbox "$canary/n-inbox/notifications.json" "found the telegram bot token in clear in $canary/n-inbox/notifications.json"
  if ! sweep_test "$work" TestEachNotificationSecretPlantedInAPlaceIsFoundThereAndNothingElseIs >"$work/canary-notify.out" 2>&1; then
    printf '    the sweep did not find exactly the channel secrets planted by the test:\n' >&2
    tail -n 30 "$work/canary-notify.out" >&2
    exit 1
  fi
  if grep -q -- '--- SKIP' "$work/canary-notify.out"; then
    printf '    the test skipped: the stack has no notification channel\n' >&2
    exit 1
  fi
  printf '    caught: each channel secret planted by the test, in a log, an API answer, an event stream, a dump and an inbox, and nothing else\n'
fi
say "Sweeping"
sweep_test "$dirs" | tee "$work/sweep.out"
grep -q '^ok ' "$work/sweep.out"
