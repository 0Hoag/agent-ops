#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
mutation=${1:?usage: $0 auth|validation|pricing|repository-list|repository-aggregate|wave-list|wave-aggregate|issue-list|issue-aggregate|noop}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
git -C "$ROOT" archive HEAD | tar -x -C "$tmp"
cp "$ROOT/scripts/agent-run-acceptance.sh" "$tmp/scripts/"
case "$mutation" in
 auth) perl -0pi -e 's/ctx.GetHeader\("X-Agent-Run-Token"\) != c.token/ctx.GetHeader("X-Agent-Run-Token") == c.token/' "$tmp/api/modules/agentrun/controller.go"; target="$tmp/api/modules/agentrun/controller.go"; pattern='authenticated POST: expected HTTP 201' ;;
 validation) perl -0pi -e 's/if in\.InputTokens < 0 \|\| in\.CachedInputTokens < 0 \|\| in\.CacheWriteTokens < 0 \|\| in\.OutputTokens < 0 \|\| in\.ReasoningTokens < 0/if false/' "$tmp/api/modules/agentrun/controller.go"; target="$tmp/api/modules/agentrun/controller.go"; pattern='negative token validation: expected HTTP 400' ;;
 pricing) perl -0pi -e 's#/1e6#/1e5#g' "$tmp/api/modules/agentrun/repository.go"; target="$tmp/api/modules/agentrun/repository.go"; pattern='AGGREGATE FILTER ASSERTION:' ;;
 repository-list|repository-aggregate) perl -0pi -e 's/q = q\.Where\("repository_id = \?", f\.RepositoryID\)/\/\/ mutated repository predicate/' "$tmp/api/modules/agentrun/repository.go"; target="$tmp/api/modules/agentrun/repository.go"; [[ $mutation == repository-list ]] && pattern='LIST FILTER ASSERTION:' || pattern='AGGREGATE FILTER ASSERTION:' ;;
 wave-list|wave-aggregate) perl -0pi -e 's/q = q\.Where\("wave = \?", f\.Wave\)/\/\/ mutated wave predicate/' "$tmp/api/modules/agentrun/repository.go"; target="$tmp/api/modules/agentrun/repository.go"; [[ $mutation == wave-list ]] && pattern='LIST FILTER ASSERTION:' || pattern='AGGREGATE FILTER ASSERTION:' ;;
 issue-list|issue-aggregate) perl -0pi -e 's/q = q\.Where\("issue_numbers @> \?::jsonb", fmt\.Sprintf\("\[%d\]", f\.IssueNumber\)\)/\/\/ mutated issue predicate/' "$tmp/api/modules/agentrun/repository.go"; target="$tmp/api/modules/agentrun/repository.go"; [[ $mutation == issue-list ]] && pattern='LIST FILTER ASSERTION:' || pattern='AGGREGATE FILTER ASSERTION:' ;;
 noop) perl -0pi -e 's/THIS_PATTERN_DOES_NOT_EXIST/changed/' "$tmp/api/modules/agentrun/repository.go"; target="$tmp/api/modules/agentrun/repository.go"; pattern='';;
 *) echo "unknown mutation: $mutation" >&2; exit 2;;
esac
if [[ $(shasum "$target" | awk '{print $1}') == $(git -C "$tmp" show "HEAD:${target#"$tmp"/}" | shasum | awk '{print $1}') ]]; then echo "control $mutation FAIL: mutation did not change target" >&2; exit 1; fi
if [[ $mutation == noop ]]; then echo 'control noop passed: unchanged target rejected before execution'; exit 0; fi
set +e
output=$(cd "$tmp" && bash scripts/agent-run-acceptance.sh 2>&1)
nested_status=$?
set -e
printf '%s\n' "$output"
[[ $nested_status -ne 0 ]] || { echo "control $mutation FAIL: nested acceptance exited 0" >&2; exit 1; }
if ! NEEDLE="$pattern" perl -ne 'exit 0 if /^acceptance: FAIL: / && index($_, $ENV{NEEDLE}) >= 0; END { exit 1 }' <<<"$output"; then echo "control $mutation FAIL: missing anchored assertion diagnostic '$pattern' (nested exit=$nested_status)" >&2; exit 1; fi
