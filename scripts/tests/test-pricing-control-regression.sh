#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ARTIFACT_DIR=${PRICING_CONTROL_ARTIFACT_DIR:-$ROOT/.agent-runs/pricing-control-regression-test}
TEST_TMP=$(mktemp -d "${TMPDIR:-/tmp}/pricing-control-test.XXXXXX")
OWNER_EVIDENCE="$TEST_TMP/owner-evidence"
VERIFIER_STUB="$TEST_TMP/verifier.sh"
cleanup_test() { rm -rf "$TEST_TMP"; }
trap cleanup_test EXIT INT TERM
mkdir -p "$OWNER_EVIDENCE"
printf 'owner evidence must survive\n' >"$OWNER_EVIDENCE/evidence.log"
before_evidence=$(sha256sum "$OWNER_EVIDENCE/evidence.log" | awk '{print $1}')
cat >"$VERIFIER_STUB" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
exit 0
EOF
chmod 755 "$VERIFIER_STUB"

if PRICING_CONTROL_ARTIFACT_DIR="$ARTIFACT_DIR" \
  PRICING_CONTROL_READONLY_FIXTURE="$OWNER_EVIDENCE" \
  PRICING_CONTROL_VERIFIER_SCRIPT="$VERIFIER_STUB" \
  bash "$ROOT/scripts/verify/pricing-control-regression.sh"; then
  echo 'expected refusal for pre-existing owner evidence' >&2
  exit 1
fi

after_evidence=$(sha256sum "$OWNER_EVIDENCE/evidence.log" | awk '{print $1}')
test "$before_evidence" = "$after_evidence"
test -e "$OWNER_EVIDENCE/evidence.log"
printf 'PASS pre-existing owner evidence refusal preserves identical evidence\n'
