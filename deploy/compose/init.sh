#!/usr/bin/env bash
set -euo pipefail
umask 077

dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
env_file="$dir/.env"
secrets_dir="$dir/secrets"
keyring="$secrets_dir/dhow-keyring"
container_uid="${DHOW_CONTAINER_UID:-65532}"
public_url="${1:-${DHOW_PUBLIC_URL:-https://dhow.localhost}}"

fail() {
  printf 'init.sh: %s\n' "$1" >&2
  exit 1
}

if [[ -e "$env_file" || -L "$env_file" ]]; then
  fail "$env_file already exists and will not be overwritten. Move it away first if you really want new secrets: they cannot decrypt data stored with the old ones."
fi
if [[ -e "$keyring" || -L "$keyring" ]]; then
  fail "$keyring already exists and will not be overwritten: it may be the only copy of the keys that open your secrets."
fi
command -v openssl >/dev/null 2>&1 || fail "openssl is required to generate secrets"
if [[ ! "$public_url" =~ ^https://[^[:space:]/?#@]+(/[^[:space:]?#]*)?$ ]]; then
  fail "the public URL must be an absolute https URL without credentials, a query or a fragment, got: $public_url"
fi
if [[ ! "$container_uid" =~ ^[0-9]+$ ]]; then
  fail "DHOW_CONTAINER_UID must be a number, got: $container_uid"
fi

give_to_container() {
  if [[ $(id -u) -eq "$container_uid" ]]; then
    return 0
  fi
  if [[ $(id -u) -eq 0 ]]; then
    chown "$container_uid:$container_uid" "$keyring"
    return 0
  fi
  command -v sudo >/dev/null 2>&1 && sudo chown "$container_uid:$container_uid" "$keyring"
}

secret_key=$(openssl rand -hex 32)
postgres_password=$(openssl rand -hex 24)
kek=$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n')

[[ -d "$secrets_dir" ]] || mkdir -m 700 "$secrets_dir"
chmod 700 "$secrets_dir"
(
  set -o noclobber
  printf 'kek 1 %s\n' "$kek" >"$keyring"
)
chmod 600 "$keyring"
if ! give_to_container; then
  rm -f -- "$keyring"
  fail "the dhow container runs as user $container_uid and must own $keyring, but chown needs root. Run ./init.sh again with sudo, or as root."
fi

(
  set -o noclobber
  {
    printf 'POSTGRES_PASSWORD=%s\n' "$postgres_password"
    printf 'DHOW_SECRET_KEY=%s\n' "$secret_key"
    printf 'DHOW_PUBLIC_URL=%s\n' "$public_url"
  } >"$env_file"
)
chmod 600 "$env_file"

printf 'Wrote %s (mode 600).\n' "$env_file"
printf 'Wrote the keyring %s (mode 600, owned by the container user %s).\n' "$keyring" "$container_uid"
printf 'Public URL: %s\n' "$public_url"
printf 'Back up .env and secrets/ together: without them the stored secrets cannot be decrypted.\n'
printf 'Export the keyring and keep the copy off this machine: docker compose run --rm --no-deps dhow keys export --recipient age1... --out - > dhow-keyring.age\n'
printf 'Next: docker compose up -d\n'
