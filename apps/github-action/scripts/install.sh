#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() {
  printf '::error::%s\n' "$1" >&2
  exit 1
}

action_path=${ACTION_PATH:?ACTION_PATH is not set}
pin="$action_path/dhow.pin"
[[ -f $pin && ! -L $pin ]] || fail "the pin file is missing from this copy of the action"

pinned() {
  awk -F= -v key="$1" '$1 == key { print substr($0, length(key) + 2); found = 1; exit } END { exit !found }' "$pin"
}

version=$(pinned version) || fail "the pin file has no version"
if [[ $version == unreleased ]]; then
  fail "this copy of the action is not pinned to a Dhow release. Use Khaymah/dhow-action at a release tag."
fi
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || fail "the pinned version is not a semantic version"

case ${RUNNER_OS:-} in
  Linux) os=linux ;;
  macOS) os=darwin ;;
  *) fail "this action runs on Linux and macOS runners, not ${RUNNER_OS:-an unknown system}" ;;
esac
case ${RUNNER_ARCH:-} in
  X64) arch=amd64 ;;
  ARM64) arch=arm64 ;;
  *) fail "this action runs on x64 and ARM64 runners, not ${RUNNER_ARCH:-an unknown architecture}" ;;
esac

expected=$(pinned "${os}_${arch}") || fail "no checksum is pinned for ${os}/${arch}"
[[ $expected =~ ^[0-9a-f]{64}$ ]] || fail "the pinned checksum for ${os}/${arch} is not a SHA-256"

base=${DHOW_RELEASE_BASE_URL:-https://github.com/Khaymah/dhow/releases/download/v${version}}
base=${base%/}
proto='=https'
if [[ $base != https://* ]]; then
  [[ $base =~ ^http://(127\.0\.0\.1|localhost)(:[0-9]+)?(/|$) ]] || fail "the release base URL must be https, or http on this machine"
  proto='=http'
fi

name="dhow_${version}_${os}_${arch}.tar.gz"
work=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/dhow-action.XXXXXX")
archive="$work/$name"

curl --fail --silent --show-error --location --proto "$proto" --proto-redir "$proto" \
  --max-time 300 --retry 3 --output "$archive" --url "$base/$name" ||
  fail "could not download $name"

if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum -- "$archive" | cut -d' ' -f1)
else
  actual=$(shasum -a 256 -- "$archive" | cut -d' ' -f1)
fi
if [[ $actual != "$expected" ]]; then
  rm -f -- "$archive"
  fail "the SHA-256 of $name is $actual, not the $expected pinned in this action. Nothing was run."
fi

tar -xzf "$archive" -C "$work" -- dhow || fail "$name has no dhow binary"
rm -f -- "$archive"
bin="$work/dhow"
[[ -f $bin && ! -L $bin ]] || fail "the dhow in $name is not a regular file"
chmod 0755 -- "$bin"

banner=$("$bin" version) || fail "the downloaded dhow does not run"
[[ $banner == "dhow ${version} "* ]] || fail "the downloaded dhow says it is not version ${version}"

if [[ -n ${GITHUB_OUTPUT:-} ]]; then
  printf 'bin=%s\nversion=%s\n' "$bin" "$version" >>"$GITHUB_OUTPUT"
fi
printf 'dhow %s (%s/%s) verified against the pinned SHA-256\n' "$version" "$os" "$arch"
