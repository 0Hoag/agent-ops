#!/usr/bin/env bash
# Branch hygiene on GitHub, no tokens. Read-only unless --delete. Silent and exit 0 when clean.
#   scripts/branches.sh           # report
#   scripts/branches.sh --delete  # delete the STALE ones only
# Per remote branch (never touched: main, reviews/development, protected branches, the base of any PR in any state,
# a branch with an open PR, a branch a running worker names):
#   STALE   a merged PR has exactly this tip as its head: everything on it is in the base (covers squash merges,
#           where ancestry cannot be used). Deleted by --delete, and only if the tip is still the one checked.
#   LATE    a merged PR exists but the tip moved after it: commits pushed after the merge never reached the base.
#           Work at risk; never deleted. Open a new PR from it.
#   NO-PR   no PR was ever opened from it: lost work waiting to happen. Push-and-PR or delete on purpose.
#   CLOSED  only closed, unmerged PRs: the owner decides.
set -uo pipefail
REPOS=${REPOS:-"0Hoag/agent-ops 0Hoag/agent-ops-client"}
delete=0; [ "${1:-}" = "--delete" ] && delete=1
out=$(mktemp)

for r in $REPOS; do
  prs=$(gh pr list -R "$r" --state all --limit 500 --json number,state,headRefName,headRefOid,baseRefName) || exit 2
  protected=$(gh api "repos/$r/branches?protected=true" --paginate --jq '.[].name')
  bases=$(jq -r '[.[].baseRefName] | unique | .[]' <<<"$prs")
  gh api "repos/$r/branches" --paginate --jq '.[] | "\(.name) \(.commit.sha)"' | while read -r b tip; do
    [ "$b" = main ] || [ "$b" = reviews/development ] && continue
    grep -qxF -- "$b" <<<"$protected" && continue
    grep -qxF -- "$b" <<<"$bases" && continue
    [ "$(jq -r --arg b "$b" '[.[] | select(.headRefName == $b and .state == "OPEN")] | length' <<<"$prs")" -gt 0 ] && continue
    pgrep -f -- "$b" > /dev/null && continue
    merged_tip=$(jq -r --arg b "$b" --arg t "$tip" '[.[] | select(.headRefName == $b and .state == "MERGED" and .headRefOid == $t)] | length' <<<"$prs")
    merged_any=$(jq -r --arg b "$b" '[.[] | select(.headRefName == $b and .state == "MERGED")] | length' <<<"$prs")
    any=$(jq -r --arg b "$b" '[.[] | select(.headRefName == $b)] | length' <<<"$prs")
    if [ "$merged_tip" -gt 0 ]; then
      echo "STALE  $r $b"
      [ $delete = 1 ] && [ "$(gh api "repos/$r/branches/${b//\//%2F}" --jq .commit.sha)" = "$tip" ] \
        && gh api -X DELETE "repos/$r/git/refs/heads/$b" > /dev/null && echo "       deleted"
    elif [ "$merged_any" -gt 0 ]; then echo "LATE   $r $b (commits after the merged PR)"
    elif [ "$any" -eq 0 ]; then echo "NO-PR  $r $b"
    else echo "CLOSED $r $b (closed, unmerged PR)"; fi
  done
done >> "$out"
cat "$out"
if [ -s "$out" ]; then rc=1; else rc=0; fi
rm -f "$out"
exit $rc
