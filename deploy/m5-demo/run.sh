#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  printf 'usage: %s\n' "$0" >&2
  printf 'Runs the M5 demo in Docker: Forgejo, Postgres, the server and a rootless Podman runner, an environment that allows\n' >&2
  printf 'only main and needs a reviewer with an authenticator code, secrets in all three scopes, a skipped push, a push\n' >&2
  printf 'that waits for approval, a refused approval, an approved deploy, a rotation of the keyring in the middle, a second\n' >&2
  printf 'approved deploy and a sweep for leaked secrets. Needs Docker with Compose v2 and cgroup v2. Settings:\n' >&2
  printf 'DHOW_M5_PORT (8500), DHOW_M5_DB_PORT (55500), DHOW_M5_REGISTRY_PORT (55501), DHOW_M5_SUBNET (10.231.100.0/24),\n' >&2
  printf 'DHOW_DEMO_KEEP=1 keeps the stack.\n' >&2
  exit 2
}
[[ ${1:-} != -h && ${1:-} != --help ]] || usage
(($# == 0)) || usage

export DHOW_DEMO_PORT=${DHOW_M5_PORT:-8500}
export DHOW_DEMO_DB_PORT=${DHOW_M5_DB_PORT:-55500}
export DHOW_DEMO_REGISTRY_PORT=${DHOW_M5_REGISTRY_PORT:-55501}
export DHOW_DEMO_SUBNET=${DHOW_M5_SUBNET:-10.231.100.0/24}
export DHOW_DEMO_FORGEJO_IP=${DHOW_M5_FORGEJO_IP:-10.231.100.10}
export COMPOSE_PROJECT_NAME=${COMPOSE_PROJECT_NAME:-dhow-m5}
token_name='m5 demo'
sealed_environment=production
sealed_creates_environment=0
m5_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

# shellcheck source=deploy/m4-demo/lib.sh
. "$m5_dir/../m4-demo/lib.sh"
# shellcheck source=deploy/m4-demo/project.sh
. "$demo_dir/project.sh"
# shellcheck source=deploy/m5-demo/lib.sh
. "$m5_dir/lib.sh"

write_pipeline() {
  install -d -m 0755 "$work/repo"
  printf '# M5 demo app\n' >"$work/repo/README.md"
  sed "s|@IMAGE@|$image|" >"$work/repo/.dhow.yml" <<'YAML'
version: 1
name: M5 demo
on:
  push:
    branches: [main, "feature/*"]
  manual: {}
jobs:
  deploy:
    image: @IMAGE@
    environment: production
    steps:
      - name: Deploy with a secret of each scope
        run: |
          mkdir -p out
          echo "deployed" > out/report.txt
          echo "dhow-demo deploy secrets=${#DEPLOY_TOKEN},${#ORG_VALUE},${#PROJECT_VALUE}"
          case "$DEPLOY_TOKEN$ORG_VALUE$PROJECT_VALUE" in demo-*demo-*demo-*) echo "dhow-demo deploy prefix=ok" ;; esac
        env:
          DEPLOY_TOKEN: ${{ secrets.DEMO_environment_SECRET }}
          ORG_VALUE: ${{ secrets.DEMO_organization_SECRET }}
          PROJECT_VALUE: ${{ secrets.DEMO_project_SECRET }}
    artifacts:
      - name: report
        paths: [out]
YAML
  chmod -R a+rX "$work/repo"
}

first_push() {
  write_pipeline
  git_in_tools init --quiet -b main && git_in_tools remote add origin "http://$forgejo_user@git.dhow-demo.test:3000/demo/app.git" || return 1
  push "Add the pipeline"
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
need "the pipeline reaches Forgejo before the project is linked" first_push
need "a project is made with the API token" project

say "A reviewer with an authenticator app, and the production environment"
need "the owner invites a second user as a Developer and the invitation is accepted" invite_reviewer
need "the reviewer turns on TOTP and signs in with the second step" reviewer_ready
need "production allows only main and names the reviewer" production_environment
check "production keeps one allowed ref and no pull requests" environment_rules
need "a secret in each scope (production holds the environment one) and a registry password are stored" sealed_values
need "the repository is linked and its host key is pinned after a fingerprint check" link
need "Forgejo has the read-only deploy key and the webhook" forgejo_hooks
need "the owner turns on TOTP for the sensitive steps" owner_ready

say "A push to a branch the environment does not allow"
need "the push to feature/demo reaches Forgejo" push_ref feature/demo
need "the webhook starts run 1" wait_runs 1
check "the deploy job of run 1 is skipped with ENVIRONMENT_REF_NOT_ALLOWED and never asks for approval" job_skipped_by_ref_rule "${run_ids[0]}"

say "A push to main waits for approval"
need "the push to main reaches Forgejo" push_ref main
need "the webhook starts run 2" wait_runs 2
need "the deploy job of run 2 waits at the approval gate (the run is running, not a fork approval)" wait_gate "${run_ids[1]}" approval
need "run 2 has one pending approval request" pending_approval "${run_ids[1]}"
check "run 2 itself is running, so this is a deploy approval and not a fork approval" run_is_running "${run_ids[1]}"
check "the owner, who is no reviewer, is refused with REVIEWER_NOT_ELIGIBLE" refused_with owner "${run_ids[1]}" 422 REVIEWER_NOT_ELIGIBLE
need "the reviewer approves with an authenticator code" approved_by_reviewer "${run_ids[1]}"
need "run 2 succeeds after the approval" run_ends "${run_ids[1]}"
check "the job got the secrets of the environment, the project and the organization" deployed_with_secrets "${run_ids[1]}"
check "run 2 kept the artifact" artifact_kept "${run_ids[1]}"
check "the deployment of run 2 is recorded as succeeded" deployment_recorded "${run_ids[1]}"

say "Self-approval is refused"
need "the owner is added to the reviewers" owner_joins_reviewers
need "the owner starts a manual run on main" manual_run
need "the deploy job of the manual run waits at the approval gate" wait_gate "$manual_run_id" approval
need "the manual run has one pending approval request" pending_approval "$manual_run_id"
check "the owner, who started the run, is refused with SELF_REVIEW_NOT_ALLOWED" refused_with owner "$manual_run_id" 403 SELF_REVIEW_NOT_ALLOWED
need "the owner cancels the manual run" cancel_run "$manual_run_id"

say "Rotating the keyring in the middle: generate, restart, activate, re-wrap"
need "keys generate adds KEK version 2 to the keyring file" kek_added
need "the server restarts with the new keyring and the runner comes back" server_restarted
need "KEK version 2 is registered as pending" kek_pending
need "keys activate makes version 2 active" kek_activated
need "every data key is re-wrapped with version 2 and version 1 is retired" data_keys_rewrapped

say "A second push to main after the rotation"
need "the push to main reaches Forgejo" push_ref main
need "the webhook starts run 4" wait_runs 4
need "the deploy job of run 4 waits at the approval gate" wait_gate "${run_ids[3]}" approval
need "run 4 has one pending approval request" pending_approval "${run_ids[3]}"
need "the reviewer approves with an authenticator code" approved_by_reviewer "${run_ids[3]}"
need "run 4 succeeds after the approval" run_ends "${run_ids[3]}"
check "the same secrets reach the job after the rotation" deployed_with_secrets "${run_ids[3]}"

say "The credential leak sweep"
check "no credential and no secret value is stored in clear anywhere" bash "$demo_dir/leak-sweep.sh"

summary
((failed == 0))
