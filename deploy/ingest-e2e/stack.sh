#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  printf 'usage: %s up | verify | down\n' "$0" >&2
  printf 'up: starts Postgres, the server and Forgejo in Docker, makes a project with a pipeline that takes the artifact web,\n' >&2
  printf '    and an ingest token for main. It prints the settings as KEY=value lines (the token is masked for GitHub Actions).\n' >&2
  printf 'verify: checks that the server recorded the digest in EXPECTED_DIGEST and that a run was made for it.\n' >&2
  printf 'down: removes the stack. Settings: DHOW_INGEST_PORT (8600), DHOW_INGEST_DB_PORT (55600), DHOW_INGEST_SUBNET\n' >&2
  printf '(10.231.110.0/24), DHOW_DEMO_WORK (a folder that must stay between the commands).\n' >&2
  exit 2
}
(($# == 1)) || usage

export DHOW_DEMO_PORT=${DHOW_INGEST_PORT:-8600}
export DHOW_DEMO_DB_PORT=${DHOW_INGEST_DB_PORT:-55600}
export DHOW_DEMO_REGISTRY_PORT=${DHOW_INGEST_REGISTRY_PORT:-55601}
export DHOW_DEMO_SUBNET=${DHOW_INGEST_SUBNET:-10.231.110.0/24}
export DHOW_DEMO_FORGEJO_IP=${DHOW_INGEST_FORGEJO_IP:-10.231.110.10}
export COMPOSE_PROJECT_NAME=${COMPOSE_PROJECT_NAME:-dhow-ingest}
export DHOW_DEMO_WORK=${DHOW_DEMO_WORK:-${RUNNER_TEMP:-${TMPDIR:-/tmp}}/dhow-ingest-e2e}
token_name='ingest e2e'
ingest_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

# shellcheck source=deploy/m4-demo/lib.sh
. "$ingest_dir/../m4-demo/lib.sh"
# shellcheck source=deploy/m4-demo/project.sh
. "$demo_dir/project.sh"

work=$DHOW_DEMO_WORK
state="$work/state.env"
ingest_token=
head_sha=

prepare() {
  install -d -m 0700 "$work" "$work/server" "$work/certs"
  od -An -tx1 -N32 /dev/urandom | tr -d ' \n' >"$work/server/secret-key"
  printf 'kek 1 %s\n' "$(head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=\n')" >"$work/server/keyring"
  chmod 600 "$work/server/keyring"
  : >"$work/needles"
}

write_pipeline() {
  install -d -m 0755 "$work/repo"
  printf '# Ingest e2e\n' >"$work/repo/README.md"
  sed "s|@IMAGE@|$image|" >"$work/repo/.dhow.yml" <<'YAML'
version: 1
name: Ingest e2e
on:
  push:
    branches: [main]
  artifact:
    names: [web]
    refs: [main]
jobs:
  ship:
    image: @IMAGE@
    ingested: [web]
    steps:
      - run: ls
YAML
  chmod -R a+rX "$work/repo"
}

first_push() {
  write_pipeline
  git_in_tools init --quiet -b main && git_in_tools remote add origin "http://$forgejo_user@git.dhow-demo.test:3000/demo/app.git" || return 1
  push "Add the pipeline" || return 1
  head_sha=$(git_in_tools rev-parse HEAD | tr -d '\r')
  [[ $head_sha =~ ^[0-9a-f]{40}$ ]]
}

make_ingest_token() {
  local status
  status=$(api "$work/ingest" POST "/api/v1/organizations/$org_id/projects/$project_id/ingest-tokens" \
    "$(printf '{"Name":"ingest e2e","RefPatterns":[{"Kind":"branch","Pattern":"main"}],"ExpiresInDays":1,"AcknowledgeProtectedReach":true,"CurrentFactor":{"Method":"password","Password":"%s"}}' "$password")" \
    "${session[@]}")
  [[ $status == 201 ]] || { why "creating the ingest token answered $status"; return 1; }
  ingest_token=$(json_field "$work/ingest.body" Value)
  [[ $ingest_token =~ ^dhow_ing_[A-Za-z0-9_-]{43}$ ]]
}

save_state() {
  {
    printf 'org_id=%q\nproject_id=%q\ntoken=%q\nsession_cookie=%q\ncsrf_token=%q\nhead_sha=%q\n' \
      "$org_id" "$project_id" "$token" "$session_cookie" "$csrf_token" "$head_sha"
  } >"$state"
  chmod 600 "$state"
}

load_state() {
  [[ -f $state ]] || { printf 'no state at %s: run up first\n' "$state" >&2; exit 2; }
  # shellcheck source=/dev/null
  . "$state"
  session=(--header "Cookie: $session_cookie" --header "X-CSRF-Token: $csrf_token")
}

up() {
  prerequisites
  prepare
  clean_start
  say "Postgres, the server and Forgejo"
  docker compose up --detach --build db server forgejo tools >/dev/null
  need "the server starts and prints its setup link" server_ready
  need "the owner and the organization are set up" setup_owner
  need "an API token is made" api_token
  need "Forgejo has a user and an empty repository" forgejo_repo
  need "a project is made" project
  need "the repository is linked and its host key is pinned" link
  need "Forgejo has the deploy key and the webhook" forgejo_hooks
  need "the pipeline is pushed" first_push
  need "the push reaches the server" wait_runs 1 300
  need "an ingest token for main is made" make_ingest_token
  save_state
  printf '::add-mask::%s\n' "$ingest_token"
  emit "$(printf 'server=%s\ntoken=%s\nhead-sha=%s\n' "$public_url" "$ingest_token" "$head_sha")"
}

emit() {
  if [[ -n ${GITHUB_OUTPUT:-} ]]; then
    printf '%s\n' "$1" >>"$GITHUB_OUTPUT"
  else
    printf '%s\n' "$1"
  fi
}

artifacts_of_project() {
  api "$work/artifacts" GET "/api/v1/organizations/$org_id/projects/$project_id/ingested-artifacts" '' "${session[@]}" >/dev/null
}

recorded_digest_matches() {
  local want=${EXPECTED_DIGEST:?EXPECTED_DIGEST is not set} count
  artifacts_of_project || return 1
  count=$(grep -o '"Digest":"[0-9a-f]\{64\}"' "$work/artifacts.body" | sort -u | wc -l)
  if grep -qF "\"Digest\":\"$want\"" "$work/artifacts.body" && ((count == 1)); then
    return 0
  fi
  why "the server recorded: $(grep -o '"Digest":"[^"]*"' "$work/artifacts.body" | tr '\n' ' ') and not only $want"
  return 1
}

run_has_the_digest() {
  local run=${EXPECTED_RUN_ID:?EXPECTED_RUN_ID is not set} want=${EXPECTED_DIGEST:?EXPECTED_DIGEST is not set}
  artifacts_of_project || return 1
  jq -e --arg digest "$want" --arg run "$run" 'any(.Data[]; .Digest == $digest and .RunId == $run)' "$work/artifacts.body" >/dev/null || {
    why "no ingested artifact with the digest $want and the run $run"
    return 1
  }
  api "$work/run" GET "/api/v1/organizations/$org_id/projects/$project_id/runs/$run" '' --header "$(bearer)" >/dev/null
  grep -qF "\"Trigger\":\"artifact\"" "$work/run.body" || { why "run $run is not an artifact run: $(head -c 300 "$work/run.body")"; return 1; }
  grep -qF "\"Ref\":\"refs/heads/main\"" "$work/run.body"
}

provenance_was_recorded() {
  artifacts_of_project || return 1
  grep -qF '"Provider":"github-actions"' "$work/artifacts.body" &&
    grep -qF "\"Repository\":\"${GITHUB_REPOSITORY:-}\"" "$work/artifacts.body" &&
    grep -qF '"ProvenanceVerified":false' "$work/artifacts.body"
}

verify() {
  load_state
  check "the server recorded exactly the digest the Action reported" recorded_digest_matches
  if [[ -n ${EXPECTED_RUN_ID:-} ]]; then
    check "a run was made for that digest" run_has_the_digest
  fi
  if [[ -n ${GITHUB_REPOSITORY:-} ]]; then
    check "the provenance came from GitHub Actions and is not marked verified" provenance_was_recorded
  fi
  summary
  ((failed == 0))
}

down() {
  evidence "before the teardown"
  docker compose --profile fake-runner down --volumes --remove-orphans >/dev/null 2>&1 || true
  docker run --rm -v "$work:/w" "$DHOW_DEV_GO_IMAGE" find /w -mindepth 1 -delete >/dev/null 2>&1 || true
  rm -rf -- "$work"
}

case $1 in
  up) up ;;
  verify) verify ;;
  down) down ;;
  *) usage ;;
esac
