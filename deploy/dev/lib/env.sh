#!/usr/bin/env bash

env_file=
http_port=
secret_key=
smtp_host=''
smtp_port=''
smtp_user=''
smtp_password=''
smtp_from=''
smtp_tls=''

quoted() {
  local q="'" v=$1
  printf "'%s'" "${v//$q/$q\\$q$q}"
}

env_get() {
  [[ -f $env_file ]] || return 0
  local v q="'"
  v=$(grep -m1 "^$1=" "$env_file" | cut -d= -f2- || true)
  if [[ $v == "$q"*"$q" ]]; then
    v=${v#"$q"}
    v=${v%"$q"}
    v=${v//"$q\\$q$q"/$q}
  fi
  printf '%s' "$v"
}

password_of() {
  local url=$1 creds
  creds=${url#*://}
  creds=${creds%%@*}
  [[ $creds == *:* ]] && printf '%s' "${creds#*:}"
}

port_of() {
  local url=$1 hostport
  hostport=${url#*@}
  hostport=${hostport%%/*}
  [[ $hostport == *:* ]] && printf '%s' "${hostport##*:}"
}

is_port() {
  [[ $1 =~ ^[0-9]+$ ]] && (($1 >= 1 && $1 <= 65535))
}

is_host() {
  [[ $1 =~ ^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$ ]]
}

is_tls() {
  [[ $1 == starttls || $1 == tls || $1 == none ]]
}

is_mailbox() {
  local addr='[^[:space:]<>@,;"]+@[^[:space:]<>@,;"]+'
  local plain="^${addr}\$" named="^[^<>@,;\"]*<${addr}>\$"
  [[ $1 =~ $plain || $1 =~ $named ]]
}

smtp_valid() {
  is_host "$smtp_host" && is_port "$smtp_port" && is_tls "$smtp_tls" && is_mailbox "$smtp_from"
}

load_existing() {
  [[ -f $env_file ]] || return 0
  step "Found $env_file"
  if ! confirm "Keep its secret key and database settings?"; then
    confirm "This makes a new secret key, so data sealed with the old one can no longer be read. Continue?" n || fail "stopped; nothing changed"
    return 0
  fi
  secret_key=$(env_get DHOW_SECRET_KEY)
  local url
  url=$(env_get DHOW_DATABASE_URL)
  if [[ -n $url && -z $db_url ]]; then
    db_password=$(password_of "$url")
    db_port=$(port_of "$url")
    if [[ $db_mode == url ]]; then
      db_url=$url
      db_test_url=$(env_get DHOW_TEST_DATABASE_URL)
    fi
  fi
  http_port=$(env_get DHOW_LISTEN_ADDR)
  http_port=${http_port##*:}
  smtp_host=$(env_get DHOW_SMTP_HOST)
  smtp_port=$(env_get DHOW_SMTP_PORT)
  smtp_user=$(env_get DHOW_SMTP_USERNAME)
  smtp_password=$(env_get DHOW_SMTP_PASSWORD)
  smtp_from=$(env_get DHOW_SMTP_FROM)
  smtp_tls=$(env_get DHOW_SMTP_TLS)
}

collect_settings() {
  step "Settings"
  ask_valid http_port "Port for the server on localhost" "${http_port:-8080}" is_port "Type a number from 1 to 65535."
  if [[ $db_mode != url ]]; then
    ask_valid db_port "Port for PostgreSQL on localhost" "${db_port:-5432}" is_port "Type a number from 1 to 65535."
  fi
  [[ -n $secret_key ]] || secret_key=$(random_hex 32)
  [[ -n $db_password ]] || db_password=$(random_hex 16)
  if [[ -n $smtp_host ]] && ! smtp_valid; then
    warn "the SMTP settings in $env_file are not valid, so they are asked again"
    smtp_host='' smtp_port='' smtp_tls='' smtp_user='' smtp_password='' smtp_from=''
  fi
  if [[ -z $smtp_host ]] && confirm "Send email (invitations and sign-in alerts) through an SMTP server?" n; then
    ask_valid smtp_host "SMTP host" "" is_host "Type a host name such as smtp.example.com."
    ask_valid smtp_port "SMTP port" "587" is_port "Type a number from 1 to 65535."
    ask_valid smtp_tls "TLS mode: starttls, tls or none" "starttls" is_tls "Type starttls, tls or none."
    ask smtp_user "SMTP user name (empty for none)" ""
    [[ -n $smtp_user ]] && ask_secret smtp_password "SMTP password"
    ask_valid smtp_from "From address" "Dhow <dhow@localhost>" is_mailbox "Type an address such as dhow@example.com or Dhow <dhow@example.com>."
  fi
}

write_env() {
  local tmp line keyring
  keyring="$(cd -- "$(dirname -- "$env_file")" && pwd)/.dhow-dev-keyring"
  write_dev_keyring "$keyring"
  tmp=$(mktemp "$env_file.XXXXXX")
  {
    for line in "DHOW_DEV=true" "DHOW_PUBLIC_URL=http://localhost:$http_port" "DHOW_LISTEN_ADDR=127.0.0.1:$http_port" \
      "DHOW_DATABASE_URL=$db_url" "DHOW_TEST_DATABASE_URL=$db_test_url" "DHOW_SECRET_KEY=$secret_key" "DHOW_KEYRING_FILE=$keyring" \
      "DHOW_SMTP_HOST=$smtp_host" "DHOW_SMTP_PORT=$smtp_port" "DHOW_SMTP_TLS=$smtp_tls" \
      "DHOW_SMTP_USERNAME=$smtp_user" "DHOW_SMTP_PASSWORD=$smtp_password" "DHOW_SMTP_FROM=$smtp_from"; do
      if [[ -n ${line#*=} && ( $line != DHOW_SMTP_* || -n $smtp_host ) ]]; then
        printf '%s=%s\n' "${line%%=*}" "$(quoted "${line#*=}")"
      fi
    done
  } >"$tmp"
  chmod 600 "$tmp"
  mv -f "$tmp" "$env_file"
  printf 'Wrote %s (mode 600)\n' "$env_file"
}
