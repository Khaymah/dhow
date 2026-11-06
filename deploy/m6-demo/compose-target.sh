#!/usr/bin/env bash
# shellcheck disable=SC2034,SC2154

publish_app() {
  local tag=dhow-m6-app:demo registry=localhost:5000/demo-app:demo out
  printf 'FROM %s\nRUN mkdir /www && echo m6 > /www/index.html\nEXPOSE 8080\nCMD ["httpd", "-f", "-p", "8080", "-h", "/www"]\n' "$image" |
    docker build --quiet --tag "$tag" - >/dev/null || return 1
  docker save "$tag" | docker compose exec -T compose-host docker load >/dev/null || return 1
  docker compose exec -T compose-host docker tag "$tag" "$registry" </dev/null || return 1
  out=$(docker compose exec -T compose-host docker push "$registry" </dev/null) || return 1
  app_ref="localhost:5000/demo-app@$(grep -o 'sha256:[0-9a-f]\{64\}' <<<"$out" | head -1)"
  [[ $app_ref == *@sha256:* ]]
}

write_compose() {
  local probe='["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/index.html"]'
  if [[ ${1:-healthy} == broken ]]; then probe='["CMD", "false"]'; fi
  install -d -m 0755 "$work/repo"
  cat >"$work/repo/compose.yaml" <<YAML
services:
  web:
    image: $app_ref
    healthcheck:
      test: $probe
      interval: 1s
      timeout: 2s
      retries: 2
      start_period: 1s
YAML
  readable_repo
}

write_compose_pipeline() {
  sed -e "s|@IMAGE@|$image|" -e "s|@PLUGIN@|$compose_plugin|" >"$work/repo/.dhow.yml" <<'YAML'
version: 1
name: M6 demo compose
on:
  push:
    branches: [main]
jobs:
  stack:
    image: @IMAGE@
    steps:
      - name: Collect the Compose file
        run: |
          mkdir stack
          cp compose.yaml stack/compose.yaml
    artifacts:
      - name: stack
        paths: [stack/]
  deploy:
    image: @IMAGE@
    needs: [stack]
    environment: production
    steps:
      - uses: @PLUGIN@
        with:
          artifact: stack
          target: shop
          wait_timeout: 60s
YAML
  readable_repo
}

compose_runs() {
  docker compose exec -T compose-host docker ps --filter "label=com.docker.compose.project=$compose_project" --format '{{.Image}} {{.Status}}' </dev/null
}

compose_serves() {
  local want=$1 got
  for _ in $(seq 90); do
    got=$(compose_runs | tr -d '\r') || true
    if grep -qF -- "$app_ref" <<<"$got" && grep -qF -- "$want" <<<"$got"; then return 0; fi
    sleep 1
  done
  why "the project $compose_project runs: $got"
  return 1
}

compose_target() {
  create_target shop compose "$compose_ip" "\"Settings\":{\"Compose\":{\"ProjectName\":\"$compose_project\",\"ComposeFile\":\"stack/compose.yaml\"}},"
}

compose_key() {
  generate_key shop || return 1
  authorize_key compose-host "$(cat -- "$work/genkey-shop.pub")"
}

ship_stack() { write_compose "$1" && write_compose_pipeline && push_and_wait "$2"; }
