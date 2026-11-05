#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
. "$root/scripts/autopilot-state.sh"
state_root=$tmp/state

assert() { "$@" || { echo "FAIL assertion: $1"; exit 1; }; }
report=$tmp/report
printf 'AUTOPILOT: CONTINUE\nwork pending\n' > "$report"
assert autopilot_validate_report "$report"
: > "$report"
if autopilot_validate_report "$report" >/dev/null 2>&1; then echo 'FAIL assertion: empty report rejected'; exit 1; fi
printf 'not a report\n' > "$report"
if autopilot_validate_report "$report" >/dev/null 2>&1; then echo 'FAIL assertion: malformed report rejected'; exit 1; fi

if autopilot_reserve_recovery 0Hoag/agent-ops 408 unitA >/dev/null; then :; else echo 'FAIL assertion: first recovery accepted'; exit 1; fi
if autopilot_reserve_recovery 0Hoag/agent-ops 408 renamed-unit >/dev/null 2>&1; then echo 'FAIL assertion: renamed identity refused'; exit 1; fi
if autopilot_reserve_recovery 0Hoag/agent-ops 408 unitA >/dev/null 2>&1; then echo 'FAIL assertion: second recovery refused'; exit 1; fi

# Mutate the production atomic reservation guard in a disposable copy. Identical inputs must now fail the test.
cp "$root/scripts/autopilot-state.sh" "$tmp/mutated.sh"
sed 's/if \[ -e "$file" \]; then/if false; then/' "$tmp/mutated.sh" > "$tmp/mutated2.sh"
if AUTOPILOT_STATE_DIR="$tmp/mut-state" bash -c '. "$1"; autopilot_reserve_recovery r 1 u >/dev/null; autopilot_reserve_recovery r 1 renamed >/dev/null 2>&1' _ "$tmp/mutated2.sh"; then
  echo 'FAIL assertion: mutation control did not detect disabled identity reservation'; exit 1
fi
echo 'PASS executable report validation and atomic stable-identity recovery reservation'

# Execute production wait logic without network, workers, merges, or real sleep.
mkdir -p "$tmp/repo/scripts" "$tmp/bin" "$tmp/repo/.agent-runs/coordinator"
cp "$root/scripts/autopilot.sh" "$tmp/repo/scripts/autopilot.sh"
cp "$root/scripts/autopilot-state.sh" "$tmp/repo/scripts/autopilot-state.sh"
cat > "$tmp/repo/scripts/worktrees.sh" <<'EOF'
#!/bin/sh
echo slept >> "$SLEEP_LOG"
touch .agent-runs/STOP
exit 1
exit 0
EOF
cp "$tmp/repo/scripts/worktrees.sh" "$tmp/repo/scripts/branches.sh"
cp "$tmp/repo/scripts/worktrees.sh" "$tmp/repo/scripts/audit.sh"
cat > "$tmp/bin/git" <<'EOF'
#!/bin/sh
[ "$1" = rev-parse ] && { echo "$FIXTURE_REPO"; exit 0; }
exec /usr/bin/git "$@"
EOF
cat > "$tmp/bin/opencode" <<'EOF'
#!/bin/sh
printf '%s\n' "$REPORT_JSON" > "$(find .agent-runs/coordinator -name '*.jsonl' | tail -1)"
EOF
cat > "$tmp/bin/jq" <<'EOF'
#!/bin/sh
case "$*" in
  *'select(.type=="text")'*) [ -n "$REPORT_TEXT" ] && printf '%s\n' "$REPORT_TEXT" || : ;;
  *'step_finish'*) echo '{}' ;;
  *) echo '0' ;;
esac
EOF
cat > "$tmp/bin/gh" <<'EOF'
#!/bin/sh
case "$*" in
  *'commits/main'*) echo mainsha ;;
  *'commits/reviews%2Fdevelopment'*) echo devsha ;;
  *'gh issue list'*|*'issue list'*) printf '%s\n' "${ISSUES:-[]}" ;;
  *'pr list'*) echo '[]' ;;
  *'run list'*) echo '[]' ;;
  *) echo 'fixture gh unexpected: $*' >&2; exit 1 ;;
esac
EOF
cat > "$tmp/bin/pgrep" <<'EOF'
#!/bin/sh
exit 1
EOF
cat > "$tmp/bin/sleep" <<'EOF'
#!/bin/sh
exit 0
EOF
cat > "$tmp/bin/opencode" <<'EOF'
#!/bin/sh
exit 0
EOF
chmod +x "$tmp/bin/"*
run_fixture() {
  (cd "$tmp/repo" && PATH="$tmp/bin:/usr/bin:/bin" FIXTURE_REPO="$tmp/repo" SLEEP_LOG="$tmp/sleeps" REPOS='r' RUN_DIRS="$tmp/none" \
    INTERVAL=1 POLL=1 MERGE_EVERY=100 MIN_GAP=0 REPORT_JSON='{}' REPORT_TEXT="$1" GH_FAIL="${GH_FAIL:-0}" \
    bash scripts/autopilot.sh) > "$tmp/output" 2>&1
}
cat > "$tmp/bin/gh" <<'EOF'
#!/bin/sh
case "$*" in
  *'commits/main'*) echo mainsha ;;
  *'commits/reviews%2Fdevelopment'*) echo devsha ;;
  *'issue list'*) [ "${GH_FAIL:-0}" = 1 ] && exit 1; echo '0' ;;
  *'pr list'*) echo '[]' ;;
  *'run list'*) echo '[]' ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$tmp/bin/gh"
if run_fixture 'AUTOPILOT: CONTINUE'; then :; else echo 'FAIL assertion: valid fixture pass'; cat "$tmp/output"; exit 1; fi
grep -q 'stalled: no worker, GitHub unchanged' "$tmp/output" || { echo 'FAIL assertion: blocked workload stalls'; cat "$tmp/output"; exit 1; }
if run_fixture ''; then echo 'FAIL assertion: empty final output fails safely'; exit 1; fi
grep -q 'invalid final output' "$tmp/output" || { echo 'FAIL assertion: empty report diagnostic'; cat "$tmp/output"; exit 1; }
echo 'PASS production loop fixture: all-blocked idle state stalls; empty final output stops safely'
GH_FAIL=1 run_fixture 'AUTOPILOT: CONTINUE' || :
cp "$tmp/output" "$tmp/api-output"
grep -q 'GitHub API fingerprint unavailable' "$tmp/api-output" || { echo 'FAIL assertion: API failure diagnostic'; cat "$tmp/api-output"; exit 1; }
echo 'PASS API failure stops explicitly, never interpreted as no work'

# Mutation control: disabling production stall return restores the old loop and must fail its behavior assertion.
cp "$root/scripts/autopilot.sh" "$tmp/repo/scripts/autopilot.sh"
sed 's/echo "  stalled: no worker, GitHub unchanged, no pending work"/echo "  idle: no worker, GitHub unchanged, skipping pass"/' "$tmp/repo/scripts/autopilot.sh" > "$tmp/repo/scripts/autopilot-mutated.sh"
mv "$tmp/repo/scripts/autopilot-mutated.sh" "$tmp/repo/scripts/autopilot.sh"
if (cd "$tmp/repo" && PATH="$tmp/bin:/usr/bin:/bin" FIXTURE_REPO="$tmp/repo" SLEEP_LOG="$tmp/mut-sleeps" REPOS=r RUN_DIRS="$tmp/none" INTERVAL=1 POLL=1 MIN_GAP=0 REPORT_JSON='{}' REPORT_TEXT='AUTOPILOT: CONTINUE' bash scripts/autopilot.sh) > "$tmp/mut-output" 2>&1; then
  grep -q 'idle: no worker, GitHub unchanged, skipping pass' "$tmp/mut-output" || { echo 'FAIL assertion: stall-guard mutation control'; exit 1; }
fi
grep -q 'idle: no worker' "$tmp/mut-output" || { echo 'FAIL assertion: mutation must reach idle branch'; cat "$tmp/mut-output"; exit 1; }
echo 'PASS mutation control: disabling production stalled return fails identical fixture'

# Preflight occurs before guarded spawn's reservation and detached launch.
if (cd "$tmp/repo" && PATH="$tmp/bin:/usr/bin:/bin" AUTOPILOT_STATE_DIR="$tmp/preflight-state" \
  bash "$root/scripts/spawn.sh" --recovery r 9 unit "$tmp/preflight.log" definitely-not-a-real-launcher); then
  echo 'FAIL assertion: guarded spawn preflight rejects missing launcher'; exit 1
fi
[ ! -d "$tmp/preflight-state" ] || { echo 'FAIL assertion: preflight failure consumed recovery'; exit 1; }
echo 'PASS recovery preflight failure leaves durable recovery allowance unused'
