#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() {
  printf '::error::%s\n' "$1" >&2
  exit 1
}

bin=${DHOW_BIN:?DHOW_BIN is not set}
[[ -x $bin && -f $bin ]] || fail "the dhow binary is missing"
command -v jq >/dev/null 2>&1 || fail "jq is needed to read the result, and it is not installed on this runner"

if [[ ${GITHUB_EVENT_NAME:-} == pull_request_target ]]; then
  fail "refusing to upload on pull_request_target: that event runs with secrets next to code from the pull request. Upload from a push to a branch or a tag instead."
fi

clip() {
  printf '%s' "$1" | LC_ALL=C tr -d '\000-\037\177' | LC_ALL=C head -c 200 | iconv -c -f UTF-8 -t UTF-8 || true
}

server=${DHOW_ACTION_SERVER:-}
path=${DHOW_ACTION_PATH:-}
[[ -n $server ]] || fail "the server input is empty"
[[ -n $path ]] || fail "the path input is empty"
[[ -n ${DHOW_INGEST_TOKEN:-} ]] || fail "the token input is empty. Pass it from a secret: token: \${{ secrets.DHOW_INGEST_TOKEN }}. Secrets are not sent to pull requests from forks."

args=(
  artifact push
  --format json
  --server "$server"
  --name "${DHOW_ACTION_NAME:-}"
  --ref "${DHOW_ACTION_REF:-}"
  --ref-kind "${DHOW_ACTION_REF_KIND:-}"
  --sha "${DHOW_ACTION_SHA:-}"
  --retries "${DHOW_ACTION_RETRIES:-3}"
  --provenance "Provider=github-actions"
  --provenance "Repository=$(clip "${DHOW_PROV_REPOSITORY:-}")"
  --provenance "RunId=$(clip "${DHOW_PROV_RUN_ID:-}")"
  --provenance "RunAttempt=$(clip "${DHOW_PROV_RUN_ATTEMPT:-}")"
  --provenance "Workflow=$(clip "${DHOW_PROV_WORKFLOW:-}")"
  --provenance "Actor=$(clip "${DHOW_PROV_ACTOR:-}")"
  --provenance "Event=$(clip "${DHOW_PROV_EVENT:-}")"
  --provenance "RunUrl=$(clip "${DHOW_PROV_RUN_URL:-}")"
)
case ${DHOW_ACTION_TRIGGER:-true} in
  true) ;;
  false) args+=(--no-trigger) ;;
  *) fail "the trigger input is true or false" ;;
esac
if [[ -n ${DHOW_ACTION_CA_FILE:-} ]]; then
  args+=(--ca-file "$DHOW_ACTION_CA_FILE")
fi

status=0
result=$("$bin" "${args[@]}" -- "$path") || status=$?
if ((status != 0)); then
  exit "$status"
fi

field() {
  jq -r --arg key "$1" 'if has($key) and .[$key] != null then .[$key] | tostring else empty end' <<<"$result"
}

digest=$(field Digest)
size=$(field SizeBytes)
created=$(field Created)
triggered=$(field Triggered)
run_id=$(field RunId)
run_number=$(field RunNumber)
run_url=$(field RunUrl)

[[ $digest =~ ^[0-9a-f]{64}$ ]] || fail "the result has no valid digest"
[[ $size =~ ^[0-9]+$ ]] || fail "the result has no valid size"
[[ $created == true || $created == false ]] || fail "the result has no valid created flag"
[[ $triggered == true || $triggered == false ]] || fail "the result has no valid triggered flag"
[[ -z $run_id || $run_id =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]] || fail "the result has an invalid run id"
[[ -z $run_number || $run_number =~ ^[0-9]+$ ]] || fail "the result has an invalid run number"
[[ -z $run_url || $run_url =~ ^https?://[^[:cntrl:][:space:]]+$ ]] || fail "the result has an invalid run URL"

if [[ -n ${GITHUB_OUTPUT:-} ]]; then
  {
    printf 'digest=%s\n' "$digest"
    printf 'size=%s\n' "$size"
    printf 'created=%s\n' "$created"
    printf 'triggered=%s\n' "$triggered"
    printf 'run-id=%s\n' "$run_id"
    printf 'run-number=%s\n' "$run_number"
    printf 'run-url=%s\n' "$run_url"
  } >>"$GITHUB_OUTPUT"
fi
if [[ -n ${GITHUB_STEP_SUMMARY:-} ]]; then
  printf 'Dhow stored sha256:%s (%s bytes). Created: %s. Run: %s\n' "$digest" "$size" "$created" "${run_url:-none}" >>"$GITHUB_STEP_SUMMARY"
fi
printf 'sha256:%s stored (%s bytes), created=%s, triggered=%s\n' "$digest" "$size" "$created" "$triggered"
