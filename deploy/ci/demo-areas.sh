#!/usr/bin/env bash
set -euo pipefail

event=${1:-}
if [[ $event != pull_request ]]; then
  printf 'demos=true\ne2e=true\n'
  exit 0
fi

demos=false
e2e=false
while IFS= read -r file; do
  [[ -n $file ]] || continue
  case $file in
    docs/* | Plan/* | .claude/* | *.md) ;;
    apps/web/* | packages/openapi/* | deploy/e2e/*) e2e=true ;;
    apps/server/* | internal/* | packages/proto/* | deploy/dev/* | deploy/dhow-git/* | deploy/m4-demo/* | go.mod | go.sum | \
      .github/workflows/demo.yml | deploy/ci/demo-areas.sh)
      demos=true
      e2e=true
      ;;
    apps/runner/* | deploy/m3-demo/*) demos=true ;;
  esac
done < <(git diff --name-only HEAD^1 HEAD)
printf 'demos=%s\ne2e=%s\n' "$demos" "$e2e"
