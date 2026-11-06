#!/usr/bin/env bash
# shellcheck disable=SC2154

write_pipeline() {
  install -d -m 0755 "$work/repo"
  printf '# M6 demo app\n' >"$work/repo/README.md"
  sed -e "s|@IMAGE@|$image|g" -e "s|@PLUGIN@|$ssh_plugin|g" >"$work/repo/.dhow.yml" <<'YAML'
version: 1
name: M6 demo
on:
  push:
    branches: [main]
  artifact:
    names: [shipped]
    refs: [main]
jobs:
  build:
    if: ${{ git.event == 'push' }}
    image: @IMAGE@
    steps:
      - name: Build the site
        run: |
          mkdir site
          cp index.html site/index.html
          if [ ! -e unhealthy ]; then echo ok > site/healthz; fi
    artifacts:
      - name: web
        paths: [site/]
  deploy:
    if: ${{ git.event == 'push' }}
    image: @IMAGE@
    needs: [build]
    environment: production
    steps:
      - uses: @PLUGIN@
        with:
          artifact: web
          target: web
          health_check_url: http://{host}:8080/current/site/healthz
          health_check_retries: "4"
          health_check_interval: 1s
          health_check_timeout: 3s
          health_check_stable: "2"
  ship:
    if: ${{ git.event == 'artifact' }}
    image: @IMAGE@
    ingested: [shipped]
    environment: production
    steps:
      - uses: @PLUGIN@
        with:
          artifact: shipped
          target: web
          health_check_url: http://{host}:8080/current/site/healthz
          health_check_retries: "4"
          health_check_interval: 1s
          health_check_timeout: 3s
          health_check_stable: "2"
YAML
  chmod -R a+rX "$work/repo"
}

first_push() {
  write_site v0 healthy
  write_pipeline
  git_in_tools init --quiet -b main && git_in_tools remote add origin "http://$forgejo_user@git.dhow-demo.test:3000/demo/app.git" || return 1
  push "Add the pipeline"
}
