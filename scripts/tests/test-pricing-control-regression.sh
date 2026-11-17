#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ARTIFACT_DIR=${PRICING_CONTROL_ARTIFACT_DIR:-$ROOT/.agent-runs/pricing-control-regression-test}
OWNER_EVIDENCE="$ROOT/.agent-runs/pricing-control-owner-evidence"
cleanup_test() { rm -rf "$OWNER_EVIDENCE"; }
trap cleanup_test EXIT INT TERM
mkdir -p "$OWNER_EVIDENCE"
printf 'owner evidence must survive\n' >"$OWNER_EVIDENCE/evidence.log"
before_evidence=$(sha256sum "$OWNER_EVIDENCE/evidence.log" | awk '{print $1}')

PRICING_CONTROL_ARTIFACT_DIR="$ARTIFACT_DIR" \
  bash "$ROOT/scripts/verify/pricing-control-regression.sh"

after_evidence=$(sha256sum "$OWNER_EVIDENCE/evidence.log" | awk '{print $1}')
test "$before_evidence" = "$after_evidence"
test -e "$OWNER_EVIDENCE/evidence.log"
test ! -e "$ROOT/.agent-runs/pricing-control-readonly-fixture"
grep -Fxq 'old_copy_control=fail (ignored evidence was copied)' "$ARTIFACT_DIR/summary.txt"
printf 'PASS pricing control bounded-source regression and cleanup\n'
