#!/usr/bin/env bash
set -euo pipefail
umask 077

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
failures=0
base=http://server.test
public_url=http://localhost:1

# shellcheck source=deploy/dev/lib/stack-api.sh
. "$root/deploy/dev/lib/stack-api.sh"

fake_curl() {
  printf '%s\n' "$@" >"$work/argv"
  cat >"$work/stdin"
  local arg prev=
  for arg in "$@"; do
    if [[ $prev == --data-binary ]]; then
      cat -- "${arg#@}" >"$work/body"
    fi
    prev=$arg
  done
  printf '200'
}
stack_curl=(fake_curl)

absent() { ! grep -qF -- "$1" "$2"; }

check() {
  if "${@:2}"; then printf '  ok    %s\n' "$1"; else
    printf '  FAIL  %s\n' "$1" >&2
    failures=$((failures + 1))
  fi
}

session=(--header "Cookie: dhow_session=cookie-value-0123456789" --header "X-CSRF-Token: csrf-value-0123456789")
api "$work/call" POST /api/v1/sessions '{"Password":"password-value-0123456789"}' "${session[@]}" --header 'Authorization: Bearer dhow_pat_token-value-0123456789' >/dev/null

for secret in cookie-value password-value csrf-value token-value; do
  check "no $secret in the argv of curl" absent "$secret" "$work/argv"
done
check "the headers reach curl on stdin" grep -qF 'header = "Cookie: dhow_session=cookie-value-0123456789"' "$work/stdin"
check "the bearer token reaches curl on stdin" grep -qF 'header = "Authorization: Bearer dhow_pat_token-value-0123456789"' "$work/stdin"
check "the body reaches curl from a file" grep -qF '"Password":"password-value-0123456789"' "$work/body"
check "the body file is removed after the call" test ! -e "$work/call.req"
check "a quote in a header cannot break out of the curl config" test "$(curl_quote 'a"b\c')" = 'a\"b\\c'
for script in deploy/m4-demo/run.sh deploy/m4-demo/project.sh deploy/e2e/run.sh deploy/e2e/push.sh; do
  for pattern in '--user ' 'extraHeader' '-hmac ' '--password '; do
    check "$script passes no secret with $pattern" absent "$pattern" "$root/$script"
  done
done
check "the Forgejo password reaches curl on stdin" grep -qF 'curl --config - ' "$root/deploy/m4-demo/project.sh"
check "git reads the Forgejo password through GIT_ASKPASS" grep -qF 'GIT_ASKPASS="$work/askpass"' "$root/deploy/m4-demo/project.sh"

if ((failures > 0)); then
  printf '%d check(s) failed\n' "$failures" >&2
  exit 1
fi
printf 'No secret of the demo helpers reaches the argv of a process\n'
