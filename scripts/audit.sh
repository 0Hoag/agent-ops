#!/usr/bin/env bash
# Tracker audit, no tokens. Prints one line per finding and exits 1; prints nothing and exits 0 when healthy.
#   scripts/audit.sh
# Checks (open issues skip epics and the status board): no status label, two status labels, no priority,
# blocked without a `Status:` comment as the newest one, closed but still labelled as moving; open PRs that name no
# issue (promotion PRs reviews/development -> main are exempt).
set -uo pipefail
REPOS=${REPOS:-"0Hoag/agent-ops 0Hoag/agent-ops-client"}
found=0
say() { [ -n "$2" ] && { echo "$1: $2"; found=1; }; return 0; }

for r in $REPOS; do
  short=${r#*/}
  issues=$(gh issue list -R "$r" --state open --limit 500 --json number,labels,comments) || exit 2
  work='map(select(([.labels[].name] | any(. == "type:epic" or . == "status-board")) | not))'
  say "$short no status" "$(jq -r "$work | map(select([.labels[].name | startswith(\"status:\")] | any | not) | \"#\\(.number)\") | join(\" \")" <<<"$issues")"
  say "$short two statuses" "$(jq -r "$work | map(select([.labels[].name | select(startswith(\"status:\"))] | length > 1) | \"#\\(.number)\") | join(\" \")" <<<"$issues")"
  say "$short no priority" "$(jq -r "$work | map(select([.labels[].name | startswith(\"priority:\")] | any | not) | \"#\\(.number)\") | join(\" \")" <<<"$issues")"
  say "$short blocked without a Status: note" "$(jq -r "$work | map(select(([.labels[].name] | index(\"status:blocked\")) and ((.comments | last | .body // \"\") | startswith(\"Status:\") | not)) | \"#\\(.number)\") | join(\" \")" <<<"$issues")"
  for s in "in progress" review blocked todo; do
    say "$short closed but status:$s" "$(gh issue list -R "$r" --state closed --label "status:$s" --limit 200 --json number --jq 'map("#\(.number)") | join(" ")')"
  done
  say "$short PR names no issue" "$(gh pr list -R "$r" --json number,body,headRefName,baseRefName --jq 'map(select((.baseRefName == "main" and .headRefName == "reviews/development") | not) | select((.body // "") | test("(?i)(closes|fixes|resolves) +([A-Za-z0-9._-]+/[A-Za-z0-9._-]+)?#[0-9]+") | not) | "!\(.number)") | join(" ")')"
done
exit $found
