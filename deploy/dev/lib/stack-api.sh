#!/usr/bin/env bash
# shellcheck disable=SC2034,SC2154

stack_curl=(curl)
owner_name=${owner_name:-Dev Stack}
owner_email=${owner_email:-owner@dev-stack.test}
org_name=${org_name:-Dev Stack}
org_slug=${org_slug:-dev-stack}
session=()
org_id=
runner_id=
job_id=
job_state=
registration_token=

json_field() { grep -o "\"$2\":\"[^\"]*\"" "$1" | head -1 | cut -d'"' -f4; }

json_number() { grep -o "\"$2\":[0-9]*" "$1" | head -1 | cut -d: -f2; }

job_field() { sed 's/.*"Job":{//' "$1" | grep -o "\"$2\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }

job_number() { sed 's/.*"Job":{//' "$1" | grep -o "\"$2\":[0-9]*" | head -1 | cut -d: -f2; }

api() {
  local prefix=$1 method=$2 path=$3 body=$4
  shift 4
  "${stack_curl[@]}" --silent --show-error --max-time 30 --dump-header "$prefix.head" --output "$prefix.body" \
    --write-out '%{http_code}' --request "$method" --header 'Content-Type: application/json' \
    --header "Origin: $public_url" "$@" ${body:+--data "$body"} -- "$base$path" </dev/null
}

setup_owner() {
  local setup_token status cookie
  setup_token=$(docker compose logs --no-color server | grep -o "$public_url/setup#[A-Za-z0-9_-]*" | tail -1 | cut -d'#' -f2)
  [[ -n $setup_token ]] || return 1
  status=$(api "$work/setup" POST /api/v1/setup "$(printf '{"SetupToken":"%s","OwnerName":"%s","Email":"%s","Password":"%s","OrganizationName":"%s","OrganizationSlug":"%s"}' \
    "$setup_token" "$owner_name" "$owner_email" "$password" "$org_name" "$org_slug")")
  [[ $status == 200 ]] || return 1
  status=$(api "$work/login" POST /api/v1/sessions "$(printf '{"Email":"%s","Password":"%s"}' "$owner_email" "$password")")
  [[ $status == 200 ]] || return 1
  cookie=$(sed -n 's/^[Ss]et-[Cc]ookie: \([^=;]*dhow_session=[^;]*\);.*/\1/p' "$work/login.head" | tr -d '\r' | head -1)
  session=(--header "Cookie: $cookie" --header "X-CSRF-Token: $(json_field "$work/login.body" CsrfToken)")
  api "$work/me" GET /api/v1/me '' "${session[@]}" >/dev/null
  org_id=$(json_field "$work/me.body" OrganizationId)
  [[ -n $org_id ]]
}

register() {
  local service=$1 status token
  status=$(api "$work/rrt" POST "/api/v1/organizations/$org_id/runner-registration-tokens" '{"Labels":["pool=dev"]}' "${session[@]}")
  [[ $status == 201 ]] || return 1
  token=$(json_field "$work/rrt.body" Token)
  registration_token=$token
  printf '%s\n' "$token" | docker compose exec -T "$service" dhow-dev-register >"$work/register.log" 2>&1 || {
    cat -- "$work/register.log" >&2
    return 1
  }
  cat -- "$work/register.log"
  runner_id=$(sed -n 's/^Registered runner \([0-9a-f-]*\) .*/\1/p' "$work/register.log")
  [[ -n $runner_id ]] && ! grep -qF -- "$token" "$work/register.log"
}

online() {
  for _ in $(seq 120); do
    api "$work/runner" GET "/api/v1/organizations/$org_id/runners/$runner_id" '' "${session[@]}" >/dev/null
    [[ $(json_field "$work/runner.body" Status) == online ]] && return 0
    sleep 1
  done
  return 1
}

ready() {
  local service=$1 log
  for _ in $(seq 120); do
    log=$(docker compose logs --no-color "$service" 2>/dev/null) || true
    if grep -F -- '"msg":"the runner is ready"' <<<"$log" | grep -qF -- "\"runner_id\":\"$runner_id\""; then
      return 0
    fi
    sleep 1
  done
  return 1
}

create_job() {
  local name=$1 spec=$2 extra=${3:-} status
  status=$(api "$work/new-job" POST "/api/v1/organizations/$org_id/jobs" "$(printf '{"Name":"%s","Spec":%s%s}' "$name" "$spec" "$extra")" "${session[@]}")
  if [[ $status != 201 ]]; then
    cat -- "$work/new-job.body" >&2
    return 1
  fi
  job_id=$(job_field "$work/new-job.body" Id)
}

wait_job() {
  local id=$1 seconds=${2:-300}
  job_state=
  for _ in $(seq "$seconds"); do
    api "$work/job" GET "/api/v1/organizations/$org_id/jobs/$id" '' "${session[@]}" >/dev/null
    job_state=$(job_field "$work/job.body" State)
    case $job_state in
      succeeded | failed | canceled | lost | timed_out) return 0 ;;
    esac
    sleep 1
  done
  return 1
}
