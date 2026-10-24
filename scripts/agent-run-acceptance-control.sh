#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
mutation=${1:?usage: $0 auth|validation|pricing|repository-list|repository-aggregate|wave-list|wave-aggregate|issue-list|issue-aggregate|noop|timestamp|cleanup-failure|cleanup-remove|cleanup-omit-pid|cleanup-omit-listener|cleanup-omit-fixtures|cleanup-missing-injection|cleanup-extra-failure}
tmp=$(mktemp -d /private/tmp/agent-run-control.XXXXXX)
trap 'rm -rf "$tmp"' EXIT
export SMOKE_ARTIFACT_DIR="$tmp/test-results"
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
  repository-list|repository-aggregate) python3 - "$tmp/api/modules/agentrun/repository.go" <<'PY'
import pathlib,sys
p=pathlib.Path(sys.argv[1]);s=p.read_text(); old='q = q.Where("repository_id = ?", f.RepositoryID)'
if s.count(old)!=1: raise SystemExit('repository predicate mutation target missing')
p.write_text(s.replace(old,'_ = f.RepositoryID // mutated repository predicate',1))
PY
    target="$tmp/api/modules/agentrun/repository.go"; [[ $mutation == repository-list ]] && pattern='LIST FILTER ASSERTION:' || pattern='AGGREGATE FILTER ASSERTION:' ;;
  wave-list|wave-aggregate) python3 - "$tmp/api/modules/agentrun/repository.go" <<'PY'
import pathlib,sys
p=pathlib.Path(sys.argv[1]);s=p.read_text(); old='q = q.Where("wave = ?", f.Wave)'
if s.count(old)!=1: raise SystemExit('wave predicate mutation target missing')
p.write_text(s.replace(old,'_ = f.Wave // mutated wave predicate',1))
PY
    target="$tmp/api/modules/agentrun/repository.go"; [[ $mutation == wave-list ]] && pattern='LIST FILTER ASSERTION:' || pattern='AGGREGATE FILTER ASSERTION:' ;;
  issue-list|issue-aggregate) python3 - "$tmp/api/modules/agentrun/repository.go" <<'PY'
import pathlib,sys
p=pathlib.Path(sys.argv[1]);s=p.read_text(); old='q = q.Where("issue_numbers @> ?::jsonb", fmt.Sprintf("[%d]", f.IssueNumber))'
if s.count(old)!=1: raise SystemExit('issue predicate mutation target missing')
p.write_text(s.replace(old,'_ = f.IssueNumber // mutated issue predicate',1))
PY
    target="$tmp/api/modules/agentrun/repository.go"; [[ $mutation == issue-list ]] && pattern='LIST FILTER ASSERTION:' || pattern='AGGREGATE FILTER ASSERTION:' ;;
  noop) perl -0pi -e 's/THIS_PATTERN_DOES_NOT_EXIST/changed/' "$tmp/api/modules/agentrun/repository.go"; target="$tmp/api/modules/agentrun/repository.go"; pattern='' ;;
  timestamp) target="$tmp/scripts/agent-run-acceptance.sh"; pattern='exact SQL normalization/timestamp mismatch:'; export AGENT_RUN_ACCEPTANCE_CORRUPT_TIMESTAMP=1 ;;
  cleanup-failure|cleanup-extra-failure)
    python3 - "$tmp/scripts/agent-run-acceptance.sh" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); s = p.read_text()
old = '[[ "$actual_ids" == "$expected_ids" ]] || die "LIST FILTER ASSERTION:'
new = '[[ "$actual_ids" != "$expected_ids" ]] || die "LIST FILTER ASSERTION:'
if s.count(old) != 1: raise SystemExit('expected exactly one list assertion to mutate')
p.write_text(s.replace(old, new, 1))
PY
    target="$tmp/scripts/agent-run-acceptance.sh"; pattern='LIST FILTER ASSERTION:'
    export AGENT_RUN_ACCEPTANCE_INJECT_CLEANUP_FAILURE=1
    if [[ $mutation == cleanup-extra-failure ]]; then export AGENT_RUN_ACCEPTANCE_EXTRA_CLEANUP_FAILURE=1; fi ;;
  cleanup-remove)
    target="$tmp/scripts/agent-run-acceptance.sh"; pattern=''; export AGENT_RUN_ACCEPTANCE_SKIP_COMPOSE_CLEANUP=1 AGENT_RUN_ACCEPTANCE_FORCE_CLEANUP=1 AGENT_RUN_ACCEPTANCE_API_PORT=18371 AGENT_RUN_ACCEPTANCE_PG_PORT=15432 ;;
  cleanup-omit-pid|cleanup-omit-listener|cleanup-omit-fixtures|cleanup-missing-injection)
    target="$tmp/scripts/agent-run-acceptance.sh"; pattern='exact SQL normalization/timestamp mismatch:'
    export AGENT_RUN_ACCEPTANCE_CORRUPT_TIMESTAMP=1
    if [[ $mutation == cleanup-missing-injection ]]; then
      export AGENT_RUN_ACCEPTANCE_INJECT_CLEANUP_FAILURE=1 AGENT_RUN_ACCEPTANCE_DISABLE_CLEANUP_INJECTION=1
    else
      python3 - "$tmp/scripts/agent-run-acceptance.sh" "$mutation" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); s = p.read_text()
needles = {
  'cleanup-omit-pid': 'else log "cleanup verified: API PID absent"; fi',
  'cleanup-omit-listener': 'else log "cleanup verified: no API listener on $API_PORT"; fi',
  'cleanup-omit-fixtures': 'log "cleanup verified: no owned containers or volumes ($PROJECT)"',
}
needle = needles[sys.argv[2]]
if s.count(needle) != 1: raise SystemExit(f'expected one cleanup evidence class: {needle}')
p.write_text(s.replace(needle, 'true; fi # omitted cleanup evidence' if sys.argv[2] != 'cleanup-omit-fixtures' else ': # omitted cleanup evidence', 1))
PY
    fi ;;
  *) echo "unknown mutation: $mutation" >&2; exit 2 ;;
esac
[[ $mutation == *-aggregate ]] && export AGENT_RUN_ACCEPTANCE_SKIP_LIST=1

if [[ $mutation == noop ]]; then
  echo 'control noop passed: no-op mutation was unchanged; nested acceptance not invoked'
  exit 0
fi
set +e
output=$(cd "$tmp" && bash scripts/agent-run-acceptance.sh 2>&1)
nested_status=$?
set -e
printf '%s\n' "$output"

if [[ $mutation == cleanup-remove ]]; then
  if [[ $nested_status -eq 0 ]] || ! printf '%s\n' "$output" | perl -ne 'if (/^acceptance: cleanup FAILED: owned containers remain:/) {$container=1} if (/^acceptance: cleanup FAILED: owned volumes remain:/) {$volume=1} if (/^acceptance: cleanup removal regression: deliberately retained fixtures for (agent-run-acceptance-\d+-\d+)$/) {$project=$1} if (/^acceptance: cleanup verified: no owned containers or volumes \(([^)]+)\)$/ && $project && $1 eq $project) {$absence=1} if (/^acceptance: cleanup verified: API PID absent$/) {$pid=1} if (/^acceptance: cleanup verified: no API listener on \d+$/) {$listener=1} END {exit($container && $volume && $absence && $pid && $listener ? 0 : 1)}'; then
    echo 'control cleanup-remove FAIL: deliberately retained real fixtures were not rejected' >&2; exit 1
  fi
  echo 'control cleanup-remove passed: regression rejected retained real fixtures; outer cleanup follows'
  exit 0
fi

if [[ $mutation == cleanup-omit-pid || $mutation == cleanup-omit-listener || $mutation == cleanup-omit-fixtures || $mutation == cleanup-missing-injection || $mutation == cleanup-extra-failure ]]; then
  [[ $nested_status -ne 0 ]] || { echo "control $mutation FAIL: nested acceptance exited 0" >&2; exit 1; }
  case "$mutation" in
    cleanup-omit-pid) missing='API PID absent' ;;
    cleanup-omit-listener) missing='no API listener' ;;
    cleanup-omit-fixtures) missing='no owned containers or volumes' ;;
    cleanup-missing-injection) missing='injected cleanup failure' ;;
    cleanup-extra-failure) missing='injected additional cleanup failure' ;;
  esac
  if [[ $mutation != cleanup-extra-failure && $mutation != cleanup-omit-pid && $mutation != cleanup-omit-listener && $mutation != cleanup-omit-fixtures ]] && printf '%s\n' "$output" | NEEDLE="$missing" perl -ne 'if (/^acceptance: cleanup verified:/ && index($_, $ENV{NEEDLE}) >= 0) {$found=1} if (/^acceptance: cleanup FAILED:/ && index($_, $ENV{NEEDLE}) >= 0) {$found=1} END {exit($found ? 0 : 1)}'; then
    echo "control $mutation FAIL: expected bad evidence absent in diagnostics was present" >&2; exit 1
  fi
  if [[ $mutation == cleanup-extra-failure ]]; then
    printf '%s\n' "$output" | perl -ne 'if (/^acceptance: FAIL: LIST FILTER ASSERTION:/) {$assert=1} if (/^acceptance: cleanup FAILED: injected cleanup failure$/) {$injected=1} if (/^acceptance: cleanup FAILED: injected additional cleanup failure$/) {$extra=1} END {exit($assert && $injected && $extra ? 0 : 1)}' || { echo 'control cleanup-extra-failure FAIL: intended assertion and real extra cleanup failure not observed' >&2; exit 1; }
  elif [[ $mutation == cleanup-missing-injection ]]; then
    printf '%s\n' "$output" | perl -ne 'if (/^acceptance: FAIL: exact SQL normalization\/timestamp mismatch:/) {$assert=1} if (/^acceptance: cleanup verified: API PID absent$/) {$pid=1} if (/^acceptance: cleanup verified: no API listener on \d+$/) {$listener=1} if (/^acceptance: cleanup verified: no owned containers or volumes \(/) {$fixtures=1} END {exit($assert && $pid && $listener && $fixtures ? 0 : 1)}' || { echo 'control cleanup-missing-injection FAIL: assertion or cleanup proof missing' >&2; exit 1; }
  elif [[ $mutation == cleanup-omit-pid || $mutation == cleanup-omit-listener || $mutation == cleanup-omit-fixtures ]]; then
    printf '%s\n' "$output" | MUTATION="$mutation" perl -ne 'if (/^acceptance: FAIL: exact SQL normalization\/timestamp mismatch:/) {$assert=1} if (/^acceptance: cleanup verified: API PID absent$/) {$pid=1} if (/^acceptance: cleanup verified: no API listener on \d+$/) {$listener=1} if (/^acceptance: cleanup verified: no owned containers or volumes \(/) {$fixtures=1} END {$pid=1 if $ENV{MUTATION} eq "cleanup-omit-pid"; $listener=1 if $ENV{MUTATION} eq "cleanup-omit-listener"; $fixtures=1 if $ENV{MUTATION} eq "cleanup-omit-fixtures"; exit($assert && $pid && $listener && $fixtures ? 0 : 1)}' || { echo "control $mutation FAIL: assertion or other cleanup proof missing" >&2; exit 1; }
  fi
  echo "control $mutation passed: wrapper rejected invalid evidence; nested acceptance exit=$nested_status; wrapper exit=1"
  exit 0
fi

[[ $nested_status -ne 0 ]] || { echo "control $mutation FAIL: nested acceptance exited 0" >&2; exit 1; }
if [[ -n $pattern ]] && ! printf '%s\n' "$output" | NEEDLE="$pattern" perl -ne 'if (/^acceptance: FAIL: / && index($_, $ENV{NEEDLE}) >= 0) {$found=1} END {exit($found ? 0 : 1)}'; then
  echo "control $mutation FAIL: intended diagnostic '$pattern' missing" >&2; exit 1
fi
if [[ $mutation == cleanup-failure ]]; then
  printf '%s\n' "$output" | perl -ne 'if (/^acceptance: FAIL: LIST FILTER ASSERTION:/) {$assert=1} if (/^acceptance: cleanup FAILED: injected cleanup failure$/) {$inject++} if (/^acceptance: cleanup FAILED:/ && $_ !~ /^acceptance: cleanup FAILED: injected cleanup failure\n$/) {$real=1} if (/^acceptance: cleanup verified: API PID absent$/) {$pid=1} if (/^acceptance: cleanup verified: no API listener on \d+$/) {$listener=1} if (/^acceptance: cleanup verified: no owned containers or volumes \(/) {$fixtures=1} END {exit($assert && $inject == 1 && !$real && $pid && $listener && $fixtures ? 0 : 1)}' || { echo 'control cleanup-failure FAIL: intended assertion, single injection or cleanup evidence mismatch' >&2; exit 1; }
  echo "control cleanup-failure passed: nested acceptance exit=$nested_status; intended assertion and one synthetic cleanup failure; all cleanup verified; wrapper exit=0"
  exit 0
fi
printf '%s\n' "$output" | perl -ne 'if (/^acceptance: cleanup FAILED:/) {$bad=1} END {exit($bad ? 0 : 1)}' && { echo "control $mutation FAIL: cleanup failed" >&2; exit 1; }
echo "control $mutation passed: nested acceptance exit=$nested_status at '$pattern'; cleanup verified; wrapper exit=0"
