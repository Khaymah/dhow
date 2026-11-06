#!/usr/bin/env bash
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
root=$(mktemp -d)
trap 'rm -rf -- "$root"' EXIT

failures=0
checks=0

ok() {
  checks=$((checks + 1))
  printf 'ok   %s\n' "$1"
}

bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf 'FAIL %s\n' "$1"
}

expect() {
  local name=$1
  shift
  if "$@"; then ok "$name"; else bad "$name"; fi
}

tick=$'\x60'
contains() { grep -qF -- "$2" "$1"; }
lacks() { ! grep -qF -- "$2" "$1"; }
missing() { [[ ! -e $1 ]]; }

sha_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- "$1" | cut -d' ' -f1
  else
    shasum -a 256 -- "$1" | cut -d' ' -f1
  fi
}

mkdir -p "$root/bin" "$root/dist" "$root/tmp"

cat >"$root/bin/curl" <<'EOF'
#!/usr/bin/env bash
out=
url=
proto=
while (($#)); do
  case $1 in
    --output) out=$2; shift 2 ;;
    --url) url=$2; shift 2 ;;
    --proto) proto=$2; shift 2 ;;
    *) shift ;;
  esac
done
printf '%s %s\n' "$proto" "$url" >>"$CURL_LOG"
src="$DIST/$(basename -- "$url")"
[[ -f $src ]] || exit 22
cp -- "$src" "$out"
EOF
chmod +x "$root/bin/curl"

cat >"$root/stub-dhow" <<'EOF'
#!/bin/sh
printf 'ran\n' >>"$STUB_MARKER"
case $1 in
  version) printf 'dhow %s (abc, go1.27)\n' "${STUB_VERSION:-1.2.3}" ;;
  artifact)
    printf '%s\n' "$@" >"$STUB_ARGV"
    env >"$STUB_ENV"
    cat -- "$STUB_REPLY"
    exit "${STUB_EXIT:-0}"
    ;;
esac
EOF
chmod +x "$root/stub-dhow"

export DIST="$root/dist" CURL_LOG="$root/curl.log" STUB_MARKER="$root/marker"
export PATH="$root/bin:$PATH"

release() {
  local version=$1 os=$2 arch=$3 dir
  dir=$(mktemp -d "$root/pack.XXXXXX")
  cp -- "$root/stub-dhow" "$dir/dhow"
  tar -czf "$DIST/dhow_${version}_${os}_${arch}.tar.gz" -C "$dir" dhow
  sha_of "$DIST/dhow_${version}_${os}_${arch}.tar.gz"
}

fresh_action() {
  local version=$1 os=$2 arch=$3 sum=$4 dir
  dir=$(mktemp -d "$root/action.XXXXXX")
  cp -R -- "$here/." "$dir/"
  printf 'version=%s\n%s_%s=%s\n' "$version" "$os" "$arch" "$sum" >"$dir/dhow.pin"
  printf '%s' "$dir"
}

install() {
  local action=$1 os=${2:-Linux} arch=${3:-X64}
  rm -f -- "$root/out" "$root/install.log" "$STUB_MARKER" "$CURL_LOG"
  : >"$root/out"
  ACTION_PATH=$action RUNNER_OS=$os RUNNER_ARCH=$arch RUNNER_TEMP=$root/tmp GITHUB_OUTPUT=$root/out \
    bash "$here/scripts/install.sh" >"$root/install.log" 2>&1
}

sum=$(release 1.2.3 linux amd64)
action=$(fresh_action 1.2.3 linux amd64 "$sum")
code=0
install "$action" || code=$?
expect "the pinned release installs" test "$code" -eq 0
expect "the output names the binary" contains "$root/out" "bin=$root/tmp/dhow-action."
expect "the download came from the Dhow release page over https" contains "$CURL_LOG" "=https https://github.com/Khaymah/dhow/releases/download/v1.2.3/dhow_1.2.3_linux_amd64.tar.gz"

action=$(fresh_action 1.2.3 linux amd64 "$(printf '0%.0s' {1..64})")
code=0
install "$action" || code=$?
expect "a wrong SHA-256 fails the action" test "$code" -ne 0
expect "the failure says nothing was run" contains "$root/install.log" "Nothing was run."
expect "the downloaded binary never ran" missing "$STUB_MARKER"
expect "no binary is announced" lacks "$root/out" "bin="

action=$(fresh_action 1.2.3 linux amd64 "ABCDEF")
code=0
install "$action" || code=$?
expect "a malformed pinned checksum fails" test "$code" -ne 0

action=$(fresh_action unreleased linux amd64 "$sum")
code=0
install "$action" || code=$?
expect "an unreleased copy of the action fails" test "$code" -ne 0
expect "and says to use the released action" contains "$root/install.log" "Khaymah/dhow-action"
expect "and downloads nothing" missing "$CURL_LOG"

action=$(fresh_action "1.2.3; rm -rf /" linux amd64 "$sum")
code=0
install "$action" || code=$?
expect "a version that is not semantic fails" test "$code" -ne 0
expect "and downloads nothing" missing "$CURL_LOG"

action=$(fresh_action 1.2.3 linux amd64 "$sum")
code=0
install "$action" Windows X64 || code=$?
expect "a Windows runner is refused" test "$code" -ne 0
code=0
install "$action" Linux X86 || code=$?
expect "a 32-bit runner is refused" test "$code" -ne 0
code=0
install "$action" macOS ARM64 || code=$?
expect "a platform with no pinned checksum fails" test "$code" -ne 0
expect "and downloads nothing" missing "$CURL_LOG"

darwin_sum=$(release 1.2.3 darwin arm64)
action=$(fresh_action 1.2.3 darwin arm64 "$darwin_sum")
code=0
install "$action" macOS ARM64 || code=$?
expect "macOS on ARM64 picks the darwin arm64 archive" test "$code" -eq 0
expect "with the right file name" contains "$CURL_LOG" "dhow_1.2.3_darwin_arm64.tar.gz"

action=$(fresh_action 1.2.3 linux amd64 "$sum")
code=0
DHOW_RELEASE_BASE_URL=http://evil.example.com/dhow install "$action" || code=$?
expect "a plain http mirror is refused" test "$code" -ne 0
expect "and downloads nothing" missing "$CURL_LOG"
code=0
DHOW_RELEASE_BASE_URL=http://127.0.0.1.evil.example.com install "$action" || code=$?
expect "a host that only starts like localhost is refused" test "$code" -ne 0
code=0
DHOW_RELEASE_BASE_URL=http://127.0.0.1:8099/dist install "$action" || code=$?
expect "http on this machine is allowed for tests" test "$code" -eq 0
expect "with the http protocol only" contains "$CURL_LOG" "=http http://127.0.0.1:8099/dist/dhow_1.2.3_linux_amd64.tar.gz"
code=0
DHOW_RELEASE_BASE_URL=https://mirror.example.com/dhow install "$action" || code=$?
expect "an https mirror is used, and the checksum still decides" test "$code" -eq 0

link_dir=$(mktemp -d "$root/link.XXXXXX")
ln -s /bin/sh "$link_dir/dhow"
tar -czf "$DIST/dhow_1.2.3_linux_arm64.tar.gz" -C "$link_dir" dhow
action=$(fresh_action 1.2.3 linux arm64 "$(sha_of "$DIST/dhow_1.2.3_linux_arm64.tar.gz")")
code=0
install "$action" Linux ARM64 || code=$?
expect "an archive whose dhow is a link fails" test "$code" -ne 0

other=$(release 1.2.4 linux amd64)
action=$(fresh_action 1.2.4 linux amd64 "$other")
code=0
install "$action" || code=$?
expect "a binary that says another version fails" test "$code" -ne 0

empty_dir=$(mktemp -d "$root/empty.XXXXXX")
: >"$empty_dir/README"
tar -czf "$DIST/dhow_1.2.5_linux_amd64.tar.gz" -C "$empty_dir" README
action=$(fresh_action 1.2.5 linux amd64 "$(sha_of "$DIST/dhow_1.2.5_linux_amd64.tar.gz")")
code=0
install "$action" || code=$?
expect "an archive without dhow fails" test "$code" -ne 0

reply="$root/reply.json"
digest=$(printf 'a%.0s' {1..64})
printf '{"Name":"web","Digest":"%s","SizeBytes":1234,"Created":true,"Triggered":true,"RunId":"0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b","RunNumber":7,"RunUrl":"https://dhow.example.com/runs/7"}\n' "$digest" >"$reply"

token=dhow_ing_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789_-abcde
export STUB_REPLY=$reply STUB_ARGV=$root/argv STUB_ENV=$root/env

push() {
  rm -f -- "$STUB_MARKER" "$STUB_ARGV" "$STUB_ENV" "$root/push.out" "$root/push.log" "$root/summary"
  : >"$root/push.out"
  env -i PATH="$PATH" HOME="$root" STUB_MARKER="$STUB_MARKER" STUB_REPLY="$STUB_REPLY" STUB_ARGV="$STUB_ARGV" STUB_ENV="$STUB_ENV" \
    STUB_EXIT="${STUB_EXIT:-0}" \
    DHOW_BIN="$root/stub-dhow" DHOW_INGEST_TOKEN="${TOKEN-$token}" DHOW_ACTION_SERVER=https://dhow.example.com \
    DHOW_ACTION_PATH=dist/web DHOW_ACTION_NAME=web DHOW_ACTION_REF=main DHOW_ACTION_REF_KIND=branch \
    DHOW_ACTION_SHA=1111111111111111111111111111111111111111 DHOW_ACTION_TRIGGER="${TRIGGER-true}" DHOW_ACTION_RETRIES=3 DHOW_ACTION_ALLOW_PRIVILEGED="${ALLOW-}" \
    DHOW_PROV_REPOSITORY=Khaymah/dhow DHOW_PROV_RUN_ID=42 DHOW_PROV_RUN_ATTEMPT=1 DHOW_PROV_WORKFLOW="${WORKFLOW-CI}" \
    DHOW_PROV_ACTOR=sara DHOW_PROV_EVENT="${EVENT-push}" DHOW_PROV_RUN_URL=https://github.com/Khaymah/dhow/actions/runs/42 \
    GITHUB_EVENT_NAME="${EVENT-push}" GITHUB_OUTPUT="$root/push.out" GITHUB_STEP_SUMMARY="$root/summary" \
    bash "$here/scripts/push.sh" >"$root/push.log" 2>&1
}

code=0
push || code=$?
expect "a push runs the CLI" test "$code" -eq 0
expect "the path comes after --" contains "$STUB_ARGV" "--
dist/web"
expect "the token is not in the arguments" lacks "$STUB_ARGV" "$token"
expect "the token reaches the CLI through the environment" contains "$STUB_ENV" "DHOW_INGEST_TOKEN=$token"
expect "the token is not in the log" lacks "$root/push.log" "$token"
expect "the provenance names the repository" contains "$STUB_ARGV" "Repository=Khaymah/dhow"
expect "the provenance names the run" contains "$STUB_ARGV" "RunId=42"
expect "the provenance names the workflow and the actor" contains "$STUB_ARGV" "Actor=sara"
expect "the provenance says it came from GitHub Actions" contains "$STUB_ARGV" "Provider=github-actions"
expect "the digest is an output" contains "$root/push.out" "digest=$digest"
expect "the run id is an output" contains "$root/push.out" "run-id=0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"
expect "the run URL is an output" contains "$root/push.out" "run-url=https://dhow.example.com/runs/7"
expect "no run is suppressed by default" lacks "$STUB_ARGV" "--no-trigger"

TRIGGER=false
code=0
push || code=$?
unset TRIGGER
expect "trigger false stores without a run" contains "$STUB_ARGV" "--no-trigger"

TRIGGER=maybe
code=0
push || code=$?
unset TRIGGER
expect "any other trigger value fails" test "$code" -ne 0
expect "and the CLI never ran" missing "$STUB_MARKER"

EVENT=pull_request_target
code=0
push || code=$?
unset EVENT
expect "pull_request_target is refused" test "$code" -ne 0
expect "and the CLI never ran" missing "$STUB_MARKER"
expect "with a reason" contains "$root/push.log" "pull_request_target"

for event in workflow_run issue_comment; do
  EVENT=$event
  code=0
  push || code=$?
  expect "$event is refused by default" test "$code" -ne 0
  expect "and the CLI never ran for $event" missing "$STUB_MARKER"
  ALLOW=true
  code=0
  push || code=$?
  unset ALLOW
  unset EVENT
  expect "$event is allowed with the explicit opt-in" test "$code" -eq 0
done
code=0
EVENT=pull_request_target ALLOW=true push || code=$?
expect "pull_request_target stays refused even with the opt-in" test "$code" -ne 0

code=0
push || code=$?
expect "the summary shows the run URL as code" contains "$root/summary" 'Run: '"$tick"'https://dhow.example.com/runs/7'"$tick"
expect "the CLI never gets the option that packs the history" lacks "$STUB_ARGV" "--include-g"

TOKEN=
code=0
push || code=$?
unset TOKEN
expect "an empty token fails before the CLI runs" test "$code" -ne 0
expect "and the CLI never ran" missing "$STUB_MARKER"

WORKFLOW=$'Build\nand\ttest\x07'"$(printf 'x%.0s' {1..300})"
code=0
push || code=$?
unset WORKFLOW
expect "a hostile workflow name is cleaned" test "$code" -eq 0
workflow_arg=$(grep -F "Workflow=" "$STUB_ARGV")
expect "control characters are removed" test "$(printf '%s' "$workflow_arg" | LC_ALL=C tr -d '[:print:]' | wc -c | tr -d ' ')" -eq 0
expect "and the value is cut to fit the provenance limit" test "${#workflow_arg}" -le 220

STUB_EXIT=3
code=0
push || code=$?
unset STUB_EXIT
expect "the exit code of the CLI is the exit code of the step" test "$code" -eq 3
expect "and no output is written" lacks "$root/push.out" "digest="

printf '{"Digest":"%s","SizeBytes":0,"Created":false,"Triggered":false,"Reason":"NO_ARTIFACT_TRIGGER"}\n' "$digest" >"$reply"
code=0
push || code=$?
expect "a duplicate with false flags and no run is accepted" test "$code" -eq 0
expect "created=false is written" contains "$root/push.out" "created=false"
expect "triggered=false is written" contains "$root/push.out" "triggered=false"
expect "a zero size is written" contains "$root/push.out" "size=0"

for broken in \
  '{"Digest":"zz","SizeBytes":1,"Created":true,"Triggered":true}' \
  '{"Digest":"'"$digest"'","SizeBytes":"x","Created":true,"Triggered":true}' \
  '{"Digest":"'"$digest"'","SizeBytes":1,"Created":"yes","Triggered":true}' \
  '{"Digest":"'"$digest"'","SizeBytes":1,"Created":true,"Triggered":true,"RunId":"nope"}' \
  '{"Digest":"'"$digest"'","SizeBytes":1,"Created":true,"Triggered":true,"RunUrl":"https://a.example/\nINJECT=1"}' \
  '{"Digest":"'"$digest"'","SizeBytes":1,"Created":true,"Triggered":true,"RunUrl":"javascript:alert(1)"}' \
  '{"Digest":"'"$digest"'","SizeBytes":1,"Created":true,"Triggered":true,"RunUrl":"https://a.example/'"$tick"'x'"$tick"'"}' \
  '{"Digest":"'"$digest"'","SizeBytes":1,"Created":true,"Triggered":true,"RunUrl":"https://a.example/<img src=x>"}'; do
  printf '%s\n' "$broken" >"$reply"
  code=0
  push || code=$?
  expect "a result that is not valid is refused: ${broken:0:40}" test "$code" -ne 0
  expect "and nothing is written to the outputs" lacks "$root/push.out" "INJECT"
done

if ((failures > 0)); then
  printf '%d of %d checks failed\n' "$failures" "$checks"
  exit 1
fi
printf 'all %d checks passed\n' "$checks"
