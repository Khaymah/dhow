#!/usr/bin/env bash
# shellcheck disable=SC2034,SC2154

token_name=${token_name:-m4 demo}
forgejo_user=${forgejo_user:-demo}
forgejo_password=
forgejo="http://git.dhow-demo.test:3000"
clone_url="ssh://git@git.dhow-demo.test/demo/app.git"
token=
project_id=
hook_url=
hook_secret=
run_ids=()

forgejo_api() {
  local prefix=$1 method=$2 path=$3 body=$4 args=() status=0
  if [[ -n $body ]]; then
    (umask 077 && printf '%s' "$body" >"$prefix.req")
    args+=(--data-binary "@$prefix.req")
  fi
  printf 'user = "%s"\n' "$(curl_quote "$forgejo_user:$forgejo_password")" |
    docker compose exec -T tools curl --config - --silent --show-error --max-time 30 --output "$prefix.body" \
      --write-out '%{http_code}' --request "$method" --header 'Content-Type: application/json' "${args[@]}" \
      -- "$forgejo/api/v1$path" || status=$?
  rm -f -- "$prefix.req"
  return "$status"
}

bearer() { printf 'Authorization: Bearer %s' "$token"; }

git_in_tools() {
  docker compose exec -T -w "$work/repo" -e GIT_ASKPASS="$work/askpass" -e GIT_TERMINAL_PROMPT=0 tools \
    git -c safe.directory='*' -c user.name='Demo' -c user.email=demo@dhow-demo.test "$@"
}

askpass() {
  (umask 077 && printf '%s' "$forgejo_password" >"$work/forgejo-password")
  printf '#!/bin/sh\ncat -- %s\n' "$work/forgejo-password" >"$work/askpass"
  chmod 0700 "$work/askpass"
}

needle() { printf '%s=%s\n' "$1" "$2" >>"$work/needles"; }

forgejo_repo() {
  wait_http "$forgejo/api/healthz" 300 || return 1
  forgejo_password=$(docker compose exec -T -u git forgejo forgejo admin user create --admin --username "$forgejo_user" \
    --random-password --email demo@dhow-demo.test --must-change-password=false | sed -n "s/.*random password is '\\(.*\\)'.*/\\1/p")
  [[ -n $forgejo_password ]] || return 1
  askpass
  [[ $(forgejo_api "$work/repo-create" POST /user/repos '{"name":"app","default_branch":"main","private":true}') == 2?? ]]
}

api_token() {
  local status
  status=$(api "$work/token" POST "/api/v1/organizations/$org_id/api-tokens" \
    "$(printf '{"Name":"%s","Scopes":["project.read","project.manage","run.read","run.trigger","job.read"],"ExpiresInDays":1,"CurrentFactor":{"Method":"password","Password":"%s"}}' "$token_name" "$password")" \
    "${session[@]}")
  [[ $status == 2?? ]] || return 1
  token=$(json_field "$work/token.body" Value)
  [[ -n $token ]] && needle "API token" "$token"
}

project() {
  [[ $(api "$work/project" POST "/api/v1/organizations/$org_id/projects" '{"Name":"Demo app","Slug":"demo-app"}' --header "$(bearer)") == 2?? ]] || return 1
  project_id=$(json_field "$work/project.body" Id)
  [[ -n $project_id ]]
}

push() {
  local message=$1
  git_in_tools add --all && git_in_tools commit --quiet -m "$message" && git_in_tools push --quiet origin main 2>&1
}

link() {
  local fingerprint ours
  [[ $(api "$work/link" PUT "/api/v1/organizations/$org_id/projects/$project_id/repository" \
    "$(printf '{"CloneUrl":"%s","DefaultBranch":"main"}' "$clone_url")" --header "$(bearer)") == 2?? ]] || return 1
  hook_secret=$(json_field "$work/link.body" WebhookSecret)
  hook_url=$(json_field "$work/link.body" WebhookUrl)
  hook_url="$base${hook_url#"$public_url"}"
  fingerprint=$(json_field "$work/link.body" PendingHostKeyFingerprint)
  [[ -n $hook_secret && -n $fingerprint ]] || return 1
  needle "webhook secret shown once" "$hook_secret"
  ours=$(docker compose exec -T forgejo find /data/ssh -name 'ssh_host_*_key.pub' -exec ssh-keygen -lf {} \; | cut -d' ' -f2)
  grep -qxF -- "$fingerprint" <<<"$ours" || {
    printf '    the pending host key %s is not one of Forgejo'\''s keys\n' "$fingerprint" >&2
    return 1
  }
  [[ $(api "$work/confirm" POST "/api/v1/organizations/$org_id/projects/$project_id/repository/confirm-host-key" \
    "$(printf '{"Fingerprint":"%s"}' "$fingerprint")" --header "$(bearer)") == 2?? ]]
}

forgejo_hooks() {
  local key
  key=$(json_field "$work/link.body" DeployKeyPublic)
  [[ $(forgejo_api "$work/key" POST /repos/demo/app/keys "$(printf '{"title":"dhow","key":"%s","read_only":true}' "$key")") == 2?? ]] || return 1
  [[ $(forgejo_api "$work/hook" POST /repos/demo/app/hooks \
    "$(printf '{"type":"forgejo","active":true,"events":["push"],"config":{"url":"%s","content_type":"json","secret":"%s"}}' "$hook_url" "$hook_secret")") == 2?? ]]
}

list_runs() {
  local ids numbers
  run_ids=()
  api "$work/runs" GET "/api/v1/organizations/$org_id/projects/$project_id/runs" '' --header "$(bearer)" >/dev/null || return 1
  ids=$(grep -o '"Id":"[^"]*"' "$work/runs.body" | cut -d'"' -f4)
  numbers=$(grep -o '"Number":[0-9]*' "$work/runs.body" | cut -d: -f2)
  [[ $(wc -l <<<"$ids") == $(wc -l <<<"$numbers") ]] || return 1
  [[ -n $ids ]] || return 0
  mapfile -t run_ids < <(paste -d' ' <(printf '%s\n' "$numbers") <(printf '%s\n' "$ids") | sort -n | cut -d' ' -f2)
}

run_count() {
  list_runs
  printf '%d' "${#run_ids[@]}"
}

wait_runs() {
  local want=$1 seconds=${2:-1200}
  for _ in $(seq "$seconds"); do
    list_runs
    ((${#run_ids[@]} >= want)) && return 0
    sleep 1
  done
  why "the project has ${#run_ids[@]} runs after $seconds seconds, not $want; the last webhook delivery: $(json_field "$work/runs.body" Message)"
  return 1
}

run_field() { sed 's/.*"Run":{//' "$work/run.body" | grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }

run_ends() {
  local id=$1 state
  for _ in $(seq 1800); do
    api "$work/run" GET "/api/v1/organizations/$org_id/projects/$project_id/runs/$id" '' --header "$(bearer)" >/dev/null
    state=$(run_field State)
    case $state in
      succeeded) return 0 ;;
      failed | canceled | skipped)
        printf '    run %s ended %s: %s %s\n' "$id" "$state" "$(run_field FailureReason)" \
          "$(grep -o '"FailureReason":"[^"]*"' "$work/run.body" | sort -u | tr '\n' ' ')" >&2
        return 1
        ;;
    esac
    sleep 1
  done
  return 1
}

checkout_image() {
  local registry="127.0.0.1:$DHOW_DEMO_REGISTRY_PORT/dhow-git:demo" digest
  docker run --rm -v "$root:/src:ro" -v "$work/certs:/certs" "$DHOW_DEV_GO_IMAGE" bash /src/deploy/m4-demo/certs.sh /certs
  docker compose up --detach --wait registry >/dev/null
  docker build --quiet --tag "$registry" "$root/deploy/dhow-git" >/dev/null
  docker push --quiet "$registry" >/dev/null
  digest=$(docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$registry" | grep -o '@sha256:[0-9a-f]\{64\}' | head -1)
  docker image rm "$registry" >/dev/null
  [[ -n $digest ]] || return 1
  export DHOW_DEMO_CHECKOUT_IMAGE="registry.dhow-demo.test/dhow-git$digest"
  printf '    checkout image %s\n' "$DHOW_DEMO_CHECKOUT_IMAGE"
}

sealed_values() {
  local name value scope path
  api "$work/environment" POST "/api/v1/organizations/$org_id/projects/$project_id/environments" \
    "$(printf '{"Name":"staging","CurrentFactor":{"Method":"password","Password":"%s"}}' "$password")" "${session[@]}" >/dev/null || return 1
  for scope in "organization secret:/api/v1/organizations/$org_id" \
    "project secret:/api/v1/organizations/$org_id/projects/$project_id" \
    "environment secret:/api/v1/organizations/$org_id/projects/$project_id/environments/staging"; do
    name=${scope%%:*}
    path=${scope#*:}
    value="demo-$(od -An -tx1 -N18 /dev/urandom | tr -d ' \n')"
    [[ $(api "$work/secret" PUT "$path/secrets/DEMO_${name%% *}_SECRET" "$(printf '{"Value":"%s"}' "$value")" "${session[@]}") == 2?? ]] || return 1
    needle "$name" "$value"
  done
  value="demo-$(od -An -tx1 -N18 /dev/urandom | tr -d ' \n')"
  [[ $(api "$work/registry" POST "/api/v1/organizations/$org_id/registry-credentials" \
    "$(printf '{"Host":"registry.dhow-demo.test","Username":"demo","Password":"%s"}' "$value")" "${session[@]}") == 2?? ]] || return 1
  needle "registry password" "$value"
}
