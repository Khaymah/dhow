#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  printf 'usage: %s\n' "$0" >&2
  printf 'Runs the M7 demo in Docker, on top of the M6 demo: Forgejo, Postgres, the server, a rootless Podman runner and a real\n' >&2
  printf 'sshd, plus deploy/e2e-targets/notify-receiver standing in for Slack, Discord, Telegram and a signed webhook over TLS\n' >&2
  printf '(the service hosts resolve to it inside the demo network and the server trusts its CA). It makes a channel of each kind\n' >&2
  printf 'and refuses addresses that must not be reached, deploys a release, deploys one that fails and rolls back, and checks that\n' >&2
  printf 'every channel got the message in its own language with the mentions made harmless and the webhook signature valid, that a\n' >&2
  printf 'release that works sends nothing, that the organization event stream carried the deploys, that a log of 50,000 lines\n' >&2
  printf 'downloads whole, that a server killed in the middle of a send delivers the message after it starts again, and that a dead\n' >&2
  printf 'delivery is sent after a retry. It ends with the leak sweep, widened to the Slack address, the Telegram token and the\n' >&2
  printf 'webhook secret, with the API answers, the event stream and the inbox swept as well. Needs Docker with Compose v2 and cgroup v2.\n' >&2
  printf 'Settings: DHOW_M7_PORT (8710), DHOW_M7_DB_PORT (55710), DHOW_M7_REGISTRY_PORT (55711), DHOW_M7_SUBNET (10.231.130.0/24),\n' >&2
  printf 'DHOW_DEMO_KEEP=1 keeps the stack.\n' >&2
  exit 2
}
[[ ${1:-} != -h && ${1:-} != --help ]] || usage
(($# == 0)) || usage

export DHOW_DEMO_PORT=${DHOW_M7_PORT:-8710}
export DHOW_DEMO_DB_PORT=${DHOW_M7_DB_PORT:-55710}
export DHOW_DEMO_REGISTRY_PORT=${DHOW_M7_REGISTRY_PORT:-55711}
export DHOW_DEMO_SUBNET=${DHOW_M7_SUBNET:-10.231.130.0/24}
export DHOW_DEMO_FORGEJO_IP=${DHOW_M7_FORGEJO_IP:-10.231.130.10}
export DHOW_M6_SSHD_IP=${DHOW_M7_SSHD_IP:-10.231.130.20}
export DHOW_M6_COMPOSE_HOST_IP=${DHOW_M7_COMPOSE_HOST_IP:-10.231.130.30}
export DHOW_M7_NOTIFY_IP=${DHOW_M7_NOTIFY_IP:-10.231.130.40}
export COMPOSE_PROJECT_NAME=${COMPOSE_PROJECT_NAME:-dhow-m7}
token_name='m7 demo'
sealed_environment=production
sealed_creates_environment=1
sshd_ip=$DHOW_M6_SSHD_IP
m7_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
export DHOW_DEMO_COMPOSE_EXTRA="$m7_dir/../m6-demo/compose.yaml:$m7_dir/compose.yaml"

# shellcheck source=deploy/m4-demo/lib.sh
. "$m7_dir/../m4-demo/lib.sh"
# shellcheck source=deploy/m4-demo/project.sh
. "$demo_dir/project.sh"
# shellcheck source=deploy/m5-demo/lib.sh
. "$m7_dir/../m5-demo/lib.sh"
# shellcheck source=deploy/m6-demo/lib.sh
. "$m7_dir/../m6-demo/lib.sh"
# shellcheck source=deploy/m6-demo/pipeline.sh
. "$m7_dir/../m6-demo/pipeline.sh"
# shellcheck source=deploy/m7-demo/notify.sh
. "$m7_dir/notify.sh"
# shellcheck source=deploy/m7-demo/scenario.sh
. "$m7_dir/scenario.sh"

plugin_images() {
  plugin_image dhow-deploy-ssh apps/plugins/deploy-ssh/Dockerfile ssh_plugin || return 1
  export DHOW_DEMO_DEPLOY_SSH_IMAGE=$ssh_plugin
}

hosts_up() {
  for _ in $(seq 240); do
    docker compose exec -T sshd test -s /etc/ssh/ssh_host_ed25519_key.pub </dev/null 2>/dev/null &&
      docker compose exec -T sshd nc -z 127.0.0.1 22 </dev/null 2>/dev/null && return 0
    sleep 1
  done
  why "sshd never became ready"
  return 1
}

leak_sweep() {
  DHOW_DEMO_EXPECT_TARGETS=1 DHOW_DEMO_EXPECT_NOTIFY=1 DHOW_DEMO_SWEEP_EXTRA="$captures" bash "$demo_dir/leak-sweep.sh"
}

finish() {
  local code=$?
  stop_events
  (exit "$code")
  teardown
}

prerequisites
prepare_work
trap finish EXIT
clean_start
: >"$work/needles"
captures=$work/captures
install -d -m 0755 "$work/keys-ssh"
: >"$work/keys-ssh/authorized_keys"
chmod 0644 "$work/keys-ssh/authorized_keys"

say "The notify receiver: Slack, Discord, Telegram and a signed webhook over TLS, with a CA the server trusts"
need "the receiver is built, runs behind the hosts of the four services and answers over TLS with its own CA" notify_receiver
say "The checkout image, the SSH deploy plugin and a registry for them"
need "the dhow-git image is built and pushed to a TLS registry" checkout_image
need "the SSH plugin is built from its Dockerfile and pushed under its official name" plugin_images
say "Postgres, the server, Forgejo, the Podman runner and a real sshd (the first build takes a few minutes)"
docker compose up --detach --build db server forgejo runner tools sshd >/dev/null
need "the server starts and prints its setup link" server_ready
need "the owner and the organization are set up" setup_owner
needle "session cookie" "${session_cookie#*=}"
needle "CSRF token" "$csrf_token"
needle "owner password" "$password"
need "the Podman runner registers and comes online" runner_up
needle "runner registration token" "$registration_token"
need "an API token is made" api_token
need "Forgejo has a user and an empty repository" forgejo_repo
need "the sshd is up" hosts_up
need "the pipeline reaches Forgejo before the project is linked" first_push
need "a project is made, its name carrying a channel mention on purpose" project
need "the repository is linked and its host key is pinned after a fingerprint check" link
need "Forgejo has the read-only deploy key and the webhook" forgejo_hooks
need "a secret in each scope, a registry password and the production environment are stored" sealed_values

say "The SSH target"
need "an access key is made and its private half is stored as a secret of the environment" own_key
need "the private key is stored as the secret $key_secret and no response shows it back" store_target_key
need "the public key is allowed on the sshd" authorize_key sshd "$(cat -- "$work/target-key.pub")"
need "the target web is made with the server's address and the key secret" create_target web ssh "$sshd_ip" "\"KeySecretName\":\"$key_secret\","
need "the host key of the sshd is read from the server itself" pending_host_key web sshd
need "the right fingerprint is confirmed" confirm_host_key web
need "the target is ready" target_ready web

say "A channel of each kind, and the addresses that must be refused"
need "the secrets of the channels are made up" make_values
need "a webhook, a Slack, a Discord, a Telegram and an in-app channel are made, and the webhook secret is shown once" channels
check "no list of channels shows an address, a token or a secret" secrets_shown_once
check "the cloud metadata address, loopback, a private address, plain http and a foreign Slack host are all refused" addresses_refused
check "a test message reaches each channel in its language, with Discord parsing no mention and the webhook signature valid" test_messages_arrive
need "the event stream of the organization opens with the session" start_events
check "an API token is refused by the event stream" events_refuse_token

say "A release that works sends nothing"
need "the push of release one starts a run" ship_release 'release one' healthy 'Release one'
need "the run succeeds" run_ends "$current_run"
check "the sshd serves release one" served /current/site/index.html 'release one'
need "the target records release one as succeeded" keep_first
check "the event stream carried the release" events_saw release.succeeded
first_number=$(run_number "$current_run")
check "no channel got a message for run $first_number" not_notified "$first_number"

say "A deploy that fails and rolls back by itself"
need "release two, broken on purpose, fails with DEPLOY_ROLLED_BACK" failed_deploy
check "the sshd still serves release one" served /current/site/index.html 'release one'
check "the event stream carried the rollback" events_saw release.rolled_back
check "Slack and Telegram got it in Arabic, Discord in English, and the webhook got it signed" failed_run_notified "$failed_number"
check "the mention in the project name stayed text in Slack, Discord and Telegram, and no live mention was sent" mentions_neutralised
check "every delivery is settled and none is dead" deliveries_settled 120
check "no delivery is dead" no_dead_deliveries
check "the inbox of the owner holds an unread notification made of keys and plain values" inbox_filled

say "A release that works after the failure"
need "release three deploys" good_deploy
check "the event stream carried it" events_saw release.succeeded
check "no channel got a message for run $ok_number" not_notified "$ok_number"

say "A big log"
need "a log of $log_lines lines with escape codes, markup and a bidi override is made, and the raw download has every line in order" big_log
check "the raw download is plain text for download, and keeps the markup, the escape code and the override as they are" raw_log_is_safe_text

say "The server killed in the middle of a send"
need "the receiver holds the webhook request, the server is killed while it waits and starts again" killed_while_sending
check "the message reaches the receiver once or twice with the same key, signed, and the delivery ends sent" sent_once_or_again_with_the_same_key

say "A delivery that dies and is sent again"
need "the receiver refuses with 400, the delivery goes dead with its code, and Retry sends it" dead_then_retried
check "every delivery is settled" deliveries_settled 120

say "The credential leak sweep, widened to channel secrets"
need "the answers, the event stream and the inbox are kept for the sweep" capture_all
check "the doctor answer has no detail" doctor_is_plain
check "no credential, no secret value and no channel secret is stored in clear anywhere" leak_sweep

summary
((failed == 0))
