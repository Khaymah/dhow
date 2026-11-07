#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  printf 'usage: %s IMAGE [nginx|nginx-chain|caddy|traefik]...\n' "$0" >&2
  exit 2
}
[[ $# -ge 1 && -n "$1" ]] || usage
image=$1
shift
scenarios=("$@")
((${#scenarios[@]})) || scenarios=(nginx nginx-chain caddy traefik)
for s in "${scenarios[@]}"; do
  [[ $s =~ ^(nginx|nginx-chain|caddy|traefik)$ ]] || usage
done

here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
root=$(cd -- "$here/../.." && pwd)
domain=dhow.example.com
https_port=${DHOW_PROXY_CHECK_PORT:-18443}
http_port=${DHOW_PROXY_CHECK_HTTP_PORT:-18080}
direct_port=${DHOW_PROXY_CHECK_DIRECT_PORT:-18081}
public_url="https://$domain:$https_port"
nginx_image=${DHOW_PROXY_CHECK_NGINX_IMAGE:-nginx:1.30.6-alpine}
caddy_image=${DHOW_PROXY_CHECK_CADDY_IMAGE:-caddy:2.11.7-alpine}
pebble_image=${DHOW_PROXY_CHECK_PEBBLE_IMAGE:-ghcr.io/letsencrypt/pebble:2.8.0}
capture_image=${DHOW_PROXY_CHECK_CAPTURE_IMAGE:-}
postgres_image=$(sed -n 's/^ *image: *\(postgres:[^ ]*\)$/\1/p' "$root/deploy/compose/compose.yaml")
label=dhow-proxy-check
prefix=dhow-pc
front_net=$prefix-front
data_net=$prefix-data
front_gateway=10.231.1.1
proxy_ip=10.231.1.10
edge_ip=10.231.1.20
traefik_project=dhow-pc-traefik
password='proxy-check passphrase 2026'
spoofed_ip=203.0.113.66
work=$(mktemp -d)
failures=0
scenario_failures=()

make_keyring() {
  local file=$1
  mkdir -p "$(dirname -- "$file")"
  chmod 700 "$(dirname -- "$file")"
  printf 'kek 1 %s\n' "$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n')" >"$file"
  chmod 600 "$file"
  if [[ $(id -u) -eq 0 ]]; then
    chown 65532:65532 "$file"
  else
    sudo chown 65532:65532 "$file"
  fi
}

traefik_compose() {
  docker compose --progress quiet --project-name "$traefik_project" --project-directory "$work/traefik" \
    -f "$root/deploy/compose/compose.yaml" -f "$here/traefik/compose.traefik.yaml" \
    -f "$here/traefik/compose.check.yaml" "$@"
}

remove_all() {
  local ids
  if [[ -d $work/traefik ]]; then
    traefik_compose down --volumes --remove-orphans --timeout 10 >/dev/null 2>&1 || true
  fi
  ids=$(docker ps -aq --filter "label=$label")
  if [[ -n $ids ]]; then
    xargs docker rm -f -v >/dev/null 2>&1 <<<"$ids" || true
  fi
  ids=$(docker network ls -q --filter "label=$label")
  if [[ -n $ids ]]; then
    xargs docker network rm >/dev/null 2>&1 <<<"$ids" || true
  fi
}

cleanup() {
  local status=$?
  remove_all
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
  local name=$1
  shift
  if "$@"; then ok "$name"; else bad "$name"; fi
}
note() { printf '  info  %s\n' "$1"; }
absent() { ! grep -q -- "$1" "$2"; }
cookie_without_domain() { ! grep -i '^set-cookie:' "$1" | grep -qi 'domain='; }
login_recorded_from() { grep '"Action":"auth.login"' "$1" | grep -q "\"Ip\":\"$2\""; }

header_value() {
  sed -n "s/^$2: *//Ip" "$1" | tr -d '\r' | tail -1
}

header_count() {
  grep -ic "^$2:" "$1" || true
}

build_capture_image() {
  if [[ -n $capture_image ]]; then
    return
  fi
  capture_image=dhow-proxy-check-capture:local
  docker build --quiet --tag "$capture_image" "$here/capture" >/dev/null
}

make_certificates() {
  local dir=$1 live=$1/live/$domain
  mkdir -p "$live"
  openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 2 \
    -subj '/CN=Dhow proxy-check CA' -keyout "$dir/ca.key" -out "$dir/ca.crt" 2>/dev/null
  openssl req -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes \
    -subj "/CN=$domain" -keyout "$live/privkey.pem" -out "$dir/leaf.csr" 2>/dev/null
  openssl x509 -req -in "$dir/leaf.csr" -CA "$dir/ca.crt" -CAkey "$dir/ca.key" -CAcreateserial -days 2 \
    -extfile <(printf 'subjectAltName=DNS:%s\nextendedKeyUsage=serverAuth\n' "$domain") \
    -out "$live/fullchain.pem" 2>/dev/null
  chmod 644 "$live/fullchain.pem" "$live/privkey.pem" "$dir/ca.crt"
  chmod 755 "$dir" "$dir/live" "$live"
}

create_networks() {
  docker network create --label "$label" --subnet 10.231.1.0/24 --gateway "$front_gateway" "$front_net" >/dev/null
  docker network create --label "$label" --internal "$data_net" >/dev/null
}

start_postgres() {
  pg_password=$(openssl rand -hex 24)
  docker run -d --name "$prefix-postgres" --label "$label" --user 999:999 \
    --network "$data_net" --network-alias postgres \
    -e POSTGRES_USER=dhow -e POSTGRES_DB=dhow -e POSTGRES_PASSWORD="$pg_password" \
    --entrypoint bash "$postgres_image" -ec '
      umask 077
      mkdir -p /tmp/tls
      openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 2 -subj /CN=postgres \
        -keyout /tmp/tls/server.key -out /tmp/tls/server.crt 2>/dev/null
      exec docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/tmp/tls/server.crt -c ssl_key_file=/tmp/tls/server.key' \
    >/dev/null
  local i
  for ((i = 0; i < 60; i++)); do
    if docker exec "$prefix-postgres" pg_isready -h 127.0.0.1 -U dhow -d dhow -q 2>/dev/null; then
      return
    fi
    sleep 1
  done
  printf 'postgres did not become ready\n' >&2
  return 1
}

network_gateway() {
  local gateway subnet
  gateway=$(docker network inspect "$1" --format '{{(index .IPAM.Config 0).Gateway}}')
  if [[ -z $gateway ]]; then
    subnet=$(docker network inspect "$1" --format '{{(index .IPAM.Config 0).Subnet}}')
    gateway="${subnet%.*/*}.1"
  fi
  printf '%s' "$gateway"
}

start_dhow() {
  local netns=$1 trusted=$2
  [[ -f $work/keyring/dhow-keyring ]] || make_keyring "$work/keyring/dhow-keyring"
  docker run -d --name "$prefix-dhow" --label "$label" --network "container:$netns" \
    --read-only --cap-drop ALL --security-opt no-new-privileges:true --tmpfs /tmp:size=16m \
    --tmpfs /var/lib/dhow/blobs:size=64m,mode=0700,uid=65532,gid=65532 \
    -v "$work/keyring/dhow-keyring:/run/secrets/dhow-keyring:ro" -e DHOW_KEYRING_FILE=/run/secrets/dhow-keyring \
    -e DHOW_DATABASE_URL="postgres://dhow:$pg_password@postgres:5432/dhow?sslmode=require" \
    -e DHOW_SECRET_KEY="$(openssl rand -hex 32)" \
    -e DHOW_PUBLIC_URL="$public_url" \
    -e DHOW_LISTEN_ADDR=127.0.0.1:8080 \
    -e DHOW_TRUSTED_PROXIES="$trusted" \
    -e DHOW_BREACHED_PASSWORD_CHECK=false \
    "$image" >/dev/null
  dhow_container=$prefix-dhow
}

start_capture() {
  docker run -d --name "$prefix-capture" --label "$label" --network "container:$1" \
    --cap-drop ALL --cap-add NET_RAW --cap-add NET_ADMIN \
    "$capture_image" tcpdump -i any -l -A -s 0 -n 'tcp dst port 8080' >/dev/null
  local i
  for ((i = 0; i < 20; i++)); do
    if docker logs "$prefix-capture" 2>&1 | grep -q 'listening on'; then
      return
    fi
    sleep 0.5
  done
  printf 'tcpdump did not start\n' >&2
  return 1
}

request() {
  local name=$1 method=$2 path=$3
  shift 3
  curl --silent --show-error --noproxy "*" --max-time 20 --cacert "$cacert" --resolve "$domain:$https_port:127.0.0.1" \
    --output "$out/$name.body" --dump-header "$out/$name.head" --write-out '%{http_code}' \
    --request "$method" "$@" -- "$public_url$path" || printf '000'
}

wait_ready() {
  local i status
  for ((i = 0; i < 90; i++)); do
    status=$(request ready GET /api/v1/health/ready 2>/dev/null)
    if [[ $status == 200 ]]; then
      return 0
    fi
    sleep 1
  done
  return 1
}

captured_request() {
  local marker=$1
  docker logs "$prefix-capture" >"$out/tcpdump.txt" 2>/dev/null
  awk -v marker="x-proxy-check: $marker" '
    /^[0-9][0-9]:[0-9][0-9]:[0-9][0-9]\./ { if (found) exit; block = ""; next }
    { block = block $0 "\n" }
    index(tolower($0), marker) == 1 { found = 1 }
    END { if (found) printf "%s", block }' "$out/tcpdump.txt"
}

check_scenario() {
  local name=$1 expected_ip=$2 check_redirect=$3 xff_tail=${4:-$2}
  local status setup_link token setup_body request_id org_id marker cookie logs

  printf 'TLS and health through %s\n' "$name"
  expect "the proxy serves $public_url/api/v1/health/ready with 200 over verified TLS" wait_ready
  status=$(request ready GET /api/v1/health/ready)
  expect "/api/v1/health/ready reports ok" grep -q '"Status":"ok"' "$out/ready.body"
  expect "exactly one Strict-Transport-Security header" test "$(header_count "$out/ready.head" Strict-Transport-Security)" = 1
  expect "HSTS max-age is two years" grep -iq '^strict-transport-security: max-age=63072000' "$out/ready.head"
  if [[ $check_redirect == yes ]]; then
    status=$(curl --silent --noproxy "*" --max-time 10 --output /dev/null --dump-header "$out/redirect.head" --write-out '%{http_code}' \
      --header "Host: $domain" -- "http://127.0.0.1:$http_port/login" || printf '000')
    expect "plain HTTP redirects to HTTPS ($status)" test "$status" = 301 -o "$status" = 308
    expect "the redirect goes to https://$domain" grep -q "^https://$domain/login" <<<"$(header_value "$out/redirect.head" Location)"
  fi

  printf 'Origin and first-owner setup through %s\n' "$name"
  logs=$(docker logs "$dhow_container" 2>&1)
  setup_link=$(grep -o "$public_url/setup#[A-Za-z0-9_-]*" <<<"$logs" | tail -1 || true)
  expect "Dhow logged the setup link on $public_url" test -n "$setup_link"
  token=${setup_link#*#}
  setup_body=$(printf '{"SetupToken":"%s","OwnerName":"Proxy Check","Email":"owner@proxy-check.test","Password":"%s","OrganizationName":"Proxy Check","OrganizationSlug":"proxy-check"}' "$token" "$password")

  status=$(request setup-foreign POST /api/v1/setup --header 'Content-Type: application/json' \
    --header 'Origin: https://evil.example' --data "$setup_body")
  expect "setup from a foreign Origin answers 403" test "$status" = 403
  expect "setup from a foreign Origin is CSRF_FAILED" grep -q '"Code":"CSRF_FAILED"' "$out/setup-foreign.body"

  marker=$(openssl rand -hex 8)
  status=$(request setup POST /api/v1/setup --header 'Content-Type: application/json' \
    --header "Origin: $public_url" --header "X-Proxy-Check: $marker" \
    --header "X-Forwarded-For: $spoofed_ip" --data "$setup_body")
  expect "setup with Origin $public_url answers 200" test "$status" = 200
  expect "setup with the right Origin is not CSRF_FAILED" absent CSRF_FAILED "$out/setup.body"
  request_id=$(header_value "$out/setup.head" X-Request-Id)
  org_id=$(sed -n 's/.*"Organization":{[^}]*"Id":"\([0-9a-f-]*\)".*/\1/p' "$out/setup.body")

  printf 'Headers as Dhow received them (tcpdump on its socket) through %s\n' "$name"
  sleep 1
  captured_request "$marker" >"$out/captured.txt"
  expect "the request was captured on Dhow's port" test -s "$out/captured.txt"
  note "Host: $(header_value "$out/captured.txt" Host)"
  note "Origin: $(header_value "$out/captured.txt" Origin)"
  note "X-Forwarded-For: $(header_value "$out/captured.txt" X-Forwarded-For)"
  note "X-Forwarded-Proto: $(header_value "$out/captured.txt" X-Forwarded-Proto)"
  expect "Host arrives unchanged ($domain:$https_port)" test "$(header_value "$out/captured.txt" Host)" = "$domain:$https_port"
  expect "Origin arrives unchanged" test "$(header_value "$out/captured.txt" Origin)" = "$public_url"
  expect "X-Forwarded-Proto is https" test "$(header_value "$out/captured.txt" X-Forwarded-Proto)" = https
  expect "X-Forwarded-For ends with the hops the proxies added ($xff_tail)" \
    grep -Eq "(^|, *)$xff_tail\$" <<<"$(header_value "$out/captured.txt" X-Forwarded-For)"

  printf 'Client address in Dhow through %s\n' "$name"
  logs=$(docker logs "$dhow_container" 2>&1)
  expect "the access log has the setup request" grep -q "\"request_id\":\"$request_id\"" <<<"$logs"
  expect "the access log records client_ip $expected_ip, not the proxy or the spoofed value" \
    grep -q "\"request_id\":\"$request_id\".*\"client_ip\":\"$expected_ip\"" <<<"$logs"

  status=$(request login POST /api/v1/sessions --header 'Content-Type: application/json' \
    --header "Origin: $public_url" --header "X-Forwarded-For: $spoofed_ip" \
    --data "$(printf '{"Email":"owner@proxy-check.test","Password":"%s"}' "$password")")
  expect "sign-in through the proxy answers 200" test "$status" = 200
  local attribute
  for attribute in '__Host-dhow_session=' 'Secure' 'HttpOnly' 'Path=/'; do
    expect "the session cookie has $attribute" grep -iq "^set-cookie:.*$attribute" "$out/login.head"
  done
  expect "the session cookie has no Domain" cookie_without_domain "$out/login.head"
  cookie=$(sed -n 's/^[Ss]et-[Cc]ookie: \(__Host-dhow_session=[^;]*\);.*/\1/p' "$out/login.head" | tr -d '\r')
  status=$(request me GET /api/v1/me --header "Cookie: $cookie")
  expect "the cookie reaches Dhow through the proxy (/api/v1/me answers 200)" test "$status" = 200
  status=$(request audit GET "/api/v1/organizations/$org_id/audit-events" --header "Cookie: $cookie")
  expect "the audit log answers 200" test "$status" = 200
  sed 's/},{"Action"/}\n{"Action"/g' "$out/audit.body" >"$out/audit.events"
  expect "the audit log records the sign-in from $expected_ip" \
    login_recorded_from "$out/audit.events" "$expected_ip"
  expect "the audit log never records the spoofed $spoofed_ip" absent "$spoofed_ip" "$out/audit.body"

  printf 'Runner plane and log stream through %s\n' "$name"
  status=$(request runner POST /runner-api/dhow.runner.v1.AuthService/Challenge \
    --header 'Content-Type: application/json' --header 'Dhow-Protocol: 1' --data '{}')
  expect "/runner-api/ reaches Dhow's runner plane (400, got $status)" test "$status" = 400
  expect "the runner plane answer comes from Dhow" grep -q 'VALIDATION_FAILED: Runner' "$out/runner.body"
  status=$(request stream GET "/api/v1/organizations/$org_id/jobs/00000000-0000-7000-8000-000000000000/logs/stream" \
    --header "Cookie: $cookie" --header 'Accept: text/event-stream')
  expect "the log stream path reaches Dhow with the session (404 for a missing job, got $status)" test "$status" = 404
  expect "the log stream answer comes from Dhow" grep -q '"Code":"NOT_FOUND"' "$out/stream.body"

  printf 'Request size through %s\n' "$name"
  head -c $((3 << 20)) /dev/zero | tr '\0' a >"$out/big.body.in"
  status=$(request big POST /api/v1/sessions --header 'Content-Type: application/json' \
    --header "Origin: $public_url" --data-binary "@$out/big.body.in")
  expect "a 3 MiB body is refused with 413" test "$status" = 413
  printf 'Artifact upload path through %s\n' "$name"
  status=$(request ingest PUT "/ingest/v1/artifacts/$(printf 'a%.0s' {1..64})" --header 'Content-Type: application/gzip' \
    --header 'Authorization: Bearer dhow_ing_proxy-check-token-that-is-not-real' --data-binary "@$out/big.body.in")
  expect "a 3 MiB upload on /ingest/ passes the proxy and Dhow answers 401 (got $status)" test "$status" = 401
  expect "the upload answer comes from Dhow" grep -q '"Code":"INGEST_TOKEN_INVALID"' "$out/ingest.body"
}

run_nginx() {
  local certs=$work/nginx-certs
  make_certificates "$certs"
  cacert=$certs/ca.crt
  create_networks
  start_postgres
  docker run -d --name "$prefix-nginx" --label "$label" --network "$front_net" --ip "$proxy_ip" \
    -p "127.0.0.1:$https_port:443" -p "127.0.0.1:$http_port:80" \
    -v "$here/nginx/dhow.conf:/etc/nginx/conf.d/dhow.conf:ro" -v "$certs:/etc/letsencrypt:ro" \
    "$nginx_image" >/dev/null
  docker network connect "$data_net" "$prefix-nginx"
  start_dhow "$prefix-nginx" 127.0.0.1
  start_capture "$prefix-nginx"
  check_scenario nginx "$front_gateway" yes
}

run_nginx_chain() {
  local certs=$work/nginx-certs
  make_certificates "$certs"
  cacert=$certs/ca.crt
  create_networks
  start_postgres
  docker run -d --name "$prefix-nginx" --label "$label" --network "$front_net" --ip "$proxy_ip" \
    -v "$here/nginx/dhow.conf:/etc/nginx/conf.d/dhow.conf:ro" -v "$certs:/etc/letsencrypt:ro" \
    "$nginx_image" >/dev/null
  docker network connect "$data_net" "$prefix-nginx"
  docker run -d --name "$prefix-edge" --label "$label" --network "$front_net" --ip "$edge_ip" \
    -p "127.0.0.1:$https_port:443" \
    -v "$here/nginx/edge.conf:/etc/nginx/conf.d/edge.conf:ro" -v "$certs:/etc/letsencrypt:ro" \
    "$nginx_image" >/dev/null
  start_dhow "$prefix-nginx" "127.0.0.1,$edge_ip"
  start_capture "$prefix-nginx"
  check_scenario "an edge proxy ($edge_ip) and nginx" "$front_gateway" no "$front_gateway, $edge_ip"
}

run_caddy() {
  local i
  create_networks
  start_postgres
  docker run -d --name "$prefix-caddy" --label "$label" --network "$front_net" --ip "$proxy_ip" \
    -p "127.0.0.1:$https_port:443" -p "127.0.0.1:$http_port:80" \
    -v "$here/caddy/Caddyfile:/etc/caddy/dhow.caddyfile:ro" \
    -v "$here/caddy/Harness.Caddyfile:/etc/caddy/Caddyfile:ro" \
    "$caddy_image" >/dev/null
  docker network connect "$data_net" "$prefix-caddy"
  start_dhow "$prefix-caddy" 127.0.0.1
  start_capture "$prefix-caddy"
  cacert=$work/caddy-root.crt
  for ((i = 0; i < 60; i++)); do
    if docker cp "$prefix-caddy:/data/caddy/pki/authorities/local/root.crt" "$cacert" >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  check_scenario caddy "$front_gateway" yes
}

run_traefik() {
  local i pebble_id
  if docker network inspect dhow_proxy >/dev/null 2>&1; then
    bad "a Docker network named dhow_proxy already exists (a real Traefik install?); remove it or skip the traefik scenario"
    return
  fi
  mkdir -p "$work/traefik"
  make_keyring "$work/traefik/secrets/dhow-keyring"
  pebble_id=$(docker create "$pebble_image")
  docker cp "$pebble_id:/test/certs/pebble.minica.pem" "$work/traefik/pebble.minica.pem" >/dev/null
  docker rm "$pebble_id" >/dev/null
  chmod 644 "$work/traefik/pebble.minica.pem"
  export DHOW_IMAGE=$image DHOW_PUBLIC_URL=$public_url DHOW_PUBLISH=127.0.0.1:$direct_port
  export DHOW_BREACHED_PASSWORD_CHECK=false ACME_EMAIL=proxy-check@dhow.test
  export ACME_CA_SERVER=https://pebble:14000/dir
  export DHOW_PROXY_CHECK_PORT=$https_port DHOW_PROXY_CHECK_HTTP_PORT=$http_port
  export DHOW_PROXY_CHECK_PEBBLE_IMAGE=$pebble_image
  POSTGRES_PASSWORD=$(openssl rand -hex 24)
  DHOW_SECRET_KEY=$(openssl rand -hex 32)
  export POSTGRES_PASSWORD DHOW_SECRET_KEY
  traefik_compose up --detach --wait --wait-timeout 120 >/dev/null
  dhow_container=$(traefik_compose ps --quiet dhow)
  start_capture "$dhow_container"
  cacert=$work/traefik/pebble-root.pem
  for ((i = 0; i < 60; i++)); do
    if docker run --rm --label "$label" --network "${traefik_project}_acme" \
      -v "$work/traefik/pebble.minica.pem:/minica.pem:ro" "$capture_image" \
      curl --silent --fail --cacert /minica.pem https://pebble:15000/roots/0 >"$cacert" 2>/dev/null &&
      [[ -s $cacert ]]; then
      break
    fi
    sleep 1
  done
  check_scenario traefik "$(network_gateway dhow_proxy)" yes
  note "certificate issuer: $(openssl s_client -connect "127.0.0.1:$https_port" -servername "$domain" </dev/null 2>/dev/null |
    openssl x509 -noout -issuer 2>/dev/null)"
}

for tool in docker curl openssl awk; do
  command -v "$tool" >/dev/null 2>&1 || {
    printf 'proxy-check: %s is required\n' "$tool" >&2
    exit 1
  }
done
docker image inspect "$image" >/dev/null
build_capture_image
remove_all

for scenario in "${scenarios[@]}"; do
  printf '\n== %s ==\n' "$scenario"
  out=$work/$scenario
  mkdir -p "$out"
  before=$failures
  case $scenario in
    nginx) run_nginx ;;
    nginx-chain) run_nginx_chain ;;
    caddy) run_caddy ;;
    traefik) run_traefik ;;
  esac
  if [[ -n ${DHOW_PROXY_CHECK_OUTPUT:-} ]]; then
    mkdir -p "$DHOW_PROXY_CHECK_OUTPUT/$scenario"
    cp -R "$out/." "$DHOW_PROXY_CHECK_OUTPUT/$scenario/"
  fi
  if ((failures != before)); then
    scenario_failures+=("$scenario")
    if [[ -n ${dhow_container:-} ]]; then
      docker logs --tail 40 "$dhow_container" 2>&1 | sed 's/^/    dhow| /' >&2 || true
    fi
  fi
  remove_all
  dhow_container=
done

if ((failures != 0)); then
  printf '\n%d check(s) failed in: %s\n' "$failures" "${scenario_failures[*]}" >&2
  exit 1
fi
printf '\nAll proxy checks passed: %s\n' "${scenarios[*]}"
