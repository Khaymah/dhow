#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf 'usage: %s\n' "$0" >&2
  printf 'Deploys real releases to real sshd containers in Docker with the dhow-deploy-ssh plugin: a healthy deploy, a failed\n' >&2
  printf 'health check that rolls back, keep_releases, a wrong host key, an unsafe archive, hooks and shared paths; and on a\n' >&2
  printf 'group of three servers: rolling deploys in batches, a failure in the middle that rolls back every switched server,\n' >&2
  printf 'a failed rollback, an unreachable server, a wrong host key on one server and a cancel during the rollout.\n' >&2
  printf 'Then it deploys Docker Compose projects with the dhow-deploy-compose plugin to two Docker in Docker hosts that have a\n' >&2
  printf 'registry on localhost:5000: pinned images, an update, a failed health check that rolls back to the images of the previous\n' >&2
  printf 'release, files the policy refuses before the server is touched, a missing image, and on two hosts a second one that\n' >&2
  printf 'fails in the second batch so that the first rolls back. The hosts are privileged containers; set DHOW_E2E_SKIP_COMPOSE=1\n' >&2
  printf 'on a machine that does not allow that.\n' >&2
  printf 'Then it replays the reports the plugins wrote into the server (needs DHOW_TEST_DATABASE_URL).\n' >&2
  printf 'Needs Docker and Go. It does not need Podman, a runner or a server. Settings: DHOW_E2E_CA_BUNDLE (a CA file\n' >&2
  printf 'for the apk downloads behind a TLS-inspecting proxy), DHOW_E2E_KEEP=1 keeps the temporary directory.\n' >&2
  exit 2
}
[[ ${1:-} != -h && ${1:-} != --help ]] || usage

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
work=$(mktemp -d)
cleanup() {
  if [[ ${DHOW_E2E_KEEP:-0} == 1 ]]; then
    printf 'kept %s\n' "$work" >&2
  else
    rm -rf -- "$work"
  fi
}
trap cleanup EXIT

image=${DHOW_E2E_SSHD_IMAGE:-dhow-e2e-sshd:local}
secret_args=()
if [[ -n ${DHOW_E2E_CA_BUNDLE:-} ]]; then
  secret_args+=(--secret "id=ca,src=$DHOW_E2E_CA_BUNDLE")
fi

printf '==> build the sshd image\n'
DOCKER_BUILDKIT=1 docker build --tag "$image" "${secret_args[@]}" "$root/deploy/e2e-targets/sshd"

printf '==> build the plugin\n'
(cd "$root" && CGO_ENABLED=0 go build -trimpath -o "$work/dhow-deploy-ssh" ./apps/plugins/deploy-ssh/cmd/dhow-deploy-ssh)

printf '==> deploy to the sshd\n'
(
  cd "$root"
  DHOW_E2E_SSHD_IMAGE=$image DHOW_E2E_PLUGIN=$work/dhow-deploy-ssh DHOW_E2E_REPORTS=$work/reports \
    go test -count=1 -timeout 15m -v ./apps/plugins/deploy-ssh/e2e/
)

if [[ ${DHOW_E2E_SKIP_COMPOSE:-0} != 1 ]]; then
  compose_image=${DHOW_E2E_COMPOSE_HOST_IMAGE:-dhow-e2e-compose-host:local}

  printf '==> build the compose host image\n'
  DOCKER_BUILDKIT=1 docker build --tag "$compose_image" "${secret_args[@]}" "$root/deploy/e2e-targets/compose-host"

  printf '==> build the compose plugin\n'
  (cd "$root" && CGO_ENABLED=0 go build -trimpath -o "$work/dhow-deploy-compose" ./apps/plugins/deploy-compose/cmd/dhow-deploy-compose)

  printf '==> deploy to the compose hosts\n'
  (
    cd "$root"
    DHOW_E2E_COMPOSE_HOST_IMAGE=$compose_image DHOW_E2E_PLUGIN=$work/dhow-deploy-compose DHOW_E2E_REPORTS=$work/reports \
      go test -count=1 -timeout 25m -v ./apps/plugins/deploy-compose/e2e/
  )
else
  printf '==> skipped the compose hosts (DHOW_E2E_SKIP_COMPOSE=1)\n'
fi

if [[ -n ${DHOW_TEST_DATABASE_URL:-} ]]; then
  printf '==> replay the reports of the plugin into the server\n'
  (
    cd "$root"
    DHOW_E2E_REPORTS=$work/reports \
      go test -count=1 -timeout 15m -v -run '^TestTheReportsOfTheRealPluginRun' ./apps/server/internal/Modules/Pipelines/Test/E2E/
  )
else
  printf '==> skipped the replay into the server: set DHOW_TEST_DATABASE_URL to an empty PostgreSQL 18 test database\n'
fi
printf 'deploy-e2e: ok\n'
