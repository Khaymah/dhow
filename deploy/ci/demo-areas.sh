#!/usr/bin/env bash
set -euo pipefail

event=${1:-}
if [[ $event != pull_request ]]; then
  printf 'demos=true\ne2e=true\ndeploy=true\ningest=true\n'
  exit 0
fi

demos=false
e2e=false
deploy=false
ingest=false
while IFS= read -r file; do
  [[ -n $file ]] || continue
  case $file in
    docs/* | Plan/* | .claude/* | *.md) ;;
    internal/deployrelease/* | internal/deployplugin/*) deploy=true ;;
    internal/ingestpush/*) ingest=true ;;
    apps/web/* | packages/openapi/* | deploy/e2e/*) e2e=true ;;
    apps/server/* | internal/* | packages/proto/* | deploy/dev/* | deploy/dhow-git/* | deploy/m4-demo/* | go.mod | go.sum | \
      apps/runner/* | .github/workflows/demo.yml | deploy/ci/demo-areas.sh)
      demos=true
      e2e=true
      deploy=true
      ingest=true
      ;;
    apps/plugins/* | deploy/e2e-targets/*) deploy=true ;;
    apps/cli/* | apps/github-action/* | deploy/ingest-e2e/*) ingest=true ;;
    deploy/m3-demo/* | deploy/m5-demo/*) demos=true ;;
  esac
done < <(git diff --name-only HEAD^1 HEAD)
printf 'demos=%s\ne2e=%s\ndeploy=%s\ningest=%s\n' "$demos" "$e2e" "$deploy" "$ingest"
