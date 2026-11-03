#!/usr/bin/env bash
set -euo pipefail

web_port=${DHOW_DEV_WEB_PORT:-5173}
poll=${DHOW_DEV_POLL_SECONDS:-1}
bin=/tmp/dhow-server
pid=
watched=(apps/server internal packages/proto/gen/go go.mod go.sum)

set -a
# shellcheck source=/dev/null
. /src/.env.dev
set +a
export DHOW_DEV=true
export DHOW_LISTEN_ADDR=0.0.0.0:8080
export DHOW_PUBLIC_URL="http://localhost:${web_port}"
export DHOW_DATABASE_URL="postgres://dhow:dhow-dev-only@db:5432/dhow?sslmode=disable"
export DHOW_TEST_DATABASE_URL="postgres://dhow:dhow-dev-only@db:5432/dhow_test?sslmode=disable"
install -m 600 /src/.dhow-dev-keyring /tmp/dhow-keyring
export DHOW_KEYRING_FILE=/tmp/dhow-keyring

say() {
  printf '\033[1;36m[dev]\033[0m %s\n' "$*"
}

fingerprint() {
  {
    find "${watched[@]}" -type f -not -name '*_test.go' -not -path 'apps/server/internal/App/WebDist/*' \
      -printf '%T@ %s %p\n' 2>/dev/null || true
  } | sort | md5sum
}

announce() {
  local url=$1 tries=0
  until curl -s -o /dev/null --max-time 2 "http://web:${web_port}/"; do
    tries=$((tries + 1))
    if ((tries > 300)); then
      break
    fi
    sleep 2
  done
  printf '\n\033[1;32m%s\033[0m\n' "=================================================================="
  printf '\033[1;32m  Dhow is ready. Open this link within one hour to create the owner:\033[0m\n'
  printf '\033[1;32m  %s\033[0m\n' "$url"
  printf '\033[1;32m%s\033[0m\n\n' "=================================================================="
}

relay() {
  local line url
  while IFS= read -r line; do
    printf '%s\n' "$line"
    if [[ $line == *setup_url* ]]; then
      url=$(printf '%s' "$line" | grep -oE 'https?://[^" ]+/setup#[^" ]+' | head -n1 || true)
      if [[ -n $url ]]; then
        announce "$url" &
      fi
    fi
  done
}

stop() {
  if [[ -n $pid ]] && kill -0 "$pid" 2>/dev/null; then
    kill -TERM "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  pid=
}

build() {
  say "building the server"
  if go build -o "$bin.new" ./apps/server/cmd/dhow-server; then
    return 0
  fi
  say "the build failed; the previous server keeps running until a save builds again"
  return 1
}

run() {
  stop
  mv "$bin.new" "$bin"
  say "starting the server; the web app is on http://localhost:${web_port}"
  "$bin" > >(relay) 2>&1 &
  pid=$!
}

trap 'stop; exit 0' TERM INT

last=$(fingerprint)
if build; then
  run
fi
while sleep "$poll"; do
  now=$(fingerprint)
  if [[ $now != "$last" ]]; then
    last=$now
    say "a change was saved; rebuilding"
    if build; then
      run
    fi
  fi
done
