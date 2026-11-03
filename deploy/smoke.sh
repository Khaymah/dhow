#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 || -z "$1" ]]; then
  printf 'usage: %s IMAGE\n' "$0" >&2
  exit 2
fi

image=$1
port=${DHOW_SMOKE_PORT:-18080}
public_url=https://dhow.smoke.test
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d)
failures=0

export COMPOSE_PROJECT_NAME=dhow-smoke
export DHOW_IMAGE=$image
export DHOW_PUBLISH=127.0.0.1:$port

compose() { docker compose --project-directory "$work" -f "$work/compose.yaml" "$@"; }

cleanup() {
  local status=$?
  if ((status != 0 || failures != 0)); then
    compose logs --no-color --tail 60 2>&1 || true
  fi
  compose down --volumes --remove-orphans --timeout 30 >/dev/null 2>&1 || true
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

has_header() { grep -iq -- "^$2:.*$3" "$1"; }

api() {
  local prefix=$1 method=$2 path=$3 body=$4
  shift 4
  curl --silent --show-error --max-time 10 --output "$prefix.body" --dump-header "$prefix.head" \
    --write-out '%{http_code}' --request "$method" --header 'Content-Type: application/json' "$@" \
    --data "$body" -- "http://127.0.0.1:$port$path"
}

http() {
  local prefix=$1 path=$2
  curl --silent --show-error --max-time 5 --output "$prefix.body" --dump-header "$prefix.head" \
    --write-out '%{http_code}' -- "http://127.0.0.1:$port$path"
}

printf 'Preparing the install directory\n'
cp -- "$root/deploy/compose/compose.yaml" "$root/deploy/compose/init.sh" "$root/deploy/compose/.env.example" "$work/"
"$work/init.sh" "$public_url" >/dev/null
printf 'DHOW_RATE_LIMIT_PUBLIC=20\nDHOW_RATE_LIMIT_INVITES_USER=3\n' >>"$work/.env"
expect "the smoke stack runs with low rate limits" grep -q '^DHOW_RATE_LIMIT_PUBLIC=20$' "$work/.env"
expect ".env is mode 600" test "$(stat -c %a "$work/.env")" = 600
expect "init.sh refuses to overwrite .env" bash -c '! "$1" >/dev/null 2>&1' _ "$work/init.sh"
expect "the keyring is mode 600 and belongs to the container user" test "$(stat -c '%a %u' "$work/secrets/dhow-keyring")" = "600 65532"

printf 'The image refuses to start without configuration\n'
set +e
noenv=$(docker run --rm "$image" 2>&1)
noenv_status=$?
set -e
expect "exits non-zero without config" test "$noenv_status" -ne 0
expect "names the missing secret key" grep -q DHOW_SECRET_KEY <<<"$noenv"
expect "names the missing keyring" grep -q DHOW_KEYRING_FILE <<<"$noenv"

printf 'Starting the stack\n'
compose up --detach --wait --wait-timeout 120
dhow_id=$(compose ps --quiet dhow)
postgres_id=$(compose ps --quiet postgres)
expect "dhow is healthy" test "$(docker inspect --format '{{.State.Health.Status}}' "$dhow_id")" = healthy
expect "postgres is healthy" test "$(docker inspect --format '{{.State.Health.Status}}' "$postgres_id")" = healthy

printf 'HTTP behaviour\n'
status=$(http "$work/ready" /api/v1/health/ready)
expect "/api/v1/health/ready answers 200" test "$status" = 200
expect "/api/v1/health/ready reports ok" grep -q '"Status":"ok"' "$work/ready.body"
expect "/api/v1/health/ready checks the keyring" grep -q '"Name":"keyring","Ok":true' "$work/ready.body"
kek_text=$(sudo -n cat "$work/secrets/dhow-keyring" 2>/dev/null || cat "$work/secrets/dhow-keyring" 2>/dev/null || true)
kek=${kek_text##* }
expect "the logs never show the keyring key" bash -c '[[ -n "$1" ]] && ! grep -qF -- "$1" <<<"$2"' _ "$kek" "$(compose logs --no-color dhow 2>&1)"

status=$(http "$work/meta" /api/v1/meta)
expect "/api/v1/meta answers 200" test "$status" = 200
expect "/api/v1/meta uses the envelope" grep -q '"Success":true' "$work/meta.body"
expect "/api/v1/meta reports an empty instance" grep -q '"SetupRequired":true' "$work/meta.body"

status=$(http "$work/missing" /api/v1/does-not-exist)
expect "unknown API paths answer 404" test "$status" = 404
expect "unknown API paths use the envelope" grep -q '"Success":false' "$work/missing.body"

status=$(http "$work/web" /)
expect "/ answers 200" test "$status" = 200
expect "/ is text/html" has_header "$work/web.head" Content-Type 'text/html'
expect "/ serves the web app" grep -q '<div id="root">' "$work/web.body"

printf 'First-owner setup, sign-in and sessions\n'
password='smoke-test passphrase 2026'
dhow_logs=$(compose logs --no-color dhow 2>&1)
setup_link=$(grep -o "$public_url/setup#[A-Za-z0-9_-]*" <<<"$dhow_logs" | tail -1)
expect "the setup link is logged exactly once" test "$(grep -c "$public_url/setup#" <<<"$dhow_logs")" = 1
expect "the setup link carries a 43-character token" test "${#setup_link}" -eq $((${#public_url} + 7 + 43))
setup_token=${setup_link#*#}
setup_body=$(printf '{"SetupToken":"%s","OwnerName":"Smoke Owner","Email":"Owner@Smoke.Test","Password":"%s","OrganizationName":"Smoke","OrganizationSlug":"smoke"}' "$setup_token" "$password")

status=$(api "$work/setup-no-origin" POST /api/v1/setup "$setup_body")
expect "setup without an Origin header answers 403" test "$status" = 403
expect "setup without an Origin header is CSRF_FAILED" grep -q '"Code":"CSRF_FAILED"' "$work/setup-no-origin.body"
status=$(api "$work/setup-bad" POST /api/v1/setup "${setup_body/$setup_token/AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA}" --header "Origin: $public_url")
expect "setup with an unknown token answers 403" test "$status" = 403
status=$(api "$work/setup-weak" POST /api/v1/setup "${setup_body/$password/short}" --header "Origin: $public_url")
expect "setup with a short password answers 422" test "$status" = 422
status=$(api "$work/setup" POST /api/v1/setup "$setup_body" --header "Origin: $public_url")
expect "setup answers 200" test "$status" = 200
expect "setup stores the email lowercased" grep -q '"Email":"owner@smoke.test"' "$work/setup.body"
status=$(api "$work/setup-again" POST /api/v1/setup "$setup_body" --header "Origin: $public_url")
expect "a second setup answers 409" test "$status" = 409
status=$(http "$work/meta2" /api/v1/meta)
expect "/api/v1/meta reports that setup is done" grep -q '"SetupRequired":false' "$work/meta2.body"

login_body=$(printf '{"Email":"owner@smoke.test","Password":"%s"}' "$password")
status=$(api "$work/bad-login" POST /api/v1/sessions '{"Email":"owner@smoke.test","Password":"not the password"}' --header "Origin: $public_url")
expect "a wrong password answers 401" test "$status" = 401
expect "a wrong password is INVALID_CREDENTIALS" grep -q '"Code":"INVALID_CREDENTIALS"' "$work/bad-login.body"
expect "a wrong password sets no cookie" bash -c '! grep -qi "^set-cookie:" "$1"' _ "$work/bad-login.head"
status=$(api "$work/login" POST /api/v1/sessions "$login_body" --header "Origin: $public_url")
expect "sign-in answers 200" test "$status" = 200
for attribute in '__Host-dhow_session=' 'HttpOnly' 'Secure' 'SameSite=Strict' 'Path=/'; do
  expect "the session cookie has $attribute" has_header "$work/login.head" Set-Cookie "$attribute"
done
expect "the session cookie has no Domain" bash -c '! grep -i "^set-cookie:" "$1" | grep -qi "domain="' _ "$work/login.head"
cookie=$(sed -n 's/^[Ss]et-[Cc]ookie: \(__Host-dhow_session=[^;]*\);.*/\1/p' "$work/login.head" | tr -d '\r')
csrf=$(grep -o '"CsrfToken":"[^"]*"' "$work/login.body" | cut -d'"' -f4)
expect "sign-in returns a CSRF token" test -n "$csrf"

status=$(curl --silent --max-time 5 --output "$work/anon-me.body" --write-out '%{http_code}' -- "http://127.0.0.1:$port/api/v1/me")
expect "/api/v1/me without a session answers 401" test "$status" = 401
status=$(curl --silent --max-time 5 --output "$work/me.body" --write-out '%{http_code}' --header "Cookie: $cookie" -- "http://127.0.0.1:$port/api/v1/me")
expect "/api/v1/me with the session answers 200" test "$status" = 200
expect "/api/v1/me names the owner role" grep -q '"Role":"owner"' "$work/me.body"
status=$(api "$work/logout-no-csrf" DELETE /api/v1/sessions/current '' --header "Origin: $public_url" --header "Cookie: $cookie")
expect "sign-out without a CSRF token answers 403" test "$status" = 403
status=$(api "$work/logout-origin" DELETE /api/v1/sessions/current '' --header "Origin: https://evil.example" --header "Cookie: $cookie" --header "X-CSRF-Token: $csrf")
expect "sign-out from a foreign Origin answers 403" test "$status" = 403
status=$(api "$work/logout" DELETE /api/v1/sessions/current '' --header "Origin: $public_url" --header "Cookie: $cookie" --header "X-CSRF-Token: $csrf")
expect "sign-out with the CSRF token answers 200" test "$status" = 200
status=$(curl --silent --max-time 5 --output /dev/null --write-out '%{http_code}' --header "Cookie: $cookie" -- "http://127.0.0.1:$port/api/v1/me")
expect "the revoked session answers 401 at once" test "$status" = 401

dhow_logs=$(compose logs --no-color dhow 2>&1)
expect "the password never reaches the logs" bash -c '! grep -qF -- "$1" <<<"$2"' _ "$password" "$dhow_logs"
expect "the session cookie never reaches the logs" bash -c '! grep -qF -- "$1" <<<"$2"' _ "${cookie#*=}" "$dhow_logs"
expect "the setup token is logged only inside the setup link" test "$(grep -oF -- "$setup_token" <<<"$dhow_logs" | wc -l)" = 1

totp() {
  if command -v oathtool >/dev/null 2>&1; then
    oathtool --totp --base32 --now "now + $(($2 * 30)) seconds" -- "$1"
  else
    (cd -- "$root" && go run ./deploy/smoke/totp "$1" "$2")
  fi
}
json_field() { grep -o "\"$2\":\"[^\"]*\"" "$1" | head -1 | cut -d'"' -f4; }
set_cookie() { sed -n "s/^[Ss]et-[Cc]ookie: \($2=[^;]*\);.*/\1/p" "$1" | tr -d '\r' | head -1; }

printf 'Passkeys and the second step\n'
origin=(--header "Origin: $public_url")
status=$(api "$work/login2" POST /api/v1/sessions "$login_body" "${origin[@]}")
expect "sign-in without a second factor still answers 200" test "$status" = 200
cookie=$(set_cookie "$work/login2.head" __Host-dhow_session)
csrf=$(json_field "$work/login2.body" CsrfToken)
session=(--header "Cookie: $cookie" --header "X-CSRF-Token: $csrf")
status=$(curl --silent --max-time 5 --output "$work/mfa-status.body" --write-out '%{http_code}' --header "Cookie: $cookie" -- "http://127.0.0.1:$port/api/v1/me/mfa")
expect "/api/v1/me/mfa answers 200" test "$status" = 200
expect "a new account has no second factor" grep -q '"Enabled":false' "$work/mfa-status.body"
status=$(curl --silent --max-time 5 --output "$work/passkeys.body" --write-out '%{http_code}' --header "Cookie: $cookie" -- "http://127.0.0.1:$port/api/v1/me/passkeys")
expect "/api/v1/me/passkeys answers 200" test "$status" = 200
expect "a new account has no passkeys" grep -q '"Data":\[\]' "$work/passkeys.body"
status=$(api "$work/pk-options" POST /api/v1/sessions/passkey-options '' "${origin[@]}")
expect "passkey sign-in options answer 200" test "$status" = 200
expect "the RP ID comes from DHOW_PUBLIC_URL" grep -q '"rpId":"dhow.smoke.test"' "$work/pk-options.body"
expect "passkey sign-in requires user verification" grep -q '"userVerification":"required"' "$work/pk-options.body"
challenge=$(json_field "$work/pk-options.body" ChallengeId)
status=$(api "$work/pk-login" POST /api/v1/sessions/passkey "{\"ChallengeId\":\"$challenge\",\"Credential\":{\"id\":\"AAAA\",\"type\":\"public-key\"}}" "${origin[@]}")
expect "an unknown passkey answers 401" test "$status" = 401
expect "an unknown passkey is INVALID_CREDENTIALS" grep -q '"Code":"INVALID_CREDENTIALS"' "$work/pk-login.body"

status=$(api "$work/totp" POST /api/v1/me/totp '' "${origin[@]}" "${session[@]}")
expect "TOTP enrollment answers 200" test "$status" = 200
secret=$(json_field "$work/totp.body" Secret)
expect "the TOTP secret has 160 bits" test "${#secret}" = 32
expect "the enrollment carries an otpauth URI" grep -q '"Uri":"otpauth://totp/' "$work/totp.body"
confirm_body=$(printf '{"Code":"%s","CurrentFactor":{"Method":"password","Password":"%s"}}' "$(totp "$secret" 0)" "$password")
status=$(api "$work/totp-confirm" POST /api/v1/me/totp/confirm "$confirm_body" "${origin[@]}" "${session[@]}")
expect "confirming TOTP answers 200" test "$status" = 200
expect "confirming TOTP returns 10 recovery codes" test "$(grep -o '[0-9A-Z]\{4\}-[0-9A-Z]\{4\}-[0-9A-Z]\{4\}-[0-9A-Z]\{4\}' "$work/totp-confirm.body" | wc -l)" = 10
recovery=$(grep -o '[0-9A-Z]\{4\}-[0-9A-Z]\{4\}-[0-9A-Z]\{4\}-[0-9A-Z]\{4\}' "$work/totp-confirm.body" | head -1)

status=$(api "$work/mfa-step" POST /api/v1/sessions "$login_body" "${origin[@]}")
expect "the password step now answers 202" test "$status" = 202
expect "the password step sets no session" bash -c '! grep -qi "^set-cookie: __Host-dhow_session=" "$1"' _ "$work/mfa-step.head"
for attribute in '__Host-dhow_mfa=' 'HttpOnly' 'Secure' 'SameSite=Strict' 'Path=/' 'Max-Age=300'; do
  expect "the challenge cookie has $attribute" has_header "$work/mfa-step.head" Set-Cookie "$attribute"
done
mfa_cookie=$(set_cookie "$work/mfa-step.head" __Host-dhow_mfa)
status=$(api "$work/mfa-wrong" POST /api/v1/sessions/mfa '{"Method":"totp","Code":"000000"}' "${origin[@]}" --header "Cookie: $mfa_cookie")
expect "a wrong code answers 401 INVALID_MFA_CODE" grep -q '"Code":"INVALID_MFA_CODE"' "$work/mfa-wrong.body"
status=$(api "$work/mfa-ok" POST /api/v1/sessions/mfa "{\"Method\":\"totp\",\"Code\":\"$(totp "$secret" 1)\"}" "${origin[@]}" --header "Cookie: $mfa_cookie")
expect "the right TOTP code answers 200" test "$status" = 200
mfa_session=$(set_cookie "$work/mfa-ok.head" __Host-dhow_session)
status=$(curl --silent --max-time 5 --output /dev/null --write-out '%{http_code}' --header "Cookie: $mfa_session" -- "http://127.0.0.1:$port/api/v1/me")
expect "the session from the second step works" test "$status" = 200
status=$(api "$work/mfa-reuse" POST /api/v1/sessions/mfa "{\"Method\":\"totp\",\"Code\":\"$(totp "$secret" 1)\"}" "${origin[@]}" --header "Cookie: $mfa_cookie")
expect "a used challenge answers MFA_CHALLENGE_EXPIRED" grep -q '"Code":"MFA_CHALLENGE_EXPIRED"' "$work/mfa-reuse.body"

api "$work/rc-step" POST /api/v1/sessions "$login_body" "${origin[@]}" >/dev/null
rc_cookie=$(set_cookie "$work/rc-step.head" __Host-dhow_mfa)
status=$(api "$work/rc-ok" POST /api/v1/sessions/mfa "{\"Method\":\"recovery_code\",\"Code\":\"$recovery\"}" "${origin[@]}" --header "Cookie: $rc_cookie")
expect "a recovery code signs in once" test "$status" = 200
api "$work/rc-step2" POST /api/v1/sessions "$login_body" "${origin[@]}" >/dev/null
rc_cookie=$(set_cookie "$work/rc-step2.head" __Host-dhow_mfa)
status=$(api "$work/rc-again" POST /api/v1/sessions/mfa "{\"Method\":\"recovery_code\",\"Code\":\"$recovery\"}" "${origin[@]}" --header "Cookie: $rc_cookie")
expect "the same recovery code is refused the second time" test "$status" = 401

dhow_logs=$(compose logs --no-color dhow 2>&1)
expect "the TOTP secret never reaches the logs" bash -c '! grep -qF -- "$1" <<<"$2"' _ "$secret" "$dhow_logs"
expect "recovery codes never reach the logs" bash -c '! grep -qF -- "$1" <<<"$2"' _ "$recovery" "$dhow_logs"
expect "the challenge cookie never reaches the logs" bash -c '! grep -qF -- "$1" <<<"$2"' _ "${mfa_cookie#*=}" "$dhow_logs"

printf 'Runner registration over /runner-api/\n'
(cd -- "$root" && go build -o "$work/smoke-runner" ./deploy/smoke/runner)
runner() { "$work/smoke-runner" "$@"; }
rpc() {
  local prefix=$1 procedure=$2 body=$3
  shift 3
  curl --silent --show-error --max-time 10 --output "$prefix.body" --write-out '%{http_code}' --request POST \
    --header 'Content-Type: application/json' "$@" --data "$body" -- "http://127.0.0.1:$port/runner-api/$procedure"
}
csrf=$(json_field "$work/mfa-ok.body" CsrfToken)
session=(--header "Cookie: $mfa_session" --header "X-CSRF-Token: $csrf")
status=$(curl --silent --max-time 5 --output "$work/me3.body" --write-out '%{http_code}' --header "Cookie: $mfa_session" -- "http://127.0.0.1:$port/api/v1/me")
org_id=$(json_field "$work/me3.body" OrganizationId)
expect "the organization id is known" test -n "$org_id"
status=$(api "$work/rrt" POST "/api/v1/organizations/$org_id/runner-registration-tokens" '{"Labels":["pool=smoke"]}' "${origin[@]}" "${session[@]}")
expect "creating a registration token answers 201" test "$status" = 201
rrt=$(json_field "$work/rrt.body" Token)
expect "the registration token starts with dhow_rrt_" test "${rrt:0:9}" = dhow_rrt_
expect "the register command reads the token from stdin" grep -q -- '--token-file -' "$work/rrt.body"
mkdir -m 700 "$work/runner"
printf '%s' "$rrt" >"$work/runner/token"
chmod 600 "$work/runner/token"
runner_url=(--url "http://127.0.0.1:$port" --origin "$public_url" --dir "$work/runner")
set +e
registered=$(runner register "${runner_url[@]}" --token-file "$work/runner/token" 2>"$work/runner/register.err")
register_status=$?
again=$(runner register "${runner_url[@]}" --token-file "$work/runner/token" 2>&1 >/dev/null)
printf 'dhow_rrt_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' >"$work/runner/forged"
forged=$(runner register "${runner_url[@]}" --token-file "$work/runner/forged" 2>&1 >/dev/null)
authenticated=$(runner authenticate "${runner_url[@]}" 2>&1)
set -e
expect "a runner registers with the token" test "$register_status" = 0
runner_id=${registered%% *}
expect "the runner gets the token labels and its capabilities only" test "${registered#* }" = 'pool=smoke,dhow.os=linux,dhow.arch=amd64,dhow.isolation=container'
expect "a used token answers INVALID_REGISTRATION_TOKEN" test "$(tail -1 <<<"$again")" = INVALID_REGISTRATION_TOKEN
expect "an unknown token answers the same INVALID_REGISTRATION_TOKEN" test "$(tail -1 <<<"$forged")" = INVALID_REGISTRATION_TOKEN
expect "the runner authenticates with a signed challenge" test "$authenticated" = authenticated
access=$(cat "$work/runner/access.token")
expect "the access token starts with dhow_rat_" test "${access:0:9}" = dhow_rat_
status=$(curl --silent --max-time 5 --output "$work/runner.body" --write-out '%{http_code}' --header "Cookie: $mfa_session" -- "http://127.0.0.1:$port/api/v1/organizations/$org_id/runners/$runner_id")
expect "the runner is listed in the API" test "$status" = 200
expect "the runner is online" grep -q '"Status":"online"' "$work/runner.body"
expect "the runner view carries no key" bash -c '! grep -qi "PublicKey" "$1"' _ "$work/runner.body"

challenge_body="{\"runner\":{\"runnerId\":\"$runner_id\"}}"
status=$(rpc "$work/ch" dhow.runner.v1.AuthService/Challenge "$challenge_body" --header 'Dhow-Protocol: 1')
expect "Challenge answers 200 over Connect JSON" test "$status" = 200
status=$(rpc "$work/ch-cookie" dhow.runner.v1.AuthService/Challenge "$challenge_body" --header 'Dhow-Protocol: 1' --header "Cookie: $mfa_session")
expect "a browser session on /runner-api/ answers 401 UNAUTHENTICATED" test "$status:$(json_field "$work/ch-cookie.body" message)" = 401:UNAUTHENTICATED
status=$(rpc "$work/ch-old" dhow.runner.v1.AuthService/Challenge "$challenge_body")
expect "a runner without the protocol header must upgrade" test "$(json_field "$work/ch-old.body" message)" = RUNNER_UPGRADE_REQUIRED
status=$(rpc "$work/rotate-anon" dhow.runner.v1.AuthService/RotateKeys '{}' --header 'Dhow-Protocol: 1')
expect "RotateKeys without a token answers 401" test "$status" = 401
status=$(rpc "$work/rotate" dhow.runner.v1.AuthService/RotateKeys '{}' --header 'Dhow-Protocol: 1' --header "Authorization: Bearer $access")
expect "RotateKeys with the access token passes the gate" test "$(json_field "$work/rotate.body" message)" = 'VALIDATION_FAILED: NewSigningPublicKey'
status=$(curl --silent --max-time 5 --output "$work/me-runner.body" --write-out '%{http_code}' --header "Authorization: Bearer $access" -- "http://127.0.0.1:$port/api/v1/me")
expect "the runner token on /api/v1 answers 401" test "$status" = 401

status=$(api "$work/revoke-runner" POST "/api/v1/organizations/$org_id/runners/$runner_id/revoke" '' "${origin[@]}" "${session[@]}")
expect "revoking the runner answers 200" test "$status" = 200
status=$(rpc "$work/rotate-revoked" dhow.runner.v1.AuthService/RotateKeys '{}' --header 'Dhow-Protocol: 1' --header "Authorization: Bearer $access")
expect "the first call after the revocation is RUNNER_REVOKED" test "$status:$(json_field "$work/rotate-revoked.body" message)" = 403:RUNNER_REVOKED

dhow_logs=$(compose logs --no-color dhow 2>&1)
expect "the registration token never reaches the logs" bash -c '! grep -qF -- "$1" <<<"$2"' _ "${rrt#dhow_rrt_}" "$dhow_logs"
expect "the access token never reaches the logs" bash -c '! grep -qF -- "$1" <<<"$2"' _ "${access#dhow_rat_}" "$dhow_logs"
expect "runner calls are in the access log" grep -q '"path":"/runner-api/dhow.runner.v1.AuthService/Authenticate"' <<<"$dhow_logs"

printf 'Invitations, roles and organizations\n'
org_api="/api/v1/organizations/$org_id"
status=$(api "$work/invite" POST "$org_api/invitations" '{"Email":"Dev@Smoke.Test","Role":"developer"}' "${origin[@]}" "${session[@]}")
expect "inviting a developer answers 201" test "$status" = 201
expect "without SMTP the invitation says email is disabled" grep -q '"EmailStatus":"disabled"' "$work/invite.body"
invite_link=$(json_field "$work/invite.body" Link)
invite_token=${invite_link#*#}
expect "the invitation link carries the token in the fragment" test "$invite_link" = "$public_url/invite#$invite_token"
expect "the invitation token has 43 characters" test "${#invite_token}" = 43
status=$(api "$work/invite-owner" POST "$org_api/invitations" '{"Email":"boss@smoke.test","Role":"owner"}' "${origin[@]}" "${session[@]}")
expect "an invitation can never grant owner" test "$status" = 422
status=$(api "$work/preview" POST /api/v1/invitations/preview "{\"Token\":\"$invite_token\"}" "${origin[@]}")
expect "the invitation preview answers 200" test "$status" = 200
expect "the preview names the invited email" grep -q '"Email":"dev@smoke.test"' "$work/preview.body"
dev_password='smoke developer passphrase 2026'
accept_body=$(printf '{"Token":"%s","Name":"Smoke Developer","Password":"%s","Locale":"en"}' "$invite_token" "$dev_password")
status=$(api "$work/accept" POST /api/v1/invitations/accept "$accept_body" "${origin[@]}")
expect "accepting the invitation answers 200" test "$status" = 200
dev_id=$(grep -o '"User":{[^}]*}' "$work/accept.body" | grep -o '"Id":"[^"]*"' | cut -d'"' -f4)
expect "the new account is known" test -n "$dev_id"
status=$(api "$work/accept-again" POST /api/v1/invitations/accept "${accept_body/Smoke Developer/Someone Else}" "${origin[@]}")
expect "a used invitation answers 410 INVITATION_INVALID" test "$status:$(json_field "$work/accept-again.body" Code)" = 410:INVITATION_INVALID
dev_login=$(printf '{"Email":"dev@smoke.test","Password":"%s"}' "$dev_password")
status=$(api "$work/dev-login" POST /api/v1/sessions "$dev_login" "${origin[@]}")
expect "the new member signs in" test "$status" = 200
dev_cookie=$(set_cookie "$work/dev-login.head" __Host-dhow_session)
curl --silent --max-time 5 --output "$work/dev-me.body" --header "Cookie: $dev_cookie" -- "http://127.0.0.1:$port/api/v1/me"
expect "the new member is a developer" grep -q '"Role":"developer"' "$work/dev-me.body"
status=$(api "$work/demote" PATCH "$org_api/members/$dev_id" '{"Role":"viewer"}' "${origin[@]}" "${session[@]}")
expect "changing the member's role answers 200" test "$status" = 200
status=$(curl --silent --max-time 5 --output /dev/null --write-out '%{http_code}' --header "Cookie: $dev_cookie" -- "http://127.0.0.1:$port/api/v1/me")
expect "the role change revokes the member's session at once" test "$status" = 401
status=$(curl --silent --max-time 5 --output /dev/null --write-out '%{http_code}' --header "Cookie: $mfa_session" -- "http://127.0.0.1:$port/api/v1/me")
expect "the owner keeps the session that made the change" test "$status" = 200

status=$(api "$work/org2" POST /api/v1/organizations '{"Name":"Smoke Two","Slug":"smoke-two"}' "${origin[@]}" "${session[@]}")
expect "the instance owner creates a second organization" test "$status" = 201
org2_id=$(json_field "$work/org2.body" Id)
api "$work/dev-login2" POST /api/v1/sessions "$dev_login" "${origin[@]}" >/dev/null
dev_cookie=$(set_cookie "$work/dev-login2.head" __Host-dhow_session)
dev_csrf=$(json_field "$work/dev-login2.body" CsrfToken)
status=$(api "$work/dev-org" POST /api/v1/organizations '{"Name":"Mine","Slug":"mine"}' "${origin[@]}" --header "Cookie: $dev_cookie" --header "X-CSRF-Token: $dev_csrf")
expect "a member who is not the instance owner cannot create one" test "$status" = 403
for path in "" /members /teams; do
  status=$(curl --silent --max-time 5 --output "$work/cross.body" --write-out '%{http_code}' --header "Cookie: $dev_cookie" -- "http://127.0.0.1:$port/api/v1/organizations/$org2_id$path")
  expect "another organization's ${path:-/} answers 404" test "$status" = 404
done
status=$(api "$work/cross-invite" POST "/api/v1/organizations/$org2_id/invitations" '{"Email":"x@smoke.test","Role":"viewer"}' "${origin[@]}" --header "Cookie: $dev_cookie" --header "X-CSRF-Token: $dev_csrf")
expect "inviting into another organization answers 404" test "$status" = 404

dhow_logs=$(compose logs --no-color dhow 2>&1)
expect "the invitation token never reaches the logs" bash -c '! grep -qF -- "$1" <<<"$2"' _ "$invite_token" "$dhow_logs"
expect "the member's password never reaches the logs" bash -c '! grep -qF -- "$1" <<<"$2"' _ "$dev_password" "$dhow_logs"

printf 'API tokens, preferences and sign-in providers\n'
recovery2=$(grep -o '[0-9A-Z]\{4\}-[0-9A-Z]\{4\}-[0-9A-Z]\{4\}-[0-9A-Z]\{4\}' "$work/totp-confirm.body" | sed -n 2p)
pat_body=$(printf '{"Name":"Smoke CI","Scopes":["org.read","member.read"],"ExpiresInDays":7,"CurrentFactor":{"Method":"recovery_code","Code":"%s"}}' "$recovery2")
status=$(api "$work/pat-nofactor" POST "$org_api/api-tokens" '{"Name":"Smoke CI","Scopes":["org.read"],"ExpiresInDays":7,"CurrentFactor":{"Method":"password","Password":"x"}}' "${origin[@]}" "${session[@]}")
expect "creating a token with a wrong factor answers 403" test "$status" = 403
status=$(api "$work/pat" POST "$org_api/api-tokens" "$pat_body" "${origin[@]}" "${session[@]}")
expect "creating an API token answers 200" test "$status" = 200
pat=$(json_field "$work/pat.body" Value)
pat_id=$(grep -o '"Token":{"[^}]*' "$work/pat.body" | grep -o '"Id":"[^"]*"' | head -1 | cut -d'"' -f4)
expect "the token starts with dhow_pat_ and has 32 random bytes" test "${pat:0:9}:${#pat}" = "dhow_pat_:52"
bearer=(--header "Authorization: Bearer $pat")
status=$(curl --silent --max-time 5 --output "$work/pat-members.body" --write-out '%{http_code}' "${bearer[@]}" -- "http://127.0.0.1:$port$org_api/members")
expect "the token reads members without a cookie, Origin or CSRF token" test "$status" = 200
status=$(api "$work/pat-rename" PATCH "$org_api" '{"Name":"Renamed"}' "${bearer[@]}")
expect "the token cannot do what its scopes leave out" test "$status:$(json_field "$work/pat-rename.body" Code)" = 403:FORBIDDEN
status=$(curl --silent --max-time 5 --output "$work/pat-me.body" --write-out '%{http_code}' "${bearer[@]}" -- "http://127.0.0.1:$port/api/v1/me")
expect "the token is refused on browser-only routes" test "$status:$(json_field "$work/pat-me.body" Code)" = 403:SESSION_REQUIRED
status=$(api "$work/pat-password" PUT /api/v1/me/password "{\"CurrentPassword\":\"$password\",\"NewPassword\":\"another smoke passphrase\"}" "${bearer[@]}")
expect "the token cannot change the password" test "$status" = 403
status=$(curl --silent --max-time 5 --output /dev/null --write-out '%{http_code}' "${bearer[@]}" --header "Cookie: $mfa_session" -- "http://127.0.0.1:$port$org_api/members")
expect "a token next to a session cookie answers 401" test "$status" = 401
status=$(curl --silent --max-time 5 --output /dev/null --write-out '%{http_code}' "${bearer[@]}" -- "http://127.0.0.1:$port/api/v1/organizations/$org2_id/members")
expect "the token sees nothing of another organization" test "$status" = 404
status=$(rpc "$work/pat-runner" dhow.runner.v1.AuthService/RotateKeys '{}' --header 'Dhow-Protocol: 1' "${bearer[@]}")
expect "the token on /runner-api/ answers 401 UNAUTHENTICATED" test "$status:$(json_field "$work/pat-runner.body" message)" = 401:UNAUTHENTICATED
status=$(curl --silent --max-time 5 --output "$work/pat-list.body" --write-out '%{http_code}' --header "Cookie: $mfa_session" -- "http://127.0.0.1:$port/api/v1/me/api-tokens")
expect "the token list answers 200" test "$status" = 200
expect "the token list never shows the value" bash -c '! grep -qF -- "$1" "$2"' _ "${pat#dhow_pat_}" "$work/pat-list.body"
status=$(api "$work/pat-revoke" POST "/api/v1/me/api-tokens/$pat_id/revoke" '' "${origin[@]}" "${session[@]}")
expect "revoking the token answers 200" test "$status" = 200
status=$(curl --silent --max-time 5 --output /dev/null --write-out '%{http_code}' "${bearer[@]}" -- "http://127.0.0.1:$port$org_api/members")
expect "the revoked token answers 401 at once" test "$status" = 401

status=$(curl --silent --max-time 5 --output "$work/prefs.body" --write-out '%{http_code}' --header "Cookie: $mfa_session" -- "http://127.0.0.1:$port/api/v1/me/preferences")
expect "/api/v1/me/preferences answers 200" test "$status" = 200
expect "preferences start unsaved" grep -q '"Saved":false' "$work/prefs.body"
status=$(api "$work/prefs-save" PUT /api/v1/me/preferences "{\"Locale\":\"en\",\"ThemeId\":\"dusk\",\"CustomThemes\":[],\"OrganizationId\":\"$org_id\"}" "${origin[@]}" "${session[@]}")
expect "saving preferences answers 200" test "$status" = 200
curl --silent --max-time 5 --output "$work/prefs2.body" --header "Cookie: $mfa_session" -- "http://127.0.0.1:$port/api/v1/me/preferences"
expect "the saved theme comes back" grep -q '"ThemeId":"dusk"' "$work/prefs2.body"
curl --silent --max-time 5 --output "$work/me4.body" --header "Cookie: $mfa_session" -- "http://127.0.0.1:$port/api/v1/me"
expect "the saved language is the account locale" grep -q '"Locale":"en"' "$work/me4.body"
status=$(api "$work/prefs-bad" PUT /api/v1/me/preferences '{"Locale":"en","ThemeId":"rainbow","CustomThemes":[],"OrganizationId":null}' "${origin[@]}" "${session[@]}")
expect "an unknown theme answers 422" test "$status" = 422

status=$(http "$work/oidc" /api/v1/oidc/providers)
expect "/api/v1/oidc/providers answers 200" test "$status" = 200
expect "no sign-in provider is configured by default" grep -q '"Data":\[\]' "$work/oidc.body"
status=$(api "$work/oidc-start" POST /api/v1/oidc/authorize '{"Provider":"google"}' "${origin[@]}")
expect "an unconfigured provider answers 404" test "$status" = 404
status=$(api "$work/oidc-cb" POST /api/v1/oidc/callback '{"Provider":"google","Code":"x","State":"y"}' "${origin[@]}")
expect "a callback without a flow answers OIDC_FLOW_INVALID" test "$status:$(json_field "$work/oidc-cb.body" Code)" = 400:OIDC_FLOW_INVALID

dhow_logs=$(compose logs --no-color dhow 2>&1)
expect "the API token never reaches the logs" bash -c '! grep -qF -- "$1" <<<"$2"' _ "${pat#dhow_pat_}" "$dhow_logs"

printf 'Rate limits\n'
limited=0
for attempt in $(seq 1 25); do
  status=$(api "$work/rl" POST /api/v1/oidc/authorize '{"Provider":"google"}' "${origin[@]}" --header "X-Forwarded-For: 192.0.2.$attempt")
  if [[ $status = 429 ]]; then
    limited=$attempt
    break
  fi
done
expect "the OIDC routes answer 429 within the limit of 20 per 10 minutes" test "$limited" -ge 1 -a "$limited" -le 19
expect "a forged X-Forwarded-For does not escape the limit" test "$limited" -ne 0
expect "the 429 is RATE_LIMITED" grep -q '"Code":"RATE_LIMITED"' "$work/rl.body"
expect "the 429 carries Retry-After" has_header "$work/rl.head" Retry-After '[0-9]'
expect "the 429 message is in Arabic by default" grep -q 'انتظر قليلًا' "$work/rl.body"
status=$(api "$work/rl-en" POST /api/v1/oidc/callback '{"Provider":"google","Code":"x","State":"y"}' "${origin[@]}" --header 'Accept-Language: en')
expect "the callback shares the OIDC counter" test "$status" = 429
expect "the 429 message is in English with Accept-Language: en" grep -q 'Wait a little and try again' "$work/rl-en.body"
status=$(api "$work/rl-pk" POST /api/v1/sessions/passkey-options '' "${origin[@]}")
expect "passkey options keep their own counter" test "$status" = 200
for n in 1 2; do
  status=$(api "$work/rl-invite-$n" POST "$org_api/invitations" "{\"Email\":\"limit$n@smoke.test\",\"Role\":\"viewer\"}" "${origin[@]}" "${session[@]}")
  expect "invitation $((n + 1)) of 3 this hour answers 201" test "$status" = 201
done
status=$(api "$work/rl-invite-3" POST "$org_api/invitations" '{"Email":"limit3@smoke.test","Role":"viewer"}' "${origin[@]}" "${session[@]}")
expect "the fourth invitation of the hour answers 429 RATE_LIMITED" test "$status:$(json_field "$work/rl-invite-3.body" Code)" = 429:RATE_LIMITED
rows=$(compose exec -T postgres psql -U dhow -d dhow -tAc 'SELECT count(*) FROM "RateLimits"' | tr -d '[:space:]')
expect "the counters live in the database" test "${rows:-0}" -ge 3

printf 'Security headers\n'
for page in web ready; do
  expect "$page: Content-Security-Policy is 'self'" has_header "$work/$page.head" Content-Security-Policy "default-src 'self'"
  expect "$page: X-Content-Type-Options" has_header "$work/$page.head" X-Content-Type-Options nosniff
  expect "$page: X-Frame-Options" has_header "$work/$page.head" X-Frame-Options DENY
  expect "$page: Referrer-Policy" has_header "$work/$page.head" Referrer-Policy no-referrer
  expect "$page: Strict-Transport-Security" has_header "$work/$page.head" Strict-Transport-Security max-age
  expect "$page: Cross-Origin-Opener-Policy" has_header "$work/$page.head" Cross-Origin-Opener-Policy same-origin
done

printf 'Hardening\n'
expect "the container user is 65532" test "$(docker inspect --format '{{.Config.User}}' "$dhow_id")" = 65532:65532
expect "the root filesystem is read-only" test "$(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}' "$dhow_id")" = true
expect "all capabilities are dropped" test "$(docker inspect --format '{{join .HostConfig.CapDrop ","}}' "$dhow_id")" = ALL
expect "dhow is published on loopback only" test "$(compose port dhow 8080)" = "127.0.0.1:$port"
expect "postgres is not published" bash -c '! docker port "$1" 2>/dev/null | grep -q .' _ "$postgres_id"
set +e
shell=$(docker exec "$dhow_id" sh -c true 2>&1)
shell_status=$?
set -e
expect "docker exec sh fails" test "$shell_status" -ne 0
expect "docker exec sh fails because there is no shell" grep -q 'executable file not found' <<<"$shell"

if ((failures != 0)); then
  printf '\n%d check(s) failed\n' "$failures" >&2
  exit 1
fi
printf '\nAll smoke checks passed\n'
