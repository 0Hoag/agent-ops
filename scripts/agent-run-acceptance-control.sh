#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
mutation=${1:?usage: $0 auth|validation|pricing|repository-list|repository-aggregate|wave-list|wave-aggregate|issue-list|issue-aggregate|noop|timestamp|cleanup-failure}
tmp=$(mktemp -d /private/tmp/agent-run-control.XXXXXX)
trap 'rm -rf "$tmp"' EXIT
control_cache=${AGENT_RUN_CONTROL_CACHE:-/private/tmp/agent-run-control-cache}
mkdir -p "$control_cache/mod" "$control_cache/build"
export GOMODCACHE="$control_cache/mod" GOCACHE="$control_cache/build"
git -C "$ROOT" archive HEAD | tar -x -C "$tmp"
cp "$ROOT/scripts/agent-run-acceptance.sh" "$tmp/scripts/"
git -C "$tmp" init -q
git -C "$tmp" add .
git -C "$tmp" -c user.name=control -c user.email=control@example.invalid commit -qm baseline
case "$mutation" in
 auth) perl -0pi -e 's/ctx.GetHeader\("X-Agent-Run-Token"\) != c.token/ctx.GetHeader("X-Agent-Run-Token") == c.token/' "$tmp/api/modules/agentrun/controller.go"; target="$tmp/api/modules/agentrun/controller.go"; pattern='authenticated POST: expected HTTP 201' ;;
 validation) perl -0pi -e 's/if in\.InputTokens < 0 \|\| in\.CachedInputTokens < 0 \|\| in\.CacheWriteTokens < 0 \|\| in\.OutputTokens < 0 \|\| in\.ReasoningTokens < 0/if false/' "$tmp/api/modules/agentrun/controller.go"; target="$tmp/api/modules/agentrun/controller.go"; pattern='negative token validation: expected HTTP 400' ;;
 pricing) perl -0pi -e 's#/1e6#/1e5#g' "$tmp/api/modules/agentrun/repository.go"; target="$tmp/api/modules/agentrun/repository.go"; pattern='AGGREGATE FILTER ASSERTION:' ;;
 repository-list|repository-aggregate) perl -0pi -e 's/q = q\.Where\("repository_id = \?", f\.RepositoryID\)/\/\/ mutated repository predicate/' "$tmp/api/modules/agentrun/repository.go"; target="$tmp/api/modules/agentrun/repository.go"; [[ $mutation == repository-list ]] && pattern='LIST FILTER ASSERTION:' || pattern='AGGREGATE FILTER ASSERTION:' ;;
 wave-list|wave-aggregate) perl -0pi -e 's/q = q\.Where\("wave = \?", f\.Wave\)/\/\/ mutated wave predicate/' "$tmp/api/modules/agentrun/repository.go"; target="$tmp/api/modules/agentrun/repository.go"; [[ $mutation == wave-list ]] && pattern='LIST FILTER ASSERTION:' || pattern='AGGREGATE FILTER ASSERTION:' ;;
 issue-list|issue-aggregate) perl -0pi -e 's/q = q\.Where\("issue_numbers @> \?::jsonb", fmt\.Sprintf\("\[%d\]", f\.IssueNumber\)\)/\/\/ mutated issue predicate/' "$tmp/api/modules/agentrun/repository.go"; target="$tmp/api/modules/agentrun/repository.go"; [[ $mutation == issue-list ]] && pattern='LIST FILTER ASSERTION:' || pattern='AGGREGATE FILTER ASSERTION:' ;;
 noop) perl -0pi -e 's/THIS_PATTERN_DOES_NOT_EXIST/changed/' "$tmp/api/modules/agentrun/repository.go"; target="$tmp/api/modules/agentrun/repository.go"; pattern='';;
 timestamp) target="$tmp/scripts/agent-run-acceptance.sh"; pattern='exact SQL normalization/timestamp mismatch:'; export AGENT_RUN_ACCEPTANCE_CORRUPT_TIMESTAMP=1 ;;
 cleanup-failure) perl -0pi -e 's/\[\[ "\$actual_ids" == "\$expected_ids" \]\] \|\| die "LIST FILTER ASSERTION:/[[ "$actual_ids" != "$expected_ids" ]] || die "LIST FILTER ASSERTION:/' "$tmp/scripts/agent-run-acceptance.sh"; target="$tmp/scripts/agent-run-acceptance.sh"; pattern='LIST FILTER ASSERTION:'; export AGENT_RUN_ACCEPTANCE_INJECT_CLEANUP_FAILURE=1 ;;
 *) echo "unknown mutation: $mutation" >&2; exit 2;;
esac
[[ $mutation == *-aggregate ]] && export AGENT_RUN_ACCEPTANCE_SKIP_LIST=1
if [[ $mutation != timestamp && $mutation != cleanup-failure && $(shasum "$target" | awk '{print $1}') == $(git -C "$tmp" show "HEAD:${target#"$tmp"/}" | shasum | awk '{print $1}') ]]; then
  if [[ $mutation == noop ]]; then echo 'control noop passed: wrapper rejected no-op mutation; nested acceptance not invoked'; exit 0; fi
  echo "control $mutation FAIL: mutation did not change target" >&2
  exit 1
fi
set +e
output=$(cd "$tmp" && bash scripts/agent-run-acceptance.sh 2>&1)
nested_status=$?
set -e
printf '%s\n' "$output"
[[ $nested_status -ne 0 ]] || { echo "control $mutation FAIL: nested acceptance exited 0" >&2; exit 1; }
if ! printf '%s\n' "$output" | NEEDLE="$pattern" perl -ne 'if (/^acceptance: FAIL: / && index($_, $ENV{NEEDLE}) >= 0) { $found=1 } END { exit($found ? 0 : 1) }'; then echo "control $mutation FAIL: missing anchored assertion diagnostic '$pattern' (nested exit=$nested_status)" >&2; exit 1; fi
if printf '%s\n' "$output" | perl -ne 'if (/^acceptance: cleanup FAILED:/) {$bad=1} END {exit($bad ? 0 : 1)}'; then
 if [[ $mutation == cleanup-failure ]]; then
  echo "control $mutation FAIL: intended assertion matched; nested exit=$nested_status; injected cleanup failure rejected"
  exit 1
 fi
 echo "control $mutation FAIL: cleanup failure evidence present (nested exit=$nested_status)" >&2; exit 1
fi
if [[ $mutation != cleanup-failure ]] && ! printf '%s\n' "$output" | perl -ne '$pid=1 if /^acceptance: cleanup verified: API PID absent$/; $listener=1 if /^acceptance: cleanup verified: no API listener on \d+$/; $fixtures=1 if /^acceptance: cleanup verified: no owned containers or volumes \(/; END {exit($pid && $listener && $fixtures ? 0 : 1)}'; then echo "control $mutation FAIL: missing distinct verified cleanup evidence" >&2; exit 1; fi
echo "control $mutation passed: nested acceptance exit=$nested_status at '$pattern'; verified cleanup; wrapper exit=0"
