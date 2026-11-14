#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
VERIFIER="$ROOT/scripts/verify/insights.sh"
BRIEF="$ROOT/briefs/W19h-insights-516.md"

fail() { printf 'insights-lock-regression: FAIL: %s\n' "$*" >&2; exit 1; }
pass() { printf 'insights-lock-regression: PASS: %s\n' "$*"; }

[[ -f "$VERIFIER" ]] || fail "missing verifier: $VERIFIER"
[[ -f "$BRIEF" ]] || fail "missing invocation brief: $BRIEF"
bash -n "$VERIFIER"

for command in \
  'bash scripts/verify/insights.sh' \
  'INSIGHTS_VERIFY_CONTROL=1 bash scripts/verify/insights.sh'; do
  grep -Fq "${command}" "$BRIEF" || fail "documented command missing: $command"
  if grep -F "scripts/heavy.sh $command" "$BRIEF" >/dev/null; then
    fail "documented command recursively wrapped: $command"
  fi
done

# The verifier must own the heavy boundary for each operation. This is a
# bounded structural regression for the non-reentrant global lock: the
# supported command strings are direct, while the verifier retains internal
# heavy calls. It never acquires or removes the shared lock itself.
heavy_calls=$(grep -cF '"$ROOT/scripts/heavy.sh"' "$VERIFIER")
(( heavy_calls >= 6 )) || fail "expected internal heavy serialization, found $heavy_calls calls"
if grep -Eq '(^|[[:space:]])scripts/heavy\.sh[[:space:]]+bash scripts/verify/insights\.sh' "$BRIEF"; then
  fail 'brief still documents an outer heavy wrapper'
fi
pass "normal and control invocations are direct; verifier retains $heavy_calls internal serialized heavy calls"
