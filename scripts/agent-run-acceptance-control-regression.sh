#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tmp=$(mktemp -d /private/tmp/agent-control-regression.XXXXXX)
trap 'rm -rf "$tmp"' EXIT

make_fixture() {
  local name=$1
  local fixture="$tmp/$name"
  mkdir -p "$fixture/scripts" "$fixture/api/modules/agentrun"
  cp "$ROOT/scripts/agent-run-acceptance-control.sh" "$fixture/scripts/"
  cp "$ROOT/api/modules/agentrun/"*.go "$fixture/api/modules/agentrun/"
  cat > "$fixture/scripts/agent-run-acceptance.sh" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'fixture invoked\n'
printf 'acceptance: FAIL: authenticated POST: expected HTTP 201\n'
status=1
[[ ${OMIT_PID:-0} == 1 ]] || printf 'acceptance: cleanup verified: API PID absent\n'
[[ ${OMIT_LISTENER:-0} == 1 ]] || printf 'acceptance: cleanup verified: no API listener on 28080\n'
[[ ${OMIT_FIXTURES:-0} == 1 ]] || printf 'acceptance: cleanup verified: no owned containers or volumes (fixture-project)\n'
[[ ${EXTRA_FAILURE:-0} == 1 ]] && printf 'acceptance: cleanup FAILED: injected additional cleanup failure\n'
exit "$status"
EOF
  chmod +x "$fixture/scripts/agent-run-acceptance.sh"
  git -C "$fixture" init -q
  git -C "$fixture" add .
  git -C "$fixture" -c user.name=fixture -c user.email=fixture@example.invalid commit -qm fixture
  printf '%s\n' "$fixture"
}

mutate_baseline() {
  local fixture=$1 mutation=$2
  python3 - "$fixture/scripts/agent-run-acceptance-control.sh" <<'PY'
import pathlib, re, sys
p = pathlib.Path(sys.argv[1]); s = p.read_text()
start = s.index('case "$mutation" in')
end = s.index('  noop)', start)
replacement = '''case "$mutation" in
  auth|validation|pricing|repository-list|repository-aggregate|wave-list|wave-aggregate|issue-list|issue-aggregate)
    target="$tmp/api/modules/agentrun/controller.go"; [[ $mutation == pricing || $mutation == *-aggregate || $mutation == *-list ]] && target="$tmp/api/modules/agentrun/repository.go"
    pattern='fixture no-op'
    ;;
'''
p.write_text(s[:start] + replacement + s[end:])
PY
  git -C "$fixture" add .
  git -C "$fixture" -c user.name=fixture -c user.email=fixture@example.invalid commit -qm "baseline $mutation"
}

valid_fixture=$(make_fixture valid)
out=$(cd "$valid_fixture" && bash scripts/agent-run-acceptance-control.sh auth 2>&1)
[[ "$out" == *'fixture invoked'* && "$out" == *'control auth passed:'* ]] || { printf '%s\n' "$out" >&2; exit 1; }
printf '%s\n' 'regression valid-evidence passed: ordinary wrapper accepted all three cleanup classes'

for control in auth validation pricing repository-list repository-aggregate wave-list wave-aggregate issue-list issue-aggregate; do
  fixture=$(make_fixture "unchanged-$control")
  mutate_baseline "$fixture" "$control"
  status=0
  out=$(cd "$fixture" && bash scripts/agent-run-acceptance-control.sh "$control" 2>&1) || status=$?
  [[ $status -eq 1 && "$out" == *'no-op mutation: target unchanged'* && "$out" != *'fixture invoked'* ]] || { printf '%s\n' "$out" >&2; exit 1; }
  printf 'regression unchanged-%s passed: rejected before nested fixture\n' "$control"
done

for spec in 'OMIT_PID=1:API PID absent' 'OMIT_LISTENER=1:no API listener' 'OMIT_FIXTURES=1:no owned containers or volumes'; do
  assignment=${spec%%:*}; expected=${spec#*:}; status=0
  out=$(cd "$valid_fixture" && env "$assignment" bash scripts/agent-run-acceptance-control.sh auth 2>&1) || status=$?
  [[ $status -eq 1 && "$out" == *'fixture invoked'* && "$out" != *"$expected"* ]] || { printf '%s\n' "$out" >&2; exit 1; }
  printf 'regression omitted-evidence (%s) passed: named cleanup assertion rejected fixture\n' "$expected"
done

status=0
out=$(cd "$valid_fixture" && EXTRA_FAILURE=1 bash scripts/agent-run-acceptance-control.sh auth 2>&1) || status=$?
[[ $status -eq 1 && "$out" == *'cleanup FAILED: injected additional cleanup failure'* ]] || { printf '%s\n' "$out" >&2; exit 1; }
printf '%s\n' 'regression extra-cleanup-failure passed: ordinary wrapper rejected additional failure'

disable_guard() {
  local fixture=$1 guard=$2
  python3 - "$fixture/scripts/agent-run-acceptance-control.sh" "$guard" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); guard = sys.argv[2]; s = p.read_text()
if guard == 'mutation':
    old = "if [[ $mutation == auth || $mutation == validation || $mutation == pricing || $mutation == *-list || $mutation == *-aggregate || $mutation == noop ]]; then"
    new = 'if false; then # mutation guard disabled for negative control'
elif guard == 'cleanup':
    old = "if [[ $mutation == auth || $mutation == validation || $mutation == pricing || $mutation == *-list || $mutation == *-aggregate ]]; then"
    new = 'if false; then # cleanup-evidence guard disabled for negative control'
else:
    raise SystemExit(f'unknown guard: {guard}')
if s.count(old) != 1:
    raise SystemExit(f'expected one {guard} guard')
p.write_text(s.replace(old, new, 1))
PY
}

for control in auth validation pricing repository-list repository-aggregate wave-list wave-aggregate issue-list issue-aggregate; do
  fixture=$(make_fixture "disabled-mutation-$control")
  mutate_baseline "$fixture" "$control"
  disable_guard "$fixture" mutation
  status=0
  out=$(cd "$fixture" && bash scripts/agent-run-acceptance-control.sh "$control" 2>&1) || status=$?
  [[ $status -eq 1 && "$out" == *"intended diagnostic 'fixture no-op' missing"* && "$out" == *'fixture invoked'* ]] || {
    printf '%s\n' "$out" >&2
    exit 1
  }
  printf 'regression disabled-mutation-%s passed: missing mutation guard failed named assertion\n' "$control"
done

for spec in 'OMIT_PID=1:API PID absent' 'OMIT_LISTENER=1:no API listener' 'OMIT_FIXTURES=1:no owned containers or volumes'; do
  assignment=${spec%%:*}; expected=${spec#*:}
  fixture=$(make_fixture "disabled-cleanup-${expected// /-}")
  disable_guard "$fixture" cleanup
  status=0
  out=$(cd "$fixture" && env "$assignment" bash scripts/agent-run-acceptance-control.sh auth 2>&1) || status=$?
  [[ $status -eq 0 && "$out" == *'control auth passed:'* && "$out" == *'fixture invoked'* ]] || {
    printf '%s\n' "$out" >&2
    exit 1
  }
  printf 'regression disabled-cleanup (%s) passed: missing cleanup guard produced a false pass\n' "$expected"
done
