#!/usr/bin/env bash
# Merge a PR with a pinned head, retrying only transient GitHub failures.
# scripts/merge-pr.sh <owner/repo> <number> <full-head-sha> <squash|merge>
set -uo pipefail
repo=${1:?repository is required}; number=${2:?pull request number is required}
expected_sha=${3:?full head SHA is required}; mode=${4:?merge mode is required}
case "$mode" in squash|merge) ;; *) echo "merge-pr: invalid merge mode: $mode" >&2; exit 2 ;; esac
[[ "$expected_sha" =~ ^[[:xdigit:]]{40}$ ]] || { echo "merge-pr: expected a full 40-character SHA" >&2; exit 2; }
state_and_head=$(gh pr view "$number" -R "$repo" --json state,headRefOid --jq '[.state, .headRefOid] | @tsv' 2>/dev/null) || { echo "merge-pr: unable to read PR state before merge" >&2; exit 1; }
read -r state actual_sha <<<"$state_and_head"
[ "$actual_sha" = "$expected_sha" ] || { echo "merge-pr: head SHA mismatch (expected $expected_sha, got ${actual_sha:-unreadable})" >&2; exit 1; }
[ "$state" = OPEN ] || { echo "merge-pr: PR is not open: ${state:-unreadable}" >&2; exit 1; }
attempt=1
while [ "$attempt" -le 3 ]; do
  error_file=$(mktemp)
  if gh pr merge "$number" -R "$repo" "--$mode" --match-head-commit "$expected_sha" > /dev/null 2>"$error_file"; then
    rm -f "$error_file"
    final_state=$(gh pr view "$number" -R "$repo" --json state --jq .state 2>/dev/null) || { echo "merge-pr: unable to verify PR state after merge" >&2; exit 1; }
    [ "$final_state" = MERGED ] && { echo "merged $repo#$number ($mode)"; exit 0; }
    echo "merge-pr: merge command succeeded but PR state is ${final_state:-unreadable}" >&2; exit 1
  fi
  error=$(<"$error_file"); rm -f "$error_file"
  if ! grep -Eiq '(^|[^0-9])5[0-9]{2}([^0-9]|$)|timed out|timeout|connection (reset|refused|closed)|network|temporarily unavailable|bad gateway|gateway timeout' <<<"$error"; then
    echo "merge-pr: merge failed without retry: ${error:-unknown error}" >&2; exit 1
  fi
  [ "$attempt" -eq 3 ] && { echo "merge-pr: transient merge failure after 3 attempts: ${error:-unknown error}" >&2; exit 1; }
  sleep "$attempt"; attempt=$((attempt + 1))
done
