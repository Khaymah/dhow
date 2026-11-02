#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=deploy/dev/stack/runner-env.sh
. /src/deploy/dev/stack/runner-env.sh

if (($# > 0)); then
  die "dhow-dev-register takes no arguments; it reads the registration token from stdin"
fi
if ((EUID != 0)); then
  die "run dhow-dev-register through docker compose exec, which runs it as root before it switches to the $user user"
fi
if [[ ! -x $bin ]]; then
  die "the runner is still building; watch docker compose -f deploy/dev/compose.yaml logs -f $service and try again when it waits for a token"
fi
if [[ -e $state/identity ]]; then
  die "this runner is already registered. To register it again, reset it as docs/en/development.md describes"
fi

token=
if [[ -t 0 ]]; then
  printf 'Paste the registration token (dhow_rrt_...) and press Enter: ' >&2
  IFS= read -rs token || true
  printf '\n' >&2
else
  IFS= read -r token || true
fi
if [[ -z $token ]]; then
  die "no token was given; create one on the Runners page and paste it"
fi

printf '%s\n' "$token" | (as_runner "$bin" register --dev --url "$server_url" --token-file - --name "$name") | sed '/^Start it with:/d'
say "the runner starts within a few seconds; it shows as online on the Runners page"
