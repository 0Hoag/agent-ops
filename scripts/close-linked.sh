#!/usr/bin/env bash
# Closes the issues a merged PR names with `Closes #n` / `Fixes #n` / `Resolves #n` (optionally `owner/repo#n`).
# GitHub only does this for PRs merged into the default branch; our work merges into reviews/development.
#
#   scripts/close-linked.sh <owner/repo> <pr-number>
set -uo pipefail

repo=$1 pr=$2
body=$(gh pr view "$pr" -R "$repo" --json body --jq .body)
grep -oEi '(closes|fixes|resolves) +([A-Za-z0-9._-]+/[A-Za-z0-9._-]+)?#[0-9]+' <<<"$body" | sort -u | while read -r _ ref; do
  target=$repo n=${ref#\#}
  if [[ $ref == */*#* ]]; then target=${ref%#*} n=${ref#*#}; fi
  state=$(gh issue view "$n" -R "$target" --json state --jq .state 2>/dev/null) || continue
  [ "$state" = OPEN ] && gh issue close "$n" -R "$target" --reason completed \
    --comment "Merged by $repo#$pr into reviews/development." > /dev/null && echo "closed $target#$n"
done
