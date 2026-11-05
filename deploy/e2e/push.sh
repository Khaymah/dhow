#!/usr/bin/env bash
set -euo pipefail
umask 077

branch=${1:-}
[[ $branch =~ ^[a-z0-9][a-z0-9/_-]{0,62}$ && $branch != *//* ]] || {
  printf 'usage: %s <branch>, a branch name of lowercase letters, digits, / _ and -\n' "$0" >&2
  exit 2
}
[[ -n ${DHOW_DEMO_WORK:-} && -d $DHOW_DEMO_WORK/repo ]] || {
  printf 'run this from deploy/e2e/run.sh: DHOW_DEMO_WORK has no repository\n' >&2
  exit 2
}

# shellcheck source=deploy/m4-demo/lib.sh
. "$(dirname -- "${BASH_SOURCE[0]}")/../m4-demo/lib.sh"
# shellcheck source=deploy/m4-demo/project.sh
. "$demo_dir/project.sh"
work=$DHOW_DEMO_WORK

git_in_tools checkout --quiet main
if [[ $branch != main ]]; then
  git_in_tools checkout --quiet -B "$branch"
fi
printf 'A change on %s at %s.\n' "$branch" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"$work/repo/README.md"
git_in_tools add --all
git_in_tools commit --quiet -m "Change $branch"
git_in_tools push --quiet origin "HEAD:refs/heads/$branch" 2>&1
git_in_tools checkout --quiet main
