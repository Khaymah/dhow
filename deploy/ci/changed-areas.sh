#!/usr/bin/env bash
set -euo pipefail

event=${1:-}
if [[ $event != pull_request ]]; then
  printf 'code=true\nweb=true\nactions=true\n'
  exit 0
fi

code=false
web=false
actions=false
while IFS= read -r file; do
  [[ -n $file ]] || continue
  case $file in
    docs/* | Plan/* | .claude/* | .github/ISSUE_TEMPLATE/* | .github/*.md | *.md) ;;
    apps/web/*) web=true ;;
    packages/openapi/*)
      web=true
      code=true
      ;;
    .github/workflows/ci.yml | .github/workflows/codeql.yml | deploy/ci/*)
      web=true
      code=true
      actions=true
      ;;
    .github/workflows/* | .github/actions/* | apps/github-action/*)
      code=true
      actions=true
      ;;
    *) code=true ;;
  esac
done < <(git diff --name-only HEAD^1 HEAD)
printf 'code=%s\nweb=%s\nactions=%s\n' "$code" "$web" "$actions"
