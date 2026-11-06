#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  printf 'usage: %s\n' "$0" >&2
  printf 'Runs the M6 demo in Docker: Forgejo, Postgres, the server and a rootless Podman runner, a real sshd and a Docker host\n' >&2
  printf 'as deploy targets. It pins the host keys after a fingerprint check, deploys a release, deploys one that fails its health\n' >&2
  printf 'check and rolls back by itself, rolls back by hand, deploys an artifact uploaded with dhow artifact push, refuses a\n' >&2
  printf 'server whose host key changed, deploys and rolls back a Docker Compose project, and sweeps for leaked secrets. Needs\n' >&2
  printf 'Docker with Compose v2 and cgroup v2. Settings: DHOW_M6_PORT (8700), DHOW_M6_DB_PORT (55700), DHOW_M6_REGISTRY_PORT\n' >&2
  printf '(55701), DHOW_M6_SUBNET (10.231.120.0/24), DHOW_DEMO_KEEP=1 keeps the stack.\n' >&2
  exit 2
}
[[ ${1:-} != -h && ${1:-} != --help ]] || usage
(($# == 0)) || usage

export DHOW_DEMO_PORT=${DHOW_M6_PORT:-8700}
export DHOW_DEMO_DB_PORT=${DHOW_M6_DB_PORT:-55700}
export DHOW_DEMO_REGISTRY_PORT=${DHOW_M6_REGISTRY_PORT:-55701}
export DHOW_DEMO_SUBNET=${DHOW_M6_SUBNET:-10.231.120.0/24}
export DHOW_DEMO_FORGEJO_IP=${DHOW_M6_FORGEJO_IP:-10.231.120.10}
export DHOW_M6_SSHD_IP=${DHOW_M6_SSHD_IP:-10.231.120.20}
export DHOW_M6_COMPOSE_HOST_IP=${DHOW_M6_COMPOSE_HOST_IP:-10.231.120.30}
export COMPOSE_PROJECT_NAME=${COMPOSE_PROJECT_NAME:-dhow-m6}
token_name='m6 demo'
sealed_environment=production
sealed_creates_environment=1
sshd_ip=$DHOW_M6_SSHD_IP
compose_ip=$DHOW_M6_COMPOSE_HOST_IP
m6_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
export DHOW_DEMO_COMPOSE_EXTRA="$m6_dir/compose.yaml"

# shellcheck source=deploy/m4-demo/lib.sh
. "$m6_dir/../m4-demo/lib.sh"
# shellcheck source=deploy/m4-demo/project.sh
. "$demo_dir/project.sh"
# shellcheck source=deploy/m5-demo/lib.sh
. "$m6_dir/../m5-demo/lib.sh"
# shellcheck source=deploy/m6-demo/lib.sh
. "$m6_dir/lib.sh"
# shellcheck source=deploy/m6-demo/pipeline.sh
. "$m6_dir/pipeline.sh"
# shellcheck source=deploy/m6-demo/compose-target.sh
. "$m6_dir/compose-target.sh"

leak_sweep() { bash "$demo_dir/leak-sweep.sh"; }

prerequisites
prepare_work
trap teardown EXIT
clean_start
: >"$work/needles"
install -d -m 0755 "$work/keys-ssh" "$work/keys-compose"
: >"$work/keys-ssh/authorized_keys"
: >"$work/keys-compose/authorized_keys"
chmod 0644 "$work/keys-ssh/authorized_keys" "$work/keys-compose/authorized_keys"

say "The checkout image, the two official deploy plugins and a registry for them"
need "the dhow-git image is built and pushed to a TLS registry" checkout_image
need "the SSH and Compose plugins are built from their Dockerfiles and pushed under their official names" plugin_images
say "Postgres, the server, Forgejo, the Podman runner, a real sshd and a Docker host (the first build takes a few minutes)"
docker compose up --detach --build db server forgejo runner tools sshd compose-host >/dev/null
need "the server starts and prints its setup link" server_ready
need "the owner and the organization are set up" setup_owner
needle "session cookie" "${session_cookie#*=}"
needle "CSRF token" "$csrf_token"
needle "owner password" "$password"
need "the Podman runner registers and comes online" runner_up
needle "runner registration token" "$registration_token"
need "an API token is made" api_token
need "Forgejo has a user and an empty repository" forgejo_repo
need "the sshd and the Docker host are up" hosts_up
need "the pipeline reaches Forgejo before the project is linked" first_push
need "a project is made with the API token" project
need "the repository is linked and its host key is pinned after a fingerprint check" link
need "Forgejo has the read-only deploy key and the webhook" forgejo_hooks
need "a secret in each scope, a registry password and the production environment are stored" sealed_values

say "The SSH target: its key, its pinned host key and a check"
need "an access key is made and its private half is stored as a secret of the environment" own_key
need "the private key is stored as the secret $key_secret and no response shows it back" store_target_key
need "the public key is allowed on the sshd" authorize_key sshd "$(cat -- "$work/target-key.pub")"
need "the target web is made with the server's address and the key secret" create_target web ssh "$sshd_ip" "\"KeySecretName\":\"$key_secret\","
need "the host key of the sshd is read from the server itself and its fingerprint is the one the sshd shows" pending_host_key web sshd
check "a wrong fingerprint is refused with HOST_KEY_FINGERPRINT_MISMATCH and pins nothing" wrong_fingerprint_refused web
need "the right fingerprint is confirmed" confirm_host_key web
need "the target is ready" target_ready web
need "a check of the target finds the server ready" target_checked web

say "A release, a release that fails its health check, and another release"
need "the push of release one reaches Forgejo and starts a run" ship_release 'release one' healthy 'Release one'
need "the run succeeds" run_ends "$current_run"
check "the sshd serves release one" served /current/site/index.html 'release one'
need "the target records release one as succeeded" keep_first
check "the list of releases says the group is on one release" release_has web '"State":"succeeded"' '"Partial":false'
need "the push of release two, which fails its health check, starts a run" ship_release 'release two' broken 'Release two, broken on purpose'
check "the job fails by itself with DEPLOY_ROLLED_BACK" run_failed_with "$current_run" DEPLOY_ROLLED_BACK
check "current still points to release one and the sshd still serves it" current_is "$first_release"
check "the sshd serves release one after the automatic rollback" served /current/site/index.html 'release one'
check "the list of releases shows the rollback" release_has web '"State":"rolled_back"'
need "the push of release three starts a run" ship_release 'release three' healthy 'Release three'
need "the run succeeds" run_ends "$current_run"
check "the sshd serves release three" served /current/site/index.html 'release three'
need "release three is in the list" keep_third

say "A rollback by hand"
need "the owner rolls back to release one with the second step" manual_rollback web "$first_release"
need "the rollback run succeeds" run_ends "$rollback_run"
check "current points to release one again" current_is "$first_release"
check "the sshd serves release one after the rollback by hand" served /current/site/index.html 'release one'
check "the list records the rollback with the release it went back to" release_has web '"Kind":"rollback"' "\"RestoredReleaseId\":\"$first_release\""
check "release three is still a release that can be rolled back to" release_has web "\"ReleaseId\":\"$third_release\""

say "An artifact from an outside CI"
need "an ingest token for main is made, and it is shown once" make_ingest_token
need "the dhow CLI is built" build_cli
need "dhow artifact push uploads the folder and starts a run" push_artifact 'release from outside CI'
need "the run of the artifact succeeds" run_ends "$current_run"
check "the sshd serves the artifact that was pushed" served /current/site/index.html 'release from outside CI'
check "pushing the same artifact again starts no second run" push_artifact_is_idempotent

say "A server whose host key changed"
need "the sshd is made again with a new host key" fresh_host_key
need "the push of release five starts a run" ship_release 'release five' healthy 'Release five'
check "the job fails with HOST_KEY_MISMATCH" run_failed_with "$current_run" HOST_KEY_MISMATCH
check "nothing was written on the new server" nothing_written

say "A Docker Compose target"
need "the application image is published to the registry of the Docker host" publish_app
need "the target shop is made" compose_target
need "Dhow makes the access key of the target and shows only its public half" compose_key
need "the host key of the Docker host is read and its fingerprint is the one the host shows" pending_host_key shop compose-host
need "the fingerprint is confirmed" confirm_host_key shop
need "the target is ready" target_ready shop
need "the push of the Compose project starts a run" ship_stack healthy 'Deploy the stack'
need "the run succeeds" run_ends "$current_run"
check "the service runs the pinned image and is healthy" compose_serves '(healthy)'
need "the push of a project whose service never becomes healthy starts a run" ship_stack broken 'Deploy a stack that is not healthy'
check "the job fails by itself with DEPLOY_ROLLED_BACK" run_failed_with "$current_run" DEPLOY_ROLLED_BACK
check "the service runs the pinned image and is healthy again" compose_serves '(healthy)'

say "The credential leak sweep, with the target key and the ingest token added"
check "no credential and no secret value is stored in clear anywhere" leak_sweep

summary
((failed == 0))
