#!/usr/bin/env bash
# shellcheck disable=SC2034,SC2154

ssh_plugin=
compose_plugin=
app_ref=
current_run=
rollback_run=
rollback_to=
remembered=
first_release=
third_release=
ingest_token=
head_sha=
pinned_fingerprint=
target_dir=/srv/app
compose_project=demo-shop
key_secret=WEB_SSH_KEY

env_path() { printf '%s/environments/production' "$(project_path)"; }

target_path() { printf '%s/targets/%s' "$(env_path)" "$1"; }

host_id_of() { sed 's/.*"Hosts":\[//' "$1" | grep -o '"Id":"[^"]*"' | head -1 | cut -d'"' -f4; }

plugin_image() {
  local name=$1 dockerfile=$2 out=$3 ref="127.0.0.1:$DHOW_DEMO_REGISTRY_PORT/khaymah/$1:demo" digest
  docker build --quiet --tag "$ref" --file "$root/$dockerfile" "$root" >/dev/null || return 1
  docker push --quiet "$ref" >/dev/null || return 1
  digest=$(docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$ref" | grep -o '@sha256:[0-9a-f]\{64\}' | head -1)
  docker image rm "$ref" >/dev/null
  [[ -n $digest ]] || return 1
  printf -v "$out" '%s' "ghcr.io/khaymah/$name$digest"
  printf '    %s\n' "${!out}"
}

plugin_images() {
  plugin_image dhow-deploy-ssh apps/plugins/deploy-ssh/Dockerfile ssh_plugin || return 1
  plugin_image dhow-deploy-compose apps/plugins/deploy-compose/Dockerfile compose_plugin || return 1
  export DHOW_DEMO_DEPLOY_SSH_IMAGE=$ssh_plugin
}

hosts_up() {
  local service
  for service in sshd compose-host; do
    for _ in $(seq 240); do
      docker compose exec -T "$service" test -s /etc/ssh/ssh_host_ed25519_key.pub </dev/null 2>/dev/null &&
        docker compose exec -T "$service" nc -z 127.0.0.1 22 </dev/null 2>/dev/null && break
      sleep 1
    done
  done
  for _ in $(seq 240); do
    docker compose exec -T compose-host docker info </dev/null >/dev/null 2>&1 &&
      docker compose exec -T compose-host wget -q -O /dev/null http://127.0.0.1:5000/v2/ </dev/null 2>/dev/null && return 0
    sleep 1
  done
  why "the Docker host never became ready"
  return 1
}

own_key() {
  docker compose exec -T forgejo ssh-keygen -q -t ed25519 -N '' -C dhow-m6-demo -f /tmp/m6-key </dev/null || return 1
  docker compose exec -T forgejo cat /tmp/m6-key </dev/null >"$work/target-key" || return 1
  docker compose exec -T forgejo cat /tmp/m6-key.pub </dev/null >"$work/target-key.pub" || return 1
  docker compose exec -T forgejo rm -f /tmp/m6-key /tmp/m6-key.pub </dev/null
  [[ $(wc -l <"$work/target-key") -ge 4 ]] || return 1
  needle "target private key, line 1" "$(sed -n 2p "$work/target-key")"
  needle "target private key, line 2" "$(sed -n 3p "$work/target-key")"
  needle "target private key, line 3" "$(sed -n 4p "$work/target-key")"
}

store_target_key() {
  local value
  value=$(sed -z 's/\n/\\n/g' "$work/target-key")
  ok_status "$work/key-secret" "$(api "$work/key-secret" PUT "$(env_path)/secrets/$key_secret" \
    "$(printf '{"Value":"%s"}' "$value")" "${session[@]}")" '2??'
}

authorize_key() {
  printf 'restrict %s\n' "$2" | docker compose exec -T -u deploy "$1" tee /home/deploy/.ssh/authorized_keys >/dev/null
}

create_target() {
  local name=$1 kind=$2 host=$3 extra=$4
  ok_status "$work/target-$name" "$(api "$work/target-$name" POST "$(env_path)/targets" \
    "$(printf '{"Name":"%s","Kind":"%s","Username":"deploy","DeployPath":"%s",%s"Hosts":[{"Host":"%s"}],"CurrentFactor":{"Method":"password","Password":"%s"}}' \
      "$name" "$kind" "$target_dir" "$extra" "$host" "$password")" "${session[@]}")" 201
}

generate_key() {
  local name=$1 pub
  ok_status "$work/genkey-$name" "$(api "$work/genkey-$name" POST "$(target_path "$name")/generate-key" \
    "$(printf '{"CurrentFactor":{"Method":"password","Password":"%s"}}' "$password")" "${session[@]}")" '2??' || return 1
  pub=$(json_field "$work/genkey-$name.body" PublicKey)
  [[ $pub == ssh-ed25519\ * ]] || return 1
  printf '%s' "$pub" >"$work/genkey-$name.pub"
  ! grep -q 'PRIVATE KEY' "$work/genkey-$name.body"
}

pending_host_key() {
  local name=$1 service=$2 host line ours fingerprint
  host=$(host_id_of "$work/target-$name.body")
  [[ -n $host ]] || return 1
  line=$(docker compose exec -T "$service" cat /etc/ssh/ssh_host_ed25519_key.pub </dev/null | tr -d '\r')
  ok_status "$work/pending-$name" "$(api "$work/pending-$name" PUT "$(target_path "$name")/hosts/$host/pending-host-key" \
    "$(printf '{"HostKey":"%s"}' "$line")" "${session[@]}")" '2??' || return 1
  fingerprint=$(json_field "$work/pending-$name.body" PendingHostKeyFingerprint)
  ours=$(docker compose exec -T "$service" ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub </dev/null | cut -d' ' -f2)
  [[ -n $fingerprint && $fingerprint == "$ours" ]] || {
    why "the pending fingerprint is $fingerprint and the server says $ours"
    return 1
  }
  pinned_fingerprint=$fingerprint
}

wrong_fingerprint_refused() {
  local name=$1 host status
  host=$(host_id_of "$work/target-$name.body")
  status=$(api "$work/wrong-$name" POST "$(target_path "$name")/hosts/$host/confirm-host-key" \
    "$(printf '{"Fingerprint":"SHA256:%s","CurrentFactor":{"Method":"password","Password":"%s"}}' "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" "$password")" "${session[@]}")
  [[ $status == 4?? && $(json_field "$work/wrong-$name.body" Code) == HOST_KEY_FINGERPRINT_MISMATCH ]] && return 0
  why "a wrong fingerprint answered $status $(json_field "$work/wrong-$name.body" Code)"
  return 1
}

confirm_host_key() {
  local name=$1 host
  host=$(host_id_of "$work/target-$name.body")
  ok_status "$work/confirm-$name" "$(api "$work/confirm-$name" POST "$(target_path "$name")/hosts/$host/confirm-host-key" \
    "$(printf '{"Fingerprint":"%s","CurrentFactor":{"Method":"password","Password":"%s"}}' "$pinned_fingerprint" "$password")" "${session[@]}")" '2??'
}

target_ready() {
  api "$work/ready-$1" GET "$(target_path "$1")" '' "${session[@]}" >/dev/null || return 1
  grep -qF '"Ready":true' "$work/ready-$1.body" && return 0
  why "the target $1: $(head -c 500 "$work/ready-$1.body")"
  return 1
}

target_checked() {
  local name=$1 run
  ok_status "$work/check-$name" "$(api "$work/check-$name" POST "$(target_path "$name")/check" '' "${session[@]}")" 202 || return 1
  run=$(json_field "$work/check-$name.body" RunId)
  [[ -n $run ]] && run_ends "$run" || return 1
  api "$work/checked-$name" GET "$(target_path "$name")/check" '' "${session[@]}" >/dev/null || return 1
  grep -qF '"Reason":"ok"' "$work/checked-$name.body" && return 0
  why "the check of $name: $(head -c 500 "$work/checked-$name.body")"
  return 1
}

releases_of() { api "$work/releases-$1" GET "$(target_path "$1")/releases" '' "${session[@]}" >/dev/null; }

release_has() {
  local name=$1
  shift
  releases_of "$name" || return 1
  local pattern
  for pattern in "$@"; do
    grep -qF -- "$pattern" "$work/releases-$name.body" || {
      why "the releases of $name have no $pattern: $(head -c 700 "$work/releases-$name.body")"
      return 1
    }
  done
}

served() {
  local want=$2 got
  for _ in $(seq 30); do
    got=$(docker compose exec -T tools curl -fsS --max-time 5 -- "http://$sshd_ip:8080$1" 2>/dev/null | tr -d '\r\n') || true
    [[ $got == "$want" ]] && return 0
    sleep 1
  done
  why "http://$sshd_ip:8080$1 answered '$got' and not '$want'"
  return 1
}

current_release() { docker compose exec -T sshd readlink "$target_dir/current" </dev/null | tr -d '\r\n'; }

current_is() {
  local got
  got=$(current_release)
  [[ $got == "releases/$1" ]] && return 0
  why "current points to '$got' and not to releases/$1"
  return 1
}

run_failed_with() {
  run_settled "$1" || return 1
  [[ $(run_field State) == failed ]] && grep -qF "\"FailureReason\":\"$2\"" "$work/run.body" && return 0
  why "run $1 is $(run_field State) and has $(grep -o '"FailureReason":"[^"]*"' "$work/run.body" | sort -u | tr '\n' ' ')"
  return 1
}

write_site() {
  local version=$1 broken=${2:-healthy}
  install -d -m 0755 "$work/repo"
  printf '%s\n' "$version" >"$work/repo/index.html"
  rm -f -- "$work/repo/unhealthy"
  if [[ $broken == broken ]]; then : >"$work/repo/unhealthy"; fi
  chmod -R a+rX "$work/repo"
}

push_and_wait() {
  local before
  before=$(run_count)
  push "$1" || return 1
  wait_runs $((before + 1)) 300 || return 1
  current_run=${run_ids[${#run_ids[@]}-1]}
}

run_number() {
  run_detail "$1" || return 1
  sed 's/.*"Run":{//' "$work/run.body" | grep -o '"Number":[0-9]*' | head -1 | cut -d: -f2
}

ship_release() { write_site "$1" "$2" && push_and_wait "$3"; }

remember_release() {
  local number
  number=$(run_number "$current_run") || return 1
  releases_of web || return 1
  remembered=$(grep -o "\"ReleaseId\":\"$number-[A-Za-z0-9._-]*\"" "$work/releases-web.body" | head -1 | cut -d'"' -f4)
  [[ -n $remembered ]] && return 0
  why "no release of run number $number: $(head -c 600 "$work/releases-web.body")"
  return 1
}

keep_first() { remember_release && first_release=$remembered; }

keep_third() { remember_release && third_release=$remembered; }

manual_rollback() {
  local name=$1
  rollback_to=$2
  ok_status "$work/rollback" "$(api "$work/rollback" POST "$(target_path "$name")/rollback" \
    "$(printf '{"ReleaseId":"%s","CurrentFactor":{"Method":"password","Password":"%s"}}' "$rollback_to" "$password")" "${session[@]}")" 202 || return 1
  rollback_run=$(json_field "$work/rollback.body" RunId)
  [[ -n $rollback_run ]]
}

make_ingest_token() {
  ok_status "$work/ingest" "$(api "$work/ingest" POST "$(project_path)/ingest-tokens" \
    "$(printf '{"Name":"m6 demo","RefPatterns":[{"Kind":"branch","Pattern":"main"}],"ExpiresInDays":1,"AcknowledgeProtectedReach":true,"CurrentFactor":{"Method":"password","Password":"%s"}}' "$password")" \
    "${session[@]}")" 201 || return 1
  ingest_token=$(json_field "$work/ingest.body" Value)
  [[ $ingest_token =~ ^dhow_ing_[A-Za-z0-9_-]{43}$ ]] || return 1
  needle "ingest token value" "$ingest_token"
}

build_cli() {
  docker compose exec -T tools go build -trimpath -o "$work/server/dhow" ./apps/cli/cmd/dhow </dev/null || return 1
  chmod 0755 "$work/server/dhow"
}

push_artifact() {
  local version=$1 dir="$work/server/ingest/site"
  install -d -m 0755 "$dir"
  printf '%s\n' "$version" >"$dir/index.html"
  printf 'ok\n' >"$dir/healthz"
  chmod -R a+rX "$work/server/ingest"
  head_sha=$(git_in_tools rev-parse HEAD | tr -d '\r')
  [[ $head_sha =~ ^[0-9a-f]{40}$ ]] || return 1
  printf '%s' "$ingest_token" | docker compose exec -T server /run/dhow-demo/dhow artifact push --server http://localhost:8080 \
    --token-stdin --name shipped --ref main --sha "$head_sha" --provenance Provider=m6-demo --format json -- /run/dhow-demo/ingest \
    >"$work/push.json" || { cat -- "$work/push.json" >&2; return 1; }
  grep -qF '"Triggered":true' "$work/push.json" || { why "the push: $(head -c 400 "$work/push.json")"; return 1; }
  current_run=$(json_field "$work/push.json" RunId)
  [[ -n $current_run ]]
}

push_artifact_is_idempotent() {
  local before
  before=$(run_count)
  printf '%s' "$ingest_token" | docker compose exec -T server /run/dhow-demo/dhow artifact push --server http://localhost:8080 \
    --token-stdin --name shipped --ref main --sha "$head_sha" --provenance Provider=m6-demo --format json -- /run/dhow-demo/ingest \
    >"$work/push-again.json" || return 1
  [[ $(run_count) == "$before" ]]
}

fresh_host_key() {
  docker compose up --detach --force-recreate --no-deps sshd >/dev/null 2>&1 || return 1
  for _ in $(seq 120); do
    docker compose exec -T sshd nc -z 127.0.0.1 22 </dev/null 2>/dev/null && break
    sleep 1
  done
  authorize_key sshd "$(cat -- "$work/target-key.pub")"
  [[ $(docker compose exec -T sshd ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub </dev/null | cut -d' ' -f2) != "$pinned_fingerprint" ]]
}

nothing_written() { ! docker compose exec -T sshd test -e "$target_dir/releases" </dev/null 2>/dev/null; }
