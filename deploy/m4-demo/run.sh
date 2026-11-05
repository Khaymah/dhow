#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  printf 'usage: %s\n' "$0" >&2
  printf 'Runs the M4 demo in Docker: Forgejo, Postgres, the server and a rootless Podman runner, a pipeline with a\n' >&2
  printf 'DAG, a matrix, an artifact and a cache, a forged and a replayed webhook, and a sweep for leaked credentials.\n' >&2
  printf 'Needs Docker with Compose v2 and cgroup v2. Settings: DHOW_DEMO_PORT (8480), DHOW_DEMO_DB_PORT (55480),\n' >&2
  printf 'DHOW_DEMO_REGISTRY_PORT (55481), DHOW_DEMO_SUBNET (10.231.80.0/24), DHOW_DEMO_KEEP=1 keeps the stack.\n' >&2
  exit 2
}
(($# == 0)) || usage

# shellcheck source=deploy/m4-demo/lib.sh
. "$(dirname -- "${BASH_SOURCE[0]}")/lib.sh"
# shellcheck source=deploy/m4-demo/project.sh
. "$demo_dir/project.sh"

write_pipeline() {
  install -d -m 0755 "$work/repo"
  printf 'demo 1\n' >"$work/repo/VERSION"
  printf 'left-pad 1.3.0\n' >"$work/repo/deps.lock"
  printf '# Demo app\n' >"$work/repo/README.md"
  sed "s|@IMAGE@|$image|" >"$work/repo/.dhow.yml" <<'YAML'
version: 1
name: M4 demo
on:
  push:
    branches: [main]
jobs:
  build:
    image: @IMAGE@
    cache:
      key: ${{ "deps-" + hash_files("deps.lock") }}
      paths: [.deps]
    steps:
      - name: Restore or fill the cache, then build
        run: |
          if [ -f .deps/ready ]; then echo "dhow-demo cache=hit"; else echo "dhow-demo cache=miss"; mkdir -p .deps; cat deps.lock > .deps/ready; fi
          mkdir -p dist
          echo "app $(cat VERSION)" > dist/app.txt
    artifacts:
      - name: app
        paths: [dist]
  test:
    needs: [build]
    image: @IMAGE@
    strategy:
      matrix:
        shard: [a, b]
    env:
      SHARD: ${{ matrix.shard }}
    steps:
      - run: echo "dhow-demo shard=$SHARD artifact=$(cat dist/app.txt)"
  report:
    needs: [test]
    image: @IMAGE@
    steps:
      - run: echo "dhow-demo every shard passed"
YAML
  chmod -R a+rX "$work/repo"
}

first_push() {
  write_pipeline
  git_in_tools init --quiet -b main && git_in_tools remote add origin "http://$forgejo_user@git.dhow-demo.test:3000/demo/app.git" || return 1
  push "Add the pipeline"
}

run_logs() {
  local id=$1 job
  api "$work/run" GET "/api/v1/organizations/$org_id/projects/$project_id/runs/$id" '' --header "$(bearer)" >/dev/null
  : >"$work/logs.txt"
  while IFS= read -r job; do
    api "$work/job-log" GET "/api/v1/organizations/$org_id/jobs/$job/logs" '' "${session[@]}" >/dev/null
    cat -- "$work/job-log.body" >>"$work/logs.txt"
  done < <(grep -o '"CurrentJobId":"[^"]*"' "$work/run.body" | cut -d'"' -f4)
}

saw() { grep -qF -- "$1" "$work/logs.txt"; }

dag_and_matrix() {
  run_logs "$1"
  [[ $(grep -o '"JobId":"test"' "$work/run.body" | wc -l) == 2 ]] && saw 'dhow-demo shard=a artifact=app demo 1' &&
    saw 'dhow-demo shard=b artifact=app demo 1' && saw 'dhow-demo every shard passed' && return 0
  why "the jobs of the run: $(grep -o '"Key":"[^"]*"\|"State":"[^"]*"' "$work/run.body" | tr '\n' ' '); the demo lines seen: $(grep -o 'dhow-demo [^"\\]*' "$work/logs.txt" | sort -u | tr '\n' ';')"
  return 1
}

artifact_kept() {
  api "$work/artifacts" GET "/api/v1/organizations/$org_id/projects/$project_id/runs/$1/artifacts" '' "${session[@]}" >/dev/null
  grep -qF '"Name":"app"' "$work/artifacts.body" && return 0
  why "the artifacts of the run: $(head -c 400 "$work/artifacts.body")"
  return 1
}

cache_was() {
  run_logs "$1"
  saw "dhow-demo cache=$2" && return 0
  why "the cache lines seen: $(grep -o 'dhow[-:] [^"\\]*cache[^"\\]*' "$work/logs.txt" | sort -u | tr '\n' ';')"
  return 1
}

hmac_hex() { docker compose exec -T tools go run ./deploy/m4-demo/hmac "$1"; }

key_file() {
  (umask 077 && printf '%s' "$2" >"$work/$1")
  printf '%s' "$work/$1"
}

deliver() {
  local prefix=$1 body=$2 signature=$3 delivery=$4
  docker compose exec -T tools curl --silent --show-error --max-time 30 --output "$prefix.body" --write-out '%{http_code}' \
    --header 'Content-Type: application/json' --header 'X-Forgejo-Event: push' --header "X-Forgejo-Delivery: $delivery" \
    --header "X-Forgejo-Signature: $signature" --data "$body" -- "$hook_url" </dev/null
}

forged_refused() {
  local before body status
  before=$(run_count)
  body=$(printf '{"ref":"refs/heads/main","after":"%s","pusher":{"login":"mallory"}}' "$(printf '%040d' 7)")
  status=$(deliver "$work/forged" "$body" "$(printf '%s' "$body" | hmac_hex "$(key_file forged-key 'not the webhook secret of this repository')")" forged-1)
  sleep 3
  printf '    forged delivery answered %s\n' "$status"
  [[ $status == 401 && $(run_count) == "$before" ]]
}

replay_ignored() {
  local before sha body signature first second
  before=$(run_count)
  sha=$(git_in_tools rev-parse HEAD | tr -d '\r')
  body=$(printf '{"ref":"refs/heads/main","after":"%s","pusher":{"login":"demo"}}' "$sha")
  signature=$(printf '%s' "$body" | hmac_hex "$(key_file hook-key "$hook_secret")")
  first=$(deliver "$work/replay-1" "$body" "$signature" replay-1)
  second=$(deliver "$work/replay-2" "$body" "$signature" replay-1)
  sleep 3
  printf '    the delivery answered %s, the replay %s (%s)\n' "$first" "$second" "$(json_field "$work/replay-2.body" Message)"
  [[ $first == 202 && $second == 200 && $(json_field "$work/replay-2.body" Message) == sources.webhook.duplicate &&
    $(run_count) == "$before" ]]
}

second_push() {
  printf 'A second commit; deps.lock did not change, so the cache key is the same.\n' >>"$work/repo/README.md"
  push "Change the README"
}

prerequisites
prepare_work
trap teardown EXIT
clean_start
: >"$work/needles"

say "The checkout image and a registry for it"
need "the dhow-git image is built and pushed to a TLS registry" checkout_image
say "Postgres, the server, Forgejo and the Podman runner (the first build takes a few minutes)"
docker compose up --detach --build db server forgejo runner tools >/dev/null
need "the server starts and prints its setup link" server_ready
need "the owner and the organization are set up" setup_owner
needle "session cookie" "${session_cookie#*=}"
needle "CSRF token" "$csrf_token"
needle "owner password" "$password"
need "the Podman runner registers and comes online" runner_up
needle "runner registration token" "$registration_token"
need "an API token is made" api_token
need "Forgejo has a user and an empty repository" forgejo_repo
need "a project is made with the API token" project
need "the repository is linked and its host key is pinned after a fingerprint check" link
need "Forgejo has the read-only deploy key and the webhook" forgejo_hooks

say "First push: a webhook from Forgejo starts run 1"
need "the push reaches Forgejo" first_push
need "the webhook starts run 1" wait_runs 1
need "run 1 succeeds (checkout, DAG, matrix, artifact)" run_ends "${run_ids[0]}"
check "run 1 has the build, two test shards and the report, and both shards read the artifact" dag_and_matrix "${run_ids[0]}"
check "run 1 kept the artifact" artifact_kept "${run_ids[0]}"
check "run 1 missed the cache and filled it" cache_was "${run_ids[0]}" miss

say "A forged webhook"
check "a delivery signed with the wrong secret gets 401 and starts no run" forged_refused

say "Second push: run 2 restores the cache"
need "the second push reaches Forgejo" second_push
need "the webhook starts run 2" wait_runs 2
need "run 2 succeeds" run_ends "${run_ids[1]}"
check "run 2 restored the cache of run 1" cache_was "${run_ids[1]}" hit

say "A replayed webhook"
check "the same delivery sent twice is a duplicate and starts no run" replay_ignored

say "The credential leak sweep"
check "no credential is stored in clear anywhere" bash "$demo_dir/leak-sweep.sh"

summary
((failed == 0))
