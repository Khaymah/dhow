#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 || -z "$1" || ! -x "$2" ]]; then
  printf 'usage: %s BASE_IMAGE PATH_TO_DHOW_SERVER\n' "$0" >&2
  exit 2
fi

base=$1
binary=$2
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
slug=$(tr -c 'a-z0-9\n' '-' <<<"${base,,}")
name=dhow-e2e-$slug
tag=dhow-e2e:$slug
work=$(mktemp -d)
failures=0
read -r -a build_flags <<<"${DHOW_E2E_BUILD_FLAGS:-}"
read -r -a run_flags <<<"${DHOW_E2E_RUN_FLAGS:---cgroupns=private}"
read -r -a exec_flags <<<"${DHOW_E2E_EXEC_FLAGS:-}"

in_box() { docker exec "${exec_flags[@]}" "$name" "$@"; }
in_box_stdin() { docker exec -i "${exec_flags[@]}" "$name" "$@"; }

cleanup() {
  local status=$?
  if ((status != 0 || failures != 0)); then
    docker exec "$name" journalctl --no-pager -n 80 -u dhow 2>&1 || true
  fi
  docker rm -f "$name" >/dev/null 2>&1 || true
  rm -rf -- "$work"
  exit "$status"
}
trap cleanup EXIT

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
absent() { ! in_box test -e "$1"; }
lacks() { ! grep -qF -- "$1" "$2"; }
journal_lacks() { ! in_box journalctl --no-pager | grep -qF -- "$1"; }
journal_has_keyring() { in_box journalctl -u dhow -o cat --no-pager | grep -q '"msg":"keyring loaded"'; }
journal_has_setup_link() { in_box journalctl -u dhow -o cat --no-pager | grep -q '"setup_url":"https://dhow.e2e.test/setup#'; }
every_step_skipped() { ! grep -E '^\[[0-9]+/[0-9]+\] [^:]*$' "$1"; }
pgdg_installed() { in_box test -x /usr/lib/postgresql/18/bin/postgres || in_box test -x /usr/pgsql-18/bin/postgres; }
nologin() { in_box getent passwd dhow | grep -q ':/usr/sbin/nologin$'; }
only_mptcp_advisory() {
  grep -qx 'Everything looks good.' "$1" && return 0
  grep -qx 'Nothing blocks Dhow, but read the warnings above.' "$1" || return 1
  ! grep -E '^  (WARN|FAIL) ' "$1" | grep -v -e 'MPTCP, CVE-2026-98432' -e 'The keyring was never exported'
}
doctor_clean() {
  in_box /root/dhow-server doctor >"$work/doctor.log"
  cat -- "$work/doctor.log"
  only_mptcp_advisory "$work/doctor.log"
}
unit_unknown() { ! in_box systemctl cat dhow.service >/dev/null 2>&1; }
user_gone() { ! in_box getent passwd dhow >/dev/null; }
database_gone() { ! docker exec -w / "$name" runuser -u postgres -- psql -XtA -d postgres -c "SELECT 1 FROM pg_database WHERE datname = 'dhow'" | grep -q 1; }
mode_is() { [[ $(in_box stat -c '%a %U:%G' "$1") == "$2" ]]; }

printf 'Booting %s with systemd\n' "$base"
docker build --quiet "${build_flags[@]}" --build-arg "BASE=$base" -t "$tag" \
  -f "${DHOW_E2E_DOCKERFILE:-$here/Dockerfile}" "$here" >/dev/null
docker run -d --name "$name" --privileged "${run_flags[@]}" --tmpfs /run --tmpfs /run/lock "$tag" >/dev/null
for _ in $(seq 60); do
  state=$(docker exec "$name" systemctl is-system-running 2>/dev/null || true)
  [[ $state == running || $state == degraded ]] && break
  sleep 1
done
expect "systemd is up ($state)" test "$state" = running -o "$state" = degraded
docker cp "$binary" "$name:/root/dhow-server"
docker cp "$here/answers.yaml" "$name:/root/answers.yaml"

printf 'Installing from the answers file\n'
in_box /root/dhow-server install --answers /root/answers.yaml --yes | tee "$work/install.log"
expect "the installer printed the setup link" grep -q 'https://dhow.e2e.test/setup#' "$work/install.log"
expect "dhow is active" in_box systemctl is-active --quiet dhow
expect "dhow is enabled" in_box systemctl is-enabled --quiet dhow
expect "/api/v1/health/ready answers 200" in_box env DHOW_LISTEN_ADDR=127.0.0.1:8080 /usr/local/bin/dhow-server healthcheck
expect "the journal holds the setup link" journal_has_setup_link
expect "PostgreSQL 18 came from PGDG" pgdg_installed
for f in /etc/dhow/credentials/dhow-secret-key /etc/dhow/credentials/dhow-database-url /etc/dhow/credentials/dhow-keyring /etc/dhow/dhow.env /etc/dhow/answers.yaml; do
  expect "$f is 600 root:root" mode_is "$f" "600 root:root"
done
expect "/etc/dhow is 700 root:root" mode_is /etc/dhow "700 root:root"
expect "the service user has no login shell" nologin
key=$(in_box cat /etc/dhow/credentials/dhow-secret-key)
expect "the secret key never reached the output" lacks "$key" "$work/install.log"
expect "the secret key never reached the journal" journal_lacks "$key"
kek=$(in_box cat /etc/dhow/credentials/dhow-keyring)
kek=${kek##* }
expect "the keyring never reached the output" lacks "$kek" "$work/install.log"
expect "the keyring never reached the journal" journal_lacks "$kek"
expect "the server registered the keyring" journal_has_keyring
in_box systemd-analyze security dhow.service --no-pager | tail -1 | tee "$work/security.txt"
expect "systemd-analyze rates the unit OK or better" grep -Eq 'OK|SAFE|PERFECT' "$work/security.txt"

printf 'Running the installer again\n'
in_box /root/dhow-server install --answers /root/answers.yaml --yes >"$work/rerun.log"
expect "a re-run keeps the secret key" test "$(in_box cat /etc/dhow/credentials/dhow-secret-key)" = "$key"
expect "a re-run keeps the keyring" in_box grep -qF -- "$kek" /etc/dhow/credentials/dhow-keyring
expect "a re-run skips every step" every_step_skipped "$work/rerun.log"

printf 'Doctor\n'
expect "doctor reports clean" doctor_clean

printf 'Uninstall\n'
in_box /root/dhow-server uninstall --yes >"$work/uninstall.log"
expect "the unit is gone" absent /etc/systemd/system/dhow.service
expect "systemd no longer knows dhow" unit_unknown
expect "the binary is gone" absent /usr/local/bin/dhow-server
expect "the config is gone" absent /etc/dhow/dhow.env
expect "the answers are kept for a later purge" in_box test -s /etc/dhow/answers.yaml
expect "the secrets are kept" in_box test -s /etc/dhow/credentials/dhow-secret-key
expect "the keyring is kept" in_box test -s /etc/dhow/credentials/dhow-keyring
printf 'delete dhow data\n' | in_box_stdin /root/dhow-server uninstall --purge >"$work/purge.log"
expect "purge removes /etc/dhow" absent /etc/dhow
expect "purge removes the dhow user" user_gone
expect "purge drops the database" database_gone

if ((failures != 0)); then
  printf '\n%d check(s) failed on %s\n' "$failures" "$base" >&2
  exit 1
fi
printf '\nAll installer checks passed on %s\n' "$base"
