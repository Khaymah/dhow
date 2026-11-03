#!/usr/bin/env bash
set -euo pipefail
umask 077

if [[ $# -ne 3 || ! -x "$1" || ! -x "$2" || -z "$3" ]]; then
  printf 'usage: %s PATH_TO_DHOW_SERVER PATH_TO_DHOW_RUNNER DATABASE_URL\n' "$0" >&2
  exit 2
fi
((EUID == 0)) || {
  printf 'smoke-install.sh: run this as root on a disposable host; it installs a system user and a unit\n' >&2
  exit 2
}

server_bin=$1
runner_bin=$2
database_url=$3
port=${DHOW_SMOKE_PORT:-18080}
runner_port=${DHOW_SMOKE_RUNNER_PORT:-18443}
driver=${DHOW_SMOKE_DRIVER:-podman}
identity=${DHOW_SMOKE_IDENTITY:-auto}
image=${DHOW_SMOKE_IMAGE:-mirror.gcr.io/library/busybox@sha256:a7dbc319b1438115a207e11930dc4bb3e643dcb3ec0bdd694bb36cd8fad289ec}
public_url="https://localhost:$port"
base="http://127.0.0.1:$port"
runner_url="https://127.0.0.1:$runner_port"
shared=/usr/local/share/dhow-smoke
work=$(mktemp -d)
chmod 0755 "$work"
server_pid=
failures=0

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

cleanup() {
  local status=$?
  if ((status != 0 || failures != 0)); then
    journalctl --no-pager -n 60 -u dhow-runner 2>&1 || true
    if command -v ausearch >/dev/null 2>&1; then ausearch -m avc -ts recent 2>&1 | tail -n 30 || true; fi
    tail -n 40 "$work/server.log" 2>&1 || true
  fi
  systemctl disable --now dhow-runner >/dev/null 2>&1 || true
  if [[ -n $server_pid ]]; then kill "$server_pid" 2>/dev/null || true; fi
  rm -rf -- "$work" "$shared"
  exit "$status"
}
trap cleanup EXIT

json_field() { grep -o "\"$2\":\"[^\"]*\"" "$1" | head -1 | cut -d'"' -f4; }
json_number() { grep -o "\"$2\":[0-9]*" "$1" | head -1 | cut -d: -f2; }
api() {
  local prefix=$1 method=$2 path=$3 body=$4
  shift 4
  curl --silent --show-error --max-time 10 --dump-header "$prefix.head" --output "$prefix.body" --write-out '%{http_code}' \
    --request "$method" --header 'Content-Type: application/json' --header "Origin: $public_url" "$@" \
    ${body:+--data "$body"} -- "$base$path"
}
absent() { [[ ! -e $1 ]]; }
lacks() { ! grep -qF -- "$1" "$2"; }
every_step_skipped() { ! grep -E '^\[[0-9]+/[0-9]+\] [^:]*$' "$1"; }
user_gone() { ! getent passwd dhow-runner >/dev/null; }
subids_gone() { ! grep -q '^dhow-runner:' /etc/subuid /etc/subgid; }
unit_unknown() { ! systemctl cat dhow-runner.service >/dev/null 2>&1; }

printf 'Starting dhow-server with a TLS runner listener\n'
install -d -m 0755 "$shared"
openssl req -x509 -newkey ed25519 -nodes -days 1 -subj /CN=127.0.0.1 -addext subjectAltName=IP:127.0.0.1 \
  -keyout "$work/runner.key" -out "$shared/ca.pem" 2>/dev/null
chmod 0644 "$shared/ca.pem"
"$server_bin" keys init --file "$work/dhow-keyring" >/dev/null
DHOW_DEV=true DHOW_PUBLIC_URL="$public_url" DHOW_LISTEN_ADDR="127.0.0.1:$port" DHOW_DATABASE_URL="$database_url" \
  DHOW_SECRET_KEY="$(openssl rand -hex 32)" DHOW_KEYRING_FILE="$work/dhow-keyring" DHOW_RUNNER_LISTEN_ADDR="127.0.0.1:$runner_port" \
  DHOW_RUNNER_TLS_CERT_FILE="$shared/ca.pem" DHOW_RUNNER_TLS_KEY_FILE="$work/runner.key" \
  "$server_bin" >"$work/server.log" 2>&1 &
server_pid=$!
for _ in $(seq 60); do
  curl -sf "$base/api/v1/health/ready" >/dev/null 2>&1 && break
  sleep 1
done
expect "the server is ready" curl -sf "$base/api/v1/health/ready" -o /dev/null

setup_token=$(grep -o "$public_url/setup#[A-Za-z0-9_-]*" "$work/server.log" | tail -1 | cut -d'#' -f2)
password='runner install smoke passphrase'
status=$(api "$work/setup" POST /api/v1/setup "$(printf '{"SetupToken":"%s","OwnerName":"Install Smoke","Email":"owner@install.test","Password":"%s","OrganizationName":"Smoke","OrganizationSlug":"smoke"}' "$setup_token" "$password")")
expect "the first owner is set up" test "$status" = 200
status=$(api "$work/login" POST /api/v1/sessions "$(printf '{"Email":"owner@install.test","Password":"%s"}' "$password")")
expect "the owner signs in" test "$status" = 200
cookie=$(sed -n 's/^[Ss]et-[Cc]ookie: \(__Host-dhow_session=[^;]*\);.*/\1/p' "$work/login.head" | tr -d '\r' | head -1)
session=(--header "Cookie: $cookie" --header "X-CSRF-Token: $(json_field "$work/login.body" CsrfToken)")
api "$work/me" GET /api/v1/me '' "${session[@]}" >/dev/null
org_id=$(json_field "$work/me.body" OrganizationId)
status=$(api "$work/rrt" POST "/api/v1/organizations/$org_id/runner-registration-tokens" '{"Labels":["pool=smoke"]}' "${session[@]}")
expect "a registration token is created" test "$status" = 201
token=$(json_field "$work/rrt.body" Token)
printf '%s\n' "$token" >"$work/token"

{
  printf 'url: %s\nserver_origin: %s\nca_file: %s/ca.pem\nname: install-smoke\ntoken_file: %s/token\n' \
    "$runner_url" "$public_url" "$shared" "$work"
  printf 'driver: %s\nidentity: %s\ninstall_packages: true\n' "$driver" "$identity"
  if [[ $driver == fake ]]; then
    printf '{"Steps":[{"Output":[{"Data":"hello from the fake driver\\n"}]}]}\n' >"$shared/recording.json"
    chmod 0644 "$shared/recording.json"
    printf 'env:\n  DHOW_RUNNER_FAKE_RECORDING: %s/recording.json\n' "$shared"
  fi
} >"$work/answers.yaml"

runner_field() {
  api "$work/runner" GET "/api/v1/organizations/$org_id/runners/$runner_id" '' "${session[@]}" >/dev/null
  json_field "$work/runner.body" "$1"
}
wait_online() {
  for _ in $(seq 90); do
    [[ $(runner_field Status) == online ]] && return 0
    sleep 1
  done
  return 1
}
wait_ready() {
  local id
  for _ in $(seq 90); do
    id=$(systemctl show -p InvocationID --value dhow-runner)
    if [[ -n $id ]] && journalctl --no-pager --output cat "_SYSTEMD_INVOCATION_ID=$id" | grep -q '"msg":"the runner is ready"'; then
      return 0
    fi
    sleep 1
  done
  return 1
}
online_after_start() { wait_ready && wait_online; }
job_state() {
  api "$work/job" GET "/api/v1/organizations/$org_id/jobs/$1" '' "${session[@]}" >/dev/null
  json_field "$work/job.body" State
}
job_succeeds() {
  local status id state
  status=$(api "$work/new-job" POST "/api/v1/organizations/$org_id/jobs" \
    "$(printf '{"Name":"install smoke","Spec":{"Image":"%s","Steps":[{"Name":"hello","Run":"echo hello"}]}}' "$image")" "${session[@]}")
  [[ $status == 201 ]] || return 1
  id=$(json_field "$work/new-job.body" Id)
  for _ in $(seq 180); do
    state=$(job_state "$id")
    case $state in
      succeeded) return 0 ;;
      failed | canceled | lost | timed_out)
        printf '    job %s ended %s\n' "$id" "$state" >&2
        return 1
        ;;
    esac
    sleep 1
  done
  return 1
}
only_mptcp_advisory() {
  grep -qx 'Everything looks good.' "$1" && return 0
  grep -qx 'Nothing blocks Dhow, but read the warnings above.' "$1" || return 1
  ! grep -E '^  (WARN|FAIL) ' "$1" | grep -v 'MPTCP, CVE-2026-98432'
}
doctor_clean() {
  dhow-runner doctor >"$work/doctor.log" 2>&1 || {
    cat -- "$work/doctor.log"
    return 1
  }
  only_mptcp_advisory "$work/doctor.log"
}
keys_off_disk() {
  local seed enc
  seed=$(systemd-creds decrypt --name=identity /var/lib/dhow-runner/identity.cred - | grep -o '"SigningSeed":"[^"]*"' | cut -d'"' -f4)
  enc=$(systemd-creds decrypt --name=identity /var/lib/dhow-runner/identity.cred - | grep -o '"EncryptionKey":"[^"]*"' | cut -d'"' -f4)
  [[ -n $seed && -n $enc ]] || return 1
  ! grep -rqsF -e "$seed" -e "${enc:0:64}" -- /var/lib/dhow-runner /etc /root /tmp /var/tmp /usr/local/bin &&
    ! journalctl --no-pager | grep -qF -e "$seed" -e "${enc:0:64}"
}

printf 'Installing from the answers file\n'
"$runner_bin" install --answers "$work/answers.yaml" | tee "$work/install.log"
runner_id=$(sed -n 's/^Registered runner \([0-9a-f-]*\) .*/\1/p' "$work/install.log")
expect "install registered the runner" test -n "$runner_id"
expect "the runner is online" online_after_start
expect "the unit is active and enabled" systemctl is-enabled --quiet dhow-runner
expect "the runner runs as dhow-runner" test "$(ps -o user= -p "$(systemctl show -p MainPID --value dhow-runner)")" = dhow-runner
expect "the token never reached the output" lacks "$token" "$work/install.log"
state_dir_private() { [[ $(stat -c '%a %U' /var/lib/dhow-runner) =~ ^7(00|11)\ dhow-runner$ ]]; }
expect "the state directory is 700 or 711 dhow-runner" state_dir_private
expect "runner.env is 640 root:dhow-runner" test "$(stat -c '%a %U:%G' /etc/dhow-runner/runner.env)" = "640 root:dhow-runner"
expect "the runner user has its sub-ids" grep -q '^dhow-runner:[0-9]*:1048576$' /etc/subuid
if [[ $identity != file ]] && systemd-creds encrypt --name=probe - - </dev/null >/dev/null 2>&1; then
  expect "the identity is encrypted with systemd-creds" test -s /var/lib/dhow-runner/identity.cred
  expect "no plain identity is on disk" absent /var/lib/dhow-runner/identity
  expect "the unit loads the credential" grep -q '^LoadCredentialEncrypted=identity:/var/lib/dhow-runner/identity.cred$' \
    /etc/systemd/system/dhow-runner.service.d/10-dhow-identity.conf
  expect "grep finds the private keys nowhere on disk or in the journal" keys_off_disk
else
  expect "without systemd-creds the identity is a 0600 file" test "$(stat -c '%a %U' /var/lib/dhow-runner/identity)" = "600 dhow-runner"
  expect "the install said the identity is not encrypted" grep -Eq 'cannot encrypt on this host|You chose identity: file' "$work/install.log"
  expect "no credential drop-in is installed" absent /etc/systemd/system/dhow-runner.service.d/10-dhow-identity.conf
fi
systemd-analyze security dhow-runner.service --no-pager | tail -1 | tee "$work/security.txt"
expect "systemd-analyze rates the installed unit 6.9 or lower" grep -Eq ': [0-6]\.[0-9] (MEDIUM|OK|SAFE|PERFECT)' "$work/security.txt"

printf 'Doctor\n'
expect "doctor reports clean" doctor_clean
expect "doctor printed no secret" lacks "$token" "$work/doctor.log"
printf 'A job\n'
expect "a job succeeds on the installed runner" job_succeeds

printf 'Rotating the keys of the installed runner\n'
systemctl stop dhow-runner
dhow-runner rotate-keys >"$work/rotate.log"
expect "rotate-keys reports key version 2" grep -q 'key version 2' "$work/rotate.log"
systemctl start dhow-runner
expect "the runner is online with its new key" online_after_start
if [[ -e /var/lib/dhow-runner/identity.cred ]]; then
  expect "the rotated identity stays encrypted" absent /var/lib/dhow-runner/identity
  expect "the rotated keys are nowhere on disk" keys_off_disk
else
  expect "the rotated identity stays a 0600 file of dhow-runner" test "$(stat -c '%a %U' /var/lib/dhow-runner/identity)" = "600 dhow-runner"
fi
expect "doctor is clean after the rotation" doctor_clean

printf 'Running install again\n'
"$runner_bin" install --answers "$work/answers.yaml" >"$work/rerun.log"
expect "a re-run skips every step" every_step_skipped "$work/rerun.log"
expect "the runner is still online" wait_online

printf 'Uninstall\n'
dhow-runner uninstall --yes | tee "$work/uninstall.log"
expect "uninstall revoked the runner itself" grep -q "Revoked runner $runner_id" "$work/uninstall.log"
expect "the server shows the runner revoked" test "$(runner_field State)" = revoked
for path in /etc/systemd/system/dhow-runner.service /etc/systemd/system/dhow-runner.service.d /var/lib/dhow-runner \
  /etc/dhow-runner /usr/local/bin/dhow-runner; do
  expect "$path is gone" absent "$path"
done
expect "systemd no longer knows dhow-runner" unit_unknown
expect "the user stays without --remove-user" getent passwd dhow-runner
"$runner_bin" uninstall --yes --remove-user >"$work/remove-user.log"
expect "--remove-user deletes the user" user_gone
expect "--remove-user deletes its sub-ids" subids_gone
"$runner_bin" uninstall --yes >"$work/again.log"
expect "a second uninstall finds nothing" grep -q 'Nothing to remove' "$work/again.log"

if ((failures != 0)); then
  printf '\n%d install check(s) failed\n' "$failures" >&2
  exit 1
fi
printf '\nInstall, doctor, a job, rotate-keys and uninstall passed\n'
