#!/usr/bin/env bash
# Closes the issues a merged PR names with `Closes #n` / `Fixes #n` / `Resolves #n` (optionally `owner/repo#n`).
# GitHub only does this for PRs merged into the default branch; our work merges into reviews/development.
#
#   scripts/close-linked.sh <owner/repo> <pr-number>
set -uo pipefail

repo=$1 pr=$2
state_and_base=$(gh pr view "$pr" -R "$repo" --json state,baseRefName --jq '[.state, .baseRefName] | @tsv' 2>/dev/null) || {
  echo "close-linked: unable to read PR state" >&2
  exit 1
}
read -r state base <<<"$state_and_base"
if [ "$state" != MERGED ]; then
  echo "close-linked: PR $repo#$pr is not MERGED (state: ${state:-unreadable}); no issues closed" >&2
  exit 1
fi
body=$(gh pr view "$pr" -R "$repo" --json body --jq .body 2>/dev/null) || {
  echo "close-linked: unable to read PR body" >&2
  exit 1
}
grep -oEi '(closes|fixes|resolves) +([A-Za-z0-9._-]+/[A-Za-z0-9._-]+)?#[0-9]+' <<<"$body" | sort -u | while read -r _ ref; do
  target=$repo n=${ref#\#}
  if [[ $ref == */*#* ]]; then target=${ref%#*} n=${ref#*#}; fi
  state=$(gh issue view "$n" -R "$target" --json state --jq .state 2>/dev/null) || continue
  [ "$state" = OPEN ] || continue
  gh issue close "$n" -R "$target" --reason completed --comment "Merged by $repo#$pr into $base." > /dev/null || continue
  gh issue edit "$n" -R "$target" --remove-label "status:todo,status:in progress,status:review,status:blocked" > /dev/null 2>&1
  echo "closed $target#$n"
done
