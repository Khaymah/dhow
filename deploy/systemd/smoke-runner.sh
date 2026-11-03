#!/usr/bin/env bash
set -euo pipefail
umask 077

if [[ $# -ne 3 || ! -x "$1" || ! -x "$2" || -z "$3" ]]; then
  printf 'usage: %s PATH_TO_DHOW_SERVER PATH_TO_DHOW_RUNNER DATABASE_URL\n' "$0" >&2
  exit 2
fi
((EUID == 0)) || {
  printf 'smoke-runner.sh: run this as root on a disposable host; it installs a system user and a unit\n' >&2
  exit 2
}

server_bin=$1
runner_bin=$2
database_url=$3
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
port=${DHOW_SMOKE_PORT:-18080}
runner_port=${DHOW_SMOKE_RUNNER_PORT:-18443}
public_url="https://localhost:$port"
base="http://127.0.0.1:$port"
runner_url="https://127.0.0.1:$runner_port"
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
    tail -n 40 "$work/server.log" 2>&1 || true
  fi
  systemctl stop dhow-runner >/dev/null 2>&1 || true
  if [[ -n $server_pid ]]; then kill "$server_pid" 2>/dev/null || true; fi
  rm -rf -- "$work"
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
as_runner() {
  runuser -u dhow-runner -- env DHOW_RUNNER_CA_FILE=/etc/dhow-runner/ca.pem DHOW_RUNNER_SERVER_ORIGIN="$public_url" \
    DHOW_RUNNER_STATE_DIR=/var/lib/dhow-runner "$@"
}

printf 'Starting dhow-server with a TLS runner listener\n'
openssl req -x509 -newkey ed25519 -nodes -days 1 -subj /CN=127.0.0.1 -addext subjectAltName=IP:127.0.0.1 \
  -keyout "$work/runner.key" -out "$work/ca.pem" 2>/dev/null
"$server_bin" keys init --file "$work/dhow-keyring" >/dev/null
DHOW_DEV=true DHOW_PUBLIC_URL="$public_url" DHOW_LISTEN_ADDR="127.0.0.1:$port" DHOW_DATABASE_URL="$database_url" \
  DHOW_SECRET_KEY="$(openssl rand -hex 32)" DHOW_KEYRING_FILE="$work/dhow-keyring" DHOW_RUNNER_LISTEN_ADDR="127.0.0.1:$runner_port" \
  DHOW_RUNNER_TLS_CERT_FILE="$work/ca.pem" DHOW_RUNNER_TLS_KEY_FILE="$work/runner.key" \
  "$server_bin" >"$work/server.log" 2>&1 &
server_pid=$!
for _ in $(seq 60); do
  curl -sf "$base/api/v1/health/ready" >/dev/null 2>&1 && break
  sleep 1
done
expect "the server is ready" curl -sf "$base/api/v1/health/ready" -o /dev/null

setup_token=$(grep -o "$public_url/setup#[A-Za-z0-9_-]*" "$work/server.log" | tail -1 | cut -d'#' -f2)
password='runner smoke passphrase 2026'
status=$(api "$work/setup" POST /api/v1/setup "$(printf '{"SetupToken":"%s","OwnerName":"Runner Smoke","Email":"owner@runner.test","Password":"%s","OrganizationName":"Smoke","OrganizationSlug":"smoke"}' "$setup_token" "$password")")
expect "the first owner is set up" test "$status" = 200
status=$(api "$work/login" POST /api/v1/sessions "$(printf '{"Email":"owner@runner.test","Password":"%s"}' "$password")")
expect "the owner signs in" test "$status" = 200
cookie=$(sed -n 's/^[Ss]et-[Cc]ookie: \(__Host-dhow_session=[^;]*\);.*/\1/p' "$work/login.head" | tr -d '\r' | head -1)
session=(--header "Cookie: $cookie" --header "X-CSRF-Token: $(json_field "$work/login.body" CsrfToken)")
api "$work/me" GET /api/v1/me '' "${session[@]}" >/dev/null
org_id=$(json_field "$work/me.body" OrganizationId)
status=$(api "$work/rrt" POST "/api/v1/organizations/$org_id/runner-registration-tokens" '{"Labels":["pool=smoke"]}' "${session[@]}")
expect "a registration token is created" test "$status" = 201

printf 'Installing the shipped unit\n'
id dhow-runner >/dev/null 2>&1 || useradd --system --home-dir /var/lib/dhow-runner --shell /usr/sbin/nologin dhow-runner
sed -i '/^dhow-runner:/d' /etc/subuid /etc/subgid
printf 'dhow-runner:1300000:1048576\n' | tee -a /etc/subuid >>/etc/subgid
install -d -m 0700 -o dhow-runner -g dhow-runner /var/lib/dhow-runner
install -d -m 0750 -o root -g dhow-runner /etc/dhow-runner
install -m 0644 "$work/ca.pem" /etc/dhow-runner/ca.pem
install -m 0755 "$runner_bin" /usr/local/bin/dhow-runner
install -m 0644 "$here/dhow-runner.service" /etc/systemd/system/dhow-runner.service
{
  printf 'DHOW_RUNNER_CA_FILE=/etc/dhow-runner/ca.pem\nDHOW_RUNNER_SERVER_ORIGIN=%s\n' "$public_url"
  printf '%s\n' "${DHOW_SMOKE_RUNNER_ENV:-}"
} >/etc/dhow-runner/runner.env
chmod 0640 /etc/dhow-runner/runner.env
chgrp dhow-runner /etc/dhow-runner/runner.env
systemctl daemon-reload

json_field "$work/rrt.body" Token | as_runner /usr/local/bin/dhow-runner register --url "$runner_url" --token-file - --name smoke >"$work/register.log"
expect "the runner registers as the dhow-runner user" grep -q 'Registered runner' "$work/register.log"
runner_id=$(sed -n 's/^Registered runner \([0-9a-f-]*\) .*/\1/p' "$work/register.log")

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
seen_after() {
  local before=$1
  for _ in $(seq 90); do
    [[ $(runner_field LastSeenAt) > $before ]] && return 0
    sleep 1
  done
  return 1
}
key_version() {
  api "$work/runner" GET "/api/v1/organizations/$org_id/runners/$runner_id" '' "${session[@]}" >/dev/null
  json_number "$work/runner.body" KeyVersion
}

systemctl start dhow-runner
expect "the unit starts" systemctl is-active --quiet dhow-runner
expect "the runner comes online under the unit" wait_online
expect "the runner runs as dhow-runner" test "$(ps -o user= -p "$(systemctl show -p MainPID --value dhow-runner)")" = dhow-runner
expect "no new privileges is off, so newuidmap works" grep -q 'NoNewPrivs:[[:space:]]*0' "/proc/$(systemctl show -p MainPID --value dhow-runner)/status"

printf 'Rotating the keys while the unit is stopped\n'
systemctl stop dhow-runner
before=$(runner_field LastSeenAt)
as_runner /usr/local/bin/dhow-runner rotate-keys >"$work/rotate.log"
expect "rotate-keys reports key version 2" grep -q 'key version 2' "$work/rotate.log"
expect "the server has key version 2" test "$(key_version)" = 2
systemctl start dhow-runner
expect "the runner signs in again with the new key" seen_after "$before"
expect "the runner is online after the rotation" wait_online
expect "the identity stays private" test "$(stat -c '%a %U' /var/lib/dhow-runner/identity)" = "600 dhow-runner"
expect "no pending identity is left" test ! -e /var/lib/dhow-runner/identity.pending

systemd-analyze security dhow-runner.service --no-pager | tail -1 | tee "$work/security.txt"
expect "systemd-analyze rates the installed unit MEDIUM or better" grep -Eq ': [0-6]\.[0-9] (MEDIUM|OK|SAFE|PERFECT)' "$work/security.txt"

if ((failures != 0)); then
  printf '\n%d runner unit check(s) failed\n' "$failures" >&2
  exit 1
fi
printf '\nThe runner runs under the shipped unit\n'
