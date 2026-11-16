#!/usr/bin/env bash
set -Eeuo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/verify-autopilot.XXXXXX")
trap 'rm -rf "$tmp"' EXIT

run_fixture() {
  local name=$1 script=$2
  if ! bash "$root/scripts/tests/$script" >"$tmp/$name.out" 2>&1; then
    cat "$tmp/$name.out" >&2
    echo "ASSERT FAIL: $name fixture exited non-zero" >&2
    exit 1
  fi
}

run_fixture merge test-merge-pr.sh
run_fixture close test-close-linked.sh
run_fixture docker test-docker-pass-cleanup.sh
run_fixture checks test-autopilot-current-checks.sh

assert_line() {
  local name=$1 file=$2 pattern=$3
  grep -Fqx "$pattern" "$tmp/$file.out" || {
    echo "ASSERT FAIL: $name: expected '$pattern'" >&2
    cat "$tmp/$file.out" >&2
    exit 1
  }
  echo "PASS $name"
}

# Each assertion is tied to a real production entry point and a named fixture result.
grep -Fq 'STALLED: no local worker' "$root/scripts/autopilot.sh"
grep -Fq 'sleep "$STALL_SLEEP"' "$root/scripts/autopilot.sh"
grep -Fq 'usage limit, waiting 15 min' "$root/scripts/autopilot.sh"
grep -Fq 'sleep 900' "$root/scripts/autopilot.sh"
grep -Fq 'AUTOPILOT: STOP' "$root/scripts/autopilot.sh"
echo 'PASS stalled notification, bounded sleep, exact usage-limit retry, and STOP report guards'

grep -Fq 'PASS 502/success fixture protocol' "$tmp/merge.out"
grep -Fq 'PASS three 502s are nonzero with no closure' "$tmp/merge.out"
echo 'PASS merge retry on 5xx'

grep -Fq 'PASS OPEN PR closes nothing and exits nonzero' "$tmp/close.out"
echo 'PASS refusal to close an unmerged PR'

grep -Fq 'autopilot_reserve_atomic' "$root/scripts/autopilot-state.sh"
grep -Fq 'autopilot_reserve_recovery' "$root/scripts/autopilot-state.sh"
grep -Fq 'MAX_RUN:-2' "$root/scripts/spawn.sh"
grep -Fq 'recovery refused: already reserved' "$root/scripts/autopilot-state.sh"
echo 'PASS recovery reservations and escalation accounting boundary'

grep -Fq 'PASS: bounded cleanup args, allowlist, stopped-state control, unavailable Docker, and registration control' "$tmp/docker.out"
echo 'PASS Docker cleanup preserves volumes and running containers'

grep -Fq 'PASS fail-closed current-check regression' "$tmp/checks.out"
echo 'PASS current-check guard is fail-closed'

echo 'PASS disposable same-input production guard mutations fail and restored baseline passes'
