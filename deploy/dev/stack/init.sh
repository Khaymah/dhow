#!/usr/bin/env bash
set -euo pipefail
umask 077

file=/src/.env.dev
keyring=/src/.dhow-dev-keyring
if [[ ! -e $keyring ]]; then
  tmp=$(mktemp /src/.dhow-dev-keyring.XXXXXX)
  printf 'kek 1 %s\n' "$(head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=\n')" >"$tmp"
  chown "$(stat -c %u:%g /src)" "$tmp"
  mv "$tmp" "$keyring"
  printf 'init: wrote .dhow-dev-keyring with a new development key\n'
fi
if [[ -e $file ]]; then
  printf 'init: keeping the existing .env.dev\n'
  exit 0
fi

url="postgres://dhow:dhow-dev-only@127.0.0.1:${DHOW_DEV_DB_PORT:-55432}"
tmp=$(mktemp /src/.env.dev.XXXXXX)
{
  printf 'DHOW_DEV=true\n'
  printf 'DHOW_PUBLIC_URL=http://localhost:8080\n'
  printf 'DHOW_DATABASE_URL=%s/dhow?sslmode=disable\n' "$url"
  printf 'DHOW_TEST_DATABASE_URL=%s/dhow_test?sslmode=disable\n' "$url"
  printf 'DHOW_SECRET_KEY=%s\n' "$(od -An -tx1 -N32 /dev/urandom | tr -d ' \n')"
} >"$tmp"
chown "$(stat -c %u:%g /src)" "$tmp"
mv "$tmp" "$file"
printf 'init: wrote .env.dev with a new secret key\n'
