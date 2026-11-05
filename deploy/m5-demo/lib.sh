#!/usr/bin/env bash
# shellcheck disable=SC2034,SC2154

reviewer_name='Demo Reviewer'
reviewer_email='reviewer@dhow-demo.test'
reviewer_password='a long demo reviewer passphrase'
reviewer_id=
reviewer_totp=
reviewer_session=()
owner_id=
owner_totp=
enrolled_secret=
approval_id=
decision_status=
error_code=
manual_run_id=
database_url='postgres://dhow:dhow-dev-only@db:5432/dhow?sslmode=disable'
secret_lengths='41,41,41'

project_path() { printf '/api/v1/organizations/%s/projects/%s' "$org_id" "$project_id"; }

totp_code() {
  printf '%s' "$2" | docker compose exec -T tools go run ./deploy/m5-demo/totp "$work/totp-$1" | tr -d '\r\n'
}

ok_status() {
  # shellcheck disable=SC2254
  case $2 in
    $3) return 0 ;;
  esac
  printf '    %s answered %s %s\n' "${1##*/}" "$2" "$(json_field "$1.body" Code)" >&2
  return 1
}

user_id_of() { sed 's/.*"User":{//' "$1" | grep -o '"Id":"[^"]*"' | head -1 | cut -d'"' -f4; }

session_cookie_of() { sed -n 's/^[Ss]et-[Cc]ookie: \([^=;]*dhow_session=[^;]*\);.*/\1/p' "$1" | tr -d '\r' | head -1; }

invite_reviewer() {
  local link token
  ok_status "$work/invite" "$(api "$work/invite" POST "/api/v1/organizations/$org_id/invitations" \
    "$(printf '{"Email":"%s","Role":"developer"}' "$reviewer_email")" "${session[@]}")" '2??' || return 1
  link=$(json_field "$work/invite.body" Link)
  token=${link#*#}
  [[ -n $link && $token != "$link" && -n $token ]] || return 1
  needle "invitation token" "$token"
  needle "reviewer password" "$reviewer_password"
  ok_status "$work/accept" "$(api "$work/accept" POST /api/v1/invitations/accept \
    "$(printf '{"Token":"%s","Name":"%s","Password":"%s"}' "$token" "$reviewer_name" "$reviewer_password")")" 200 || return 1
  reviewer_id=$(user_id_of "$work/accept.body")
  [[ -n $reviewer_id ]]
}

login_as() {
  local name=$1 email=$2 pass=$3 who=$4 secret=$5 status mfa cookie csrf last
  login_session=()
  last=$work/$name-login
  status=$(api "$last" POST /api/v1/sessions "$(printf '{"Email":"%s","Password":"%s"}' "$email" "$pass")")
  if [[ $status == 202 ]]; then
    [[ -n $secret ]] || return 1
    mfa=$(sed -n 's/^[Ss]et-[Cc]ookie: \([^=;]*dhow_mfa=[^;]*\);.*/\1/p' "$last.head" | tr -d '\r' | head -1)
    [[ -n $mfa ]] || return 1
    last=$work/$name-mfa
    status=$(api "$last" POST /api/v1/sessions/mfa \
      "$(printf '{"Method":"totp","Code":"%s"}' "$(totp_code "$who" "$secret")")" --header "Cookie: $mfa")
  fi
  ok_status "$last" "$status" 200 || return 1
  cookie=$(session_cookie_of "$last.head")
  csrf=$(json_field "$last.body" CsrfToken)
  [[ -n $cookie && -n $csrf ]] || return 1
  needle "$who session cookie" "${cookie#*=}"
  needle "$who CSRF token" "$csrf"
  login_session=(--header "Cookie: $cookie" --header "X-CSRF-Token: $csrf")
}

enroll_totp() {
  local who=$1 pass=$2 secret factor
  shift 2
  factor=$(printf '{"Method":"password","Password":"%s"}' "$pass")
  ok_status "$work/$who-totp" "$(api "$work/$who-totp" POST /api/v1/me/totp "$(printf '{"CurrentFactor":%s}' "$factor")" "$@")" '2??' || return 1
  secret=$(json_field "$work/$who-totp.body" Secret)
  [[ -n $secret ]] || return 1
  needle "$who TOTP secret" "$secret"
  ok_status "$work/$who-totp-confirm" "$(api "$work/$who-totp-confirm" POST /api/v1/me/totp/confirm \
    "$(printf '{"Code":"%s","CurrentFactor":%s}' "$(totp_code "$who" "$secret")" "$factor")" "$@")" '2??' || return 1
  enrolled_secret=$secret
}

reviewer_ready() {
  login_as reviewer-first "$reviewer_email" "$reviewer_password" reviewer '' || return 1
  enroll_totp reviewer "$reviewer_password" "${login_session[@]}" || return 1
  reviewer_totp=$enrolled_secret
  login_as reviewer "$reviewer_email" "$reviewer_password" reviewer "$reviewer_totp" || return 1
  reviewer_session=("${login_session[@]}")
}

owner_ready() {
  owner_id=$(user_id_of "$work/login.body")
  [[ -n $owner_id ]] || return 1
  enroll_totp owner "$password" "${session[@]}" || return 1
  owner_totp=$enrolled_secret
}

production_environment() {
  ok_status "$work/production" "$(api "$work/production" POST "$(project_path)/environments" \
    "$(printf '{"Name":"production","CurrentFactor":{"Method":"password","Password":"%s"},"Rules":{"AllowedRefs":[{"Kind":"branch","Pattern":"main"}],"Reviewers":{"UserIds":["%s"],"TeamIds":[]}}}' \
      "$password" "$reviewer_id")" "${session[@]}")" 201
}

environment_rules() {
  api "$work/environment-read" GET "$(project_path)/environments/production" '' --header "$(bearer)" >/dev/null || return 1
  grep -qF '"Pattern":"main"' "$work/environment-read.body" && [[ $(grep -o '"Pattern":' "$work/environment-read.body" | wc -l) == 1 ]] &&
    ! grep -qF '"AllowPullRequests":true' "$work/environment-read.body" && return 0
  why "the rules of production: $(head -c 600 "$work/environment-read.body")"
  return 1
}

owner_joins_reviewers() {
  local version
  api "$work/environment-read" GET "$(project_path)/environments/production" '' --header "$(bearer)" >/dev/null || return 1
  version=$(json_number "$work/environment-read.body" Version)
  [[ -n $version ]] || return 1
  ok_status "$work/environment-update" "$(api "$work/environment-update" PUT "$(project_path)/environments/production" \
    "$(printf '{"Version":%s,"CurrentFactor":{"Method":"totp","Code":"%s"},"Rules":{"Reviewers":{"UserIds":["%s","%s"],"TeamIds":[]}}}' \
      "$version" "$(totp_code owner "$owner_totp")" "$reviewer_id" "$owner_id")" "${session[@]}")" 200
}

push_ref() {
  local branch=$1 change
  change="change-${branch//\//-}-$(date -u +%Y%m%dT%H%M%S%N).txt"
  git_in_tools checkout --quiet main || return 1
  [[ $branch == main ]] || git_in_tools checkout --quiet -B "$branch" || return 1
  printf 'A change on %s.\n' "$branch" >"$work/repo/$change"
  chmod 0644 "$work/repo/$change"
  git_in_tools add --all && git_in_tools commit --quiet -m "Change $branch" &&
    git_in_tools push --quiet origin "HEAD:refs/heads/$branch" 2>&1 || return 1
  git_in_tools checkout --quiet main
}

run_detail() {
  api "$work/run" GET "$(project_path)/runs/$1" '' --header "$(bearer)" >/dev/null
}

run_settled() {
  local id=$1
  for _ in $(seq 900); do
    run_detail "$id" || true
    case $(run_field State) in
      succeeded | failed | canceled | skipped) return 0 ;;
    esac
    sleep 1
  done
  return 1
}

job_skipped_by_ref_rule() {
  run_settled "$1" || return 1
  grep -qF '"FailureReason":"ENVIRONMENT_REF_NOT_ALLOWED"' "$work/run.body" && grep -qF '"State":"skipped"' "$work/run.body" &&
    ! grep -qF '"Gate":' "$work/run.body" && return 0
  why "the run after the push to the feature branch: $(head -c 800 "$work/run.body")"
  return 1
}

run_is_running() {
  run_detail "$1" || return 1
  [[ $(run_field State) == running ]] && return 0
  why "the run is $(run_field State), not running"
  return 1
}

wait_gate() {
  local id=$1 gate=$2
  for _ in $(seq 900); do
    run_detail "$id" || true
    grep -qF "\"Gate\":\"$gate\"" "$work/run.body" && return 0
    case $(run_field State) in
      succeeded | failed | canceled | skipped) break ;;
    esac
    sleep 1
  done
  why "no job of run $id reached the $gate gate: $(head -c 800 "$work/run.body")"
  return 1
}

pending_approval() {
  api "$work/approvals" GET "$(project_path)/runs/$1/approvals" '' --header "$(bearer)" >/dev/null || return 1
  approval_id=$(json_field "$work/approvals.body" Id)
  [[ -n $approval_id && $(json_field "$work/approvals.body" State) == pending ]]
}

decide() {
  local who=$1 action=$2 run=$3 secret code headers=()
  case $who in
    owner) secret=$owner_totp headers=("${session[@]}") ;;
    reviewer) secret=$reviewer_totp headers=("${reviewer_session[@]}") ;;
    *) return 1 ;;
  esac
  code=$(totp_code "$who" "$secret") || return 1
  decision_status=$(api "$work/decision" POST "$(project_path)/runs/$run/approvals/$approval_id/$action" \
    "$(printf '{"CurrentFactor":{"Method":"totp","Code":"%s"},"Comment":"demo"}' "$code")" "${headers[@]}")
  error_code=$(json_field "$work/decision.body" Code)
}

refused_with() {
  local who=$1 run=$2 status=$3 code=$4
  decide "$who" approve "$run" || return 1
  printf '    %s approving answered %s %s\n' "$who" "$decision_status" "$error_code"
  [[ $decision_status == "$status" && $error_code == "$code" ]] || return 1
  pending_approval "$run" && run_detail "$run" && grep -qF '"Gate":"approval"' "$work/run.body"
}

approved_by_reviewer() {
  decide reviewer approve "$1" || return 1
  printf '    the reviewer approving answered %s\n' "$decision_status"
  [[ $decision_status == 200 ]]
}

run_logs() {
  local job
  run_detail "$1" || return 1
  : >"$work/logs.txt"
  while IFS= read -r job; do
    api "$work/job-log" GET "/api/v1/organizations/$org_id/jobs/$job/logs" '' "${session[@]}" >/dev/null
    cat -- "$work/job-log.body" >>"$work/logs.txt"
  done < <(grep -o '"CurrentJobId":"[^"]*"' "$work/run.body" | cut -d'"' -f4)
}

saw() { grep -qF -- "$1" "$work/logs.txt"; }

deployed_with_secrets() {
  run_logs "$1" || return 1
  saw "dhow-demo deploy secrets=$secret_lengths" && saw 'dhow-demo deploy prefix=ok' && return 0
  why "the demo lines seen: $(grep -o 'dhow-demo [^"\\]*' "$work/logs.txt" | sort -u | tr '\n' ';')"
  return 1
}

artifact_kept() {
  api "$work/artifacts" GET "$(project_path)/runs/$1/artifacts" '' "${session[@]}" >/dev/null
  grep -qF '"Name":"report"' "$work/artifacts.body" && return 0
  why "the artifacts of the run: $(head -c 400 "$work/artifacts.body")"
  return 1
}

deployment_recorded() {
  api "$work/deployments" GET "$(project_path)/environments/production/deployments" '' --header "$(bearer)" >/dev/null || return 1
  grep -qF "\"RunId\":\"$1\"" "$work/deployments.body" && grep -qF '"State":"succeeded"' "$work/deployments.body" && return 0
  why "the deployments: $(head -c 600 "$work/deployments.body")"
  return 1
}

manual_run() {
  ok_status "$work/manual" "$(api "$work/manual" POST "$(project_path)/runs" '{"Ref":"main"}' "${session[@]}")" 201 || return 1
  manual_run_id=$(json_field "$work/manual.body" Id)
  [[ -n $manual_run_id ]]
}

cancel_run() {
  ok_status "$work/cancel" "$(api "$work/cancel" POST "$(project_path)/runs/$1/cancel" '' "${session[@]}")" 200 || return 1
  for _ in $(seq 120); do
    run_detail "$1" || true
    [[ $(run_field State) == canceled ]] && return 0
    sleep 1
  done
  return 1
}

keys_exec() {
  docker compose exec -T -e DHOW_DEV=true -e "DHOW_DATABASE_URL=$database_url" server /tmp/dhow-server keys "$@" --file /tmp/dhow-keyring </dev/null
}

kek_added() {
  keys_exec generate >"$work/keys-generate.out" 2>&1 || {
    cat -- "$work/keys-generate.out" >&2
    return 1
  }
  docker compose exec -T server cat /tmp/dhow-keyring </dev/null >"$work/server/keyring.next" || return 1
  [[ $(head -n 1 "$work/server/keyring.next") == "$(head -n 1 "$work/server/keyring")" ]] && grep -q '^kek 2 ' "$work/server/keyring.next" || return 1
  chmod 600 "$work/server/keyring.next"
  mv -- "$work/server/keyring.next" "$work/server/keyring"
}

server_restarted() {
  docker compose restart server >/dev/null 2>&1 || return 1
  wait_http "$base/api/v1/health/ready" 900 || { why "the server never answered /api/v1/health/ready after the restart"; return 1; }
  online || { why "the runner $runner_id did not come back online"; return 1; }
}

kek_pending() {
  for _ in $(seq 60); do
    keys_exec status >"$work/keys-status.out" 2>&1 || true
    grep -Eq '^2[[:space:]]+pending' "$work/keys-status.out" && return 0
    sleep 2
  done
  why "keys status: $(tr '\n' '|' <"$work/keys-status.out")"
  return 1
}

kek_activated() {
  for _ in $(seq 90); do
    if keys_exec activate 2 >"$work/keys-activate.out" 2>&1; then return 0; fi
    sleep 5
  done
  why "keys activate: $(tr '\n' '|' <"$work/keys-activate.out")"
  return 1
}

data_keys_rewrapped() {
  for _ in $(seq 120); do
    keys_exec status >"$work/keys-status.out" 2>&1 || true
    if grep -Eq '^Data keys to re-wrap:[[:space:]]+0[[:space:]]*$' "$work/keys-status.out" &&
      grep -Eq '^2[[:space:]]+active[[:space:]]+[1-9]' "$work/keys-status.out" && grep -Eq '^1[[:space:]]+retired[[:space:]]+0' "$work/keys-status.out"; then
      return 0
    fi
    sleep 5
  done
  why "keys status: $(tr '\n' '|' <"$work/keys-status.out")"
  return 1
}
