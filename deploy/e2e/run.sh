#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  printf 'usage: %s [playwright arguments]\n' "$0" >&2
  printf 'Runs the browser tests in apps/web/E2E against a real stack in Docker: Postgres, the server with the web app\n' >&2
  printf 'built in (run make web first), Forgejo and a rootless Podman runner. Needs Docker with Compose v2, cgroup v2\n' >&2
  printf 'and pnpm. Settings: DHOW_E2E_PORT (8490), DHOW_E2E_DB_PORT (55490), DHOW_E2E_REGISTRY_PORT (55491),\n' >&2
  printf 'DHOW_E2E_SUBNET (10.231.90.0/24), DHOW_DEMO_KEEP=1 keeps the stack.\n' >&2
  exit 2
}
[[ ${1:-} != -h && ${1:-} != --help ]] || usage

export DHOW_DEMO_PORT=${DHOW_E2E_PORT:-8490}
export DHOW_DEMO_DB_PORT=${DHOW_E2E_DB_PORT:-55490}
export DHOW_DEMO_REGISTRY_PORT=${DHOW_E2E_REGISTRY_PORT:-55491}
export DHOW_DEMO_SUBNET=${DHOW_E2E_SUBNET:-10.231.90.0/24}
export DHOW_DEMO_FORGEJO_IP=${DHOW_E2E_FORGEJO_IP:-10.231.90.10}
export COMPOSE_PROJECT_NAME=${COMPOSE_PROJECT_NAME:-dhow-e2e}
token_name='e2e'

# shellcheck source=deploy/m4-demo/lib.sh
. "$(dirname -- "${BASH_SOURCE[0]}")/../m4-demo/lib.sh"
# shellcheck source=deploy/m4-demo/project.sh
. "$demo_dir/project.sh"

web_built() {
  [[ -f $root/apps/server/internal/App/WebDist/index.html ]] || {
    why "apps/server/internal/App/WebDist has no web build: run make web first"
    return 1
  }
}

write_pipeline() {
  install -d -m 0755 "$work/repo"
  printf '# E2E app\n' >"$work/repo/README.md"
  sed "s|@IMAGE@|$image|" >"$work/repo/.dhow.yml" <<'YAML'
version: 1
name: E2E
on:
  push:
    branches: [main, "feature/*"]
jobs:
  deploy:
    image: @IMAGE@
    environment: production
    steps:
      - name: Deploy
        run: echo "deploying"
        env:
          DEPLOY_TOKEN: ${{ secrets.DEPLOY_TOKEN }}
YAML
  chmod -R a+rX "$work/repo"
}

first_push() {
  write_pipeline
  git_in_tools init --quiet -b main && git_in_tools remote add origin "http://$forgejo_user@git.dhow-demo.test:3000/demo/app.git" || return 1
  push "Add the pipeline"
}

browser_tests() {
  DHOW_E2E_SECRET_VALUE=$secret_value DHOW_E2E_URL=$public_url DHOW_E2E_EMAIL=$owner_email DHOW_E2E_PASSWORD=$password DHOW_E2E_ORG_ID=$org_id \
    DHOW_E2E_PROJECT_ID=$project_id DHOW_E2E_PUSH="$root/deploy/e2e/push.sh" \
    pnpm -C "$root/apps/web" exec playwright test "$@"
}

prerequisites
command -v pnpm >/dev/null 2>&1 || { printf 'pnpm is required\n' >&2; exit 2; }
web_built || exit 2
prepare_work
trap teardown EXIT
clean_start
: >"$work/needles"
secret_value="e2e-$(od -An -tx1 -N16 /dev/urandom | tr -d ' \n')"
needle "environment secret" "$secret_value"
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
need "the pipeline reaches Forgejo before the project is linked" first_push
need "a project is made with the API token" project
need "the repository is linked and its host key is pinned after a fingerprint check" link
need "Forgejo has the read-only deploy key and the webhook" forgejo_hooks
say "The browser tests"
check "the browser tests pass" browser_tests "$@"
say "The credential leak sweep"
check "no credential and no secret value is stored in clear anywhere" bash "$demo_dir/leak-sweep.sh"
summary
((failed == 0))
