#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  printf 'usage: %s build <folder-to-upload> | tamper | stop\n' "$0" >&2
  printf 'build: builds the CLI as release 0.0.0-e2e, packs it like the release does, pins its SHA-256 in apps/github-action/dhow.pin,\n' >&2
  printf '       serves the archive on 127.0.0.1 and prints release-base-url and expected-digest as key=value lines.\n' >&2
  printf 'tamper: pins a wrong SHA-256 for this platform. stop: stops the server. Settings: DHOW_E2E_RELEASE_DIR, DHOW_E2E_RELEASE_PORT (8601).\n' >&2
  exit 2
}
(($# >= 1)) || usage

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
dir=${DHOW_E2E_RELEASE_DIR:-${RUNNER_TEMP:-${TMPDIR:-/tmp}}/dhow-ingest-release}
port=${DHOW_E2E_RELEASE_PORT:-8601}
pin="$root/apps/github-action/dhow.pin"
version=0.0.0-e2e
os=$(go env GOOS)
arch=$(go env GOARCH)

emit() {
  if [[ -n ${GITHUB_OUTPUT:-} ]]; then
    printf '%s\n' "$1" >>"$GITHUB_OUTPUT"
  else
    printf '%s\n' "$1"
  fi
}

sha_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- "$1" | cut -d' ' -f1
  else
    shasum -a 256 -- "$1" | cut -d' ' -f1
  fi
}

build() {
  (($# == 1)) || usage
  local folder=$1 archive sum digest
  rm -rf -- "$dir"
  install -d -m 0755 "$dir/pack" "$dir/dist"
  (cd "$root" && CGO_ENABLED=0 go build -trimpath \
    -ldflags "-X github.com/Khaymah/dhow/internal/buildinfo.version=$version" -o "$dir/pack/dhow" ./apps/cli/cmd/dhow)
  archive="$dir/dist/dhow_${version}_${os}_${arch}.tar.gz"
  tar -czf "$archive" -C "$dir/pack" dhow
  sum=$(sha_of "$archive")
  printf 'version=%s\n%s_%s=%s\n' "$version" "$os" "$arch" "$sum" >"$pin"
  digest=$("$dir/pack/dhow" artifact push --dry-run --format json --name web --ref main \
    --sha 1111111111111111111111111111111111111111 -- "$folder" | jq -r '.Digest')
  [[ $digest =~ ^[0-9a-f]{64}$ ]] || { printf 'the dry run printed no digest\n' >&2; exit 1; }
  python3 -m http.server "$port" --bind 127.0.0.1 --directory "$dir/dist" >"$dir/http.log" 2>&1 &
  printf '%s\n' "$!" >"$dir/http.pid"
  for _ in $(seq 50); do
    curl --silent --fail --output /dev/null --url "http://127.0.0.1:$port/dhow_${version}_${os}_${arch}.tar.gz" && break
    sleep 0.2
  done
  emit "$(printf 'release-base-url=http://127.0.0.1:%s\nexpected-digest=%s\n' "$port" "$digest")"
}

tamper() {
  printf 'version=%s\n%s_%s=%s\n' "$version" "$os" "$arch" "$(printf '0%.0s' {1..64})" >"$pin"
}

stop() {
  if [[ -f $dir/http.pid ]]; then
    kill "$(cat -- "$dir/http.pid")" 2>/dev/null || true
  fi
  git -C "$root" checkout -- apps/github-action/dhow.pin
}

case $1 in
  build) shift; build "$@" ;;
  tamper) tamper ;;
  stop) stop ;;
  *) usage ;;
esac
